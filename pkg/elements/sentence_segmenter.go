// Package elements provides pipeline processing elements.
//
// SentenceSegmenter 实现流式文本分句，用于 LLM 输出后尽快分句送入 TTS。
//
// 主要功能:
//   - 流式分句：逐字符/token 接收，检测句子边界
//   - 多语言支持：中文、英文、日文等标点
//   - 特殊情况处理：缩写词、小数、URL、省略号等
//   - 长度控制：最小/最大句子长度限制
//   - 超时机制：避免长时间等待
//
// 设计原则:
//   - 宁可稍晚分句，不可错误分句（错误分句会导致语音不自然）
//   - 优先保证完整语义单元
//   - 支持流式处理，每次只处理增量数据
//
// 使用示例:
//
//	segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
//	    MinLength:  5,
//	    MaxLength:  200,
//	    FlushTimeout: 500 * time.Millisecond,
//	})
//	segmenter.OnSentence(func(sentence string, isFinal bool) {
//	    // 发送给 TTS
//	})
//	segmenter.Feed("Hello")
//	segmenter.Feed(" world.")  // 触发回调: "Hello world."
package elements

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// SentenceSegmenterConfig 分句器配置
type SentenceSegmenterConfig struct {
	// MinLength 最小句子长度（字符数），低于此长度不分句
	// 避免产生太短的语音片段，如 "OK." "Hi."
	// 默认值: 10
	MinLength int

	// MaxLength 最大句子长度（字符数），超过此长度强制分句
	// 避免 TTS 处理过长文本，影响延迟
	// 默认值: 200
	MaxLength int

	// FlushTimeout 超时时间，超过此时间未检测到句尾则强制分句
	// 用于处理 LLM 输出卡顿或无标点文本
	// 默认值: 800ms
	FlushTimeout time.Duration

	// Language 主要语言 ("zh", "en", "ja", "auto")
	// 影响分句策略和标点识别
	// 默认值: "auto"
	Language string

	// EnableSmartPunctuation 启用智能标点检测
	// 处理缩写词、小数等特殊情况
	// 默认值: true
	EnableSmartPunctuation bool
}

// SentenceCallback 句子回调函数
// sentence: 完整句子文本
// isFinal: 是否为最后一个句子（流结束时）
type SentenceCallback func(sentence string, isFinal bool)

// SentenceSegmenter 流式分句器
type SentenceSegmenter struct {
	config   SentenceSegmenterConfig
	buffer   strings.Builder
	callback SentenceCallback

	lastFeedTime time.Time
	timer        *time.Timer

	mu sync.Mutex
}

// 常用英文缩写词（句点后面不应分句）
var commonAbbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true,
	"sr": true, "jr": true, "vs": true, "etc": true, "inc": true,
	"ltd": true, "corp": true, "co": true, "no": true, "vol": true,
	"pg": true, "pp": true, "fig": true, "e.g": true, "i.e": true,
	"a.m": true, "p.m": true, "u.s": true, "u.k": true, "u.n": true,
	"st": true, "ave": true, "blvd": true, "rd": true, "dept": true,
	"est": true, "approx": true, "govt": true, "min": true, "max": true,
}

// 匹配小数、货币、百分比等数字模式
var numberPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\d+\.\d*$`),           // 12.34 或 12.
	regexp.MustCompile(`\$\d+\.\d*$`),         // $9.99
	regexp.MustCompile(`€\d+\.\d*$`),          // €9.99
	regexp.MustCompile(`£\d+\.\d*$`),          // £9.99
	regexp.MustCompile(`¥\d+\.\d*$`),          // ¥9.99
	regexp.MustCompile(`\d+\.\d*%$`),          // 3.14%
	regexp.MustCompile(`v\d+\.\d*$`),          // v1.0
	regexp.MustCompile(`\d+\.\d+\.\d*$`),      // 1.2.3 版本号
	regexp.MustCompile(`\d{1,3}\.\d{1,3}\.`),  // IP 地址开头
}

// URL/Email 模式
var urlEmailPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://\S*$`),       // URL
	regexp.MustCompile(`www\.\S*$`),           // www.
	regexp.MustCompile(`\S+@\S+\.\S*$`),       // email
	regexp.MustCompile(`\S+\.(com|org|net|io|ai|cn|jp)$`), // 域名
}

// 中文句尾标点
var chineseSentenceEnders = map[rune]bool{
	'。': true, '！': true, '？': true, '；': true,
	'…': true, // 省略号也可作为句尾
}

// 英文句尾标点
var englishSentenceEnders = map[rune]bool{
	'.': true, '!': true, '?': true, ';': true,
}

// 日文句尾标点
var japaneseSentenceEnders = map[rune]bool{
	'。': true, '！': true, '？': true,
	'．': true, // 全角句点
}

// 可选分句点（逗号等，用于超长句子强制分割）
var softBreakPunctuation = map[rune]bool{
	',': true, '，': true, // 逗号
	':': true, '：': true, // 冒号
	'、': true, // 顿号
}

// NewSentenceSegmenter 创建分句器
func NewSentenceSegmenter(config SentenceSegmenterConfig) *SentenceSegmenter {
	// 设置默认值
	if config.MinLength <= 0 {
		config.MinLength = 10
	}
	if config.MaxLength <= 0 {
		config.MaxLength = 200
	}
	if config.FlushTimeout <= 0 {
		config.FlushTimeout = 800 * time.Millisecond
	}
	if config.Language == "" {
		config.Language = "auto"
	}

	return &SentenceSegmenter{
		config:       config,
		lastFeedTime: time.Now(),
	}
}

// OnSentence 设置句子回调
func (s *SentenceSegmenter) OnSentence(callback SentenceCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callback = callback
}

// Feed 喂入文本（流式调用）
// 返回是否触发了分句
func (s *SentenceSegmenter) Feed(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if text == "" {
		return false
	}

	s.buffer.WriteString(text)
	s.lastFeedTime = time.Now()

	// 重置超时计时器
	s.resetTimer()

	// 尝试分句
	return s.tryFlush(false)
}

// Flush 强制刷新缓冲区（流结束时调用）
func (s *SentenceSegmenter) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopTimer()
	s.flushBuffer(true)
}

// Reset 重置分句器状态
func (s *SentenceSegmenter) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopTimer()
	s.buffer.Reset()
}

// GetBuffer 获取当前缓冲区内容（用于调试）
func (s *SentenceSegmenter) GetBuffer() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

// tryFlush 尝试分句
func (s *SentenceSegmenter) tryFlush(isTimeout bool) bool {
	content := s.buffer.String()
	if content == "" {
		return false
	}

	// 超时情况：直接输出缓冲区内容（如果非空）
	if isTimeout {
		sentence := strings.TrimSpace(content)
		if sentence != "" {
			s.buffer.Reset()
			if s.callback != nil {
				s.callback(sentence, false)
			}
			return true
		}
		return false
	}

	// 查找分句点
	breakPoint := s.findSentenceBreak(content, false)
	if breakPoint <= 0 {
		return false
	}

	// 提取句子
	sentence := strings.TrimSpace(content[:breakPoint])
	remaining := content[breakPoint:]

	// 检查最小长度
	if utf8.RuneCountInString(sentence) < s.config.MinLength {
		return false
	}

	// 刷新句子
	s.buffer.Reset()
	s.buffer.WriteString(remaining)

	if s.callback != nil && sentence != "" {
		s.callback(sentence, false)
	}

	// 递归检查剩余内容是否还有完整句子
	if remaining != "" {
		s.tryFlush(false)
	}

	return true
}

// findSentenceBreak 查找句子分割点
// 返回分割位置（字节索引），0 表示未找到
func (s *SentenceSegmenter) findSentenceBreak(text string, isTimeout bool) int {
	runes := []rune(text)
	runeCount := len(runes)

	if runeCount == 0 {
		return 0
	}

	// 1. 查找句尾标点（优先）
	for i := 0; i < runeCount && i < s.config.MaxLength; i++ {
		r := runes[i]
		bytePos := len(string(runes[:i+1]))

		// 检查是否为句尾标点
		if s.isSentenceEnder(r) {
			// 智能检测：排除特殊情况
			if s.config.EnableSmartPunctuation {
				textBefore := string(runes[:i+1])
				textAfter := ""
				if i+1 < runeCount {
					textAfter = string(runes[i+1:])
				}

				if s.isSpecialCase(textBefore, textAfter, r) {
					continue
				}
			}

			// 找到有效分句点
			return bytePos
		}
	}

	// 2. 检查是否超过最大长度，需要强制分句
	if runeCount >= s.config.MaxLength {
		return s.findForcedBreak(text, runes)
	}

	// 3. 超时情况下，在软分隔符处分句
	if isTimeout && runeCount >= s.config.MinLength {
		if pos := s.findSoftBreak(runes); pos > 0 {
			return pos
		}
		// 超时且没有软分隔符，返回全部内容
		return len(text)
	}

	return 0
}

// findForcedBreak 超长句子强制分句
func (s *SentenceSegmenter) findForcedBreak(text string, runes []rune) int {
	// 优先在软分隔符处分割
	if pos := s.findSoftBreak(runes); pos > 0 {
		return pos
	}

	// 在空格处分割
	if pos := s.findSpaceBreak(runes); pos > 0 {
		return pos
	}

	// 最后手段：直接在 MaxLength 处截断
	maxRunes := s.config.MaxLength
	if maxRunes > len(runes) {
		maxRunes = len(runes)
	}
	return len(string(runes[:maxRunes]))
}

// findSoftBreak 在软分隔符（逗号等）处分割
func (s *SentenceSegmenter) findSoftBreak(runes []rune) int {
	// 从后向前查找软分隔符
	for i := len(runes) - 1; i >= s.config.MinLength; i-- {
		if softBreakPunctuation[runes[i]] {
			return len(string(runes[:i+1]))
		}
	}
	return 0
}

// findSpaceBreak 在空格处分割
func (s *SentenceSegmenter) findSpaceBreak(runes []rune) int {
	// 从后向前查找空格
	for i := len(runes) - 1; i >= s.config.MinLength; i-- {
		if unicode.IsSpace(runes[i]) {
			return len(string(runes[:i+1]))
		}
	}
	return 0
}

// isSentenceEnder 检查是否为句尾标点
func (s *SentenceSegmenter) isSentenceEnder(r rune) bool {
	switch s.config.Language {
	case "zh":
		return chineseSentenceEnders[r] || englishSentenceEnders[r]
	case "en":
		return englishSentenceEnders[r]
	case "ja":
		return japaneseSentenceEnders[r] || englishSentenceEnders[r]
	default: // auto
		return chineseSentenceEnders[r] || englishSentenceEnders[r] || japaneseSentenceEnders[r]
	}
}

// isSpecialCase 检查是否为特殊情况（不应分句）
func (s *SentenceSegmenter) isSpecialCase(textBefore, textAfter string, punct rune) bool {
	// 只对英文句点进行特殊处理
	if punct != '.' {
		return false
	}

	// 关键检查：如果后面是 空格+大写字母，通常是新句子开始
	// 例如: "Dr. Smith" vs "Hello. How"
	if isNewSentenceStart(textAfter) {
		// 即使是缩写，如果后面看起来像新句子，也应该分句
		// 但某些缩写后面常跟名字（Dr. Mr. Mrs.），需要保留
		if !isPrefixAbbreviation(textBefore) {
			return false
		}
	}

	// 1. 检查缩写词
	if s.isAbbreviation(textBefore) {
		// 如果是称谓缩写（Dr. Mr.），不分句
		if isPrefixAbbreviation(textBefore) {
			return true
		}
		// 其他缩写（etc. e.g.），如果后面是新句子则分句
		if isNewSentenceStart(textAfter) {
			return false
		}
		return true
	}

	// 2. 检查数字模式（小数、版本号等）
	// 只有当数字不完整时才跳过（如 "3." 还没输入完）
	for _, pattern := range numberPatterns {
		if pattern.MatchString(textBefore) {
			// 如果后面是空格+大写字母，说明数字已完整，应分句
			if isNewSentenceStart(textAfter) {
				return false
			}
			return true
		}
	}

	// 3. 检查 URL/Email
	for _, pattern := range urlEmailPatterns {
		if pattern.MatchString(textBefore) {
			// 如果后面是空格+大写字母，说明 URL 已完整，应分句
			if isNewSentenceStart(textAfter) {
				return false
			}
			return true
		}
	}

	// 4. 检查省略号
	if strings.HasSuffix(textBefore, "..") {
		return true // 还在省略号中间
	}

	// 5. 检查后续字符：如果紧跟小写字母（无空格），可能是 URL 的一部分
	if len(textAfter) > 0 {
		nextRune, _ := utf8.DecodeRuneInString(textAfter)
		if unicode.IsLower(nextRune) {
			return true
		}
	}

	return false
}

// isNewSentenceStart 检查后续文本是否像新句子的开始
// 新句子通常以 空格+大写字母 开头
func isNewSentenceStart(textAfter string) bool {
	if len(textAfter) < 2 {
		return false
	}

	// 跳过开头的空白字符
	trimmed := strings.TrimLeft(textAfter, " \t")
	if len(trimmed) == 0 {
		return false
	}

	// 检查第一个非空白字符是否为大写字母
	firstRune, _ := utf8.DecodeRuneInString(trimmed)

	// 大写字母开头通常表示新句子
	return unicode.IsUpper(firstRune)
}

// isPrefixAbbreviation 检查是否为称谓缩写（后面通常跟名字）
func isPrefixAbbreviation(text string) bool {
	text = strings.TrimSuffix(text, ".")
	text = strings.TrimSpace(text)
	words := strings.Fields(text)
	if len(words) == 0 {
		return false
	}
	lastWord := strings.ToLower(words[len(words)-1])

	// 称谓缩写列表
	prefixAbbrs := map[string]bool{
		"mr": true, "mrs": true, "ms": true, "dr": true, "prof": true,
		"sr": true, "jr": true, "st": true, "rev": true, "gen": true,
		"col": true, "lt": true, "sgt": true, "capt": true,
	}

	return prefixAbbrs[lastWord]
}

// isAbbreviation 检查是否为缩写词
func (s *SentenceSegmenter) isAbbreviation(text string) bool {
	// 移除末尾句点
	text = strings.TrimSuffix(text, ".")
	text = strings.TrimSpace(text)

	// 获取最后一个词
	words := strings.Fields(text)
	if len(words) == 0 {
		return false
	}
	lastWord := strings.ToLower(words[len(words)-1])
	lastWord = strings.TrimSuffix(lastWord, ".") // 处理 e.g. i.e. 等

	return commonAbbreviations[lastWord]
}

// resetTimer 重置超时计时器
func (s *SentenceSegmenter) resetTimer() {
	s.stopTimer()

	s.timer = time.AfterFunc(s.config.FlushTimeout, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		// 超时触发：尝试在软分隔符处分句
		if s.buffer.Len() > 0 {
			s.tryFlush(true)
		}
	})
}

// stopTimer 停止计时器
func (s *SentenceSegmenter) stopTimer() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// flushBuffer 刷新缓冲区
func (s *SentenceSegmenter) flushBuffer(isFinal bool) {
	content := strings.TrimSpace(s.buffer.String())
	s.buffer.Reset()

	if content != "" && s.callback != nil {
		s.callback(content, isFinal)
	}
}

// ============================================================
// SentenceSegmenterElement - Pipeline Element 封装
// ============================================================

// SentenceSegmenterElement 将分句器封装为 Pipeline Element
// 用于在 LLM Element 和 TTS Element 之间进行分句处理
type SentenceSegmenterElement struct {
	*SentenceSegmenter
	// 可扩展为完整的 Element 实现
}

// NewSentenceSegmenterElement 创建分句器 Element
func NewSentenceSegmenterElement(config SentenceSegmenterConfig) *SentenceSegmenterElement {
	return &SentenceSegmenterElement{
		SentenceSegmenter: NewSentenceSegmenter(config),
	}
}
