# gRPC Gemini Assistant Example

This example demonstrates how to use the gRPC-based Realtime AI framework with Google Gemini.

## Architecture

```
Client (Go gRPC Client)
    ↓ gRPC Bidirectional Streaming
Server (GRPCServer)
    ↓ Pipeline
AudioResampleElement → GeminiElement → PlayoutSinkElement
    ↓
Google Gemini API
```

## Prerequisites

1. **Go 1.23.4+**
2. **Google API Key** - Get it from [Google AI Studio](https://makersuite.google.com/app/apikey)
3. **System dependencies**:
   ```bash
   # macOS
   brew install opus ffmpeg
   ```

## Setup

1. Set your Google API key:
   ```bash
   export GOOGLE_API_KEY="your_api_key_here"
   ```

   Or create a `.env` file in the project root:
   ```
   GOOGLE_API_KEY=your_api_key_here
   ```

2. Install Go dependencies:
   ```bash
   cd /path/to/realtime-ai
   go mod download
   ```

## Running the Example

### Option 1: Run Server and Client Separately

**Terminal 1 - Start the server:**
```bash
go run examples/grpc-assis/server.go
```

Output:
```
[Main] Starting gRPC Gemini Assistant Server on port 50051
[GRPCServer] Starting gRPC server on port 50051
```

**Terminal 2 - Run the client:**
```bash
go run examples/grpc-assis/client.go client
```

Output:
```
[Client] Connected to server: localhost:50051
[Client] Sent audio frame #1
[Client] Sent audio frame #2
[Client] Received audio: 1920 bytes, 48000 Hz, 1 channels
...
```

### Option 2: Build and Run

Build the binaries:
```bash
# Build server
go build -o bin/grpc-server examples/grpc-assis/server.go

# Build client
go build -o bin/grpc-client examples/grpc-assis/client.go
```

Run:
```bash
# Terminal 1
./bin/grpc-server

# Terminal 2
./bin/grpc-client client
```

## What the Example Does

1. **Server**:
   - Starts a gRPC server on port 50051
   - Waits for client connections
   - On connection, creates a pipeline: Resample → Gemini → Playout
   - Processes incoming audio/text and sends responses back

2. **Client**:
   - Connects to the gRPC server
   - Sends 5 test audio frames (dummy PCM data)
   - Sends a text message: "Hello, Gemini!"
   - Receives and logs responses from the server

## Message Flow

```
1. Client connects to server
   → gRPC bidirectional stream established

2. Client sends audio frame
   → StreamMessage(type=AUDIO, AudioFrame{48kHz, mono, PCM})
   → Server: AudioResampleElement (48kHz → 16kHz)
   → Server: GeminiElement (AI processing)
   → Server: PlayoutSinkElement
   → StreamMessage response back to client

3. Client sends text message
   → StreamMessage(type=TEXT, TextMessage{"Hello, Gemini!"})
   → Server: Gemini processes text
   → StreamMessage response back to client
```

## Protobuf Message Structure

The gRPC API uses the following message types (defined in `pkg/proto/streamingai/v1/streaming_ai.proto`):

- **StreamMessage**: Main container for all messages
  - `MessageType`: AUDIO, VIDEO, TEXT, CONTROL
  - `Payload`: AudioFrame, VideoFrame, TextMessage, ControlMessage

- **AudioFrame**: Audio data with metadata
  - `data`: Raw audio bytes (PCM or encoded)
  - `sample_rate`: 16000, 48000, etc.
  - `channels`: 1 (mono), 2 (stereo)
  - `media_type`: "audio/x-raw", "audio/opus"

- **TextMessage**: Text data
  - `data`: Text content as bytes
  - `text_type`: "plain", "json", "markdown"

## Customization

### Change the Pipeline

Edit `server.go`, modify the `OnConnectionStateChange` handler:

```go
// Example: Add VAD (Voice Activity Detection)
vadElement := elements.NewVADElement()
audioResampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)
geminiElement := elements.NewGeminiElement()
playoutSinkElement := elements.NewPlayoutSinkElement()

elements := []pipeline.Element{
    audioResampleElement,
    vadElement,           // Add VAD
    geminiElement,
    playoutSinkElement,
}

pipeline.Link(audioResampleElement, vadElement)
pipeline.Link(vadElement, geminiElement)
pipeline.Link(geminiElement, playoutSinkElement)
```

### Change the Server Port

Edit `server.go`:
```go
func main() {
    StartGRPCServer(9090) // Change from 50051 to 9090
}
```

And update client:
```go
serverAddr := "localhost:9090"
```

## Comparison: gRPC vs WebRTC

| Feature | gRPC (This Example) | WebRTC (gemini-assis) |
|---------|---------------------|------------------------|
| **Protocol** | HTTP/2 over TCP | UDP (RTP) + SCTP (DataChannel) |
| **Client** | Go gRPC client | Web browser (JavaScript) |
| **Setup** | Simple (direct connection) | Complex (SDP negotiation, ICE) |
| **NAT Traversal** | Not needed (TCP) | STUN/TURN required |
| **Latency** | Slightly higher (TCP) | Lower (UDP, no retransmission) |
| **Use Case** | Server-to-server, mobile apps | Browser real-time communication |

## Troubleshooting

### Error: "failed to connect"
- Ensure the server is running first
- Check firewall settings for port 50051

### Error: "GOOGLE_API_KEY not set"
- Set the environment variable or create `.env` file

### Audio Processing Issues
- Ensure `opus` and `ffmpeg` are installed
- Check audio format (should be PCM, 48kHz, mono)

## Next Steps

- Implement a real audio capture client (using microphone)
- Add video support (VideoFrame messages)
- Implement authentication and TLS
- Add session management (CreateSession/CloseSession RPCs)
- Deploy to production with proper gRPC load balancing

## Resources

- [gRPC Documentation](https://grpc.io/docs/languages/go/)
- [Protocol Buffers Guide](https://protobuf.dev/getting-started/gotutorial/)
- [Google Gemini API](https://ai.google.dev/docs)
- [Realtime AI Framework Documentation](../../CLAUDE.md)
