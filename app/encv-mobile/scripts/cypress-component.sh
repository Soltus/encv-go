#!/usr/bin/env bash
# scripts/cypress-component.sh
# =====================================================
# Cypress 组件测试唯一入口 + 分模块守卫 + 硬超时。
# 强保证：无论内部发生什么，HARD_TIMEOUT 后必返回。
#
# 设计参考：scripts/test-go.sh（Go 测试编排体系）
#
# 核心约束：
#   - 无参数默认触发守卫（拒绝瞎跑全部，CI 才允许全量）
#   - 单 spec 永远合法（模糊匹配）
#   - 多 spec 用逗号分隔
#   - 全量必须 ENCV_TEST_FULL=1
#
# 用法：
#   bash scripts/cypress-component.sh                                   # ❌ 守卫拒绝（缺参数）
#   bash scripts/cypress-component.sh TaskTimeline                      # ✅ 单 spec 模糊匹配
#   bash scripts/cypress-component.sh "TaskTimeline,TaskBasicInfo"       # ✅ 多 spec
#   bash scripts/cypress-component.sh --spec "**/Tasks*.cy.ts"          # ✅ 透传 cypress flag
#   ENCV_TEST_FULL=1 bash scripts/cypress-component.sh                  # ✅ 全量（CI 专用）
#   HARD_TIMEOUT=300 bash scripts/cypress-component.sh Tasks            # 自定义超时
#
# 退出码：
#   0   = OK
#   1   = TEST_FAILURE
#   64  = GUARD_REJECTED（调用方式错误，参考 sysexits.h EX_USAGE）
#   124 = TIMEOUT（硬超时）
#   137 = OOM_KILLED（OOM kill）
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# ====================================================
# === 参数解析：分离 SPECS（spec 名）与 FLAGS（透传给 cypress） ===
# ====================================================
# 类似 test-go.sh 的 PKGS vs FLAGS 分离。
# - 无 - 前缀且不是 -spec/-browser/.. 的 value → 视为 spec 名（模糊匹配用）
# - -开头的 → cypress flag，原样透传
# ====================================================

RAW_ARGS=("$@")

# 收集 spec 名（无 - 前缀的 token，且不是 flag 的 value）
SPEC_NAMES=""
EXTRA_FLAGS=""
skip_next=0

for token in "${RAW_ARGS[@]}"; do
  # 跳过 flag 的 value
  if [ $skip_next -eq 1 ]; then
    skip_next=0
    EXTRA_FLAGS="$EXTRA_FLAGS $token"
    continue
  fi

  # -spec / -browser / -run / -env 等 flag，下一个 token 是 value
  if [[ "$token" == "-"* ]]; then
    EXTRA_FLAGS="$EXTRA_FLAGS $token"
    # 常见带 value 的 flag
    case "$token" in
      -spec|--spec|-browser|--browser|-env|--env|-config|--config)
        skip_next=1
        ;;
    esac
    continue
  fi

  # 无 - 前缀 → spec 名（用于模糊匹配）
  SPEC_NAMES="$SPEC_NAMES $token"
done
SPEC_NAMES="${SPEC_NAMES# }"
EXTRA_FLAGS="${EXTRA_FLAGS# }"

# ====================================================
# === 守卫层 1：全量运行必须 ENCV_TEST_FULL=1 ===
# ====================================================
# 检测：无 spec 参数 + 无 --spec flag → 视为"默认全量" → 守卫拒绝
# 除非显式 ENCV_TEST_FULL=1
# ====================================================

has_spec_flag=0
for token in "${RAW_ARGS[@]}"; do
  if [[ "$token" == "-spec" ]] || [[ "$token" == "--spec" ]]; then
    has_spec_flag=1
    break
  fi
done

if [ -z "$SPEC_NAMES" ] && [ $has_spec_flag -eq 0 ]; then
  # 没有指定 spec → 企图跑全部
  if [ "${ENCV_TEST_FULL:-0}" != "1" ]; then
    cat >&2 <<'GUARD_ERROR_EOF'
╔══════════════════════════════════════════════════════════╗
║  [cypress-component] 全量运行守卫触发！立即终止。       ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  ❌ 检测到未指定任何 spec（企图跑全部）。                  ║
║                                                          ║
║  ❓ 为什么禁止：                                          ║
║    ① 全量 Cypress 组件测试动辄 5-10min+                   ║
║    ② 本地开发应按需跑单个 spec 快速验证                    ║
║    ③ CI 才有跑全量的需求（ENCV_TEST_FULL=1）              ║
║                                                          ║
║  ✅ 单 spec（永远合法，模糊匹配）：                        ║
║    bash scripts/cypress-component.sh TaskTimeline        ║
║    bash scripts/cypress-component.sh Tasks               ║
║    bash scripts/cypress-component.sh GroupDetail         ║
║                                                          ║
║  ✅ 多 spec（逗号分隔）：                                 ║
║    bash scripts/cypress-component.sh "Tasks,GroupDetail" ║
║                                                          ║
║  ✅ 全量（必须显式声明 ENCV_TEST_FULL=1，CI 专用）：      ║
║    ENCV_TEST_FULL=1 bash scripts/cypress-component.sh   ║
║                                                          ║
║  ✅ 透传 cypress flag：                                   ║
║    bash scripts/cypress-component.sh --spec "**/Tasks*.cy.ts" ║
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
# 如果指定了 SPEC_NAMES，把每个名字展开为完整路径
# 支持逗号分隔
# ====================================================

SPEC_ARG=""
if [ -n "$SPEC_NAMES" ]; then
  IFS=' ' read -ra SPEC_NAME_ARR <<< "$SPEC_NAMES"
  SPEC_PATHS=()
  for name in "${SPEC_NAME_ARR[@]}"; do
    # 支持逗号分隔的多个 spec
    IFS=',' read -ra COMMA_SPLIT <<< "$name"
    for s in "${COMMA_SPLIT[@]}"; do
      s_trimmed="$(echo "$s" | xargs)"
      if [ -z "$s_trimmed" ]; then continue; fi
      # 找匹配的 .cy.ts 文件（模糊匹配）
      matched=$(find "$PROJECT_ROOT/cypress/component" -name "*${s_trimmed}*.cy.ts" -type f 2>/dev/null | sort | head -1)
      if [[ -n "$matched" ]]; then
        SPEC_PATHS+=("$matched")
      else
        echo "⚠️  未找到匹配的 spec: $s_trimmed" >&2
      fi
    done
  done

  if [[ ${#SPEC_PATHS[@]} -gt 0 ]]; then
    # 用逗号拼接
    SPEC_ARG="--spec $(IFS=,; echo "${SPEC_PATHS[*]}")"
  fi
fi

# 如果用户传了 --spec，优先用用户的（覆盖模糊匹配）
if [ $has_spec_flag -eq 1 ]; then
  SPEC_ARG=""
fi

# ====================================================
# === 默认配置 ===
# ====================================================
HARD_TIMEOUT=${HARD_TIMEOUT:-600}        # 总硬超时 10min
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

# xvfb-run 是否可用
XVFB_PREFIX=""
if command -v xvfb-run >/dev/null 2>&1; then
  XVFB_PREFIX="xvfb-run -a --server-args=\"-screen 0 1024x768x24\""
fi

# PM2_HOME 避免污染用户 pm2
export PM2_HOME=${PM2_HOME:-/tmp/cypress-pm2}

# ====================================================
# === pre-flight 清理 ===
# ====================================================

# 清理旧 cypress 进程（上次崩溃残留）
# 注意：不能 pkill -f "cypress"，否则会把当前 bash 进程也杀掉（命令行有 "cypress-component"）
STUCK_CYPRESS=$(pgrep -f "cypress run|cypress open" 2>/dev/null | wc -l || echo 0)
STUCK_CYPRESS=$(echo "$STUCK_CYPRESS" | tr -d '[:space:]')
if [ "${STUCK_CYPRESS:-0}" -gt 0 ] 2>/dev/null; then
  echo "[cypress-component][pre-flight] killing $STUCK_CYPRESS stuck cypress processes"
  pkill -9 -f "cypress run|cypress open" 2>/dev/null || true
  sleep 1
fi

# 清理旧 electron 进程（只杀 cypress 启动的，不误杀其他 electron 应用）
STUCK_ELECTRON=$(pgrep -f "electron.*cypress|cypress.*electron" 2>/dev/null | wc -l || echo 0)
STUCK_ELECTRON=$(echo "$STUCK_ELECTRON" | tr -d '[:space:]')
if [ "${STUCK_ELECTRON:-0}" -gt 0 ] 2>/dev/null; then
  echo "[cypress-component][pre-flight] killing $STUCK_ELECTRON stuck electron processes"
  pkill -9 -f "electron.*cypress|cypress.*electron" 2>/dev/null || true
  sleep 1
fi

# 保留最近 10 次测试日志，更早的清理
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

# 构建完整命令
RUN_CMD="npx cypress run --component --browser $BROWSER $SPEC_ARG $EXTRA_FLAGS"

# 有 xvfb 就包一层
if [ -n "$XVFB_PREFIX" ]; then
  RUN_CMD="$XVFB_PREFIX $RUN_CMD"
fi

echo "[cypress-component] command: $RUN_CMD" | tee -a "$SUMMARY_FILE"

# 用 timeout 包一层，确保硬超时
# --kill-after=30s：SIGTERM 后 30s 还没死就 SIGKILL
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

# 退出码含义
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
