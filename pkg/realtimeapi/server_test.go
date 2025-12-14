package realtimeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// echoElement is a simple element that echoes messages back.
type echoElement struct {
	*pipeline.BaseElement
	cancel context.CancelFunc
}

func newEchoElement() *echoElement {
	return &echoElement{
		BaseElement: pipeline.NewBaseElement("echo", 100),
	}
}

func (e *echoElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.InChan:
				if msg == nil {
					continue
				}
				select {
				case e.OutChan <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return nil
}

func (e *echoElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	return nil
}

// echoPipelineFactory creates a simple echo pipeline for testing.
func echoPipelineFactory(ctx context.Context, session *Session) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("test-echo-" + session.ID)
	echo := newEchoElement()
	p.AddElement(echo)
	return p, nil
}

func TestServer_NewServer(t *testing.T) {
	config := DefaultServerConfig()
	server := NewServer(config)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.config.Addr != config.Addr {
		t.Errorf("expected addr %s, got %s", config.Addr, server.config.Addr)
	}
}

func TestServer_SetPipelineFactory(t *testing.T) {
	config := DefaultServerConfig()
	server := NewServer(config)

	server.SetPipelineFactory(echoPipelineFactory)

	if server.pipelineFactory == nil {
		t.Error("pipelineFactory should not be nil after SetPipelineFactory")
	}
}

func TestServer_WebSocketConnection(t *testing.T) {
	config := DefaultServerConfig()
	config.Addr = ":0" // Use random port
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	// Create test HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	// Connect via WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read session.created event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	var sessionCreated events.SessionCreatedEvent
	if err := json.Unmarshal(msg, &sessionCreated); err != nil {
		t.Fatalf("Failed to unmarshal session.created: %v", err)
	}

	if sessionCreated.Type != events.ServerEventTypeSessionCreated {
		t.Errorf("expected session.created event, got %s", sessionCreated.Type)
	}

	if sessionCreated.Session.ID == "" {
		t.Error("session ID should not be empty")
	}
}

func TestServer_SessionUpdate(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	// Create test HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read session.created event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadMessage() // Consume session.created

	// Send session.update event
	updateEvent := map[string]interface{}{
		"type": "session.update",
		"session": map[string]interface{}{
			"voice":        "nova",
			"instructions": "Be helpful",
		},
	}
	updateData, _ := json.Marshal(updateEvent)
	if err := conn.WriteMessage(websocket.TextMessage, updateData); err != nil {
		t.Fatalf("Failed to send session.update: %v", err)
	}

	// Read session.updated event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read session.updated: %v", err)
	}

	var sessionUpdated events.SessionUpdatedEvent
	if err := json.Unmarshal(msg, &sessionUpdated); err != nil {
		t.Fatalf("Failed to unmarshal session.updated: %v", err)
	}

	if sessionUpdated.Type != events.ServerEventTypeSessionUpdated {
		t.Errorf("expected session.updated event, got %s", sessionUpdated.Type)
	}

	if sessionUpdated.Session.Voice != "nova" {
		t.Errorf("expected voice 'nova', got '%s'", sessionUpdated.Session.Voice)
	}
}

func TestServer_InvalidEvent(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read session.created event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadMessage()

	// Send invalid event
	invalidEvent := map[string]interface{}{
		"type": "invalid.event.type",
	}
	invalidData, _ := json.Marshal(invalidEvent)
	if err := conn.WriteMessage(websocket.TextMessage, invalidData); err != nil {
		t.Fatalf("Failed to send invalid event: %v", err)
	}

	// Read error event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read error: %v", err)
	}

	var errorEvent events.ErrorEvent
	if err := json.Unmarshal(msg, &errorEvent); err != nil {
		t.Fatalf("Failed to unmarshal error: %v", err)
	}

	if errorEvent.Type != events.ServerEventTypeError {
		t.Errorf("expected error event, got %s", errorEvent.Type)
	}
}

func TestServer_Authentication(t *testing.T) {
	config := DefaultServerConfig()
	config.AuthToken = "secret-token"
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path

	// Test without auth token - should fail
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("expected connection to fail without auth token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// Test with wrong auth token - should fail
	header := http.Header{}
	header.Add("Authorization", "Bearer wrong-token")
	_, resp, err = websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Error("expected connection to fail with wrong token")
	}

	// Test with correct auth token - should succeed
	header = http.Header{}
	header.Add("Authorization", "Bearer secret-token")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?model=echo", header)
	if err != nil {
		t.Fatalf("expected connection to succeed with correct token: %v", err)
	}
	conn.Close()
}

func TestServer_ModelValidation(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"model-a", "model-b"}
	server := NewServer(config)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path

	// Test with disallowed model - should fail
	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"?model=invalid-model", nil)
	if err == nil {
		t.Error("expected connection to fail with invalid model")
	}
	if resp != nil && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	// Test with allowed model - should succeed
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?model=model-a", nil)
	if err != nil {
		t.Fatalf("expected connection to succeed with allowed model: %v", err)
	}
	conn.Close()
}

func TestServer_SessionCount(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	// Initially no sessions
	if server.SessionCount() != 0 {
		t.Errorf("expected 0 sessions, got %d", server.SessionCount())
	}

	// Connect first client
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}

	// Wait for session to be registered
	time.Sleep(100 * time.Millisecond)

	if server.SessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", server.SessionCount())
	}

	// Connect second client
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if server.SessionCount() != 2 {
		t.Errorf("expected 2 sessions, got %d", server.SessionCount())
	}

	// Close first connection
	conn1.Close()
	time.Sleep(200 * time.Millisecond)

	if server.SessionCount() != 1 {
		t.Errorf("expected 1 session after close, got %d", server.SessionCount())
	}

	// Close second connection
	conn2.Close()
	time.Sleep(200 * time.Millisecond)

	if server.SessionCount() != 0 {
		t.Errorf("expected 0 sessions after all closed, got %d", server.SessionCount())
	}
}

func TestServer_InputAudioBufferAppend(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read session.created
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadMessage()

	// Send audio buffer append event
	appendEvent := map[string]interface{}{
		"type":  "input_audio_buffer.append",
		"audio": "SGVsbG8gV29ybGQ=", // Base64 encoded "Hello World"
	}
	appendData, _ := json.Marshal(appendEvent)
	if err := conn.WriteMessage(websocket.TextMessage, appendData); err != nil {
		t.Fatalf("Failed to send input_audio_buffer.append: %v", err)
	}

	// No error means audio was accepted
	// (No direct response for append, but we can verify by committing)
}

func TestServer_InputAudioBufferCommit(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read session.created
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadMessage()

	// Send some audio first
	appendEvent := map[string]interface{}{
		"type":  "input_audio_buffer.append",
		"audio": "SGVsbG8gV29ybGQ=",
	}
	appendData, _ := json.Marshal(appendEvent)
	conn.WriteMessage(websocket.TextMessage, appendData)

	// Send commit event
	commitEvent := map[string]interface{}{
		"type": "input_audio_buffer.commit",
	}
	commitData, _ := json.Marshal(commitEvent)
	if err := conn.WriteMessage(websocket.TextMessage, commitData); err != nil {
		t.Fatalf("Failed to send input_audio_buffer.commit: %v", err)
	}

	// Read committed event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read committed event: %v", err)
	}

	var committed events.InputAudioBufferCommittedEvent
	if err := json.Unmarshal(msg, &committed); err != nil {
		t.Fatalf("Failed to unmarshal committed: %v", err)
	}

	if committed.Type != events.ServerEventTypeInputAudioBufferCommitted {
		t.Errorf("expected input_audio_buffer.committed event, got %s", committed.Type)
	}
}

func TestServer_InputAudioBufferClear(t *testing.T) {
	config := DefaultServerConfig()
	config.AllowedModels = []string{"echo"} // Allow echo model for testing
	server := NewServer(config)
	server.SetPipelineFactory(echoPipelineFactory)

	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, server.handleWebSocket)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + config.Path + "?model=echo"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	// Read session.created
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadMessage()

	// Send clear event
	clearEvent := map[string]interface{}{
		"type": "input_audio_buffer.clear",
	}
	clearData, _ := json.Marshal(clearEvent)
	if err := conn.WriteMessage(websocket.TextMessage, clearData); err != nil {
		t.Fatalf("Failed to send input_audio_buffer.clear: %v", err)
	}

	// Read cleared event
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read cleared event: %v", err)
	}

	var cleared events.InputAudioBufferClearedEvent
	if err := json.Unmarshal(msg, &cleared); err != nil {
		t.Fatalf("Failed to unmarshal cleared: %v", err)
	}

	if cleared.Type != events.ServerEventTypeInputAudioBufferCleared {
		t.Errorf("expected input_audio_buffer.cleared event, got %s", cleared.Type)
	}
}
