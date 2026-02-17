#!/bin/bash
# Setup FFmpeg environment for go-astiav
#
# This script downloads and configures FFmpeg 7.0 for use with go-astiav.
# It uses prebuilt binaries from BtbN/FFmpeg-Builds for stability.
#
# Usage:
#   ./scripts/setup-ffmpeg.sh              # Install FFmpeg
#   source scripts/setup-ffmpeg.sh --env   # Set environment variables
#   eval "$(scripts/setup-ffmpeg.sh --env)" # Alternative env setup

set -e

FFMPEG_VERSION="8.0"
FFMPEG_DIR="${FFMPEG_DIR:-$HOME/ffmpeg}"
# go-astiav v0.30.0 requires FFmpeg n8.0
FFMPEG_URL="https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n8.0-latest-linux64-gpl-shared-8.0.tar.xz"
FFMPEG_MACOS_URL="https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n8.0-latest-macos64-gpl-shared-8.0.tar.xz"

print_env() {
    echo "export PKG_CONFIG_PATH=\"$FFMPEG_DIR/lib/pkgconfig:\$PKG_CONFIG_PATH\""
    echo "export LD_LIBRARY_PATH=\"$FFMPEG_DIR/lib:\$LD_LIBRARY_PATH\""
    echo "export DYLD_LIBRARY_PATH=\"$FFMPEG_DIR/lib:\$DYLD_LIBRARY_PATH\""
    echo "export CGO_CFLAGS=\"-I$FFMPEG_DIR/include\""
    echo "export CGO_LDFLAGS=\"-L$FFMPEG_DIR/lib\""
    echo "export PATH=\"$FFMPEG_DIR/bin:\$PATH\""
}

set_env() {
    export PKG_CONFIG_PATH="$FFMPEG_DIR/lib/pkgconfig:$PKG_CONFIG_PATH"
    export LD_LIBRARY_PATH="$FFMPEG_DIR/lib:$LD_LIBRARY_PATH"
    export DYLD_LIBRARY_PATH="$FFMPEG_DIR/lib:$DYLD_LIBRARY_PATH"
    export CGO_CFLAGS="-I$FFMPEG_DIR/include"
    export CGO_LDFLAGS="-L$FFMPEG_DIR/lib"
    export PATH="$FFMPEG_DIR/bin:$PATH"
}

# Handle --env flag
if [[ "$1" == "--env" ]]; then
    if [[ ! -d "$FFMPEG_DIR" ]]; then
        echo "Error: FFmpeg not installed at $FFMPEG_DIR" >&2
        echo "Run: ./scripts/setup-ffmpeg.sh" >&2
        exit 1
    fi
    print_env
    # If sourced directly, also set the variables
    if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
        set_env
        echo "FFmpeg environment configured: $FFMPEG_DIR" >&2
    fi
    exit 0
fi

# Detect OS
OS="$(uname -s)"
ARCH="$(uname -m)"

echo "=== FFmpeg Setup for go-astiav ==="
echo "OS: $OS, Arch: $ARCH"
echo "Install directory: $FFMPEG_DIR"
echo ""

# Check if already installed
if [[ -f "$FFMPEG_DIR/bin/ffmpeg" ]]; then
    echo "FFmpeg already installed at $FFMPEG_DIR"
    "$FFMPEG_DIR/bin/ffmpeg" -version | head -1
    echo ""
    echo "To use, run:"
    echo "  eval \"\$(./scripts/setup-ffmpeg.sh --env)\""
    exit 0
fi

install_linux() {
    echo "Installing FFmpeg $FFMPEG_VERSION for Linux..."

    # Check for pkg-config
    if ! command -v pkg-config &> /dev/null; then
        echo "Installing pkg-config..."
        if command -v apt-get &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y pkg-config
        elif command -v yum &> /dev/null; then
            sudo yum install -y pkgconfig
        else
            echo "Error: Please install pkg-config manually" >&2
            exit 1
        fi
    fi

    # Download and extract
    TMPDIR=$(mktemp -d)
    cd "$TMPDIR"
    echo "Downloading FFmpeg prebuilt from BtbN/FFmpeg-Builds..."
    wget -q --show-progress "$FFMPEG_URL" -O ffmpeg.tar.xz
    echo "Extracting..."
    tar xf ffmpeg.tar.xz
    mv ffmpeg-n${FFMPEG_VERSION}-latest-linux64-gpl-shared-8.0 "$FFMPEG_DIR"
    cd -
    rm -rf "$TMPDIR"

    echo "FFmpeg installed successfully!"
}

install_macos() {
    echo "Installing FFmpeg for macOS..."

    # Prefer Homebrew for macOS (simpler and well-maintained)
    if command -v brew &> /dev/null; then
        echo "Using Homebrew to install FFmpeg..."
        brew install ffmpeg

        # Get Homebrew FFmpeg path
        BREW_FFMPEG="$(brew --prefix ffmpeg)"

        # Create symlink to standard location
        if [[ ! -d "$FFMPEG_DIR" ]]; then
            ln -s "$BREW_FFMPEG" "$FFMPEG_DIR"
        fi

        echo "FFmpeg installed via Homebrew at $BREW_FFMPEG"
    else
        echo "Error: Homebrew not found. Please install Homebrew first:" >&2
        echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"" >&2
        exit 1
    fi
}

# Install based on OS
case "$OS" in
    Linux)
        install_linux
        ;;
    Darwin)
        install_macos
        ;;
    *)
        echo "Error: Unsupported OS: $OS" >&2
        exit 1
        ;;
esac

# Verify installation
echo ""
echo "=== Verification ==="
"$FFMPEG_DIR/bin/ffmpeg" -version | head -3

echo ""
echo "=== Setup Complete ==="
echo ""
echo "To configure your environment, run:"
echo "  eval \"\$(./scripts/setup-ffmpeg.sh --env)\""
echo ""
echo "Or add to your shell profile (~/.bashrc or ~/.zshrc):"
print_env
