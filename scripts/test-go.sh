#!/usr/bin/env bash
# scripts/test-go.sh
# =====================================================
# Go 测试唯一入口。
# 强保证：无论内部发生什么，HARD_TIMEOUT 后必返回。
#
# 用法：
#   bash scripts/test-go.sh                                # 默认 ./internal/...
#   bash scripts/test-go.sh ./internal/crypto/...          # 指定包
#   bash scripts/test-go.sh -run TestFoo ./internal/...    # 透传
#   HARD_TIMEOUT=60 bash scripts/test-go.sh                # 自定义总超时
#
# 行为：
#   1. pre-flight 清理（上轮残留的 /tmp/encv-test-*, 端口 2025, 崩溃堆栈目录）
#   2. go test（带 -timeout + 外层 timeout 兜底）
#   3. 退出时强杀残留 go test / go-build 进程
#
# 2026-06-15 沙箱崩溃根因防御层
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

HARD_TIMEOUT=${HARD_TIMEOUT:-600}        # 总硬超时 10min
PACKAGE_TIMEOUT=${PACKAGE_TIMEOUT:-120}  # 单包超时 2min
LOG_ROOT=${LOG_ROOT:-.test-runs}
# PKGS 优先级：命令行位置参数 > PKGS 环境变量 > 默认 ./internal/...
if [ $# -gt 0 ] && [ -z "${PKGS:-}" ]; then
  PKGS="$*"
fi
PKGS=${PKGS:-"./internal/..."}
EXTRA_ARGS=${EXTRA_ARGS:-}

# 当 PKGS 是 ./internal/... 等多包形式时，go test 不能接受 -run 等参数同时只跑部分包
# 用户传额外参数走 EXTRA_ARGS 透传

TS=$(date +%Y%m%d-%H%M%S)
LOG_DIR="$LOG_ROOT/go-$TS"
mkdir -p "$LOG_DIR"

echo "==================================="
echo "[test-go] started at $(date -Iseconds)"
echo "[test-go] pkgs=$PKGS"
echo "[test-go] hard_timeout=${HARD_TIMEOUT}s package_timeout=${PACKAGE_TIMEOUT}s"
echo "[test-go] extra_args=$EXTRA_ARGS"
echo "[test-go] log_dir=$LOG_DIR"
echo "===================================" | tee "$LOG_DIR/summary.txt"

# ── pre-flight 1：清理崩溃堆栈目录（保留 5 个最新） ──
if [ -d ".test-runs/crashes" ]; then
  find .test-runs/crashes -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn | tail -n +6 | awk '{print $2}' | xargs -r rm -rf 2>/dev/null || true
fi

# ── pre-flight 2：清理 /tmp 大文件（上次崩溃的产物） ──
TEMP_LEAKED=$(find /tmp -maxdepth 1 -name "encv-test-*" -size +100M 2>/dev/null | wc -l)
if [ "$TEMP_LEAKED" -gt 0 ]; then
  echo "[test-go][pre-flight] cleaning $TEMP_LEAKED large /tmp/encv-test-* files"
  find /tmp -maxdepth 1 -name "encv-test-*" -size +100M -exec rm -rf {} \; 2>/dev/null || true
fi

# ── pre-flight 3：端口 2025 占用检查（mock backend 可能残留） ──
if command -v lsof >/dev/null 2>&1; then
  if lsof -i :2025 -t >/dev/null 2>&1; then
    STUCK_PID=$(lsof -i :2025 -t | head -1)
    echo "[test-go][pre-flight] WARNING: port 2025 occupied by PID $STUCK_PID, killing"
    kill -9 "$STUCK_PID" 2>/dev/null || true
  fi
fi

# ── 跑测试（带强超时链） ──
START_TS=$(date +%s)

# shellcheck disable=SC2086
timeout --kill-after=30s "${HARD_TIMEOUT}s" go test $PKGS \
  -count=1 \
  -timeout="${PACKAGE_TIMEOUT}s" \
  -v \
  $EXTRA_ARGS \
  > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log"
EXIT=$?

END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

# ── 兜底：超时时强杀残留 ──
if [ $EXIT -eq 124 ] || [ $EXIT -eq 137 ]; then
  echo "[test-go] HARD TIMEOUT — killing residual" | tee -a "$LOG_DIR/summary.txt"
  pkill -9 -f "go test" 2>/dev/null
  pkill -9 -f "go-build" 2>/dev/null
  pkill -9 -f "compile" 2>/dev/null
fi

# ── 写摘要 ──
cat >> "$LOG_DIR/summary.txt" <<EOF
========================================
end_time:     $(date -d "@$END_TS" -Iseconds)
duration_sec: $DURATION
exit_code:    $EXIT
exit_meaning: $([ $EXIT -eq 0 ] && echo "OK" || ([ $EXIT -eq 124 ] && echo "TIMEOUT" || ([ $EXIT -eq 137 ] && echo "KILLED_OOM" || echo "TEST_FAILURE")))

=== stderr 摘要 (最后 30 行) ===
$(tail -30 "$LOG_DIR/stderr.log" 2>/dev/null || echo "(empty)")

=== stdout 摘要 (最后 10 行) ===
$(tail -10 "$LOG_DIR/stdout.log" 2>/dev/null || echo "(empty)")
EOF

# ── 打印结果 ──
EXIT_MSG="OK"
[ $EXIT -ne 0 ] && EXIT_MSG="FAIL"
[ $EXIT -eq 124 ] && EXIT_MSG="TIMEOUT"
[ $EXIT -eq 137 ] && EXIT_MSG="OOM_KILLED"

echo ""
echo "==================================="
echo "[test-go] $EXIT_MSG in ${DURATION}s (exit=$EXIT)"
echo "[test-go] logs: $LOG_DIR"
echo "==================================="

exit $EXIT
