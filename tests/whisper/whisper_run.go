package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
	chunkDuration   = 200 * time.Millisecond
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		log.Fatalf("failed to locate repo root: %v", err)
	}

	if err := loadRootEnv(repoRoot); err != nil {
		log.Fatalf("failed to load .env: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required – set it in the root .env or environment")
	}

	audioPath, err := ensureTestAudio(repoRoot)
	if err != nil {
		log.Fatalf("failed to locate test audio: %v", err)
	}

	whisperConfig := elements.WhisperSTTConfig{
		APIKey:               apiKey,
		Language:             "",
		Model:                "whisper-1",
		EnablePartialResults: true,
		VADEnabled:           false,
		SampleRate:           audioSampleRate,
		Channels:             audioChannels,
		BitsPerSample:        bitsPerSample,
	}

	elem, err := elements.NewWhisperSTTElement(whisperConfig)
	if err != nil {
		log.Fatalf("failed to create whisper element: %v", err)
	}

	if err := elem.Start(ctx); err != nil {
		log.Fatalf("failed to start whisper element: %v", err)
	}
	defer func() {
		if stopErr := elem.Stop(); stopErr != nil {
			log.Printf("failed to stop whisper element cleanly: %v", stopErr)
		}
	}()

	log.Printf("Streaming %s to Whisper (model=%s)...", audioPath, whisperConfig.Model)
	finalText, err := transcribeFile(ctx, elem, audioPath)
	if err != nil {
		log.Fatalf("transcription failed: %v", err)
	}

	log.Println("=== Final transcription ===")
	fmt.Println(finalText)
}

func transcribeFile(ctx context.Context, elem *elements.WhisperSTTElement, audioPath string) (string, error) {
	pcmData, err := decodeToPCM(ctx, audioPath, audioSampleRate, audioChannels)
	if err != nil {
		return "", err
	}

	finalCh := make(chan string, 1)
	go collectResults(ctx, elem, finalCh)

	if err := streamPCMToElement(ctx, elem, pcmData); err != nil {
		return "", err
	}

	select {
	case text := <-finalCh:
		if strings.TrimSpace(text) == "" {
			return "", errors.New("received empty transcription")
		}
		return text, nil
	case <-time.After(1 * time.Minute):
		return "", errors.New("timed out waiting for transcription")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func streamPCMToElement(ctx context.Context, elem *elements.WhisperSTTElement, pcm []byte) error {
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
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
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

func collectResults(ctx context.Context, elem *elements.WhisperSTTElement, finalCh chan<- string) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-elem.Out():
			if !ok {
				return
			}

			if msg.Type != pipeline.MsgTypeData || msg.TextData == nil {
				continue
			}

			text := strings.TrimSpace(string(msg.TextData.Data))
			if text == "" {
				continue
			}

			log.Printf("[%s] %s", msg.TextData.TextType, text)

			if msg.TextData.TextType == "text/final" {
				select {
				case finalCh <- text:
				default:
				}
				return
			}
		}
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("unable to find repository root")
}

func loadRootEnv(root string) error {
	envPath := filepath.Join(root, ".env")
	if _, err := os.Stat(envPath); err != nil {
		return fmt.Errorf(".env not found at %s: %w", envPath, err)
	}

	return godotenv.Load(envPath)
}

func ensureTestAudio(root string) (string, error) {
	audioPath := filepath.Join(root, "tests", "whisper", "test.m4a")
	if _, err := os.Stat(audioPath); err != nil {
		return "", fmt.Errorf("test audio not found: %w", err)
	}
	return audioPath, nil
}
