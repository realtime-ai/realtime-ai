package pipeline

import (
	"context"
	"testing"
	"time"
)

// MockElement 用于测试的简单 Element 实现
type MockElement struct {
	*BaseElement
}

func NewMockElement() *MockElement {
	return &MockElement{
		BaseElement: NewBaseElement("mock-element", 10),
	}
}

func (e *MockElement) Start(ctx context.Context) error {
	return nil
}

func (e *MockElement) Stop() error {
	return nil
}

func TestPipelineLinkUnlink(t *testing.T) {
	p := NewPipeline("test")

	elem1 := NewMockElement()
	elem2 := NewMockElement()

	p.AddElement(elem1)
	p.AddElement(elem2)

	// 测试 Link 返回 unlink 函数
	unlink := p.Link(elem1, elem2)
	if unlink == nil {
		t.Fatal("Link should return an unlink function")
	}

	// 发送消息
	msg := &PipelineMessage{
		Type:      MsgTypeAudio,
		SessionID: "test-session",
		Timestamp: time.Now(),
	}

	// 在后台发送消息
	go func() {
		elem1.OutChan <- msg
	}()

	// 应该能接收到消息
	select {
	case received := <-elem2.InChan:
		if received.SessionID != "test-session" {
			t.Errorf("Expected session ID 'test-session', got '%s'", received.SessionID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for message")
	}

	// 测试 unlink
	unlink()

	// unlink 后发送的消息不应该被接收（因为 goroutine 已退出）
	// 注意：这个测试只是验证 unlink 不会 panic，实际行为取决于实现
	time.Sleep(50 * time.Millisecond)
}

func TestPipelineStartStop(t *testing.T) {
	p := NewPipeline("test")

	elem := NewMockElement()
	p.AddElement(elem)

	ctx := context.Background()

	// 测试启动
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// 测试停止
	if err := p.Stop(); err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}
}

func TestPipelinePushPull(t *testing.T) {
	p := NewPipeline("test")

	elem := NewMockElement()
	p.AddElement(elem)

	// 测试 GetName
	if elem.GetName() != "mock-element" {
		t.Errorf("Expected element name 'mock-element', got '%s'", elem.GetName())
	}

	// 启动一个 goroutine 来处理消息
	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()

	// 模拟 element 处理：从 InChan 读取并写入 OutChan
	go func() {
		for msg := range elem.InChan {
			elem.OutChan <- msg
		}
	}()

	// Push 消息
	msg := &PipelineMessage{
		Type:      MsgTypeAudio,
		SessionID: "test-session",
		Timestamp: time.Now(),
	}
	p.Push(msg)

	// Pull 消息
	received := p.Pull()
	if received == nil {
		t.Fatal("Expected to receive message")
	}
	if received.SessionID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got '%s'", received.SessionID)
	}
}
