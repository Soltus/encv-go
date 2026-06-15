#!/usr/bin/env bash
# scripts/test-all-go.sh
# =====================================================
# Go 测试唯一外层入口（wraps test-go.sh）。
#
# 行为：
#   1. pre-flight 清理（call kill-orphan-children.sh + 清 /tmp）
#   2. 调用 test-go.sh 跑测试
#   3. 合并 .test-runs/*/reports-*.json → report-all.json
#   4. 合并 .test-runs/*/probe-*.json → probe-all.json
#   5. 打印人类可读摘要
#   6. 若有失败 case 或崩溃 → exit != 0
#
# 用法：
#   bash scripts/test-all-go.sh
#
# 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 2）
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

LOG_ROOT=${LOG_ROOT:-.test-runs}

echo "==================================="
echo "[test-all-go] started at $(date -Iseconds)"
echo "[test-all-go] log_root=$LOG_ROOT"
echo "==================================="

# ── pre-flight：调用现成的 kill-orphan-children.sh ──
if [ -x "$SCRIPT_DIR/kill-orphan-children.sh" ]; then
  echo "[pre-flight] running kill-orphan-children.sh"
  bash "$SCRIPT_DIR/kill-orphan-children.sh" 2>/dev/null || true
fi

# ── pre-flight：清大文件 ──
find /tmp -maxdepth 1 -name "encv-*" -size +100M -exec rm -rf {} \; 2>/dev/null || true

# ── 跑测试 ──
START_TS=$(date +%s)
# PKGS 环境变量优先，否则用所有位置参数，最后默认 ./internal/...
if [ -n "${PKGS:-}" ]; then
  bash "$SCRIPT_DIR/test-go.sh"
else
  bash "$SCRIPT_DIR/test-go.sh" "$@"
fi
TEST_EXIT=$?
END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

echo ""
echo "==================================="
echo "[test-all-go] test-go.sh exit=$TEST_EXIT duration=${DURATION}s"
echo "==================================="

# ── 合并报告 ──
mkdir -p "$LOG_ROOT"

REPORT_ALL="$LOG_ROOT/report-all.json"
PROBE_ALL="$LOG_ROOT/probe-all.json"

# 合并 JSON 报告（用 python 更稳）
if command -v python3 >/dev/null 2>&1; then
  python3 "$SCRIPT_DIR/merge-test-reports.py" "$LOG_ROOT" \
    > "$REPORT_ALL" 2>/dev/null || echo '{"total":0,"passed":0,"failed":0,"skipped":0}' > "$REPORT_ALL"
else
  echo '{"total":0,"passed":0,"failed":0,"skipped":0,"note":"python3 not found"}' > "$REPORT_ALL"
fi

# ── 打印摘要 ──
echo ""
echo "==================================="
echo "[test-all-go] SUMMARY"
echo "==================================="
if [ -f "$REPORT_ALL" ] && command -v python3 >/dev/null 2>&1; then
  python3 -c "
import json
data = json.load(open('$REPORT_ALL'))
print(f'Total:  {data.get(\"total\", 0)}')
print(f'Passed: {data.get(\"passed\", 0)}')
print(f'Failed: {data.get(\"failed\", 0)}')
print(f'Skipped: {data.get(\"skipped\", 0)}')
print()
fs = data.get('failures', [])
if fs:
    print('=== FAILURES ===')
    for f in fs[:20]:
        print(f'  ✗ {f.get(\"name\", \"?\")}: {f.get(\"error_msg\", \"\")[:120]}')
    if len(fs) > 20:
        print(f'  ... and {len(fs) - 20} more')
crashes = data.get('crashes', [])
if crashes:
    print()
    print('=== CRASHES (forensic dumps) ===')
    for c in crashes[:10]:
        print(f'  💥 {c}')
"
fi

# ── 检查崩溃堆栈 ──
CRASH_COUNT=0
if [ -d "$LOG_ROOT/crashes" ]; then
  CRASH_COUNT=$(find "$LOG_ROOT/crashes" -mindepth 2 -name "*.stack" 2>/dev/null | wc -l)
fi
if [ "$CRASH_COUNT" -gt 0 ]; then
  echo ""
  echo "💥 $CRASH_COUNT crash dumps in $LOG_ROOT/crashes/"
  find "$LOG_ROOT/crashes" -mindepth 2 -name "*.stack" 2>/dev/null | head -10
fi

# ── 退出码策略 ──
if [ $TEST_EXIT -eq 124 ] || [ $TEST_EXIT -eq 137 ]; then
  echo ""
  echo "[FATAL] test process was killed (timeout/OOM)"
  exit 137
fi
exit $TEST_EXIT
