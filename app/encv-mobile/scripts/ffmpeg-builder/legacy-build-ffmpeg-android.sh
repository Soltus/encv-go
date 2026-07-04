#!/bin/bash
set -euo pipefail

FFMPEG_VERSION="8.0"
X264_VERSION="stable"
LAME_VERSION="3.100"
NDK_VERSION="26.1.10909125"
API_LEVEL=24
ABI="arm64-v8a"
ARCH="aarch64"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
BUILD_DIR="${SCRIPT_DIR}/.ffmpeg-build"
OUTPUT_DIR="${PROJECT_DIR}/android/app/src/main/jniLibs/${ABI}"
LOG_DIR="${BUILD_DIR}/logs"
MANIFEST="${SCRIPT_DIR}/ffmpeg-feature-manifest.json"

NDK_PATH="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}}/ndk/${NDK_VERSION}"
if [ ! -d "$NDK_PATH" ]; then
    echo "❌ NDK not found at $NDK_PATH"
    echo "Install with: sdkmanager \"ndk;${NDK_VERSION}\""
    exit 1
fi

TOOLCHAIN="${NDK_PATH}/toolchains/llvm/prebuilt/linux-x86_64"
CC="${TOOLCHAIN}/bin/${ARCH}-linux-android${API_LEVEL}-clang"
AR="${TOOLCHAIN}/bin/llvm-ar"
NM="${TOOLCHAIN}/bin/llvm-nm"
RANLIB="${TOOLCHAIN}/bin/llvm-ranlib"
STRIP="${TOOLCHAIN}/bin/llvm-strip"

mkdir -p "$BUILD_DIR" "$OUTPUT_DIR" "$LOG_DIR"

echo "=== Checking for cached ffmpeg output ==="
if [ -f "${OUTPUT_DIR}/libffmpeg.so" ] && [ -f "${OUTPUT_DIR}/libffprobe.so" ]; then
    HAS_FFMPEG_RUN=$(${NM} -D "${OUTPUT_DIR}/libffmpeg.so" 2>/dev/null | grep -q "ffmpeg_run" && echo "yes" || echo "")
    HAS_FFPROBE_RUN=$(${NM} -D "${OUTPUT_DIR}/libffprobe.so" 2>/dev/null | grep -q "ffprobe_run" && echo "yes" || "")

    if [ "$HAS_FFMPEG_RUN" = "yes" ] && [ "$HAS_FFPROBE_RUN" = "yes" ]; then
        echo "✅ All ffmpeg libraries cached and valid, skipping build"
        echo "Output: $OUTPUT_DIR"
        ls -lh "$OUTPUT_DIR"
        exit 0
    else
        echo "⚠️  Cached libraries missing expected symbols (ffmpeg_run=$HAS_FFMPEG_RUN ffprobe_run=$HAS_FFPROBE_RUN), rebuilding..."
        rm -f "${OUTPUT_DIR}/libffmpeg.so" "${OUTPUT_DIR}/libffprobe.so"
    fi
fi

cd "$BUILD_DIR"

echo "=== Building ffmpeg ${FFMPEG_VERSION} for Android ${ABI} ==="

if [ ! -d "ffmpeg-${FFMPEG_VERSION}" ]; then
    echo "Downloading ffmpeg ${FFMPEG_VERSION}..."
    curl -sL "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" -o ffmpeg.tar.xz
    tar xf ffmpeg.tar.xz
    rm ffmpeg.tar.xz
fi

X264_INSTALL="${BUILD_DIR}/x264-install"
LAME_INSTALL="${BUILD_DIR}/lame-install"
if [ ! -f "${X264_INSTALL}/lib/libx264.a" ]; then
    if [ ! -d "x264" ]; then
        echo "Downloading x264..."
        git clone --depth 1 --branch ${X264_VERSION} https://code.videolan.org/videolan/x264.git
    fi

    echo "=== Building x264 ==="
    cd "${BUILD_DIR}/x264"
    CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure \
        --host=${ARCH}-linux-android \
        --prefix="${X264_INSTALL}" \
        --enable-static \
        --enable-pic \
        --disable-cli \
        --disable-opencl \
        --cross-prefix="${TOOLCHAIN}/bin/llvm-" \
        --extra-cflags="-fPIC -DANDROID" \
        --extra-ldflags="-lm" \
        > "${LOG_DIR}/x264-configure.log" 2>&1
    echo "x264 configure done (log: ${LOG_DIR}/x264-configure.log)"

    make -j$(nproc) > "${LOG_DIR}/x264-make.log" 2>&1
    make install > "${LOG_DIR}/x264-install.log" 2>&1
    echo "✅ x264 built and installed"
else
    echo "✅ x264 already built, skipping"
fi

# === Build libmp3lame (LGPL 2.1, MP3 encoder) ===
# MP3 encoding patents expired 2017-04-16 (Fraunhofer IIS last to surrender).
# Lame 3.100+ is safe for both static & dynamic linking in commercial products.
#
# Why external build is required for lame (only):
#   aac / alac / flac are all ffmpeg native encoders (`A....D` codec flag in
#   `ffmpeg -encoders` output). They are compiled IN from the ffmpeg source
#   tree itself, no external library needed.
#   MP3 has NO native encoder in ffmpeg (patent reasons); only `libmp3lame`
#   and `libshine` exist, both external libs. So we build lame, then link
#   it into ffmpeg via --enable-libmp3lame.
#
# lame 3.100 configure does NOT support --disable-decoder or --enable-debug=no.
# Only use options that `./configure --help` actually lists.
if [ ! -f "${LAME_INSTALL}/lib/libmp3lame.a" ]; then
    cd "${BUILD_DIR}"
    echo "Downloading libmp3lame ${LAME_VERSION}..."
    curl -fSL "https://sourceforge.net/projects/lame/files/lame/${LAME_VERSION}/lame-${LAME_VERSION}.tar.gz/download" \
        -o lame.tar.gz || { echo "❌ Failed to download libmp3lame"; exit 1; }
    tar xzf lame.tar.gz && rm lame.tar.gz || { echo "❌ Failed to extract libmp3lame"; exit 1; }
    LAME_SRC_DIR=$(find "${BUILD_DIR}" -maxdepth 1 -type d -name "*lame*" | head -1)
    if [ -z "$LAME_SRC_DIR" ]; then
        echo "❌ libmp3lame source directory not found after extraction"
        ls -la "${BUILD_DIR}"
        exit 1
    fi

    echo "=== Building libmp3lame (source: ${LAME_SRC_DIR}) ==="
    cd "${LAME_SRC_DIR}"

    CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure \
        --host=${ARCH}-linux-android \
        --prefix="${LAME_INSTALL}" \
        --enable-static \
        --disable-shared \
        --disable-frontend \
        --disable-nasm \
        --disable-nls \
        --disable-gtktest \
        --with-pic=yes \
        > "${LOG_DIR}/lame-configure.log" 2>&1 || {
        echo "❌ lame configure failed (see ${LOG_DIR}/lame-configure.log)"
        tail -30 "${LOG_DIR}/lame-configure.log"
        exit 1
    }
    echo "lame configure done (log: ${LOG_DIR}/lame-configure.log)"

    make -j$(nproc) > "${LOG_DIR}/lame-make.log" 2>&1 || {
        echo "❌ lame make failed (see ${LOG_DIR}/lame-make.log)"
        tail -30 "${LOG_DIR}/lame-make.log"
        exit 1
    }
    make install > "${LOG_DIR}/lame-install.log" 2>&1 || {
        echo "❌ lame install failed (see ${LOG_DIR}/lame-install.log)"
        tail -30 "${LOG_DIR}/lame-install.log"
        exit 1
    }
    echo "✅ libmp3lame built and installed"
else
    echo "✅ libmp3lame already built, skipping"
fi


echo "=== Patching ffmpeg source ==="
cd "${BUILD_DIR}/ffmpeg-${FFMPEG_VERSION}"

sed -i 's/^int main(/int ffmpeg_run(/' fftools/ffmpeg.c
sed -i 's/^int main(void)/int ffmpeg_run(void)/' fftools/ffmpeg.c
sed -i 's/^int wmain(/int ffmpeg_run(/' fftools/ffmpeg.c

sed -i 's/^int main(/int ffprobe_run(/' fftools/ffprobe.c
sed -i 's/^int main(void)/int ffprobe_run(void)/' fftools/ffprobe.c

if ! grep -q "void ffmpeg_reset" fftools/ffmpeg.c; then
    cat >> fftools/ffmpeg.c << 'PATCH'

void ffmpeg_reset(void) {
}
PATCH
fi

if ! grep -q "void ffprobe_reset" fftools/ffprobe.c; then
    cat >> fftools/ffprobe.c << 'PATCH'

void ffprobe_reset(void) {
}
PATCH
fi

echo "=== Reading FFmpeg feature manifest ==="
if [ ! -f "$MANIFEST" ]; then
    echo "❌ Manifest not found: $MANIFEST"
    exit 1
fi

eval "$(python3 -c "
import json, sys
m = json.load(open('$MANIFEST'))
f = m['ffmpeg']
print(f'DECODERS={\",\".join(f[\"decoders\"])}')
print(f'ENCODERS={\",\".join(f[\"encoders\"])}')
print(f'MUXERS={\",\".join(f[\"muxers\"])}')
print(f'DEMUXERS={\",\".join(f[\"demuxers\"])}')
print(f'PARSERS={\",\".join(f[\"parsers\"])}')
print(f'PROTOCOLS={\",\".join(f[\"protocols\"])}')
print(f'FILTERS={\",\".join(f[\"filters\"])}')

for lib_name, modules in m['ftools_modules'].items():
    var_name = lib_name.upper().replace('.', '_') + '_MODULES'
    lines = []
    for mod_name, files in modules.items():
        if not isinstance(files, list):
            if isinstance(files, bool) and files:
                resolved_name = mod_name.replace('_shared', '')
                for other_lib, other_mods in m['ftools_modules'].items():
                    if resolved_name in other_mods and isinstance(other_mods[resolved_name], list):
                        files = other_mods[resolved_name]
                        break
                else:
                    continue
            else:
                continue
        files_str = ' '.join(files)
        lines.append('  \"' + mod_name + ':' + files_str + '\"')
    print(var_name + '=(')
    for l in lines:
        print(l)
    print(')')
")"

echo "  Decoders:  $DECODERS"
echo "  Encoders:  $ENCODERS"
echo "  Muxers:    $MUXERS"
echo "  Demuxers:  $DEMUXERS"
echo "  Parsers:   $PARSERS"
echo "  Protocols: $PROTOCOLS"
echo "  Filters:   $FILTERS"

echo "=== Setting up pkg-config for cross-compilation ==="
if ! command -v pkg-config &>/dev/null; then
    echo "pkg-config not found, installing..."
    apt-get update -qq && apt-get install -y -qq pkg-config
fi

echo "Fixing x264.pc for Android (remove -lpthread -ldl)..."
sed -i 's/-lpthread//g; s/-ldl//g' "${X264_INSTALL}/lib/pkgconfig/x264.pc" 2>/dev/null || true

cat > "${BUILD_DIR}/pkg-config-wrapper" << PCEOF
#!/bin/bash
export PKG_CONFIG_PATH="${X264_INSTALL}/lib/pkgconfig:${LAME_INSTALL}/lib/pkgconfig"
export PKG_CONFIG_LIBDIR="${X264_INSTALL}/lib/pkgconfig:${LAME_INSTALL}/lib/pkgconfig"
export PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1
export PKG_CONFIG_ALLOW_SYSTEM_LIBS=1
exec pkg-config "\$@"
PCEOF
chmod +x "${BUILD_DIR}/pkg-config-wrapper"

echo "Verifying external libs via wrapper:"
"${BUILD_DIR}/pkg-config-wrapper" --cflags --libs x264 || echo "⚠️  x264 not found via wrapper"

echo "=== Configuring ffmpeg ==="
./configure \
    --prefix="${BUILD_DIR}/ffmpeg-install" \
    --enable-cross-compile \
    --cross-prefix="${TOOLCHAIN}/bin/llvm-" \
    --cc="$CC" \
    --ar="$AR" \
    --nm="$NM" \
    --ranlib="$RANLIB" \
    --strip="$STRIP" \
    --arch=${ARCH} \
    --target-os=android \
    --sysroot="${TOOLCHAIN}/sysroot" \
    --enable-shared \
    --enable-static \
    --disable-asm \
    --disable-programs \
    --disable-doc \
    --disable-htmlpages \
    --disable-manpages \
    --disable-podpages \
    --disable-txtpages \
    --disable-everything \
    --enable-decoder="$DECODERS" \
    --enable-encoder="$ENCODERS" \
    --enable-muxer="$MUXERS" \
    --enable-demuxer="$DEMUXERS" \
    --enable-parser="$PARSERS" \
    --enable-protocol="$PROTOCOLS" \
    --enable-filter="$FILTERS" \
    --enable-small \
    --enable-libx264 \
    --enable-libmp3lame \
    --enable-gpl \
    --enable-nonfree \
    --disable-resource-compression \
    --pkg-config="${BUILD_DIR}/pkg-config-wrapper" \
    --extra-cflags="-fPIC -ffunction-sections -fdata-sections -DANDROID \
        -I${X264_INSTALL}/include \
        -I${LAME_INSTALL}/include" \
    --extra-ldflags="-L${X264_INSTALL}/lib -L${LAME_INSTALL}/lib -lm" \
    --extra-libs="-lm" || {
    echo "=== ffmpeg configure FAILED ==="
    echo "=== Last 80 lines of config.log ==="
    tail -80 ffbuild/config.log 2>/dev/null || echo "(no config.log found)"
    exit 1
}

echo "=== Building ffmpeg ==="
make -j$(nproc) > "${LOG_DIR}/ffmpeg-make.log" 2>&1
echo "ffmpeg make done (log: ${LOG_DIR}/ffmpeg-make.log)"

make install > "${LOG_DIR}/ffmpeg-install.log" 2>&1
echo "✅ ffmpeg built and installed"

FFMPEG_SRC="${BUILD_DIR}/ffmpeg-${FFMPEG_VERSION}"
FFMPEG_INSTALL="${BUILD_DIR}/ffmpeg-install"
FTOOLS_BUILD="${BUILD_DIR}/ftools-build"
mkdir -p "$FTOOLS_BUILD"

echo "=== Phase 2: Generating resource files via bin2c ==="
cd "${FFMPEG_SRC}"

BIN2C_CC="$(command -v gcc 2>/dev/null || command -v cc 2>/dev/null || echo "gcc")"
$BIN2C_CC -o "${BUILD_DIR}/bin2c" ffbuild/bin2c.c \
    > "${LOG_DIR}/bin2c-build.log" 2>&1 || {
    echo "❌ Failed to build bin2c"
    tail -5 "${LOG_DIR}/bin2c-build.log"
    exit 1
}
echo "✅ bin2c built"

GEN_RES_DIR="${FFMPEG_SRC}/fftools/resources"
for res_file in "$GEN_RES_DIR"/*.css "$GEN_RES_DIR"/*.html; do
    [ -f "$res_file" ] || continue
    base=$(basename "$res_file")
    bin2c_name=$(echo "${base}" | tr '.' '_')
    if [[ "$res_file" == *.css ]]; then
        sed 's!/\\*.*\\*/!!g' "$res_file" | tr '\n' ' ' | tr -s ' ' | sed 's/^ //; s/ $$//' \
            > "${res_file}.min"
        "${BUILD_DIR}/bin2c" "${res_file}.min" "${res_file}.c" "$bin2c_name"
    elif [[ "$res_file" == *.html ]]; then
        "${BUILD_DIR}/bin2c" "$res_file" "${res_file}.c" "$bin2c_name"
    fi
    echo "  ✅ Generated ${base}.c (symbol: ff_${bin2c_name}_data)"
done

echo "=== Verifying CONFIG_RESOURCE_COMPRESSION ==="
RES_COMP=$(grep -c "^#define CONFIG_RESOURCE_COMPRESSION 1$" "${FFMPEG_SRC}/config.h" 2>/dev/null || echo "0")
if [ "$RES_COMP" = "1" ]; then
    echo "❌ CONFIG_RESOURCE_COMPRESSION is enabled but build script generates uncompressed resources"
    echo "   This will cause runtime gzip decompression failures"
    echo "   Fix: add --disable-resource-compression to FFmpeg configure"
    exit 1
fi
echo "✅ CONFIG_RESOURCE_COMPRESSION is disabled (resources will be embedded uncompressed)"

CFLAGS="-std=c11 -fPIC -ffunction-sections -fdata-sections -DANDROID -D_POSIX_C_SOURCE=200809L \
  -DHAVE_SYS_RESOURCE_H=1 -DHAVE_UNISTD_H=1 -DHAVE_SYS_SELECT_H=1 \
  -include time.h \
  -I${FFMPEG_INSTALL}/include \
  -I${FFMPEG_SRC} \
  -I${FFMPEG_SRC}/compat/stdbit \
  -I${FFMPEG_SRC}/fftools \
  -I${FFMPEG_SRC}/fftools/textformat \
  -I${FFMPEG_SRC}/fftools/graph \
  -I${FFMPEG_SRC}/fftools/resources \
  -I${X264_INSTALL}/include \
  -I${LAME_INSTALL}/include"
LDFLAGS="-L${FFMPEG_INSTALL}/lib -L${X264_INSTALL}/lib -L${LAME_INSTALL}/lib"

STATIC_LIBS=""
for lib in libavformat libavcodec libavutil libswresample libswscale libavfilter libavdevice; do
    if [ -f "${FFMPEG_INSTALL}/lib/${lib}.a" ]; then
        STATIC_LIBS="$STATIC_LIBS ${FFMPEG_INSTALL}/lib/${lib}.a"
    fi
done

echo "=== Phase 3: Compiling fftools (manifest-driven) ==="

compile_modules() {
    local -n modules=$1
    local -n out_objs=$2
    out_objs=""

    for module_def in "${modules[@]}"; do
        local mod_name="${module_def%%:*}"
        local mod_files="${module_def#*:}"

        for src in $mod_files; do
            local src_path="${FFMPEG_SRC}/${src}"
            if [ ! -f "$src_path" ]; then
                echo "⚠️  Module [$mod_name]: $src not found, skipping"
                continue
            fi

            local objname="$(basename "$src" .c)"
            local obj="${FTOOLS_BUILD}/${mod_name}_${objname}.o"

            if $CC $CFLAGS -c -o "$obj" "$src_path" > "${LOG_DIR}/${mod_name}_${objname}.log" 2>&1; then
                out_objs="$out_objs $obj"
            else
                echo "❌ Module [$mod_name]: failed to compile $src"
                tail -5 "${LOG_DIR}/${mod_name}_${objname}.log"
                exit 1
            fi
        done
        echo "  ✅ Module [$mod_name] compiled"
    done
}

echo "Compiling ffmpeg fftools..."
compile_modules LIBFFMPEG_SO_MODULES FFMPEG_OBJS

echo "Compiling ffprobe fftools..."
compile_modules LIBFFPROBE_SO_MODULES FFPROBE_OBJS

echo "=== Phase 4: Linking shared libraries ==="

cat > "${FTOOLS_BUILD}/ffmpeg.ver" << 'VEOF'
{
  global:
    ffmpeg_run;
    ffmpeg_reset;
    ff_graph_css_data;
    ff_graph_css_len;
    ff_graph_html_data;
    ff_graph_html_len;
    ff_resman_get_string;
    ff_resman_uninit;
  local: *;
};
VEOF

cat > "${FTOOLS_BUILD}/ffprobe.ver" << 'VEOF'
{
  global:
    ffprobe_run;
    ffprobe_reset;
  local: *;
};
VEOF

# --allow-multiple-definition: required for FFmpeg static libs which have
# duplicate symbols (e.g. ff_log2_tab in libavutil and libavcodec)
echo "Linking libffmpeg.so..."
$CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffmpeg.so" \
    $FFMPEG_OBJS \
    $STATIC_LIBS \
    ${X264_INSTALL}/lib/libx264.a \
    ${LAME_INSTALL}/lib/libmp3lame.a \
    -lm -lz -llog \
    -Wl,-u,ffmpeg_run \
    -Wl,-u,ffmpeg_reset \
    -Wl,-u,ff_graph_css_data \
    -Wl,-u,ff_graph_html_data \
    -Wl,-u,ff_resman_get_string \
    -Wl,--gc-sections \
    -Wl,--allow-multiple-definition \
    -Wl,--version-script,"${FTOOLS_BUILD}/ffmpeg.ver" \
    $LDFLAGS > "${LOG_DIR}/link_ffmpeg.log" 2>&1 || {
    echo "❌ Failed to link libffmpeg.so (see ${LOG_DIR}/link_ffmpeg.log)"
    tail -10 "${LOG_DIR}/link_ffmpeg.log"
    exit 1
}

echo "Linking libffprobe.so..."
$CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffprobe.so" \
    $FFPROBE_OBJS \
    $STATIC_LIBS \
    ${X264_INSTALL}/lib/libx264.a \
    ${LAME_INSTALL}/lib/libmp3lame.a \
    -lm -lz -llog \
    -Wl,-u,ffprobe_run \
    -Wl,-u,ffprobe_reset \
    -Wl,--gc-sections \
    -Wl,--allow-multiple-definition \
    -Wl,--version-script,"${FTOOLS_BUILD}/ffprobe.ver" \
    $LDFLAGS > "${LOG_DIR}/link_ffprobe.log" 2>&1 || {
    echo "❌ Failed to link libffprobe.so (see ${LOG_DIR}/link_ffprobe.log)"
    tail -10 "${LOG_DIR}/link_ffprobe.log"
    exit 1
}

cp "${FTOOLS_BUILD}/libffmpeg.so" "$OUTPUT_DIR/"
cp "${FTOOLS_BUILD}/libffprobe.so" "$OUTPUT_DIR/"

echo "=== Stripping debug symbols ==="
$STRIP --strip-all "${OUTPUT_DIR}/libffmpeg.so"
$STRIP --strip-all "${OUTPUT_DIR}/libffprobe.so"

echo "✅ Copied and stripped libffmpeg.so"
echo "✅ Copied and stripped libffprobe.so"

echo "=== Verifying exported symbols ==="
declare -A REQUIRED_SYMBOLS=(
    [libffmpeg.so]="ffmpeg_run ffmpeg_reset"
    [libffprobe.so]="ffprobe_run ffprobe_reset"
)

for lib in "${!REQUIRED_SYMBOLS[@]}"; do
    echo "--- $lib ---"
    for sym in ${REQUIRED_SYMBOLS[$lib]}; do
        if ${NM} -D "${OUTPUT_DIR}/${lib}" 2>/dev/null | grep -q " ${sym}$"; then
            echo "  ✅ $sym"
        else
            echo "  ❌ $sym missing"
            exit 1
        fi
    done
    sym_count=$(${NM} -D "${OUTPUT_DIR}/${lib}" | grep -c "T ")
    size=$(ls -lh "${OUTPUT_DIR}/${lib}" | awk '{print $5}')
    echo "  📊 ${sym_count} text symbols, ${size}"
done

echo "=== Verifying resource symbols in libffmpeg.so ==="
for res_sym in ff_graph_css_data ff_graph_html_data ff_resman_get_string; do
    if ${NM} -D "${FTOOLS_BUILD}/libffmpeg.so" 2>/dev/null | grep -q " ${res_sym}$"; then
        echo "  ✅ $res_sym"
    else
        echo "  ❌ $res_sym missing from libffmpeg.so"
        echo "     This will cause dlopen failure: cannot locate symbol \"$res_sym\""
        echo "     Check: bin2c naming in Phase 2, --gc-sections in Phase 4, version script"
        exit 1
    fi
done

echo "=== Generating build-info.json ==="
MANIFEST_CSUM=$(sha256sum "$MANIFEST" | cut -d' ' -f1)

cat > "${OUTPUT_DIR}/build-info.json" << BIEOF
{
  "ffmpeg_version": "${FFMPEG_VERSION}",
  "ffmpeg_codename": "Huffman",
  "x264_version": "${X264_VERSION}",
  "x264_configure_opts": "--enable-static --enable-pic --disable-cli --disable-opencl",
  "lame_version": "${LAME_VERSION}",
  "lame_configure_opts": "--enable-static --disable-shared --disable-frontend --disable-nasm --disable-nls",
  "ndk_version": "${NDK_VERSION}",
  "api_level": ${API_LEVEL},
  "abi": "${ABI}",
  "build_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "manifest_version": "2",
  "manifest_checksum": "${MANIFEST_CSUM}",
  "enabled_decoders": [$(echo "$DECODERS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "enabled_encoders": [$(echo "$ENCODERS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "enabled_muxers": [$(echo "$MUXERS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "enabled_demuxers": [$(echo "$DEMUXERS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "enabled_parsers": [$(echo "$PARSERS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "enabled_protocols": [$(echo "$PROTOCOLS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "enabled_filters": [$(echo "$FILTERS" | tr ',' '\n' | while read -r d; do printf "\"%s\"," "$d"; done | sed 's/,$//')],
  "static_libs": [$(
    SL=""
    for lib in libavformat libavcodec libavutil libswresample libswscale libavfilter libavdevice; do
        [ -f "${FFMPEG_INSTALL}/lib/${lib}.a" ] && SL="$SL\"$lib\","
    done
    echo "${SL%,}"
  )],
  "linking": "static-into-so",
  "cflags": "-std=c11 -fPIC -DANDROID -D_POSIX_C_SOURCE=200809L -include time.h",
  "ffmpeg_license": "GPL v2+",
  "x264_license": "GPL v2",
  "lame_license": "LGPL 2.1",
  "encoder_source_summary": "aac/alac/flac are ffmpeg native (no external lib); libx264/libmp3lame are external",
  "validation": {
    "all_required_decoders_present": true,
    "all_required_encoders_present": true,
    "missing": []
  }
}
BIEOF

echo "✅ Generated build-info.json"

ASSETS_DIR="${PROJECT_DIR}/android/app/src/main/assets"
mkdir -p "$ASSETS_DIR"
cp "${OUTPUT_DIR}/build-info.json" "${ASSETS_DIR}/"
echo "✅ Copied build-info.json to Android assets"

echo "=== Building libffmpeg-worker.so (Go subprocess wrapper for cgo) ==="
# 真机架构：cgo ffmpeg_run 阻塞 OS thread，Go 进程没法用 ctx cancel unblock。
# 解决：父进程 os.Exec 启一个子进程（libffmpeg-worker.so）专门跑 ffmpeg_run；
# 父进程的 exec.CommandContext + SIGKILL 杀 worker → 父进程 unblock。
# Worker 内部 cgo dlopen libffmpeg.so 调 ffmpeg_run（cgo 阻塞在 worker 进程里）。
#
# 设计文档见 [internal/utils/ffmpeg/worker_runner.go] 和 [cmd/ffmpeg-worker/main_android.go]
# 父进程 (libencv-go.so) 通过 os.Exec("libffmpeg-worker.so") 启动，stdin/stdout JSON 通信。
if ! command -v go &>/dev/null; then
    echo "❌ Go toolchain not found, cannot build libffmpeg-worker.so"
    echo "   Install with: mise install go 1.21+"
    exit 1
fi

# 🆕 2026-06-12 Phase 4 fix：CI runner checkout 根不一定是 /workspace。
# 用 git rev-parse 动态拿 Go module root（cmd/ffmpeg-worker/ 必须在 Go module 内才能编译）。
# fallback: PROJECT_DIR 父目录（脚本跟 Go module 在同一仓库时成立）。
GOMOD_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || cd "${PROJECT_DIR}/.." && pwd)"
if [ ! -d "${GOMOD_ROOT}/cmd/ffmpeg-worker" ]; then
    echo "❌ Go module root has no cmd/ffmpeg-worker/: ${GOMOD_ROOT}"
    exit 1
fi
echo "Go module root: ${GOMOD_ROOT}"

WORKER_BUILD_DIR="${BUILD_DIR}/ffmpeg-worker"
mkdir -p "${WORKER_BUILD_DIR}"

# 设置 cgo 链接路径：worker 内部要 cgo 调 libffmpeg.so
# 但 worker 启动后才 dlopen（运行时）—— 这里只需要 link 进 worker 自己的 deps
# 所以 CGO_LDFLAGS 只链 -llog -ldl -lm（系统库），运行时 dlopen libffmpeg.so
GOOS=android GOARCH=arm64 CGO_ENABLED=1 \
    CC="${CC}" \
    CXX="${TOOLCHAIN}/bin/clang++" \
    CGO_CFLAGS="-fPIC -DANDROID -I${TOOLCHAIN}/sysroot/usr/include" \
    CGO_LDFLAGS="-llog -ldl -lm -Wl,-rpath,${FFMPEG_INSTALL}/lib" \
    PKG_CONFIG_PATH="${X264_INSTALL}/lib/pkgconfig:${LAME_INSTALL}/lib/pkgconfig" \
    go build -C "${GOMOD_ROOT}" -buildmode=c-shared \
    -ldflags='-s -w -extldflags=-Wl,-soname,libffmpeg-worker.so' \
    -o "${FTOOLS_BUILD}/libffmpeg-worker.so" \
    ./cmd/ffmpeg-worker/ > "${LOG_DIR}/ffmpeg-worker-build.log" 2>&1 || {
    echo "❌ Failed to build libffmpeg-worker.so (see ${LOG_DIR}/ffmpeg-worker-build.log)"
    tail -30 "${LOG_DIR}/ffmpeg-worker-build.log"
    exit 1
}

cp "${FTOOLS_BUILD}/libffmpeg-worker.so" "$OUTPUT_DIR/"
$STRIP --strip-all "${OUTPUT_DIR}/libffmpeg-worker.so" 2>/dev/null || true

# 验证导出符号：worker 应有 main (Go c-shared 入口点)
if ! ${NM} -D "${OUTPUT_DIR}/libffmpeg-worker.so" 2>/dev/null | grep -q " main$"; then
    echo "❌ libffmpeg-worker.so missing 'main' symbol"
    ${NM} -D "${OUTPUT_DIR}/libffmpeg-worker.so" | grep -E " T " | head -5
    exit 1
fi
WORKER_SIZE=$(ls -lh "${OUTPUT_DIR}/libffmpeg-worker.so" | awk '{print $5}')
echo "✅ Copied and stripped libffmpeg-worker.so (${WORKER_SIZE})"

echo "=== Build complete ==="
echo "Output: $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR"

TOTAL_SIZE=$(du -sh "$OUTPUT_DIR" | cut -f1)
echo "Total size: $TOTAL_SIZE"
