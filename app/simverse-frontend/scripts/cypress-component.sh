#!/usr/bin/env bash
# scripts/cypress-component.sh
# =====================================================
# SimVerse Cypress 组件测试入口 + 分模块守卫 + 硬超时。
# 设计参考：app/encv-mobile/scripts/cypress-component.sh
#
# 核心约束：
#   - 无参数默认触发守卫（拒绝瞎跑全部，CI 才允许全量）
#   - 单 spec 永远合法（模糊匹配）
#   - 多 spec 用逗号分隔
#   - 全量必须 SIMVERSE_TEST_FULL=1
#
# 用法：
#   bash scripts/cypress-component.sh                                    # ❌ 守卫拒绝（缺参数）
#   bash scripts/cypress-component.sh SimverseHome                       # ✅ 单 spec 模糊匹配
#   bash scripts/cypress-component.sh "SimverseHome,SimverseWorld"        # ✅ 多 spec
#   bash scripts/cypress-component.sh --spec "**/Simverse*.cy.ts"         # ✅ 透传 cypress flag
#   SIMVERSE_TEST_FULL=1 bash scripts/cypress-component.sh                # ✅ 全量（CI 专用）
#   HARD_TIMEOUT=300 bash scripts/cypress-component.sh Simverse           # 自定义超时
#
# 退出码：
#   0   = OK
#   1   = TEST_FAILURE
#   64  = GUARD_REJECTED（调用方式错误）
#   124 = TIMEOUT（硬超时）
#   137 = OOM_KILLED（OOM kill）
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 如果是 simverse-frontend 目录下的脚本，则 PROJECT_ROOT 就是 simverse-frontend
# 否则需要在 simverse-frontend 目录下执行
if [ -f "$PROJECT_ROOT/cypress.config.ts" ]; then
  SIMVERSE_ROOT="$PROJECT_ROOT"
else
  # 尝试从 app 目录进入 simverse-frontend
  SIMVERSE_ROOT="$PROJECT_ROOT/simverse-frontend"
  if [ ! -f "$SIMVERSE_ROOT/cypress.config.ts" ]; then
    echo "❌ 错误：请在 simverse-frontend 目录下执行此脚本" >&2
    exit 1
  fi
fi

cd "$SIMVERSE_ROOT"

# ====================================================
# === 参数解析：分离 SPECS（spec 名）与 FLAGS ===
# ====================================================

RAW_ARGS=("$@")

SPEC_NAMES=""
EXTRA_FLAGS=""
skip_next=0

for token in "${RAW_ARGS[@]}"; do
  if [ $skip_next -eq 1 ]; then
    skip_next=0
    EXTRA_FLAGS="$EXTRA_FLAGS $token"
    continue
  fi

  if [[ "$token" == "-"* ]]; then
    EXTRA_FLAGS="$EXTRA_FLAGS $token"
    case "$token" in
      -spec|--spec|-browser|--browser|-env|--env|-config|--config)
        skip_next=1
        ;;
    esac
    continue
  fi

  SPEC_NAMES="$SPEC_NAMES $token"
done
SPEC_NAMES="${SPEC_NAMES# }"
EXTRA_FLAGS="${EXTRA_FLAGS# }"

# ====================================================
# === 守卫层 1：全量运行必须 SIMVERSE_TEST_FULL=1 ===
# ====================================================

has_spec_flag=0
for token in "${RAW_ARGS[@]}"; do
  if [[ "$token" == "-"* ]] || [[ "$token" == "--spec" ]]; then
    has_spec_flag=1
    break
  fi
done

# 检查是否有 --spec flag
for ((i=0; i<${#RAW_ARGS[@]}; i++)); do
  if [[ "${RAW_ARGS[$i]}" == "--spec" ]] || [[ "${RAW_ARGS[$i]}" == "-spec" ]]; then
    has_spec_flag=1
    break
  fi
done

if [ -z "$SPEC_NAMES" ] && [ $has_spec_flag -eq 0 ]; then
  if [ "${SIMVERSE_TEST_FULL:-0}" != "1" ]; then
    cat >&2 <<'GUARD_ERROR_EOF'
╔══════════════════════════════════════════════════════════╗
║  [cypress-component] 全量运行守卫触发！立即终止。       ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  ❌ 检测到未指定任何 spec（企图跑全部）。                  ║
║                                                          ║
║  ✅ 单 spec（永远合法，模糊匹配）：                        ║
║    bash scripts/cypress-component.sh SimverseHome        ║
║    bash scripts/cypress-component.sh SimverseWorld       ║
║                                                          ║
║  ✅ 多 spec（逗号分隔）：                                 ║
║    bash scripts/cypress-component.sh "Home,World"        ║
║                                                          ║
║  ✅ 全量（必须显式声明 SIMVERSE_TEST_FULL=1，CI 专用）：  ║
║    SIMVERSE_TEST_FULL=1 bash scripts/cypress-component.sh║
║                                                          ║
║  ✅ 透传 cypress flag：                                   ║
║    bash scripts/cypress-component.sh --spec "**/*.cy.ts" ║
║                                                          ║
║  💡 查看所有 spec 列表：                                  ║
║    ls cypress/component/                                 ║
╚══════════════════════════════════════════════════════════╝
GUARD_ERROR_EOF
    exit 64  # EX_USAGE
  fi
fi

# ====================================================
# === spec 模糊匹配解析 ===
# ====================================================

SPEC_ARG=""
if [ -n "$SPEC_NAMES" ]; then
  IFS=' ' read -ra SPEC_NAME_ARR <<< "$SPEC_NAMES"
  SPEC_PATHS=()
  for name in "${SPEC_NAME_ARR[@]}"; do
    IFS=',' read -ra COMMA_SPLIT <<< "$name"
    for s in "${COMMA_SPLIT[@]}"; do
      s_trimmed="$(echo "$s" | xargs)"
      if [ -z "$s_trimmed" ]; then continue; fi
      matched=$(find "$SIMVERSE_ROOT/cypress/component" -name "*${s_trimmed}*.cy.ts" -type f 2>/dev/null | sort | head -1)
      if [[ -n "$matched" ]]; then
        SPEC_PATHS+=("$matched")
      else
        echo "⚠️  未找到匹配的 spec: $s_trimmed" >&2
      fi
    done
  done

  if [[ ${#SPEC_PATHS[@]} -gt 0 ]]; then
    SPEC_ARG="--spec $(IFS=,; echo "${SPEC_PATHS[*]}")"
  fi
fi

if [ $has_spec_flag -eq 1 ]; then
  SPEC_ARG=""
fi

# ====================================================
# === 默认配置 ===
# ====================================================
HARD_TIMEOUT=${HARD_TIMEOUT:-600}
LOG_ROOT=${LOG_ROOT:-.test-runs}
BROWSER=${BROWSER:-electron}

TS=$(date +%Y%m%d-%H%M%S)
LOG_DIR="$LOG_ROOT/cypress-$TS"
mkdir -p "$LOG_DIR"

STDOUT_LOG="$LOG_DIR/stdout.log"
STDERR_LOG="$LOG_DIR/stderr.log"
SUMMARY_FILE="$LOG_DIR/summary.txt"

# ====================================================
# === 环境准备 ===
# ====================================================

XVFB_PREFIX=""
if command -v xvfb-run >/dev/null 2>&1; then
  XVFB_PREFIX="xvfb-run -a --server-args=\"-screen 0 1024x768x24\""
fi

export PM2_HOME=${PM2_HOME:-/tmp/cypress-pm2}

# ====================================================
# === pre-flight 清理 ===
# ====================================================

STUCK_CYPRESS=$(pgrep -f "cypress run|cypress open" 2>/dev/null | wc -l || echo 0)
STUCK_CYPRESS=$(echo "$STUCK_CYPRESS" | tr -d '[:space:]')
if [ "${STUCK_CYPRESS:-0}" -gt 0 ] 2>/dev/null; then
  echo "[cypress-component][pre-flight] killing $STUCK_CYPRESS stuck cypress processes"
  pkill -9 -f "cypress run|cypress open" 2>/dev/null || true
  sleep 1
fi

STUCK_ELECTRON=$(pgrep -f "electron.*cypress|cypress.*electron" 2>/dev/null | wc -l || echo 0)
STUCK_ELECTRON=$(echo "$STUCK_ELECTRON" | tr -d '[:space:]')
if [ "${STUCK_ELECTRON:-0}" -gt 0 ] 2>/dev/null; then
  echo "[cypress-component][pre-flight] killing $STUCK_ELECTRON stuck electron processes"
  pkill -9 -f "electron.*cypress|cypress.*electron" 2>/dev/null || true
  sleep 1
fi

if [ -d "$LOG_ROOT" ]; then
  find "$LOG_ROOT" -mindepth 1 -maxdepth 1 -type d -name "cypress-*" -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn | tail -n +11 | awk '{print $2}' | xargs -r rm -rf 2>/dev/null || true
fi

# ====================================================
# === 启动信息 ===
# ====================================================

echo "==================================="
echo "[cypress-component] started at $(date -Iseconds)"
echo "[cypress-component] specs=$SPEC_NAMES"
echo "[cypress-component] spec_arg=$SPEC_ARG"
echo "[cypress-component] extra_flags=$EXTRA_FLAGS"
echo "[cypress-component] browser=$BROWSER"
echo "[cypress-component] hard_timeout=${HARD_TIMEOUT}s"
echo "[cypress-component] log_dir=$LOG_DIR"
echo "===================================" | tee "$SUMMARY_FILE"

# ====================================================
# === 运行测试（带硬超时） ===
# ====================================================

START_TS=$(date +%s)

RUN_CMD="npx cypress run --component --browser $BROWSER $SPEC_ARG $EXTRA_FLAGS"

if [ -n "$XVFB_PREFIX" ]; then
  RUN_CMD="$XVFB_PREFIX $RUN_CMD"
fi

echo "[cypress-component] command: $RUN_CMD" | tee -a "$SUMMARY_FILE"

set +e
timeout --kill-after=30s "${HARD_TIMEOUT}s" bash -c "$RUN_CMD" \
  > "$STDOUT_LOG" 2> "$STDERR_LOG"
EXIT=$?
set -e

END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

# ====================================================
# === 超时 / OOM 兜底：强杀残留进程 ===
# ====================================================
if [ $EXIT -eq 124 ] || [ $EXIT -eq 137 ]; then
  echo "[cypress-component] HARD TIMEOUT / OOM — killing residual processes" | tee -a "$SUMMARY_FILE"
  pkill -9 -f "cypress run\|cypress open" 2>/dev/null || true
  pkill -9 -f "electron.*cypress\|cypress.*electron" 2>/dev/null || true
  pkill -9 -f "xvfb-run.*cypress" 2>/dev/null || true
  pkill -9 -f "chrome.*cypress\|cypress.*chrome" 2>/dev/null || true
fi

# ====================================================
# === 写摘要 ===
# ====================================================

EXIT_MEANING="UNKNOWN"
case $EXIT in
  0)   EXIT_MEANING="OK" ;;
  1)   EXIT_MEANING="TEST_FAILURE" ;;
  64)  EXIT_MEANING="GUARD_REJECTED" ;;
  124) EXIT_MEANING="TIMEOUT" ;;
  137) EXIT_MEANING="OOM_KILLED" ;;
esac

cat >> "$SUMMARY_FILE" <<EOF
========================================
end_time:      $(date -d "@$END_TS" -Iseconds)
duration_sec:  $DURATION
exit_code:     $EXIT
exit_meaning:  $EXIT_MEANING
log_dir:       $LOG_DIR

=== 失败用例列表（最多 30 个）===
$(grep -n "^[[:space:]]*[0-9]*) " "$STDOUT_LOG" 2>/dev/null | head -30 || echo "(none / all passed)")

=== 断言错误摘要（每个失败取前 8 行上下文，最多 120 行）===
$(grep -n -A 8 "AssertionError\|Error: " "$STDOUT_LOG" 2>/dev/null | head -120 || echo "(no assertion errors)")

=== stderr 最后 30 行 ===
$(tail -30 "$STDERR_LOG" 2>/dev/null || echo "(empty)")

=== stdout 最后 20 行 ===
$(tail -20 "$STDOUT_LOG" 2>/dev/null || echo "(empty)")
EOF

# ====================================================
# === 打印结果 ===
# ====================================================

echo ""
echo "==================================="

if [ $EXIT -eq 0 ]; then
  echo "[cypress-component] ✅ OK in ${DURATION}s"
  echo "[cypress-component] logs: $LOG_DIR"
else
  echo "[cypress-component] ❌ $EXIT_MEANING in ${DURATION}s (exit=$EXIT)"
  echo "[cypress-component] logs: $LOG_DIR"
  echo ""
  echo "─────────────────────────────────────────────────────"
  echo "📋 失败用例："
  grep -n "^[[:space:]]*[0-9]*) " "$STDOUT_LOG" 2>/dev/null | head -20 || echo "(none)"
  echo ""
  echo "─────────────────────────────────────────────────────"
  echo "🔍 错误摘要："
  grep -n -A 5 "AssertionError\|Error: " "$STDOUT_LOG" 2>/dev/null | head -60 || echo "(no errors)"
fi

echo "==================================="
echo "💡 查看完整 stdout: cat $STDOUT_LOG"
echo "💡 查看完整 stderr: cat $STDERR_LOG"
echo "💡 查看摘要: cat $SUMMARY_FILE"
echo "==================================="

exit $EXIT
