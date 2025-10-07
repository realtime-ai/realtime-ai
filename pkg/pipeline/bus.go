package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EventType 用于区分不同的事件类型
type EventType string

const (
	EventError         EventType = "Error"
	EventWarning       EventType = "Warning"
	EventPartialResult EventType = "PartialResult"
	EventFinalResult   EventType = "FinalResult"
	EventBargeIn       EventType = "BargeIn"
	EventInterrupted   EventType = "Interrupted"
	EventStarted       EventType = "Started"
	EventStopped       EventType = "Stopped"
	// 可继续扩展更多事件类型...
)

// Event 代表一条通用事件
type Event struct {
	Type      EventType
	Timestamp time.Time
	Payload   interface{} // 任意附加数据
}

// Bus 定义了事件总线的接口
type Bus interface {
	// Subscribe 订阅某一类型的事件，事件将被投递到 ch 通道
	Subscribe(eventType EventType, ch chan<- Event)

	// Unsubscribe 取消订阅
	Unsubscribe(eventType EventType, ch chan<- Event)

	// Publish 发布一条事件到总线，返回是否成功投递（false 表示被丢弃）
	Publish(evt Event) bool

	// Start 启动总线内部处理（如果需要异步工作）
	Start(ctx context.Context) error

	// Stop 停止总线内部处理
	Stop()
}

type EventBus struct {
	// key: EventType, value: 订阅该事件类型的通道列表
	subscribers map[EventType][]chan<- Event

	// 保护 subscribers 的互斥锁
	lock sync.RWMutex

	// 是否需要支持异步缓冲队列或后台处理，可以加一个 channel
	eventChan chan Event

	running bool
	cancel  context.CancelFunc // 新增：存储 context 取消函数
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]chan<- Event),
		eventChan:   make(chan Event, 100), // 缓冲大小可根据需要调整
	}
}

// Subscribe 订阅某种事件类型
func (b *EventBus) Subscribe(eventType EventType, ch chan<- Event) {
	b.lock.Lock()
	defer b.lock.Unlock()

	// 在该类型的订阅者列表中加入新通道
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
}

// Unsubscribe 取消订阅
func (b *EventBus) Unsubscribe(eventType EventType, ch chan<- Event) {
	b.lock.Lock()
	defer b.lock.Unlock()

	chans := b.subscribers[eventType]
	for i, c := range chans {
		if c == ch {
			// 移除
			chans = append(chans[:i], chans[i+1:]...)
			break
		}
	}
	b.subscribers[eventType] = chans
}

// Publish 直接发布事件。如果需要异步处理，可以向 b.eventChan 写入
// 返回 true 表示事件成功投递，false 表示至少一个订阅者丢弃了事件
func (b *EventBus) Publish(evt Event) bool {
	allDelivered := true
	if b.running {
		// 若有后台协程在处理，就写入 eventChan
		select {
		case b.eventChan <- evt:
			// 成功写入事件通道
		default:
			fmt.Println("[EventBus] Warning: dropping event due to full event channel.")
			allDelivered = false
		}
	} else {
		// 若不使用后台协程，则直接分发
		b.lock.RLock()
		defer b.lock.RUnlock()
		subs := b.subscribers[evt.Type]
		for _, ch := range subs {
			// 不要阻塞，若通道满则丢弃或另行处理
			select {
			case ch <- evt:
			default:
				// 可做警告处理：订阅者通道堵塞导致事件丢失
				fmt.Println("[EventBus] Warning: dropping event due to full channel.")
				allDelivered = false
			}
		}
	}
	return allDelivered
}

// Start 启动后台协程，异步分发事件
func (b *EventBus) Start(ctx context.Context) error {
	if b.running {
		return nil
	}

	// 创建一个新的 context 和取消函数
	ctx, cancel := context.WithCancel(ctx) // ignore error
	b.cancel = cancel
	b.running = true

	go func() {
		for {
			select {
			case evt := <-b.eventChan:
				// 分发事件给订阅者
				b.lock.RLock()
				subs := b.subscribers[evt.Type]
				b.lock.RUnlock()

				for _, ch := range subs {
					// 发送给订阅者
					select {
					case ch <- evt:
						// sent ok
					default:
						fmt.Println("[EventBus] Warning: dropping event due to full channel.")
					}
				}

			case <-ctx.Done():
				// 退出
				fmt.Println("[EventBus] Stopping...")
				return
			}
		}
	}()

	return nil
}

// Stop 停止后台协程
func (b *EventBus) Stop() {
	if !b.running {
		return
	}
	if b.cancel != nil {
		b.cancel() // 调用 cancel 来停止事件处理
		b.cancel = nil
	}
	b.running = false
}
