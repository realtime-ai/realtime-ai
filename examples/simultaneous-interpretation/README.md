# Real-time Simultaneous Interpretation (Modular Pipeline)

**高度可定制的模块化语音同传系统**

## 🎯 两种方案对比

| 特性 | **本方案 (模块化)** | [Gemini 方案](../simultaneous-interpretation-gemini/) |
|------|-------------------|------------------------|
| **架构** | 7 个独立模块 | 3 个模块 (Gemini 一体化) |
| **延迟** | 4-7 秒 | 1-2 秒 |
| **成本** | $0.022/分钟 | $0.014/分钟 |
| **可定制性** | ✅ **高** - 可换任意 STT/TTS | ⚠️ 低 - 仅限 Gemini |
| **Provider 选择** | ✅ OpenAI/Azure/自定义 | ⚠️ 仅 Google |
| **细粒度控制** | ✅ 每步骤可调 | ⚠️ 黑盒处理 |
| **适合场景** | 企业定制、合规要求 | 快速原型、低延迟需求 |

## ✅ 选择本方案当...

- 🏢 **需要特定 Provider** - 企业已有 Azure/AWS 合约
- 🔧 **需要细粒度控制** - 自定义每个处理步骤
- 📊 **需要中间结果** - 获取原文、译文、音频各阶段数据
- 🔒 **合规要求** - 必须使用特定云服务商
- 🎨 **自定义 TTS 声音** - 使用特定的语音合成服务
- 🧪 **研究/实验** - 测试不同 STT/翻译/TTS 组合

## 🏗️ 模块化架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     模块化同传 Pipeline                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  🎤 麦克风                                                       │
│      ↓                                                          │
│  [1] AudioResample ──────────────── 可换: 任意采样率转换        │
│      ↓                                                          │
│  [2] SileroVAD (可选) ───────────── 可换: WebRTC VAD, 自定义    │
│      ↓                                                          │
│  [3] WhisperSTT ─────────────────── 可换: Azure STT, 讯飞, 自定义│
│      ↓                                                          │
│  [4] TranslateElement ───────────── 可换: GPT, Gemini, DeepL    │
│      ↓                                                          │
│  [5] UniversalTTS ───────────────── 可换: Azure TTS, 讯飞, 自定义│
│      ↓                                                          │
│  [6] AudioResample                                              │
│      ↓                                                          │
│  [7] OpusEncode                                                 │
│      ↓                                                          │
│  🔊 扬声器                                                       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**优势**: 每个模块都可以独立替换，支持混合使用不同服务商

## 🚀 快速开始

### 1. 安装

```bash
cd examples/simultaneous-interpretation
go mod download
```

### 2. 配置

```bash
cp .env.example .env
```

编辑 `.env`:

```env
# 必需
OPENAI_API_KEY=sk-your-key

# 语言设置
SOURCE_LANG=zh          # 源语言
TARGET_LANG=en          # 目标语言

# 翻译 Provider (openai 或 gemini)
TRANSLATE_PROVIDER=openai
TRANSLATE_MODEL=gpt-4o-mini

# TTS 设置
TTS_VOICE=alloy         # alloy, echo, fable, onyx, nova, shimmer
TTS_SPEED=1.0           # 0.25-4.0

# 可选: Gemini 翻译 (需要 GOOGLE_API_KEY)
# TRANSLATE_PROVIDER=gemini
# GOOGLE_API_KEY=your-google-key
```

### 3. 运行

```bash
# 标准模式
go run main.go

# 带 VAD 支持 (推荐)
go build -tags vad -o interpretation && ./interpretation
```

打开 http://localhost:8080

## 🔧 定制示例

### 示例 1: 使用 Azure STT + OpenAI TTS

```go
// 替换 Whisper 为 Azure STT
azureSTT := elements.NewAzureSTTElement(azureConfig)

// 保持 OpenAI TTS
tts := elements.NewUniversalTTSElement(openaiProvider)
```

### 示例 2: 使用 DeepL 翻译

```go
// 自定义翻译 Provider
translateConfig := elements.TranslateConfig{
    Provider:   "deepl",
    APIKey:     os.Getenv("DEEPL_API_KEY"),
    SourceLang: "ZH",
    TargetLang: "EN",
}
```

### 示例 3: 获取中间结果

```go
// 订阅原文 (STT 输出)
bus.Subscribe(pipeline.EventFinalResult, func(e pipeline.Event) {
    originalText := e.Payload.(string)
    log.Printf("原文: %s", originalText)
})

// 订阅译文 (翻译输出)
bus.Subscribe(pipeline.EventTranslationResult, func(e pipeline.Event) {
    translatedText := e.Payload.(string)
    log.Printf("译文: %s", translatedText)
})
```

## 📊 性能特点

| 指标 | 本方案 | 说明 |
|------|--------|------|
| **延迟** | 4-7 秒 | STT (2-3s) + 翻译 (1-2s) + TTS (1-2s) |
| **成本** | $0.022/分钟 | Whisper + GPT + TTS 总计 |
| **可用性** | 99.9% | 多 Provider 可做故障转移 |
| **定制性** | ⭐⭐⭐⭐⭐ | 完全可控 |

## 🆚 何时选择 Gemini 方案

如果你：
- ✅ 追求最低延迟 (1-2 秒)
- ✅ 追求最低成本
- ✅ 不需要特定 Provider
- ✅ 快速原型开发

👉 使用 [simultaneous-interpretation-gemini](../simultaneous-interpretation-gemini/)

## 📚 进阶文档

- [COMPARISON.md](../simultaneous-interpretation-gemini/COMPARISON.md) - 详细对比
- [pkg/asr/README.md](../../pkg/asr/README.md) - ASR 接口文档
- [pkg/tts/README.md](../../pkg/tts/README.md) - TTS 接口文档

## 🔧 故障排除

### 延迟过高
- 启用 VAD 减少无效 API 调用
- 使用更快的翻译模型 (gpt-4o-mini)
- 检查网络延迟

### 音频卡顿
- 添加 AudioPacer 元素平滑输出
- 检查 WebRTC 连接质量

### 翻译质量差
- 调整翻译 prompt
- 尝试不同模型
- 检查语言代码是否正确

## 📄 License

See main repository for license information.
