#!/usr/bin/env bash
# targets/dispatcher.sh - TARGET → 具体 target 脚本路由
#
# 用法：source common/paths.sh; source targets/dispatcher.sh
#       ffmpeg_builder_init; target_setup
#
# 强制要求 caller 已 export TARGET=host|android|ios
# 任何 .sh 主链（ffmpeg.sh / postprocess.sh / worker.sh）只调 `target_setup`，
# 不知道当前是哪个 target → 跨 target 复用。

set -euo pipefail

# 兄弟库 toolchain.sh 提供 detect_tool / require_toolchain / make_pkgconfig_wrapper
# 所有 target_setup 都会用到，先 source
# shellcheck source=../common/toolchain.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../common/toolchain.sh"

target_setup() {
    case "${TARGET:-}" in
        host)    source "${BUILDER_DIR}/targets/host.sh";    target_host_setup    ;;
        android) source "${BUILDER_DIR}/targets/android.sh"; target_android_setup ;;
        ios)     source "${BUILDER_DIR}/targets/ios.sh";     target_ios_setup     ;;
        *) die "Unknown TARGET='${TARGET:-}' (supported: host|android|ios)" ;;
    esac

    # 必校验：DEPS_INSTALL_DIR / FFMPEG_INSTALL_DIR
    require_dir "$DEPS_INSTALL_DIR" "DEPS_INSTALL_DIR"
    require_dir "$FFMPEG_INSTALL_DIR" "FFMPEG_INSTALL_DIR"
    log_ok "target ${TARGET} setup done (abi=$TARGET_ABI arch=$TARGET_ARCH)"
}
