package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// https://chatgpt.com/c/678d0634-058c-8002-909d-d298453449e9

type AudioData struct {
	Data       []byte
	SampleRate int
	Channels   int
	MediaType  string // "audio/x-raw", "audio/x-opus", etc.
	Codec      string
	Timestamp  time.Time
}

type VideoData struct {
	Data           []byte
	Width          int
	Height         int
	MediaType      string
	Format         string
	FramerateNum   int
	FramerateDenom int
	Codec          string
	Timestamp      time.Time
}

type TextData struct {
	Data      []byte
	TextType  string
	Timestamp time.Time
}

type PipelineMessageType int

const (
	MsgTypeAudio PipelineMessageType = iota
	MsgTypeVideo
	MsgTypeData
	MsgTypeCommand
)

type PipelineMessage struct {
	Type PipelineMessageType

	// SessionID 会话 ID
	SessionID string
	// Timestamp 时间戳
	Timestamp time.Time

	// AudioData 音频数据块
	AudioData *AudioData

	// VideoData 视频数据块
	VideoData *VideoData

	// TextData 文本数据块
	TextData *TextData

	// Metadata 元数据
	Metadata interface{}
}

func (p *PipelineMessage) String() string {
	return fmt.Sprintf("PipelineMessage{Type: %d, SessionID: %s, Timestamp: %s}", p.Type, p.SessionID, p.Timestamp)
}

type Pipeline struct {
	sync.Mutex
	name     string
	bus      Bus
	elements []Element
}

func NewPipeline(name string) *Pipeline {
	bus := NewEventBus()
	return &Pipeline{
		name:     name,
		bus:      bus,
		elements: []Element{},
	}
}

func (p *Pipeline) AddElement(element Element) {
	p.Lock()
	defer p.Unlock()
	element.SetBus(p.bus)
	p.elements = append(p.elements, element)
}

func (p *Pipeline) AddElements(elements []Element) {
	p.Lock()
	defer p.Unlock()
	for _, element := range elements {
		element.SetBus(p.bus)
	}
	p.elements = append(p.elements, elements...)
}

// Link 连接两个 Element，返回一个取消函数用于断开连接
// 返回的函数调用后会停止数据传输并关闭目标 Element 的输入通道
func (p *Pipeline) Link(a, b Element) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				// 取消连接，退出
				return
			case msg, ok := <-a.Out():
				if !ok {
					// 源通道已关闭
					close(b.In())
					return
				}
				select {
				case <-ctx.Done():
					return
				case b.In() <- msg:
				}
			}
		}
	}()

	// 返回取消函数
	return func() {
		cancel()
		<-done // 等待 goroutine 退出
	}
}

func (p *Pipeline) Bus() Bus {
	return p.bus
}

func (p *Pipeline) Push(msg *PipelineMessage) {
	if len(p.elements) == 0 {
		return
	}
	select {
	case p.elements[0].In() <- msg:
	default:
		fmt.Println("pipeline input channel is full")
	}
}

// Pull 从 pipeline 的最后一个元素获取消息
func (p *Pipeline) Pull() *PipelineMessage {
	if len(p.elements) == 0 {
		return nil
	}
	return <-p.elements[len(p.elements)-1].Out()
}

func (p *Pipeline) Start(ctx context.Context) error {
	for _, e := range p.elements {
		if err := e.Start(ctx); err != nil {
			return err
		}
	}
	p.bus.Start(ctx)
	return nil
}

func (p *Pipeline) Stop() error {
	p.Lock()
	defer p.Unlock()
	// 倒序停止更稳妥，也可以正序
	for i := len(p.elements) - 1; i >= 0; i-- {
		if err := p.elements[i].Stop(); err != nil {
			return err
		}
	}
	// 停止事件总线
	p.bus.Stop()
	return nil
}
