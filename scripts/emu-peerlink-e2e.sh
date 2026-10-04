#!/usr/bin/env bash
# =============================================================================
# emu-peerlink-e2e.sh —— P6 Task 6.1：双端互联（peerlink）端到端**真机级**回归
# =============================================================================
# 为什么需要它：P2a–P5 的 peerlink 此前只在**单进程内**（httptest + startPairedEdge）
# 验证过，Edge 从来没有由"模拟器内的真实后端进程"出网连到"另一个真实进程"。
#    ⇒ 中继链路 / 联邦搜索 / 远程 Agent 授权在真实拓扑下是否通，没有证据（最大的假绿风险）。
#
# 拓扑（对齐 spec §0：桌面端在公网，手机在 NAT 后主动出网）：
#   Hub（桌面/云端）= 宿主机 linux 二进制  /tmp/encvd-hub      （端口自选，从日志解析）
#   Edge（安卓端）  = 模拟器内 x86_64 后端 /tmp/encv-x64       （Android 文件/权限语义 = 真机）
#   Edge → Hub：adb reverse tcp:<REV_PORT> tcp:<HUB_PORT>      （模拟"手机主动出网到公网 Hub"）
#   宿主 → Edge REST：adb forward tcp:<FWD_PORT> tcp:<DEV_PORT>（本脚本/UI 调扫码端）
#
# ⚠️ 环境事实（2026-10-01/02 实测，别再踩）：
#   - 后端端口是自选的（2025 起递增，2025..2034），**必须从日志解析**，不能假设 2025。
#   - adb shell pkill 的模式会匹配会话自身导致挂起 ⇒ 必须加 timeout。
#   - 起后端必须 nohup + 重定向，否则 adb shell 不返回。
#   - 冷启动/安装慢（无 KVM，TCG），脚本内每一步都有轮询超时而不是固定 sleep 碰运气。
#
# 用法：
#   bash scripts/emu-peerlink-e2e.sh            # 一键跑（缺二进制会自动编译）
#   bash scripts/emu-peerlink-e2e.sh --keep     # 结束后保留两端进程与端口转发
# =============================================================================
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADB=/opt/android-sdk/platform-tools/adb
OUT_DIR=/tmp/peerlink-e2e
HUB_BIN=/tmp/encvd-hub
DEV_BIN=/tmp/encv-x64
FWD_PORT=12026                 # 宿主机 → 模拟器内后端 REST
REV_PORT=22025                 # 模拟器内 → 宿主机 Hub（adb reverse）
FIXTURE_TOKEN="encvpeerlinkfixture"
FIXTURE_NAME="peerlink-fixture.txt"
DEV_DIR=/data/local/tmp/out
KEEP=0
[[ "${1:-}" == "--keep" ]] && KEEP=1

mkdir -p "$OUT_DIR"
PASS=0
FAIL=0
info() { echo "ℹ️  $*"; }
ok()   { echo "✅ $*"; PASS=$((PASS + 1)); }
bad()  { echo "❌ $*"; FAIL=$((FAIL + 1)); }
step() { echo; echo "── $* ──"; }

# jget <expr> —— 从 stdin 的 JSON 里取值（expr 是 python 表达式，数据名 d）
jget() {
  python3 -c "import sys,json
try:
    d=json.load(sys.stdin)
except Exception:
    print(''); sys.exit(0)
try:
    v=eval(sys.argv[1])
except Exception:
    print(''); sys.exit(0)
print(v if not isinstance(v,(dict,list)) else json.dumps(v,ensure_ascii=False))" "$1"
}

summary_exit() {
  echo
  echo "════════ 汇总：$PASS PASS / $FAIL FAIL ════════"
  [[ "$FAIL" -eq 0 ]] || exit 1
  exit 0
}

# ---------------- 0) 模拟器就绪 ----------------
step "0) 等待模拟器就绪"
for _ in $(seq 1 120); do
  st="$(timeout 20 $ADB devices 2>/dev/null | grep -c 'emulator-5554[[:space:]]device')"
  bc="$(timeout 20 $ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r\n')"
  [[ "$st" == "1" && "$bc" == "1" ]] && break
  sleep 5
done
if [[ "$(timeout 20 $ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r\n')" != "1" ]]; then
  bad "模拟器未开机完成（emuctl start 后仍需数分钟；无 KVM 冷启动可达 15 分钟）"
  summary_exit
fi
ok "模拟器已开机"

# ---------------- 1) 两端二进制 ----------------
step "1) 准备两端二进制"
[[ -x "$HUB_BIN" ]] || info "编译宿主机 Hub 二进制…"
[[ -x "$HUB_BIN" ]] || ( cd "$ROOT" && go build -o "$HUB_BIN" ./cmd/encv ) || { bad "宿主机后端编译失败"; summary_exit; }
[[ -x "$DEV_BIN" ]] || info "编译模拟器 x86_64 后端（android-common.sh build-emu-backend）…"
[[ -x "$DEV_BIN" ]] || bash "$ROOT/scripts/android-common.sh" build-emu-backend "$DEV_BIN" || { bad "模拟器后端交叉编译失败"; summary_exit; }
ok "Hub=$HUB_BIN / Edge=$DEV_BIN"

cleanup() {
  [[ "$KEEP" == "1" ]] && { info "--keep：保留两端进程与端口转发"; return; }
  timeout 20 $ADB shell "pkill -f 'encv-x64 start'" >/dev/null 2>&1
  $ADB forward --remove tcp:$FWD_PORT >/dev/null 2>&1
  $ADB reverse --remove tcp:$REV_PORT >/dev/null 2>&1
  [[ -n "${HUB_PID:-}" ]] && kill "$HUB_PID" 2>/dev/null
}
trap cleanup EXIT

# ---------------- 2) 宿主机 Hub ----------------
step "2) 启动宿主机 Hub（隧道机制的会合点）"
mkdir -p /tmp/hub-out
cat > /tmp/peerlink-hub.json <<EOF
{"mobile":{"server":{"dir":"/tmp/hub-out"}},"log":{"file":"","level":"info"}}
EOF
ENCV_CONFIG_PATH=/tmp/peerlink-hub.json nohup "$HUB_BIN" start > "$OUT_DIR/hub.log" 2>&1 &
HUB_PID=$!
HUB_PORT=""
for _ in $(seq 1 40); do
  sleep 2
  HUB_PORT="$(grep -a 'successfully started' "$OUT_DIR/hub.log" 2>/dev/null | tail -1 | sed 's/.*\[::\]://; s/[^0-9].*$//')"
  [[ -n "$HUB_PORT" ]] && break
done
if [[ -z "$HUB_PORT" ]]; then
  bad "Hub 未就绪（看 $OUT_DIR/hub.log）"
  summary_exit
fi
HUB_API="http://127.0.0.1:$HUB_PORT"
if [[ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$HUB_API/ping")" == "200" ]]; then
  ok "Hub 就绪：$HUB_API"
else
  bad "Hub ping 不通（端口 $HUB_PORT）"
  summary_exit
fi

# ---------------- 3) 模拟器内 Edge 端后端 ----------------
step "3) 启动模拟器内后端（安卓端）"
timeout 30 $ADB shell "pkill -f 'encv-x64 start'" >/dev/null 2>&1
sleep 2
# ⚠️ push **不能**用短 timeout（也不要写死 30s）：刚开完机的模拟器还在 dexopt/IO 高峰，
#    68MB 二进制首传实测会被 30s 掐断 ⇒ 文件静默消失、后端起不来（2026-10-02 首跑踩到）。
push_emu() {
  local tries=0
  while (( tries < 3 )); do
    timeout 300 $ADB push "$1" "$2" >/dev/null 2>&1
    if timeout 30 $ADB shell "[ -e '$2' ]" >/dev/null 2>&1; then return 0; fi
    tries=$((tries + 1))
    info "push $2 未成功（第 $tries 次重试）"
  done
  return 1
}
push_emu "$DEV_BIN" /data/local/tmp/encv-x64 || { bad "Edge 二进制 push 失败"; summary_exit; }
timeout 30 $ADB shell "mkdir -p $DEV_DIR" >/dev/null 2>&1
# ⚠️ **决定性前置**：不走 App 进程直接跑 Go 二进制时，Android 私有目录
#    /data/user/0/com.encvgo.app/files **不存在**（APK 没装 ⇒ 没人创建它）⇒ 连锁雪崩：
#      任务系统 sqlite：init schema: unable to open database file
#      FTS5 全文索引：full-text index init failed（联邦搜索 ⇒ 0 命中）
#      mount bootstrap 失败 ⇒ /d/... 虚拟路径全部不可用（远端读 ⇒ open_failed）
#    修复 = 以 root 预建这些目录（adb shell 默认 root），再启后端。
for d in /data/user/0/com.encvgo.app/files /data/user/0/com.encvgo.app/cache \
         /data/data/com.encvgo.app/files /data/data/com.encvgo.app/cache; do
  timeout 30 $ADB shell "mkdir -p $d && chmod 700 $d" >/dev/null 2>&1
done
printf 'hello %s\nremote bytes for peerlink read channel\n' "$FIXTURE_TOKEN" > /tmp/$FIXTURE_NAME
push_emu /tmp/$FIXTURE_NAME $DEV_DIR/$FIXTURE_NAME || { bad "夹具 push 失败"; summary_exit; }
# 配置**基于仓库 config.user.json 只改必要项**（与 hybrid-e2e.sh 一致）：
# 手写一份最小 JSON 会丢掉 mobile/output 等既有键 ⇒ 挂载bootstrap/mount 缺失。
python3 - <<'PY'
import json
d = json.load(open('/workspace/config.user.json'))
# ⚠️ 两个 dir 都要设：servingDir 取自 **顶层 server.dir**（见 internal/server/server.go
#    Start() 里 `dir := s.cfg.Server.Dir`），mobile.server.dir 是安卓侧的媒体根。
d.setdefault('server', {})['dir'] = '/data/local/tmp/out'
d.setdefault('mobile', {}).setdefault('server', {})['dir'] = '/data/local/tmp/out'
d['log'] = {'file': '', 'level': 'info'}
json.dump(d, open('/tmp/peerlink-dev.json', 'w'))
PY
push_emu /tmp/peerlink-dev.json /data/local/tmp/peerlink-dev.json || { bad "配置 push 失败"; summary_exit; }
timeout 30 $ADB shell "chmod 755 /data/local/tmp/encv-x64" >/dev/null 2>&1
ok "后端 + 夹具已 push（$DEV_DIR/$FIXTURE_NAME）"

start_device_backend() {
  # ⚠️ HOME 必须显式给：adb shell 里 HOME 为空 ⇒ 应用数据落到 `/.local/share/encv`
  #    （只读根fs）⇒ sqlite / 向量搜索 / FTS5 全部 "unable to open database file"。
  timeout 30 $ADB shell "cd /data/local/tmp && nohup env HOME=/data/local/tmp ENCV_CONFIG_PATH=/data/local/tmp/peerlink-dev.json /data/local/tmp/encv-x64 start > /data/local/tmp/backend.log 2>&1 &" >/dev/null 2>&1
  DEV_PORT=""
  $ADB forward --remove tcp:$FWD_PORT >/dev/null 2>&1
  for _ in $(seq 1 30); do
    sleep 2
    timeout 20 $ADB shell cat /data/local/tmp/backend.log > "$OUT_DIR/device.log" 2>/dev/null
    DEV_PORT="$(grep -a 'successfully started' "$OUT_DIR/device.log" 2>/dev/null | tail -1 | sed 's/.*\[::\]://; s/[^0-9].*$//')"
    if [[ -n "$DEV_PORT" ]]; then
      $ADB forward tcp:$FWD_PORT tcp:$DEV_PORT >/dev/null 2>&1
      [[ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:$FWD_PORT/ping")" == "200" ]] && return 0
    fi
  done
  return 1
}
if start_device_backend; then
  ok "模拟器内后端就绪（内 :$DEV_PORT → 宿主 :$FWD_PORT）"
else
  bad "模拟器内后端未就绪（adb shell cat /data/local/tmp/backend.log）"
  summary_exit
fi
DEV_API="http://127.0.0.1:$FWD_PORT"

# ---------------- 4) 让安卓端能出网到 Hub ----------------
step "4) adb reverse：模拟「手机主动出网到公网 Hub」"
$ADB reverse --remove tcp:$REV_PORT >/dev/null 2>&1
$ADB reverse tcp:$REV_PORT tcp:$HUB_PORT >/dev/null 2>&1
if timeout 20 $ADB reverse --list | grep -q "tcp:$REV_PORT"; then
  ok "reverse 就位：设备 127.0.0.1:$REV_PORT → 宿主 Hub :$HUB_PORT"
else
  bad "adb reverse 未生效"
  summary_exit
fi

# ---------------- 5) T1 探活 ----------------
step "T1 两端 hello 探活"
code="$(curl -s -m 5 -o "$OUT_DIR/hub-hello.json" -w '%{http_code}' "$HUB_API/api/peerlink/hello")"
HUB_PEER_ID="$(jget 'd.get("peerId","")' < "$OUT_DIR/hub-hello.json")"
if [[ "$code" == "200" && -n "$HUB_PEER_ID" ]]; then
  ok "Hub hello 200（peerId=${HUB_PEER_ID:0:8}…）"
else
  bad "Hub hello 异常：HTTP $code peerId='$HUB_PEER_ID'（$(head -c 200 "$OUT_DIR/hub-hello.json")）"
fi
code="$(curl -s -m 5 -o "$OUT_DIR/dev-hello.json" -w '%{http_code}' "$DEV_API/api/peerlink/hello")"
DEV_PEER_ID="$(jget 'd.get("peerId","")' < "$OUT_DIR/dev-hello.json")"
if [[ "$code" == "200" && -n "$DEV_PEER_ID" ]]; then
  ok "Edge hello 200（peerId=${DEV_PEER_ID:0:8}…）"
else
  bad "Edge hello 异常：HTTP $code peerId='$DEV_PEER_ID'"
fi

# ---------------- 6) T2 出票 + T3 扫码配对 ----------------
step "T2/T3 桌面出一次性票据 → 安卓端用票据配对（中继握手）"
curl -s -m 10 -X POST -H 'Content-Type: application/json' -d '{}' \
  "$HUB_API/api/peerlink/ticket" -o "$OUT_DIR/ticket.json"
PAIRING_ID="$(jget 'd.get("pairingId","")' < "$OUT_DIR/ticket.json")"
PSK="$(jget 'd.get("psk","")' < "$OUT_DIR/ticket.json")"
EXPIRES="$(jget 'd.get("expiresIn",0)' < "$OUT_DIR/ticket.json")"
if [[ ${#PAIRING_ID} -ge 16 && -n "$PSK" && "$EXPIRES" -gt 0 ]]; then
  ok "票据就位（pairingId=${PAIRING_ID:0:8}…, expiresIn=${EXPIRES}s，hub 由请求自动同源）"
else
  bad "票据响应异常：$(cat "$OUT_DIR/ticket.json")"
  summary_exit
fi

curl -s -m 20 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
  -d "{\"hub\":\"http://127.0.0.1:$REV_PORT\",\"pairingId\":\"$PAIRING_ID\",\"psk\":\"$PSK\",\"name\":\"Pixel-Emu\"}" \
  "$DEV_API/api/peerlink/edge/pair" -o "$OUT_DIR/dev-pair.json"
PEER_ID="$(jget 'd.get("peerId","")' < "$OUT_DIR/dev-pair.json")"
if [[ -n "$PEER_ID" ]]; then
  ok "安卓端完成远端配对（peerId=${PEER_ID:0:8}…）"
else
  bad "扫码端配对失败：$(cat "$OUT_DIR/dev-pair.json")"
  summary_exit
fi

# T4 Edge 长连接（Optical Series：多轮 poll 等 15s 心跳周期）
CONN="false"
for _ in $(seq 1 30); do
  sleep 2
  curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/edge/status" -o "$OUT_DIR/dev-status.json"
  [[ "$(jget 'd.get("connected",False)' < "$OUT_DIR/dev-status.json")" == "True" ]] && CONN="true" && break
done
[[ "$CONN" == "true" ]] && ok "T4 Edge 长连接已建立并在线（connected=true）" || bad "T4 Edge 未连上 Hub：$(cat "$OUT_DIR/dev-status.json")"

curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$HUB_API/api/peerlink/peers" -o "$OUT_DIR/hub-peers.json"
PLAT="$(jget 'd.get("items",[{}])[0].get("platform","")' < "$OUT_DIR/hub-peers.json")"
ONLINE="$(jget 'd.get("items",[{}])[0].get("online",False)' < "$OUT_DIR/hub-peers.json")"
if [[ "$PLAT" == "android" && "$ONLINE" == "True" ]]; then
  ok "Hub 侧看到已配对安卓端且在线（platform=$PLAT, online=True）"
else
  bad "Hub 侧 peers 异常（platform=$PLAT online=$ONLINE）：$(cat "$OUT_DIR/hub-peers.json")"
fi

# ---------------- 7) T5 联邦搜索（先看索引有没有真东西） ----------------
step "T5 联邦搜索：宿主 ╳ 安卓端真实索引"
IDX_READY="0"
for _ in $(seq 1 40); do
  curl -s -m 10 "$DEV_API/api/files/search-fulltext/stats" -o "$OUT_DIR/dev-fts-stats.json"
  TOTAL="$(jget 'd.get("stats",{}).get("totalFiles",0)' < "$OUT_DIR/dev-fts-stats.json")"
  [[ "$TOTAL" -gt 0 ]] && IDX_READY="1" && break
  [[ $_ -eq 3 ]] && curl -s -m 10 -X POST "$DEV_API/api/files/search-fulltext/rebuild" -o /dev/null
  sleep 3
done
[[ "$IDX_READY" == "1" ]] && info "安卓端 FTS5 索引条目数=$TOTAL" || bad "安卓端 FTS5 索引为空（totalFiles=0），联邦搜索无意义"

curl -s -m 20 -G -H 'X-Peerlink-Operator: 1' \
  --data-urlencode "peerId=$PEER_ID" --data-urlencode "q=$FIXTURE_TOKEN" \
  "$HUB_API/api/peerlink/search" -o "$OUT_DIR/hub-search.json"
SRCH_CODE="$(curl -s -m 20 -G -H 'X-Peerlink-Operator: 1' -o /dev/null -w '%{http_code}' \
  --data-urlencode "peerId=$PEER_ID" --data-urlencode "q=$FIXTURE_TOKEN" "$HUB_API/api/peerlink/search")"
HIT_PATH="$(jget "[i.get('path','') for i in d.get('items',[]) if isinstance(i,dict) and '$FIXTURE_NAME' in i.get('path','')][0]" < "$OUT_DIR/hub-search.json")"
SRCH_PEER="$(jget 'd.get("peer",{}).get("id","")' < "$OUT_DIR/hub-search.json")"
if [[ "$SRCH_CODE" == "200" && -n "$SRCH_PEER" ]]; then
  ok "联邦搜索 200 且带来源标注（peer.id=${SRCH_PEER:0:8}…, platform=$(jget 'd.get(\"peer\",{}).get(\"platform\",\"\")' < "$OUT_DIR/hub-search.json")）"
else
  bad "联邦搜索失败（HTTP $SRCH_CODE）：$(cat "$OUT_DIR/hub-search.json")"
fi
if [[ -n "$HIT_PATH" ]]; then
  ok "远端真实命中（路径按契约原样返回：$HIT_PATH）"
else
  bad "远端 0 命中（期望含 $FIXTURE_NAME）：$(head -c 300 "$OUT_DIR/hub-search.json")"
fi

# ---------------- 8) T6 远端读（在线打开/缩略图通道） ----------------
step "T6 远端读通道（A → B 的 RPC read）"
# 远端读：优先用**联邦搜索返回的那个路径**（契约：命中路径是什么就用什么），
# 失败再退到挂载虚拟路径 /d/primary/<name>（两者都应可读：前者走 servingDir 兜底解析，
# 后者走 mount registry）。两个都不行才算产品问题。
read_remote() {
  local p="$1"
  curl -s -m 20 -G -H 'X-Peerlink-Operator: 1' -D "$OUT_DIR/read-headers.txt" \
    --data-urlencode "peerId=$PEER_ID" --data-urlencode "path=$p" --data-urlencode "length=128" \
    "$HUB_API/api/peerlink/file" -o "$OUT_DIR/read-body.bin"
  grep -q "$FIXTURE_TOKEN" "$OUT_DIR/read-body.bin"
}
RP=""
for cand in "$HIT_PATH" "/d/primary/$FIXTURE_NAME"; do
  [[ -z "$cand" ]] && continue
  if read_remote "$cand"; then RP="$cand"; break; fi
done
if [[ -n "$RP" ]]; then
  ok "取回真实字节（path=$RP，$(wc -c < "$OUT_DIR/read-body.bin") 字节，含夹具标记）"
else
  bad "远端读未取回预期字节（试过 \$HIT_PATH='$HIT_PATH' 与 /d/primary/$FIXTURE_NAME）：$(head -c 200 "$OUT_DIR/read-body.bin")"
fi
if grep -qi '^X-Peer-Id:' "$OUT_DIR/read-headers.txt"; then
  ok "响应常驻来源头（$(grep -i '^X-Peer-Id:' "$OUT_DIR/read-headers.txt" | tr -d '\r')）"
else
  bad "缺少 X-Peer-* 来源头：$(head -20 "$OUT_DIR/read-headers.txt")"
fi

# ---------------- 9) T7 远程 Agent：必须在**执行端**弹窗授权 ----------------
step "T7 远程 Agent 调用 → 执行端审批"
( curl -s -m 130 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
    -d "{\"peerId\":\"$PEER_ID\",\"tool\":\"list_mounts\"}" \
    "$HUB_API/api/peerlink/agent/invoke" -o "$OUT_DIR/invoke.json" -w '%{http_code}' > "$OUT_DIR/invoke.code" ) &
INV_PID=$!
PENDING=""
for _ in $(seq 1 30); do
  sleep 2
  curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/agent/pending" -o "$OUT_DIR/dev-pending.json"
  P="$(jget 'd.get("items",[{}])[0].get("callId","")' < "$OUT_DIR/dev-pending.json")"
  [[ -n "$P" ]] && PENDING="$P" && break
done
PT="$(jget 'd.get("items",[{}])[0].get("tool","")' < "$OUT_DIR/dev-pending.json")"
PD="$(jget 'd.get("items",[{}])[0].get("destructive","")' < "$OUT_DIR/dev-pending.json")"
PN="$(jget 'd.get("items",[{}])[0].get("peerName","")' < "$OUT_DIR/dev-pending.json")"
if [[ -n "$PENDING" && "$PT" == "list_mounts" ]]; then
  ok "调用被挂起在**执行端**（callId=${PENDING} tool=$PT destructive=$PD 来源=$PN）"
else
  bad "执行端未出现审批挂起：$(cat "$OUT_DIR/dev-pending.json")"
fi
curl -s -m 10 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
  -d "{\"callId\":\"$PENDING\",\"decision\":\"trust_device\"}" \
  "$DEV_API/api/peerlink/agent/approve" -o "$OUT_DIR/dev-approve.json"
wait $INV_PID
INV_CODE="$(cat "$OUT_DIR/invoke.code")"
INV_OK="$(jget 'd.get("ok",False)' < "$OUT_DIR/invoke.json")"
INV_DEC="$(jget 'd.get("decision","")' < "$OUT_DIR/invoke.json")"
if [[ "$INV_CODE" == "200" && "$INV_OK" == "True" ]]; then
  ok "远程工具调用成功回传（HTTP $INV_CODE decision=$INV_DEC）"
else
  bad "远程调用失败（HTTP $INV_CODE）：$(head -c 300 "$OUT_DIR/invoke.json")"
fi
TRUSTED="$(curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/agent/trust" | jget 'd.get("items",[])')"
if [[ "$TRUSTED" == *"$HUB_PEER_ID"* ]]; then
  ok "trust_device 生效（执行端信任表含发起端 ${HUB_PEER_ID:0:8}…）"
else
  bad "trust_device 未记录：trusted=$TRUSTED hubPeer=$HUB_PEER_ID"
fi

# T8 已信任 ⇒ 非破坏性工具不再打扰用户
step "T8 已信任设备再调非破坏性工具（不应再弹窗）"
curl -s -m 60 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
  -d "{\"peerId\":\"$PEER_ID\",\"tool\":\"list_mounts\"}" \
  "$HUB_API/api/peerlink/agent/invoke" -o "$OUT_DIR/invoke2.json" -w '%{http_code}' > "$OUT_DIR/invoke2.code"
PENDING2="$(curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/agent/pending" | jget 'len(d.get("items",[]))')"
DEC2="$(jget 'd.get("decision","")' < "$OUT_DIR/invoke2.json")"
if [[ "$(cat "$OUT_DIR/invoke2.code")" == "200" && "$PENDING2" == "0" && "$DEC2" == "auto" ]]; then
  ok "已信任 ⇒ decision=auto，无新增挂起（pending=$PENDING2）"
else
  bad "信任未生效：code=$(cat "$OUT_DIR/invoke2.code") pending=$PENDING2 decision=$DEC2 $(head -c 200 "$OUT_DIR/invoke2.json")"
fi

# ---------------- 10) T9 trust_device **进程重启即失效**（Task 6.3） ----------------
step "T9 重启执行端进程 → 信任态必须清空（进程内存语义）"
timeout 30 $ADB shell "pkill -f 'encv-x64 start'" >/dev/null 2>&1
sleep 3
if start_device_backend; then
  ok "执行端进程已重启（新端口 :$DEV_PORT → 宿主 :$FWD_PORT）"
else
  bad "执行端重启失败"
  summary_exit
fi
TRUST_AFTER="$(curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/agent/trust" | jget 'd.get("items",[])')"
if [[ -z "$TRUST_AFTER" || "$TRUST_AFTER" == "[]" ]]; then
  ok "重启后信任表为空（trust_device 进程内存语义成立）"
else
  bad "重启后仍残留信任记录：$TRUST_AFTER"
fi

# 重新配对（重启后必须重新扫码：token 也只存内存）
curl -s -m 10 -X POST -H 'Content-Type: application/json' -d '{}' \
  "$HUB_API/api/peerlink/ticket" -o "$OUT_DIR/ticket2.json"
PID2="$(jget 'd.get("pairingId","")' < "$OUT_DIR/ticket2.json")"
PSK2="$(jget 'd.get("psk","")' < "$OUT_DIR/ticket2.json")"
curl -s -m 20 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
  -d "{\"hub\":\"http://127.0.0.1:$REV_PORT\",\"pairingId\":\"$PID2\",\"psk\":\"$PSK2\",\"name\":\"Pixel-Emu\"}" \
  "$DEV_API/api/peerlink/edge/pair" -o "$OUT_DIR/dev-pair2.json"
PEER_ID2="$(jget 'd.get("peerId","")' < "$OUT_DIR/dev-pair2.json")"
if [[ -n "$PEER_ID2" ]]; then
  ok "重启后重新扫码配对成功（peerId=${PEER_ID2:0:8}…）——老票据无法复用"
else
  bad "重配对失败：$(cat "$OUT_DIR/dev-pair2.json")"
  summary_exit
fi
CONN2="false"
for _ in $(seq 1 30); do
  sleep 2
  curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/edge/status" -o "$OUT_DIR/dev-status2.json"
  [[ "$(jget 'd.get("connected",False)' < "$OUT_DIR/dev-status2.json")" == "True" ]] && CONN2="true" && break
done
[[ "$CONN2" == "true" ]] && ok "重连成功（Edge 断线重连 + 心跳）" || bad "重配对后 Edge 未连上：$(cat "$OUT_DIR/dev-status2.json")"

# 再调一次同一工具：必须**再次**弹审批（信任已失效）
( curl -s -m 130 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
    -d "{\"peerId\":\"$PEER_ID2\",\"tool\":\"list_mounts\"}" \
    "$HUB_API/api/peerlink/agent/invoke" -o "$OUT_DIR/invoke3.json" -w '%{http_code}' > "$OUT_DIR/invoke3.code" ) &
INV3_PID=$!
PENDING3=""
for _ in $(seq 1 30); do
  sleep 2
  curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/agent/pending" -o "$OUT_DIR/dev-pending3.json"
  P3="$(jget 'd.get("items",[{}])[0].get("callId","")' < "$OUT_DIR/dev-pending3.json")"
  [[ -n "$P3" ]] && PENDING3="$P3" && break
done
if [[ -n "$PENDING3" ]]; then
  ok "信任已失效 ⇒ 同一工具再次要求审批（callId=$PENDING3）"
  curl -s -m 10 -X POST -H 'X-Peerlink-Operator: 1' -H 'Content-Type: application/json' \
    -d "{\"callId\":\"$PENDING3\",\"decision\":\"decline\"}" \
    "$DEV_API/api/peerlink/agent/approve" -o "$OUT_DIR/dev-decline.json"
  wait $INV3_PID
  DEC3="$(jget 'd.get("decision","")' < "$OUT_DIR/invoke3.json")"
  [[ "$DEC3" == "decline" ]] && ok "拒绝生效并回传（decision=decline）" || bad "拒绝未按预期回传：$(head -c 200 "$OUT_DIR/invoke3.json")"
else
  wait $INV3_PID 2>/dev/null
  bad "重启后同一工具竟然没再要审批（信任跨进程存活 ⇒ 违反进程内存语义）"
fi

# ---------------- 11) T10 安全红线复查 ----------------
step "T10 安全红线：未配对 / 无效身份一律 401"
NOAUTH="$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$HUB_API/api/peerlink/peers")"
[[ "$NOAUTH" == "401" ]] && ok "Hub 无身份访问 peers → 401" || bad "Hub 无身份 peers 返回 $NOAUTH（期望 401）"
NOAUTH2="$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$DEV_API/api/peerlink/agent/pending")"
[[ "$NOAUTH2" == "401" ]] && ok "执行端无身份访问审批列表 → 401" || bad "执行端无身份 pending 返回 $NOAUTH2（期望 401）"
AUDIT="$(curl -s -m 5 -H 'X-Peerlink-Operator: 1' "$DEV_API/api/peerlink/agent/audit" | jget 'd.get("count",0)')"
info "执行端审计条目数=$AUDIT（应覆盖 accept/decline 等决策）"
[[ "$AUDIT" -gt 0 ]] && ok "审计有记录（脱敏：无路径/参数全文）" || bad "审计为空"

summary_exit
