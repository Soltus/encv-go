#!/usr/bin/env bash
# deps/x264.sh - 编译 x264 (H.264 encoder, GPLv2)
#
# 来源：https://code.videolan.org/videolan/x264.git (主仓)
# 镜像：https://github.com/mirror/x264 (GitHub 自动同步)
#
# 暴露：build_x264  （idempotent：检测到 libx264.a 跳过）
# 安装到：$DEPS_INSTALL_DIR/{include,lib,pkgconfig}
#
# 跨 host 通用：configure 自动识别 ./configure --host=<triple>

set -euo pipefail

# 兄弟库：每个 dep build_* 都需要 download_and_verify / find_source_root
# shellcheck source=./common.sh
_THIS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! type download_and_verify >/dev/null 2>&1; then
    # shellcheck source=../common/downloads.sh
    source "$_THIS_DIR/../common/downloads.sh"
fi
unset _THIS_DIR

# x264 配置（不写死版本：从 manifest 拿；目前 main branch stable）
X264_GIT_REF="${X264_GIT_REF:-stable}"
X264_PKG_NAME="x264"

build_x264() {
    log_section "build x264 (ref=$X264_GIT_REF)"

    local install_marker="${DEPS_INSTALL_DIR}/lib/libx264.a"
    if [ -f "$install_marker" ]; then
        log_ok "x264 already built ($install_marker), skipping"
        return 0
    fi

    # === 准备源码 ===
    local src_dir="${BUILD_DIR}/${X264_PKG_NAME}"
    if [ ! -d "$src_dir" ]; then
        log_info "cloning x264 (ref=$X264_GIT_REF)"
        # 优先 code.videolan.org（官方），失败回退 github mirror
        if ! git clone --depth 1 --branch "$X264_GIT_REF" \
            "https://code.videolan.org/videolan/x264.git" "$src_dir" \
            > "${LOG_DIR}/x264-clone.log" 2>&1; then
            log_warn "videolan clone failed, trying github mirror"
            rm -rf "$src_dir"
            git clone --depth 1 --branch "$X264_GIT_REF" \
                "https://github.com/mirror/x264.git" "$src_dir" \
                > "${LOG_DIR}/x264-clone.log" 2>&1 || \
                die "x264 clone failed (see ${LOG_DIR}/x264-clone.log)"
        fi
    fi

    cd "$src_dir"

    # === host triple（Android 需要，host 用本机即可） ===
    local host_triple=""
    if [ "${TARGET:-}" = "android" ]; then
        host_triple="${TARGET_ARCH}-linux-android"
    fi

    # === x264 configure 选项 ===
    # Android 关键点：
    #   --cross-prefix=llvm-  ← NDK 工具链统一前缀
    #   --extra-cflags 加 -DANDROID -fPIC
    # host 关键点：
    #   不传 --host，让 configure 自动探测
    local extra_cflags="${CFLAGS_CROSS:-} -DANDROID -fPIC"
    local extra_ldflags="-lm"

    # 走 ffmpeg-builder 的 run_logged 拿 tail 30 错误信息
    log_cmd "./configure (x264)"
    if ! CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
        ./configure \
            $([ -n "$host_triple" ] && echo "--host=$host_triple") \
            --prefix="$DEPS_INSTALL_DIR" \
            --enable-static \
            --enable-pic \
            --disable-cli \
            --disable-opencl \
            --cross-prefix="${TOOLCHAIN_BIN:+$TOOLCHAIN_BIN/llvm-}" \
            --extra-cflags="$extra_cflags" \
            --extra-ldflags="$extra_ldflags" \
            > "${LOG_DIR}/x264-configure.log" 2>&1; then
        log_error "x264 configure failed (see ${LOG_DIR}/x264-configure.log)"
        tail -30 "${LOG_DIR}/x264-configure.log" >&2 || true
        die "x264 configure failed"
    fi
    log_ok "x264 configured"

    run_logged "x264-make" make "-j$(nproc)"
    run_logged "x264-install" make install

    # === Android 特殊：修补 x264.pc 去掉 -lpthread -ldl ===
    if [ "${TARGET:-}" = "android" ]; then
        patch_x264_pc "${DEPS_INSTALL_DIR}/lib/pkgconfig/x264.pc"
    fi

    require_file "$install_marker" "x264 install incomplete (no libx264.a)"
    log_ok "x264 built and installed to $DEPS_INSTALL_DIR"
}
