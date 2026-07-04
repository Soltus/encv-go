#!/usr/bin/env bash
# 交叉编译 LibSQL C 绑定为 Android 平台的静态/动态库
#
# 用法：
#   ./scripts/build-libsql-android.sh [--ndk <path>] [--api <level>] [--arch <arch>] [--version <tag>]
#
# 支持的架构：arm64-v8a, armeabi-v7a, x86_64
#
# 输出：pkg/libsql/libs/android_<arch>/libsql_experimental.a 和/或 .so
#
# 说明：
#   - 官方没有 Android 预编译库，必须从源码构建
#   - 构建的是 libsql 主仓库的 bindings/c crate（crate 名: sql-experimental）
#   - 产出为 staticlib (.a) + cdylib (.so)
#
# CI 友好：
#   - 成功退出码 0，失败退出码 1
#   - 产物路径通过最后一行 "OUTPUT: <path>" 输出

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_BASE="$ROOT_DIR/pkg/libsql/libs"

# 默认值
API_LEVEL=24
ARCHS=("arm64-v8a")
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

# 为指定 toolchain 安装 target（best-effort，失败不中断）
ensure_target() {
  local target="$1"
  local toolchain="$2"

  if [[ -n "$toolchain" ]] && [[ "$toolchain" != "none" ]]; then
    echo "     为 toolchain '$toolchain' 安装 target '$target'..."
    set +e
    local output
    output=$(rustup target add "$target" --toolchain "$toolchain" 2>&1)
    local rc=$?
    set -e
    echo "$output" | tail -5
    if [[ $rc -ne 0 ]]; then
      echo "     ⚠️  target 安装返回码: $rc (继续尝试)"
    fi
  else
    echo "     为默认 toolchain 安装 target '$target'..."
    set +e
    local output
    output=$(rustup target add "$target" 2>&1)
    local rc=$?
    set -e
    echo "$output" | tail -5
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
  export "CXX_${target//-/_}=${ndk_bin}/${cc_prefix}${api_level}-clang++"
  export "AR_${target//-/_}=${ndk_bin}/llvm-ar"
  export "CARGO_TARGET_${target_upper}_LINKER=${ndk_bin}/${cc_prefix}${api_level}-clang"

  echo "     CC_${target//-/_}=${ndk_bin}/${cc_prefix}${api_level}-clang"
  echo "     CXX_${target//-/_}=${ndk_bin}/${cc_prefix}${api_level}-clang++"

  local build_cmd="cargo build --target $target --release --lib"
  if [[ -n "$toolchain" ]] && [[ "$toolchain" != "none" ]]; then
    build_cmd="rustup run $toolchain cargo build --target $target --release --lib"
  fi

  echo "     编译命令: $build_cmd"
  echo "     编译中（输出末尾 80 行）..."
  set +e
  (cd "$crate_dir" && $build_cmd 2>&1) | tail -80
  local exit_code=${PIPESTATUS[0]}
  set -e

  return $exit_code
}

# ============================================================
# 主逻辑
# ============================================================

echo "═══ LibSQL Android 构建脚本 ═══"
echo "架构: ${ARCHS[*]}"
echo "API level: $API_LEVEL"
echo "libsql 版本: $LIBSQL_VERSION"
echo ""

# 检查 NDK
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

echo "Rust 版本: $(rustc --version)"
echo "Rustup 版本: $(rustup --version)"
echo "默认 toolchain: $(rustup default 2>&1 || echo unknown)"
echo ""

# 克隆 libsql 源码
LIBSQL_SRC_DIR=$(mktemp -d)
trap 'rm -rf "$LIBSQL_SRC_DIR"' EXIT

echo "==> 克隆 libsql 源码 ($LIBSQL_VERSION)..."
git clone --depth 1 --branch "$LIBSQL_VERSION" \
  https://github.com/tursodatabase/libsql.git "$LIBSQL_SRC_DIR" 2>&1 | tail -10

# 找 C API crate
CRATE_DIR=""
if [[ -f "$LIBSQL_SRC_DIR/bindings/c/Cargo.toml" ]]; then
  CRATE_DIR="$LIBSQL_SRC_DIR/bindings/c"
  echo "✅ 找到 C API crate: bindings/c"
else
  echo "❌ 无法找到 libsql C API crate (bindings/c/Cargo.toml)"
  exit 1
fi

# 检测 toolchain
TOOLCHAIN=$(detect_toolchain "$LIBSQL_SRC_DIR")
echo "检测到的 toolchain: ${TOOLCHAIN:-'(默认)'}"
echo ""

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

  # 检查是否已有产物
  if [[ -f "$out_dir/libsql_experimental.a" ]] || [[ -f "$out_dir/libsql_experimental.so" ]]; then
    echo "  ✅ 产物已存在，跳过构建"
    ls -lh "$out_dir/" | sed 's/^/     /'
    ARCH_SUCCESS=1
  fi

  if [[ $ARCH_SUCCESS -eq 0 ]]; then
    # 安装 target（双重保险：默认 + 指定 toolchain）
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

    # 打印编译前诊断信息
    echo ""
    echo "  编译前诊断:"
    echo "    crate 目录: $CRATE_DIR"
    echo "    目标 toolchain: ${TOOLCHAIN:-'(默认)'}"
    echo "    crate 内活跃 toolchain: $(cd "$CRATE_DIR" && rustup show active-toolchain 2>&1 | head -1 | awk '{print $1}')"
    echo "    已安装的 targets: $(rustup target list --installed 2>/dev/null | tr '\n' ', ')"
    if [[ -n "$TOOLCHAIN" ]] && [[ "$TOOLCHAIN" != "none" ]]; then
      echo "    toolchain '$TOOLCHAIN' 的 targets: $(rustup target list --installed --toolchain "$TOOLCHAIN" 2>/dev/null | tr '\n' ', ')"
    fi
    echo ""

    # 编译
    if build_with_cargo "$target" "$TOOLCHAIN" "$CRATE_DIR" "$NDK_BIN" "$cc_prefix" "$API_LEVEL"; then
      echo ""
      echo "  ✅ 编译成功"
    else
      echo ""
      echo "  ❌ 编译失败"
      continue
    fi

    # 查找输出（crate 名 sql-experimental → libsql_experimental.a/.so）
    # 注意：target 目录在 workspace 根目录（$LIBSQL_SRC_DIR），不在 crate 子目录
    BUILT_LIB=""
    for lib_name in "libsql_experimental.a" "libsql_experimental.so"; do
      found=$(find "$LIBSQL_SRC_DIR/target/$target/release" -name "$lib_name" -type f 2>/dev/null | head -1)
      if [[ -n "$found" ]]; then
        BUILT_LIB="$found"
        break
      fi
    done

    if [[ -z "$BUILT_LIB" ]]; then
      # 更广泛搜索
      BUILT_LIB=$(find "$LIBSQL_SRC_DIR/target/$target/release" -name "libsql*.a" -type f 2>/dev/null | head -1)
      if [[ -z "$BUILT_LIB" ]]; then
        BUILT_LIB=$(find "$LIBSQL_SRC_DIR/target/$target/release" -name "libsql*.so" -type f 2>/dev/null | head -1)
      fi
    fi

    if [[ -z "$BUILT_LIB" ]]; then
      # 兜底：也搜一下 crate 目录下的 target（以防不是 workspace）
      BUILT_LIB=$(find "$CRATE_DIR/target/$target/release" -name "libsql*" -type f 2>/dev/null | head -1)
    fi

    if [[ -n "$BUILT_LIB" ]]; then
      lib_basename=$(basename "$BUILT_LIB")
      cp "$BUILT_LIB" "$out_dir/$lib_basename"
      echo "  ✅ 输出:"
      ls -lh "$out_dir/" | sed 's/^/     /'
      ARCH_SUCCESS=1
    else
      echo "  ❌ 编译成功但未找到输出库"
      echo "     target/$target/release 目录内容:"
      ls -lh "$LIBSQL_SRC_DIR/target/$target/release/" 2>/dev/null | sed 's/^/       /' | head -30 || echo "       (目录不存在)"
    fi
  fi

  if [[ $ARCH_SUCCESS -eq 1 ]]; then
    SUCCESS_ARCHS+=("$arch")
  else
    OVERALL_FAILED=1
  fi
done

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
  if [[ ${#SUCCESS_ARCHS[@]} -gt 0 ]]; then
    echo "OUTPUT: $OUTPUT_BASE"
    exit 0
  else
    exit 1
  fi
fi
