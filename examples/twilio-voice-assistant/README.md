# Twilio Voice Assistant

A complete phone customer service application using Twilio Media Streams.

## Architecture

```
Twilio Phone Call
       ↓
TwiML <Connect><Stream>
       ↓
┌──────────────────────────────────────────────────────────────┐
│              Twilio WebSocket Server                         │
│  (receives μ-law 8kHz → sends μ-law 8kHz)                    │
└──────────────────────────────────────────────────────────────┘
       ↓                                      ↑
    mulaw→PCM                              PCM→mulaw
    8kHz→16kHz                            16kHz→8kHz
       ↓                                      ↑
┌──────────────────────────────────────────────────────────────┐
│                     Pipeline                                  │
│                                                              │
│  Audio → VAD → STT → GPT → TTS → Output                     │
│  (16kHz)       (ElevenLabs)  (ElevenLabs)                    │
└──────────────────────────────────────────────────────────────┘
```

## Components

| Component | Technology | Latency |
|-----------|------------|---------|
| VAD | Silero VAD | ~30ms |
| STT | ElevenLabs Scribe V2 Realtime | ~150ms |
| LLM | OpenAI GPT-4o-mini | ~500ms |
| TTS | ElevenLabs Turbo V2.5 | ~200ms |

**Total estimated latency: ~900ms** (end-to-end)

## Quick Start

### 1. Prerequisites

- Go 1.21+
- Twilio account with a phone number
- OpenAI API key
- ElevenLabs API key
- Public URL (ngrok or similar for development)

### 2. Setup

```bash
# Clone and navigate to example
cd examples/twilio-voice-assistant

# Copy environment file
cp .env.example .env

# Edit .env with your API keys
```

### 3. Download VAD Model

```bash
mkdir -p models
wget https://github.com/snakers4/silero-vad/raw/master/src/silero_vad/data/silero_vad.onnx -O models/silero_vad.onnx
```

### 4. Start the Server

```bash
# From project root
go run examples/twilio-voice-assistant/main.go
```

### 5. Expose with ngrok (development)

```bash
ngrok http 8080
```

Note the HTTPS URL (e.g., `https://abc123.ngrok.io`)

### 6. Configure Twilio

1. Go to Twilio Console → Phone Numbers → Your Number
2. Under "Voice & Fax", set:
   - **A Call Comes In**: Webhook
   - **URL**: `https://abc123.ngrok.io/twiml`
   - **Method**: POST

3. Update your `.env`:
   ```
   TWILIO_STREAM_URL=wss://abc123.ngrok.io/media
   ```

4. Restart the server

### 7. Make a Call

Call your Twilio phone number and start talking!

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | 8080 |
| `TWILIO_STREAM_URL` | Public WebSocket URL | (required) |
| `OPENAI_API_KEY` | OpenAI API key | (required) |
| `ELEVENLABS_API_KEY` | ElevenLabs API key | (required) |
| `ELEVENLABS_VOICE_ID` | Voice to use | Rachel |
| `VAD_ENABLED` | Enable voice activity detection | true |
| `VAD_MODEL_PATH` | Path to Silero VAD model | models/silero_vad.onnx |
| `SYSTEM_PROMPT` | LLM system prompt | (default prompt) |

## Production Deployment

For production, consider:

1. **HTTPS**: Use a proper TLS certificate
2. **Health Checks**: Monitor `/health` endpoint
3. **Scaling**: Deploy behind a load balancer
4. **Monitoring**: Add metrics and logging
5. **Fallback**: Handle API errors gracefully

## Troubleshooting

### No audio received
- Check Twilio console for WebSocket errors
- Verify ngrok is running and URL is correct
- Check firewall allows WebSocket connections

### High latency
- Use a server closer to your callers
- Consider using a faster LLM model
- Reduce TTS quality settings

### STT errors
- Verify ElevenLabs API key is valid
- Check audio sample rate is 16kHz
- Review VAD settings if speech is being cut off

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/twiml` | POST | TwiML webhook for incoming calls |
| `/media` | WS | WebSocket for Twilio Media Streams |
| `/health` | GET | Health check with session count |

## References

- [Twilio Media Streams](https://www.twilio.com/docs/voice/media-streams)
- [ElevenLabs Scribe](https://elevenlabs.io/docs/api-reference/speech-to-text)
- [OpenAI Chat API](https://platform.openai.com/docs/guides/chat)
