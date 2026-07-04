#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
JNI_DIR="$PROJECT_DIR/plugin-mpv-player/src/main/jni"
OUTPUT_DIR="$PROJECT_DIR/plugin-mpv-player/src/main/jniLibs/arm64-v8a"

echo "build-player-so: Building libplayer.so from source..."

# --- Find NDK ---
NDK_ROOT="${ANDROID_NDK_HOME:-}"
if [ -z "$NDK_ROOT" ] || [ ! -d "$NDK_ROOT" ]; then
    for d in "$HOME/Android/Sdk/ndk/"*/; do
        if [ -d "$d" ]; then NDK_ROOT="$d"; break; fi
    done
fi
if [ -z "$NDK_ROOT" ] || [ ! -f "$NDK_ROOT/ndk-build" ]; then
    echo "ERROR: Android NDK not found (ndk-build not found)"
    echo "Set ANDROID_NDK_HOME or install via: sdkmanager 'ndk;26.1.10909125'"
    exit 1
fi
echo "  NDK: $NDK_ROOT"

# --- Check prerequisites ---
if [ ! -f "$JNI_DIR/Android.mk" ]; then
    echo "ERROR: $JNI_DIR/Android.mk not found. JNI source files missing."
    exit 1
fi

PREBUILT_COUNT=$(find "$OUTPUT_DIR" -name "*.so" ! -name "libplayer.so" | wc -l)
if [ "$PREBUILT_COUNT" -lt 5 ]; then
    echo "ERROR: Not enough prebuilt .so files in $OUTPUT_DIR ($PREBUILT_COUNT found, need libmpv.so + ffmpeg libs)"
    echo "Run setup-mpv-libs.sh first to extract prebuilt libraries from AAR."
    exit 1
fi
echo "  Prebuilt libraries: $PREBUILT_COUNT .so files in $OUTPUT_DIR"

# --- Remove old libplayer.so if it exists (from AAR) ---
rm -f "$OUTPUT_DIR/libplayer.so"
echo "  Removed any existing libplayer.so (will rebuild from source)"

# --- Build with ndk-build ---
NDBUILD_CMD="$NDK_ROOT/ndk-build"
echo ""
echo "  Running ndk-build..."
$NDBUILD_CMD \
    -C "$PROJECT_DIR/plugin-mpv-player/src/main" \
    APP_ABI=arm64-v8a \
    APP_PLATFORM=android-21 \
    NDK_PROJECT_PATH=. \
    NDK_APPLICATION_MK=jni/Application.mk \
    -j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4) \
    2>&1

# --- Verify output ---
if [ -f "$OUTPUT_DIR/libplayer.so" ]; then
    SIZE=$(ls -lh "$OUTPUT_DIR/libplayer.so" | awk '{print $5}')
    echo ""
    echo "✅ libplayer.so built successfully ($SIZE)"
else
    # ndk-build may output to libs/ instead of jniLibs/
    ALT_OUTPUT="$PROJECT_DIR/plugin-mpv-player/src/main/libs/arm64-v8a/libplayer.so"
    if [ -f "$ALT_OUTPUT" ]; then
        mkdir -p "$OUTPUT_DIR"
        cp "$ALT_OUTPUT" "$OUTPUT_DIR/libplayer.so"
        SIZE=$(ls -lh "$OUTPUT_DIR/libplayer.so" | awk '{print $5}')
        echo ""
        echo "✅ libplayer.so built and copied ($SIZE)"
    else
        echo ""
        echo "❌ libplayer.so not found after build!"
        echo "  Expected at: $OUTPUT_DIR/libplayer.so"
        exit 1
    fi
fi

TOTAL=$(find "$OUTPUT_DIR" -name "*.so" | wc -l)
echo "  Total .so files in $OUTPUT_DIR: $TOTAL"
