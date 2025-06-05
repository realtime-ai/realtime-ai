package tokenizer

import (
	"context"
	"strings"
	"sync"
	"unicode"
)

var _ SentenceTokenizer = (*RuleBoundaryTokenizer)(nil)

// RuleBoundaryConfig 基于规则的分词器配置
type RuleBoundaryConfig struct {
	MinLength      int  // 最小句子长度
	MaxLength      int  // 最大句子长度
	TrimSpace      bool // 是否去除首尾空白
	MergeNewlines  bool // 是否合并连续换行
	KeepEmpty      bool // 是否保留空行
	EnableCJK      bool // 是否启用中日韩文字处理
	EnableWestern  bool // 是否启用西文处理
	EnableEmoji    bool // 是否启用表情符号处理
	EnableURLs     bool // 是否启用URL处理
	EnableNumbers  bool // 是否启用数字处理
	EnableQuotes   bool // 是否启用引号处理
	EnableEllipsis bool // 是否启用省略号处理
}

// Clone 实现 TokenizerConfig 接口
func (c *RuleBoundaryConfig) Clone() TokenizerConfig {
	clone := *c
	return &clone
}

// DefaultRuleBoundaryConfig 返回默认配置
func DefaultRuleBoundaryConfig() *RuleBoundaryConfig {
	return &RuleBoundaryConfig{
		MinLength:      10,
		MaxLength:      1000,
		TrimSpace:      true,
		MergeNewlines:  true,
		KeepEmpty:      false,
		EnableCJK:      true,
		EnableWestern:  true,
		EnableEmoji:    true,
		EnableURLs:     true,
		EnableNumbers:  true,
		EnableQuotes:   true,
		EnableEllipsis: true,
	}
}

// RuleBoundaryTokenizer 基于规则的句子分词器
type RuleBoundaryTokenizer struct {
	config *RuleBoundaryConfig
	mu     sync.Mutex

	buffer     strings.Builder
	sentences  []string
	inQuote    bool
	inURL      bool
	inNumber   bool   // 是否在处理数字
	inAbbr     bool   // 是否在处理缩略词
	abbrBuffer []rune // 缓存可能的缩略词字符
	nextChar   rune   // 缓存下一个字符，用于前瞻

	// 新增前瞻缓冲区
	lookAheadBuffer []rune
	bufferSize      int
}

// NewRuleBoundaryTokenizer 创建新的基于规则的分词器
func NewRuleBoundaryTokenizer(config *RuleBoundaryConfig) *RuleBoundaryTokenizer {
	if config == nil {
		config = DefaultRuleBoundaryConfig()
	}
	return &RuleBoundaryTokenizer{
		config:          config,
		sentences:       make([]string, 0),
		abbrBuffer:      make([]rune, 0, 8),
		lookAheadBuffer: make([]rune, 0, 32), // 预分配32个字符的缓冲区
		bufferSize:      32,                  // 默认缓冲区大小
	}
}

// Init 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) Init(ctx context.Context) error {
	return nil
}

// Feed 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) Feed(text string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, char := range text {
		d.processChar(char)
	}

	return d.flushSentences()
}

// FeedChar 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) FeedChar(char rune) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.processChar(char)
	return d.flushSentences()
}

// Reset 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buffer.Reset()
	d.sentences = d.sentences[:0]
	d.inQuote = false
	d.inURL = false
	d.inNumber = false
	d.inAbbr = false
	d.abbrBuffer = d.abbrBuffer[:0]
	d.nextChar = 0
	d.lookAheadBuffer = d.lookAheadBuffer[:0]
}

// Config 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) Config() TokenizerConfig {
	return d.config
}

// SetConfig 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) SetConfig(config TokenizerConfig) error {
	if cfg, ok := config.(*RuleBoundaryConfig); ok {
		d.config = cfg
		return nil
	}
	return ErrInvalidConfig
}

// Close 实现 SentenceTokenizer 接口
func (d *RuleBoundaryTokenizer) Close() error {
	d.Reset()
	return nil
}

// processChar 处理单个字符
func (d *RuleBoundaryTokenizer) processChar(char rune) {
	// 将字符添加到前瞻缓冲区
	d.lookAheadBuffer = append(d.lookAheadBuffer, char)

	// 如果缓冲区未满，继续收集
	if len(d.lookAheadBuffer) < d.bufferSize {
		return
	}

	// 处理缓冲区中的第一个字符
	charToProcess := d.lookAheadBuffer[0]
	// 更新前瞻字符为缓冲区中的下一个字符
	if len(d.lookAheadBuffer) > 1 {
		d.nextChar = d.lookAheadBuffer[1]
	}

	// 如果在缩略词状态，收集字符直到能确定是否为缩略词
	if d.inAbbr {
		if unicode.IsSpace(charToProcess) {
			// 检查前瞻缓冲区中是否包含完整的缩略词
			bufferedText := string(d.lookAheadBuffer)
			words := strings.Fields(bufferedText)
			if len(words) > 0 && d.isCompleteAbbreviation(words[0]) {
				// 是完整的缩略词，写入缓冲区
				for _, ch := range d.abbrBuffer {
					d.buffer.WriteRune(ch)
				}
				d.buffer.WriteRune(charToProcess)
			} else {
				// 不是完整的缩略词，当作句子结束处理
				d.appendSentence()
				d.buffer.WriteRune(charToProcess)
			}
			d.inAbbr = false
			d.abbrBuffer = d.abbrBuffer[:0]
		} else {
			d.abbrBuffer = append(d.abbrBuffer, charToProcess)
			// 检查是否可能是缩略词的一部分
			if !d.isPossibleAbbreviation(string(d.abbrBuffer)) {
				d.appendSentence()
				d.buffer.WriteRune(charToProcess)
				d.inAbbr = false
				d.abbrBuffer = d.abbrBuffer[:0]
			}
		}
		// 移除已处理的字符
		d.lookAheadBuffer = d.lookAheadBuffer[1:]
		return
	}

	// 检查是否开始一个新的缩略词
	if charToProcess == '.' {
		text := d.buffer.String()
		words := strings.Fields(text)
		if len(words) > 0 {
			lastWord := words[len(words)-1]
			// 检查前瞻缓冲区中的内容
			bufferedText := string(d.lookAheadBuffer)
			if d.isPossibleAbbreviation(lastWord + bufferedText) {
				d.inAbbr = true
				d.abbrBuffer = []rune(lastWord + string(charToProcess))
				// 从buffer中移除最后一个词
				d.buffer.Reset()
				d.buffer.WriteString(strings.TrimSuffix(text, lastWord))
				// 移除已处理的字符
				d.lookAheadBuffer = d.lookAheadBuffer[1:]
				return
			}
		}
	}

	// 如果当前在处理数字状态
	if d.inNumber {
		// 检查是否是合法的数字字符
		if unicode.IsDigit(charToProcess) || // 数字
			charToProcess == '.' || // 小数点
			charToProcess == ',' || // 千位分隔符
			charToProcess == '-' || // 负号或范围分隔符
			charToProcess == '+' || // 正号
			charToProcess == 'e' || charToProcess == 'E' || // 科学计数法
			charToProcess == '%' { // 百分比
			d.buffer.WriteRune(charToProcess)
			return
		}
		// 退出数字状态
		d.inNumber = false
	}

	// 处理URL
	if d.config.EnableURLs {
		if d.inURL {
			if unicode.IsSpace(charToProcess) {
				d.inURL = false
			} else {
				d.buffer.WriteRune(charToProcess)
				return
			}
		} else if strings.HasSuffix(d.buffer.String(), "http") && charToProcess == ':' ||
			strings.HasSuffix(d.buffer.String(), "https") && charToProcess == ':' ||
			strings.HasSuffix(d.buffer.String(), "ftp") && charToProcess == ':' {
			d.inURL = true
			d.buffer.WriteRune(charToProcess)
			return
		}
	}

	// 处理引号
	if d.config.EnableQuotes {
		if charToProcess == '"' || charToProcess == '\u2018' || charToProcess == '\u2019' {
			d.inQuote = !d.inQuote
			d.buffer.WriteRune(charToProcess)
			return
		}
		if d.inQuote {
			d.buffer.WriteRune(charToProcess)
			return
		}
	}

	// 检查是否开始一个新的数字
	if d.config.EnableNumbers && !d.inNumber {
		// 检查是否是数字开始
		if unicode.IsDigit(charToProcess) || // 数字开始
			(charToProcess == '-' || charToProcess == '+') || // 正负号开始
			(charToProcess == '.' && unicode.IsDigit(d.nextChar)) { // 小数点开始且后面是数字
			d.inNumber = true
			d.buffer.WriteRune(charToProcess)
			return
		}
	}

	// 检查是否在处理缩略词
	if d.inAbbr {
		d.buffer.WriteRune(charToProcess)
		// 如果遇到空格，退出缩略词状态
		if unicode.IsSpace(charToProcess) {
			d.inAbbr = false
		}
		return
	}

	// 检查是否开始缩略词
	text := d.buffer.String()
	if charToProcess == '.' {
		words := strings.Fields(text)
		if len(words) > 0 {
			lastWord := words[len(words)-1]
			// 检查常见缩略词模式
			if d.isAbbreviationPattern(lastWord + string(charToProcess)) {
				d.inAbbr = true
				d.buffer.WriteRune(charToProcess)
				return
			}
		}
	}

	// 处理句子结束标记
	if d.isSentenceEnd(charToProcess) {
		d.buffer.WriteRune(charToProcess)
		d.appendSentence()
		return
	}

	// 处理换行
	if charToProcess == '\n' {
		if d.config.KeepEmpty {
			if d.buffer.Len() == 0 {
				d.sentences = append(d.sentences, "")
			} else {
				d.appendSentence()
			}
		} else if !d.config.MergeNewlines {
			d.appendSentence()
		}
		return
	}

	// 处理其他字符
	d.buffer.WriteRune(charToProcess)

	// 检查最大长度
	if d.config.MaxLength > 0 && d.buffer.Len() >= d.config.MaxLength {
		d.appendSentence()
	}

	// 移除已处理的字符
	d.lookAheadBuffer = d.lookAheadBuffer[1:]
}

// isSentenceEnd 判断字符是否为句子结束标记
func (d *RuleBoundaryTokenizer) isSentenceEnd(char rune) bool {
	// 如果在处理 URL，不要分割句子
	if d.inURL {
		return false
	}

	// 中文句子结束标记
	if d.config.EnableCJK && (char == '。' || char == '！' || char == '？' || char == '…') {
		return true
	}

	// 西文句子结束标记
	if d.config.EnableWestern && (char == '.' || char == '!' || char == '?') {
		text := d.buffer.String()

		// 如果是点号，需要进行额外检查
		if char == '.' {
			// 检查是否是缩略词
			if d.isAbbreviation(text) {
				return false
			}

			// 检查是否是数值中的小数点
			if d.isNumberDecimalPoint(text) {
				return false
			}

			// 检查是否是 URL 中的点号
			if d.isURLComponent(text) {
				return false
			}
		}
		return true
	}

	return false
}

// isAbbreviation 判断文本是否为缩略词
func (d *RuleBoundaryTokenizer) isAbbreviation(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return false
	}

	// 常见缩略词
	commonAbbreviations := map[string]bool{
		"Mr.":    true,
		"Mrs.":   true,
		"Ms.":    true,
		"Dr.":    true,
		"Prof.":  true,
		"Sr.":    true,
		"Jr.":    true,
		"vs.":    true,
		"etc.":   true,
		"i.e.":   true,
		"e.g.":   true,
		"U.S.":   true,
		"U.K.":   true,
		"Ph.D.":  true,
		"M.D.":   true,
		"B.A.":   true,
		"M.A.":   true,
		"D.D.S.": true,
	}

	// 检查词尾是否是 a.m. 或 p.m.
	words := strings.Fields(text)
	if len(words) > 0 {
		lastWord := words[len(words)-1]
		lowerLastWord := strings.ToLower(lastWord)
		if lowerLastWord == "a.m." || lowerLastWord == "p.m." ||
			lowerLastWord == "a.m" || lowerLastWord == "p.m" {
			return true
		}
	}

	// 检查常见缩略词
	for abbr := range commonAbbreviations {
		if strings.HasSuffix(text, abbr) {
			// 确保前面是词边界（空格或文本开始）
			if len(text) == len(abbr) || unicode.IsSpace(rune(text[len(text)-len(abbr)-1])) {
				return true
			}
		}
	}

	// 检查多个点号的缩略词（如 U.S.A.）
	if strings.Count(text, ".") > 1 {
		words := strings.Fields(text)
		if len(words) > 0 {
			lastWord := words[len(words)-1]
			// 检查是否每个字符都是大写字母或点号
			isValid := true
			for _, ch := range lastWord {
				if ch != '.' && !unicode.IsUpper(ch) {
					isValid = false
					break
				}
			}
			if isValid && strings.Contains(lastWord, ".") {
				return true
			}
		}
	}

	return false
}

// isNumberDecimalPoint 判断当前点号是否是数值中的小数点
func (d *RuleBoundaryTokenizer) isNumberDecimalPoint(text string) bool {
	if !d.config.EnableNumbers {
		return false
	}

	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return false
	}

	// 检查点号前面的字符
	lastChar := rune(text[len(text)-1])
	if !unicode.IsDigit(lastChar) {
		// 特殊处理科学计数法
		if lastChar == 'e' || lastChar == 'E' {
			if len(text) > 1 {
				prevChar := rune(text[len(text)-2])
				if unicode.IsDigit(prevChar) {
					return true
				}
			}
		}
		return false
	}

	// 检查下一个字符是否是数字（使用缓存的前瞻字符）
	if !unicode.IsDigit(d.nextChar) {
		return false
	}

	return true
}

// isURLComponent 判断文本是否为 URL 组件
func (d *RuleBoundaryTokenizer) isURLComponent(text string) bool {
	// 常见的顶级域名
	commonTLDs := map[string]bool{
		"com": true,
		"org": true,
		"net": true,
		"edu": true,
		"gov": true,
		"io":  true,
		"ai":  true,
		"cn":  true,
		"uk":  true,
		"ru":  true,
		"de":  true,
		"jp":  true,
		"fr":  true,
	}

	text = strings.TrimSpace(text)
	parts := strings.Split(text, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// 检查是否是常见顶级域名
		if commonTLDs[lastPart] {
			return true
		}
	}

	// 检查是否包含常见的 URL 字符
	if strings.Contains(text, "://") ||
		strings.Contains(text, "www.") {
		return true
	}

	return false
}

// isAbbreviationPattern 检查是否匹配缩略词模式
func (d *RuleBoundaryTokenizer) isAbbreviationPattern(word string) bool {
	// 常见缩略词模式
	patterns := []string{
		"Mr.", "Mrs.", "Ms.", "Dr.", "Prof.",
		"Sr.", "Jr.", "vs.", "etc.", "i.e.", "e.g.",
		"U.S.", "U.K.", "Ph.D.", "M.D.", "B.A.", "M.A.", "D.D.S.",
		"a.m", "p.m", "A.M", "P.M",
	}

	word = strings.TrimSpace(word)
	for _, pattern := range patterns {
		if strings.EqualFold(word, pattern) {
			return true
		}
	}

	// 检查是否是多点号缩略词（如 U.S.A.）
	if strings.Count(word, ".") > 1 {
		isValid := true
		for _, ch := range word {
			if ch != '.' && !unicode.IsUpper(ch) {
				isValid = false
				break
			}
		}
		return isValid
	}

	return false
}

// appendSentence 添加句子到结果列表
func (d *RuleBoundaryTokenizer) appendSentence() {
	text := d.buffer.String()
	if d.config.TrimSpace {
		text = strings.TrimSpace(text)
	}

	if len(text) >= d.config.MinLength || (d.config.KeepEmpty && text == "") {
		d.sentences = append(d.sentences, text)
	}

	d.buffer.Reset()
}

// flushSentences 返回并清空已检测到的句子
func (d *RuleBoundaryTokenizer) flushSentences() []string {
	if d.buffer.Len() > 0 {
		d.appendSentence()
	}

	sentences := make([]string, len(d.sentences))
	copy(sentences, d.sentences)
	d.sentences = d.sentences[:0]

	return sentences
}

// isPossibleAbbreviation 检查是否可能是缩略词的一部分
func (d *RuleBoundaryTokenizer) isPossibleAbbreviation(text string) bool {
	// 检查常见缩略词前缀
	prefixes := []string{
		"Mr", "Mrs", "Ms", "Dr", "Prof",
		"Sr", "Jr", "vs", "etc", "i.e", "e.g",
		"U.S", "U.K", "Ph.D", "M.D", "B.A", "M.A", "D.D.S",
		"a", "p", "A", "P", // a.m. 和 p.m. 的前缀
	}

	text = strings.TrimSuffix(text, ".")
	for _, prefix := range prefixes {
		if strings.HasPrefix(prefix, text) {
			return true
		}
	}

	// 检查是否可能是多点号缩略词（如 U.S.A）
	if strings.Contains(text, ".") {
		for _, ch := range text {
			if ch != '.' && !unicode.IsUpper(ch) {
				return false
			}
		}
		return true
	}

	return false
}

// isCompleteAbbreviation 检查是否是完整的缩略词
func (d *RuleBoundaryTokenizer) isCompleteAbbreviation(text string) bool {
	// 常见完整缩略词
	completeAbbrs := map[string]bool{
		"Mr.": true, "Mrs.": true, "Ms.": true,
		"Dr.": true, "Prof.": true, "Sr.": true,
		"Jr.": true, "vs.": true, "etc.": true,
		"i.e.": true, "e.g.": true,
		"U.S.": true, "U.K.": true,
		"Ph.D.": true, "M.D.": true,
		"B.A.": true, "M.A.": true, "D.D.S.": true,
		"a.m.": true, "p.m.": true,
		"A.M.": true, "P.M.": true,
		"U.S.A.": true,
	}

	return completeAbbrs[text] || completeAbbrs[strings.ToUpper(text)]
}
