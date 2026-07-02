#!/usr/bin/env bash
# scripts/test-frontend.sh
# =====================================================
# 前端 vitest 唯一入口 + 多包运行守卫。
# 强保证：无论内部发生什么，HARD_TIMEOUT 后必返回。
#
# 🆕 2026-07-02 前端测试入口收编（与 test-go.sh 对齐）：
#   - 浅层（shallow）：单文件/单模块 vitest 跑（默认 -short）
#   - 全量（full）：ENCV_TEST_FULL=1 跑全包（CI 专用，~2-5min）
#   - 默认 -bail=1（失败早停，本地友好）
#
# 用法：
#   bash scripts/test-frontend.sh                                    # 缺 FILES = 全包（需 ENCV_TEST_FULL=1）
#   bash scripts/test-frontend.sh src/composables/useFileList.ts     # ✅ 单文件（shallow）
#   bash scripts/test-frontend.sh src/views/__tests__/Files.test.ts  # ✅ 单文件
#   bash scripts/test-frontend.sh -t "clientSearchTokenize"          # ✅ -t 测试名匹配
#   ENCV_TEST_FULL=1 bash scripts/test-frontend.sh                   # ✅ 全量（CI 模式）
#   HARD_TIMEOUT=300 bash scripts/test-frontend.sh                   # 自定义总超时
#
# 行为：
#   1. pre-flight 守卫（FILES 合规性 + 多文件必须 ENCV_TEST_FULL）
#   2. pre-flight 清理（上轮残留的 .test-runs/）
#   3. vitest run（带 --bail + 外层 timeout 兜底 + 默认 --reporter=default）
#   4. 退出时清理残留 vitest 进程
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
MOBILE_DIR="$ROOT/app/encv-mobile"
cd "$MOBILE_DIR"

# ====================================================
# === 参数解析：分离 FILES（文件路径）与 FLAGS（透传给 vitest） ===
# ====================================================
# 注意：vitest 形如 `vitest run -t "clientSearch" src/...`：
#   - -t / -reporter / -bail / -coverage                → vitest flag
#   - -t 的 value（"clientSearch"）紧跟 -t             → vitest flag value（跳过）
#   - src/composables/useFileList.ts                   → file path
#   - -t 后无 value 是错误用法（vitest 自己会报错）
# ====================================================

RAW_ARGS=("$@")

# 提取纯文件路径（用于 vitest 调用）
FILES=""
skip_next=0
for token in "${RAW_ARGS[@]}"; do
  # 跳过 -flag 的 value
  if [ $skip_next -eq 1 ]; then
    skip_next=0
    continue
  fi
  # 标记下一个 token 是 value 的 flag
  case "$token" in
    -t|--testNamePattern)
      skip_next=1
      continue
      ;;
  esac
  # 跳过 -flag 本身（含等号的合并形如 -t=clientSearch）
  if [[ "$token" == -* ]]; then
    if [[ "$token" == "-t="* ]] || [[ "$token" == "--testNamePattern="* ]]; then
      continue
    fi
    continue
  fi
  FILES="$FILES $token"
done
FILES="${FILES# }"

# ── 守卫：检测多文件 / 全包模式 ──
# 全包模式触发条件：FILES 为空（用户没指定文件），或 FILES 含 glob (*.test.ts) / src/...
is_multifile=0
if [ -z "$FILES" ]; then
  is_multifile=1
else
  for token in $FILES; do
    if [[ "$token" == *"..."* ]] || [[ "$token" == *"*"* ]]; then
      is_multifile=1
      break
    fi
    # 多目录模式（多个独立路径）也视为多文件
  done
  # 多文件 = 多个 token（每个 token 视为一个文件/目录）
  TOKEN_COUNT=$(echo "$FILES" | wc -w)
  if [ "$TOKEN_COUNT" -gt 1 ]; then
    is_multifile=1
  fi
fi

if [ $is_multifile -eq 1 ] && [ "${ENCV_TEST_FULL:-0}" != "1" ]; then
  cat >&2 <<'GUARD_ERROR_EOF'
╔══════════════════════════════════════════════════════════╗
║  [test-frontend] 多文件/全包运行守卫触发！立即终止。       ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  ❌ 检测到多文件/全包运行模式：                            ║
║     FILES 包含多个路径 / 通配符 / 缺省                    ║
║                                                          ║
║  ❓ 为什么禁止：                                          ║
║    ① vitest 多文件跑动辄 2-5min，PR 自动跑会消耗 GitHub     ║
║       Actions 配额                                       ║
║    ② Layer1 应只跑单文件/单模块，Layer2 才跑全量           ║
║                                                          ║
║  ✅ 单文件（永远合法，本地开发友好）：                      ║
║    bash scripts/test-frontend.sh src/composables/useFileList.ts
║    bash scripts/test-frontend.sh src/views/__tests__/Files.test.ts
║    bash scripts/test-frontend.sh -t "clientSearchTokenize" src/composables
║                                                          ║
║  ✅ 全量（必须显式声明 ENCV_TEST_FULL=1，CI 专用）：       ║
║    ENCV_TEST_FULL=1 bash scripts/test-frontend.sh         ║
║                                                          ║
║  💡 跑单模块的所有测试：                                  ║
║    bash scripts/test-frontend.sh src/composables          # 单目录
║    bash scripts/test-frontend.sh src/composables/useFileList.ts
║                                                          ║
║  💡 测试名匹配（-t / --testNamePattern）：                 ║
║    bash scripts/test-frontend.sh -t "split" src/utils     # 跑 split 相关的
╚══════════════════════════════════════════════════════════╝
GUARD_ERROR_EOF
  exit 64  # EX_USAGE
fi

# ── 守卫通过：剥离 FILES 中的 -flag（防御性补刀） ──
FILES_CLEAN=""
for token in $FILES; do
  if [[ "$token" == -* ]]; then continue; fi
  FILES_CLEAN="$FILES_CLEAN $token"
done
FILES="$FILES_CLEAN"
FILES="${FILES# }"

# ====================================================
# === 默认行为（--bail=1） ===
# ====================================================
HARD_TIMEOUT=${HARD_TIMEOUT:-300}        # 总硬超时 5min
LOG_ROOT=${LOG_ROOT:-.test-runs}

# --bail 开关：本地单文件早停，CI 全量不早停
BUIL_FLAG="--bail=1"
if [ "${ENCV_TEST_FULL:-0}" = "1" ]; then
  BUIL_FLAG=""  # 全量模式不早停
fi

# --reporter 开关
REPORTER_FLAG=${REPORTER_FLAG:-"--reporter=default"}

# -t 测试名模式（如果传入）
TEST_NAME_FLAG=""
skip_next=0
for token in "${RAW_ARGS[@]}"; do
  if [ $skip_next -eq 1 ]; then
    skip_next=0
    continue
  fi
  case "$token" in
    -t)
      TEST_NAME_FLAG="-t"
      skip_next=1
      continue
      ;;
    --testNamePattern)
      TEST_NAME_FLAG="--testNamePattern"
      skip_next=1
      continue
      ;;
    -t=*)
      TEST_NAME_FLAG="-t=\"${token#-t=}\""
      continue
      ;;
    --testNamePattern=*)
      TEST_NAME_FLAG="--testNamePattern=\"${token#--testNamePattern=}\""
      continue
      ;;
  esac
done

# 提取 -t 的 value
TEST_NAME_VALUE=""
skip_next=0
for token in "${RAW_ARGS[@]}"; do
  if [ $skip_next -eq 1 ]; then
    TEST_NAME_VALUE="$token"
    skip_next=0
    continue
  fi
  case "$token" in
    -t|--testNamePattern)
      skip_next=1
      continue
      ;;
  esac
done

EXTRA_ARGS=${EXTRA_ARGS:-}

TS=$(date +%Y%m%d-%H%M%S)
LOG_DIR="$LOG_ROOT/fe-$TS"
mkdir -p "$LOG_DIR"

echo "==================================="
echo "[test-frontend] started at $(date -Iseconds)"
echo "[test-frontend] files=${FILES:-(all)}"
echo "[test-frontend] test_name=${TEST_NAME_VALUE:-(none)}"
echo "[test-frontend] bail_flag=${BUIL_FLAG:-(disabled, full mode)}"
echo "[test-frontend] hard_timeout=${HARD_TIMEOUT}s"
echo "[test-frontend] log_dir=$LOG_DIR"
echo "===================================" | tee "$LOG_DIR/summary.txt"

# ── pre-flight：清理 .test-runs 旧日志（保留 5 个最新） ──
if [ -d ".test-runs" ]; then
  find .test-runs -maxdepth 1 -mindepth 1 -type d -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn | tail -n +6 | awk '{print $2}' | xargs -r rm -rf 2>/dev/null || true
fi

# ====================================================
# === 拦截 vitest 裸调用（双保险：bash 守卫 + npm script 注释） ===
# ====================================================
# 1. bash 守卫：已通过多文件/单文件检测
# 2. vitest config 内部可在 setup 文件检查 ENCV_TEST_INVOKED_BY
#    （对应 Go 的 testguard 包，未来阶段加入）
# ====================================================
export ENCV_TEST_INVOKED_BY="scripts/test-frontend.sh"

# ── 跑测试（带强超时链） ──
START_TS=$(date +%s)

# shellcheck disable=SC2086
if [ -n "$FILES" ]; then
  # 单文件模式
  timeout --kill-after=15s "${HARD_TIMEOUT}s" npx vitest run \
    $BUIL_FLAG \
    $REPORTER_FLAG \
    $TEST_NAME_FLAG \
    $TEST_NAME_VALUE \
    $EXTRA_ARGS \
    $FILES \
    > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log"
else
  # 全量模式（已通过守卫）
  timeout --kill-after=15s "${HARD_TIMEOUT}s" npx vitest run \
    $BUIL_FLAG \
    $REPORTER_FLAG \
    $TEST_NAME_FLAG \
    $TEST_NAME_VALUE \
    $EXTRA_ARGS \
    > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log"
fi
EXIT=$?

END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

# ── 兜底：超时时强杀残留 ──
if [ $EXIT -eq 124 ] || [ $EXIT -eq 137 ]; then
  echo "[test-frontend] HARD TIMEOUT — killing residual" | tee -a "$LOG_DIR/summary.txt"
  pkill -9 -f "vitest" 2>/dev/null
  pkill -9 -f "node.*vitest" 2>/dev/null
fi

# ── 写摘要 ──
cat >> "$LOG_DIR/summary.txt" <<EOF
========================================
end_time:     $(date -d "@$END_TS" -Iseconds)
duration_sec: $DURATION
exit_code:    $EXIT
exit_meaning: $([ $EXIT -eq 0 ] && echo "OK" || ([ $EXIT -eq 64 ] && echo "GUARD_REJECTED" || ([ $EXIT -eq 124 ] && echo "TIMEOUT" || ([ $EXIT -eq 137 ] && echo "KILLED_OOM" || echo "TEST_FAILURE"))))

=== stderr 摘要 (最后 30 行) ===
$(tail -30 "$LOG_DIR/stderr.log" 2>/dev/null || echo "(empty)")

=== stdout 摘要 (最后 10 行) ===
$(tail -10 "$LOG_DIR/stdout.log" 2>/dev/null || echo "(empty)")
EOF

# ── 打印结果 ──
EXIT_MSG="OK"
[ $EXIT -ne 0 ] && EXIT_MSG="FAIL"
[ $EXIT -eq 64 ] && EXIT_MSG="GUARD_REJECTED"
[ $EXIT -eq 124 ] && EXIT_MSG="TIMEOUT"
[ $EXIT -eq 137 ] && EXIT_MSG="OOM_KILLED"

echo ""
echo "==================================="
echo "[test-frontend] $EXIT_MSG in ${DURATION}s (exit=$EXIT)"
echo "[test-frontend] logs: $LOG_DIR"
echo "==================================="

exit $EXIT
