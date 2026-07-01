#!/usr/bin/env bash
# 交叉编译 LibSQL 为 Android 平台的动态库
#
# 用法：
#   ./scripts/build-libsql-android.sh [--ndk <path>] [--api <level>] [--arch <arch>]
#
# 支持的架构：arm64-v8a, armeabi-v7a, x86_64
#
# 输出：pkg/libsql/libs/android_<arch>/libsql_experimental.so
#
# 依赖：
#   - Android NDK r25+
#   - Rust 工具链（rustup）
#   - cargo-ndk（cargo install cargo-ndk）
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
  echo "错误：请设置 ANDROID_NDK_HOME 环境变量，或使用 --ndk 参数"
  exit 1
fi

if [[ ! -d "$ANDROID_NDK_HOME" ]]; then
  echo "错误：NDK 目录不存在: $ANDROID_NDK_HOME"
  exit 1
fi

# 检查 cargo-ndk
if ! command -v cargo-ndk &> /dev/null; then
  echo "错误：请先安装 cargo-ndk：cargo install cargo-ndk"
  exit 1
fi

# Rust target 映射
declare -A RUST_TARGETS=(
  ["arm64-v8a"]="aarch64-linux-android"
  ["armeabi-v7a"]="armv7-linux-androideabi"
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

echo "==> 下载 libsql 源码..."
cd "$TMP_DIR"
git clone --depth 1 --branch "$LIBSQL_VERSION" https://github.com/tursodatabase/libsql.git
cd libsql

echo "==> 安装 Rust targets..."
for arch in "${ARCHS[@]}"; do
  target="${RUST_TARGETS[$arch]}"
  rustup target add "$target"
done

echo "==> 编译 libsql..."
for arch in "${ARCHS[@]}"; do
  target="${RUST_TARGETS[$arch]}"
  out_dir="$OUTPUT_BASE/${OUTPUT_DIRS[$arch]}"

  echo "  -> $arch ($target)"
  mkdir -p "$out_dir"

  # 使用 cargo-ndk 交叉编译
  # 编译 libsql 的 C API 部分（libsql_experimental）
  # 注意：实际编译命令可能需要根据 libsql 的源码结构调整
  cd "$TMP_DIR/libsql"

  # 如果 libsql 有对应的 crate，用 cargo ndk build
  # 这里假设 libsql 的 cbindings 在 crates/libsql-c 或类似位置
  if [[ -d "libsql-c" ]]; then
    cd "libsql-c"
  elif [[ -d "crates/libsql-c" ]]; then
    cd "crates/libsql-c"
  elif [[ -d "libsql-sync" ]]; then
    cd "libsql-sync"
  fi

  cargo ndk -t "$target" -p "$API_LEVEL" build --release

  # 找到生成的 .so 文件并复制
  so_file="$(find "$TMP_DIR/libsql/target/$target/release" -name "libsql_experimental.so" -o -name "libsql.so" | head -1)"
  if [[ -n "$so_file" ]]; then
    cp "$so_file" "$out_dir/libsql_experimental.so"
    echo "     输出: $out_dir/libsql_experimental.so"
  else
    echo "     警告：未找到生成的 .so 文件"
    echo "     请检查 libsql 源码结构，调整编译脚本"
  fi
done

echo "==> 完成！"
echo "输出目录: $OUTPUT_BASE"
