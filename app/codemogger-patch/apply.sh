#!/usr/bin/env bash
#
# codemogger patch package — apply script
#
# Applies two changes to a codemogger@0.1.5 installation:
#   1. Kotlin (.kt/.kts) language support
#   2. Full-text (keyword) search over code bodies, with camelCase/snake_case
#      token splitting via a normalized `body` FTS column.
#
# Usage:
#   CODEMOGGER_DIR=/path/to/codemogger ./apply.sh          # apply
#   CODEMOGGER_DIR=/path/to/codemogger ./apply.sh --check  # dry-run only
#
# Defaults to the global install at /usr/local/lib/node_modules/codemogger.
# Re-running is safe (patch is applied with --forward and skipped if already
# present).
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH_FILE="$SCRIPT_DIR/patches/codemogger+0.1.5.patch"
VENDOR_DIR="$SCRIPT_DIR/vendor/tree-sitter-kotlin"

CODEMOGGER_DIR="${CODEMOGGER_DIR:-/usr/local/lib/node_modules/codemogger}"

if [ ! -f "$CODEMOGGER_DIR/dist/cli.mjs" ]; then
  echo "ERROR: codemogger not found at $CODEMOGGER_DIR" >&2
  echo "Set CODEMOGGER_DIR to the directory containing dist/cli.mjs" >&2
  exit 1
fi

# node_modules parent so that patch -p1 maps a/node_modules/codemogger/... -> node_modules/codemogger/...
NM_PARENT="$(dirname "$(dirname "$CODEMOGGER_DIR")")"

# 1) Vendor the Kotlin wasm grammar (cli.mjs resolves it via require.resolve).
KOTLIN_DST="$CODEMOGGER_DIR/node_modules/@tree-sitter-grammars/tree-sitter-kotlin"
if [ ! -f "$KOTLIN_DST/tree-sitter-kotlin.wasm" ]; then
  mkdir -p "$(dirname "$KOTLIN_DST")"
  cp -r "$VENDOR_DIR" "$KOTLIN_DST"
  echo "[ok] vendored @tree-sitter-grammars/tree-sitter-kotlin ($(cat "$KOTLIN_DST/package.json" | grep '"version"' | head -1 | tr -d ' ",'))"
else
  echo "[skip] tree-sitter-kotlin already present"
fi

# 2) Apply the cli.mjs patch.
cd "$NM_PARENT"
if [ "${1:-}" = "--check" ]; then
  patch -p1 --forward --dry-run < "$PATCH_FILE"
  echo "[ok] dry-run succeeded; run without --check to apply"
  exit 0
fi

if patch -p1 --forward --dry-run < "$PATCH_FILE" >/dev/null 2>&1; then
  patch -p1 --forward < "$PATCH_FILE"
  echo "[ok] patch applied to $CODEMOGGER_DIR/dist/cli.mjs"
else
  echo "[skip] patch already applied or not applicable (dry-run failed)"
fi

echo "Done."
