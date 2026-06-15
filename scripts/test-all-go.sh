#!/usr/bin/env bash
# scripts/test-all-go.sh
# =====================================================
# Go 测试模块化编排入口（wraps test-go.sh）。
#
# 🆕 2026-06-15 模块化测试编排升级（test-orchestration-defense）：
#   - 默认按 "go list ./internal/..." 拿包列表，逐个单包调 test-go.sh
#   - 单包循环 + -short 默认 → 总耗时 30-90s（vs 全包 380s+ 拉爆沙箱网络）
#   - 自动获 test-go.sh 的多包守卫（避免重复违规）
#   - 失败早停（第一个包失败立即退出）
#   - ENCV_TEST_FULL=1 → 改为单次 go test ./internal/...（CI 专用）
#
# 行为：
#   1. pre-flight 清理（call kill-orphan-children.sh + 清 /tmp）
#   2. 模块化跑：go list 拿包 → 循环 test-go.sh 单包跑
#   3. 合并 .test-runs/*/reports-*.json → report-all.json
#   4. 打印人类可读摘要
#   5. 若有失败 case 或崩溃 → exit != 0
#
# 用法：
#   bash scripts/test-all-go.sh                       # 模块化（默认推荐）
#   ENCV_TEST_FULL=1 bash scripts/test-all-go.sh      # 全量（CI 专用，慎用）
#   PKGS_PATTERN="./internal/service" bash scripts/test-all-go.sh  # 自定义模式
#
# 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 2）
# 🆕 2026-06-15 模块化编排升级（test-orchestration-defense）
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

LOG_ROOT=${LOG_ROOT:-.test-runs}

# ====================================================
# === 模式选择 ===
# ====================================================
# FULL_MODE 决策：
#   - ENCV_TEST_FULL=1   → CI 全量（go test ./internal/... 单次）
#   - 默认                → 模块化（go list 拿包 + 循环单包 test-go.sh）
PKGS_PATTERN=${PKGS_PATTERN:-"./internal/..."}
FULL_MODE=${ENCV_TEST_FULL:-0}

echo "==================================="
echo "[test-all-go] started at $(date -Iseconds)"
echo "[test-all-go] log_root=$LOG_ROOT"
echo "[test-all-go] mode=$(if [ "$FULL_MODE" = "1" ]; then echo "FULL (CI)"; else echo "MODULAR (default)"; fi)"
echo "[test-all-go] pkgs_pattern=$PKGS_PATTERN"
echo "==================================="

# ── pre-flight：调用现成的 kill-orphan-children.sh ──
if [ -x "$SCRIPT_DIR/kill-orphan-children.sh" ]; then
  echo "[pre-flight] running kill-orphan-children.sh"
  bash "$SCRIPT_DIR/kill-orphan-children.sh" 2>/dev/null || true
fi

# ── pre-flight：清大文件 ──
find /tmp -maxdepth 1 -name "encv-*" -size +100M -exec rm -rf {} \; 2>/dev/null || true

START_TS=$(date +%s)

# ====================================================
# === 分支 1：FULL 模式（CI 专用）── ENCV_TEST_FULL=1 ===
# ====================================================
if [ "$FULL_MODE" = "1" ]; then
  echo "[test-all-go][FULL] running go test $PKGS_PATTERN -short default disabled"
  if [ -n "${PKGS:-}" ]; then
    bash "$SCRIPT_DIR/test-go.sh"
  else
    bash "$SCRIPT_DIR/test-go.sh" "$@"
  fi
  TEST_EXIT=$?

# ====================================================
# === 分支 2：模块化模式（默认推荐）── 逐包单跑 ===
# ====================================================
else
    # 拿包列表（依赖 -deps 不需要，仅列包）
    echo "[test-all-go][MODULAR] listing packages via 'go list $PKGS_PATTERN'"

    # 守卫：modular 模式跑包数上限（避免 12+min 爆沙箱 600s 配额）
    # 上限来源：600s 沙箱预算 / 2 (重试 buffer) / 15s 平均单包 = 20
    # 但我们更保守：modular 模式默认最多 10 包（用户场景：modified 5-10 包 + deps）
    MODULAR_MAX_PKGS=${MODULAR_MAX_PKGS:-10}
    if [ "${ENCV_TEST_FULL:-0}" = "1" ]; then
      MODULAR_MAX_PKGS=999  # CI 模式无限
    fi

    # 守卫：用户传入了 -run 模式 → 透传单包（不进入模块化循环）
    HAS_RUN_FLAG=0
    for arg in "$@"; do
      if [[ "$arg" == "-run"* ]] || [[ "$arg" == "-skip"* ]]; then
        HAS_RUN_FLAG=1
        break
      fi
    done

    if [ $HAS_RUN_FLAG -eq 1 ]; then
      echo "[test-all-go][MODULAR] -run/-skip detected → passthrough to test-go.sh"
      if [ -n "${PKGS:-}" ]; then
        bash "$SCRIPT_DIR/test-go.sh"
      else
        bash "$SCRIPT_DIR/test-go.sh" "$@"
      fi
      TEST_EXIT=$?
    else
      # 拿包列表（依赖 -deps 不需要，仅列包）
      PKGS_LIST=$(go list -f '{{.Dir}}|{{.ImportPath}}' $PKGS_PATTERN 2>/dev/null | sort)
      if [ -z "$PKGS_LIST" ]; then
        echo "[test-all-go][MODULAR][FATAL] no packages found for pattern: $PKGS_PATTERN" >&2
        exit 1
      fi
      PKGS_COUNT=$(echo "$PKGS_LIST" | wc -l)
      echo "[test-all-go][MODULAR] found $PKGS_COUNT packages, running each with -short -failfast"

      # 🆕 2026-06-15：modular 模式跑包数守卫（避免 12+min 爆沙箱）
      if [ "$PKGS_COUNT" -gt "$MODULAR_MAX_PKGS" ]; then
        cat >&2 <<MODULAR_GUARD_EOF
╔══════════════════════════════════════════════════════════╗
║  [test-all-go][MODULAR] 跑包数超过沙箱预算！立即终止。   ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  ❌ 检测到 modular 模式跑 $PKGS_COUNT 个包                  ║
║     MODULAR_MAX_PKGS=$MODULAR_MAX_PKGS                     ║
║                                                          ║
║  ❓ 为什么禁止：                                          ║
║    沙箱网络配额 ~600s/任务                                ║
║    modular 模式每包 2-15s，全 51 包累加 12+ min            ║
║    容易触发 OOM kill exit 137 → 沙箱断网                   ║
║                                                          ║
║  ✅ 解决方案（按推荐度排序）：                            ║
║    1. 缩小 PKGS_PATTERN 到只含你修改的包：                  ║
║       PKGS_PATTERN=./internal/service bash test-all-go.sh  ║
║                                                          ║
║    2. 直接单包跑（最高效）：                                ║
║       bash scripts/test-go.sh ./internal/service           ║
║                                                          ║
║    3. 显式调高上限（需自负责任）：                          ║
║       MODULAR_MAX_PKGS=20 bash scripts/test-all-go.sh      ║
║                                                          ║
║    4. CI 模式（无上限，ENCV_TEST_FULL=1）：                 ║
║       ENCV_TEST_FULL=1 bash scripts/test-all-go.sh        ║
╚══════════════════════════════════════════════════════════╝
MODULAR_GUARD_EOF
        exit 64
      fi

    # ── 拓扑感知分批：先跑 "叶子"（无 internal 依赖），再跑 "根" ──
    # 简化：按字母序跑也行（无 internal 内部交叉依赖），但内部分包不强制排序
    # 因为 -short 单包 2-15s，错了也只影响一个包

    PASSED=()
    FAILED=()
    SKIPPED=()

    idx=0
    while IFS='|' read -r pkg_dir pkg_import; do
      [ -z "$pkg_import" ] && continue
      idx=$((idx + 1))
      pkg_short=$(echo "$pkg_import" | sed 's|^github.com/Soltus/encv-go/||')
      echo ""
      echo "[test-all-go][$idx/$PKGS_COUNT] ▶ $pkg_short"

      # 单包跑（test-go.sh 内部 -short + 守卫）
      if HARD_TIMEOUT=120 PACKAGE_TIMEOUT=90 \
         bash "$SCRIPT_DIR/test-go.sh" "$pkg_import" 2>&1 \
         | tail -3; then
        PASSED+=("$pkg_short")
      else
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 64 ]; then
          # 守卫抛错（不应当发生 — pkg 是单包）
          FAILED+=("$pkg_short (GUARD_REJECTED)")
          echo "[test-all-go][$idx/$PKGS_COUNT] ✗ $pkg_short GUARD_REJECTED"
        else
          FAILED+=("$pkg_short (exit=$EXIT_CODE)")
          echo "[test-all-go][$idx/$PKGS_COUNT] ✗ $pkg_short exit=$EXIT_CODE"
        fi
        # 失败早停（除非显式禁用）
        if [ "${ENCV_TEST_NO_FAILFAST:-0}" != "1" ]; then
          echo "[test-all-go][MODULAR][FAIL-FAST] stopping at first failure"
          break
        fi
      fi
    done <<< "$PKGS_LIST"

    # ── 汇总 ──
    END_TS=$(date +%s)
    DURATION=$((END_TS - START_TS))
    echo ""
    echo "==================================="
    echo "[test-all-go][MODULAR] done in ${DURATION}s"
    echo "[test-all-go][MODULAR] passed: ${#PASSED[@]}"
    echo "[test-all-go][MODULAR] failed: ${#FAILED[@]}"
    for f in "${FAILED[@]:-}"; do echo "  ✗ $f"; done
    echo "==================================="

    if [ ${#FAILED[@]} -gt 0 ]; then
      TEST_EXIT=1
    else
      TEST_EXIT=0
    fi
  fi
fi

END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

echo ""
echo "==================================="
echo "[test-all-go] done in ${DURATION}s exit=$TEST_EXIT"
echo "==================================="

# ── 合并报告 ──
mkdir -p "$LOG_ROOT"

REPORT_ALL="$LOG_ROOT/report-all.json"

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
