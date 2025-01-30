package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/google/uuid"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/utils"
	"google.golang.org/genai"
)

const (
	sampleRate    = 48000
	channels      = 2
	frameSize     = 960      // 20ms @ 48kHz
	opusFrameSize = 960      // 20ms @ 48kHz
	maxDataBytes  = 1000 * 2 // Buffer for Opus encoded data
)

// WebRTCServer manages WebRTC connections
type WebRTCServer struct {
	sync.RWMutex
	peers map[string]*PeerConnection
}

// 创建一个PeerConnection 封装
type PeerConnection struct {
	id             string
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	metadata       map[string]interface{}
	remoteAudio    *webrtc.TrackRemote
	localAudio     *webrtc.TrackLocalStaticSample
	// 初始化一个genai client
	genaiSession *genai.Session
	// 初始化一个audio buffer
	audioBuffer *audio.PlayoutBuffer
}

// NewWebRTCServer creates a new WebRTC server instance
func NewWebRTCServer() *WebRTCServer {
	return &WebRTCServer{
		peers: make(map[string]*PeerConnection),
	}
}

// HandleNegotiate handles the WebRTC negotiation endpoint
func (s *WebRTCServer) HandleNegotiate(w http.ResponseWriter, r *http.Request) {
	// 添加 CORS 头
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	fmt.Println("request: ", r.Method)

	// 处理 OPTIONS 请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	offer := webrtc.SessionDescription{}
	if err := json.Unmarshal(body, &offer); err != nil {
		http.Error(w, "Failed to parse offer", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Create WebRTC configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		http.Error(w, "Failed to create peer connection", http.StatusInternalServerError)
		return
	}

	peer := &PeerConnection{
		id:             uuid.New().String(),
		peerConnection: peerConnection,
		dataChannel:    nil,
		metadata:       make(map[string]interface{}),
	}

	audioBuffer, err := audio.NewPlayoutBuffer()
	if err != nil {
		log.Fatal("create audio buffer error: ", err)
		http.Error(w, "Failed to create audio buffer", http.StatusInternalServerError)
		return
	}
	peer.audioBuffer = audioBuffer

	// Set up data channel handler
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {

		log.Printf("data channel: %+v", d)
		peer.dataChannel = d
		go s.HandleDataChannel(ctx, peer)
	})

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Received track: %+v", track)
		peer.remoteAudio = track
		go s.HandleRemoteAudio(ctx, peer)
	})

	audioTrack, audioTrackErr := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if audioTrackErr != nil {
		panic(audioTrackErr)
	}
	peer.localAudio = audioTrack

	peerConnection.AddTransceiverFromTrack(peer.localAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})

	go s.HandleLocalAudio(ctx, peer)

	// Set the remote SessionDescription
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		http.Error(w, "Failed to set remote description", http.StatusInternalServerError)
		return
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		return
	}

	// Set local SessionDescription
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		http.Error(w, "Failed to set local description", http.StatusInternalServerError)
		return
	}

	apiKey := os.Getenv("GOOGLE_API_KEY")

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGoogleAI})
	if err != nil {
		log.Fatal("create client error: ", err)
		http.Error(w, "Failed to create client", http.StatusInternalServerError)
		return
	}

	session, err := client.Live.Connect("gemini-2.0-flash-exp", &genai.LiveConnectConfig{
		ResponseModalities: []string{"AUDIO"},
	})
	if err != nil {
		log.Fatal("connect to model error: ", err)
		http.Error(w, "Failed to connect to model", http.StatusInternalServerError)
		return
	}

	peer.genaiSession = session

	go s.HandleSession(ctx, peer)

	// Wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	<-gatherComplete

	s.Lock()
	s.peers[peer.id] = peer
	s.Unlock()

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Received track: %s\n", track.ID())

		if track.Kind() == webrtc.RTPCodecTypeAudio {
			log.Printf("Received audio track: %+v", track)
			peer.remoteAudio = track
			go s.HandleRemoteAudio(ctx, peer)
		}
	})

	// Marshal and send the answer
	w.Header().Set("Content-Type", "application/sdp")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(peerConnection.LocalDescription())
}

func (s *WebRTCServer) HandleSession(ctx context.Context, peer *PeerConnection) {
	var dumper *audio.Dumper
	if os.Getenv("DUMP_SESSION_AUDIO") == "true" {
		var err error
		dumper, err = audio.NewDumper("session", 24000, 1)
		if err != nil {
			log.Printf("创建 session dumper 失败: %v\n", err)
		} else {
			defer dumper.Close()
		}
	}

	for {
		message, err := peer.genaiSession.Receive()
		if err != nil {
			log.Fatal("receive model response error: ", err)
		}

		log.Printf("Received message: %+v\n", message)

		// 解析audio
		if message.ServerContent != nil {
			if message.ServerContent.ModelTurn != nil {
				for _, part := range message.ServerContent.ModelTurn.Parts {
					if part.Text != "" {
						log.Printf("model turn: %+v", part.Text)
					}

					if part.InlineData != nil {
						log.Printf("model turn: recieve audio data: %d", len(part.InlineData.Data))
						err := peer.audioBuffer.Write(part.InlineData.Data)
						if err != nil {
							log.Fatal("write audio data error: ", err)
						}

						if dumper != nil {
							dumper.Write(part.InlineData.Data)
						}
					}
				}
			}

			message.ServerContent.ModelTurn = nil
			if message.ServerContent.Interrupted {
				log.Printf("model turn: interrupted")
				peer.audioBuffer.Clear()
			}
		}

		messageBytes, err := json.Marshal(message)
		if err != nil {
			log.Fatal("marhal model response error: ", message, err)
		}
		err = peer.dataChannel.Send(messageBytes)
		if err != nil {
			log.Println("write message error: ", err)
			break
		}
	}
}

func (s *WebRTCServer) HandleRemoteAudio(ctx context.Context, peer *PeerConnection) {
	var dumper *audio.Dumper
	if os.Getenv("DUMP_REMOTE_AUDIO") == "true" {
		var err error
		dumper, err = audio.NewDumper("remoteaudio", 48000, 1)
		if err != nil {
			log.Printf("创建 remote dumper 失败: %v\n", err)
		} else {
			defer dumper.Close()
		}
	}

	decoder, err := opus.NewDecoder(48000, 1)
	if err != nil {
		log.Printf("创建 Opus 解码器失败: %v\n", err)
		return
	}

	resample, err := audio.NewResample(48000, 16000, astiav.ChannelLayoutMono, astiav.ChannelLayoutMono)
	if err != nil {
		log.Printf("创建 resample 失败: %v\n", err)
		return
	}

	pcm := make([]int16, frameSize)

	var logSample int = 0
	for {
		rtp, _, err := peer.remoteAudio.ReadRTP()
		if err != nil {
			log.Printf("read rtp error: %v\n", err)
			break
		}

		if logSample%1000 == 0 {
			log.Printf("read rtp logSample: %d\n", logSample)
			log.Printf("rtp payload len: %+v\n", len(rtp.Payload))
		}

		logSample++

		n, err := decoder.Decode(rtp.Payload, pcm)
		if err != nil {
			log.Printf("解码音频失败: %v\n", err)
			continue
		}

		samples := utils.Int16SliceToByteSlice(pcm[:n])

		if dumper != nil {
			dumper.Write(samples)
		}

		resamplePcm, err := resample.Resample(samples)
		if err != nil {
			log.Printf("resample error: %v\n", err)
			continue
		}

		err = peer.genaiSession.Send(&genai.LiveClientMessage{
			RealtimeInput: &genai.LiveClientRealtimeInput{
				MediaChunks: []*genai.Blob{
					{Data: resamplePcm, MIMEType: "audio/pcm"},
				},
			},
		})

		if err != nil {
			log.Printf("send message error: %v\n", err)
			continue
		}
	}
}

func (s *WebRTCServer) HandleLocalAudio(ctx context.Context, peer *PeerConnection) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var dumper *audio.Dumper
	if os.Getenv("DUMP_LOCAL_AUDIO") == "true" {
		var err error
		dumper, err = audio.NewDumper("localaudio", 48000, 1)
		if err != nil {
			log.Printf("创建 local dumper 失败: %v\n", err)
		} else {
			defer dumper.Close()
		}
	}

	// 创建 Opus 编码器 (48kHz, mono)
	encoder, err := opus.NewEncoder(48000, 1, opus.AppVoIP)
	if err != nil {
		log.Printf("Failed to create Opus encoder: %v", err)
		return
	}

	var lastSendTime time.Time
	// 创建编码缓冲区
	pcm := make([]int16, frameSize)
	opusData := make([]byte, maxDataBytes)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 检查距离上次发送是否已经超过20ms
			if time.Since(lastSendTime) >= 20*time.Millisecond {
				// 从buffer中读取一帧音频数据
				audioData := peer.audioBuffer.ReadFrame()

				if dumper != nil {
					dumper.Write(audioData)
				}

				// 将字节数据转换为int16切片
				for i := 0; i < len(audioData); i += 2 {
					pcm[i/2] = int16(audioData[i]) | int16(audioData[i+1])<<8
				}

				// Opus编码
				n, err := encoder.Encode(pcm[:frameSize], opusData)
				if err != nil {
					log.Printf("Failed to encode audio: %v", err)
					continue
				}

				// 创建音频样本
				sample := media.Sample{
					Data:     opusData[:n],
					Duration: 20 * time.Millisecond,
				}

				// 写入音频轨道
				if err := peer.localAudio.WriteSample(sample); err != nil {
					log.Printf("Failed to write audio sample: %v", err)
					continue
				}

				// 更新发送时间
				lastSendTime = time.Now()
			}
		}
	}
}

// 	数据格式
// 	data = { 'clientContent': { 'turnComplete': true, 'turns': [{ 'parts': [{ 'text': msg }] }] } };
// 	data = { 'realtimeInput': { 'mediaChunks': [{ 'data': msg, 'mimeType': 'image/jpeg' }] } };
// 	data = { 'realtimeInput': { 'mediaChunks': [{ 'data': msg, 'mimeType': 'audio/pcm' }] } };

// HandleDataChannel 处理数据通道
func (s *WebRTCServer) HandleDataChannel(ctx context.Context, peer *PeerConnection) {

	defer peer.dataChannel.Close()

	peer.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Received message: %s", string(msg.Data))

		message := msg.Data

		var sendMessage genai.LiveClientMessage
		if err := json.Unmarshal(message, &sendMessage); err != nil {
			log.Println("unmarshal message error ", string(message), err)
			return
		}
		peer.genaiSession.Send(&sendMessage)
	})

	<-ctx.Done()

}

// StartServer starts the WebRTC server on the specified port
func StartServer(addr string) error {
	server := NewWebRTCServer()
	http.HandleFunc("/session", server.HandleNegotiate)
	log.Printf("WebRTC server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}
