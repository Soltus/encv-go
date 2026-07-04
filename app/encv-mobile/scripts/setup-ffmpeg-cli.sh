#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
JNI_DIR="$PROJECT_DIR/android/app/src/main/jniLibs"

FFMPEG_ANDROID_VERSION="build-259"
FFMPEG_BASE_URL="https://github.com/KaluaBilla/ffmpeg-android/releases/download/${FFMPEG_ANDROID_VERSION}"
FFMPEG_ZIP="ffmpeg-8.0-e05f8ac-Dynamic-android-arm64-v8a.zip"

echo "setup-ffmpeg-cli: downloading ffmpeg/ffprobe CLI binaries for Android arm64..."
TMPDIR=$(mktemp -d)
curl -fSL -o "$TMPDIR/ffmpeg-android.zip" "${FFMPEG_BASE_URL}/${FFMPEG_ZIP}"

echo "setup-ffmpeg-cli: extracting..."
cd "$TMPDIR"
unzip -o "ffmpeg-android.zip" || true

FFMPEG_BIN=""
FFPROBE_BIN=""

find "$TMPDIR" -name "ffmpeg" -type f 2>/dev/null | head -1 && FFMPEG_BIN=$(find "$TMPDIR" -name "ffmpeg" -type f 2>/dev/null | head -1)
find "$TMPDIR" -name "ffprobe" -type f 2>/dev/null | head -1 && FFPROBE_BIN=$(find "$TMPDIR" -name "ffprobe" -type f 2>/dev/null | head -1)

if [ -z "$FFMPEG_BIN" ] || [ -z "$FFPROBE_BIN" ]; then
    echo "setup-ffmpeg-cli: listing archive contents to find binaries..."
    unzip -l "ffmpeg-android.zip" | grep -E "ffmpeg|ffprobe" | head -20
    echo "ERROR: Could not find ffmpeg/ffprobe in archive"
    rm -rf "$TMPDIR"
    exit 1
fi

echo "setup-ffmpeg-cli: found ffmpeg=$FFMPEG_BIN, ffprobe=$FFPROBE_BIN"

mkdir -p "$JNI_DIR/arm64-v8a"

cp "$FFMPEG_BIN" "$JNI_DIR/arm64-v8a/libffmpeg_exec.so"
cp "$FFPROBE_BIN" "$JNI_DIR/arm64-v8a/libffprobe_exec.so"
chmod +x "$JNI_DIR/arm64-v8a/libffmpeg_exec.so" "$JNI_DIR/arm64-v8a/libffprobe_exec.so"

echo "setup-ffmpeg-cli: binaries installed to $JNI_DIR/arm64-v8a/"
ls -lh "$JNI_DIR/arm64-v8a/libffmpeg_exec.so" "$JNI_DIR/arm64-v8a/libffprobe_exec.so"

rm -rf "$TMPDIR"
echo "setup-ffmpeg-cli: done"
