package pipeline

import (
	"log"
	"sync"
)

type ClearableChan struct {
	mu sync.Mutex
	ch chan *PipelineMessage
}

// NewClearableChan 创建一个带缓冲区大小为 size 的 ClearableChan。
func NewClearableChan(size int) *ClearableChan {
	return &ClearableChan{
		ch: make(chan *PipelineMessage, size),
	}
}

// Send 向通道发送元素。
func (cc *ClearableChan) Send(val *PipelineMessage) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	select {
	case cc.ch <- val:
	default:
		log.Printf("channel is full, val: %+v \n", val)
	}
}

// Recv 从通道接收元素（阻塞读取）。
func (cc *ClearableChan) Recv() *PipelineMessage {
	// 这里不一定需要锁，因为仅仅读通道本身是线程安全的
	// 但如果你对 cc 的其他操作并发频繁，也可考虑加锁确保一致性
	return <-cc.ch
}

func (cc *ClearableChan) Chan() <-chan *PipelineMessage {
	return cc.ch
}

// Clear 清空通道中现有的所有元素。
func (cc *ClearableChan) Clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	// 不断从通道中读，直到读不到为止
	for {
		select {
		case <-cc.ch:
			// 将读取到的数据丢弃
		default:
			// 通道里没有更多可读的数据
			return
		}
	}
}
