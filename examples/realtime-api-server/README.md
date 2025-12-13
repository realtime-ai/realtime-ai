# Realtime API Server Example

This example demonstrates how to create a Realtime API server similar to OpenAI's Realtime API using the realtime-ai framework.

## Features

- WebSocket-based bidirectional communication
- JSON event protocol compatible with OpenAI Realtime API
- Server-side VAD (Voice Activity Detection)
- Streaming audio and text responses
- Session management
- Multiple AI model support (Gemini, OpenAI)

## Quick Start

### 1. Set up API key

```bash
export GOOGLE_API_KEY=your_api_key_here
```

### 2. Run the server

```bash
go run examples/realtime-api-server/main.go
```

### 3. Connect via WebSocket

Using wscat:
```bash
npm install -g wscat
wscat -c "ws://localhost:8080/v1/realtime?model=gemini-2.0-flash"
```

Or with authentication:
```bash
export API_TOKEN=your_secret_token
wscat -c "ws://localhost:8080/v1/realtime" -H "Authorization: Bearer your_secret_token"
```

## Protocol

### Client Events

#### session.update
Update session configuration:
```json
{
    "type": "session.update",
    "session": {
        "modalities": ["text", "audio"],
        "voice": "alloy",
        "turn_detection": {
            "type": "server_vad",
            "threshold": 0.5,
            "silence_duration_ms": 500
        }
    }
}
```

#### input_audio_buffer.append
Send audio data (base64 encoded PCM16, 24kHz, mono):
```json
{
    "type": "input_audio_buffer.append",
    "audio": "BASE64_ENCODED_AUDIO"
}
```

#### input_audio_buffer.commit
Commit the audio buffer:
```json
{
    "type": "input_audio_buffer.commit"
}
```

#### response.create
Trigger AI response:
```json
{
    "type": "response.create"
}
```

### Server Events

#### session.created
Sent when connection is established:
```json
{
    "type": "session.created",
    "event_id": "evt_abc123",
    "session": {
        "id": "sess_001",
        "model": "gemini-2.0-flash",
        "modalities": ["text", "audio"]
    }
}
```

#### response.audio.delta
Audio response chunk:
```json
{
    "type": "response.audio.delta",
    "event_id": "evt_audio_001",
    "response_id": "resp_001",
    "item_id": "item_001",
    "delta": "BASE64_ENCODED_AUDIO"
}
```

#### response.done
Response completed:
```json
{
    "type": "response.done",
    "event_id": "evt_done_001",
    "response": {
        "id": "resp_001",
        "status": "completed"
    }
}
```

## JavaScript Client Example

```javascript
const ws = new WebSocket('ws://localhost:8080/v1/realtime?model=gemini-2.0-flash');

ws.onopen = () => {
    console.log('Connected');

    // Configure session
    ws.send(JSON.stringify({
        type: 'session.update',
        session: {
            modalities: ['text', 'audio'],
            voice: 'alloy'
        }
    }));
};

ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log('Event:', data.type);

    switch (data.type) {
        case 'session.created':
            console.log('Session ID:', data.session.id);
            break;
        case 'response.audio.delta':
            // Play audio (data.delta is base64 encoded)
            playAudio(data.delta);
            break;
        case 'error':
            console.error('Error:', data.error.message);
            break;
    }
};

// Send audio (must be PCM16, 24kHz, mono)
function sendAudio(pcmData) {
    const base64 = btoa(String.fromCharCode(...new Uint8Array(pcmData)));
    ws.send(JSON.stringify({
        type: 'input_audio_buffer.append',
        audio: base64
    }));
}
```

## Configuration

### Server Options

| Option | Default | Description |
|--------|---------|-------------|
| `Addr` | `:8080` | Server address |
| `Path` | `/v1/realtime` | WebSocket endpoint path |
| `AuthToken` | `""` | Bearer token for authentication |
| `DefaultModel` | `gemini-2.0-flash` | Default AI model |
| `MaxSessionsPerIP` | `10` | Max sessions per IP |
| `SessionTimeout` | `30m` | Session timeout |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `GOOGLE_API_KEY` | Google AI API key (required for Gemini) |
| `OPENAI_API_KEY` | OpenAI API key (for OpenAI models) |
| `API_TOKEN` | Authentication token (optional) |

## Custom Pipeline

You can customize the AI pipeline by providing your own `PipelineFactory`:

```go
func myCustomPipeline(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
    p := pipeline.NewPipeline("custom-" + session.ID)

    // Add your custom elements
    resample := elements.NewAudioResampleElement(24000, 16000, 1, 1)
    whisper := elements.NewWhisperSTTElement(...)
    translate := elements.NewTranslateElement(...)
    tts := elements.NewUniversalTTSElement(...)

    p.AddElements([]pipeline.Element{resample, whisper, translate, tts})
    p.Link(resample, whisper)
    p.Link(whisper, translate)
    p.Link(translate, tts)

    return p, nil
}

server.SetPipelineFactory(myCustomPipeline)
```
