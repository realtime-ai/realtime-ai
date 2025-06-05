package tokenizer

import "context"

// TokenizerConfig 分词器配置接口
type TokenizerConfig interface {
	// Clone 克隆配置
	Clone() TokenizerConfig
}

// SentenceTokenizer 句子分词器接口
type SentenceTokenizer interface {
	// Init 初始化分词器
	Init(ctx context.Context) error

	// Feed 输入一段文本，返回分词结果
	Feed(text string) []string

	// FeedChar 输入单个字符，返回分词结果（用于流式输入）
	FeedChar(char rune) []string

	// Reset 重置分词器状态
	Reset()

	// Config 获取分词器配置
	Config() TokenizerConfig

	// SetConfig 设置分词器配置
	SetConfig(config TokenizerConfig) error

	// Close 关闭分词器，释放资源
	Close() error
}
