#!/usr/bin/env bash
# =============================================================================
# build-android-plugin-simverse.sh — 本地构建 SimVerse (ComboLite) 插件 APK
# -----------------------------------------------------------------------------
# 仅构建 plugin-simverse 这个独立插件 APK，镜像
# .github/workflows/android.yml 的 "Build SimVerse Plugin" 两步
# （lines ~1433-1483），适配在 .ide/Dockerfile 提供的环境里本地执行。
#
# 与 android.yml 的对应关系：
#   android.yml "Build SimVerse frontend"  (1434-1451)
#     ↔ 本脚本 Step 1：pnpm install + build web + 拷贝 dist → assets/simverse
#   android.yml "Build SimVerse plugin APK" (1453-1483)
#     ↔ 本脚本 Step 2：gradle compile + convert_plugin-simverse_<type>
#
# 说明：
#   - SimVerse 是纯 ComboLite 插件（Kotlin + 前端 web），**不需要** Go 二进制 /
#     libsql / objectbox / ffmpeg / openlist，因此本脚本不含这些步骤。
#   - 插件 APK 通过 -PincludePlugins=true 单独编译、磁盘加载，不入主 APK。
#   - NDK 路径取自 $ANDROID_NDK（.ide/Dockerfile 已设），回退到 $ANDROID_HOME/ndk/*。
#
# 用法：
#   bash scripts/build-android-plugin-simverse.sh            # Debug 插件 APK
#   bash scripts/build-android-plugin-simverse.sh --version 1.0.0   # Release 插件 APK
#   bash scripts/build-android-plugin-simverse.sh --skip-web  # 跳过前端构建（assets 已就绪）
#   bash scripts/build-android-plugin-simverse.sh -h
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
MONOREPO_MOBILE_DIR="app/encv-mobile"
# 共用路径解析 / android_cc 等 helper 统一走 android-common.sh，
# 与 .github/workflows/android.yml 同源，避免本地与 CI 维护冲突。
source "$SCRIPT_DIR/android-common.sh"
MOBILE_DIR="$ROOT_DIR/$MONOREPO_MOBILE_DIR"
ANDROID_DIR="$MOBILE_DIR/android"
SIMVERSE_WEB_DIR="$MOBILE_DIR/plugin-simverse/web"
SIMVERSE_ASSETS_DIR="$MOBILE_DIR/plugin-simverse/src/main/assets/simverse"

# run_gradle <label> <gradle-task> [extra-gradle-args...]
# 运行 Gradle：完整输出写入独立目录 android/build-logs/<slug>.log，
# 终端只显示进度与失败时的"关键错误行"（参考 app/scripts/check-all.mjs 的做法：
# 完整输出另存，终端仅呈现结构化提取的错误，避免刷屏）。
# 返回 Gradle 退出码。
run_gradle() {
  local label="$1"; shift
  local android_dir="$ROOT_DIR/$MONOREPO_MOBILE_DIR/android"
  local log_dir="$android_dir/build-logs"
  mkdir -p "$log_dir"
  local slug
  slug=$(printf '%s' "$label" | tr '[:upper:]' '[:lower:]' | tr -s ' .' '--' | tr -cd 'a-z0-9-')
  local log_file="$log_dir/${slug}.log"

  echo "  ▶ $label"
  echo "    完整输出 → $log_file"
  local start; start=$(date +%s)
  local rc=0
  ( cd "$android_dir" && chmod +x gradlew && ./gradlew "$@" --console=plain ) > "$log_file" 2>&1 || rc=$?
  local end; end=$(date +%s)
  echo "    ⏱ $((end-start))s  退出码=$rc"

  if [[ $rc -ne 0 ]]; then
    echo "  ❌ $label 失败 — 关键错误："
    grep -nE -e 'BUILD FAILED' \
             -e 'FAILURE:' \
             -e 'What went wrong:' \
             -e 'Execution failed for task' \
             -e '> Task .*FAILED' \
             -e '^e: .*error:' \
             -e 'error: ' \
             -e 'Caused by:' \
             -e 'Exception in thread' \
             -e 'Could not (GET|HEAD|resolve|download|find|create)' \
             -e 'Received status code' \
             "$log_file" 2>/dev/null | sed 's/^/    /' | tail -40 || true
    echo "  📄 完整日志: $log_file"
  else
    echo "  ✅ $label 完成"
  fi
  return $rc
}

# ---- 默认值 / 参数 ----
VERSION=""
SKIP_WEB=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --skip-web)
      SKIP_WEB=1
      shift
      ;;
    -h|--help)
      grep '^#' "$0" | grep -v '#!/' | sed 's/^#//' | sed 's/^ //'
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

BUILD_TYPE="debug"
APK_BUILD_TYPE="debug"
if [[ -n "$VERSION" ]]; then
  BUILD_TYPE="release"
  APK_BUILD_TYPE="release"
fi

# ---- 环境校验 ----
echo "══════════════════════════════════"
echo "  环境校验 (SimVerse 插件)"
echo "══════════════════════════════════"
command -v pnpm >/dev/null || { echo "❌ pnpm 未安装"; exit 1; }
command -v npx  >/dev/null || { echo "❌ npx 未安装"; exit 1; }
[[ -n "${ANDROID_HOME:-}" ]] || { echo "❌ ANDROID_HOME 未设置（请使用 .ide/Dockerfile 环境）"; exit 1; }
[[ -n "${ANDROID_NDK:-}" ]] || ANDROID_NDK="$(ls -d "${ANDROID_HOME}"/ndk/*/ 2>/dev/null | sort -V | tail -1)"
[[ -n "$ANDROID_NDK" ]] || { echo "❌ NDK 未安装"; exit 1; }
echo "  ANDROID_HOME=$ANDROID_HOME"
echo "  ANDROID_NDK=$ANDROID_NDK"
echo "  pnpm=$(pnpm --version)"
echo "  build type=$BUILD_TYPE${VERSION:+ (v$VERSION)}"
echo ""

# 插件 APK 必须用 keystore 签名：aar2apk 的 ConvertAarToApkTask 对所有变体
# （debug 与 release）都走 apksigner sign --ks <mobile>/keystore/release.jks
# （android/build.gradle.kts signing 块，ksPath 默认
# rootProject.file("../keystore/release.jks")，密码/别名固定 encv2025/encvrelease）。
# 注意：debug 插件 APK 也走这条签名路径（与主应用 assembleDebug 用 Android 默认
# debug keystore 不同）——日志实锤：convert_plugin-simverse_debug 失败正是
# 找不到 keystore/release.jks。因此 debug 与 release 都必须先生成 keystore。
# 主应用由 build-android.sh 调 ensure_android_keystore 生成同一份，本脚本复用。
echo "══════════════════════════════════"
echo "  准备插件签名 keystore (${BUILD_TYPE})"
echo "══════════════════════════════════"
ensure_android_keystore "$MONOREPO_MOBILE_DIR"
echo ""

# ---- Step 1: SimVerse 前端 web 构建 + 拷贝到插件 assets ----
if [[ $SKIP_WEB -eq 0 ]]; then
  echo "══════════════════════════════════"
  echo "  Step 1: SimVerse 前端 web 构建"
  echo "══════════════════════════════════"
  ( cd "$SIMVERSE_WEB_DIR" && pnpm install --prefer-offline && NODE_ENV=production pnpm build )
  echo "✅ SimVerse 前端构建完成"
  echo ""

  mkdir -p "$SIMVERSE_ASSETS_DIR"; rm -rf "$SIMVERSE_ASSETS_DIR"/*
  cp -r "$SIMVERSE_WEB_DIR/dist/." "$SIMVERSE_ASSETS_DIR"/
  echo "✅ 已拷贝到插件 assets: $SIMVERSE_ASSETS_DIR"
  ls -lh "$SIMVERSE_ASSETS_DIR/" | head -20
  echo ""
fi

# ---- Step 2: Gradle 编译 + convert 插件 APK ----
echo "══════════════════════════════════"
echo "  Step 2: Gradle $BUILD_TYPE (SimVerse 插件)"
echo "══════════════════════════════════"
run_gradle "plugin-simverse-compile-${BUILD_TYPE}" -PincludePlugins=true ":plugin-simverse:compile${BUILD_TYPE}Kotlin" --stacktrace
run_gradle "plugin-simverse-${APK_BUILD_TYPE}" -PincludePlugins=true "convert_plugin-simverse_${APK_BUILD_TYPE}" --stacktrace
echo ""

# ---- 定位产物 ----
SIMVERSE_APK=""
for variant in "$APK_BUILD_TYPE" release debug; do
  P="$ANDROID_DIR/build/outputs/plugin-apks/$variant/plugin-simverse-$variant.apk"
  if [[ -f "$P" ]]; then SIMVERSE_APK="$P"; break; fi
done
if [[ -z "$SIMVERSE_APK" ]]; then
  SIMVERSE_APK=$(find "$ROOT_DIR/$MONOREPO_MOBILE_DIR" -name "plugin-simverse-*.apk" -type f 2>/dev/null | head -1)
fi
[[ -n "$SIMVERSE_APK" && -f "$SIMVERSE_APK" ]] || { echo "❌ 未找到 SimVerse 插件 APK（详见 build-logs/ 日志）"; exit 1; }
echo "✅ SimVerse 插件 APK: $SIMVERSE_APK ($(ls -lh "$SIMVERSE_APK" | awk '{print $5}'))"

echo ""
echo "══════════════════════════════════"
echo "=== APK 内容检查 ==="
unzip -l "$SIMVERSE_APK" | head -20
echo ""
echo "=== SimVerse assets 检查 ==="
if unzip -l "$SIMVERSE_APK" | grep -q "simverse/index.html"; then
  echo "✅ simverse/index.html 已打进插件 APK"
else
  echo "⚠️  插件 APK 未含 simverse/index.html（前端 assets 可能未拷贝）"
fi

echo ""
echo "══════════════════════════════════"
echo "✅ SimVerse 插件构建完成"
echo "  插件 APK : $SIMVERSE_APK"
echo "══════════════════════════════════"
