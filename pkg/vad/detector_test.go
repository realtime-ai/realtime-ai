//go:build vad

package vad

import (
	"os"
	"path/filepath"
	"testing"
)

func getModelPath(t *testing.T) string {
	// Try to find the model in common locations
	paths := []string{
		"../../models/silero_vad.onnx",
		"models/silero_vad.onnx",
		"/tmp/silero_vad.onnx",
	}

	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath
		}
	}

	t.Skip("silero_vad.onnx model not found, skipping test")
	return ""
}

func TestDetectorConfigIsValid(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DetectorConfig
		wantErr bool
	}{
		{
			name: "valid config 16kHz",
			cfg: DetectorConfig{
				ModelPath:  "/path/to/model.onnx",
				SampleRate: 16000,
			},
			wantErr: false,
		},
		{
			name: "valid config 8kHz",
			cfg: DetectorConfig{
				ModelPath:  "/path/to/model.onnx",
				SampleRate: 8000,
			},
			wantErr: false,
		},
		{
			name: "empty model path",
			cfg: DetectorConfig{
				ModelPath:  "",
				SampleRate: 16000,
			},
			wantErr: true,
		},
		{
			name: "invalid sample rate",
			cfg: DetectorConfig{
				ModelPath:  "/path/to/model.onnx",
				SampleRate: 44100,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.IsValid()
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewDetector(t *testing.T) {
	modelPath := getModelPath(t)

	cfg := DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: 16000,
		LogLevel:   LogLevelWarn,
	}

	detector, err := NewDetector(cfg)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	defer detector.Destroy()

	if detector == nil {
		t.Fatal("NewDetector() returned nil detector")
	}
}

func TestDetectorInfer(t *testing.T) {
	modelPath := getModelPath(t)

	cfg := DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: 16000,
		LogLevel:   LogLevelWarn,
	}

	detector, err := NewDetector(cfg)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	defer detector.Destroy()

	// Create 512 samples of silence (zeros)
	silence := make([]float32, 512)

	prob, err := detector.Infer(silence)
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}

	// Silence should have low speech probability
	if prob < 0 || prob > 1 {
		t.Errorf("Infer() probability = %v, want in range [0, 1]", prob)
	}

	t.Logf("Silence speech probability: %.4f", prob)
}

func TestDetectorInferWithSpeech(t *testing.T) {
	modelPath := getModelPath(t)

	cfg := DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: 16000,
		LogLevel:   LogLevelWarn,
	}

	detector, err := NewDetector(cfg)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	defer detector.Destroy()

	// Create 512 samples simulating speech-like signal (simple sine wave)
	samples := make([]float32, 512)
	for i := range samples {
		// Generate a 440Hz sine wave at 16kHz sample rate
		samples[i] = float32(0.5) * float32(i%36) / 18.0
		if i%36 >= 18 {
			samples[i] = float32(0.5) * float32(36-i%36) / 18.0
		}
	}

	prob, err := detector.Infer(samples)
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}

	if prob < 0 || prob > 1 {
		t.Errorf("Infer() probability = %v, want in range [0, 1]", prob)
	}

	t.Logf("Simulated signal speech probability: %.4f", prob)
}

func TestDetectorReset(t *testing.T) {
	modelPath := getModelPath(t)

	cfg := DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: 16000,
		LogLevel:   LogLevelWarn,
	}

	detector, err := NewDetector(cfg)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	defer detector.Destroy()

	// Run inference to change internal state
	samples := make([]float32, 512)
	_, err = detector.Infer(samples)
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}

	// Reset should not error
	err = detector.Reset()
	if err != nil {
		t.Errorf("Reset() error = %v", err)
	}
}

func TestDetectorNilSafety(t *testing.T) {
	var detector *Detector

	err := detector.Reset()
	if err == nil {
		t.Error("Reset() on nil detector should return error")
	}

	err = detector.Destroy()
	if err == nil {
		t.Error("Destroy() on nil detector should return error")
	}
}
