#!/usr/bin/env bash
# targets/android.sh - Android target（NDK 跨编译）
#
# 用途：交叉编译 ffmpeg 给 Android arm64-v8a / x86_64 / armeabi-v7a
#
# 用法：
#   TARGET=android ANDROID_ABI=arm64-v8a ANDROID_API=24 target_setup
#
# 暴露（与 host.sh 同 schema，ffmpeg.sh 不需知道是 host 还是 android）：
#   TARGET_ABI / TARGET_OS=android / TARGET_ARCH
#   CC / CXX / AR / NM / RANLIB / STRIP / LD
#   SYSROOT / TOOLCHAIN_BIN
#   CFLAGS_COMMON / CFLAGS_CROSS / LDFLAGS_COMMON
#   PKG_CONFIG / PKG_CONFIG_PATH
#   DEPS_INSTALL_DIR / FFMPEG_INSTALL_DIR
#   FFMPEG_CONFIGURE_EXTRA
#   NEEDS_ANDROID_NDK = 1
#
# 关键点：
#   - NDK 路径 100% 从环境变量 / 自动探测拿（见 common/paths.sh）
#   - ABI/API 默认值：arm64-v8a + 24
#   - 不写死 NDK 版本号（自动选 $ANDROID_HOME/ndk/ 下的最新）

set -euo pipefail

target_android_setup() {
    log_section "target android: ${ANDROID_ABI:-arm64-v8a} api=${ANDROID_API:-24}"

    # === ABI → ARCH 映射（不依赖 uname，由 caller 决定 ABI） ===
    ANDROID_ABI="${ANDROID_ABI:-arm64-v8a}"
    ANDROID_API="${ANDROID_API:-24}"

    case "$ANDROID_ABI" in
        arm64-v8a)   TARGET_ARCH=aarch64 ;;
        x86_64)      TARGET_ARCH=x86_64 ;;
        armeabi-v7a) TARGET_ARCH=armv7 ;;
        x86)         TARGET_ARCH=x86 ;;
        *) die "Unsupported ANDROID_ABI: $ANDROID_ABI (supported: arm64-v8a, x86_64, armeabi-v7a, x86)" ;;
    esac
    TARGET_OS="android"
    TARGET_ABI="${ANDROID_ABI}"

    # === NDK 路径（绝不硬编码） ===
    NDK_PATH="$(detect_ndk)"
    if [ -z "$NDK_PATH" ] || [ ! -d "$NDK_PATH" ]; then
        die "Android NDK not found. Set ANDROID_NDK_HOME or install NDK via sdkmanager."
    fi
    log_info "NDK: $NDK_PATH"

    # === 选 host 平台子目录（linux-x86_64 / darwin-x86_64 / darwin-arm64 / windows-x86_64） ===
    local host_subdir=""
    case "$HOST_OS" in
        linux)   host_subdir="linux-x86_64" ;;
        darwin)
            if [ "$HOST_ARCH" = "aarch64" ]; then
                host_subdir="darwin-x86_64"  # NDK 至今没有 darwin-arm64 官方构建；Rosetta 跑
            else
                host_subdir="darwin-x86_64"
            fi
            ;;
        windows) host_subdir="windows-x86_64" ;;
        *)       die "Cannot determine NDK host subdir for: $HOST_OS-$HOST_ARCH" ;;
    esac

    TOOLCHAIN_BIN="${NDK_PATH}/toolchains/llvm/prebuilt/${host_subdir}/bin"
    SYSROOT="${NDK_PATH}/toolchains/llvm/prebuilt/${host_subdir}/sysroot"
    if [ ! -d "$TOOLCHAIN_BIN" ]; then
        die "NDK toolchain bin not found: $TOOLCHAIN_BIN (host_subdir=$host_subdir)"
    fi
    if [ ! -d "$SYSROOT" ]; then
        die "NDK sysroot not found: $SYSROOT"
    fi

    # === 编译器探测（NDK clang / llvm-ar / llvm-nm / ...） ===
    # NDK 26+ 用统一 clang，前缀是 aarch64-linux-android24-clang
    local triple_prefix="${TARGET_ARCH}-linux-android${ANDROID_API}"
    CC="$(detect_ndk_tool "$TOOLCHAIN_BIN" "${triple_prefix}-clang")"
    CXX="$(detect_ndk_tool "$TOOLCHAIN_BIN" "${triple_prefix}-clang++")"
    AR="$(detect_ndk_tool "$TOOLCHAIN_BIN" "llvm-ar")"
    NM="$(detect_ndk_tool "$TOOLCHAIN_BIN" "llvm-nm")"
    RANLIB="$(detect_ndk_tool "$TOOLCHAIN_BIN" "llvm-ranlib")"
    STRIP="$(detect_ndk_tool "$TOOLCHAIN_BIN" "llvm-strip")"
    LD="${CC}"  # Android 链接也是 clang driver

    # === pkg-config（cross-compile 模式用 wrapper） ===
    PKG_CONFIG="${BUILD_ROOT}/pkg-config-wrapper"
    PKG_CONFIG_PATH=""  # 由 make_pkgconfig_wrapper 在 deps 编译完后填
    mkdir -p "$BUILD_ROOT"

    # === cflags / ldflags ===
    # -DANDROID + sysroot 必备；-fPIC 给 deps 静态库用
    CFLAGS_COMMON="-fPIC -ffunction-sections -fdata-sections"
    CFLAGS_CROSS="-DANDROID --sysroot=${SYSROOT} -fPIC"
    LDFLAGS_COMMON="-Wl,--gc-sections -Wl,-rpath,'\$ORIGIN' -llog"

    # === ffmpeg configure 跨编译选项（host 不需要） ===
    FFMPEG_CONFIGURE_EXTRA="--enable-cross-compile \
--target-os=android \
--arch=${TARGET_ARCH} \
--cross-prefix=${TOOLCHAIN_BIN}/llvm- \
--sysroot=${SYSROOT}"

    DEPS_INSTALL_DIR="${BUILD_ROOT}/deps-install"
    FFMPEG_INSTALL_DIR="${BUILD_ROOT}/ffmpeg-install"
    NEEDS_ANDROID_NDK=1

    # 产物落点走单一权威函数 resolve_output_lib_dir：
    #   - OUT_DIR env 优先（caller 控制）
    #   - 否则默认 OUTPUT_DIR/${ANDROID_ABI}（<mobile>/build/ffmpeg/android-${HOST_ARCH}/out/${ABI}/）
    # Makefile show-output-dir 也调同一个函数，永远 100% 一致。
    OUTPUT_LIB_DIR="$(resolve_output_lib_dir)"
    if [ -n "${OUT_DIR:-}" ]; then
        log_info "OUT_DIR override: $OUTPUT_LIB_DIR (caller-controlled)"
    fi
    mkdir -p "$DEPS_INSTALL_DIR" "$FFMPEG_INSTALL_DIR" "$OUTPUT_LIB_DIR"

    require_toolchain CC CXX AR NM RANLIB STRIP LD
    log_info "android toolchain ready: $CC"
    log_info "  triple: $triple_prefix"
    log_info "  sysroot: $SYSROOT"
    log_info "  install dirs: deps=$DEPS_INSTALL_DIR ffmpeg=$FFMPEG_INSTALL_DIR"
}
