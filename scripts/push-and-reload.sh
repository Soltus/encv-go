#!/usr/bin/env bash
# =============================================================================
# push-and-reload.sh —— 推包 + **自动触发对应级别的重载**
# -----------------------------------------------------------------------------
# 为什么需要它（2026-10-06 用户质询："云控重启了为什么还要我重启？"）：
#
#   热更包推完后，此前**只能靠用户手动重启 App** 才能生效 ⇒ 云控热更新形同虚设，
#   "推完了但没变化"还容易被误判成"没推成功"。
#
#   现在推完立刻按包类型触发重载：
#     web / preview-assets → reload(web)   前端无感重载（不用重开 App）
#     go-binary            → reload(app)   自动重启进程（换执行体必须重启）
#
#   ⇒ 整个流程不需要人在手机上做任何操作。
#
# 用法：
#   bash scripts/push-and-reload.sh <peerId> <name> <version> [abi]
#   bash scripts/push-and-reload.sh aa2ae6d94645 web v0.0.1-trust1
#   bash scripts/push-and-reload.sh aa2ae6d94645 go-binary v0.0.2-script arm64-v8a
#   HUB=http://127.0.0.1:2025 bash scripts/push-and-reload.sh <peerId> web v1
#
# 环境变量：
#   HUB    Hub 地址（默认 http://127.0.0.1:2025）
# =============================================================================

set -euo pipefail

HUB="${HUB:-http://127.0.0.1:2025}"
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

# ── 1) 推包 ────────────────────────────────────────────────────────────────
push_body="{\"peerId\":\"$PEER_ID\",\"name\":\"$NAME\",\"version\":\"$VERSION\""
if [[ -n "$ABI" ]]; then
  push_body="$push_body,\"abi\":\"$ABI\""
fi
push_body="$push_body}"

log "push $NAME@$VERSION → $PEER_ID"
PUSH_RES="$(curl -s -m 180 -X POST "${OP_HEADER[@]}" -d "$push_body" "$HUB/api/peerlink/bundle/push")"
echo "$PUSH_RES"

# 失败就到此为止（reload 没有意义）
if ! printf '%s' "$PUSH_RES" | python3 -c "import json,sys; sys.exit(0 if json.load(sys.stdin).get('ok') else 1)"; then
  err "推送失败，跳过重载（先看上面的 error）"
  exit 1
fi

# ── 2) 按包类型决定重载级别 ────────────────────────────────────────────────
# 换执行体（go-binary）必须整进程重启；其余都是 web 资源 ⇒ 无感重载即可。
if [[ "$NAME" == go-binary* ]]; then
  LEVEL="app"
else
  LEVEL="web"
fi

log "reload level=$LEVEL（让设备自己生效，不需要用户手动重启）"
RELOAD_RES="$(curl -s -m 120 -X POST "${OP_HEADER[@]}" \
  -d "{\"peerId\":\"$PEER_ID\",\"level\":\"$LEVEL\",\"reason\":\"auto after push $NAME@$VERSION\"}" \
  "$HUB/api/peerlink/bundle/reload")"
echo "$RELOAD_RES"

if ! printf '%s' "$RELOAD_RES" | python3 -c "import json,sys; sys.exit(0 if json.load(sys.stdin).get('ok') else 1)"; then
  err "重载指令失败（包已装上，但可能需要设备端取指令；确认 App 在前台）"
  exit 1
fi

log "完成：包已装 + 已触发 $LEVEL 级重载"
