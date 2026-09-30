#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$ROOT_DIR/build"
DIST_DIR="$ROOT_DIR/dist"
APP_NAME="Duplicate Finder"
APP_DIR="$DIST_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
BIN_NAME="DuplicateFinder"
BIN_PATH="$BUILD_DIR/$BIN_NAME"
ICON_PATH="$ROOT_DIR/assets/app-icon.icns"
LOCAL_GO="$ROOT_DIR/.tools/go/bin/go"

if [ -x "$LOCAL_GO" ]; then
  GO_CMD="$LOCAL_GO"
else
  GO_CMD="go"
fi

export TMPDIR="$ROOT_DIR/.tmp"

mkdir -p "$BUILD_DIR" "$DIST_DIR"
rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

cd "$ROOT_DIR"

"$GO_CMD" mod tidy
CGO_ENABLED=1 "$GO_CMD" build -o "$BIN_PATH" .

cp "$BIN_PATH" "$MACOS_DIR/$BIN_NAME"

if [ -f "$ICON_PATH" ]; then
  cp "$ICON_PATH" "$RESOURCES_DIR/app-icon.icns"
fi

/usr/bin/plutil -convert xml1 -o "$CONTENTS_DIR/Info.plist" - <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>zh_CN</string>
  <key>CFBundleDisplayName</key>
  <string>Duplicate Finder</string>
  <key>CFBundleExecutable</key>
  <string>DuplicateFinder</string>
  <key>CFBundleIconFile</key>
  <string>app-icon.icns</string>
  <key>CFBundleIdentifier</key>
  <string>com.zhaohui.duplicatefinder.go</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>Duplicate Finder</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0.0</string>
  <key>CFBundleVersion</key>
  <string>1.0.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

printf 'APPL????' > "$CONTENTS_DIR/PkgInfo"

echo "Packaged app: $APP_DIR"
