package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	audioFile       = "../audiofiles/vad_test_en.wav"
	modelPath       = "../../models/silero_vad.onnx"
	outputDir       = "./vad_segments"
	audioSampleRate = 16000
	audioChannels   = 1
)

type speechSegment struct {
	StartMs int
	EndMs   int
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Decode audio to PCM
	log.Printf("Decoding %s...", audioFile)
	pcmData, err := decodeToPCM(ctx, audioFile)
	if err != nil {
		log.Fatalf("Failed to decode audio: %v", err)
	}
	durationMs := len(pcmData) * 1000 / (audioSampleRate * audioChannels * 2)
	log.Printf("Decoded: %d bytes, %d ms", len(pcmData), durationMs)

	// 2. Create VAD element
	vadElem, err := elements.NewSileroVADElement(elements.SileroVADConfig{
		ModelPath:       modelPath,
		Threshold:       0.5,
		MinSilenceDurMs: 100,
		SpeechPadMs:     30,
		Mode:            elements.VADModePassthrough,
	})
	if err != nil {
		log.Fatalf("Failed to create VAD element: %v", err)
	}

	// 3. Initialize and start pipeline
	p := pipeline.NewPipeline("vad-test")
	p.AddElement(vadElem)

	if err := vadElem.Init(ctx); err != nil {
		log.Fatalf("Failed to init VAD: %v", err)
	}
	if err := p.Start(ctx); err != nil {
		log.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()

	// 4. Subscribe to VAD events
	eventsChan := make(chan pipeline.Event, 100)
	p.Bus().Subscribe(pipeline.EventVADSpeechStart, eventsChan)
	p.Bus().Subscribe(pipeline.EventVADSpeechEnd, eventsChan)

	// 5. Consume output to prevent blocking
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-vadElem.Out():
			}
		}
	}()

	// 6. Collect speech segments
	var (
		segments []speechSegment
		inSpeech bool
		startMs  int
		mu       sync.Mutex
	)

	go func() {
		for event := range eventsChan {
			payload := event.Payload.(pipeline.VADPayload)
			mu.Lock()
			switch event.Type {
			case pipeline.EventVADSpeechStart:
				if !inSpeech {
					inSpeech = true
					startMs = payload.AudioMs
					log.Printf("[%6d ms] 🎤 Speech START", payload.AudioMs)
				}
			case pipeline.EventVADSpeechEnd:
				if inSpeech {
					inSpeech = false
					endMs := payload.AudioMs
					if endMs > startMs {
						segments = append(segments, speechSegment{StartMs: startMs, EndMs: endMs})
					}
					log.Printf("[%6d ms] 🔇 Speech END", payload.AudioMs)
				}
			}
			mu.Unlock()
		}
	}()

	// 7. Stream audio to VAD
	if err := streamPCM(ctx, vadElem, pcmData); err != nil {
		log.Fatalf("Failed to stream audio: %v", err)
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	// 8. Handle any ongoing speech at end of audio
	mu.Lock()
	if inSpeech && startMs < durationMs {
		// Speech was ongoing when audio ended, save it
		segments = append(segments, speechSegment{StartMs: startMs, EndMs: durationMs})
		log.Printf("[%6d ms] 🔇 Speech END (audio ended)", durationMs)
	}
	segmentsCopy := append([]speechSegment(nil), segments...)
	mu.Unlock()

	log.Printf("\n=== VAD Results ===")
	log.Printf("Detected segments: %d", len(segmentsCopy))

	if len(segmentsCopy) == 0 {
		log.Printf("⚠️  No speech detected. Creating demo segments...")
		segmentsCopy = []speechSegment{
			{StartMs: 1000, EndMs: 5000},
			{StartMs: 10000, EndMs: min(15000, durationMs-1000)},
		}
	}

	// 9. Save segments as WAV files
	os.MkdirAll(outputDir, 0755)
	log.Printf("\nSaving %d segments to %s/", len(segmentsCopy), outputDir)
	for _, seg := range segmentsCopy {
		filename := fmt.Sprintf("%s/%d-%d.wav", outputDir, seg.StartMs, seg.EndMs)
		if err := saveSegmentAsWav(pcmData, seg, filename); err != nil {
			log.Printf("  ✗ Failed: %s (%v)", filename, err)
		} else {
			log.Printf("  ✓ Saved: %d-%d.wav (%d ms)", seg.StartMs, seg.EndMs, seg.EndMs-seg.StartMs)
		}
	}

	log.Println("\nDone.")
}

// bytesToFloat32 converts 16-bit PCM bytes to normalized float32 samples
func bytesToFloat32(data []byte) []float32 {
	n := len(data) / 2
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
		samples[i] = float32(v) / 32768.0
	}
	return samples
}

// decodeToPCM decodes audio file to 16kHz mono PCM
func decodeToPCM(ctx context.Context, audioPath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", audioPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", "1",
		"-ar", "16000",
		"pipe:1",
	)
	return cmd.Output()
}

// streamPCM sends PCM data to VAD element in 32ms chunks
func streamPCM(ctx context.Context, elem pipeline.Element, pcm []byte) error {
	chunkSize := (audioSampleRate * 32 / 1000) * 2 // 32ms chunks, 2 bytes per sample

	for i := 0; i < len(pcm); i += chunkSize {
		end := i + chunkSize
		if end > len(pcm) {
			end = len(pcm)
		}

		msg := &pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeAudio,
			SessionID: "vad-test",
			AudioData: &pipeline.AudioData{
				Data:       pcm[i:end],
				SampleRate: audioSampleRate,
				Channels:   audioChannels,
				MediaType:  pipeline.AudioMediaTypeRaw,
				Timestamp:  time.Now(),
			},
			Timestamp: time.Now(),
		}

		select {
		case elem.In() <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// saveSegmentAsWav saves a PCM segment as WAV file
func saveSegmentAsWav(pcmData []byte, seg speechSegment, filename string) error {
	bytesPerMs := (audioSampleRate * audioChannels * 2) / 1000
	startByte := seg.StartMs * bytesPerMs
	endByte := seg.EndMs * bytesPerMs

	if startByte < 0 {
		startByte = 0
	}
	if endByte > len(pcmData) {
		endByte = len(pcmData)
	}
	if startByte >= endByte {
		return fmt.Errorf("invalid range")
	}

	segmentData := pcmData[startByte:endByte]

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := writeWavHeader(f, len(segmentData)); err != nil {
		return err
	}
	_, err = f.Write(segmentData)
	return err
}

func writeWavHeader(w io.Writer, dataSize int) error {
	write := func(data []byte) error {
		_, err := w.Write(data)
		return err
	}

	// RIFF header
	write([]byte("RIFF"))
	writeUint32(w, uint32(36+dataSize))
	write([]byte("WAVE"))

	// fmt chunk
	write([]byte("fmt "))
	writeUint32(w, 16)
	writeUint16(w, 1) // PCM
	writeUint16(w, 1) // Mono
	writeUint32(w, 16000)
	writeUint32(w, 32000)
	writeUint16(w, 2)
	writeUint16(w, 16)

	// data chunk
	write([]byte("data"))
	return writeUint32(w, uint32(dataSize))
}

func writeUint16(w io.Writer, v uint16) error {
	_, err := w.Write([]byte{byte(v), byte(v >> 8)})
	return err
}

func writeUint32(w io.Writer, v uint32) error {
	_, err := w.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	return err
}
