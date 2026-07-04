#!/usr/bin/env bash
# ENCV Capacitor 预览一键启动
# 铁律：
#   1. 整合 mock 数据生成 + 后端 air 监视 + Vite 前端为一条命令
#   2. 后端必须用 air 监视重载（禁止 go build / 手动 go run）
#   3. 不修改 config.user.json —— servingDir 永远为 /storage/emulated/0
#   4. 严禁任何符号链接 —— mock-data 真实目录在 /storage/emulated/0
#   5. Vite 强制监听 :8100（D9: vite 是纯净 SPA dev server，不做反向代理；
#                     统一由 preview-gateway :16666 接管跨上游转发）
#   6. 脚本必须保持前台运行（可被 pm2/nohup 包装，便于 OpenPreview 激活）
#   7. 脚本退出时优雅停止所有子进程（仅主预览 :2025/:8100，不动 :5174 plugin-openlist-web
#                     和 :16666 preview-gateway —— 它们由各自 pm2 app 管理）
set -euo pipefail
shopt -s lastpipe

# ---- 信号陷阱：脚本退出时杀掉所有子进程（仅主预览端口） ----
SUBPIDS=()
cleanup() {
  echo ""
  echo "==> 收到退出信号，停止主预览子进程 (:2025 / :8100)..."
  for pid in "${SUBPIDS[@]}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
    fi
  done
  sleep 1
  # 强制清理（air 还会启动 ./tmp/encv 子进程）
  pkill -P $$ 2>/dev/null || true
  pkill -x air 2>/dev/null || true
  # 精确杀 8100 端口的 vite（保留 5174 plugin-openlist-vite 和 16666 preview-gateway）
  for pid in $(lsof -ti :8100 2>/dev/null || true); do
    kill "${pid}" 2>/dev/null || true
  done
  # 兜底：杀 2025 端口的 encv 主进程
  for pid in $(lsof -ti :2025 2>/dev/null || true); do
    kill "${pid}" 2>/dev/null || true
  done
  exit 0
}
trap cleanup INT TERM

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MOBILE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${MOBILE_DIR}/../.." && pwd)"

# 确保 air 在 PATH 中（mise 安装的 Go 自带 air，但不在标准 PATH）
export PATH="/root/.local/share/mise/installs/go/1.25.1/bin:${PATH}"

BACKEND_PORT="${ENCV_MOBILE_PORT:-2025}"
MOCK_DIR="${ENCV_MOCK_ROOT:-/storage/emulated/0}"

# D9: Vite 强制监听 :8100（统一入口由 preview-gateway :16666 接管，vite 不再做反向代理）
# 不再做 :5173 占用检测 —— 5173 历史上是 Vite 默认端口，但已经被 preview-gateway 替代
VITE_PORT="${ENCV_VITE_PORT:-8100}"

cd "${REPO_ROOT}"

step() { echo ""; echo "==> $*"; }

# ---------- Step 0: 停止残留 ENCV 进程 ----------
# ⚠️ 必须精确到「主预览」端口 (2025/8100) — 不能误杀 plugin-openlist-vite (:5174)
#    和 preview-gateway (:16666)
step "0/5 停止残留 ENCV 主预览进程 (:2025 / :8100)"
pkill -x air 2>/dev/null && echo "    killed air" || true
pkill -f '^./tmp/encv' 2>/dev/null && echo "    killed ./tmp/encv" || true
pkill -f '/tmp/encv start' 2>/dev/null && echo "    killed /tmp/encv start" || true

# 精确杀 8100 端口的 vite（保留 5174 plugin-openlist-vite 和 16666 preview-gateway）
VITE_PIDS="$(lsof -ti :8100 2>/dev/null || true)"
if [[ -n "${VITE_PIDS}" ]]; then
  for pid in ${VITE_PIDS}; do
    kill "${pid}" 2>/dev/null && echo "    killed vite-on-:8100 pid=${pid}" || true
  done
fi

BACKEND_PIDS="$(lsof -ti :"${BACKEND_PORT}" 2>/dev/null || true)"
if [[ -n "${BACKEND_PIDS}" ]]; then
  for pid in ${BACKEND_PIDS}; do
    kill "${pid}" 2>/dev/null && echo "    killed backend pid=${pid} (port ${BACKEND_PORT})"
  done
  sleep 1
  # 二次确认（部分进程是 setsid+nohup 起的，父进程死子进程未必死）
  BACKEND_PIDS="$(lsof -ti :"${BACKEND_PORT}" 2>/dev/null || true)"
  if [[ -n "${BACKEND_PIDS}" ]]; then
    for pid in ${BACKEND_PIDS}; do
      kill -9 "${pid}" 2>/dev/null && echo "    force-killed backend pid=${pid} (port ${BACKEND_PORT})"
    done
  fi
fi
sleep 1

# ---------- Step 1: 确保 node_modules 就绪 ----------
step "1/5 确保 ${MOBILE_DIR}/node_modules 就绪（走 MCP 代理）"
cd "${MOBILE_DIR}"
if [[ ! -d "node_modules/vite" ]]; then
    echo "    node_modules 缺失，npm install ..."
    npm install --no-audit --no-fund --prefer-offline
fi
cd "${REPO_ROOT}"

# ---------- Step 2:（已废弃）生成 mock 数据 ----------
# 2026-06-10 改造：Node CLI 脚本已删。mock 数据改由后端 /api/mock/generate 提供（用户主动点 UI 按钮）。
# service-guard 不再检查 01-plain-media marker，只查 servingDir == /storage/emulated/0。
# 用户没主动按"生成 Mock"按钮时，目录是空的——这是预期行为。
# 注意：不在这里 mkdir /storage/emulated/0，由 backend 启动时 mobile overlay 验证目录可读即可。

# ---------- Step 3: air 启动后端（前台子进程） ----------
# env 注入策略：
#   - pm2 监管：ecosystem.config.cjs 已注入 ENCV_DEV_PREVIEW=1 / ENCV_MOBILE=1
#   - 裸脚本用户：.air-run.sh 末尾 `export ENCV_DEV_PREVIEW=:-1` 兜底
# 这里不重复 inline 设，避免在 air rebuild 时不稳定。
step "2/5 启动后端（air 监视重载，ENCV_DEV_PREVIEW/ENCV_MOBILE 由 .air-run.sh 兜底）"
cd "${REPO_ROOT}"
air &
AIR_PID=$!
SUBPIDS+=("${AIR_PID}")
echo "    air pid=${AIR_PID}"

# 等待后端就绪
for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24; do
  if curl -s "http://localhost:${BACKEND_PORT}/api/config" >/dev/null 2>&1; then
    echo "    backend ready (port ${BACKEND_PORT})"
    break
  fi
  sleep 0.5
done

# 验证 mobile overlay 生效：servingDir 必须包含 01-plain-media
# 这是 2026-06-04 修复的痛点：之前 tmp/encv 手工启动无 ENCV_DEV_PREVIEW=1，
# mobile overlay 未触发，server.dir 留在默认的 "/" → 解析为 /workspace → 看到 .md 等文件
SERVING_GUARD_OK=0
for i in 1 2 3 4 5 6 7 8 9 10; do
  GUARD_JSON=$(curl -s "http://localhost:${BACKEND_PORT}/api/service-guard" 2>/dev/null || true)
  if echo "${GUARD_JSON}" | grep -q '"ready":true'; then
    SERVING_DIR=$(echo "${GUARD_JSON}" | grep -oE '"servingDir":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "    ✅ service-guard OK: servingDir=${SERVING_DIR}"
    SERVING_GUARD_OK=1
    break
  fi
  sleep 0.5
done
if [[ "${SERVING_GUARD_OK}" != "1" ]]; then
  echo ""
  echo "❌ 错误：后端 service-guard 校验失败（10s 内未 ready）" >&2
  echo "   这通常意味着 mobile overlay (ENCV_DEV_PREVIEW=1) 没生效" >&2
  echo "   检查: ps -ef | grep -E 'air|tmp/encv' | grep -v grep" >&2
  echo "   检查: tail -20 /tmp/encv-air.log" >&2
  echo "   手工验证: curl -s http://localhost:${BACKEND_PORT}/api/service-guard | head -c 500" >&2
  echo ""
  curl -s "http://localhost:${BACKEND_PORT}/api/service-guard" | head -c 500
  echo ""
  exit 1
fi

# ---------- Step 4: Vite 前端（前台子进程） ----------
step "4/6 启动 Vite 前端（port ${VITE_PORT}）"
cd "${MOBILE_DIR}"
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

# ---------- Step 5: 状态报告 + OpenPreview 提示 ----------
step "5/6 ✅ 服务全部就绪"
cat <<EOF

========================================
✅ ENCV 预览已启动

  端口分配（单端口对外，preview-gateway 统一代理）:
     :8100  = Vite dev server（前端，纯净 SPA，不做反向代理）
     :2025  = Go Backend（air 监视重载）
     :16666 = preview-gateway（统一入口，4 上游代理）
     :5174  = plugin-openlist-web（Vite，被 :16666/openlist-ui 代理）
     :16000 = agent-tool-host（OpenPreview 工具用的外网入口）

  跨上游路由（由 :16666 接管）:
     /             → :8100  encv-mobile
     /openlist-ui  → :5174  plugin-openlist-web
     /openlist/    → :2025  encv-go
     /api          → :2025  encv-go
     /p/           → :2025  encv-go
     /play         → :2025  encv-go

  用户访问地址（必须先 OpenPreview 激活 :16666 触发自动注册）:
     http://localhost:16666/   ← 统一入口（推荐）
     http://localhost:8100/    ← 直连 vite（仅排查用，跨上游路由不可用）

  ⚠️ 重要：必须用 OpenPreview 激活 :16666 才能外部访问
     OpenPreview(command_id="<本脚本 command_id>", preview_url="http://localhost:16666/")

  配置文件:    ${REPO_ROOT}/config.user.json （未修改）
  servingDir:  ${MOCK_DIR}  （设计预期路径，脚本自建）

  停止:  Ctrl+C  （脚本会自动清理所有子进程）

  后续上传测试文件（hyYGPCwJPQ3+xrdAvfnn2.bin）：
    - 浏览器访问 http://localhost:16666/  （前提：OpenPreview 已激活）
    - Files 页面 → Upload FAB → 选择文件
========================================
EOF

# ---------- Step 5: 保持前台运行（等待子进程或信号） ----------
step "5/5 保持前台运行（按 Ctrl+C 停止）"
echo "    air pid=${AIR_PID}  vite pid=${VITE_PID}"
echo "    等待子进程..."

# 等待任何子进程退出
wait -n "${SUBPIDS[@]}" 2>/dev/null || true
echo ""
echo "==> 某个子进程退出，触发清理..."
cleanup
