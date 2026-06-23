#!/usr/bin/env bash
#
# Cypress 组件测试包装脚本
#
# 解决的痛点：
#   1. 之前 tail -40 截断输出，失败详情在前面被截掉，要重跑用 grep 捞
#   2. 现在用 tee 完整持久化 + 失败时自动提取错误摘要
#
# 用法：
#   bash scripts/cypress-component.sh                 # 跑全部
#   bash scripts/cypress-component.sh TaskTimeline    # 跑单个 spec（模糊匹配）
#   bash scripts/cypress-component.sh "TaskTimeline,TaskBasicInfo"  # 多个 spec
#
# 输出：
#   - 终端显示完整输出（不再 tail 截断）
#   - 完整日志持久化到 cypress/results/latest.log
#   - 失败时终端底部打印 "失败摘要" 区块
#
# 退出码：透传 cypress 的退出码（0=通过，非0=失败）
#
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# 结果目录
RESULTS_DIR="$PROJECT_ROOT/cypress/results"
mkdir -p "$RESULTS_DIR"
LOG_FILE="$RESULTS_DIR/latest.log"
SUMMARY_FILE="$RESULTS_DIR/latest-summary.txt"

# 构建 spec 参数
SPEC_ARG=""
if [[ $# -gt 0 ]]; then
  # 把逗号分隔的 spec 名展开为完整路径（支持模糊匹配）
  IFS=',' read -ra SPEC_NAMES <<< "$1"
  SPEC_PATHS=()
  for name in "${SPEC_NAMES[@]}"; do
    name_trimmed="$(echo "$name" | xargs)"
    # 找匹配的 .cy.ts 文件
    matched=$(find "$PROJECT_ROOT/cypress/component" -name "${name_trimmed}*.cy.ts" -type f 2>/dev/null | head -1)
    if [[ -n "$matched" ]]; then
      SPEC_PATHS+=("$matched")
    else
      echo "⚠️  未找到匹配的 spec: $name_trimmed" >&2
    fi
  done
  if [[ ${#SPEC_PATHS[@]} -gt 0 ]]; then
    # 用逗号拼接
    SPEC_ARG="--spec $(IFS=,; echo "${SPEC_PATHS[*]}")"
  fi
fi

# 运行命令（用 xvfb-run 保证 headless 环境可用）
# PM2_HOME 避免污染用户的 pm2
RUN_CMD="PM2_HOME=/tmp/cypress-pm2 xvfb-run -a --server-args=\"-screen 0 1024x768x24\" \
npx cypress run --component --browser electron $SPEC_ARG"

echo "▶  Cypress Component Tests"
echo "   日志: $LOG_FILE"
echo "   命令: $RUN_CMD"
echo "─────────────────────────────────────────────────────"

# 执行并 tee 到日志文件
set +e
eval "$RUN_CMD" 2>&1 | tee "$LOG_FILE"
EXIT_CODE=${PIPESTATUS[0]}
set -e

# ── 失败摘要 ──────────────────────────────────────────
if [[ $EXIT_CODE -ne 0 ]]; then
  echo ""
  echo "═══════════════════════════════════════════════════════"
  echo "❌  失败摘要（完整日志见 $LOG_FILE）"
  echo "═══════════════════════════════════════════════════════"

  # 提取所有失败的测试用例名（"1) ..." 格式的行）
  echo ""
  echo "📋 失败用例列表："
  grep -n "^[[:space:]]*[0-9]\+) " "$LOG_FILE" | head -30 || true

  echo ""
  echo "─────────────────────────────────────────────────────"
  echo "🔍 断言错误详情（每个失败取前 5 行上下文）："
  echo ""

  # 找所有 AssertionError / Error 的位置，输出前后上下文
  grep -n -A 8 "AssertionError\|Error: " "$LOG_FILE" | head -120 || true

  echo ""
  echo "═══════════════════════════════════════════════════════"
  echo "💡 查看完整日志: cat $LOG_FILE"
  echo "💡 查看第 N 行附近: sed -n 'N-10,N+10p' $LOG_FILE"
  echo "═══════════════════════════════════════════════════════"
fi

exit $EXIT_CODE
