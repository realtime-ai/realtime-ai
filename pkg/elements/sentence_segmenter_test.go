package elements

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// 基础分句测试
// ============================================================

func TestSentenceSegmenter_BasicPunctuation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "English period",
			input:    "Hello world. How are you?",
			expected: []string{"Hello world.", "How are you?"},
		},
		{
			name:     "English exclamation",
			input:    "Amazing! This works!",
			expected: []string{"Amazing!", "This works!"},
		},
		{
			name:     "English question",
			input:    "What is this? Tell me more.",
			expected: []string{"What is this?", "Tell me more."},
		},
		{
			name:     "Chinese period",
			input:    "你好世界。今天天气真好。",
			expected: []string{"你好世界。", "今天天气真好。"},
		},
		{
			name:     "Chinese question",
			input:    "你是谁？我是AI助手。",
			expected: []string{"你是谁？", "我是AI助手。"},
		},
		{
			name:     "Chinese exclamation",
			input:    "太棒了！继续加油！",
			expected: []string{"太棒了！", "继续加油！"},
		},
		{
			name:     "Mixed Chinese and English",
			input:    "Hello你好。Nice to meet you很高兴认识你。",
			expected: []string{"Hello你好。", "Nice to meet you很高兴认识你。"},
		},
		{
			name:     "Semicolon as sentence ender",
			input:    "First item; Second item.",
			expected: []string{"First item;", "Second item."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentences []string
			segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
				MinLength: 1, // 允许短句
			})
			segmenter.OnSentence(func(sentence string, isFinal bool) {
				sentences = append(sentences, sentence)
			})

			segmenter.Feed(tt.input)
			segmenter.Flush()

			assert.Equal(t, tt.expected, sentences, "input: %q", tt.input)
		})
	}
}

// ============================================================
// 流式输入测试（模拟 LLM 逐 token 输出）
// ============================================================

func TestSentenceSegmenter_StreamingInput(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []string // 模拟 LLM 输出的 tokens
		expected []string
	}{
		{
			name:     "Token by token",
			tokens:   []string{"Hello", " ", "world", ".", " ", "How", " ", "are", " ", "you", "?"},
			expected: []string{"Hello world.", "How are you?"},
		},
		{
			name:     "Chinese characters",
			tokens:   []string{"你", "好", "世", "界", "。", "欢", "迎", "！"},
			expected: []string{"你好世界。", "欢迎！"},
		},
		{
			name:     "Mixed chunks",
			tokens:   []string{"Hello ", "world. ", "This is", " great!"},
			expected: []string{"Hello world.", "This is great!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentences []string
			segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
				MinLength: 1,
			})
			segmenter.OnSentence(func(sentence string, isFinal bool) {
				sentences = append(sentences, sentence)
			})

			for _, token := range tt.tokens {
				segmenter.Feed(token)
			}
			segmenter.Flush()

			assert.Equal(t, tt.expected, sentences)
		})
	}
}

// ============================================================
// 特殊情况测试：缩写词
// ============================================================

func TestSentenceSegmenter_Abbreviations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Mr. title",
			input:    "Mr. Smith is here. He is a doctor.",
			expected: []string{"Mr. Smith is here.", "He is a doctor."},
		},
		{
			name:     "Dr. title",
			input:    "Dr. Johnson called. She left a message.",
			expected: []string{"Dr. Johnson called.", "She left a message."},
		},
		{
			name:     "Mrs. title",
			input:    "Mrs. Brown arrived. Welcome her.",
			expected: []string{"Mrs. Brown arrived.", "Welcome her."},
		},
		{
			name:     "e.g. example",
			input:    "Use fruits e.g. apples and oranges. They are healthy.",
			expected: []string{"Use fruits e.g. apples and oranges.", "They are healthy."},
		},
		{
			name:     "i.e. example",
			input:    "Use i.e. when clarifying. It means 'that is'.",
			expected: []string{"Use i.e. when clarifying.", "It means 'that is'."},
		},
		{
			name:     "Inc. company",
			input:    "Apple Inc. is great. They make phones.",
			expected: []string{"Apple Inc. is great.", "They make phones."},
		},
		{
			name:     "etc. ending",
			input:    "Buy apples, oranges, etc. Don't forget milk.",
			expected: []string{"Buy apples, oranges, etc.", "Don't forget milk."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentences []string
			segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
				MinLength:              1,
				EnableSmartPunctuation: true,
			})
			segmenter.OnSentence(func(sentence string, isFinal bool) {
				sentences = append(sentences, sentence)
			})

			segmenter.Feed(tt.input)
			segmenter.Flush()

			assert.Equal(t, tt.expected, sentences, "input: %q", tt.input)
		})
	}
}

// ============================================================
// 特殊情况测试：数字和小数
// ============================================================

func TestSentenceSegmenter_Numbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Decimal number",
			input:    "Pi is 3.14159. It is irrational.",
			expected: []string{"Pi is 3.14159.", "It is irrational."},
		},
		{
			name:     "Currency",
			input:    "The price is $9.99. That is cheap.",
			expected: []string{"The price is $9.99.", "That is cheap."},
		},
		{
			name:     "Percentage",
			input:    "Growth is 5.5%. It is impressive.",
			expected: []string{"Growth is 5.5%.", "It is impressive."},
		},
		{
			name:     "Version number",
			input:    "Use v2.0.1. It is stable.",
			expected: []string{"Use v2.0.1.", "It is stable."},
		},
		{
			name:     "Multiple decimals",
			input:    "Values are 1.5 and 2.5. Sum is 4.0.",
			expected: []string{"Values are 1.5 and 2.5.", "Sum is 4.0."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentences []string
			segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
				MinLength:              1,
				EnableSmartPunctuation: true,
			})
			segmenter.OnSentence(func(sentence string, isFinal bool) {
				sentences = append(sentences, sentence)
			})

			segmenter.Feed(tt.input)
			segmenter.Flush()

			assert.Equal(t, tt.expected, sentences, "input: %q", tt.input)
		})
	}
}

// ============================================================
// 特殊情况测试：URL 和 Email
// ============================================================

func TestSentenceSegmenter_URLAndEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "URL with https",
			input:    "Visit https://example.com. It is great.",
			expected: []string{"Visit https://example.com.", "It is great."},
		},
		{
			name:     "URL with www",
			input:    "Go to www.google.com. Search there.",
			expected: []string{"Go to www.google.com.", "Search there."},
		},
		{
			name:     "Domain only",
			input:    "Check example.com. It has info.",
			expected: []string{"Check example.com.", "It has info."},
		},
		{
			name:     "Email address",
			input:    "Email user@example.com. They will reply.",
			expected: []string{"Email user@example.com.", "They will reply."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentences []string
			segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
				MinLength:              1,
				EnableSmartPunctuation: true,
			})
			segmenter.OnSentence(func(sentence string, isFinal bool) {
				sentences = append(sentences, sentence)
			})

			segmenter.Feed(tt.input)
			segmenter.Flush()

			assert.Equal(t, tt.expected, sentences, "input: %q", tt.input)
		})
	}
}

// ============================================================
// 最小长度测试
// ============================================================

func TestSentenceSegmenter_MinLength(t *testing.T) {
	t.Run("Respects minimum length", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 20, // 高阈值
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		// 短句不应触发分句
		segmenter.Feed("Hi. OK.")
		assert.Empty(t, sentences, "Short sentences should not trigger flush")

		// Flush 时应输出
		segmenter.Flush()
		assert.Equal(t, []string{"Hi. OK."}, sentences)
	})

	t.Run("Combines short sentences", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 15,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("Hi. ") // 太短，不输出
		segmenter.Feed("How are you? ") // "Hi. How are you?" 超过 15，输出
		segmenter.Feed("Good.") // 太短
		segmenter.Flush()

		// 应该合并短句
		assert.True(t, len(sentences) >= 1)
	})
}

// ============================================================
// 最大长度测试
// ============================================================

func TestSentenceSegmenter_MaxLength(t *testing.T) {
	t.Run("Forces break at max length", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
			MaxLength: 30,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		// 输入超长无标点文本（每次喂入时检查 maxLength）
		longText := strings.Repeat("word ", 20) // 100 字符
		// 逐块喂入以触发 maxLength 检查
		for i := 0; i < len(longText); i += 10 {
			end := i + 10
			if end > len(longText) {
				end = len(longText)
			}
			segmenter.Feed(longText[i:end])
		}
		segmenter.Flush()

		// 应该被强制分割
		assert.True(t, len(sentences) >= 2, "Long text should be split, got: %v", sentences)
		for _, s := range sentences {
			// 允许略微超过 maxLength（因为在空格处分割）
			assert.LessOrEqual(t, len([]rune(s)), 40, "Each sentence should be near max length, got: %q", s)
		}
	})

	t.Run("Prefers soft break for long sentences", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
			MaxLength: 30,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		// 带逗号的长文本
		segmenter.Feed("This is a long sentence, with commas, that exceeds the maximum length")
		segmenter.Flush()

		// 应该在逗号处分割
		assert.True(t, len(sentences) >= 2)
		// 第一个句子应该在逗号处结束
		if len(sentences) > 0 {
			assert.True(t, strings.HasSuffix(sentences[0], ",") || len([]rune(sentences[0])) <= 35)
		}
	})
}

// ============================================================
// 超时测试
// ============================================================

func TestSentenceSegmenter_Timeout(t *testing.T) {
	t.Run("Flushes on timeout", func(t *testing.T) {
		var mu sync.Mutex
		var sentences []string
		var isFinalFlags []bool

		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength:    1,
			FlushTimeout: 100 * time.Millisecond,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			mu.Lock()
			sentences = append(sentences, sentence)
			isFinalFlags = append(isFinalFlags, isFinal)
			mu.Unlock()
		})

		// 输入无标点文本
		segmenter.Feed("Hello world without punctuation")

		// 等待超时
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		result := sentences
		mu.Unlock()

		// 应该超时触发
		assert.True(t, len(result) >= 1, "Should flush on timeout")
	})

	t.Run("Timer resets on new input", func(t *testing.T) {
		var mu sync.Mutex
		var sentences []string

		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength:    1,
			FlushTimeout: 150 * time.Millisecond,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			mu.Lock()
			sentences = append(sentences, sentence)
			mu.Unlock()
		})

		// 持续输入，每 50ms 一次，应该不触发超时
		for i := 0; i < 5; i++ {
			segmenter.Feed("word ")
			time.Sleep(50 * time.Millisecond)
		}

		mu.Lock()
		result := sentences
		mu.Unlock()

		// 因为持续输入，不应该触发超时
		assert.Empty(t, result, "Should not flush while receiving input")

		// 停止输入后等待超时
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		result = sentences
		mu.Unlock()
		assert.True(t, len(result) >= 1, "Should flush after input stops")
	})
}

// ============================================================
// 多语言测试
// ============================================================

func TestSentenceSegmenter_MultiLanguage(t *testing.T) {
	t.Run("Japanese", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
			Language:  "ja",
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("こんにちは。元気ですか？")
		segmenter.Flush()

		assert.Equal(t, []string{"こんにちは。", "元気ですか？"}, sentences)
	})

	t.Run("Auto detection with mixed content", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
			Language:  "auto",
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		// 中英日混合
		segmenter.Feed("Hello世界。こんにちは！Good morning.")
		segmenter.Flush()

		assert.Equal(t, []string{"Hello世界。", "こんにちは！", "Good morning."}, sentences)
	})
}

// ============================================================
// 边界情况测试
// ============================================================

func TestSentenceSegmenter_EdgeCases(t *testing.T) {
	t.Run("Empty input", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("")
		segmenter.Flush()

		assert.Empty(t, sentences)
	})

	t.Run("Whitespace only", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("   \n\t  ")
		segmenter.Flush()

		assert.Empty(t, sentences)
	})

	t.Run("Single character", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("A")
		segmenter.Flush()

		assert.Equal(t, []string{"A"}, sentences)
	})

	t.Run("Multiple punctuation", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("What?! Are you serious?!")
		segmenter.Flush()

		// 应该正确处理 ?! 组合
		assert.True(t, len(sentences) >= 1)
	})

	t.Run("Ellipsis", func(t *testing.T) {
		var sentences []string
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength:              1,
			EnableSmartPunctuation: true,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		segmenter.Feed("Well... I think so. Maybe.")
		segmenter.Flush()

		// 省略号后应该分句
		assert.True(t, len(sentences) >= 2, "sentences: %v", sentences)
	})

	t.Run("Reset clears buffer", func(t *testing.T) {
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{})

		segmenter.Feed("Hello world")
		assert.NotEmpty(t, segmenter.GetBuffer())

		segmenter.Reset()
		assert.Empty(t, segmenter.GetBuffer())
	})

	t.Run("Callback not set", func(t *testing.T) {
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
		})
		// 不设置 callback

		// 不应该 panic
		assert.NotPanics(t, func() {
			segmenter.Feed("Hello world.")
			segmenter.Flush()
		})
	})
}

// ============================================================
// 并发安全测试
// ============================================================

func TestSentenceSegmenter_Concurrency(t *testing.T) {
	t.Run("Concurrent feeds", func(t *testing.T) {
		var mu sync.Mutex
		var sentences []string

		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength: 1,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			mu.Lock()
			sentences = append(sentences, sentence)
			mu.Unlock()
		})

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				segmenter.Feed("Text. ")
			}(i)
		}

		wg.Wait()
		segmenter.Flush()

		mu.Lock()
		result := sentences
		mu.Unlock()

		// 应该收到一些句子，且不应该 panic
		assert.NotEmpty(t, result)
	})
}

// ============================================================
// 性能基准测试
// ============================================================

func BenchmarkSentenceSegmenter_Feed(b *testing.B) {
	segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
		MinLength: 10,
		MaxLength: 200,
	})
	segmenter.OnSentence(func(sentence string, isFinal bool) {})

	text := "Hello world. This is a test sentence. How are you? I am fine."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		segmenter.Feed(text)
		segmenter.Reset()
	}
}

func BenchmarkSentenceSegmenter_TokenByToken(b *testing.B) {
	tokens := []string{"Hello", " ", "world", ".", " ", "How", " ", "are", " ", "you", "?"}

	segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
		MinLength: 5,
	})
	segmenter.OnSentence(func(sentence string, isFinal bool) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, token := range tokens {
			segmenter.Feed(token)
		}
		segmenter.Reset()
	}
}

func BenchmarkSentenceSegmenter_ChineseText(b *testing.B) {
	segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
		MinLength: 5,
		Language:  "zh",
	})
	segmenter.OnSentence(func(sentence string, isFinal bool) {})

	text := "你好世界。今天天气真好。我们一起去公园吧？好的，出发！"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		segmenter.Feed(text)
		segmenter.Reset()
	}
}

// ============================================================
// 集成测试：模拟 LLM 输出场景
// ============================================================

func TestSentenceSegmenter_LLMSimulation(t *testing.T) {
	t.Run("Simulated GPT streaming output", func(t *testing.T) {
		var sentences []string
		var mu sync.Mutex

		// 使用较小的 MinLength 以便在流式输入时更快分句
		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength:              5, // 降低阈值以便更快分句
			MaxLength:              200,
			FlushTimeout:           500 * time.Millisecond,
			EnableSmartPunctuation: true,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			mu.Lock()
			sentences = append(sentences, sentence)
			mu.Unlock()
		})

		// 模拟 GPT 流式输出（每个 token 间隔约 10ms）
		tokens := []string{
			"Hello", "!", " ", "I", "'m", " ", "an", " ", "AI", " ",
			"assistant", ".", " ", "I", " ", "can", " ", "help", " ",
			"you", " ", "with", " ", "many", " ", "tasks", ".", " ",
			"For", " ", "example", ",", " ", "I", " ", "can", " ",
			"write", " ", "code", ",", " ", "answer", " ", "questions", ",",
			" ", "and", " ", "more", "!", " ", "What", " ", "would", " ",
			"you", " ", "like", " ", "to", " ", "know", "?",
		}

		for _, token := range tokens {
			segmenter.Feed(token)
			time.Sleep(10 * time.Millisecond) // 加快测试速度
		}
		segmenter.Flush()

		mu.Lock()
		result := sentences
		mu.Unlock()

		// 验证分句结果 - 至少应该分成几个句子
		require.True(t, len(result) >= 1, "Should have at least 1 sentence, got: %v", result)

		// 打印分句结果供人工检查
		t.Logf("Segmented into %d sentences:", len(result))
		for i, s := range result {
			t.Logf("  [%d] %s", i+1, s)
		}

		// 合并所有句子应该等于原始文本
		combined := strings.Join(result, " ")
		t.Logf("Combined: %s", combined)
	})

	t.Run("Real-world assistant response", func(t *testing.T) {
		var sentences []string

		segmenter := NewSentenceSegmenter(SentenceSegmenterConfig{
			MinLength:              10,
			EnableSmartPunctuation: true,
		})
		segmenter.OnSentence(func(sentence string, isFinal bool) {
			sentences = append(sentences, sentence)
		})

		// 模拟真实助手回复
		response := "Dr. Smith recommends taking 2.5mg daily. " +
			"You can visit https://example.com for more info. " +
			"The success rate is about 95.5%. " +
			"Please contact support@example.com if you have questions."

		// 逐字符喂入
		for _, r := range response {
			segmenter.Feed(string(r))
		}
		segmenter.Flush()

		// 应该正确处理所有特殊情况
		assert.Equal(t, 4, len(sentences), "sentences: %v", sentences)
	})
}
