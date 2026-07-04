#!/usr/bin/env bash
# 从官方 ObjectBox Android AAR 中提取 JNI 库（含完整 C API）
#
# 用法：
#   ./scripts/build-objectbox-android.sh [--version <version>] [--arch <arch>]
#
# 支持的架构：arm64-v8a, armeabi-v7a, x86, x86_64
#
# 输出：pkg/tasksystem/store/objectbox/libs/android_<arch>/libobjectbox-jni.so
#
# 说明：
#   - ObjectBox C 核心库是闭源的，但官方 Android AAR 包含了完整 C API 的 .so
#   - libobjectbox-jni.so 同时导出 JNI API + C API (obx_*) + Dart API
#   - 保持原名 libobjectbox-jni.so（不改为 libobjectbox.so），原因：
#     (a) ObjectBox Go SDK (v1.9.0) 自身 CGO 写死 -lobjectbox-jni
#     (b) .so 的 ELF SONAME 字段也是 libobjectbox-jni.so（AAR 原始命名），
#         改名不会改变 SONAME，会导致 linker 按 SONAME 查找时找不到匹配的名字
#     (c) APK 只需 libobjectbox-jni.so 一个文件，三方（SDK / 本代码 / linker）统一
#   - 详见 pkg/tasksystem/store/objectbox/objectbox.go 中 LDFLAGS 注释
#
# CI 友好：
#   - 成功退出码 0，失败退出码 1
#   - 产物路径通过最后一行 "OUTPUT: <path>" 输出

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_BASE="$ROOT_DIR/pkg/tasksystem/store/objectbox/libs"

# 默认值
OBJECTBOX_ANDROID_VERSION="${OBJECTBOX_ANDROID_VERSION:-5.4.2}"
ARCHS=("arm64-v8a")

# 参数解析
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      OBJECTBOX_ANDROID_VERSION="$2"
      shift 2
      ;;
    --arch)
      ARCHS=("$2")
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

# 架构到输出目录映射
declare -A OUTPUT_DIRS=(
  ["arm64-v8a"]="android_arm64"
  ["armeabi-v7a"]="android_armv7"
  ["x86"]="android_x86"
  ["x86_64"]="android_x86_64"
)

# ============================================================
# 主逻辑
# ============================================================

echo "═══ ObjectBox Android 库提取脚本 ═══"
echo "架构: ${ARCHS[*]}"
echo "ObjectBox Android 版本: $OBJECTBOX_ANDROID_VERSION"
echo ""

# 下载 AAR
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

AAR_URL="https://repo1.maven.org/maven2/io/objectbox/objectbox-android-db/${OBJECTBOX_ANDROID_VERSION}/objectbox-android-db-${OBJECTBOX_ANDROID_VERSION}.aar"
AAR_FILE="$WORK_DIR/objectbox-android.aar"

echo "==> 下载 AAR..."
echo "  URL: $AAR_URL"

if command -v curl &> /dev/null; then
  curl --location --fail --output "$AAR_FILE" "$AAR_URL"
else
  wget --no-verbose --output-document="$AAR_FILE" "$AAR_URL"
fi

if [[ ! -s "$AAR_FILE" ]]; then
  echo "❌ 下载失败"
  exit 1
fi

echo "  大小: $(du -h "$AAR_FILE" | cut -f1)"
echo ""

OVERALL_FAILED=0
SUCCESS_ARCHS=()

for arch in "${ARCHS[@]}"; do
  out_dir="$OUTPUT_BASE/${OUTPUT_DIRS[$arch]}"
  mkdir -p "$out_dir"

  echo "────────────────────────────────"
  echo "架构: $arch"
  echo "输出目录: $out_dir"
  echo "────────────────────────────────"

  # 检查是否已有产物
  if [[ -f "$out_dir/libobjectbox-jni.so" ]]; then
    echo "  ✅ 产物已存在，跳过"
    ls -lh "$out_dir/" | sed 's/^/     /'
    SUCCESS_ARCHS+=("$arch")
    continue
  fi

  # 从 AAR 中提取 .so
  SO_PATH_IN_AAR="jni/$arch/libobjectbox-jni.so"

  echo "  提取 $SO_PATH_IN_AAR ..."
  if unzip -o -q "$AAR_FILE" "$SO_PATH_IN_AAR" -d "$WORK_DIR"; then
    # 保持原名 libobjectbox-jni.so（不改为 libobjectbox.so），
    # 参见文件头部注释及 objectbox.go LDFLAGS 注释。
    cp "$WORK_DIR/$SO_PATH_IN_AAR" "$out_dir/libobjectbox-jni.so"
    echo "  ✅ 提取成功"
    ls -lh "$out_dir/" | sed 's/^/     /'
    SUCCESS_ARCHS+=("$arch")
  else
    echo "  ❌ 提取失败：AAR 中没有 $SO_PATH_IN_AAR"
    OVERALL_FAILED=1
  fi
  echo ""
done

# 总结
echo "════════════════════════════════"
if [[ $OVERALL_FAILED -eq 0 ]]; then
  echo "✅ 全部架构提取成功: ${SUCCESS_ARCHS[*]}"
  echo "输出目录: $OUTPUT_BASE"
  echo "OUTPUT: $OUTPUT_BASE"
  exit 0
else
  echo "⚠️  部分/全部架构提取失败"
  echo "成功: ${SUCCESS_ARCHS[*]:-无}"
  if [[ ${#SUCCESS_ARCHS[@]} -gt 0 ]]; then
    echo "OUTPUT: $OUTPUT_BASE"
    exit 0
  else
    exit 1
  fi
fi
