#!/usr/bin/env bash
set -euo pipefail

if [ "${CI:-}" = "true" ]; then
    echo "::warning::In CI, mpv libs should come from 'build-mpv-lib' workflow Release."
    echo "::warning::This script is intended for local development only."
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
JNI_DIR="$PROJECT_DIR/plugin-mpv-player/src/main/jniLibs"

MPV_LIB_VERSION="0.1.12"
AAR_URL="https://repo1.maven.org/maven2/io/github/abdallahmehiz/mpv-android-lib/${MPV_LIB_VERSION}/mpv-android-lib-${MPV_LIB_VERSION}.aar"
AAR_TMP="$(mktemp -d)/mpv-android-lib.aar"

echo "setup-mpv-libs: downloading mpv-android-lib ${MPV_LIB_VERSION} AAR..."
curl -fSL --retry 3 --retry-delay 5 -A "Mozilla/5.0 (compatible; encv-ci/1.0)" -o "$AAR_TMP" "$AAR_URL"

echo "setup-mpv-libs: Phase 1 — extracting prebuilt native libraries (mpv + ffmpeg)..."
mkdir -p "$JNI_DIR"

for abi in arm64-v8a; do
    echo "  extracting $abi..."
    mkdir -p "$JNI_DIR/$abi"
    unzip -o -j "$AAR_TMP" "jni/$abi/*.so" -d "$JNI_DIR/$abi" 2>/dev/null || true
    if [ ! -f "$JNI_DIR/$abi/libmpv.so" ]; then
        echo "  ✗ $abi: libmpv.so not found in AAR!"
        exit 1
    fi
    rm -f "$JNI_DIR/$abi/libplayer.so"
    count=$(find "$JNI_DIR/$abi" -name "*.so" | wc -l)
    echo "  ✓ $abi: ${count} .so files extracted (libplayer.so excluded, will be built from source)"
done

rm -f "$AAR_TMP"

echo ""
echo "setup-mpv-libs: Phase 2 — building libplayer.so (JNI wrapper) from source..."
bash "$SCRIPT_DIR/build-player-so.sh"

echo ""
echo "setup-mpv-libs: done. All libraries saved to $JNI_DIR"
