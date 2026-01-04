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
	// ElevenLabs requires 16kHz mono audio
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

	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required – set it in the root .env or environment")
	}

	audioPath, err := ensureTestAudio(repoRoot)
	if err != nil {
		log.Fatalf("failed to locate test audio: %v", err)
	}

	// Get language from args or default to auto
	language := "auto"
	if len(os.Args) > 1 {
		language = os.Args[1]
	}

	elevenLabsConfig := elements.ElevenLabsRealtimeSTTConfig{
		APIKey:               apiKey,
		Language:             language,
		Model:                "scribe_v2_realtime", // Use default scribe_v2_realtime
		EnablePartialResults: true,
		VADEnabled:           false, // No VAD for this test
		SampleRate:           audioSampleRate,
		Channels:             audioChannels,
		BitsPerSample:        bitsPerSample,
	}

	elem, err := elements.NewElevenLabsRealtimeSTTElement(elevenLabsConfig)
	if err != nil {
		log.Fatalf("failed to create ElevenLabs STT element: %v", err)
	}

	if err := elem.Start(ctx); err != nil {
		log.Fatalf("failed to start ElevenLabs STT element: %v", err)
	}
	defer func() {
		if stopErr := elem.Stop(); stopErr != nil {
			log.Printf("failed to stop element cleanly: %v", stopErr)
		}
	}()

	log.Printf("Streaming %s to ElevenLabs Scribe (language=%s)...", audioPath, language)
	finalText, err := transcribeFile(ctx, elem, audioPath)
	if err != nil {
		log.Fatalf("transcription failed: %v", err)
	}

	log.Println("=== Final transcription ===")
	fmt.Println(finalText)
}

func transcribeFile(ctx context.Context, elem *elements.ElevenLabsRealtimeSTTElement, audioPath string) (string, error) {
	pcmData, err := decodeToPCM(ctx, audioPath, audioSampleRate, audioChannels)
	if err != nil {
		return "", err
	}

	log.Printf("Decoded %d bytes of PCM audio (%.2f seconds)", len(pcmData), float64(len(pcmData))/(float64(audioSampleRate)*2))

	finalCh := make(chan string, 1)
	go collectResults(ctx, elem, finalCh)

	if err := streamPCMToElement(ctx, elem, pcmData); err != nil {
		return "", err
	}

	// Wait a bit for processing, then commit to trigger final transcription
	time.Sleep(500 * time.Millisecond)
	log.Printf("Committing audio to trigger final transcription...")
	if err := elem.Commit(ctx); err != nil {
		log.Printf("Warning: commit failed: %v", err)
	}

	select {
	case text := <-finalCh:
		if strings.TrimSpace(text) == "" {
			return "", errors.New("received empty transcription")
		}
		return text, nil
	case <-time.After(30 * time.Second):
		return "", errors.New("timed out waiting for transcription")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func streamPCMToElement(ctx context.Context, elem *elements.ElevenLabsRealtimeSTTElement, pcm []byte) error {
	chunkSize := int(float64(audioSampleRate*audioChannels*2) * (float64(chunkDuration) / float64(time.Second)))
	if chunkSize <= 0 {
		chunkSize = len(pcm)
	}

	log.Printf("Streaming in chunks of %d bytes (%.1fms each)", chunkSize, float64(chunkDuration)/float64(time.Millisecond))

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

		// Simulate real-time streaming
		time.Sleep(chunkDuration / 2)
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

func collectResults(ctx context.Context, elem *elements.ElevenLabsRealtimeSTTElement, finalCh chan<- string) {
	var lastText string

	for {
		select {
		case <-ctx.Done():
			// Send last text if we have any
			if lastText != "" {
				select {
				case finalCh <- lastText:
				default:
				}
			}
			return
		case msg, ok := <-elem.Out():
			if !ok {
				// Channel closed, send last text
				if lastText != "" {
					select {
					case finalCh <- lastText:
					default:
					}
				}
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

			// Always update lastText
			lastText = text

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

	return godotenv.Overload(envPath)
}

func ensureTestAudio(root string) (string, error) {
	// Try different test audio locations
	candidates := []string{
		filepath.Join(root, "tests", "elevenlabs_stt", "test.m4a"),
		filepath.Join(root, "tests", "whisper", "test.m4a"),
		filepath.Join(root, "tests", "audiofiles", "hello_zh.wav"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("test audio not found, tried: %v", candidates)
}
