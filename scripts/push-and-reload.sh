#!/usr/bin/env bash
# =============================================================================
# push-and-reload.sh —— 先问能力，再推包，再自动重载
# -----------------------------------------------------------------------------
# 为什么先问能力（2026-10-06 教训：白推三次、用户重启两次）：
#
#   此前脚本/Hub 靠**硬编码**假设设备能做什么 ⇒ 推一个注定失败的包，
#   等真机报错（missing_abi / missing required files）才知道不行。
#
#   现在下发之前先问一句：GET /api/peerlink/peer/capabilities?peerId=…
#   受控端自报"我支持哪些方法、哪些包、ABI 是什么、版本是多少"，
#   脚本据此判断：
#     · 不支持 bundle_update      ⇒ 直接退出（热更通道根本不通）
#     · 受控端没声明该包          ⇒ 直接退出（不猜必含文件）
#     · 受控端声明需要 ABI 却拿不到 ⇒ 直接退出（换执行体必崩）
#     · 不支持 bundle_reload      ⇒ 包照推，但不发重载，并提示需要手动重启
#
# 用法：
#   bash scripts/push-and-reload.sh <peerId> <name> <version> [abi]
#   bash scripts/push-and-reload.sh aa2ae6d94645 web v0.0.1-trust1
#   bash scripts/push-and-reload.sh aa2ae6d94645 go-binary v0.0.3 arm64-v8a
#   HUB=http://127.0.0.1:2025 bash scripts/push-and-reload.sh <peerId> web v1
#   FORCE=1 bash scripts/push-and-reload.sh ...   # 忽略能力预检（不建议）
# =============================================================================

set -euo pipefail

HUB="${HUB:-http://127.0.0.1:2025}"
FORCE="${FORCE:-0}"
OP_HEADER=(-H "X-Peerlink-Operator: 1" -H "Content-Type: application/json")

PEER_ID="${1:-}"
NAME="${2:-}"
VERSION="${3:-}"
ABI="${4:-}"

if [[ -z "$PEER_ID" || -z "$NAME" || -z "$VERSION" ]]; then
  echo "用法: bash scripts/push-and-reload.sh <peerId> <name> <version> [abi]" >&2
  exit 2
fi

log() { printf '\033[1;36m[push-reload]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[push-reload]\033[0m %s\n' "$*" >&2; }

# ── 0) 能力预检：问设备，而不是猜 ──────────────────────────────────────────
CAPS="$(curl -s -m 60 "${OP_HEADER[@]}" "$HUB/api/peerlink/peer/capabilities?peerId=$PEER_ID")"

if ! printf '%s' "$CAPS" | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get('ok') else 1)" 2>/dev/null; then
  err "拿不到受控端能力：$CAPS"
  err "（对端离线？还是不支持 peer_capabilities？先确认设备在 /api/peerlink/peers 里 online=true）"
  [[ "$FORCE" == "1" ]] || exit 1
  log "FORCE=1 ⇒ 忽略能力预检继续"
  CAPS=""
fi

if [[ -n "$CAPS" ]]; then
  printf '%s' "$CAPS" | python3 - "$NAME" "$ABI" <<'PY'
import json, sys
caps_raw, want_name, want_abi = sys.argv[1], sys.argv[2], sys.argv[3]
d = json.loads(caps_raw)
caps = d.get("caps") or {}
methods = caps.get("methods") or []
specs = caps.get("bundleSpecs") or []

print(f"  受控端: version={caps.get('binaryVersion') or '?'} abi={caps.get('abi') or '未知'} "
      f"{caps.get('goos')}/{caps.get('goarch')}")
print(f"  支持的方法: {', '.join(methods) or '(无)'}")
unknowns = caps.get("unknowns") or []
if unknowns:
    print(f"  ⚠️ 受控端无法确定的项: {', '.join(unknowns)}")

blockers = []
if "bundle_update" not in methods:
    blockers.append("对端不支持 bundle_update：热更通道不可用")
spec = next((s for s in specs if s.get("name") == want_name), None)
if not spec:
    blockers.append(f"对端未声明包 '{want_name}'（它只认：{', '.join(s.get('name','') for s in specs) or '无'}）")
else:
    if not spec.get("available"):
        blockers.append(f"包 '{want_name}' 在这台设备上没有落地目标（本端不可用）")
    if spec.get("abiRequired") and not (want_abi or caps.get("abi")):
        blockers.append(f"包 '{want_name}' 必须校验 ABI，但既没传 abi 参数、受控端也没自报 ABI")

for b in blockers:
    print(f"  ❌ {b}")
sys.exit(1 if blockers else 0)
PY
  PRECHECK_RC=$?
  if [[ $PRECHECK_RC -ne 0 ]]; then
    if [[ "$FORCE" != "1" ]]; then
      err "能力预检未通过 ⇒ **不下发**（避免推一个注定失败的包）。"
      err "确认无误可加 FORCE=1 强制下发（不建议）。"
      exit 1
    fi
    log "FORCE=1 ⇒ 忽略预检阻塞继续"
  fi

  RELOAD_OK="$(printf '%s' "$CAPS" | python3 -c "
import json,sys
d=json.loads(sys.stdin)
print('1' if 'bundle_reload' in (d.get('caps',{}).get('methods') or []) else '0')
")"
else
  RELOAD_OK="1"
fi

# ── 1) 推包 ────────────────────────────────────────────────────────────────
push_body="{\"peerId\":\"$PEER_ID\",\"name\":\"$NAME\",\"version\":\"$VERSION\""
if [[ -n "$ABI" ]]; then
  push_body="$push_body,\"abi\":\"$ABI\""
fi
push_body="$push_body}"

log "push $NAME@$VERSION → $PEER_ID"
PUSH_RES="$(curl -s -m 180 -X POST "${OP_HEADER[@]}" -d "$push_body" "$HUB/api/peerlink/bundle/push")"
echo "$PUSH_RES"

if ! printf '%s' "$PUSH_RES" | python3 -c "import json,sys; sys.exit(0 if json.load(sys.stdin).get('ok') else 1)"; then
  err "推送失败，跳过重载（先看上面的 error）"
  exit 1
fi

# ── 2) 按包类型决定重载级别 ────────────────────────────────────────────────
if [[ "$NAME" == go-binary* ]]; then
  LEVEL="app"
else
  LEVEL="web"
fi

if [[ "$RELOAD_OK" != "1" ]]; then
  log "⚠️ 受控端不支持 bundle_reload ⇒ 已装包，但**需要手动重启 App** 才生效"
  exit 0
fi

log "reload level=$LEVEL（设备自己生效，不需要用户手动重启）"
RELOAD_RES="$(curl -s -m 120 -X POST "${OP_HEADER[@]}" \
  -d "{\"peerId\":\"$PEER_ID\",\"level\":\"$LEVEL\",\"reason\":\"auto after push $NAME@$VERSION\"}" \
  "$HUB/api/peerlink/bundle/reload")"
echo "$RELOAD_RES"

if ! printf '%s' "$RELOAD_RES" | python3 -c "import json,sys; sys.exit(0 if json.load(sys.stdin).get('ok') else 1)"; then
  err "重载指令失败（包已装上；activity/app 级还需用户在手机上确认）"
  exit 1
fi

log "完成：包已装 + 已触发 $LEVEL 级重载"
