#!/usr/bin/env bash
# deps/dispatcher.sh - 按 manifest external_libs 顺序构建所有 deps
#
# 用法（在 ffmpeg.sh 主链）：
#   source deps/dispatcher.sh
#   build_all_deps
#
# 强制要求：load_manifest 已执行（$EXTERNAL_LIBS 有值）
# 不引入新 deps：直接 return

set -euo pipefail

# 兄弟库 downloads.sh 提供 download_and_verify / extract_to / find_source_root
# 每个 dep build_* 都会用到，先 source
# shellcheck source=../common/downloads.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../common/downloads.sh"

# === 单 dep 入口：显式维护 dep_name → (script_basename, builder_function) ===
# 之前用 _dep_builders + ${dep}.sh 隐式约定 "dep name = script name = builder name"，
# 实际 x264 命名是脚本 x264.sh / 函数 build_x264 / manifest 名字 libx264 — 三者不一致。
# 现在改成显式 source + 显式调函数，零隐式约定。
#
# 加新 dep 步骤：
#   1) 写 deps/<script_basename>.sh，暴露 <builder_function>
#   2) manifest.external_libs 加 dep_name
#   3) 在 _dep_entry 加 case
_dep_entry() {
    local dep_name="$1"
    case "$dep_name" in
        libx264)
            # shellcheck source=deps/x264.sh
            source "${BUILDER_DIR}/deps/x264.sh"
            build_x264
            ;;
        libmp3lame)
            # shellcheck source=deps/libmp3lame.sh
            source "${BUILDER_DIR}/deps/libmp3lame.sh"
            build_libmp3lame
            ;;
        *)
            die "No entry for dep: $dep_name (add case in _dep_entry + write deps/<script>.sh)"
            ;;
    esac
}

build_all_deps() {
    log_section "build deps: $EXTERNAL_LIBS"
    require_cmd make "Install build-essential / xcode-select --install"
    require_cmd nproc "coreutils required"

    local dep
    for dep in $EXTERNAL_LIBS; do
        _dep_entry "$dep"
    done
    log_ok "all deps built: $EXTERNAL_LIBS"
}
