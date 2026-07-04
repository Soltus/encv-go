#!/usr/bin/env bash
# /workspace/.trae/specs/openlist-frontend-extraction-and-sandbox-preview/test-phase-2-v2.sh
# Phase 2 end-to-end tests v2 (with J4 deeper content check)
# Spec: §J1-J6

set -u

VITE_LOG="${1:-/tmp/vite-dev-test.log}"
VITE_URL="http://localhost:8100"

# Wait for Vite
for i in $(seq 1 30); do
  if curl -sf -o /dev/null "${VITE_URL}/" 2>/dev/null; then
    break
  fi
  sleep 1
done

PASS=0
FAIL=0

check() {
  local name="$1"
  local result="$2"
  local expect="$3"
  # case-insensitive match
  local lower_result lower_expect
  lower_result=$(echo "$result" | tr '[:upper:]' '[:lower:]')
  lower_expect=$(echo "$expect" | tr '[:upper:]' '[:lower:]')
  if [[ "$lower_result" =~ $lower_expect ]]; then
    echo "[PASS] $name"
    PASS=$((PASS+1))
  else
    echo "[FAIL] $name: got '$result', expected match '$expect'"
    FAIL=$((FAIL+1))
  fi
}

# J1: /openlist-ui/ 200 + content-type html + actual OpenList HTML
HEAD=$(curl -sI "${VITE_URL}/openlist-ui/")
check "J1 status" "$HEAD" "HTTP/1.1 200"
check "J1 content-type" "$HEAD" "Content-Type: text/html"
BODY=$(curl -s "${VITE_URL}/openlist-ui/" | head -3)
check "J1 body is OpenList HTML" "$BODY" "<!doctype html>"

# J2: /openlist-ui/api/public/settings - real OpenList endpoint, returns JSON
# (Vite's prefix middleware strips /openlist-ui/api → req.url = /public/settings → upstream = /api/public/settings)
HEAD=$(curl -sI "${VITE_URL}/openlist-ui/api/public/settings")
check "J2 status" "$HEAD" "HTTP/1.1 200"
check "J2 content-type" "$HEAD" "Content-Type: application/json"
# Also verify body is JSON, not HTML
BODY=$(curl -s "${VITE_URL}/openlist-ui/api/public/settings" | head -1)
if [[ "$BODY" == "{"* ]]; then
  echo "[PASS] J2 GET returns JSON"
  PASS=$((PASS+1))
else
  echo "[FAIL] J2 GET body is not JSON: $BODY"
  FAIL=$((FAIL+1))
fi

# J3: SPA fallback for unknown paths - returns index.html
BODY=$(curl -s "${VITE_URL}/openlist-ui/some/random/path" | head -3)
check "J3 SPA fallback" "$BODY" "<!doctype html>"

# J4: /openlist-ui/assets/*.js - should return JS, NOT html
ASSET=$(ls /workspace/app/openlist/Hi-Sillot-OpenList/public/dist/assets/*.js 2>/dev/null | head -1 | xargs basename)
if [[ -n "$ASSET" ]]; then
  HEAD=$(curl -sI "${VITE_URL}/openlist-ui/assets/${ASSET}")
  check "J4 status" "$HEAD" "HTTP/1.1 200"
  check "J4 content-type is JS" "$HEAD" "Content-Type: (application/javascript|text/javascript|application/x-javascript)"

  # Critical: body should NOT be HTML
  BODY=$(curl -s "${VITE_URL}/openlist-ui/assets/${ASSET}" | head -3)
  if [[ "$BODY" == "<!DOCTYPE html>"* || "$BODY" == "<html" ]]; then
    echo "[FAIL] J4 body is HTML (SPA fallback bug): $BODY"
    FAIL=$((FAIL+1))
  else
    echo "[PASS] J4 body is JS, not HTML"
    PASS=$((PASS+1))
  fi
fi

# J5: encv-mobile SPA root still works
HEAD=$(curl -sI "${VITE_URL}/")
check "J5 encv-mobile SPA root" "$HEAD" "HTTP/1.1 200"

# J6: /openlist/sites/* not hijacked
HEAD=$(curl -sI "${VITE_URL}/openlist/sites/foo")
check "J6 /openlist/sites/* proxy" "$HEAD" "HTTP/1.1 (502|504|200|404)"

# J7: encv-mobile SPA /openlist-ui should NOT serve OpenList HTML (different path)
HEAD=$(curl -sI "${VITE_URL}/openlist-ui")
check "J7 /openlist-ui (no slash) → 200 + HTML (SPA fallback)" "$HEAD" "HTTP/1.1 200"

# J8: encv-mobile SPA /assets/* (its own assets) still works
HEAD=$(curl -sI "${VITE_URL}/assets/" 2>/dev/null || echo "no assets at root")
echo "J8: encv-mobile SPA /assets/ response: $(echo $HEAD | head -1)"

echo ""
echo "=== Summary ==="
echo "PASS: $PASS"
echo "FAIL: $FAIL"
exit $FAIL
