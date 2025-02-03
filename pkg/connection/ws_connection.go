package connection

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

var _ RTCConnection = (*wsConnectionImpl)(nil)

// WIP

// WSMessage WebSocket消息结构
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type wsConnectionImpl struct {
	peerID  string
	conn    *websocket.Conn
	handler ConnectionEventHandler

	// 消息通道
	inChan  chan *pipeline.PipelineMessage
	outChan chan *pipeline.PipelineMessage

	// 控制相关
	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.Mutex
}

func NewWSConnection(peerID string, wsConn *websocket.Conn) RTCConnection {
	ws := &wsConnectionImpl{
		peerID:  peerID,
		conn:    wsConn,
		handler: &NoOpConnectionEventHandler{},
		inChan:  make(chan *pipeline.PipelineMessage, 50),
		outChan: make(chan *pipeline.PipelineMessage, 50),
		closed:  make(chan struct{}),
	}

	// 启动读取协程
	go ws.readPump()
	// 启动写入协程
	go ws.writePump()

	return ws
}

func (w *wsConnectionImpl) readPump() {
	defer func() {
		w.Close()
	}()

	for {
		select {
		case <-w.closed:
			return
		default:
			_, message, err := w.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("websocket read error: %v", err)
					w.handler.OnError(err)
				}
				return
			}

			// 解析消息
			var wsMsg WSMessage
			if err := json.Unmarshal(message, &wsMsg); err != nil {
				log.Printf("failed to unmarshal message: %v", err)
				continue
			}

			// 根据消息类型处理
			switch wsMsg.Type {
			case "audio":
				var audioData []byte
				if err := json.Unmarshal(wsMsg.Payload, &audioData); err != nil {
					log.Printf("failed to unmarshal audio data: %v", err)
					continue
				}

				msg := &pipeline.PipelineMessage{
					Type: pipeline.MsgTypeAudio,
					AudioData: &pipeline.AudioData{
						Data:       audioData,
						SampleRate: 48000, // 默认采样率
						Channels:   1,     // 默认单声道
						MediaType:  "audio/x-raw",
						Timestamp:  time.Now(),
					},
				}
				w.handler.OnMessage(msg)

			case "text":
				var textData string
				if err := json.Unmarshal(wsMsg.Payload, &textData); err != nil {
					log.Printf("failed to unmarshal text data: %v", err)
					continue
				}

				msg := &pipeline.PipelineMessage{
					Type: pipeline.MsgTypeData,
					TextData: &pipeline.TextData{
						Data:      []byte(textData),
						TextType:  "text",
						Timestamp: time.Now(),
					},
				}
				w.handler.OnMessage(msg)
			}
		}
	}
}

func (w *wsConnectionImpl) writePump() {
	for {
		select {
		case <-w.closed:
			return
		case msg := <-w.outChan:
			w.mu.Lock()
			var wsMsg WSMessage

			switch msg.Type {
			case pipeline.MsgTypeAudio:
				wsMsg.Type = "audio"
				wsMsg.Payload, _ = json.Marshal(msg.AudioData.Data)
			case pipeline.MsgTypeData:
				wsMsg.Type = "text"
				wsMsg.Payload, _ = json.Marshal(string(msg.TextData.Data))
			}

			err := w.conn.WriteJSON(wsMsg)
			w.mu.Unlock()

			if err != nil {
				log.Printf("websocket write error: %v", err)
				w.handler.OnError(err)
				return
			}
		}
	}
}

// RTCConnection interface implementation
func (w *wsConnectionImpl) PeerID() string {
	return w.peerID
}

func (w *wsConnectionImpl) RegisterEventHandler(handler ConnectionEventHandler) {
	w.handler = handler
}

func (w *wsConnectionImpl) SendMessage(msg *pipeline.PipelineMessage) {
	select {
	case <-w.closed:
		return
	case w.outChan <- msg:
	default:
		log.Println("outChan is full, dropping message")
	}
}

func (w *wsConnectionImpl) Close() error {
	w.closeOnce.Do(func() {
		close(w.closed)
		w.conn.Close()
		close(w.inChan)
		close(w.outChan)
	})
	return nil
}

// RTCConnection interface compatibility stubs
func (w *wsConnectionImpl) SetAudioEncodeParam(sampleRate int, channels int, bitRate int) {}
func (w *wsConnectionImpl) SetAudioOutputParam(sampleRate int, channels int)              {}
