#!/usr/bin/env bash
# 交叉编译 LibSQL 为 Android 平台的静态/动态库
#
# 用法：
#   ./scripts/build-libsql-android.sh [--ndk <path>] [--api <level>] [--arch <arch>] [--version <tag>]
#
# 支持的架构：arm64-v8a, armeabi-v7a, x86_64
#
# 输出：pkg/libsql/libs/android_<arch>/libsql_experimental.a 和/或 .so
#
# 策略：
#   1. 优先从 GitHub releases 下载预编译库
#   2. 下载失败则从源码编译
#   3. 全部失败则 exit 1（调用方决定是否降级）
#
# CI 友好：
#   - 成功退出码 0，失败退出码 1
#   - 重要信息打印到 stdout，详细日志也到 stdout
#   - 产物路径通过最后一行 "OUTPUT: <path>" 输出

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_BASE="$ROOT_DIR/pkg/libsql/libs"

# 默认值
API_LEVEL=24
ARCHS=("arm64-v8a")
LIBSQL_VERSION="${LIBSQL_VERSION:-latest}"
SKIP_DOWNLOAD=0
SKIP_BUILD=0

# 参数解析
while [[ $# -gt 0 ]]; do
  case "$1" in
    --ndk)
      ANDROID_NDK_HOME="$2"
      shift 2
      ;;
    --api)
      API_LEVEL="$2"
      shift 2
      ;;
    --arch)
      ARCHS=("$2")
      shift 2
      ;;
    --version)
      LIBSQL_VERSION="$2"
      shift 2
      ;;
    --skip-download)
      SKIP_DOWNLOAD=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    -h|--help)
      grep '^#' "$0" | grep -v '#!/' | sed 's/^#//' | sed 's/^ //'
      exit 0
      ;;
    *)
      echo "未知参数: $1"
      exit 1
      ;;
  esac
done

# Rust target 与 NDK 工具映射
declare -A RUST_TARGETS=(
  ["arm64-v8a"]="aarch64-linux-android"
  ["armeabi-v7a"]="armv7-linux-androideabi"
  ["x86_64"]="x86_64-linux-android"
)

declare -A CC_PREFIX=(
  ["arm64-v8a"]="aarch64-linux-android"
  ["armeabi-v7a"]="armv7a-linux-androideabi"
  ["x86_64"]="x86_64-linux-android"
)

declare -A OUTPUT_DIRS=(
  ["arm64-v8a"]="android_arm64"
  ["armeabi-v7a"]="android_armv7"
  ["x86_64"]="android_x86_64"
)

# ============================================================
# 辅助函数
# ============================================================

# 解析 rust-toolchain.toml 获取 toolchain 版本
detect_toolchain() {
  local src_dir="$1"
  local toolchain=""

  if [[ -f "$src_dir/rust-toolchain.toml" ]]; then
    toolchain=$(grep -E '^channel' "$src_dir/rust-toolchain.toml" 2>/dev/null \
      | head -1 | sed 's/.*=[[:space:]]*//' | tr -d '"' | tr -d "'" | xargs || echo "")
  fi
  if [[ -z "$toolchain" ]] && [[ -f "$src_dir/rust-toolchain" ]]; then
    toolchain=$(cat "$src_dir/rust-toolchain" | xargs)
  fi

  echo "$toolchain"
}

# 确保指定 toolchain 已安装 target
ensure_target() {
  local target="$1"
  local toolchain="$2"

  if [[ -n "$toolchain" ]] && [[ "$toolchain" != "none" ]]; then
    echo "     为 toolchain '$toolchain' 安装 target '$target'..."
    set +e
    rustup target add "$target" --toolchain "$toolchain" 2>&1 | tail -5
    local rc=${PIPESTATUS[0]}
    set -e
    if [[ $rc -ne 0 ]]; then
      echo "     ⚠️  target 安装返回码: $rc (继续尝试)"
    fi
  else
    echo "     为默认 toolchain 安装 target '$target'..."
    set +e
    rustup target add "$target" 2>&1 | tail -5
    local rc=${PIPESTATUS[0]}
    set -e
    if [[ $rc -ne 0 ]]; then
      echo "     ⚠️  target 安装返回码: $rc (继续尝试)"
    fi
  fi
}

# 验证 target 是否可用（用 rustc --print sysroot）
verify_target() {
  local target="$1"
  local toolchain="$2"

  local sysroot=""
  local exit_code=0

  if [[ -n "$toolchain" ]] && [[ "$toolchain" != "none" ]]; then
    sysroot=$(rustup run "$toolchain" rustc --target "$target" --print sysroot 2>&1) || exit_code=$?
  else
    sysroot=$(rustc --target "$target" --print sysroot 2>&1) || exit_code=$?
  fi

  if [[ $exit_code -eq 0 ]] && [[ -n "$sysroot" ]]; then
    echo "     ✅ target 验证通过: $sysroot"
    return 0
  else
    echo "     ❌ target 验证失败 (exit=$exit_code): $sysroot"
    return 1
  fi
}

# 用 cargo 编译（自动处理 toolchain）
build_with_cargo() {
  local target="$1"
  local toolchain="$2"
  local crate_dir="$3"
  local ndk_bin="$4"
  local cc_prefix="$5"
  local api_level="$6"

  local target_upper
  target_upper=$(echo "$target" | tr '[:lower:]' '[:upper:]' | tr '-' '_')

  export "CC_${target//-/_}=${ndk_bin}/${cc_prefix}${api_level}-clang"
  export "AR_${target//-/_}=${ndk_bin}/llvm-ar"
  export "CARGO_TARGET_${target_upper}_LINKER=${ndk_bin}/${cc_prefix}${api_level}-clang"

  echo "     CC_${target//-/_}=${ndk_bin}/${cc_prefix}${api_level}-clang"

  local build_cmd="cargo build --target $target --release --lib"
  if [[ -n "$toolchain" ]] && [[ "$toolchain" != "none" ]]; then
    build_cmd="rustup run $toolchain cargo build --target $target --release --lib"
  fi

  echo "     编译命令: $build_cmd"
  echo "     编译中（输出末尾 50 行）..."
  set +e
  (cd "$crate_dir" && $build_cmd 2>&1) | tail -50
  local exit_code=${PIPESTATUS[0]}
  set -e

  return $exit_code
}

# 从 GitHub releases 下载预编译 libsql
try_download_prebuilt() {
  local arch="$1"
  local out_dir="$2"
  local version="$3"

  echo "  -> 尝试下载预编译 libsql ($arch, $version)..."

  local tag=""
  if [[ "$version" == "latest" ]]; then
    tag=$(curl -sfL "https://api.github.com/repos/tursodatabase/libsql/releases/latest" 2>/dev/null \
      | jq -r '.tag_name' 2>/dev/null || echo "")
    if [[ -z "$tag" ]]; then
      echo "     ❌ 无法获取最新版本号"
      return 1
    fi
  else
    tag="$version"
  fi
  echo "     版本: $tag"

  # 获取 release assets 列表
  local assets_json
  assets_json=$(curl -sfL "https://api.github.com/repos/tursodatabase/libsql/releases/tags/$tag" 2>/dev/null || echo "[]")

  if [[ -z "$assets_json" ]] || [[ "$assets_json" == "[]" ]]; then
    echo "     ❌ 无法获取 release assets"
    return 1
  fi

  # 匹配 Android arm64 的 asset（各种命名模式）
  local asset_url=""
  asset_url=$(echo "$assets_json" | jq -r '.assets[] | select(.name | test("android.*arm64|arm64.*android|aarch64.*android|android.*aarch64"; "i")) | .browser_download_url' 2>/dev/null | head -1)

  if [[ -z "$asset_url" ]]; then
    echo "     ℹ️  未找到 $arch 的预编译产物"
    return 1
  fi

  echo "     下载: $asset_url"

  local tmp_dir
  tmp_dir=$(mktemp -d)
  local archive="$tmp_dir/libsql-archive"

  if ! curl -fSL -o "$archive" "$asset_url" 2>/dev/null; then
    echo "     ❌ 下载失败"
    rm -rf "$tmp_dir"
    return 1
  fi

  # 解压（支持 tar.gz / tar / zip）
  cd "$tmp_dir"
  if ! tar -xzf "$archive" 2>/dev/null && ! tar -xf "$archive" 2>/dev/null && ! unzip -o "$archive" 2>/dev/null; then
    echo "     ❌ 解压失败"
    cd /
    rm -rf "$tmp_dir"
    return 1
  fi

  # 查找 libsql_experimental 库
  local found_lib
  found_lib=$(find "$tmp_dir" -maxdepth 5 -name "libsql_experimental.*" -type f 2>/dev/null | head -1)
  if [[ -z "$found_lib" ]]; then
    found_lib=$(find "$tmp_dir" -maxdepth 5 -name "libsql*.a" -type f 2>/dev/null | head -1)
  fi

  if [[ -z "$found_lib" ]]; then
    echo "     ❌ 压缩包中未找到 libsql 库"
    echo "     压缩包内容:"
    find "$tmp_dir" -type f 2>/dev/null | head -20 | sed 's/^/       /'
    cd /
    rm -rf "$tmp_dir"
    return 1
  fi

  mkdir -p "$out_dir"
  cp "$found_lib" "$out_dir/libsql_experimental.a"
  if [[ "$found_lib" == *.so ]]; then
    cp "$found_lib" "$out_dir/libsql_experimental.so"
  fi

  echo "     ✅ 下载成功: $out_dir/"
  ls -lh "$out_dir/" | sed 's/^/       /'

  cd /
  rm -rf "$tmp_dir"
  return 0
}

# ============================================================
# 主逻辑
# ============================================================

echo "═══ LibSQL Android 构建脚本 ═══"
echo "架构: ${ARCHS[*]}"
echo "API level: $API_LEVEL"
echo "版本: $LIBSQL_VERSION"
echo ""

# 检查 NDK（源码编译才需要）
if [[ $SKIP_BUILD -eq 0 ]]; then
  if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
    if [[ -n "${ANDROID_HOME:-}" ]] && [[ -d "${ANDROID_HOME}/ndk" ]]; then
      ANDROID_NDK_HOME="$(ls -d "${ANDROID_HOME}"/ndk/*/ 2>/dev/null | sort -V | tail -1)"
      if [[ -z "$ANDROID_NDK_HOME" ]]; then
        echo "❌ 错误：请设置 ANDROID_NDK_HOME 环境变量，或使用 --ndk 参数"
        exit 1
      fi
      echo "自动检测到 NDK: $ANDROID_NDK_HOME"
    else
      echo "❌ 错误：请设置 ANDROID_NDK_HOME 环境变量，或使用 --ndk 参数"
      exit 1
    fi
  fi

  if [[ ! -d "$ANDROID_NDK_HOME" ]]; then
    echo "❌ 错误：NDK 目录不存在: $ANDROID_NDK_HOME"
    exit 1
  fi

  NDK_BIN="${ANDROID_NDK_HOME}/toolchains/llvm/prebuilt/linux-x86_64/bin"
  if [[ ! -d "$NDK_BIN" ]]; then
    echo "❌ 错误：NDK bin 目录不存在: $NDK_BIN"
    exit 1
  fi

  # 检查 Rust
  if ! command -v rustc &> /dev/null; then
    echo "❌ 错误：请先安装 Rust: https://rustup.rs/"
    exit 1
  fi
  if ! command -v rustup &> /dev/null; then
    echo "❌ 错误：rustup 不可用"
    exit 1
  fi
fi

OVERALL_FAILED=0
SUCCESS_ARCHS=()

for arch in "${ARCHS[@]}"; do
  target="${RUST_TARGETS[$arch]}"
  cc_prefix="${CC_PREFIX[$arch]}"
  out_dir="$OUTPUT_BASE/${OUTPUT_DIRS[$arch]}"
  mkdir -p "$out_dir"

  echo ""
  echo "────────────────────────────────"
  echo "架构: $arch"
  echo "Rust target: $target"
  echo "输出目录: $out_dir"
  echo "────────────────────────────────"

  ARCH_SUCCESS=0

  # 策略1：检查是否已有产物
  if [[ -f "$out_dir/libsql_experimental.a" ]] || [[ -f "$out_dir/libsql_experimental.so" ]]; then
    echo "  ✅ 产物已存在，跳过构建"
    ls -lh "$out_dir/" | sed 's/^/     /'
    ARCH_SUCCESS=1
  fi

  # 策略2：下载预编译
  if [[ $ARCH_SUCCESS -eq 0 ]] && [[ $SKIP_DOWNLOAD -eq 0 ]]; then
    if try_download_prebuilt "$arch" "$out_dir" "$LIBSQL_VERSION"; then
      ARCH_SUCCESS=1
    fi
  fi

  # 策略3：源码编译
  if [[ $ARCH_SUCCESS -eq 0 ]] && [[ $SKIP_BUILD -eq 0 ]]; then
    echo ""
    echo "  -> 从源码编译..."

    # 创建临时目录（每个架构独立 clone 太浪费，用全局的）
    if [[ -z "${LIBSQL_SRC_DIR:-}" ]]; then
      LIBSQL_SRC_DIR=$(mktemp -d)
      echo "     克隆 libsql 源码 ($LIBSQL_VERSION)..."
      if [[ "$LIBSQL_VERSION" == "latest" ]]; then
        git clone --depth 1 --branch main \
          https://github.com/tursodatabase/libsql.git "$LIBSQL_SRC_DIR" 2>&1 | tail -5
      else
        git clone --depth 1 --branch "$LIBSQL_VERSION" \
          https://github.com/tursodatabase/libsql.git "$LIBSQL_SRC_DIR" 2>&1 | tail -5
      fi
    fi

    # 找 C API crate
    CRATE_DIR=""
    if [[ -f "$LIBSQL_SRC_DIR/bindings/c/Cargo.toml" ]]; then
      CRATE_DIR="$LIBSQL_SRC_DIR/bindings/c"
      echo "     使用 crate: bindings/c"
    else
      echo "     ❌ 无法找到 libsql C API crate (bindings/c/Cargo.toml)"
      continue
    fi

    # 检测 toolchain
    TOOLCHAIN=$(detect_toolchain "$LIBSQL_SRC_DIR")
    echo "     检测到的 toolchain: ${TOOLCHAIN:-'(默认)'}"

    # 安装 target（先装默认的，再装指定 toolchain 的——双重保险）
    ensure_target "$target" ""
    if [[ -n "$TOOLCHAIN" ]] && [[ "$TOOLCHAIN" != "none" ]]; then
      ensure_target "$target" "$TOOLCHAIN"
    fi

    # 验证 target
    if ! verify_target "$target" "$TOOLCHAIN"; then
      echo "     ⚠️  target 验证失败，再次尝试安装..."
      rustup target add "$target" 2>&1 || true
      if [[ -n "$TOOLCHAIN" ]] && [[ "$TOOLCHAIN" != "none" ]]; then
        rustup target add "$target" --toolchain "$TOOLCHAIN" 2>&1 || true
      fi
      verify_target "$target" "$TOOLCHAIN" || echo "     ⚠️  仍未通过，继续尝试编译..."
    fi

    # 编译
    echo ""
    echo "     编译前确认:"
    echo "       crate 目录: $CRATE_DIR"
    echo "       目标 toolchain: ${TOOLCHAIN:-'(默认)'}"
    echo "       crate 内活跃 toolchain: $(cd "$CRATE_DIR" && rustup show active-toolchain 2>&1 | head -1 | awk '{print $1}')"
    echo ""
    if build_with_cargo "$target" "$TOOLCHAIN" "$CRATE_DIR" "$NDK_BIN" "$cc_prefix" "$API_LEVEL"; then
      echo ""
      echo "     ✅ 编译成功"
    else
      echo ""
      echo "     ❌ 编译失败"
      continue
    fi

    # 查找输出
    BUILT_LIB=""
    for lib_name in "libsql_experimental.a" "libsql_experimental.so" "libsql.a" "libsql.so"; do
      found=$(find "$CRATE_DIR/target/$target/release" -name "$lib_name" -type f 2>/dev/null | head -1)
      if [[ -n "$found" ]]; then
        BUILT_LIB="$found"
        break
      fi
    done

    if [[ -z "$BUILT_LIB" ]]; then
      BUILT_LIB=$(find "$CRATE_DIR/target/$target/release" -name "libsql*.a" -o -name "libsql*.so" -type f 2>/dev/null | head -1)
    fi

    if [[ -n "$BUILT_LIB" ]]; then
      cp "$BUILT_LIB" "$out_dir/libsql_experimental.a"
      if [[ "$BUILT_LIB" == *.so ]]; then
        cp "$BUILT_LIB" "$out_dir/libsql_experimental.so"
      fi
      echo "     ✅ 输出:"
      ls -lh "$out_dir/" | sed 's/^/       /'
      ARCH_SUCCESS=1
    else
      echo "     ❌ 编译成功但未找到输出库"
      echo "     target/$target/release 目录内容:"
      ls -lh "$CRATE_DIR/target/$target/release/" 2>/dev/null | sed 's/^/       /' || echo "       (目录不存在)"
    fi
  fi

  if [[ $ARCH_SUCCESS -eq 1 ]]; then
    SUCCESS_ARCHS+=("$arch")
  else
    OVERALL_FAILED=1
  fi
done

# 清理
if [[ -n "${LIBSQL_SRC_DIR:-}" ]]; then
  rm -rf "$LIBSQL_SRC_DIR"
fi

# 总结
echo ""
echo "════════════════════════════════"
if [[ $OVERALL_FAILED -eq 0 ]]; then
  echo "✅ 全部架构构建成功: ${SUCCESS_ARCHS[*]}"
  echo "输出目录: $OUTPUT_BASE"
  echo "OUTPUT: $OUTPUT_BASE"
  exit 0
else
  echo "⚠️  部分/全部架构构建失败"
  echo "成功: ${SUCCESS_ARCHS[*]:-无}"
  echo "失败的架构将使用 SQLite-only 模式"
  if [[ ${#SUCCESS_ARCHS[@]} -gt 0 ]]; then
    echo "OUTPUT: $OUTPUT_BASE"
    exit 0
  else
    exit 1
  fi
fi
