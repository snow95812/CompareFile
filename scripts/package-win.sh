#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$ROOT_DIR/build"
DIST_DIR="$ROOT_DIR/dist/windows-11"
BIN_NAME="DuplicateFinder.exe"
BIN_PATH="$BUILD_DIR/$BIN_NAME"
LOCAL_GO="$ROOT_DIR/.tools/go/bin/go"
LOCAL_ZIG="$ROOT_DIR/.tools/zig/zig-macos-aarch64-0.13.0/zig"

if [ -x "$LOCAL_GO" ]; then
  GO_CMD="$LOCAL_GO"
else
  GO_CMD="go"
fi

if [ ! -x "$LOCAL_ZIG" ]; then
  echo "Missing Zig toolchain: $LOCAL_ZIG" >&2
  exit 1
fi

export TMPDIR="$ROOT_DIR/.tmp"
export CC="$LOCAL_ZIG cc -target x86_64-windows-gnu"
export CXX="$LOCAL_ZIG c++ -target x86_64-windows-gnu"
export CGO_ENABLED=1
export GOOS=windows
export GOARCH=amd64

mkdir -p "$BUILD_DIR" "$DIST_DIR"

cd "$ROOT_DIR"

"$GO_CMD" mod tidy
"$GO_CMD" build -ldflags="-H windowsgui" -o "$BIN_PATH" .

cp "$BIN_PATH" "$DIST_DIR/$BIN_NAME"

echo "Packaged Windows executable: $DIST_DIR/$BIN_NAME"
