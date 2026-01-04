#!/bin/bash
# Setup ONNX Runtime environment for VAD support
#
# Usage:
#   source scripts/setup-onnx-env.sh
#   # or
#   eval "$(scripts/setup-onnx-env.sh)"

set -e

if ! command -v brew &> /dev/null; then
    echo "Error: Homebrew not found. Please install onnxruntime manually." >&2
    exit 1
fi

if ! brew --prefix onnxruntime &> /dev/null; then
    echo "Error: onnxruntime not installed. Run: brew install onnxruntime" >&2
    exit 1
fi

ORT_PREFIX="$(brew --prefix onnxruntime)"

# Output export commands (can be eval'd or sourced)
echo "export CGO_CFLAGS=\"-I$ORT_PREFIX/include/onnxruntime\""
echo "export CGO_LDFLAGS=\"-L$ORT_PREFIX/lib -lonnxruntime\""

# If sourced directly, also set the variables
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
    export CGO_CFLAGS="-I$ORT_PREFIX/include/onnxruntime"
    export CGO_LDFLAGS="-L$ORT_PREFIX/lib -lonnxruntime"
    echo "ONNX Runtime environment configured: $ORT_PREFIX" >&2
fi
