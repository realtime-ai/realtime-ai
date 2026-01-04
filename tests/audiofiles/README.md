# Test Audio Files

This directory contains audio files used for integration tests across the project.

## Files

| File | Format | Description |
|------|--------|-------------|
| `vad_test_en.wav` | 16kHz mono, 60s | English speech sample for VAD, ASR, and interpretation tests |
| `test_speech.wav` | 16kHz mono | Short speech sample for quick tests |

## Usage

These files are referenced by various integration tests:

- `tests/vad/vad_run.go` - VAD element testing
- `tests/vad/vad_analyze.go` - VAD visualization analysis
- `tests/elevenlabs/elevenlabs_run.go` - ElevenLabs ASR testing
- `tests/interpretation/interpretation_run.go` - E2E interpretation pipeline testing

## Adding New Audio Files

When adding new test audio files:

1. Ensure files are in WAV format, 16kHz sample rate, mono channel
2. Keep files reasonably sized (< 10MB preferred)
3. Update this README with file descriptions
4. Update `.gitignore` if files should not be committed (e.g., large files)
