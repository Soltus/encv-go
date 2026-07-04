#!/usr/bin/env bash
# targets/host.sh - host 平台 target（直接用宿主 gcc/clang 编译）
#
# 用途：本地 sanity check / CI 烟雾测试，跑通整条链后才知道 Android 端有没有真问题。
#
# 用法（被 ffmpeg.sh 等主链脚本 source）：
#   TARGET=host target_setup
#   echo "$CC $TARGET_ABI"
#
# 暴露：
#   TARGET_ABI / TARGET_OS / TARGET_ARCH
#   CC / CXX / AR / NM / RANLIB / STRIP / LD
#   SYSROOT (空)
#   TOOLCHAIN_BIN (空)
#   CFLAGS_COMMON / CFLAGS_CROSS / LDFLAGS_COMMON
#   PKG_CONFIG / PKG_CONFIG_PATH
#   DEPS_INSTALL_DIR / FFMPEG_INSTALL_DIR
#   FFMPEG_CONFIGURE_EXTRA (无 --target-os 等)
#   NEEDS_ANDROID_NDK = 0

set -euo pipefail

# === 检测 GNU 用户空间 + glibc（macOS 需特殊处理） ===
target_host_setup() {
    log_section "target host: ${HOST_OS}-${HOST_ARCH}"

    TARGET_ABI="${HOST_ARCH}-${HOST_OS}"
    TARGET_OS="${HOST_OS}"
    TARGET_ARCH="${HOST_ARCH}"

    CC="$(detect_tool cc gcc clang cc)"
    CXX="$(detect_tool cxx g++ clang++ c++)"
    AR="$(detect_tool ar ar)"
    NM="$(detect_tool nm nm)"
    RANLIB="$(detect_tool ranlib ranlib)"
    STRIP="$(detect_tool strip strip)"
    LD="${CC}"   # host: cc 当 ld 用即可
    PKG_CONFIG="$(detect_tool pkg-config pkg-config)"
    PKG_CONFIG_PATH=""
    SYSROOT=""
    TOOLCHAIN_BIN=""

    # macOS: 强制用 clang（gcc 是 xcrun 假壳）
    if [ "$HOST_OS" = "darwin" ]; then
        # Apple Clang 不识别 -fno-canonical-prefixes 等 GNU 专属选项
        # 但 ffmpeg configure 自适应，留默认即可
        :
    fi

    CFLAGS_COMMON="-fPIC -ffunction-sections -fdata-sections"
    CFLAGS_CROSS=""
    LDFLAGS_COMMON="-Wl,--gc-sections -Wl,-rpath,'\$ORIGIN'"

    DEPS_INSTALL_DIR="${BUILD_ROOT}/deps-install"
    FFMPEG_INSTALL_DIR="${BUILD_ROOT}/ffmpeg-install"
    # 产物落点走单一权威函数 resolve_output_lib_dir（与 android target 共享）。
    OUTPUT_LIB_DIR="$(resolve_output_lib_dir)"
    if [ -n "${OUT_DIR:-}" ]; then
        log_info "OUT_DIR override: $OUTPUT_LIB_DIR (caller-controlled)"
    fi

    FFMPEG_CONFIGURE_EXTRA=""
    NEEDS_ANDROID_NDK=0

    mkdir -p "$DEPS_INSTALL_DIR" "$FFMPEG_INSTALL_DIR" "$OUTPUT_LIB_DIR"

    require_toolchain CC CXX AR NM RANLIB STRIP PKG_CONFIG
    log_info "host toolchain ready: cc=$CC ar=$AR pkg-config=$PKG_CONFIG"
    log_info "  install dirs: deps=$DEPS_INSTALL_DIR ffmpeg=$FFMPEG_INSTALL_DIR"
}
