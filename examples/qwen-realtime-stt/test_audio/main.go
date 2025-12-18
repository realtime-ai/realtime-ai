package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	audioSampleRate = 16000
	audioChannels   = 1
	bitsPerSample   = 16
	chunkDuration   = 100 * time.Millisecond
)

func main() {
	// Load .env from project root
	godotenv.Load()

	if os.Getenv("DASHSCOPE_API_KEY") == "" {
		log.Fatal("DASHSCOPE_API_KEY environment variable is required")
	}

	audioPath := "examples/qwen-realtime-stt/test.m4a"
	if _, err := os.Stat(audioPath); err != nil {
		log.Fatalf("Test audio not found at %s: %v", audioPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Create pipeline
	p := pipeline.NewPipeline("test-pipeline")

	// 2. Qwen Realtime STT Element
	qwenConfig := elements.QwenRealtimeSTTConfig{
		APIKey:               os.Getenv("DASHSCOPE_API_KEY"),
		Language:             "zh",
		Model:                "qwen3-asr-flash-realtime",
		EnablePartialResults: true,
		VADEnabled:           false, // Manual commit for file test
		SampleRate:           audioSampleRate,
		Channels:             audioChannels,
		BitsPerSample:        bitsPerSample,
	}

	qwenElement, err := elements.NewQwenRealtimeSTTElement(qwenConfig)
	if err != nil {
		log.Fatalf("Failed to create Qwen Realtime STT element: %v", err)
	}
	p.AddElement(qwenElement)

	// 3. Subscribe to events
	subscribeToEvents(p)

	// 4. Start pipeline
	if err := p.Start(ctx); err != nil {
		log.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()

	// 5. Decode audio and stream to pipeline
	log.Printf("Decoding and streaming %s...", audioPath)
	pcmData, err := decodeToPCM(ctx, audioPath, audioSampleRate, audioChannels)
	if err != nil {
		log.Fatalf("Failed to decode audio: %v", err)
	}

	if err := streamPCMToPipeline(ctx, qwenElement, pcmData); err != nil {
		log.Fatalf("Failed to stream audio: %v", err)
	}

	// 6. Explicitly commit to get final result
	log.Println("Committing audio buffer...")
	if err := qwenElement.Commit(ctx); err != nil {
		log.Printf("Failed to commit: %v", err)
	}

	// 7. Wait for a bit to get results
	log.Println("Waiting for results...")
	time.Sleep(5 * time.Second)
	log.Println("Done.")
}

func decodeToPCM(ctx context.Context, audioPath string, sampleRate, channels int) ([]byte, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", audioPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", strconv.Itoa(channels),
		"-ar", strconv.Itoa(sampleRate),
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("ffmpeg failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	return output, nil
}

func streamPCMToPipeline(ctx context.Context, elem pipeline.Element, pcm []byte) error {
	chunkSize := int(float64(audioSampleRate*audioChannels*2) * (float64(chunkDuration) / float64(time.Second)))
	if chunkSize <= 0 {
		chunkSize = len(pcm)
	}

	for offset := 0; offset < len(pcm); offset += chunkSize {
		end := offset + chunkSize
		if end > len(pcm) {
			end = len(pcm)
		}

		chunk := make([]byte, end-offset)
		copy(chunk, pcm[offset:end])

		msg := &pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeAudio,
			Timestamp: time.Now(),
			AudioData: &pipeline.AudioData{
				Data:       chunk,
				SampleRate: audioSampleRate,
				Channels:   audioChannels,
				MediaType:  "audio/x-raw",
				Timestamp:  time.Now(),
			},
		}

		select {
		case elem.In() <- msg:
			// Control streaming speed to simulate real-time
			time.Sleep(chunkDuration / 2) 
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func subscribeToEvents(p *pipeline.Pipeline) {
	bus := p.Bus()
	if bus == nil {
		return
	}

	// Subscribe to STT events
	sttEventsChan := make(chan pipeline.Event, 10)
	bus.Subscribe(pipeline.EventPartialResult, sttEventsChan)
	bus.Subscribe(pipeline.EventFinalResult, sttEventsChan)

	// Handle STT events
	go func() {
		for event := range sttEventsChan {
			if text, ok := event.Payload.(string); ok {
				switch event.Type {
				case pipeline.EventPartialResult:
					log.Printf("[Partial] %s", text)
				case pipeline.EventFinalResult:
					log.Printf("[Final] %s", text)
				}
			}
		}
	}()
}

