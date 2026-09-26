#!/usr/bin/env bash
# build-wasm.sh — 构建前端可用的 ENCV 加密 WASM 产物
#
# 产物（app/packages/encv-crypto/wasm/）：
#   encv.wasm            GOOS=js GOARCH=wasm 编译的加密层（不含任何 TS 侧密码学实现）
#   wasm_exec.js         Go 运行时胶水（浏览器 / Worker 用）
#   wasm_exec_node.js    Go 运行时胶水（Node 用，仅测试需要）
#
# 用法：
#   bash scripts/build-wasm.sh              # 构建 + 复制胶水 + 生成黄金向量
#   bash scripts/build-wasm.sh --no-vectors # 跳过向量生成
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

GO=${GO:-go}
OUT_DIR="app/packages/encv-crypto/wasm"
VECTORS_DIR="internal/v2/crypto/simple/testdata"

WASM_EXEC="$("$GO" env GOROOT)/lib/wasm/wasm_exec.js"
WASM_EXEC_NODE="$("$GO" env GOROOT)/lib/wasm/wasm_exec_node.js"

if [[ ! -f "$WASM_EXEC" ]]; then
  echo "❌ 未找到 $WASM_EXEC（Go 1.24+ 才把 wasm_exec.js 放在 lib/wasm 下）" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "▶ 构建 encv.wasm"
GOOS=js GOARCH=wasm "$GO" build -o "$OUT_DIR/encv.wasm" ./cmd/encv-wasm

echo "▶ 复制 Go wasm 运行时胶水"
cp "$WASM_EXEC" "$OUT_DIR/wasm_exec.js"
[[ -f "$WASM_EXEC_NODE" ]] && cp "$WASM_EXEC_NODE" "$OUT_DIR/wasm_exec_node.js"

if [[ "${1:-}" != "--no-vectors" ]]; then
  echo "▶ 生成黄金测试向量（Go 与 WASM 共享的契约锁）"
  "$GO" run ./cmd/encv-crypto-vectors -out "$VECTORS_DIR/vectors.json"
fi

echo "✅ 完成："
ls -lh "$OUT_DIR"
