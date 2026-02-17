// Package realtimeapi provides the Realtime API protocol implementation.
//
// ResponseManager 管理 AI 响应的完整生命周期，确保符合 OpenAI Realtime API 规范。
//
// 响应生命周期：
//   response.create (client) → response.created → response.output_item.added
//     → conversation.item.created → response.content_part.added
//     → [response.audio.delta | response.text.delta]* → response.audio.done
//     → response.content_part.done → response.output_item.done → response.done
//
// 使用示例：
//   rm := NewResponseManager(session)
//   
//   // 创建响应
//   err := rm.CreateResponse(ResponseConfig{
//       Modalities: []Modality{ModalityText, ModalityAudio},
//   })
//   
//   // 流式发送音频增量
//   rm.SendAudioDelta(audioData)
//   
//   // 完成响应
//   rm.CompleteResponse()
package realtimeapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// ResponseState 表示响应的当前状态
type ResponseState int

const (
	ResponseStateIdle ResponseState = iota
	ResponseStateCreating
	ResponseStateInProgress
	ResponseStateCompleting
	ResponseStateCompleted
	ResponseStateInterrupted
	ResponseStateFailed
)

func (s ResponseState) String() string {
	switch s {
	case ResponseStateIdle:
		return "idle"
	case ResponseStateCreating:
		return "creating"
	case ResponseStateInProgress:
		return "in_progress"
	case ResponseStateCompleting:
		return "completing"
	case ResponseStateCompleted:
		return "completed"
	case ResponseStateInterrupted:
		return "interrupted"
	case ResponseStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ResponseManager 管理单个响应的完整生命周期
type ResponseManager struct {
	session *Session
	
	// 响应状态
	mu            sync.RWMutex
	state         ResponseState
	response      *events.Response
	responseID    string
	config        events.ResponseConfig
	
	// 输出项跟踪
	outputItems   []events.ConversationItem
	currentItem   *events.ConversationItem
	outputIndex   int
	contentIndex  int
	
	// 内容累积
	audioBuffer   []byte
	textBuffer    string
	transcriptBuffer string
	
	// 控制
	ctx           context.Context
	cancel        context.CancelFunc
	doneCh        chan struct{}
}

// NewResponseManager 创建新的响应管理器
func NewResponseManager(session *Session) *ResponseManager {
	return &ResponseManager{
		session:     session,
		state:       ResponseStateIdle,
		outputItems: make([]events.ConversationItem, 0),
		doneCh:      make(chan struct{}),
	}
}

// CreateResponse 创建新的响应
// 发送事件序列：response.created → response.output_item.added → conversation.item.created
func (rm *ResponseManager) CreateResponse(config events.ResponseConfig) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// 检查当前状态
	if rm.state != ResponseStateIdle && rm.state != ResponseStateCompleted && rm.state != ResponseStateInterrupted {
		return ErrResponseAlreadyInProgress
	}
	
	// 重置状态
	rm.reset()
	rm.state = ResponseStateCreating
	rm.config = config
	rm.responseID = generateResponseID()
	rm.ctx, rm.cancel = context.WithCancel(rm.session.Context())
	
	// 创建 Response 对象
	rm.response = &events.Response{
		ID:     rm.responseID,
		Object: "realtime.response",
		Status: events.ResponseStatusInProgress,
		Output: []events.ConversationItem{},
	}
	
	// 1. 发送 response.created
	if err := rm.session.SendEvent(events.NewResponseCreatedEvent(*rm.response)); err != nil {
		rm.state = ResponseStateFailed
		return err
	}
	
	// 2. 创建输出项（助手消息）
	itemID := generateItemID()
	item := events.ConversationItem{
		ID:      itemID,
		Object:  "realtime.item",
		Type:    events.ItemTypeMessage,
		Status:  events.ItemStatusInProgress,
		Role:    events.RoleAssistant,
		Content: []events.Content{},
	}
	
	rm.currentItem = &item
	rm.outputItems = append(rm.outputItems, item)
	
	// 3. 发送 response.output_item.added
	if err := rm.session.SendEvent(events.NewResponseOutputItemAddedEvent(
		rm.responseID,
		rm.outputIndex,
		item,
	)); err != nil {
		return err
	}
	
	// 4. 发送 conversation.item.created
	previousItemID := rm.session.Conversation.GetLastItemID()
	if err := rm.session.SendEvent(events.NewConversationItemCreatedEvent(item, previousItemID)); err != nil {
		return err
	}
	
	// 5. 添加到会话的对话历史
	rm.session.Conversation.AddItem(item)
	
	// 6. 发送 response.content_part.added
	contentType := rm.determineContentType()
	part := events.Content{
		Type: contentType,
	}
	if err := rm.session.SendEvent(events.NewResponseContentPartAddedEvent(
		rm.responseID,
		itemID,
		rm.outputIndex,
		rm.contentIndex,
		part,
	)); err != nil {
		return err
	}
	
	rm.state = ResponseStateInProgress
	return nil
}

// SendAudioDelta 发送音频增量
// 发送事件：response.audio.delta
func (rm *ResponseManager) SendAudioDelta(audioData []byte) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	if rm.state != ResponseStateInProgress {
		return ErrResponseNotInProgress
	}
	
	if rm.currentItem == nil {
		return ErrNoCurrentItem
	}
	
	// 累积音频数据
	rm.audioBuffer = append(rm.audioBuffer, audioData...)
	
	// 发送 response.audio.delta
	delta := base64.StdEncoding.EncodeToString(audioData)
	return rm.session.SendEvent(events.NewResponseAudioDeltaEvent(
		rm.responseID,
		rm.currentItem.ID,
		rm.outputIndex,
		rm.contentIndex,
		delta,
	))
}

// SendTextDelta 发送文本增量
// 发送事件：response.text.delta
func (rm *ResponseManager) SendTextDelta(text string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	if rm.state != ResponseStateInProgress {
		return ErrResponseNotInProgress
	}
	
	if rm.currentItem == nil {
		return ErrNoCurrentItem
	}
	
	// 累积文本
	rm.textBuffer += text
	
	// 发送 response.text.delta
	return rm.session.SendEvent(events.NewResponseTextDeltaEvent(
		rm.responseID,
		rm.currentItem.ID,
		rm.outputIndex,
		rm.contentIndex,
		text,
	))
}

// SendAudioTranscriptDelta 发送音频转录增量
// 发送事件：response.audio_transcript.delta
func (rm *ResponseManager) SendAudioTranscriptDelta(transcript string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	if rm.state != ResponseStateInProgress {
		return ErrResponseNotInProgress
	}
	
	if rm.currentItem == nil {
		return ErrNoCurrentItem
	}
	
	// 累积转录
	rm.transcriptBuffer += transcript
	
	// 发送 response.audio_transcript.delta
	return rm.session.SendEvent(events.NewResponseAudioTranscriptDeltaEvent(
		rm.responseID,
		rm.currentItem.ID,
		rm.outputIndex,
		rm.contentIndex,
		transcript,
	))
}

// CompleteContentPart 完成当前内容部分
// 发送事件：response.audio.done / response.text.done → response.content_part.done
func (rm *ResponseManager) CompleteContentPart() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if rm.state != ResponseStateInProgress {
		return ErrResponseNotInProgress
	}
	
	if rm.currentItem == nil {
		return ErrNoCurrentItem
	}
	
	contentType := rm.determineContentType()
	
	// 发送相应的 done 事件
	switch contentType {
	case events.ContentTypeAudio:
		// 发送 response.audio.done
		if err := rm.session.SendEvent(events.NewResponseAudioDoneEvent(
			rm.responseID,
			rm.currentItem.ID,
			rm.outputIndex,
			rm.contentIndex,
		)); err != nil {
			return err
		}
		
		// 如果有转录，发送转录完成
		if rm.transcriptBuffer != "" {
			if err := rm.session.SendEvent(events.NewResponseAudioTranscriptDoneEvent(
				rm.responseID,
				rm.currentItem.ID,
				rm.outputIndex,
				rm.contentIndex,
				rm.transcriptBuffer,
			)); err != nil {
				return err
			}
		}
		
	case events.ContentTypeText:
		// 发送 response.text.done
		if err := rm.session.SendEvent(events.NewResponseTextDoneEvent(
			rm.responseID,
			rm.currentItem.ID,
			rm.outputIndex,
			rm.contentIndex,
			rm.textBuffer,
		)); err != nil {
			return err
		}
	}
	
	// 构建完整的内容部分
	part := rm.buildContentPart()
	
	// 发送 response.content_part.done
	if err := rm.session.SendEvent(events.NewResponseContentPartDoneEvent(
		rm.responseID,
		rm.currentItem.ID,
		rm.outputIndex,
		rm.contentIndex,
		part,
	)); err != nil {
		return err
	}
	
	// 更新当前项的内容
	rm.currentItem.Content = append(rm.currentItem.Content, part)
	
	return nil
}

// CompleteOutputItem 完成当前输出项
// 发送事件：response.output_item.done
func (rm *ResponseManager) CompleteOutputItem() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if rm.state != ResponseStateInProgress {
		return ErrResponseNotInProgress
	}
	
	if rm.currentItem == nil {
		return ErrNoCurrentItem
	}
	
	// 更新项状态
	rm.currentItem.Status = events.ItemStatusCompleted
	
	// 发送 response.output_item.done
	if err := rm.session.SendEvent(events.NewResponseOutputItemDoneEvent(
		rm.responseID,
		rm.outputIndex,
		*rm.currentItem,
	)); err != nil {
		return err
	}
	
	// 更新响应的输出列表
	rm.response.Output = rm.outputItems
	
	return nil
}

// CompleteResponse 完成响应
// 发送事件：response.done
func (rm *ResponseManager) CompleteResponse() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if rm.state != ResponseStateInProgress {
		return ErrResponseNotInProgress
	}
	
	rm.state = ResponseStateCompleting
	
	// 更新响应状态
	rm.response.Status = events.ResponseStatusCompleted
	rm.response.Output = rm.outputItems
	
	// 发送 response.done
	if err := rm.session.SendEvent(events.NewResponseDoneEvent(*rm.response)); err != nil {
		rm.state = ResponseStateFailed
		return err
	}
	
	rm.state = ResponseStateCompleted
	close(rm.doneCh)
	
	return nil
}

// Interrupt 中断当前响应
// 发送事件：response.interrupted (自定义扩展)
func (rm *ResponseManager) Interrupt(reason string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if rm.state != ResponseStateInProgress {
		return nil // 没有正在进行的响应
	}
	
	rm.state = ResponseStateInterrupted
	
	// 如果有正在进行的项，标记为未完成
	if rm.currentItem != nil {
		rm.currentItem.Status = events.ItemStatusIncomplete
		
		// 发送中断事件
		audioMs := len(rm.audioBuffer) / 48 // 假设 24kHz, 16bit, mono
		rm.session.SendEvent(events.NewResponseInterruptedEvent(
			rm.responseID,
			rm.currentItem.ID,
			audioMs,
			reason,
		))
	}
	
	// 更新响应状态
	rm.response.Status = events.ResponseStatusCancelled
	rm.response.Output = rm.outputItems
	
	// 发送 response.done (状态为 cancelled)
	rm.session.SendEvent(events.NewResponseDoneEvent(*rm.response))
	
	close(rm.doneCh)
	return nil
}

// GetState 返回当前响应状态
func (rm *ResponseManager) GetState() ResponseState {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.state
}

// GetResponseID 返回当前响应 ID
func (rm *ResponseManager) GetResponseID() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.responseID
}

// Wait 等待响应完成
func (rm *ResponseManager) Wait() <-chan struct{} {
	return rm.doneCh
}

// reset 重置管理器状态
func (rm *ResponseManager) reset() {
	if rm.cancel != nil {
		rm.cancel()
	}
	
	rm.state = ResponseStateIdle
	rm.response = nil
	rm.responseID = ""
	rm.outputItems = make([]events.ConversationItem, 0)
	rm.currentItem = nil
	rm.outputIndex = 0
	rm.contentIndex = 0
	rm.audioBuffer = nil
	rm.textBuffer = ""
	rm.transcriptBuffer = ""
	rm.doneCh = make(chan struct{})
}

// determineContentType 根据配置确定内容类型
func (rm *ResponseManager) determineContentType() events.ContentType {
	modalities := rm.config.Modalities
	if len(modalities) == 0 {
		modalities = rm.session.Config.Modalities
	}
	
	// 检查是否支持音频
	hasAudio := false
	hasText := false
	for _, m := range modalities {
		switch m {
		case events.ModalityAudio:
			hasAudio = true
		case events.ModalityText:
			hasText = true
		}
	}
	
	// 优先返回音频类型（如果支持）
	if hasAudio && len(rm.audioBuffer) > 0 {
		return events.ContentTypeAudio
	}
	if hasText && rm.textBuffer != "" {
		return events.ContentTypeText
	}
	
	// 默认根据配置
	if hasAudio {
		return events.ContentTypeAudio
	}
	return events.ContentTypeText
}

// buildContentPart 构建完整的内容部分
func (rm *ResponseManager) buildContentPart() events.Content {
	contentType := rm.determineContentType()
	
	switch contentType {
	case events.ContentTypeAudio:
		return events.Content{
			Type:       events.ContentTypeAudio,
			Audio:      base64.StdEncoding.EncodeToString(rm.audioBuffer),
			Transcript: rm.transcriptBuffer,
		}
	case events.ContentTypeText:
		return events.Content{
			Type: events.ContentTypeText,
			Text: rm.textBuffer,
		}
	default:
		return events.Content{Type: contentType}
	}
}

// 错误定义
var (
	ErrResponseAlreadyInProgress = fmt.Errorf("response already in progress")
	ErrResponseNotInProgress     = fmt.Errorf("no response in progress")
	ErrNoCurrentItem             = fmt.Errorf("no current output item")
)

// 辅助函数
func generateResponseID() string {
	return "resp_" + uuid.New().String()[:8]
}

func generateItemID() string {
	return "item_" + uuid.New().String()[:8]
}

// PipelineResponseAdapter 将 Pipeline 输出适配到 ResponseManager
type PipelineResponseAdapter struct {
	manager *ResponseManager
	
	// 音频累积
	audioBuffer []byte
	bufferSize  int // 触发发送的缓冲区大小
}

// NewPipelineResponseAdapter 创建 Pipeline 响应适配器
func NewPipelineResponseAdapter(manager *ResponseManager, bufferSize int) *PipelineResponseAdapter {
	if bufferSize <= 0 {
		bufferSize = 4800 // 默认 100ms @ 24kHz, 16bit, mono
	}
	return &PipelineResponseAdapter{
		manager:    manager,
		bufferSize: bufferSize,
	}
}

// OnAudioData 处理 Pipeline 音频输出
func (a *PipelineResponseAdapter) OnAudioData(data []byte, sampleRate, channels int) {
	// 累积音频
	a.audioBuffer = append(a.audioBuffer, data...)
	
	// 当缓冲区达到阈值时发送
	for len(a.audioBuffer) >= a.bufferSize {
		chunk := a.audioBuffer[:a.bufferSize]
		a.audioBuffer = a.audioBuffer[a.bufferSize:]
		
		if err := a.manager.SendAudioDelta(chunk); err != nil {
			// 记录错误但不中断
			log.Printf("[ResponseAdapter] failed to send audio delta: %v", err)
		}
	}
}

// Flush 发送剩余音频
func (a *PipelineResponseAdapter) Flush() error {
	if len(a.audioBuffer) > 0 {
		err := a.manager.SendAudioDelta(a.audioBuffer)
		a.audioBuffer = nil
		return err
	}
	return nil
}

// PipelineResponseHandler 处理 Pipeline 输出并驱动 ResponseManager
type PipelineResponseHandler struct {
	manager     *ResponseManager
	adapter     *PipelineResponseAdapter
	session     *Session
	
	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPipelineResponseHandler 创建 Pipeline 响应处理器
func NewPipelineResponseHandler(session *Session, manager *ResponseManager) *PipelineResponseHandler {
	ctx, cancel := context.WithCancel(session.Context())
	return &PipelineResponseHandler{
		session: session,
		manager: manager,
		adapter: NewPipelineResponseAdapter(manager, 4800),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 开始监听 Pipeline 输出
func (h *PipelineResponseHandler) Start(pipeline *pipeline.Pipeline) {
	h.wg.Add(1)
	go h.handlePipelineOutput(pipeline)
}

// Stop 停止处理
func (h *PipelineResponseHandler) Stop() {
	h.cancel()
	h.wg.Wait()
}

// handlePipelineOutput 处理 Pipeline 输出
func (h *PipelineResponseHandler) handlePipelineOutput(p *pipeline.Pipeline) {
	defer h.wg.Done()
	
	for {
		select {
		case <-h.ctx.Done():
			// 刷新剩余音频
			h.adapter.Flush()
			return
			
		case msg := <-p.Pull():
			if msg == nil {
				// Pipeline 关闭，完成响应
				h.adapter.Flush()
				h.manager.CompleteContentPart()
				h.manager.CompleteOutputItem()
				h.manager.CompleteResponse()
				return
			}
			
			switch msg.Type {
			case pipeline.MsgTypeAudio:
				if msg.AudioData != nil && len(msg.AudioData.Data) > 0 {
					h.adapter.OnAudioData(
						msg.AudioData.Data,
						msg.AudioData.SampleRate,
						msg.AudioData.Channels,
					)
				}
				
			case pipeline.MsgTypeData:
				// 处理文本数据（如果有）
				if msg.TextData != nil && len(msg.TextData.Data) > 0 {
					h.manager.SendTextDelta(string(msg.TextData.Data))
				}
			}
		}
	}
}

// HandleResponseCreate 处理 response.create 事件
// 这是 Session.handleResponseCreate 的替代实现
func (s *Session) HandleResponseCreate(config events.ResponseConfig) error {
	// 创建 ResponseManager
	if s.responseManager == nil {
		s.responseManager = NewResponseManager(s)
	}
	
	// 创建响应
	if err := s.responseManager.CreateResponse(config); err != nil {
		return err
	}
	
	// 创建 Pipeline 响应处理器
	handler := NewPipelineResponseHandler(s, s.responseManager)
	
	// 如果 Pipeline 已存在，开始监听
	if p := s.GetPipeline(); p != nil {
		handler.Start(p)
	}
	
	return nil
}
