#!/usr/bin/env python3
"""
VAD Visualization Tool

Visualizes audio waveform alongside VAD speech probability to help
evaluate VAD detection accuracy.

Usage:
    python3 visualize_vad.py <audio_file> <vad_json_file> [output_image]

Example:
    python3 visualize_vad.py vad_test_en.wav vad_analysis.json vad_result.png

Requirements:
    pip install numpy matplotlib

Optional (for better audio loading):
    pip install librosa
    # or just use wave module (built-in, supports WAV only)
"""

import json
import sys
import wave
import struct
from pathlib import Path

try:
    import numpy as np
    # Use non-interactive backend for saving files without display
    import matplotlib
    matplotlib.use('Agg')
    import matplotlib.pyplot as plt
    from matplotlib.patches import Patch
except ImportError:
    print("Error: Required packages not found.")
    print("Install with: pip install numpy matplotlib")
    sys.exit(1)


def load_wav_audio(audio_path: str) -> tuple[np.ndarray, int]:
    """Load WAV audio file and return normalized samples and sample rate."""
    with wave.open(audio_path, 'rb') as wf:
        sample_rate = wf.getframerate()
        n_channels = wf.getnchannels()
        sample_width = wf.getsampwidth()
        n_frames = wf.getnframes()

        raw_data = wf.readframes(n_frames)

        # Convert bytes to numpy array based on sample width
        if sample_width == 2:  # 16-bit
            audio = np.frombuffer(raw_data, dtype=np.int16)
        elif sample_width == 1:  # 8-bit
            audio = np.frombuffer(raw_data, dtype=np.uint8).astype(np.int16) - 128
        else:
            raise ValueError(f"Unsupported sample width: {sample_width}")

        # Handle stereo by averaging channels
        if n_channels == 2:
            audio = audio.reshape(-1, 2).mean(axis=1).astype(np.int16)

        # Normalize to [-1, 1]
        audio_normalized = audio.astype(np.float32) / 32768.0

        return audio_normalized, sample_rate


def load_audio_ffmpeg(audio_path: str, target_sr: int = 16000) -> tuple[np.ndarray, int]:
    """Load audio using ffmpeg subprocess (supports more formats)."""
    import subprocess

    cmd = [
        'ffmpeg', '-hide_banner', '-loglevel', 'error',
        '-i', audio_path,
        '-f', 's16le', '-acodec', 'pcm_s16le',
        '-ac', '1', '-ar', str(target_sr),
        'pipe:1'
    ]

    result = subprocess.run(cmd, capture_output=True)
    if result.returncode != 0:
        raise RuntimeError(f"ffmpeg failed: {result.stderr.decode()}")

    audio = np.frombuffer(result.stdout, dtype=np.int16)
    audio_normalized = audio.astype(np.float32) / 32768.0

    return audio_normalized, target_sr


def load_vad_results(json_path: str) -> dict:
    """Load VAD analysis results from JSON file."""
    with open(json_path, 'r') as f:
        return json.load(f)


def find_speech_segments(data_points: list, threshold: float = 0.5) -> list[tuple[float, float]]:
    """Extract continuous speech segments from VAD data points."""
    segments = []
    in_speech = False
    start_time = 0.0

    for point in data_points:
        is_speech = point['probability'] >= threshold
        time = point['time']

        if is_speech and not in_speech:
            # Speech starts
            in_speech = True
            start_time = time
        elif not is_speech and in_speech:
            # Speech ends
            in_speech = False
            segments.append((start_time, time))

    # Handle case where speech continues to end
    if in_speech and data_points:
        segments.append((start_time, data_points[-1]['time']))

    return segments


def visualize_vad(audio_path: str, vad_json_path: str, output_path: str = None):
    """
    Create visualization comparing audio waveform with VAD probability.

    Args:
        audio_path: Path to the audio file
        vad_json_path: Path to VAD analysis JSON file
        output_path: Optional path to save the output image
    """
    # Load data
    print(f"Loading audio: {audio_path}")

    # Try different audio loading methods
    try:
        if audio_path.lower().endswith('.wav'):
            audio, sample_rate = load_wav_audio(audio_path)
        else:
            audio, sample_rate = load_audio_ffmpeg(audio_path)
    except Exception as e:
        print(f"Failed to load with wave module: {e}")
        print("Trying ffmpeg...")
        audio, sample_rate = load_audio_ffmpeg(audio_path)

    print(f"  Sample rate: {sample_rate} Hz")
    print(f"  Duration: {len(audio) / sample_rate:.2f} seconds")

    print(f"Loading VAD results: {vad_json_path}")
    vad_data = load_vad_results(vad_json_path)
    data_points = vad_data['data_points']
    threshold = vad_data.get('threshold', 0.5)

    print(f"  Threshold: {threshold}")
    print(f"  Total frames: {vad_data.get('total_frames', len(data_points))}")
    print(f"  Speech frames: {vad_data.get('speech_frames', 'N/A')}")

    # Extract VAD data
    vad_times = np.array([p['time'] for p in data_points])
    vad_probs = np.array([p['probability'] for p in data_points])
    vad_rms = np.array([p['audio_rms'] for p in data_points])

    # Find speech segments for highlighting
    segments = find_speech_segments(data_points, threshold)

    # Create figure with 4 subplots
    fig, axes = plt.subplots(4, 1, figsize=(16, 12), sharex=True)
    fig.suptitle(f'VAD Analysis: {Path(audio_path).name}', fontsize=14, fontweight='bold')

    # Time axis for audio waveform
    audio_times = np.arange(len(audio)) / sample_rate

    # === Plot 1: Audio Waveform ===
    ax1 = axes[0]
    ax1.plot(audio_times, audio, linewidth=0.3, color='#2196F3', alpha=0.8)
    ax1.set_ylabel('Amplitude', fontsize=10)
    ax1.set_title('Audio Waveform', fontsize=11, fontweight='bold')
    ax1.set_ylim(-1, 1)
    ax1.grid(True, alpha=0.3)

    # Highlight speech regions on waveform
    for start, end in segments:
        ax1.axvspan(start, end, alpha=0.15, color='green')

    # === Plot 2: VAD Speech Probability ===
    ax2 = axes[1]
    ax2.plot(vad_times, vad_probs, linewidth=1.5, color='#4CAF50', label='Speech Probability')
    ax2.fill_between(vad_times, 0, vad_probs, alpha=0.3, color='#4CAF50')
    ax2.axhline(y=threshold, color='#F44336', linestyle='--', linewidth=2, label=f'Threshold ({threshold})')
    ax2.set_ylabel('Probability', fontsize=10)
    ax2.set_title('VAD Speech Probability', fontsize=11, fontweight='bold')
    ax2.set_ylim(0, 1.05)
    ax2.legend(loc='upper right', fontsize=9)
    ax2.grid(True, alpha=0.3)

    # === Plot 3: Audio RMS (from VAD frames) ===
    ax3 = axes[2]
    ax3.plot(vad_times, vad_rms, linewidth=1, color='#FF9800', label='Frame RMS')
    ax3.fill_between(vad_times, 0, vad_rms, alpha=0.3, color='#FF9800')
    ax3.set_ylabel('RMS Level', fontsize=10)
    ax3.set_title('Audio Energy (RMS per VAD frame)', fontsize=11, fontweight='bold')
    ax3.legend(loc='upper right', fontsize=9)
    ax3.grid(True, alpha=0.3)

    # Highlight speech regions
    for start, end in segments:
        ax3.axvspan(start, end, alpha=0.15, color='green')

    # === Plot 4: Combined View (Waveform + Probability overlay) ===
    ax4 = axes[3]
    ax4b = ax4.twinx()

    # Waveform (left axis)
    ax4.plot(audio_times, audio, linewidth=0.3, color='#2196F3', alpha=0.6, label='Waveform')
    ax4.set_ylabel('Amplitude', color='#2196F3', fontsize=10)
    ax4.tick_params(axis='y', labelcolor='#2196F3')
    ax4.set_ylim(-1, 1)

    # Probability (right axis)
    ax4b.plot(vad_times, vad_probs, linewidth=2, color='#F44336', label='VAD Probability')
    ax4b.axhline(y=threshold, color='#F44336', linestyle='--', linewidth=1, alpha=0.5)
    ax4b.set_ylabel('Speech Probability', color='#F44336', fontsize=10)
    ax4b.tick_params(axis='y', labelcolor='#F44336')
    ax4b.set_ylim(0, 1.05)

    # Highlight detected speech regions
    for start, end in segments:
        ax4.axvspan(start, end, alpha=0.2, color='#4CAF50')

    ax4.set_xlabel('Time (seconds)', fontsize=10)
    ax4.set_title('Combined View: Waveform + VAD Probability', fontsize=11, fontweight='bold')
    ax4.grid(True, alpha=0.3)

    # Add legend
    legend_elements = [
        Patch(facecolor='#4CAF50', alpha=0.3, label='Detected Speech'),
        Patch(facecolor='#2196F3', alpha=0.6, label='Waveform'),
        Patch(facecolor='#F44336', alpha=0.8, label='VAD Probability'),
    ]
    ax4.legend(handles=legend_elements, loc='upper right', fontsize=9)

    # Adjust layout
    plt.tight_layout()
    plt.subplots_adjust(top=0.95)

    # Save the figure
    if output_path:
        save_path = output_path
    else:
        # Default output path
        save_path = Path(vad_json_path).stem + '_visualization.png'

    plt.savefig(save_path, dpi=150, bbox_inches='tight')
    print(f"\nVisualization saved to: {save_path}")
    plt.close(fig)

    # Print summary
    print("\n=== Speech Segments Detected ===")
    total_speech_duration = 0
    for i, (start, end) in enumerate(segments, 1):
        duration = end - start
        total_speech_duration += duration
        print(f"  Segment {i}: {start:.2f}s - {end:.2f}s (duration: {duration:.2f}s)")

    total_duration = len(audio) / sample_rate
    speech_ratio = total_speech_duration / total_duration * 100 if total_duration > 0 else 0
    print(f"\nTotal speech: {total_speech_duration:.2f}s / {total_duration:.2f}s ({speech_ratio:.1f}%)")


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        print("\nUsage: python3 visualize_vad.py <audio_file> <vad_json_file> [output_image]")
        print("\nExample:")
        print("  # First, run the Go analyzer to generate JSON:")
        print("  go run -tags vad vad_analyze.go")
        print("")
        print("  # Then visualize:")
        print("  python3 visualize_vad.py vad_test_en.wav vad_analysis.json")
        sys.exit(1)

    audio_path = sys.argv[1]
    vad_json_path = sys.argv[2]
    output_path = sys.argv[3] if len(sys.argv) > 3 else None

    # Validate inputs
    if not Path(audio_path).exists():
        print(f"Error: Audio file not found: {audio_path}")
        sys.exit(1)

    if not Path(vad_json_path).exists():
        print(f"Error: VAD JSON file not found: {vad_json_path}")
        print("\nFirst run the Go analyzer:")
        print("  go run -tags vad vad_analyze.go")
        sys.exit(1)

    visualize_vad(audio_path, vad_json_path, output_path)


if __name__ == '__main__':
    main()
