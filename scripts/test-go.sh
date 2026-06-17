#!/usr/bin/env bash
# scripts/test-go.sh
# =====================================================
# Go 测试唯一入口 + 多包运行守卫。
# 强保证：无论内部发生什么，HARD_TIMEOUT 后必返回。
#
# 🆕 2026-06-15 模块化测试编排升级（test-orchestration-defense）：
#   - 多包全包跑（`go test ./...` / `./internal/...` 等）默认 panic 抛错 exit 1
#   - 唯一合法多包方式：ENCV_TEST_FULL=1（CI 专用）
#   - 单包 `go test ./internal/<pkg>` 永远合法
#   - 默认 -short（skip slow bench / e2e）
#   - 失败早停 -failfast
#
# 用法：
#   bash scripts/test-go.sh                                       # 违规：缺 PKGS = 默认多包 → 守卫抛错
#   bash scripts/test-go.sh ./internal/service                    # ✅ 合法单包（-short 默认）
#   bash scripts/test-go.sh -run TestFoo ./internal/service       # ✅ 合法单包 + -run
#   ENCV_TEST_LONG=1 bash scripts/test-go.sh ./internal/service   # ✅ 单包 + 跑 slow
#   ENCV_TEST_FULL=1 bash scripts/test-go.sh ./internal/...       # ✅ 多包 CI 模式
#   HARD_TIMEOUT=60 bash scripts/test-go.sh                       # 自定义总超时
#
# 行为：
#   1. pre-flight 守卫（PKGS 合规性 + 多包必须 ENCV_TEST_FULL）
#   2. pre-flight 清理（上轮残留的 /tmp/encv-test-*, 端口 2025, 崩溃堆栈目录）
#   3. go test（带 -timeout + 外层 timeout 兜底 + 默认 -short -failfast）
#   4. 退出时强杀残留 go test / go-build 进程
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

# ====================================================
# === 🆕 2026-06-15 守卫层 1：PKGS 合规性 ===
# ====================================================
# 检测多包 / 全包模式：./...  /xxx/...  ./internal/...  ./cmd/...  ./...
# 单包：./internal/<pkg>  ./internal/<pkg>/<file>  ./internal/<pkg>/...
# 区分原则：最后一个 path component 含 `...` = 多包
# ====================================================

# ====================================================
# === 参数解析：分离 PKGS（包路径）与 FLAGS（透传给 go test） ===
# ====================================================
# ⚠️ go test 形如 `go test -run TestX -short ./internal/service`：
#   - -run / -short / -count / -v / -timeout / -failfast → go test flag
#   - ./internal/service                              → package path
# 注意 -run 的值（TestXxx）虽然无 - 前缀，但被 -run 消费了 — 必须跳过
# ====================================================

# 收集"包路径"参数（无 - 前缀，但需要排除 -run/-skip 的 value）
# 算法：扫描所有 token，若上一个 token 是 -run/-skip 则当前是 value 跳过
# 否则若 token 不以 - 开头 → 是包路径

# 守卫扫"全包模式"用所有 args（raw），但 go test 调用只传"包路径"
RAW_ARGS=("$@")
if [ -n "${PKGS:-}" ]; then
  RAW_ARGS=("$@" "$PKGS")
fi

# 提取纯包路径（用于 go test 调用）
PKGS=""
skip_next=0
for token in "${RAW_ARGS[@]}"; do
  # 跳过 -run / -skip 的 value
  if [ $skip_next -eq 1 ]; then
    skip_next=0
    continue
  fi
  # 标记下一个 token 是 value
  if [ "$token" = "-run" ] || [ "$token" = "-skip" ]; then
    skip_next=1
    continue
  fi
  # 跳过 -flag 本身（含等号的合并形如 -run=TestX）
  if [[ "$token" == -* ]]; then
    if [[ "$token" == "-run="* ]] || [[ "$token" == "-skip="* ]]; then
      continue  # 合并形式已经处理完，不需 skip_next
    fi
    continue
  fi
  PKGS="$PKGS $token"
done
PKGS="${PKGS# }"

# ── 守卫：检测多包 / 全包模式（扫所有 token，包括 -flag 后的 value） ──
is_multipkg=0
for token in "${RAW_ARGS[@]}"; do
  if [[ "$token" == *"..."* ]]; then
    is_multipkg=1
    break
  fi
done

# 默认 PKGS（如果解析后为空）— 视为"默认多包" → 守卫会抛错
if [ -z "$PKGS" ]; then
  PKGS="./internal/..."  # 触发守卫
fi

if [ $is_multipkg -eq 1 ] && [ "${ENCV_TEST_FULL:-0}" != "1" ]; then
  cat >&2 <<'GUARD_ERROR_EOF'
╔══════════════════════════════════════════════════════════╗
║  [test-go] 多包/全包运行守卫触发！立即终止。            ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  ❌ 检测到多包运行模式：                                  ║
║     PKGS 包含 `...` 通配（如 ./internal/...）            ║
║                                                          ║
║  ❓ 为什么禁止：                                          ║
║    ① 沙箱内多包跑动辄 380s+，耗尽网络配额触发断网         ║
║    ② server 包单跑就 376s，server + mount + utils 串起来   ║
║       可能拉爆 go test 进程（OOM kill exit 137）          ║
║    ③ CI 才有跑全包的需求，本地应按模块拓扑分批             ║
║                                                          ║
║  ✅ 单包（永远合法）：                                    ║
║    bash scripts/test-go.sh ./internal/service            ║
║    bash scripts/test-go.sh ./internal/utils              ║
║    bash scripts/test-go.sh ./internal/mount              ║
║                                                          ║
║  ✅ 多包（必须显式声明 ENCV_TEST_FULL=1，CI 专用）：       ║
║    ENCV_TEST_FULL=1 bash scripts/test-go.sh ./internal/...  ║
║    ENCV_TEST_FULL=1 bash scripts/test-all-go.sh          ║
║                                                          ║
║  ✅ 模块化编排：                                          ║
║    bash scripts/test-all-go.sh                            ║
║      → 默认按依赖图分批（utils → crypto → mount → service ║
║         → server），单包 -short 跑 2-15s/包                ║
║                                                          ║
║  💡 跑 slow bench / e2e：                                 ║
║    ENCV_TEST_LONG=1 bash scripts/test-go.sh ./internal/server
║      → 解除 -short，跑到 server 全量（仍 1 包，可控）    ║
╚══════════════════════════════════════════════════════════╝
GUARD_ERROR_EOF
  exit 64  # EX_USAGE — 调用方式错误（参考 sysexits.h）
fi

# ── 守卫：通过后剥离 PKGS 中的 -flag（避免 `go test $PKGS` 误把 -flag 当包） ──
# （PKGS 已在参数解析时剥好 -flag；此处仅防御性补一刀）
PKGS_CLEAN=""
for token in $PKGS; do
  if [[ "$token" == -* ]]; then continue; fi
  PKGS_CLEAN="$PKGS_CLEAN $token"
done
PKGS="$PKGS_CLEAN"
PKGS="${PKGS# }"

# ====================================================
# === 默认行为（-short + -failfast） ===
# ====================================================
HARD_TIMEOUT=${HARD_TIMEOUT:-600}        # 总硬超时 10min
PACKAGE_TIMEOUT=${PACKAGE_TIMEOUT:-120}  # 单包超时 2min
LOG_ROOT=${LOG_ROOT:-.test-runs}

# -short 开关
GO_TEST_SHORT_FLAG="-short"
if [ "${ENCV_TEST_FULL:-0}" = "1" ] || [ "${ENCV_TEST_LONG:-0}" = "1" ]; then
  GO_TEST_SHORT_FLAG=""  # 全量模式 = 解除 -short
fi

# -failfast 开关（CI/全量模式不早停，本地单包可早停）
GO_FAILFAST_FLAG="-failfast"
if [ "${ENCV_TEST_FULL:-0}" = "1" ] || [ "${ENCV_TEST_NO_FAILFAST:-0}" = "1" ]; then
  GO_FAILFAST_FLAG=""
fi

EXTRA_ARGS=${EXTRA_ARGS:-}

TS=$(date +%Y%m%d-%H%M%S)
LOG_DIR="$LOG_ROOT/go-$TS"
mkdir -p "$LOG_DIR"

echo "==================================="
echo "[test-go] started at $(date -Iseconds)"
echo "[test-go] pkgs=$PKGS"
echo "[test-go] short_flag=${GO_TEST_SHORT_FLAG:-"(disabled, full mode)"}"
echo "[test-go] failfast_flag=${GO_FAILFAST_FLAG:-"(disabled)"}"
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

# ====================================================
# === 🆕 2026-06-17 守卫层 2：Go test binary 进程级拦截 ===
# ====================================================
# scripts/test-go.sh 的 bash 守卫对"裸 go test ./internal/<pkg>"完全无效。
# 用户多次反馈"go 完整测试拦截似乎没有考虑到所有调用方式"——
# 必须从 Go test binary 启动时（init()）强制要求 ENCV_TEST_INVOKED_BY。
#
# 工作原理：
#   - scripts/test-go.sh 执行 go test 前 export ENCV_TEST_INVOKED_BY=scripts/test-go.sh
#   - internal/testguard 包的 init() 检查该 env var
#   - 未设置 → os.Exit(64)（与 bash 守卫一致）
#   - CI 环境（CI=true / GITHUB_ACTIONS=true）自动放行
#   - 用户显式 ENCV_TEST_BYPASS_GUARD=1 可紧急 bypass
# ====================================================
export ENCV_TEST_INVOKED_BY="scripts/test-go.sh"

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
  $GO_TEST_SHORT_FLAG \
  $GO_FAILFAST_FLAG \
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
echo "[test-go] $EXIT_MSG in ${DURATION}s (exit=$EXIT)"
echo "[test-go] logs: $LOG_DIR"
echo "==================================="

exit $EXIT
