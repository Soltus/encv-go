#!/usr/bin/env bash
# =============================================================================
# build-plugin-openlist-web.sh
# -----------------------------------------------------------------------------
# 一键构建 plugin-openlist/web 并把 dist 同步到 plugin-openlist/src/main/assets/openlist/
#
# 为什么需要这个脚本：
#   1. plugin-openlist 的 Android WebView 通过 file:///android_asset/openlist/ 加载 UI
#   2. Android assets 必须在 APK 打包前就位（编译期资源）
#   3. Vite 默认 base: '/' 在 file:// 协议下 404 → vite.config.ts 已设 base: './'
#   4. CI 不能依赖 dev server → 必须预构建
#
# 用法：
#   bash scripts/build-plugin-openlist-web.sh           # 默认：生产构建（混淆+压缩）
#   bash scripts/build-plugin-openlist-web.sh --dev     # 开发构建（未压缩，含 sourcemap）
#
# ⚠️ 重要：Vite 8 不支持 --prod 参数！
#   - Webpack 时代遗留 `--prod` 在 Vite 里是「Unknown option」
#   - Vite 8 的正确方式：默认 `vite build` 即生产（NODE_ENV=production）
#   - 开发模式用 `vite build --mode development`
#   - 详见：https://vite.dev/guide/cli.html#vite-build
#
# 配套：
#   - 沙箱开发：bash scripts/dev-openlist-web.sh         （Vite HMR）
#   - 真机构建：本脚本 + ./gradlew :plugin-openlist:assembleDebug
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MOBILE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_DIR="${MOBILE_DIR}/plugin-openlist/web"
ASSETS_DIR="${MOBILE_DIR}/plugin-openlist/src/main/assets/openlist"

# ---- 解析参数 ----
# 默认生产（vite build 本身即生产）；--dev 切开发模式（--mode development）
BUILD_MODE="production"  # 内部语义，不是 vite 参数
VITE_ARGS=()

case "${1:-}" in
  --dev)
    BUILD_MODE="development"
    VITE_ARGS=(--mode development)
    echo "==> 开发构建（含 sourcemap、未压缩，便于调试）"
    ;;
  ""|--prod)
    # 默认：Vite build 自带 NODE_ENV=production，无需额外参数
    # 显式 --prod 也可接受（兼容旧调用），但实际不传给 vite
    echo "==> 生产构建（默认：混淆 + 压缩）"
    ;;
  *)
    echo "❌ 未知参数: $1"
    echo ""
    echo "用法："
    echo "  bash $0            # 生产构建（默认）"
    echo "  bash $0 --dev      # 开发构建"
    exit 1
    ;;
esac

step() { echo ""; echo "==> $*"; }

# ---- Step 1: 确保 monorepo 安装 ----
step "1/4 pnpm install（workspace 依赖）"
cd "${MOBILE_DIR}"
pnpm install --no-frozen-lockfile --silent
# ---- Step 2: 构建 plugin web ----
step "2/4 pnpm exec vite build ${VITE_ARGS[*]:-}（Vite 构建 plugin-openlist/web，mode=${BUILD_MODE}）"
cd "${WEB_DIR}"
pnpm exec vite build "${VITE_ARGS[@]}" --logLevel warn

if [[ ! -d "dist" ]]; then
  echo "    ❌ dist 目录未生成"
  exit 1
fi

# ---- Step 3: 校验产物结构 ----
step "3/4 校验构建产物"
if [[ ! -f "dist/index.html" ]]; then
  echo "    ❌ dist/index.html 缺失"
  exit 1
fi

# 校验 base: './' 是否生效（file:// 加载必须）
if grep -q 'href="/' dist/index.html; then
  echo "    ❌ dist/index.html 含绝对路径 /，file:// 加载会 404"
  echo "    请检查 vite.config.ts 的 base 配置"
  exit 1
fi
if grep -q 'src="/' dist/index.html; then
  echo "    ❌ dist/index.html 含绝对 script 路径，file:// 加载会 404"
  echo "    请检查 vite.config.ts 的 base 配置"
  exit 1
fi
echo "    ✅ dist/index.html 资源路径全部相对化"

# 列出产物
echo "    产物文件："
find dist -type f | head -20 | sed 's/^/      /'
SIZE=$(du -sh dist | cut -f1)
echo "    总大小：${SIZE}"

# ---- Step 4: 同步到 plugin assets ----
step "4/4 同步到 ${ASSETS_DIR}"
rm -rf "${ASSETS_DIR}"
mkdir -p "${ASSETS_DIR}"
cp -r dist/. "${ASSETS_DIR}/"

# 验证
if [[ ! -f "${ASSETS_DIR}/index.html" ]]; then
  echo "    ❌ 同步后 index.html 缺失"
  exit 1
fi

echo ""
echo "========================================"
echo "✅ plugin-openlist/web 构建并同步完成（mode=${BUILD_MODE}）"
echo ""
echo "  source:    ${WEB_DIR}/src"
echo "  build:     ${WEB_DIR}/dist"
echo "  assets:    ${ASSETS_DIR}"
echo "  apk load:  file:///android_asset/openlist/index.html"
echo ""
echo "  下一步："
echo "    cd ${MOBILE_DIR}/android"
echo "    ./gradlew :plugin-openlist:assembleDebug"
echo "========================================"
