#!/usr/bin/env bash
# scripts/test-diagnose.sh
# =====================================================
# 跑一次 Go 测试，收集诊断数据（不动任何代码）。
#
# 收集项：
#   - 总耗时
#   - 进程 RSS 峰值（验证"沙箱 OOM"是否真的发生）
#   - 进程 CPU/虚拟内存峰值
#   - Goroutine 数（从 runtime 通过 test 拿）
#   - 退出码（是否 0）
#   - stderr 摘要
#
# 用法：
#   bash scripts/test-diagnose.sh                  # 默认 5min
#   HARD_TIMEOUT=120 bash scripts/test-diagnose.sh # 自定义总超时
#
# 2026-06-15 沙箱崩溃根因诊断（test-architecture-refactor-defense-awareness）
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

HARD_TIMEOUT=${HARD_TIMEOUT:-300}      # 5min 默认
PACKAGE_TIMEOUT=${PACKAGE_TIMEOUT:-60}  # 单包 1min（diagnose 阶段要快）
LOG_ROOT=${LOG_ROOT:-.test-runs}

TS=$(date +%Y%m%d-%H%M%S)
LOG_DIR="$LOG_ROOT/diagnose-$TS"
mkdir -p "$LOG_DIR"

echo "==================================="
echo "[diagnose] started at $(date -Iseconds)"
echo "[diagnose] log_dir=$LOG_DIR"
echo "[diagnose] hard_timeout=${HARD_TIMEOUT}s package_timeout=${PACKAGE_TIMEOUT}s"
echo "==================================="

# 找当前 shell 的 PID 子树（go test 启动后才有）
find_my_pids() {
  pgrep -f "go test" 2>/dev/null | tr '\n' ' '
}

# 资源采样器：每 1s 采样所有 go test 相关进程的 RSS 峰值
SAMPLE_PIDS_PEAK_RSS=0
SAMPLE_PIDS_PEAK_VSZ=0
SAMPLE_COUNT=0
SAMPLER_OUT="$LOG_DIR/resource.csv"
echo "timestamp,pid,comm,rss_mb,vsz_mb,pcpu" > "$SAMPLER_OUT"

(
  END_AT=$(( $(date +%s) + HARD_TIMEOUT + 30 ))
  while [ "$(date +%s)" -lt "$END_AT" ]; do
    PIDS=$(find_my_pids)
    for pid in $PIDS; do
      if [ -d "/proc/$pid" ]; then
        LINE=$(ps -o pid=,comm=,rss=,vsz=,pcpu= -p "$pid" 2>/dev/null | tail -1)
        if [ -n "$LINE" ]; then
          PID_VAL=$(echo "$LINE" | awk '{print $1}')
          COMM_VAL=$(echo "$LINE" | awk '{print $2}')
          RSS_KB=$(echo "$LINE" | awk '{print $3}')
          VSZ_KB=$(echo "$LINE" | awk '{print $4}')
          PCPU_VAL=$(echo "$LINE" | awk '{print $5}')
          RSS_MB=$((RSS_KB / 1024))
          VSZ_MB=$((VSZ_KB / 1024))
          echo "$(date -Iseconds),$PID_VAL,$COMM_VAL,${RSS_MB},${VSZ_MB},${PCPU_VAL}" >> "$SAMPLER_OUT"
          # 跟踪峰值
          if [ "$RSS_MB" -gt "$SAMPLE_PIDS_PEAK_RSS" ]; then
            SAMPLE_PIDS_PEAK_RSS=$RSS_MB
          fi
          if [ "$VSZ_MB" -gt "$SAMPLE_PIDS_PEAK_VSZ" ]; then
            SAMPLE_PIDS_PEAK_VSZ=$VSZ_MB
          fi
          SAMPLE_COUNT=$((SAMPLE_COUNT + 1))
        fi
      fi
    done
    sleep 1
  done
) &
SAMPLER_PID=$!

# 跑测试（强超时）
START_TS=$(date +%s)
timeout --kill-after=15s "${HARD_TIMEOUT}s" go test ./internal/... \
  -count=1 \
  -timeout="${PACKAGE_TIMEOUT}s" \
  -v \
  > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log"
EXIT=$?
END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

# 杀 sampler
kill -TERM $SAMPLER_PID 2>/dev/null
wait $SAMPLER_PID 2>/dev/null

# 兜底：超时时强杀残留
if [ $EXIT -eq 124 ] || [ $EXIT -eq 137 ]; then
  echo "[diagnose] HARD TIMEOUT — killing residual go test processes" >&2
  pkill -9 -f "go test" 2>/dev/null
  pkill -9 -f "go-build" 2>/dev/null
  pkill -9 -f "compile" 2>/dev/null
fi

# 写摘要
cat > "$LOG_DIR/summary.txt" <<EOF
========================================
Go Test Diagnostic Summary
========================================
start_time:        $(date -d "@$START_TS" -Iseconds)
end_time:          $(date -d "@$END_TS" -Iseconds)
duration_sec:      $DURATION
exit_code:         $EXIT
exit_meaning:      $([ $EXIT -eq 0 ] && echo "OK" || ([ $EXIT -eq 124 ] && echo "TIMEOUT" || ([ $EXIT -eq 137 ] && echo "KILLED_OOM" || echo "TEST_FAILURE")))
peak_rss_mb:       $SAMPLE_PIDS_PEAK_RSS
peak_vsz_mb:       $SAMPLE_PIDS_PEAK_VSZ
sample_count:      $SAMPLE_COUNT

=== stdout 末尾 20 行 ===
$(tail -20 "$LOG_DIR/stdout.log" 2>/dev/null || echo "(empty)")

=== stderr 末尾 30 行 ===
$(tail -30 "$LOG_DIR/stderr.log" 2>/dev/null || echo "(empty)")

=== 退出码含义 ===
  0   = 全部通过
  1   = 有测试失败
  2   = go test 自身错误
  124 = 超时被 timeout(1) 杀
  137 = OOM 被 kill -9 杀
EOF

cat "$LOG_DIR/summary.txt"
echo ""
echo "[diagnose] full logs: $LOG_DIR"
echo "[diagnose] resource samples: $SAMPLER_OUT"
