package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	openairt "github.com/WqyJh/go-openai-realtime"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := openairt.NewClient(os.Getenv("OPENAI_API_KEY"))
	conn, err := client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Teletype response
	responseDeltaHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseAudioTranscriptDelta:
			fmt.Printf(event.(openairt.ResponseAudioTranscriptDeltaEvent).Delta)
		}
	}

	// Full response
	responseHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseAudioTranscriptDone:
			fmt.Printf("\n\n[full] %s\n\n", event.(openairt.ResponseAudioTranscriptDoneEvent).Transcript)
			fmt.Print("> ")
		}
	}

	// All event handler
	allHandler := func(ctx context.Context, event openairt.ServerEvent) {
		// data, err := json.Marshal(event)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// fmt.Printf("[%s] %s\n\n", event.ServerEventType(), string(data))

		// fmt.Printf("[%s]\n\n", event.ServerEventType())
	}

	datas := make(chan []byte, 100)

	// Audio response
	audioResponseHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseAudioDelta:
			msg := event.(openairt.ResponseAudioDeltaEvent)
			// log.Printf("audioResponseHandler: %v", delta)
			data, err := base64.StdEncoding.DecodeString(msg.Delta)
			if err != nil {
				log.Fatal(err)
			}
			// os.WriteFile(fmt.Sprintf("%s.pcm", msg.EventID), data, 0o644)
			// streamer.Append(data)
			datas <- data
		case openairt.ServerEventTypeResponseAudioDone:
			fulldata := []byte{}
			close(datas)
			for data := range datas {
				fulldata = append(fulldata, data...)
			}
			// os.WriteFile(fmt.Sprintf("%s.pcm", event.(openairt.ResponseAudioDoneEvent).EventID), fulldata, 0o644)

			log.Println("audioResponseHandler: ", fulldata)
		}
	}

	connHandler := openairt.NewConnHandler(ctx, conn, allHandler, responseDeltaHandler, responseHandler, audioResponseHandler)
	connHandler.Start()

	err = conn.SendMessage(ctx, &openairt.SessionUpdateEvent{
		Session: openairt.ClientSession{
			Modalities:        []openairt.Modality{openairt.ModalityText, openairt.ModalityAudio},
			Voice:             openairt.VoiceShimmer,
			OutputAudioFormat: openairt.AudioFormatPcm16,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Conversation")
	fmt.Println("---------------------")
	fmt.Print("> ")
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		if s.Text() == "exit" || s.Text() == "quit" {
			break
		}
		fmt.Println("---------------------")
		conn.SendMessage(ctx, &openairt.ConversationItemCreateEvent{
			Item: openairt.MessageItem{
				ID:     openairt.GenerateID("msg_", 10),
				Status: openairt.ItemStatusCompleted,
				Type:   openairt.MessageItemTypeMessage,
				Role:   openairt.MessageRoleUser,
				Content: []openairt.MessageContentPart{
					{
						Type: openairt.MessageContentTypeInputText,
						Text: s.Text(),
					},
				},
			},
		})
		conn.SendMessage(ctx, &openairt.ResponseCreateEvent{
			Response: openairt.ResponseCreateParams{
				Modalities:      []openairt.Modality{openairt.ModalityText, openairt.ModalityAudio},
				MaxOutputTokens: 4000,
			},
		})
	}
}
