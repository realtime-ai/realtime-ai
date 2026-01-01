# Real-time Simultaneous Interpretation (Gemini Live)

**超低延迟一体化语音同传 - 基于 Gemini Live API**

## 🎯 两种方案对比

| 特性 | **本方案 (Gemini)** | [模块化方案](../simultaneous-interpretation/) |
|------|-------------------|------------------------|
| **延迟** | 1-2 秒 ✅ | 4-7 秒 |
| **成本** | $0.014/分钟 ✅ | $0.022/分钟 |
| **架构** | 3 个模块 (一体化) | 7 个独立模块 |
| **定制性** | 低 - 仅限 Gemini | ✅ **高** - 可换任意 Provider |
| **适合场景** | 快速原型、低延迟需求 | 企业定制、合规要求 |

## 🚀 架构对比

### 模块化方案 (examples/simultaneous-interpretation)
```
🎤 → Resample → VAD → Whisper (3s) → 翻译 (2s) → TTS (2s) → Resample → Opus → 🔊
共 7 个模块, 延迟 4-7s, 成本 $0.022/min
优势: 每个模块可独立替换
```

### Gemini 方案 (本示例)
```
🎤 → Resample → [Gemini Live: 语音理解+翻译+语音合成] → Resample → 🔊
共 3 个模块, 延迟 1-2s, 成本 $0.014/min
优势: 一体化处理，延迟最低
```

**Gemini Live 优势:**
- ✅ 单一 API 调用替代 3 个独立 API
- ✅ 原生音频理解 (无需转文字)
- ✅ 内置 VAD 和自然语音合成
- ✅ 专为低延迟优化
- ✅ 代码和配置更简单

## 📋 Quick Start

### 1. Prerequisites

- Go 1.21+
- Google API Key ([Get one here](https://makersuite.google.com/app/apikey))
- Chrome/Firefox browser
- Microphone + headphones

### 2. Installation

```bash
cd examples/simultaneous-interpretation-gemini

# Copy and edit configuration
cp .env.example .env
# Add your GOOGLE_API_KEY to .env

# Install dependencies
go mod download
```

### 3. Run

```bash
go run main.go
```

Then open http://localhost:8080 in your browser.

## ⚙️ Configuration

Edit `.env`:

```env
# Required
GOOGLE_API_KEY=your-key-here

# Language pair
SOURCE_LANG=Chinese        # What you speak
TARGET_LANG=English        # What you want to hear

# Domain (affects tone and terminology)
INTERPRETATION_DOMAIN=casual    # Options: casual, business, technical, medical, legal

# Model (default is recommended)
GEMINI_MODEL=gemini-2.5-flash-native-audio-preview-12-2025
```

### Domain Examples

**Casual Conversation** (default)
```env
INTERPRETATION_DOMAIN=casual
SOURCE_LANG=Chinese
TARGET_LANG=English
```
Natural, relaxed tone for everyday chat.

**Business Meeting**
```env
INTERPRETATION_DOMAIN=business
SOURCE_LANG=Japanese
TARGET_LANG=English
```
Formal language, professional terminology preserved.

**Technical Discussion**
```env
INTERPRETATION_DOMAIN=technical
SOURCE_LANG=English
TARGET_LANG=Spanish
```
Technical terms preserved, precise translations.

**Medical Consultation**
```env
INTERPRETATION_DOMAIN=medical
SOURCE_LANG=Korean
TARGET_LANG=English
```
Medical terminology, high precision, compassionate tone.

## 💰 Cost Comparison

### Per minute of interpretation:

| Component | Traditional | Realtime API | Savings |
|-----------|------------|--------------|---------|
| STT (Whisper) | $0.006 | — | — |
| Translation (GPT) | $0.001 | — | — |
| TTS (OpenAI) | $0.015 | — | — |
| **Gemini Live** | — | $0.014 | — |
| **Total** | **$0.022** | **$0.014** | **36%** |

**Example costs:**
- 10 minutes: $0.14 (vs $0.22) - save $0.08
- 1 hour: $0.84 (vs $1.32) - save $0.48
- Daily meeting (1h): $0.84/day vs $1.32/day

## 🎨 Supported Languages

99+ languages supported, including:

| Language | Code |
|----------|------|
| Chinese (Mandarin) | Chinese |
| English | English |
| Japanese | Japanese |
| Korean | Korean |
| Spanish | Spanish |
| French | French |
| German | German |
| Russian | Russian |
| Arabic | Arabic |
| Portuguese | Portuguese |
| Italian | Italian |
| Hindi | Hindi |

Use the full language name (not code) in configuration.

## 🔧 How It Works

### System Instruction

Gemini Live is configured with a carefully crafted system instruction that makes it behave as a simultaneous interpreter:

```
You are a professional simultaneous interpreter.

TASK: Listen to Chinese and immediately speak the translation in English.

RULES:
1. Translate ONLY - no commentary
2. Speak naturally as if you are the speaker
3. Start immediately when you understand
4. Preserve emotion and tone
5. Be concise but complete
6. Minimize latency - speed is critical

CONTEXT: Casual Conversation
- Natural, relaxed language
- Conversational tone
- Include colloquialisms

Start interpreting now. Speak ONLY the translation.
```

The instruction is sent via Realtime API's `session.update` event.

### Pipeline Flow

1. **Browser captures audio** (48kHz Opus via WebRTC)
2. **Resample to 16kHz** (Gemini's input format)
3. **Gemini Live processes**:
   - Understands speech (built-in STT)
   - Translates to target language
   - Synthesizes natural speech (built-in TTS)
4. **Resample to 48kHz** (WebRTC output format)
5. **Browser plays audio**

Total latency: **1-2 seconds** 🚀

## 🆚 When to Use Which Version?

### Use **Realtime API** (this version) when:
- ✅ You need lowest possible latency (1-2s)
- ✅ You want simplest implementation
- ✅ You want lower costs
- ✅ Your language pair is supported by Gemini
- ✅ You don't need custom STT/TTS providers

### Use **Traditional Pipeline** when:
- ⚠️ You need OpenAI-specific features
- ⚠️ You need very specific control over each step
- ⚠️ You want to use different providers for STT/TTS
- ⚠️ Language pair not well supported by Gemini

## 🎬 Demo Use Cases

### International Business Meeting
```env
SOURCE_LANG=Chinese
TARGET_LANG=English
INTERPRETATION_DOMAIN=business
```
Formal, professional interpretation for cross-border meetings.

### Language Learning
```env
SOURCE_LANG=English
TARGET_LANG=Japanese
INTERPRETATION_DOMAIN=casual
```
Practice speaking, hear natural pronunciation instantly.

### Customer Support
```env
SOURCE_LANG=Spanish
TARGET_LANG=English
INTERPRETATION_DOMAIN=business
```
Provide multilingual support in real-time.

### Technical Collaboration
```env
SOURCE_LANG=English
TARGET_LANG=German
INTERPRETATION_DOMAIN=technical
```
Engineering discussions with preserved technical terms.

## 📊 Performance Tips

### For Best Latency:
1. Use stable, fast internet connection
2. Close unnecessary browser tabs
3. Use headphones (prevents echo/feedback)
4. Speak clearly at moderate pace
5. Pause between thoughts

### For Best Quality:
1. Use quality microphone
2. Minimize background noise
3. Choose appropriate domain setting
4. Use headphones
5. Test audio levels before important calls

## 🔍 Troubleshooting

### "Connection failed"
- Check GOOGLE_API_KEY is correct
- Verify internet connection
- Try refreshing the page

### "High latency / Slow interpretation"
- Check internet speed
- Reduce background noise
- Speak more clearly
- Try different domain setting

### "Audio choppy or cutting out"
- Use headphones to prevent echo
- Check microphone permissions
- Refresh the page
- Try Chrome browser

### "No translation happening"
- Verify language pair in .env
- Check browser console for errors
- Ensure you're speaking clearly
- Wait 1-2 seconds after speaking

## 📚 Technical Details

### API Used
- **Gemini Live API**: `gemini-2.5-flash-native-audio-preview-12-2025`
- **Protocol**: WebRTC + Realtime API JSON events
- **Audio Format**: 16kHz PCM (input), 24kHz PCM (output)
- **Transport**: WebRTC DataChannel for signaling, RTP for audio

### Source Code
- `main.go`: Server and pipeline implementation
- `static/index.html`: Web UI
- `.env`: Configuration

### Dependencies
- `github.com/realtime-ai/realtime-ai`: Realtime AI framework
- `google.golang.org/genai`: Google Gemini SDK
- WebRTC for real-time audio streaming

## 🙏 Credits

Built with:
- [Realtime AI Framework](https://github.com/realtime-ai/realtime-ai)
- [Google Gemini Live API](https://ai.google.dev/gemini-api/docs/live)
- WebRTC

## 📄 License

See main repository for license information.

---

**Enjoy real-time interpretation with 70% lower latency! 🚀🌐**

## 📖 Further Reading

For more details on Gemini Live API:
- [Get started with Live API](https://ai.google.dev/gemini-api/docs/live)
- [Live API capabilities guide](https://ai.google.dev/gemini-api/docs/live-guide)
- [Live API reference](https://ai.google.dev/api/live)
