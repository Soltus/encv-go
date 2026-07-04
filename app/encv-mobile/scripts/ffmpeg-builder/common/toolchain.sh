#!/usr/bin/env bash
# common/toolchain.sh - 编译器/打包工具探测 + pkg-config 跨编译包装
#
# 用法：
#   source common/toolchain.sh
#   cc_path="$(detect_tool cc gcc clang cc)"  # 找第一个可用的
#   make_pkgconfig_wrapper "$X264_INSTALL:$LAME_INSTALL"
#
# 设计：
#   - 不预设任何工具路径，全靠 command -v 探测
#   - pkg-config 包装器用于跨编译：把 x264/lame 的 pc 文件路径强制指向 install dir
#     （避免宿主机的 x264.pc 误导 ffmpeg configure）

set -euo pipefail

# 探测：依次尝试列表中命令，返回第一个存在路径
# 用法: detect_tool cc gcc clang cc
detect_tool() {
    local label="$1"
    shift
    local tool=""
    for candidate in "$@"; do
        if command -v "$candidate" >/dev/null 2>&1; then
            tool="$(command -v "$candidate")"
            echo "$tool"
            return 0
        fi
    done
    die "No $label found in: $*"
}

# 探测 NDK 工具链里的 LLVM 工具（带 host 前缀）
# 用法: detect_ndk_tool <prefix> <tool_name>
# 例: detect_ndk_tool "$TOOLCHAIN_BIN" clang → $TOOLCHAIN_BIN/clang
detect_ndk_tool() {
    local bin_dir="$1"
    local tool_name="$2"
    local path="${bin_dir}/${tool_name}"
    if [ ! -x "$path" ]; then
        die "NDK tool not found or not executable: $path"
    fi
    echo "$path"
}

# 验证一组必须存在的工具
# 用法: require_toolchain CC AR NM RANLIB STRIP
require_toolchain() {
    local missing=()
    local var
    for var in "$@"; do
        if [ -z "${!var:-}" ]; then
            missing+=("$var")
        elif [ ! -x "${!var}" ]; then
            missing+=("$var (not executable: ${!var})")
        fi
    done
    if [ ${#missing[@]} -gt 0 ]; then
        die "Missing toolchain variables: ${missing[*]}"
    fi
}

# 创建跨编译用的 pkg-config 包装器
# 用法: make_pkgconfig_wrapper <wrapper_path> <colon_separated_pc_dirs>
# 把所有 PC 路径强制指向 deps 的 install dir（避免 ffmpeg configure 拉到宿主机版本）
make_pkgconfig_wrapper() {
    local wrapper_path="$1"
    local pc_dirs="$2"
    local bin_dir
    bin_dir="$(dirname "$wrapper_path")"
    mkdir -p "$bin_dir"

    cat > "$wrapper_path" << PCEOF
#!/bin/bash
export PKG_CONFIG_PATH="${pc_dirs}"
export PKG_CONFIG_LIBDIR="${pc_dirs}"
export PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1
export PKG_CONFIG_ALLOW_SYSTEM_LIBS=1
exec pkg-config "\$@"
PCEOF
    chmod +x "$wrapper_path"
    log_info "pkg-config wrapper: $wrapper_path (PC dirs: $pc_dirs)"
}

# 修补 x264.pc：去掉 -lpthread -ldl（Android 链接时这些由 libc/libdl 隐式提供）
patch_x264_pc() {
    local pc_file="$1"
    if [ ! -f "$pc_file" ]; then
        return 0
    fi
    sed -i 's/-lpthread//g; s/-ldl//g' "$pc_file"
    log_info "patched x264.pc: removed -lpthread -ldl ($pc_file)"
}

# 探测 cflags 中是否需要 -fPIC（静态库要 PIC 才能被 .so 链接）
require_pic_cflags() {
    if [ "${REQUIRE_PIC:-1}" = "1" ] && ! echo "${CFLAGS_COMMON:-} ${CFLAGS_CROSS:-}" | grep -q -- "-fPIC"; then
        CFLAGS_CROSS="${CFLAGS_CROSS:-} -fPIC"
        log_warn "auto-added -fPIC to CFLAGS_CROSS (deps static libs need PIC)"
    fi
}
