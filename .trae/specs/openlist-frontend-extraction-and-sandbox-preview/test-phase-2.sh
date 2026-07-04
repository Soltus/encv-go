#!/usr/bin/env bash
# /workspace/.trae/specs/openlist-frontend-extraction-and-sandbox-preview/test-phase-2.sh
# Phase 2 end-to-end tests for Vite openlist-ui-proxy middleware
# Spec: §J1-J6

set -u

VITE_LOG="${1:-/tmp/vite-dev-test.log}"
VITE_URL="http://localhost:8100"

# Wait for Vite to be ready
echo "=== Waiting for Vite at ${VITE_URL} ==="
for i in $(seq 1 30); do
  if curl -sf -o /dev/null "${VITE_URL}/" 2>/dev/null; then
    echo "Vite ready after ${i}s"
    break
  fi
  sleep 1
done

PASS=0
FAIL=0

run_test() {
  local name="$1"
  local cmd="$2"
  local expect="$3"
  echo ""
  echo "=== $name ==="
  echo "Cmd: $cmd"
  out=$(eval "$cmd" 2>&1)
  rc=$?
  echo "$out" | head -10
  if echo "$out" | grep -qE "$expect"; then
    echo "[PASS] matched: $expect"
    PASS=$((PASS+1))
  else
    echo "[FAIL] no match for: $expect"
    FAIL=$((FAIL+1))
  fi
}

# J1: /openlist-ui/ (root, SPA fallback returns index.html)
run_test "J1: /openlist-ui/ (root, 200 OK HTML)" \
  "curl -sI ${VITE_URL}/openlist-ui/" \
  "HTTP/1.1 200"

# J2: /openlist-ui/api/* proxy to OpenList(5244) - expect 502 since no upstream
run_test "J2: /openlist-ui/api/ping (proxy, 502 if no upstream)" \
  "curl -sI ${VITE_URL}/openlist-ui/api/ping" \
  "HTTP/1.1 (200|502|504)"

# J3: SPA fallback for unknown paths
run_test "J3: /openlist-ui/some/random/path (SPA fallback)" \
  "curl -s ${VITE_URL}/openlist-ui/some/random/path | head -3" \
  "<!DOCTYPE html>"

# J4: /openlist-ui/assets/* (static resource)
ASSET=$(ls /workspace/app/openlist/Hi-Sillot-OpenList/public/dist/assets/*.js 2>/dev/null | head -1 | xargs basename)
echo ""
echo "=== J4: /openlist-ui/assets/${ASSET} ==="
if [[ -n "$ASSET" ]]; then
  curl -sI "${VITE_URL}/openlist-ui/assets/${ASSET}" | head -5
  if curl -sf -o /dev/null "${VITE_URL}/openlist-ui/assets/${ASSET}"; then
    echo "[PASS] asset served"
    PASS=$((PASS+1))
  else
    echo "[FAIL] asset 404"
    FAIL=$((FAIL+1))
  fi
else
  echo "[SKIP] no assets/*.js found"
fi

# J5: encv-mobile SPA itself still works (regression check)
run_test "J5: / (encv-mobile SPA, Vite serves index.html)" \
  "curl -sI ${VITE_URL}/" \
  "HTTP/1.1 200"

# J6: /openlist/sites/* (existing proxy not hijacked)
run_test "J6: /openlist/sites/foo (existing proxy, no hijack of /openlist-ui)" \
  "curl -sI ${VITE_URL}/openlist/sites/foo" \
  "HTTP/1.1 (502|504|200|404)"

# Summary
echo ""
echo "=== Summary ==="
echo "PASS: $PASS"
echo "FAIL: $FAIL"
echo "Vite log: ${VITE_LOG}"

if [[ $FAIL -gt 0 ]]; then
  echo "=== Vite log tail ==="
  tail -30 "${VITE_LOG}" 2>/dev/null || true
fi

exit $FAIL
