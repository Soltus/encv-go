#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MANIFEST="${SCRIPT_DIR}/ffmpeg-feature-manifest.json"

echo "=== Validating system FFmpeg against feature manifest ==="

if [ ! -f "$MANIFEST" ]; then
    echo "❌ Manifest not found: $MANIFEST"
    exit 1
fi

command -v ffmpeg >/dev/null || { echo "❌ ffmpeg not found in PATH"; exit 1; }
command -v ffprobe >/dev/null || { echo "❌ ffprobe not found in PATH"; exit 1; }
command -v python3 >/dev/null || { echo "❌ python3 not found in PATH"; exit 1; }

FFMPEG_VERSION=$(ffmpeg -version 2>/dev/null | head -1 | awk '{print $3}' | tr -d '')
echo "System FFmpeg: $FFMPEG_VERSION"
echo "Manifest target: $(python3 -c "import json; print(json.load(open('$MANIFEST'))['ffmpeg']['version'])")"
echo ""

EXIT=0

for decoder in $(python3 -c "import json; print(' '.join(json.load(open('$MANIFEST'))['ffmpeg']['decoders']))"); do
    if ffmpeg -hide_banner -decoders 2>/dev/null | grep -q "^ ${decoder}"; then
        echo "  ✅ decoder: $decoder"
    else
        echo "  ❌ decoder MISSING: $decoder"
        EXIT=1
    fi
done

echo ""

for encoder in $(python3 -c "import json; print(' '.join(json.load(open('$MANIFEST'))['ffmpeg']['encoders']))"); do
    if ffmpeg -hide_banner -encoders 2>/dev/null | grep -q "^ ${encoder}"; then
        echo "  ✅ encoder: $encoder"
    else
        case "$encoder" in
            libx264) echo "  ⚠️  encoder missing: $encoder (external library, may need --enable-gpl build)" ;;
            *) echo "  ⚠️  encoder missing: $encoder (may be hw-specific)" ;;
        esac
    fi
done

echo ""

for muxer in $(python3 -c "import json; print(' '.join(json.load(open('$MANIFEST'))['ffmpeg']['muxers']))"); do
    if ffmpeg -hide_banner -muxers 2>/dev/null | grep -q "^ ${muxer}"; then
        echo "  ✅ muxer: $muxer"
    else
        echo "  ❌ muxer MISSING: $muxer"
        EXIT=1
    fi
done

echo ""

for protocol in $(python3 -c "import json; print(' '.join(json.load(open('$MANIFEST'))['ffmpeg']['protocols']))"); do
    if ffmpeg -hide_banner -protocols 2>/dev/null | grep -q "^ ${protocol}"; then
        echo "  ✅ protocol: $protocol"
    else
        echo "  ❌ protocol MISSING: $protocol"
        EXIT=1
    fi
done

if [ $EXIT -eq 0 ]; then
    echo ""
    echo "✅ All required features present"
else
    echo ""
    echo "❌ Some required features are missing (see above)"
fi

exit $EXIT
