#!/usr/bin/env bash
# ffmpeg.sh - ffmpeg 源码下载 + patch + configure + make
#
# 用法（被 Makefile 调用）：
#   source ffmpeg.sh
#   build_ffmpeg
#
# 前置条件（caller 已做）：
#   ffmpeg_builder_init
#   load_manifest
#   target_setup
#   build_all_deps
#
# 输出：
#   $FFMPEG_INSTALL_DIR/  (静态库 + headers)

set -euo pipefail

# === Patch：把 main() 改为 ffmpeg_run/ffprobe_run，并补 reset stub ===
# 必须在源码解压后做一次；幂等（grep 检测）
patch_ffmpeg_main() {
    local src_dir="$1"
    local ffmpeg_c="${src_dir}/fftools/ffmpeg.c"
    local ffprobe_c="${src_dir}/fftools/ffprobe.c"

    # ffmpeg.c
    sed -i 's/int main(/int ffmpeg_run(/' "$ffmpeg_c"
    sed -i 's/int main(void)/int ffmpeg_run(void)/' "$ffmpeg_c"
    sed -i 's/int wmain(/int ffmpeg_run(/' "$ffmpeg_c"
    if ! grep -q "void ffmpeg_reset" "$ffmpeg_c"; then
        cat >> "$ffmpeg_c" << 'PATCH'

void ffmpeg_reset(void) {
}
PATCH
    fi

    # ffprobe.c
    sed -i 's/int main(/int ffprobe_run(/' "$ffprobe_c"
    sed -i 's/int main(void)/int ffprobe_run(void)/' "$ffprobe_c"
    if ! grep -q "void ffprobe_reset" "$ffprobe_c"; then
        cat >> "$ffprobe_c" << 'PATCH'

void ffprobe_reset(void) {
}
PATCH
    fi
    log_ok "patched ffmpeg.c + ffprobe.c (main → ffmpeg_run/ffprobe_run)"
}

# === 准备 ffmpeg 源码（下载 + 解压 + patch） ===
prepare_ffmpeg_source() {
    local archive="${BUILD_DIR}/ffmpeg-${FFMPEG_VERSION}.tar.xz"
    if [ ! -f "$archive" ]; then
        download_and_verify \
            "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
            "$archive" \
            ""  # ffmpeg.org 直链 sha 随时间会变，暂不强制校验
    fi
    # 先解压再 patch（patch 在解压目录里 sed）
    extract_to "$archive" "$BUILD_DIR"

    local src_dir
    src_dir="$(find_source_root "$BUILD_DIR" "ffmpeg-${FFMPEG_VERSION}")"
    patch_ffmpeg_main "$src_dir"
    echo "$src_dir"
}

# === ffmpeg configure（target-agnostic，host/android 都走这里） ===
configure_ffmpeg() {
    local src_dir="$1"
    cd "$src_dir"

    local extra_cflags="${CFLAGS_CROSS:-} ${CFLAGS_COMMON:-} -DANDROID \
-I${DEPS_INSTALL_DIR}/include"
    [ "${TARGET:-}" = "host" ] && extra_cflags="${CFLAGS_COMMON:-}"

    local extra_ldflags="${LDFLAGS_COMMON:-} \
-L${DEPS_INSTALL_DIR}/lib"

    # pkg-config wrapper（跨编译时把 x264/lame 的 pc 文件路径固定）
    if [ -d "${DEPS_INSTALL_DIR}/lib/pkgconfig" ]; then
        make_pkgconfig_wrapper \
            "${BUILD_ROOT}/pkg-config-wrapper" \
            "${DEPS_INSTALL_DIR}/lib/pkgconfig"
    else
        log_warn "no ${DEPS_INSTALL_DIR}/lib/pkgconfig, using system pkg-config"
    fi

    log_cmd "./configure (ffmpeg)"
    if ! ./configure \
        --prefix="$FFMPEG_INSTALL_DIR" \
        $FFMPEG_CONFIGURE_EXTRA \
        --cc="$CC" \
        --ar="$AR" \
        --nm="$NM" \
        --ranlib="$RANLIB" \
        --strip="$STRIP" \
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
        --enable-filters \
        --enable-small \
        --enable-libx264 \
        --enable-libmp3lame \
        --enable-gpl \
        --enable-nonfree \
        --disable-resource-compression \
        $([ "${PKG_CONFIG:-}" ] && echo "--pkg-config=$PKG_CONFIG") \
        --extra-cflags="$extra_cflags" \
        --extra-ldflags="$extra_ldflags" \
        --extra-libs="-lm" \
        > "${LOG_DIR}/ffmpeg-configure.log" 2>&1; then
        log_error "ffmpeg configure failed (see ${LOG_DIR}/ffmpeg-configure.log)"
        tail -80 "$src_dir/ffbuild/config.log" >&2 || true
        die "ffmpeg configure failed"
    fi
    log_ok "ffmpeg configured"

}



# === 主入口 ===
build_ffmpeg() {
    log_section "build ffmpeg $FFMPEG_VERSION (target=$TARGET)"

    # 已安装则跳过（但先验证 anull 存在）
    if [ -f "${FFMPEG_INSTALL_DIR}/lib/libavcodec.a" ]; then
        # 验证 anull 符号存在，不存在则强制重新编译
        if ${NM} -g "${FFMPEG_INSTALL_DIR}/lib/libavfilter.a" 2>/dev/null | grep -q "ff_af_anull"; then
            log_ok "ffmpeg already installed and anull present, skipping"
            return 0
        else
            log_warn "已安装的 FFmpeg 缺少 anull，强制重新编译"
            rm -rf "${FFMPEG_INSTALL_DIR}"
        fi
    fi

    require_cmd make "Install build-essential / xcode-select --install"
    require_cmd nproc "coreutils required"

    local src_dir
    src_dir="$(prepare_ffmpeg_source)"

    configure_ffmpeg "$src_dir"
    run_logged "ffmpeg-make" make "-j$(nproc)"
    run_logged "ffmpeg-install" make install

    require_file "${FFMPEG_INSTALL_DIR}/lib/libavcodec.a" "ffmpeg install incomplete"
    log_ok "ffmpeg installed: $FFMPEG_INSTALL_DIR"
}
