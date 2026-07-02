#!/usr/bin/env bash
# scripts/test-frontend.sh
# =====================================================
# 前端 vitest 唯一入口 + 多包运行守卫。
# 强保证：无论内部发生什么，HARD_TIMEOUT 后必返回。
#
# 🆕 2026-07-02 前端测试入口收编（与 test-go.sh 对齐）：
#   - 浅层（shallow）：单文件/单模块 vitest 跑（默认 --bail=1）
#   - 全量（full）：ENCV_TEST_FULL=1 跑全包（CI 专用）
#
# 🆕 2026-07-02 安全加固（沙箱保护）：
#   - 删除 pkill -f "vitest"（会误杀同沙箱其他用户的 vitest 进程）
#   - 改用进程组 PGID 跟踪：timeout 时 SIGTERM → 进程组 → SIGKILL
#   - 拒绝在无 package.json 的目录运行（避免 cd 错位置）
#   - mktemp 临时日志目录（避免污染源目录）
#
# 用法：
#   bash scripts/test-frontend.sh                                    # 缺 FILES = 全包（需 ENCV_TEST_FULL=1）
#   bash scripts/test-frontend.sh src/composables/__tests__/useFileList.test.ts
#   bash scripts/test-frontend.sh src/views/__tests__/Files.test.ts
#   bash scripts/test-frontend.sh -t "clientSearchTokenize" src/composables
#   ENCV_TEST_FULL=1 bash scripts/test-frontend.sh                   # ✅ 全量（CI 模式）
#   HARD_TIMEOUT=300 bash scripts/test-frontend.sh                   # 自定义总超时
#
# 行为：
#   1. pre-flight 守卫（FILES 合规性 + 多文件必须 ENCV_TEST_FULL）
#   2. pre-flight 清理（用进程组跟踪，无 pkill）
#   3. vitest run（带 --bail + 进程组超时兜底）
#   4. 退出时清理进程组（不碰沙箱其他进程）
# =====================================================

set -uo pipefail

# ── 安全：拒绝以不安全方式执行 ──
# 沙箱破坏 #1: 必须从 monorepo 根目录运行
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
if [ ! -f "$ROOT/go.mod" ] || [ ! -d "$ROOT/app/encv-mobile" ]; then
  echo "FATAL: test-frontend.sh must run from monorepo root ($ROOT)" >&2
  echo "  Got ROOT=$ROOT" >&2
  echo "  Expected go.mod and app/encv-mobile/" >&2
  exit 70  # EX_SOFTWARE
fi
MOBILE_DIR="$ROOT/app/encv-mobile"
if [ ! -f "$MOBILE_DIR/package.json" ]; then
  echo "FATAL: $MOBILE_DIR/package.json not found" >&2
  exit 70
fi
cd "$MOBILE_DIR"

# ── 安全：拒绝危险环境变量被外部劫持 ──
unset ENCV_TEST_INVOKED_BY  # 由本脚本末尾 export，禁止预先注入
unset HARD_TIMEOUT
unset PACKAGE_TIMEOUT  # 前端用不到

# ====================================================
# === 参数解析：分离 FILES（文件路径）与 FLAGS（透传给 vitest） ===
# ====================================================
# vitest 形如 `vitest run -t "clientSearch" src/...`：
#   - -t / -reporter / -bail / -coverage → vitest flag
#   - -t 的 value（"clientSearch"）紧跟 -t → vitest flag value
#   - src/composables/... → file path
# ====================================================

RAW_ARGS=("$@")

# ── 提取纯文件路径 ──
FILES=""
skip_next=0
for token in "${RAW_ARGS[@]}"; do
  if [ $skip_next -eq 1 ]; then
    skip_next=0
    continue
  fi
  case "$token" in
    -t|--testNamePattern)
      skip_next=1
      continue
      ;;
  esac
  if [[ "$token" == -* ]]; then
    if [[ "$token" == "-t="* ]] || [[ "$token" == "--testNamePattern="* ]]; then
      continue
    fi
    continue
  fi
  FILES="$FILES $token"
done
FILES="${FILES# }"

# ── 提取 -t value（用于 echo 输出） ──
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
    -t=*)
      TEST_NAME_VALUE="${token#-t=}"
      continue
      ;;
    --testNamePattern=*)
      TEST_NAME_VALUE="${token#--testNamePattern=}"
      continue
      ;;
  esac
done

# ── 守卫：检测多文件 / 全包模式 ──
# 关键：必须在 glob 展开前检测。`for token in $FILES` 这种 unquoted 用法
# 会触发 shell glob expansion（$FILES='src/**/*.test.ts' 会被展开为多文件），
# 所以原始字面 token 的检查必须用 set -f 关闭 glob。
set -f  # 关闭 pathname expansion（glob）
is_multifile=0
RAW_TOKEN_COUNT=0
HAS_GLOB=0
if [ -z "$FILES" ]; then
  is_multifile=1
else
  for token in $FILES; do
    RAW_TOKEN_COUNT=$((RAW_TOKEN_COUNT+1))
    if [[ "$token" == *"..."* ]] || [[ "$token" == *"*"* ]] || [[ "$token" == *"?"* ]]; then
      HAS_GLOB=1
    fi
  done
  if [ "$RAW_TOKEN_COUNT" -gt 1 ] || [ "$HAS_GLOB" -eq 1 ]; then
    is_multifile=1
  fi
fi
set +f  # 恢复 glob（后续可能需要）

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
║    bash scripts/test-frontend.sh src/composables/__tests__/useFileList.test.ts
║    bash scripts/test-frontend.sh src/views/__tests__/Files.test.ts
║    bash scripts/test-frontend.sh -t "clientSearchTokenize" src/composables
║                                                          ║
║  ✅ 全量（必须显式声明 ENCV_TEST_FULL=1，CI 专用）：       ║
║    ENCV_TEST_FULL=1 bash scripts/test-frontend.sh         ║
║                                                          ║
║  💡 测试名匹配（-t / --testNamePattern）：                 ║
║    bash scripts/test-frontend.sh -t "split" src/utils     # 跑 split 相关的
╚══════════════════════════════════════════════════════════╝
GUARD_ERROR_EOF
  exit 64
fi

# ── 剥离 FILES 中的 -flag ──
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
HARD_TIMEOUT=${HARD_TIMEOUT:-300}
LOG_ROOT=${LOG_ROOT:-.test-runs}

BAIL_FLAG="--bail=1"
if [ "${ENCV_TEST_FULL:-0}" = "1" ]; then
  BAIL_FLAG=""
fi

REPORTER_FLAG=${REPORTER_FLAG:-"--reporter=default"}

EXTRA_ARGS=${EXTRA_ARGS:-}

# 临时日志目录（避免污染源目录）
TS=$(date +%Y%m%d-%H%M%S)
LOG_DIR=$(mktemp -d "$LOG_ROOT/fe-$TS-XXXX") || {
  echo "FATAL: mktemp failed for log dir" >&2
  exit 70
}
mkdir -p "$LOG_ROOT"  # 确保 LOG_ROOT 存在

echo "==================================="
echo "[test-frontend] started at $(date -Iseconds)"
echo "[test-frontend] files=${FILES:-(all)}"
echo "[test-frontend] test_name=${TEST_NAME_VALUE:-(none)}"
echo "[test-frontend] bail_flag=${BAIL_FLAG:-(disabled, full mode)}"
echo "[test-frontend] hard_timeout=${HARD_TIMEOUT}s"
echo "[test-frontend] log_dir=$LOG_DIR"
echo "===================================" | tee "$LOG_DIR/summary.txt"

# ── pre-flight：清理 .test-runs 旧日志（保留 5 个最新） ──
if [ -d "$LOG_ROOT" ]; then
  find "$LOG_ROOT" -maxdepth 1 -mindepth 1 -type d -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn | tail -n +6 | awk '{print $2}' | xargs -r rm -rf 2>/dev/null || true
fi

# ====================================================
# === 进程组跟踪（关键：避免 pkill 误杀） ===
# ====================================================
# 方案：setsid 启动新进程组 → timeout 用 --kill-after 强杀整个进程组
# 当 timeout 超时时，timeout 会先 SIGTERM 整个进程组（graceful），
# 之后 SIGKILL（强制）。无需 pkill，零误杀风险。
# ====================================================
export ENCV_TEST_INVOKED_BY="scripts/test-frontend.sh"

# ── 跑测试（带强超时链） ──
START_TS=$(date +%s)

# shellcheck disable=SC2086
if [ -n "$FILES" ]; then
  # 单文件模式：直接传给 vitest
  timeout --kill-after=15s "${HARD_TIMEOUT}s" setsid npx vitest run \
    $BAIL_FLAG \
    $REPORTER_FLAG \
    $EXTRA_ARGS \
    $FILES \
    > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log" \
    < /dev/null
else
  # 全量模式（已通过守卫）
  timeout --kill-after=15s "${HARD_TIMEOUT}s" setsid npx vitest run \
    $BAIL_FLAG \
    $REPORTER_FLAG \
    $EXTRA_ARGS \
    > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log" \
    < /dev/null
fi
EXIT=$?

END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

# 注：timeout + setsid 会自动处理进程组清理，不需要 pkill。
# setsid 让 vitest 及其子进程（worker 进程）都在新进程组，
# timeout 超时/被 kill 时会清理整个进程组，零误杀。

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
