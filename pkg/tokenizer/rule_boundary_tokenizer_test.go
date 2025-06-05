package tokenizer

import (
	"context"
	"reflect"
	"testing"
)

func TestRuleBoundaryTokenizer_SimpleText(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil) // 使用默认配置
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "中文简单句子",
			input: "这是一个简单的句子。这是第二个句子。",
			expected: []string{
				"这是一个简单的句子。",
				"这是第二个句子。",
			},
		},
		{
			name:  "英文简单句子",
			input: "This is a simple sentence. This is the second sentence.",
			expected: []string{
				"This is a simple sentence.",
				"This is the second sentence.",
			},
		},
		{
			name:  "混合语言",
			input: "这是中文。This is English. 这是混合Chinese and 中文！",
			expected: []string{
				"这是中文。",
				"This is English.",
				"这是混合Chinese and 中文！",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_URLs(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "简单URL",
			input: "请访问 https://example.com/page.html 获取更多信息。下一句开始。",
			expected: []string{
				"请访问 https://example.com/page.html 获取更多信息。",
				"下一句开始。",
			},
		},
		{
			name:  "多个URL",
			input: "第一个链接 http://site1.com，第二个链接 https://site2.com。",
			expected: []string{
				"第一个链接 http://site1.com，第二个链接 https://site2.com。",
			},
		},
		{
			name:  "带参数的URL",
			input: "访问 https://example.com/search?q=test&page=1 查看结果。",
			expected: []string{
				"访问 https://example.com/search?q=test&page=1 查看结果。",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_Abbreviations(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "英文缩略词",
			input: "Mr. Smith visited Dr. Johnson at 9 a.m. Then he went home.",
			expected: []string{
				"Mr. Smith visited Dr. Johnson at 9 a.m.",
				"Then he went home.",
			},
		},
		{
			name:  "多个缩略词",
			input: "Prof. Lee and Dr. Wang from the U.S.A. visited us.",
			expected: []string{
				"Prof. Lee and Dr. Wang from the U.S.A. visited us.",
			},
		},
		{
			name:  "时间缩略词",
			input: "The meeting is at 9 a.m. The lunch is at 12 p.m. Sharp!",
			expected: []string{
				"The meeting is at 9 a.m.",
				"The lunch is at 12 p.m. Sharp!",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_QuotedText(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "英文引号",
			input: `He said: "This is great. I love it." Then he left.`,
			expected: []string{
				`He said: "This is great. I love it." Then he left.`,
			},
		},
		{
			name:  "中文引号",
			input: `他说："今天天气真好。明天也会很好。"然后离开了。`,
			expected: []string{
				`他说："今天天气真好。明天也会很好。"然后离开了。`,
			},
		},
		{
			name:  "嵌套引号",
			input: `他说："小明告诉我'今天天气真好'，我也这么觉得。"`,
			expected: []string{
				`他说："小明告诉我'今天天气真好'，我也这么觉得。"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_StreamingInput(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	input := "第一句话。第二句话。"
	expected := []string{
		"第一句话。",
		"第二句话。",
	}

	var got []string
	for _, char := range input {
		if sentences := tokenizer.FeedChar(char); len(sentences) > 0 {
			got = append(got, sentences...)
		}
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Streaming input result = %v, want %v", got, expected)
	}
}

func TestRuleBoundaryTokenizer_LongSentences(t *testing.T) {
	config := &RuleBoundaryConfig{
		MinLength:     1,
		MaxLength:     15,
		TrimSpace:     true,
		MergeNewlines: true,
		KeepEmpty:     false,
		EnableCJK:     true,
		EnableWestern: true,
	}

	tokenizer := NewRuleBoundaryTokenizer(config)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "长句子分割",
			input: "这是一个非常非常非常非常非常非常非常非常长的句子，需要被分割。",
			expected: []string{
				"这是一个非常非常非常",
				"非常非常非常非常",
				"非常长的句子，",
				"需要被分割。",
			},
		},
		{
			name:  "英文长句子",
			input: "This is a very very very very very very very very long sentence that needs to be split.",
			expected: []string{
				"This is a very",
				"very very very",
				"very very very",
				"very long",
				"sentence that",
				"needs to be",
				"split.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_EmptyLines(t *testing.T) {
	config := &RuleBoundaryConfig{
		MinLength:     1,
		MaxLength:     1000,
		TrimSpace:     true,
		MergeNewlines: false,
		KeepEmpty:     true,
	}

	tokenizer := NewRuleBoundaryTokenizer(config)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "空行保留",
			input: "第一行。\n\n第二行。",
			expected: []string{
				"第一行。",
				"",
				"第二行。",
			},
		},
		{
			name:  "多个空行",
			input: "第一段。\n\n\n第二段。",
			expected: []string{
				"第一段。",
				"",
				"",
				"第二段。",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_Reset(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	// 第一次输入
	input1 := "第一句。"
	got1 := tokenizer.Feed(input1)
	expected1 := []string{"第一句。"}

	if !reflect.DeepEqual(got1, expected1) {
		t.Errorf("First feed = %v, want %v", got1, expected1)
	}

	// 重置
	tokenizer.Reset()

	// 第二次输入
	input2 := "第二句。"
	got2 := tokenizer.Feed(input2)
	expected2 := []string{"第二句。"}

	if !reflect.DeepEqual(got2, expected2) {
		t.Errorf("Second feed after reset = %v, want %v", got2, expected2)
	}
}

func TestRuleBoundaryTokenizer_CustomConfig(t *testing.T) {
	config := &RuleBoundaryConfig{
		MinLength:      5,
		MaxLength:      50,
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

	tokenizer := NewRuleBoundaryTokenizer(config)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "最小长度过滤",
			input: "短。这是一个长度超过五个字的句子。",
			expected: []string{
				"这是一个长度超过五个字的句子。",
			},
		},
		{
			name:  "数字和表情符号",
			input: "价格是99.9%。😊这真是太好了！",
			expected: []string{
				"价格是99.9%。",
				"😊这真是太好了！",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_ConfigClone(t *testing.T) {
	config := &RuleBoundaryConfig{
		MinLength:     5,
		MaxLength:     50,
		TrimSpace:     true,
		MergeNewlines: true,
		KeepEmpty:     false,
	}

	tokenizer := NewRuleBoundaryTokenizer(config)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	// 获取配置并修改
	currentConfig := tokenizer.Config()
	clonedConfig := currentConfig.Clone()

	// 修改原始配置
	config.MinLength = 10
	config.MaxLength = 100

	// 验证克隆的配置没有被修改
	if cfg, ok := clonedConfig.(*RuleBoundaryConfig); ok {
		if cfg.MinLength != 5 || cfg.MaxLength != 50 {
			t.Errorf("Cloned config was modified: got MinLength=%d, MaxLength=%d, want MinLength=5, MaxLength=50",
				cfg.MinLength, cfg.MaxLength)
		}
	} else {
		t.Error("Failed to cast cloned config to RuleBoundaryConfig")
	}
}

// invalidConfig 用于测试无效配置
type invalidConfig struct{}

func (c *invalidConfig) Clone() TokenizerConfig { return c }

func TestRuleBoundaryTokenizer_InvalidConfig(t *testing.T) {
	tokenizer := NewRuleBoundaryTokenizer(nil)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	// 尝试设置无效配置
	err := tokenizer.SetConfig(&invalidConfig{})
	if err != ErrInvalidConfig {
		t.Errorf("SetConfig() with invalid config = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestRuleBoundaryTokenizer_Numbers(t *testing.T) {
	config := &RuleBoundaryConfig{
		MinLength:     1,
		MaxLength:     1000,
		TrimSpace:     true,
		MergeNewlines: true,
		KeepEmpty:     false,
		EnableCJK:     true,
		EnableWestern: true,
		EnableNumbers: true,
	}

	tokenizer := NewRuleBoundaryTokenizer(config)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "简单数字",
			input: "价格是 123.45 元。下一句开始。",
			expected: []string{
				"价格是 123.45 元。",
				"下一句开始。",
			},
		},
		{
			name:  "多个数字",
			input: "第一个数字是 1.23，第二个数字是 4.56。",
			expected: []string{
				"第一个数字是 1.23，第二个数字是 4.56。",
			},
		},
		{
			name:  "带千位分隔符的数字",
			input: "总金额为 1,234,567.89 元。结束。",
			expected: []string{
				"总金额为 1,234,567.89 元。",
				"结束。",
			},
		},
		{
			name:  "科学计数法",
			input: "数值为 1.23e-4 单位。继续。",
			expected: []string{
				"数值为 1.23e-4 单位。",
				"继续。",
			},
		},
		{
			name:  "百分比",
			input: "增长率为 12.34%。下一句。",
			expected: []string{
				"增长率为 12.34%。",
				"下一句。",
			},
		},
		{
			name:  "混合数字和文本",
			input: "版本号 2.0.1 发布了。这是新版本。",
			expected: []string{
				"版本号 2.0.1 发布了。",
				"这是新版本。",
			},
		},
		{
			name:  "IP地址",
			input: "服务器IP是 192.168.1.1。继续。",
			expected: []string{
				"服务器IP是 192.168.1.1。",
				"继续。",
			},
		},
		{
			name:  "数字范围",
			input: "温度在 36.5-37.2 度之间。正常。",
			expected: []string{
				"温度在 36.5-37.2 度之间。",
				"正常。",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRuleBoundaryTokenizer_DisabledNumbers(t *testing.T) {
	config := &RuleBoundaryConfig{
		MinLength:     1,
		MaxLength:     1000,
		TrimSpace:     true,
		MergeNewlines: true,
		KeepEmpty:     false,
		EnableCJK:     true,
		EnableWestern: true,
		EnableNumbers: false, // 禁用数字处理
	}

	tokenizer := NewRuleBoundaryTokenizer(config)
	if err := tokenizer.Init(context.Background()); err != nil {
		t.Fatalf("Failed to init tokenizer: %v", err)
	}
	defer tokenizer.Close()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "禁用数字处理时的小数",
			input: "价格是 123.45 元。",
			expected: []string{
				"价格是 123.",
				"45 元。",
			},
		},
		{
			name:  "禁用数字处理时的IP地址",
			input: "IP是 192.168.1.1。",
			expected: []string{
				"IP是 192.",
				"168.",
				"1.",
				"1。",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizer.Feed(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Feed() = %v, want %v", got, tt.expected)
			}
		})
	}
}
