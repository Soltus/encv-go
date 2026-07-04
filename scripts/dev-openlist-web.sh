#!/usr/bin/env bash
# dev-openlist-web.sh — 启动 plugin-openlist/web Vite（plugin 管理 UI，端口 5174）
#
# 重要区别（与 2026-05 之前的旧版不同）：
#   本脚本**只服务 plugin 管理 UI**（OpenListHome / Settings / ConfigEditor / WebView），
#   **不再代理 OpenList 后端 (5244)**。
#   撤销原因：subpath 路由改造（/openlist-spa/、/openlist-ui/）不可靠，
#   OpenList 应在原始环境 / 跑（Hi-Sillot fork on 127.0.0.1:5244），与 prod 模式对齐。
#
# 沙箱 dev 启动顺序：
#   Terminal 1: bash scripts/dev-openlist.sh
#               → 启动真实 OpenList fork on 127.0.0.1:5244（CORS=*）
#   Terminal 2: bash scripts/dev-openlist-web.sh  ← 你在这里
#               → Vite 5174 端口，OpenListWebView 的 iframe 直访 127.0.0.1:5244/#/login
#   浏览器: open http://localhost:5174/webview
#
# Production（Android WebView）：
#   - WebView 加载 file:///android_asset/openlist/index.html（plugin-openlist/src/main/assets/openlist/）
#   - iframe 内部直访 http://127.0.0.1:5244/（与本机 OpenList 进程同设备）
#
# 与主 app encv-mobile Vite (8100) 的 /openlist-ui-proxy 无关：
#   - 那个是主 app 开发期「在浏览器里调试 encv-mobile + 顺便看 OpenList」用的辅助中间件
#   - 本脚本（5174）是 plugin 自己的管理 UI，职责互不重叠

set -euo pipefail
shopt -s lastpipe

# ---- 信号陷阱：脚本退出时杀掉所有子进程 ----
SUBPIDS=()
cleanup() {
  echo ""
  echo "==> 收到退出信号，停止 plugin-openlist/web 预览..."
  for pid in "${SUBPIDS[@]}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
    fi
  done
  sleep 1
  pkill -P $$ 2>/dev/null || true
  pkill -f 'vite.*plugin-openlist' 2>/dev/null || true
  pkill -f 'plugin-openlist/web' 2>/dev/null || true
  exit 0
}
trap cleanup INT TERM

# Script location: <repo>/scripts/dev-openlist-web.sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Repo root = parent of scripts/
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_DIR="${REPO_DIR}/app/encv-mobile/plugin-openlist/web"

# ---- 端口选择：默认 5174，plugin-openlist/web vite.config.ts 已设；
# ---- 被占时回退到 5175
VITE_PORT="${ENCV_OPENLIST_WEB_PORT:-5174}"
if lsof -i :5174 >/dev/null 2>&1; then
  echo "    :5174 已被占用，使用 :5175"
  VITE_PORT=5175
fi

cd "${REPO_DIR}/app/encv-mobile"

step() { echo ""; echo "==> $*"; }

# ---- Step 0: 停止残留 vite 进程（plugin-openlist/web 范围）----
step "0/4 停止残留 plugin-openlist/web vite 进程"
pkill -f 'vite.*plugin-openlist' 2>/dev/null && echo "    killed vite" || true
pkill -f 'plugin-openlist/web' 2>/dev/null && echo "    killed vite" || true

WEB_VITE_PIDS="$(lsof -ti :${VITE_PORT} 2>/dev/null || true)"
if [[ -n "${WEB_VITE_PIDS}" ]]; then
  for pid in ${WEB_VITE_PIDS}; do
    kill "${pid}" 2>/dev/null && echo "    killed pid=${pid} on :${VITE_PORT}"
  done
fi
sleep 1

# ---- Step 1: 确保 plugin-openlist/web node_modules 就绪 ----
step "1/4 确保 ${WEB_DIR}/node_modules 就绪"
if [[ ! -d "${WEB_DIR}/node_modules/vite" ]]; then
  echo "    node_modules 缺失，pnpm install ..."
  cd "${REPO_DIR}/app/encv-mobile"
  pnpm install --no-frozen-lockfile --filter '@encvgo/plugin-openlist-web...'
fi
cd "${WEB_DIR}"

# ---- Step 2: 跳过 @encvgo/components 检查（已移除）----
# ---- Step 3: 启动 Vite dev server ----
step "3/4 启动 Vite dev server (port ${VITE_PORT})"
cd "${WEB_DIR}"

# ⚠️ 沙箱 dev 必须设 VITE_BASE=/openlist-ui/ —— preview-gateway 把 :16666/openlist-ui/* 透传到 :5174，
# Vite 需要在 HTML 输出 <base href="/openlist-ui/">，否则相对资源路径（./src/main.ts）会被
# 浏览器解析到 :16666/ 主 app 路径 → 404 → 整个 plugin SPA 空白。
# 重要：同时也让 vue-router 用 createWebHashHistory('/openlist-ui/')，让 hash 路由也感知 base。
export VITE_BASE="/openlist-ui/"

./node_modules/.bin/vite --host 0.0.0.0 --port "${VITE_PORT}" --strictPort &
VITE_PID=$!
SUBPIDS+=("${VITE_PID}")
echo "    vite pid=${VITE_PID}"

# 等待 Vite 就绪
for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  if curl -s "http://localhost:${VITE_PORT}/" >/dev/null 2>&1; then
    echo "    vite ready (port ${VITE_PORT})"
    break
  fi
  sleep 0.5
done

# ---- Step 4: 状态报告 + OpenPreview 提示 ----
step "4/4 ✅ plugin-openlist/web 预览已启动"
cat <<EOF

========================================
✅ plugin-openlist/web 预览已启动

  端口：    :${VITE_PORT}  = Vite dev server（plugin-openlist/web）

  路由：
     /            → 重定向到 /home
     /home        → OpenListHome（AppBar + 4 工具按钮 + StatusCard + LogList + FAB）
     /config      → OpenListConfigEditor（JSON 编辑器）
     /settings    → OpenListSettings（版本/数据目录占位）
     /webview     → OpenListWebView（需 Android WebView 容器提示）

  ⚠️ 沙箱浏览器限制：
     - window.OpenListNative 不存在（仅 Android WebView 注入）
     - 所有 Native 调用走 safe() fallback → 显示默认态「未安装/已停止」
     - 目标：UI 视觉预览 + HMR 实时迭代
     - 真机联调：需在主 app 内通过 plugin-openlist Content() 加载

  用户访问地址（必须先 OpenPreview 激活）：
     http://localhost:${VITE_PORT}/

  ⚠️ 重要：必须使用 OpenPreview 工具激活预览才能外部访问
     OpenPreview(command_id="<本脚本 command_id>", preview_url="http://localhost:${VITE_PORT}/")

  配套工具：
     - 与主 app 预览不冲突：bash scripts/start-preview.sh
     - 与 OpenList SPA 预览不冲突：bash scripts/dev-openlist.sh

  停止:  Ctrl+C  （脚本会自动清理所有子进程）

  修改 plugin-openlist/web/src/** 任意文件 → 浏览器自动 HMR
========================================
EOF

# ---- 保持前台运行 ----
echo "    vite pid=${VITE_PID}"
echo "    等待子进程..."

wait -n "${SUBPIDS[@]}" 2>/dev/null || true
echo ""
echo "==> 某个子进程退出，触发清理..."
cleanup
