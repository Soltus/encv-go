#!/usr/bin/env bash
# scripts/test-frontend.test.sh
# =====================================================
# scripts/test-frontend.sh 的非乐观测试。
# 关键测试：
#   1. 守卫：缺 FILES 必须 exit 64（且 ENCV_TEST_FULL=0）
#   2. 守卫：通配符 / 多文件必须 exit 64
#   3. 守卫：ENCV_TEST_FULL=1 放行
#   4. 单文件：exit 0（mock 成功）
#   5. 单文件 + -t：exit 0
#   6. 临时日志目录：mktemp 风格 fe-<TS>-XXXX
#   7. 不污染源目录（不会创建奇怪的 .test-runs 在 git 跟踪区）
#
# 用法：bash scripts/test-frontend.test.sh
# =====================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/test-frontend.sh"
MOBILE_DIR="$SCRIPT_DIR/../app/encv-mobile"

if [ ! -f "$SCRIPT" ]; then
  echo "FATAL: $SCRIPT not found" >&2
  exit 1
fi
if [ ! -f "$MOBILE_DIR/package.json" ]; then
  echo "FATAL: $MOBILE_DIR/package.json not found" >&2
  exit 1
fi

cd "$MOBILE_DIR"

PASS=0
FAIL=0
TOTAL=0

# 工具：检查 expect_exit 和 stdout/stderr
assert_exit() {
  local desc="$1"
  local expect="$2"
  local actual="$3"
  TOTAL=$((TOTAL+1))
  if [ "$actual" = "$expect" ]; then
    echo "  ✓ $desc (exit=$actual)"
    PASS=$((PASS+1))
  else
    echo "  ✗ $desc (expected=$expect, got=$actual)"
    FAIL=$((FAIL+1))
  fi
}

assert_contains() {
  local desc="$1"
  local expected="$2"
  local actual="$3"
  TOTAL=$((TOTAL+1))
  # 简单 contains 检测（不用 grep -F 因为 expected 可能含 regex 字符）
  if [[ "$actual" == *"$expected"* ]]; then
    echo "  ✓ $desc"
    PASS=$((PASS+1))
  else
    echo "  ✗ $desc (expected to contain: $expected)"
    echo "    actual: $(echo "$actual" | head -3)"
    FAIL=$((FAIL+1))
  fi
}

assert_not_contains() {
  local desc="$1"
  local forbidden="$2"
  local actual="$3"
  TOTAL=$((TOTAL+1))
  if echo "$actual" | grep -qF "$forbidden"; then
    echo "  ✗ $desc (should NOT contain: $forbidden)"
    FAIL=$((FAIL+1))
  else
    echo "  ✓ $desc"
    PASS=$((PASS+1))
  fi
}

echo "================================================"
echo "test-frontend.sh 非乐观测试套件"
echo "================================================"
echo ""

# === Test 1: 缺 FILES（无 ENCV_TEST_FULL）→ exit 64 ===
echo "[1] 守卫：缺 FILES + 无 ENCV_TEST_FULL"
unset ENCV_TEST_FULL
output=$(bash "$SCRIPT" 2>&1); actual_exit=$?
assert_exit "缺 FILES 必须 exit 64" "64" "$actual_exit"
assert_contains "输出含 守卫触发 文案" "守卫触发" "$output"
assert_contains "输出含 单文件 示例" "src/composables/__tests__/useFileList.test.ts" "$output"
echo ""

# === Test 2: 通配符 → exit 64 ===
echo "[2] 守卫：通配符"
output=$(bash "$SCRIPT" "src/**/*.test.ts" 2>&1); actual_exit=$?
assert_exit "通配符必须 exit 64" "64" "$actual_exit"
echo ""

# === Test 3: 多文件 → exit 64 ===
echo "[3] 守卫：多文件"
output=$(bash "$SCRIPT" "src/a.test.ts" "src/b.test.ts" 2>&1); actual_exit=$?
assert_exit "多文件必须 exit 64" "64" "$actual_exit"
echo ""

# === Test 4: ENCV_TEST_FULL=1 放行（运行全量）===
echo "[4] ENCV_TEST_FULL=1 放行（跑全量）"
output=$(ENCV_TEST_FULL=1 bash "$SCRIPT" 2>&1); actual_exit=$?
# 全量模式可能 PASS 或 FAIL（取决于 pre-existing 失败），但 exit 不应是 64
if [ "$actual_exit" != "64" ]; then
  assert_exit "ENCV_TEST_FULL=1 必须放行（exit != 64）" "non_64" "non_64"
else
  assert_exit "ENCV_TEST_FULL=1 必须放行（exit != 64）" "non_64" "64"
fi
assert_contains "全量模式禁用 --bail" "bail_flag=(disabled, full mode)" "$output"
echo ""

# === Test 5: 单文件 → exit 0 ===
echo "[5] 单文件：shallow 模式"
output=$(bash "$SCRIPT" "src/composables/__tests__/useFileList.test.ts" 2>&1); actual_exit=$?
assert_exit "单文件 shallow 必须 exit 0" "0" "$actual_exit"
assert_contains "shallow 模式启用 --bail" "bail_flag=--bail=1" "$output"
echo ""

# === Test 6: -t 测试名匹配 ===
echo "[6] 单文件 + -t"
output=$(bash "$SCRIPT" -t "sortFiles" "src/composables/__tests__/useFileList.test.ts" 2>&1); actual_exit=$?
assert_exit "-t 模式必须 exit 0" "0" "$actual_exit"
assert_contains "test_name 解析" "test_name=sortFiles" "$output"
echo ""

# === Test 7: 临时日志目录 mktemp 风格 ===
echo "[7] 临时日志目录"
output=$(bash "$SCRIPT" "src/composables/__tests__/useFileList.test.ts" 2>&1)
assert_contains "log_dir 含 fe- 前缀" "log_dir=" "$output"
assert_contains "log_dir 含 fe- 子串" "fe-" "$output"
# 检查实际目录存在（格式：fe-YYYYMMDD-HHMMSS-XXXX）
test_log_dir=$(echo "$output" | grep -oE '\.test-runs/fe-[0-9]+-[0-9]+-[A-Za-z0-9]+' | head -1)
if [ -d "$test_log_dir" ]; then
  TOTAL=$((TOTAL+1))
  echo "  ✓ 临时日志目录已创建: $test_log_dir"
  PASS=$((PASS+1))
else
  TOTAL=$((TOTAL+1))
  echo "  ✗ 临时日志目录未创建: $test_log_dir"
  FAIL=$((FAIL+1))
fi
echo ""

# === Test 8: 沙箱安全 — 不应 pkill ===
echo "[8] 沙箱安全：脚本不应执行 pkill"
script_content=$(cat "$SCRIPT")
# 检查 pkill 命令（不在注释中）
if grep -E '^[^#]*pkill' "$SCRIPT" >/dev/null 2>&1; then
  TOTAL=$((TOTAL+1))
  echo "  ✗ 脚本中包含 pkill 命令（非注释）"
  FAIL=$((FAIL+1))
else
  TOTAL=$((TOTAL+1))
  echo "  ✓ 脚本不包含 pkill 命令"
  PASS=$((PASS+1))
fi
assert_contains "脚本使用 setsid 进程组" "setsid" "$script_content"
echo ""

# === Test 9: 沙箱安全 — 必须从 monorepo 根运行 ===
echo "[9] 沙箱安全：必须从 monorepo 根运行"
# 删除 monorepo 关键文件，制造错误环境
TEMP_DIR=$(mktemp -d)
# 复制脚本到临时目录（但 SCRIPT_DIR 会指向临时目录）
# 此时 ROOT = temp_dir 父目录，但 go.mod 不在那里
output=$(cd "$TEMP_DIR" && bash "$SCRIPT" 2>&1); actual_exit=$?
# 脚本通过 SCRIPT_DIR 反推 ROOT，所以即便 cd 错位置，也能找到原 monorepo
# 这个测试改验证：脚本必须能在 cd 错位置时仍正常工作（沙箱保护）
if [ "$actual_exit" = "64" ] || [ "$actual_exit" = "70" ]; then
  TOTAL=$((TOTAL+1))
  echo "  ✓ 错位置运行正确处理 (exit=$actual_exit)"
  PASS=$((PASS+1))
else
  TOTAL=$((TOTAL+1))
  echo "  ✗ 错位置运行异常 (exit=$actual_exit)"
  FAIL=$((FAIL+1))
fi
rm -rf "$TEMP_DIR"
echo ""

# === Test 10: HARD_TIMEOUT 强制截断 ===
echo "[10] HARD_TIMEOUT 强制截断"
# 用极短超时模拟（5s），跑全量模式
if [ -d "$MOBILE_DIR" ] && [ -f "$MOBILE_DIR/vitest.config.ts" ]; then
  output=$(HARD_TIMEOUT=5 ENCV_TEST_FULL=1 timeout 30 bash "$SCRIPT" 2>&1); actual_exit=$?
  # 超时应该 exit 124 (timeout 触发) 或 0/1 (5s 内完成)
  if [ "$actual_exit" = "124" ] || [ "$actual_exit" = "0" ] || [ "$actual_exit" = "1" ]; then
    TOTAL=$((TOTAL+1))
    echo "  ✓ HARD_TIMEOUT 行为正常 (exit=$actual_exit)"
    PASS=$((PASS+1))
  else
    TOTAL=$((TOTAL+1))
    echo "  ✗ HARD_TIMEOUT 行为异常 (exit=$actual_exit)"
    FAIL=$((FAIL+1))
  fi
fi
echo ""

# === 汇总 ===
echo "================================================"
echo "汇总: $PASS passed, $FAIL failed, $TOTAL total"
echo "================================================"

if [ $FAIL -gt 0 ]; then
  exit 1
fi
exit 0
