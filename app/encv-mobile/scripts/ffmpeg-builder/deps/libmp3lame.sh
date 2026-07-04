#!/usr/bin/env bash
# deps/libmp3lame.sh - 编译 libmp3lame (MP3 encoder, LGPL 2.1)
#
# 来源：https://sourceforge.net/projects/lame/files/lame/  (官方分发)
# 版本：3.100（MP3 编码专利 2017-04-16 到期，3.100+ 可商用）
#
# 暴露：build_libmp3lame
# 安装到：$DEPS_INSTALL_DIR/{include,lib,pkgconfig}
#
# 跨 host 通用：libmp3lame configure 自带 --host=<triple> 支持
#
# 为什么必须有外部 lame：
#   aac/alac/flac 都是 ffmpeg 内置 encoder（codec flag A....D）
#   MP3 ffmpeg 没有内置 encoder（专利原因），只有 libmp3lame / libshine 两种外部

set -euo pipefail

# 兄弟库：每个 dep build_* 都需要 download_and_verify / find_source_root
# shellcheck source=./common.sh
_THIS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! type download_and_verify >/dev/null 2>&1; then
    # shellcheck source=../common/downloads.sh
    source "$_THIS_DIR/../common/downloads.sh"
fi
unset _THIS_DIR

LAME_VERSION="${LAME_VERSION:-3.100}"
LAME_PKG_NAME="lame"

build_libmp3lame() {
    log_section "build libmp3lame (version=$LAME_VERSION)"

    local install_marker="${DEPS_INSTALL_DIR}/lib/libmp3lame.a"
    if [ -f "$install_marker" ]; then
        log_ok "libmp3lame already built ($install_marker), skipping"
        return 0
    fi

    # === 准备源码 ===
    local archive="${BUILD_DIR}/lame-${LAME_VERSION}.tar.gz"
    if [ ! -f "$archive" ]; then
        download_and_verify \
            "https://sourceforge.net/projects/lame/files/lame/${LAME_VERSION}/lame-${LAME_VERSION}.tar.gz/download" \
            "$archive" \
            ""  # SourceForge 直链 sha 不稳定，留空走 fallback 镜像
    fi

    # 先解压再 find（find_source_root 找的是解压后的目录）
    extract_to "$archive" "$BUILD_DIR"

    local src_dir
    src_dir="$(find_source_root "$BUILD_DIR" "lame*")"

    cd "$src_dir"

    # === host triple ===
    local host_triple=""
    if [ "${TARGET:-}" = "android" ]; then
        host_triple="${TARGET_ARCH}-linux-android"
    fi

    log_cmd "./configure (lame)"
    if ! CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
        ./configure \
            $([ -n "$host_triple" ] && echo "--host=$host_triple") \
            --prefix="$DEPS_INSTALL_DIR" \
            --enable-static \
            --disable-shared \
            --disable-frontend \
            --disable-nasm \
            --disable-nls \
            --disable-gtktest \
            --with-pic=yes \
            > "${LOG_DIR}/lame-configure.log" 2>&1; then
        log_error "lame configure failed (see ${LOG_DIR}/lame-configure.log)"
        tail -30 "${LOG_DIR}/lame-configure.log" >&2 || true
        die "lame configure failed"
    fi
    log_ok "lame configured"

    run_logged "lame-make" make "-j$(nproc)"
    run_logged "lame-install" make install

    require_file "$install_marker" "lame install incomplete (no libmp3lame.a)"
    log_ok "libmp3lame built and installed to $DEPS_INSTALL_DIR"
}
