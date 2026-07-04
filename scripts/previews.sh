#!/usr/bin/env bash
# =============================================================================
# previews.sh — 沙箱 dev 服务统一管理（基于 pm2，方案 C 大改 2026-06-08）
# -----------------------------------------------------------------------------
# 管 2 个 pm2 app：
#   ① preview-gateway    (:16666) — 唯一对外入口 + 唯一进程管理者
#                              （内部 spawn: encv-go :2025 + encv-mobile-vite :8100）
#   ② openpreview-stub   (:15003) — OpenPreview 工具 web_server command_id 源
#
# 注意：encv-go / encv-mobile-vite / plugin-openlist-vite / openlist
#       都是 preview-gateway 的子进程，**不需独立 pm2 app**。
#       要查子进程状态：`curl :16666/__gateway/health`
#
# 用法：
#   bash scripts/previews.sh start [app]  启全部 / 单个
#   bash scripts/previews.sh stop  [app]  停全部 / 单个
#   bash scripts/previews.sh restart [app] 重启全部 / 单个
#   bash scripts/previews.sh reload        0 秒重载配置
#   bash scripts/previews.sh status        状态 + 端口 + 内存 + 子进程 health
#   bash scripts/previews.sh logs   [app]  实时日志
#   bash scripts/previews.sh monit         终端仪表盘
#   bash scripts/previews.sh kill          强杀全部 + 端口兜底
# =============================================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ECOSYSTEM="${REPO_ROOT}/ecosystem.config.cjs"

R='\033[1;31m'; G='\033[1;32m'; Y='\033[1;33m'; C='\033[1;36m'; B='\033[1;34m'; N='\033[0m'
log()  { printf "${C}[previews]${N} %s\n" "$*"; }
ok()   { printf "${G}[previews]${N} %s\n" "$*"; }
warn() { printf "${Y}[previews]${N} %s\n" "$*" >&2; }
err()  { printf "${R}[previews]${N} %s\n" "$*" >&2; }

usage() {
  cat <<EOF
用法: bash scripts/previews.sh <command> [app_name]

命令:
  start [app]   启全部 / 单个
  stop  [app]   停全部 / 单个
  restart [app] 重启全部 / 单个
  reload        0 秒重载（pickup ecosystem.config.cjs 变更）
  status        pm2 状态 + 端口 + 内存 + 子进程 health
  logs   [app]  实时日志（默认全部交错，指定 app 看单个）
  monit         终端仪表盘（CPU/内存/事件）
  kill          强杀全部 + 端口兜底

服务名 (pm2 app):
  preview-gateway    (:16666, 内部 spawn 4 个子进程)
  openpreview-stub   (:15003, OpenPreview 工具源)
EOF
}

if ! command -v pm2 >/dev/null 2>&1; then
  err "❌ pm2 未安装"
  log "  安装方法：npm i -g pm2  /  pnpm add -g pm2"
  exit 1
fi

CMD="${1:-status}"
APP_NAME="${2:-}"

# 端口状态检查（pm2 监管端口 + 子进程端口分别检查）
check_ports() {
  echo ""
  log "📡 端口状态："
  local pm2_ports=(
    "16666:preview-gateway"
    "15003:openpreview-stub"
  )
  for entry in "${pm2_ports[@]}"; do
    local port="${entry%%:*}"
    local name="${entry##*:}"
    if lsof -i ":$port" >/dev/null 2>&1; then
      local pid
      pid=$(lsof -ti ":$port" 2>/dev/null | head -1)
      printf "   :%-5s  ${G}✅${N} %s (pid=%s)\n" "$port" "$name" "$pid"
    else
      printf "   :%-5s  ${R}❌${N} %s\n" "$port" "$name"
    fi
  done

  echo ""
  log "📦 子进程端口（由 preview-gateway 内部管理）:"
  local child_ports=(
    "2025:encv-go (air)"
    "8100:encv-mobile-vite"
    "5174:plugin-openlist-vite (按需)"
    "5244:openlist (按需)"
  )
  for entry in "${child_ports[@]}"; do
    local port="${entry%%:*}"
    local name="${entry##*:}"
    if lsof -i ":$port" >/dev/null 2>&1; then
      local pid
      pid=$(lsof -ti ":$port" 2>/dev/null | head -1)
      printf "   :%-5s  ${G}✅${N} %s (pid=%s)\n" "$port" "$name" "$pid"
    else
      printf "   :%-5s  ${R}❌${N} %s\n" "$port" "$name"
    fi
  done
}

# 子进程 health 详情（直连 gateway）
show_children_health() {
  if ! command -v jq >/dev/null 2>&1; then
    log "⚠️  jq 未装，跳过子进程 health 详情"
    return
  fi
  echo ""
  log "🏥 preview-gateway 子进程 health："
  local health
  health=$(curl -s --max-time 3 http://localhost:16666/__gateway/health 2>/dev/null || echo "")
  if [[ -z "$health" ]]; then
    warn "   ⚠️  :16666 不可达 — gateway 还没启？"
    return
  fi
  local ok
  ok=$(echo "$health" | jq -r '.ok // "unknown"')
  if [[ "$ok" == "true" ]]; then
    printf "   ok: ${G}true${N}\n"
  else
    printf "   ok: ${R}${ok}${N}\n"
  fi
  printf "   children:\n"
  echo "$health" | jq -r '.children[]? | "     - \(.name) (pid=\(.pid // "?") ready=\(.ready))"' 2>/dev/null
  local optdown
  optdown=$(echo "$health" | jq -r '.optionalDown | length // 0')
  if [[ "$optdown" -gt 0 ]]; then
    echo "   optionalDown (按需, 预期):"
    echo "$health" | jq -r '.optionalDown[]? | "     - \(.name) (\(.url))"' 2>/dev/null
  fi
}

case "$CMD" in
  start)
    if [[ -n "$APP_NAME" ]]; then
      log "启 ${APP_NAME} ..."
      pm2 start "$ECOSYSTEM" --only "$APP_NAME" 2>&1 | tail -8
    else
      log "启全部 (preview-gateway + openpreview-stub) ..."
      pm2 start "$ECOSYSTEM" 2>&1 | tail -20
    fi
    sleep 2
    pm2 status
    check_ports
    show_children_health
    ;;

  stop)
    if [[ -n "$APP_NAME" ]]; then
      log "停 ${APP_NAME} ..."
      pm2 stop "$APP_NAME" 2>&1 | tail -5
    else
      log "停全部 ..."
      pm2 stop "$ECOSYSTEM" 2>&1 | tail -5
    fi
    sleep 1
    check_ports
    ;;

  restart)
    if [[ -n "$APP_NAME" ]]; then
      log "重启 ${APP_NAME} ..."
      pm2 restart "$APP_NAME" 2>&1 | tail -5
    else
      log "重启全部 ..."
      pm2 restart "$ECOSYSTEM" 2>&1 | tail -5
    fi
    sleep 2
    check_ports
    show_children_health
    ;;

  reload)
    log "重载 ecosystem（0 秒停机） ..."
    pm2 reload "$ECOSYSTEM" 2>&1 | tail -5
    pm2 status
    ;;

  status)
    pm2 status
    check_ports
    show_children_health
    ;;

  logs)
    if [[ -n "$APP_NAME" ]]; then
      pm2 logs "$APP_NAME" --lines 100
    else
      pm2 logs --lines 50
    fi
    ;;

  monit)
    pm2 monit
    ;;

  kill)
    warn "⚠️  强杀所有 pm2 进程 ..."
    pm2 kill 2>&1 | tail -3
    sleep 1
    # 清理所有相关端口（pm2 监管 + gateway 内部子进程 + 历史残留）
    for port in 16666 15003 2025 8100 5174 5244; do
      pids=$(lsof -ti ":$port" 2>/dev/null || true)
      if [[ -n "$pids" ]]; then
        log "清理 :$port 残留 pids: $pids"
        kill $pids 2>/dev/null || true
      fi
    done
    sleep 1
    check_ports
    ok "已清空"
    ;;

  -h|--help|help)
    usage
    exit 0
    ;;

  *)
    err "未知命令: $CMD"
    usage
    exit 2
    ;;
esac
