#!/usr/bin/env bash
# =============================================================================
# build-android.sh — 本地构建 ENCV-go Android APK
# -----------------------------------------------------------------------------
# 镜像 .github/workflows/android.yml 的核心流程，适配在本仓库 + .ide/Dockerfile
# 提供的环境（Android SDK / NDK / JDK21 / Go 1.25.1 / pnpm）里本地执行。
#
# 与 android.yml 的对应关系：
#   Step 1  前端依赖安装 + pnpm build        ↔ workflow 的 Install deps / Build main app web
#   Step 2  原生库（best-effort）          ↔ workflow 的 libsql/objectbox/ffmpeg/mpv 准备
#   Step 3  Go 二进制（CGO + NDK 交叉编译） ↔ workflow 的 Build Go binary for Android
#   Step 4  npx cap sync android           ↔ workflow 的 Sync web assets + Capacitor plugins
#   Step 5  Gradle assemble{Debug,Release}  ↔ workflow 的 Build APK
#   Step 6  (opt-in) 插件 APK            ↔ workflow 的 Build * Plugin
#
# 用法：
#   bash scripts/build-android.sh                  # Debug APK（主应用，最快）
#   bash scripts/build-android.sh --version 1.0.0   # Release 签名 APK
#   bash scripts/build-android.sh --with-plugins  # 额外构建 openlist/mpv/simverse 插件 APK
#   bash scripts/build-android.sh --skip-native   # 跳过所有原生库（纯 SQLite，无 ffmpeg/mpv/向量搜索）
#   bash scripts/build-android.sh -h
#
# 说明：
#   - 原生库（libsql/objectbox/ffmpeg/mpv）为 best-effort：缺失时降级为
#     SQLite-only / 无 ffmpeg，主 APK 仍可构建安装（与 android.yml 的
#     「显式可观测、不静默降级」一致，会在终端打印 ⚠️ 警告）。
#   - 默认只构建主应用 APK（assembleDebug/Release）；插件需 --with-plugins。
#   - NDK 路径取自 $ANDROID_NDK（.ide/Dockerfile 已设），回退到 $ANDROID_HOME/ndk/*。
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
MONOREPO_MOBILE_DIR="app/encv-mobile"
# 共用逻辑（Go 交叉编译 / keystore 生成）统一走 android-common.sh，
# 与 .github/workflows/android.yml 同源，避免本地与 CI 维护冲突。
source "$SCRIPT_DIR/android-common.sh"
MOBILE_DIR="$ROOT_DIR/$MONOREPO_MOBILE_DIR"
ANDROID_DIR="$MOBILE_DIR/android"

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
WITH_PLUGINS=0
SKIP_NATIVE=0
SKIP_LIBSQL=0
SKIP_OBJECTBOX=0
SKIP_FFMPEG=0
SKIP_MPV_NATIVE=0
SKIP_WEB=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --with-plugins)
      WITH_PLUGINS=1
      shift
      ;;
    --skip-native)
      SKIP_NATIVE=1
      shift
      ;;
    --skip-libsql)
      SKIP_LIBSQL=1
      shift
      ;;
    --skip-objectbox)
      SKIP_OBJECTBOX=1
      shift
      ;;
    --skip-ffmpeg)
      SKIP_FFMPEG=1
      shift
      ;;
    --skip-mpv-native)
      SKIP_MPV_NATIVE=1
      shift
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
echo "  环境校验"
echo "══════════════════════════════════"
command -v go  >/dev/null || { echo "❌ go 未安装"; exit 1; }
command -v pnpm >/dev/null || { echo "❌ pnpm 未安装"; exit 1; }
command -v npx >/dev/null || { echo "❌ npx 未安装"; exit 1; }
[[ -n "${ANDROID_HOME:-}" ]] || { echo "❌ ANDROID_HOME 未设置（请使用 .ide/Dockerfile 环境）"; exit 1; }
[[ -n "${ANDROID_NDK:-}" ]] || ANDROID_NDK="$(ls -d "${ANDROID_HOME}"/ndk/*/ 2>/dev/null | sort -V | tail -1)"
[[ -n "$ANDROID_NDK" ]] || { echo "❌ NDK 未安装"; exit 1; }
echo "  ANDROID_HOME=$ANDROID_HOME"
echo "  ANDROID_NDK=$ANDROID_NDK"
echo "  go=$(go version | head -1)"
echo "  pnpm=$(pnpm --version)"
echo "  build type=$BUILD_TYPE${VERSION:+ (v$VERSION)}"
echo ""

LIBSQL_READY=0
OBJECTBOX_READY=0

# ---- Step 1: 前端依赖安装 + 构建 ----
if [[ $SKIP_WEB -eq 0 ]]; then
  echo "══════════════════════════════════"
  echo "  Step 1: 前端依赖安装 + 构建"
  echo "══════════════════════════════════"
  ( cd "$ROOT_DIR/app" && pnpm install )
  # 强制生产模式：云开发环境的 .cnb.yml 全局 NODE_ENV=development 会泄漏进
  # 本脚本，使 vite build 以开发模式产出（import.meta.env.DEV=true），
  # 导致 APK 的 getApiBaseUrl() 回退到 :16666 沙箱入口而非真机 :2025。
  # 仅在 build 这步覆盖 NODE_ENV，不影响上面的 pnpm install（仍需 devDeps）。
  ( cd "$MOBILE_DIR" && NODE_ENV=production pnpm build )
  echo "✅ 前端构建完成"
  echo ""
fi

# ---- Step 2: 原生库（best-effort） ----
if [[ $SKIP_NATIVE -eq 0 ]]; then
  echo "══════════════════════════════════"
  echo "  Step 2: 原生库准备（best-effort）"
  echo "══════════════════════════════════"

  # 2.1 libsql（CGO + NDK 交叉编译，需 Rust）
  if [[ $SKIP_LIBSQL -eq 0 ]]; then
    echo "--- libsql (CGO + NDK 交叉编译) ---"
    set +e
    bash "$SCRIPT_DIR/build-libsql-android.sh" --arch arm64-v8a --api 24 --ndk "$ANDROID_NDK" 2>&1 | tail -15
    rc=${PIPESTATUS[0]}
    set -e
    if [[ $rc -eq 0 ]] && [[ -f "$ROOT_DIR/pkg/libsql/libs/android_arm64/libsql_experimental.so" || -f "$ROOT_DIR/pkg/libsql/libs/android_arm64/libsql_experimental.a" ]]; then
      LIBSQL_READY=1; echo "✅ libsql 就绪"
    else
      echo "⚠️  libsql 构建失败/跳过 → 降级 SQLite-only（无向量搜索）"
    fi
  fi

  # 2.2 objectbox（从 Maven AAR 提取 JNI）
  if [[ $SKIP_OBJECTBOX -eq 0 ]]; then
    echo "--- objectbox (从 Maven AAR 提取 JNI) ---"
    set +e
    bash "$SCRIPT_DIR/build-objectbox-android.sh" --arch arm64-v8a 2>&1 | tail -15
    rc=${PIPESTATUS[0]}
    set -e
    if [[ $rc -eq 0 ]] && [[ -f "$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/android_arm64/libobjectbox-jni.so" ]]; then
      OBJECTBOX_READY=1; echo "✅ objectbox 就绪"
    else
      echo "⚠️  objectbox 提取失败/跳过 → 降级 SQLite"
    fi
  fi

  # 2.3 ffmpeg（从 GitHub release 预编产物下载）
  if [[ $SKIP_FFMPEG -eq 0 ]]; then
    echo "--- ffmpeg (下载预编产物 ffmpeg-native-libs) ---"
    JNI_BASE="$ANDROID_DIR/app/src/main/jniLibs/arm64-v8a"
    mkdir -p "$JNI_BASE"
    set +e
    URL=$(curl -sf "https://api.github.com/repos/${GITHUB_REPOSITORY:-Soltus/encv-go}/releases/tags/ffmpeg-native-libs" \
      | jq -r '.assets[] | select(.name | startswith("ffmpeg-jniLibs-arm64")) | .browser_download_url' 2>/dev/null)
    if [[ -n "$URL" ]]; then
      curl -fSL -o /tmp/ffmpeg-jniLibs.zip "$URL" \
        && unzip -o /tmp/ffmpeg-jniLibs.zip -d "$JNI_BASE/" >/dev/null 2>&1 \
        && echo "✅ ffmpeg 原生库已下载"
    else
      echo "⚠️ 未找到 ffmpeg-native-libs release（跳过，运行时无 ffmpeg）"
    fi
    set -e
  fi

  # 2.4 mpv native（从 GitHub release 预编产物下载）
  if [[ $SKIP_MPV_NATIVE -eq 0 ]]; then
    echo "--- mpv native (下载预编产物 mpv-native-libs) ---"
    MPV_JNI="$MOBILE_DIR/plugin-mpv-player/src/main/jniLibs/arm64-v8a"
    mkdir -p "$MPV_JNI"
    set +e
    URL=$(curl -sf "https://api.github.com/repos/${GITHUB_REPOSITORY:-Soltus/encv-go}/releases/tags/mpv-native-libs" \
      | jq -r '.assets[] | select(.name | startswith("mpv-jniLibs-arm64")) | .browser_download_url' 2>/dev/null)
    if [[ -n "$URL" ]]; then
      curl -fSL -o /tmp/mpv-jniLibs.zip "$URL" \
        && ( cd "$MPV_JNI/.." && unzip -o /tmp/mpv-jniLibs.zip -d jniLibs/ >/dev/null 2>&1 ) \
        && echo "✅ mpv 原生库已下载"
    else
      echo "⚠️ 未找到 mpv-native-libs release（跳过，插件 APK 将缺 mpv .so）"
    fi
    set -e
  fi
  echo ""
fi

# ---- Step 3: Go 二进制（CGO + NDK 交叉编译） ----
echo "══════════════════════════════════"
echo "  Step 3: Go 二进制 (libgojni.so)"
echo "══════════════════════════════════"
build_go_binary "${VERSION:-dev}"
echo ""

# ---- Step 4: Capacitor sync ----
echo "══════════════════════════════════"
echo "  Step 4: npx cap sync android"
echo "══════════════════════════════════"
cd "$MOBILE_DIR"
npx cap sync android
echo "✅ cap sync 完成"
echo ""

# ---- Step 5: Gradle 构建（完整输出写入 build-logs/，终端仅显示关键错误）----
echo "══════════════════════════════════"
echo "  Step 5: Gradle assemble${BUILD_TYPE^}"
echo "══════════════════════════════════"
if [[ -n "$VERSION" ]]; then
  echo "=== Building RELEASE APK v$VERSION ==="
  ensure_android_keystore "$MONOREPO_MOBILE_DIR"
  run_gradle "assemble-release-v$VERSION" assembleRelease --stacktrace
  APK=$(find "$ANDROID_DIR/app/build/outputs/apk/release" -name "*.apk" ! -name "*-unsigned*" -type f 2>/dev/null | head -1)
else
  run_gradle "assemble-debug" assembleDebug --stacktrace
  APK=$(find "$ANDROID_DIR/app/build/outputs/apk/debug" -name "*.apk" -type f 2>/dev/null | head -1)
fi
[[ -n "$APK" && -f "$APK" ]] || { echo "❌ 未找到 APK（详见 build-logs/ 日志）"; exit 1; }
echo "✅ APK: $APK ($(ls -lh "$APK" | awk '{print $5}'))"
echo ""

# ---- Step 6 (opt-in): 插件 APK ----
if [[ $WITH_PLUGINS -eq 1 ]]; then
  echo "══════════════════════════════════"
  echo "  Step 6: 插件 APK (--with-plugins)"
  echo "══════════════════════════════════"

  # openlist 前端
  ( cd "$MOBILE_DIR/plugin-openlist/web" && pnpm install --prefer-offline && NODE_ENV=production pnpm build )
  OL_ASSETS="$ROOT_DIR/$MONOREPO_MOBILE_DIR/plugin-openlist/src/main/assets/openlist"
  mkdir -p "$OL_ASSETS"; rm -rf "$OL_ASSETS"/*
  cp -r "$MOBILE_DIR/plugin-openlist/web/dist/." "$OL_ASSETS"/

  # openlist 原生（Go 交叉编译 fork）
  set +e
  if [[ -d "$ROOT_DIR/app/openlist/Hi-Sillot-OpenList" ]]; then
    ( cd "$ROOT_DIR/app/openlist/Hi-Sillot-OpenList" && CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC="$(android_cc)" go build -o /tmp/libopenlist-arm64.so ./ )
    mkdir -p "$MOBILE_DIR/plugin-openlist/src/main/jniLibs/arm64-v8a"
    cp /tmp/libopenlist-arm64.so "$MOBILE_DIR/plugin-openlist/src/main/jniLibs/arm64-v8a/libopenlist.so"
    echo "✅ libopenlist.so 已构建"
  else
    echo "⚠️ 未检出 Hi-Sillot/OpenList fork，跳过 libopenlist.so"
  fi
  set -e

  # mpv 前端
  ( cd "$MOBILE_DIR/plugin-mpv-player/web" && pnpm install --prefer-offline && NODE_ENV=production pnpm build )
  MPV_ASSETS="$ROOT_DIR/$MONOREPO_MOBILE_DIR/plugin-mpv-player/src/main/assets/mpv"
  mkdir -p "$MPV_ASSETS"; rm -rf "$MPV_ASSETS"/*
  cp -r "$MOBILE_DIR/plugin-mpv-player/web/dist/." "$MPV_ASSETS"/

  # simverse 前端
  ( cd "$MOBILE_DIR/plugin-simverse/web" && pnpm install --prefer-offline && NODE_ENV=production pnpm build )
  SV_ASSETS="$ROOT_DIR/$MONOREPO_MOBILE_DIR/plugin-simverse/src/main/assets/simverse"
  mkdir -p "$SV_ASSETS"; rm -rf "$SV_ASSETS"/*
  cp -r "$MOBILE_DIR/plugin-simverse/web/dist/." "$SV_ASSETS"/

  # Gradle convert（best-effort，失败不阻断主 APK）
  run_gradle "plugin-openlist-${APK_BUILD_TYPE}" -PincludePlugins=true "convert_plugin-openlist_${APK_BUILD_TYPE}" --stacktrace || echo "⚠️ openlist 插件构建失败（继续）"
  run_gradle "plugin-mpv-player-${APK_BUILD_TYPE}" -PincludePlugins=true "convert_plugin-mpv-player_${APK_BUILD_TYPE}" --stacktrace || echo "⚠️ mpv 插件构建失败（继续）"
  run_gradle "plugin-simverse-${APK_BUILD_TYPE}" -PincludePlugins=true "convert_plugin-simverse_${APK_BUILD_TYPE}" --stacktrace || echo "⚠️ simverse 插件构建失败（继续）"
  echo "✅ 插件 APK 构建完成（见 build/outputs/plugin-apks/）"
  echo ""
fi

echo "══════════════════════════════════"
echo "✅ 构建完成"
echo "  主 APK : $APK"
echo "  原生库 : libsql=$([[ $LIBSQL_READY -eq 1 ]] && echo ON || echo OFF)  objectbox=$([[ $OBJECTBOX_READY -eq 1 ]] && echo ON || echo OFF)"
echo "══════════════════════════════════"
