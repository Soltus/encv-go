#!/usr/bin/env bash
# 交叉编译 LibSQL 为 Android 平台的静态库
#
# 用法：
#   ./scripts/build-libsql-android.sh [--ndk <path>] [--api <level>] [--arch <arch>]
#
# 支持的架构：arm64-v8a, armeabi-v7a, x86_64
#
# 输出：pkg/libsql/libs/android_<arch>/libsql_experimental.a
#
# 依赖：
#   - Android NDK r25+
#   - Rust 工具链（rustup）
#
# 环境变量：
#   ANDROID_NDK_HOME - NDK 根目录
#   LIBSQL_VERSION   - libsql 版本（默认：main）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_BASE="$ROOT_DIR/pkg/libsql/libs"

# 默认值
API_LEVEL=24
ARCHS=("arm64-v8a" "armeabi-v7a" "x86_64")
LIBSQL_VERSION="${LIBSQL_VERSION:-main}"

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

# 检查 NDK
if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
  if [[ -n "${ANDROID_HOME:-}" ]] && [[ -d "${ANDROID_HOME}/ndk" ]]; then
    # 自动查找最新的 NDK
    ANDROID_NDK_HOME="$(ls -d "${ANDROID_HOME}"/ndk/*/ 2>/dev/null | sort -V | tail -1)"
    if [[ -z "$ANDROID_NDK_HOME" ]]; then
      echo "错误：请设置 ANDROID_NDK_HOME 环境变量，或使用 --ndk 参数"
      exit 1
    fi
    echo "自动检测到 NDK: $ANDROID_NDK_HOME"
  else
    echo "错误：请设置 ANDROID_NDK_HOME 环境变量，或使用 --ndk 参数"
    exit 1
  fi
fi

if [[ ! -d "$ANDROID_NDK_HOME" ]]; then
  echo "错误：NDK 目录不存在: $ANDROID_NDK_HOME"
  exit 1
fi

NDK_BIN="${ANDROID_NDK_HOME}/toolchains/llvm/prebuilt/linux-x86_64/bin"
if [[ ! -d "$NDK_BIN" ]]; then
  echo "错误：NDK bin 目录不存在: $NDK_BIN"
  exit 1
fi

# 检查 Rust
if ! command -v rustc &> /dev/null; then
  echo "错误：请先安装 Rust: https://rustup.rs/"
  exit 1
fi

if ! command -v cargo &> /dev/null; then
  echo "错误：cargo 不可用"
  exit 1
fi

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

# 输出目录映射
declare -A OUTPUT_DIRS=(
  ["arm64-v8a"]="android_arm64"
  ["armeabi-v7a"]="android_armv7"
  ["x86_64"]="android_x86_64"
)

# 创建临时目录
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "==> 下载 libsql 源码 ($LIBSQL_VERSION)..."
cd "$TMP_DIR"
if [[ -d "libsql-src" ]]; then
  cd libsql-src
  git fetch --depth 1 origin "$LIBSQL_VERSION" 2>&1 | tail -3
  git checkout FETCH_HEAD 2>&1 | tail -3
else
  git clone --depth 1 --branch "$LIBSQL_VERSION" https://github.com/tursodatabase/libsql.git libsql-src 2>&1 | tail -10
  cd libsql-src
fi

echo "==> 编译 libsql for Android..."
BUILD_FAILED=0
for arch in "${ARCHS[@]}"; do
  target="${RUST_TARGETS[$arch]}"
  cc_prefix="${CC_PREFIX[$arch]}"
  out_dir="$OUTPUT_BASE/${OUTPUT_DIRS[$arch]}"

  echo ""
  echo "  -> $arch ($target, API $API_LEVEL)"

  # 添加 Rust target
  rustup target add "$target" 2>&1 | tail -3

  # 找到 libsql 的 C API crate
  CRATE_DIR=""
  for d in "libsql-sync" "crates/libsql-sync" "libsql" "crates/libsql" "bindings/c" "crates/bindings-c"; do
    if [[ -d "$d" ]] && [[ -f "$d/Cargo.toml" ]]; then
      CRATE_DIR="$d"
      break
    fi
  done

  if [[ -z "$CRATE_DIR" ]]; then
    echo "     ❌ 错误：无法找到 libsql C API crate"
    BUILD_FAILED=1
    continue
  fi
  echo "     使用 crate: $CRATE_DIR"

  # 设置 NDK 交叉编译环境变量（不用 cargo-ndk）
  export CC_${target//-/_}="${NDK_BIN}/${cc_prefix}${API_LEVEL}-clang"
  export AR_${target//-/_}="${NDK_BIN}/llvm-ar"
  export CARGO_TARGET_${target^^}_LINKER="${NDK_BIN}/${cc_prefix}${API_LEVEL}-clang"

  mkdir -p "$out_dir"

  # 编译
  echo "     编译中..."
  set +e
  (cd "$CRATE_DIR" && cargo build --target "$target" --release --lib 2>&1) | tail -20
  BUILD_EXIT=${PIPESTATUS[0]}
  set -e

  if [ $BUILD_EXIT -ne 0 ]; then
    echo "     ❌ 编译失败 (exit=$BUILD_EXIT)"
    BUILD_FAILED=1
    continue
  fi

  # 查找静态库（优先 .a）
  BUILT_LIB=""
  for lib_name in "libsql_experimental.a" "libsql_experimental.so" "libsql.a" "libsql.so"; do
    found=$(find "target/$target/release" -name "$lib_name" -type f 2>/dev/null | head -1)
    if [[ -n "$found" ]]; then
      BUILT_LIB="$found"
      break
    fi
  done

  if [[ -z "$BUILT_LIB" ]]; then
    # 更广泛的搜索
    BUILT_LIB=$(find "target/$target/release" -name "libsql*.a" -o -name "libsql*.so" -type f 2>/dev/null | head -1)
  fi

  if [[ -n "$BUILT_LIB" ]]; then
    cp "$BUILT_LIB" "$out_dir/libsql_experimental.a"
    # 如果是 .so，也保留一份
    if [[ "$BUILT_LIB" == *.so ]]; then
      cp "$BUILT_LIB" "$out_dir/libsql_experimental.so"
    fi
    echo "     ✅ 输出: $out_dir/$(basename "$BUILT_LIB")"
    ls -lh "$out_dir/"
  else
    echo "     ❌ 编译成功但未找到输出库"
    echo "     target/$target/release 目录内容:"
    ls -lh "target/$target/release/" 2>/dev/null || echo "     (目录不存在)"
    BUILD_FAILED=1
  fi
done

echo ""
if [ $BUILD_FAILED -ne 0 ]; then
  echo "==> ⚠️  部分架构编译失败"
  exit 1
else
  echo "==> ✅ 全部编译完成！"
  echo "输出目录: $OUTPUT_BASE"
  ls -lh "$OUTPUT_BASE"/android_* 2>/dev/null || true
fi
