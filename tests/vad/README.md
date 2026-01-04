# VAD (Voice Activity Detection) Testing

This directory contains tests for the Silero VAD element, which detects speech segments in audio streams.

## Prerequisites

### ONNX Runtime v1.20.1

```bash
# Ubuntu/Debian
wget https://github.com/microsoft/onnxruntime/releases/download/v1.20.1/onnxruntime-linux-x64-1.20.1.tgz
tar -xzf onnxruntime-linux-x64-1.20.1.tgz
export ONNXRUNTIME_LIB=$(pwd)/onnxruntime-linux-x64-1.20.1/lib/libonnxruntime.so
export LD_LIBRARY_PATH=$(pwd)/onnxruntime-linux-x64-1.20.1/lib:$LD_LIBRARY_PATH

# macOS
brew install onnxruntime
# Library is auto-detected at /opt/homebrew/lib/libonnxruntime.dylib
```

## Quick Start

### Run Simple Test

```bash
# From project root
go run -tags vad ./tests/vad/simple/main.go
```

### Run VAD Analysis

```bash
# From project root
go run -tags vad ./tests/vad/analyze/main.go -audio=tests/audiofiles/vad_test_en.wav -model=models/silero_vad.onnx
```

### Run Unit Tests

```bash
# Run pkg/vad unit tests
go test -tags vad -v ./pkg/vad/
```

## VAD Visualization & Analysis

To debug and verify VAD detection accuracy, you can visualize the audio waveform alongside VAD speech probability.

### Step 1: Generate VAD Analysis Data

```bash
cd tests/vad

# Run the analyzer with default settings (uses ../audiofiles/vad_test_en.wav)
go run -tags vad vad_analyze.go

# Or specify a different audio file
go run -tags vad vad_analyze.go my_audio.wav

# Full command-line options:
go run -tags vad vad_analyze.go -audio=my_audio.wav -threshold=0.6 -output=result.json

# Available flags:
#   -audio     Path to the audio file (default: ../audiofiles/vad_test_en.wav)
#   -model     Path to Silero VAD ONNX model (default: ../../models/silero_vad.onnx)
#   -threshold VAD speech detection threshold 0.0-1.0 (default: 0.5)
#   -output    Output JSON file path (default: <audio_name>_vad.json)
```

### Step 2: Visualize the Results

```bash
# Install Python dependencies (if not already installed)
pip install -r requirements.txt
# or: pip install numpy matplotlib

# Run visualization (use the JSON output from step 1)
python3 visualize_vad.py ../audiofiles/vad_test_en.wav vad_test_en_vad.json

# Or specify a custom output image filename
python3 visualize_vad.py ../audiofiles/vad_test_en.wav vad_test_en_vad.json my_analysis.png
```

### What the Visualization Shows

The output image contains 4 plots:

1. **Audio Waveform** - Raw audio amplitude over time, with speech regions highlighted in green
2. **VAD Speech Probability** - The probability (0-1) that each frame contains speech, with threshold line
3. **Audio Energy (RMS)** - Root mean square energy per VAD frame
4. **Combined View** - Overlays waveform and probability for direct comparison

### Using Your Own Audio Files

```bash
# 1. Place your audio file in tests/audiofiles/ (or use absolute path)
cp /path/to/your/audio.wav tests/audiofiles/

# 2. Run the analysis with your file
go run -tags vad vad_analyze.go audio.wav

# 3. Visualize (output JSON is automatically named audio_vad.json)
python3 visualize_vad.py audio.wav audio_vad.json

# Tip: Adjust threshold for different audio characteristics
go run -tags vad vad_analyze.go -audio=noisy_audio.wav -threshold=0.7
```

### JSON Output Format

The `vad_analysis.json` file contains:

```json
{
  "audio_file": "../audiofiles/vad_test_en.wav",
  "sample_rate": 16000,
  "window_size": 512,
  "threshold": 0.5,
  "duration_ms": 60000,
  "total_frames": 1875,
  "speech_frames": 934,
  "data_points": [
    {
      "time": 0.0,
      "probability": 0.12,
      "audio_rms": 0.015,
      "is_speech": false
    },
    {
      "time": 0.032,
      "probability": 0.87,
      "audio_rms": 0.234,
      "is_speech": true
    }
  ]
}
```