# Qwen Realtime STT Example

This example demonstrates real-time speech-to-text using Alibaba Cloud DashScope Qwen Realtime ASR with optional VAD (Voice Activity Detection) integration.

## Features

- **True Streaming ASR**: Uses WebSocket for real-time audio streaming (similar to OpenAI Realtime API)
- **Partial Results**: Get interim transcription results as you speak
- **Final Results**: Get complete transcription when speech ends
- **VAD Integration**: Optional Silero VAD for optimized recognition
- **Multiple Languages**: Supports Chinese (zh), English (en), Japanese (ja), Korean (ko), Cantonese (yue)

## Prerequisites

1. **DashScope API Key**: Get your API key from [Alibaba Cloud DashScope](https://dashscope.console.aliyun.com/)

2. **Set environment variable**:
   ```bash
   export DASHSCOPE_API_KEY=your_api_key_here
   ```

3. **Optional: Set language** (defaults to Chinese):
   ```bash
   export QWEN_ASR_LANGUAGE=zh  # Chinese (default)
   # or
   export QWEN_ASR_LANGUAGE=en  # English
   export QWEN_ASR_LANGUAGE=ja  # Japanese
   export QWEN_ASR_LANGUAGE=ko  # Korean
   export QWEN_ASR_LANGUAGE=yue # Cantonese
   ```

## Running the Example

### WebRTC Server Mode (Browser)

```bash
go run examples/qwen-realtime-stt/main.go
```

### Audio File Mode (CLI)

This mode uses the provided `test.m4a` for testing:

```bash
go run examples/qwen-realtime-stt/test_audio/main.go
```

### With VAD (Requires ONNX Runtime)

```bash
# First, set up ONNX Runtime (see pkg/elements/VAD_README.md)
go run -tags vad examples/qwen-realtime-stt/main.go
```

## How It Works

### Pipeline Architecture

```
┌──────────────────┐
│  WebRTC Audio    │
│  (Browser)       │
└────────┬─────────┘
         │
         ▼
┌──────────────────────┐
│ AudioResampleElement │
│  → 16kHz, mono       │
└────────┬─────────────┘
         │
         ▼
┌──────────────────────┐
│  SileroVADElement    │  (Optional)
│  Mode: Passthrough   │
│  Events: ─────────┐  │
└────────┬──────────┘  │
         │             │
         ▼             │ EventVADSpeechStart
┌──────────────────────┼───────┐
│ QwenRealtimeSTTElement       │
│  - WebSocket streaming       │
│  - Partial results: ────────►│── Partial text
│  - Final results: ─────────►│── Final text
└────────┬─────────────────────┘
         │
         ▼
┌──────────────────────────────┐
│  TextData Output             │
│  → Browser / Further         │
│    Processing                │
└──────────────────────────────┘
```

### Event Flow

1. **Without VAD**:
   - Audio is streamed continuously to Qwen Realtime
   - Partial results are emitted in real-time
   - Manual commit triggers final transcription

2. **With VAD**:
   - VAD detects speech start → Audio streaming begins
   - Partial results are emitted during speech
   - VAD detects speech end → Commits audio buffer
   - Final transcription is returned

## Qwen vs Whisper

| Feature | Qwen Realtime | OpenAI Whisper |
|---------|---------------|----------------|
| Streaming | True WebSocket streaming | Buffered (simulated) |
| Latency | Lower (real-time) | Higher (batch processing) |
| Partial Results | Native support | Limited |
| Languages | zh, en, ja, ko, yue | 99+ languages |
| API Style | OpenAI Realtime-like | REST API |

## Output Example

```
=== Qwen Realtime STT Example with VAD Integration ===
This example demonstrates:
  - True streaming Speech-to-Text using Alibaba Cloud DashScope Qwen ASR
  - Real-time partial and final transcription results
  - Voice Activity Detection (VAD) using Silero
  - Real-time audio processing pipeline

[VAD] Speech detected - streaming audio...
[Partial] 你
[Partial] 你好
[Partial] 你好世
[Partial] 你好世界
[VAD] Speech ended - committing for final result...
[Final] 你好世界
[Output] Final transcription: 你好世界
```

## Troubleshooting

### Connection Issues

If you see connection errors, check:
1. Your `DASHSCOPE_API_KEY` is valid
2. You have network access to `dashscope.aliyuncs.com`
3. The WebSocket endpoint is reachable

### No Transcription Results

1. Ensure audio is in the correct format (16kHz, mono, PCM)
2. Check that the language setting matches your speech
3. Try speaking more clearly or closer to the microphone

### VAD Not Working

1. Ensure you built with `-tags vad`
2. Check that `models/silero_vad.onnx` exists
3. Verify ONNX Runtime is properly installed

## Related Examples

- `examples/whisper-stt/`: OpenAI Whisper STT example
- `examples/translation-demo/`: Transcription + Translation
- `examples/simultaneous-interpretation/`: Voice-to-voice interpretation
