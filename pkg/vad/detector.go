// Package vad provides Voice Activity Detection using Silero VAD model.
//
// This package uses onnxruntime_go for ONNX model inference, replacing the
// previous CGO-based implementation.
//
// Usage:
//
//	// Initialize the ONNX runtime (call once at startup)
//	if err := vad.InitRuntime(""); err != nil {
//	    log.Fatal(err)
//	}
//	defer vad.DestroyRuntime()
//
//	// Create a detector
//	detector, err := vad.NewDetector(vad.DetectorConfig{
//	    ModelPath:  "path/to/silero_vad.onnx",
//	    SampleRate: 16000,
//	})
//
//go:build vad

package vad

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	stateLen   = 2 * 1 * 128
	contextLen = 64
)

// LogLevel represents the ONNX Runtime logging level.
type LogLevel int

const (
	LevelVerbose LogLevel = iota + 1
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

// runtimeInitialized tracks whether the ONNX runtime has been initialized.
var (
	runtimeInitialized bool
	runtimeMu          sync.Mutex
)

// InitRuntime initializes the ONNX runtime environment.
// libraryPath can be empty to use auto-detection, or specify the path to libonnxruntime.so.
// This should be called once at application startup before creating any detectors.
func InitRuntime(libraryPath string) error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()

	if runtimeInitialized {
		return nil
	}

	if libraryPath != "" {
		ort.SetSharedLibraryPath(libraryPath)
	} else {
		// Try to find the library in common locations
		libPath := findONNXRuntimeLibrary()
		if libPath != "" {
			ort.SetSharedLibraryPath(libPath)
		}
	}

	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("failed to initialize ONNX runtime: %w", err)
	}

	runtimeInitialized = true
	return nil
}

// DestroyRuntime destroys the ONNX runtime environment.
// This should be called once at application shutdown.
func DestroyRuntime() error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()

	if !runtimeInitialized {
		return nil
	}

	if err := ort.DestroyEnvironment(); err != nil {
		return fmt.Errorf("failed to destroy ONNX runtime: %w", err)
	}

	runtimeInitialized = false
	return nil
}

// findONNXRuntimeLibrary tries to find the ONNX Runtime shared library.
func findONNXRuntimeLibrary() string {
	// Common paths to check
	paths := []string{
		// Environment variable
		os.Getenv("ONNXRUNTIME_LIB"),
		// Linux system paths
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/opt/onnxruntime/lib/libonnxruntime.so",
		// macOS Homebrew paths
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
	}

	// Also check LD_LIBRARY_PATH
	if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
		for _, dir := range filepath.SplitList(ldPath) {
			paths = append(paths, filepath.Join(dir, "libonnxruntime.so"))
		}
	}

	// Check DYLD_LIBRARY_PATH for macOS
	if dyldPath := os.Getenv("DYLD_LIBRARY_PATH"); dyldPath != "" {
		for _, dir := range filepath.SplitList(dyldPath) {
			paths = append(paths, filepath.Join(dir, "libonnxruntime.dylib"))
		}
	}

	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// DetectorConfig holds configuration for creating a VAD detector.
type DetectorConfig struct {
	// The path to the ONNX Silero VAD model file to load.
	ModelPath string
	// The sampling rate of the input audio samples. Supported values are 8000 and 16000.
	SampleRate int
	// The loglevel for the onnx environment, by default it is set to LogLevelWarn.
	LogLevel LogLevel
}

// IsValid validates the detector configuration.
func (c DetectorConfig) IsValid() error {
	if c.ModelPath == "" {
		return fmt.Errorf("invalid ModelPath: should not be empty")
	}

	if c.SampleRate != 8000 && c.SampleRate != 16000 {
		return fmt.Errorf("invalid SampleRate: valid values are 8000 and 16000")
	}

	return nil
}

// Detector provides voice activity detection using the Silero VAD model.
type Detector struct {
	session *ort.DynamicAdvancedSession

	cfg DetectorConfig

	// RNN state (h, c) for the LSTM layers
	state [stateLen]float32
	// Context buffer for continuous processing
	ctx [contextLen]float32
	// currSample tracks total samples processed, used to determine if context should be applied.
	// On the first inference (currSample == 0), no context is prepended.
	currSample int

	// Pre-allocated tensors for inference
	inputNames  []string
	outputNames []string
}

// NewDetector creates a new VAD detector with the given configuration.
// InitRuntime must be called before creating a detector.
func NewDetector(cfg DetectorConfig) (*Detector, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Ensure runtime is initialized
	runtimeMu.Lock()
	if !runtimeInitialized {
		runtimeMu.Unlock()
		// Auto-initialize runtime
		if err := InitRuntime(""); err != nil {
			return nil, fmt.Errorf("ONNX runtime not initialized: %w", err)
		}
	} else {
		runtimeMu.Unlock()
	}

	sd := &Detector{
		cfg:         cfg,
		inputNames:  []string{"input", "state", "sr"},
		outputNames: []string{"output", "stateN"},
	}

	// Create session options
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to create session options: %w", err)
	}
	defer options.Destroy()

	// Set graph optimization level
	if err := options.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll); err != nil {
		return nil, fmt.Errorf("failed to set graph optimization level: %w", err)
	}

	// Set number of threads
	if err := options.SetIntraOpNumThreads(1); err != nil {
		return nil, fmt.Errorf("failed to set intra-op threads: %w", err)
	}
	if err := options.SetInterOpNumThreads(1); err != nil {
		return nil, fmt.Errorf("failed to set inter-op threads: %w", err)
	}

	// Create dynamic session (allows variable input sizes)
	session, err := ort.NewDynamicAdvancedSession(
		cfg.ModelPath,
		sd.inputNames,
		sd.outputNames,
		options,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	sd.session = session
	return sd, nil
}

// Infer runs inference on audio samples and returns the speech probability.
// samples should be normalized float32 values in the range [-1, 1].
// Returns a probability value in [0, 1] where higher values indicate speech.
func (sd *Detector) Infer(samples []float32) (float32, error) {
	if sd == nil {
		return 0, fmt.Errorf("invalid nil detector")
	}

	// Handle context: prepend previous samples for continuity (except on first call)
	pcm := samples
	if sd.currSample > 0 {
		// Append context from previous iteration
		pcm = append(sd.ctx[:], samples...)
	}
	// Save the last contextLen samples as context for the next iteration
	if len(samples) >= contextLen {
		copy(sd.ctx[:], samples[len(samples)-contextLen:])
	}
	sd.currSample += len(samples)

	// Create input tensor for audio
	inputShape := ort.NewShape(1, int64(len(pcm)))
	inputTensor, err := ort.NewTensor(inputShape, pcm)
	if err != nil {
		return 0, fmt.Errorf("failed to create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	// Create state tensor
	stateShape := ort.NewShape(2, 1, 128)
	stateTensor, err := ort.NewTensor(stateShape, sd.state[:])
	if err != nil {
		return 0, fmt.Errorf("failed to create state tensor: %w", err)
	}
	defer stateTensor.Destroy()

	// Create sample rate tensor
	srShape := ort.NewShape(1)
	srData := []int64{int64(sd.cfg.SampleRate)}
	srTensor, err := ort.NewTensor(srShape, srData)
	if err != nil {
		return 0, fmt.Errorf("failed to create sr tensor: %w", err)
	}
	defer srTensor.Destroy()

	// Create output tensors
	outputShape := ort.NewShape(1, 1)
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return 0, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	stateNShape := ort.NewShape(2, 1, 128)
	stateNTensor, err := ort.NewEmptyTensor[float32](stateNShape)
	if err != nil {
		return 0, fmt.Errorf("failed to create stateN tensor: %w", err)
	}
	defer stateNTensor.Destroy()

	// Run inference
	inputs := []ort.Value{inputTensor, stateTensor, srTensor}
	outputs := []ort.Value{outputTensor, stateNTensor}

	if err := sd.session.Run(inputs, outputs); err != nil {
		return 0, fmt.Errorf("failed to run inference: %w", err)
	}

	// Update state from output
	stateNData := stateNTensor.GetData()
	copy(sd.state[:], stateNData)

	// Return speech probability
	outputData := outputTensor.GetData()
	if len(outputData) == 0 {
		return 0, fmt.Errorf("empty output from inference")
	}

	return outputData[0], nil
}

// Reset resets the detector's internal state.
// This should be called when starting a new audio stream.
func (sd *Detector) Reset() error {
	if sd == nil {
		return fmt.Errorf("invalid nil detector")
	}

	for i := range stateLen {
		sd.state[i] = 0
	}
	for i := range contextLen {
		sd.ctx[i] = 0
	}
	sd.currSample = 0

	return nil
}

// Destroy releases all resources held by the detector.
// The detector should not be used after calling Destroy.
func (sd *Detector) Destroy() error {
	if sd == nil {
		return fmt.Errorf("invalid nil detector")
	}

	if sd.session != nil {
		if err := sd.session.Destroy(); err != nil {
			return fmt.Errorf("failed to destroy session: %w", err)
		}
		sd.session = nil
	}

	return nil
}

// Ensure Detector implements DetectorInterface at compile time.
var _ DetectorInterface = (*Detector)(nil)
