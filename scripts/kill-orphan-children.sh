#!/bin/bash
# scripts/kill-orphan-children.sh
# =====================================================
# 沙箱专用：强杀 preview 链路上所有 orphan / 卡死子进程
#
# 必须原因（沙箱特有，普通 Linux 不会遇到）：
#   1. air 退出后，air spawn 的 go build / go run 子进程失去父进程变 orphan，
#      沙箱内 init(1) 不主动 reap 也没人收割，zombie 累积
#   2. go build 经常卡 Sl 状态（sleeping, multithreaded）+ 0% CPU：
#      网络 pipe 死锁（沙箱 DNS / proxy.golang.org 访问卡住）
#   3. preview-gateway Node 进程被 SIGKILL 后，4 个 spawn 的子进程
#      （air / vite x2 / openlist 可选）也变 orphan
#
# 调用时机：
#   - setup-sandbox-env.sh 末尾（清干净后让用户用 pm2 start）
#   - previews.sh restart 子命令
#   - 用户手动 debug 时
#
# 用法：
#   bash scripts/kill-orphan-children.sh        # 静默
#   bash scripts/kill-orphan-children.sh --report # 报告残留
#
# ⚠️ 必须在 pm2 kill 之后调用（pm2 监管的进程交给 pm2 kill 处理）

set -e

REPORT=0
[ "${1:-}" = "--report" ] && REPORT=1

log() { echo "[kill-orphan] $*"; }

# ── 1. pm2 监管（让 pm2 自己处理，但要 force-kill）──
pm2 kill 2>/dev/null || true
sleep 1

# ── 2. air (Go live reload) ──
pkill -9 -f "/root/go/bin/air" 2>/dev/null || true
pkill -9 -f "/go/bin/air" 2>/dev/null || true
pkill -9 -x "air" 2>/dev/null || true

# ── 3. go build / go run / go test ──
pkill -9 -f "go build" 2>/dev/null || true
pkill -9 -f "go run" 2>/dev/null || true
pkill -9 -f "go test" 2>/dev/null || true

# ── 4. Go 编译 worker (Go 用 /tmp/go-build-XXX 跑 compile) ──
pkill -9 -f "go-build" 2>/dev/null || true
pkill -9 -f "/tmp/go-build" 2>/dev/null || true

# ── 5. encv 二进制 (encv-go 编译产物) ──
pkill -9 -x "encv" 2>/dev/null || true
pkill -9 -f "/workspace/tmp/encv" 2>/dev/null || true
# go run 编译产物在 ~/.cache/go-build/<hash>/encv（路径不在 /workspace）
# comm 是 "encv"（精确匹配能杀），但 pkill -9 -f 用全路径更稳
pkill -9 -f "cache/go-build.*/encv" 2>/dev/null || true
pkill -9 -f "/root/.cache/go-build" 2>/dev/null || true

# ── 6. vite dev server (encv-mobile :8100 / plugin-openlist :5174) ──
pkill -9 -f "vite/bin/vite.js" 2>/dev/null || true
pkill -9 -f "vite.*--port 8100" 2>/dev/null || true
pkill -9 -f "vite.*--port 5174" 2>/dev/null || true
pkill -9 -x "vite" 2>/dev/null || true

# ── 7. preview-gateway Node + openpreview-stub ──
pkill -9 -f "preview-gateway/dist/server" 2>/dev/null || true
pkill -9 -f "openpreview-stub" 2>/dev/null || true
pkill -9 -f "openpreview-stub.js" 2>/dev/null || true

# ── 8. openlist 二进制 (Go fork) ──
pkill -9 -x "openlist" 2>/dev/null || true

# ── 9. 兜底：PPID=1 (orphan) + comm/args 含 go/air/vite/encv/node/preview ──
# 孤儿进程被 init(1) 收养，pkill -f 可能因命令行变化漏掉
# 用 ps + PPID=1 显式抓
ORPHAN_PIDS=$(ps -eo pid,ppid,args --no-headers | \
  awk '$2 == 1 && /(air|go build|go run|go-build|vite|encv$|preview-gateway|openpreview|openlist$)/ {print $1}')
if [ -n "$ORPHAN_PIDS" ]; then
  log "kill orphan (PPID=1) PIDs: $ORPHAN_PIDS"
  echo "$ORPHAN_PIDS" | xargs -r kill -9 2>/dev/null || true
fi

# ── 10. 兜底 2：stat=Sl (sleeping) + 0% CPU + 长期 + comm 是 go/air/vite ──
# 这些是「卡死」go build 进程（网络 pipe 死锁，0% CPU 永远不醒）
STUCK_PIDS=$(ps -eo pid,stat,pcpu,comm --no-headers | \
  awk '$2 ~ /^S/ && $3+0 < 1 && ($1 == "air" || $4 == "go" || $4 == "vite") {print $1}')
if [ -n "$STUCK_PIDS" ]; then
  log "kill stuck (Sl+0%cpu) PIDs: $STUCK_PIDS"
  echo "$STUCK_PIDS" | xargs -r kill -9 2>/dev/null || true
fi

# ── 11. Go 工具链辅助进程 (compile/asm/cgo/vet/test) ──
pkill -9 -f "compile" 2>/dev/null || true
pkill -9 -f "asm" 2>/dev/null || true
pkill -9 -f "cgo" 2>/dev/null || true
pkill -9 -f "vet" 2>/dev/null || true
pkill -9 -x "compile" 2>/dev/null || true
pkill -9 -x "asm" 2>/dev/null || true
pkill -9 -x "cgo" 2>/dev/null || true
pkill -9 -x "vet" 2>/dev/null || true

# ── 12. 真正的 zombie (stat=Z) — 沙箱 init(1) 不主动 reap，必须触发 ──
# 父进程找不到 = parent PID 是 init 或已死；先 SIGCHLD 让 init 知道，
# 不行就 SIGHUP/SIGKILL
ZOMBIE_PIDS=$(ps -eo pid,stat,args --no-headers | awk '$2 ~ /^Z/ {print $1}')
if [ -n "$ZOMBIE_PIDS" ]; then
  log "触发 zombie reap PIDs: $ZOMBIE_PIDS"
  # 给僵尸的 PPID 发 SIGCHLD 让 init 收割
  for zpid in $ZOMBIE_PIDS; do
    ZPPID=$(ps -o ppid= -p "$zpid" 2>/dev/null | tr -d ' ')
    [ -n "$ZPPID" ] && kill -CHLD "$ZPPID" 2>/dev/null || true
  done
  sleep 1
  # 还活着的僵尸 = 没人收，给 1 进程发 SIGCHLD
  for zpid in $ZOMBIE_PIDS; do
    if [ -d "/proc/$zpid" ]; then
      kill -CHLD 1 2>/dev/null || true
    fi
  done
fi

# ── 13. pnpm/npx (vite 启动辅助) ──
pkill -9 -f "pnpm" 2>/dev/null || true
pkill -9 -f "/npx" 2>/dev/null || true

# ── 14. 二次扫描 PPID=1 孤儿（上一波 kill 后又出现的新孤儿）──
sleep 1
ORPHAN2=$(ps -eo pid,ppid,args --no-headers | \
  awk '$2 == 1 && /(air|go build|go run|go-build|compile|asm|cgo|vite|encv$|preview-gateway|openpreview|openlist$|node.*preview|node.*encv)/ {print $1}')
if [ -n "$ORPHAN2" ]; then
  log "二次扫描 orphan PIDs: $ORPHAN2"
  echo "$ORPHAN2" | xargs -r kill -9 2>/dev/null || true
fi

sleep 2

# ── 报告 ──
if [ "$REPORT" = "1" ]; then
  # 排除 self + children + 工具进程（ps / awk / grep / kill / kill-orphan）
  # 防止报告时把 killer 自己的 ps/awk 进程算成「残留」
  MY_PGID=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' ')
  REMAINING=$(ps -eo pid,ppid,pgid,comm,args --no-headers | \
    awk -v mypgid="$MY_PGID" '
      /(air|go build|go run|go-build|compile|asm|cgo|vite|encv$|preview-gateway|openpreview|openlist$|node.*preview|node.*encv)/ {
        if ($3 == mypgid) next       # 排除自己进程组
        if ($1 == "1") next          # ps 自身
        if ($4 == "awk" || $4 == "grep" || $4 == "ps" || $4 == "sed" || $4 == "head" || $4 == "tr" || $4 == "xargs" || $4 == "kill") next
        print
      }' | wc -l)
  if [ "$REMAINING" -gt 0 ]; then
    log "⚠️ 残留进程 $REMAINING 个："
    ps -eo pid,ppid,comm,args --no-headers | \
      awk -v mypgid="$MY_PGID" '
        /(air|go build|go run|go-build|compile|asm|cgo|vite|encv$|preview-gateway|openpreview|openlist$|node.*preview|node.*encv)/ {
          if ($3 == mypgid) next
          if ($1 == "1") next
          if ($4 == "awk" || $4 == "grep" || $4 == "ps" || $4 == "sed" || $4 == "head" || $4 == "tr" || $4 == "xargs" || $4 == "kill") next
          print
        }'
  else
    log "✅ 全部清理干净"
  fi
fi
