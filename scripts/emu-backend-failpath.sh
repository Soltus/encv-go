#!/usr/bin/env bash
# =============================================================================
# emu-backend-failpath.sh —— 模拟器内真实后端的「失败路径 / 旁路」断言
# -----------------------------------------------------------------------------
# 为什么单独成脚本：
#   前面两个闸门验的都是"文件没问题"时的主链路。真机上用户最容易撞到的
#   —— 密码错、文件坏了、文件压根不是容器 —— 这条**失败语义**从来没在真机通路上
#   被断言过。失败必须**显式失败**：
#     · 密码错   → 403 + wrong_password（前端要能提示"密码可能错误"）
#     · 数据损坏 → 4xx/5xx，**绝不能 200 吐一堆乱码**（静默乱码最难查）
#     · 非容器   → 按普通文件伺服（浏览器/下载器要能直接拿到原文件）
#     · /decrypt → 与 /stream 语义一致（前端两条入口）
#
# ⚠️ 已知第 2 项目前是**红的**（真缺陷，尚未修，等写入格式决策）——
#    2026-10-02 真机实测：篡改容器中间 512 字节后 `/stream` 返回
#    **200 + 长度完全正确 + 内容从第 25496 字节起是乱码，且不报错**。
#    根因（已取证）：`encrypt-v2` 默认写出的 v4 容器 `data_crc32 = 0`、未启用 HMAC
#    ⇒ 读取端**没有任何完整性元数据可校验**；而 `GetFragmentReader` 里唯一那份
#    CRC 校验被 `if r.headerVersion != 4` 排除，v4 根本不走。
#    （KVI 里其实有 `original_file_md5`，但目前没人用来校验。）
#    候选修法（需产品决策，勿擅自动写入格式）：
#      A. 写入端默认启用 HMAC（容器格式契约变更，向后兼容需评估）
#      B. 读取端：v4 也走 fragment CRC（前提是容器带 CRC）
#      C. 读取端：全量读完后用 KVI 的 original_file_md5 后置校验（至少覆盖下载/缓存分支）
#
# 断言（任一失败 exit 1）：
#   1) 错密码容器     → 403 且 body 含 wrong_password
#   2) 损坏容器       → 非 200/206；若 200 则内容必须与明文一致（否则=静默乱码）
#   3) 非容器明文文件 → 200 + Range 中段字节一致（Go 原生 ServeFile 路径）
#   4) /decrypt       → 206 + 与明文一致（与 /stream 同语义）
#   5) 存量容器（--legacy，无 DataCRC32 的老容器）→ 仍必须 200 + 全量一致
#      （写入端开始写 CRC 之后，**向后兼容**由这一条守住）
#   6) 大文件损坏（--big，>3MB 走流式分支）→ 后端日志必须出现完整性校验失败，
#      且**不得**出现 "Successfully served"（否则=把损坏数据当成功吐出去了）
#
# 用法：
#   bash scripts/emu-backend-failpath.sh [选项]
#     --api    <base>       默认 http://127.0.0.1:12025
#     --plain  <明文文件>    默认 /tmp/src/sample.mp4
#     --sample <合法容器>    默认 /tmp/src/out/sample.4pm.sccgv
#     --legacy <老容器>      （可选）新写入端改造前产出的容器，验证向后兼容
#     --big    <大容器>      （可选）>3MB 容器，验证流式分支的损坏能被**发现**
#     --backend-log <路径>   模拟器内后端日志，默认 /data/local/tmp/backend.log
#     --remote <模拟器目录>  默认 /data/local/tmp/out
# =============================================================================
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADB=/opt/android-sdk/platform-tools/adb
API="${EMU_API:-http://127.0.0.1:12025}"
PLAIN="${FP_PLAIN:-/tmp/src/sample.mp4}"
SAMPLE="${FP_SAMPLE:-/tmp/src/out/sample.4pm.sccgv}"
LEGACY="${FP_LEGACY:-}"
BIG="${FP_BIG:-}"
BACKEND_LOG="${FP_BACKEND_LOG:-/data/local/tmp/backend.log}"
REMOTE_DIR="${FP_REMOTE:-/data/local/tmp/out}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --api)         API="$2";         shift 2 ;;
    --plain)       PLAIN="$2";       shift 2 ;;
    --sample)      SAMPLE="$2";      shift 2 ;;
    --legacy)      LEGACY="$2";      shift 2 ;;
    --big)         BIG="$2";         shift 2 ;;
    --backend-log) BACKEND_LOG="$2"; shift 2 ;;
    --remote)      REMOTE_DIR="$2";  shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

TMP="$(mktemp -d /tmp/emu-failpath.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()   { printf '  ✅ %s\n' "$*"; PASS=$((PASS + 1)); }
info() { printf '  ℹ️  %s\n' "$*"; }
bad()  { printf '  ❌ %s\n' "$*"; FAIL=$((FAIL + 1)); }
head1() { printf '\n── %s ──\n' "$*"; }

[ -f "$PLAIN" ] || { echo "❌ 缺少明文 $PLAIN"; exit 1; }
[ -f "$SAMPLE" ] || { echo "❌ 缺少合法容器 $SAMPLE"; exit 1; }
PSIZE="$(stat -c %s "$PLAIN")"

echo "═══ 模拟器内后端 · 失败路径 / 旁路检查 ═══"
echo "  API  : $API"
echo "  明文 : $PLAIN（$PSIZE 字节）"

code_of() { curl -s -m 120 -G -o "$TMP/body.bin" -w '%{http_code}' "$@" 2>/dev/null; }

c="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "$API/ping")"
[ "$c" = "200" ] || { echo "❌ 后端未就绪（/ping → $c）"; exit 1; }

$ADB devices | grep -q emulator || { echo "❌ 模拟器未启动"; exit 1; }

# ---------- 造三种样例 ----------
# ① 错密码容器：换个口令重加密同一份明文
WRONG_DIR="$TMP/wrongout"
mkdir -p "$WRONG_DIR"
( cd "$ROOT" && go run ./cmd/encv encrypt-v2 "$PLAIN" -p definitely-wrong-key -o "$WRONG_DIR" ) \
  || { echo "❌ 造错密码容器失败"; exit 1; }
WRONG="$(ls "$WRONG_DIR"/*.sccgv 2>/dev/null | head -1)"
[ -n "$WRONG" ] || { echo "❌ 找不到错密码容器产物"; exit 1; }

# ② 损坏容器：把合法容器中间 512 字节抹成 0（不是删头，避免变成"非容器"）
CORRUPT="$TMP/corrupt.4pm.sccgv"
cp "$SAMPLE" "$CORRUPT"
python3 - "$CORRUPT" <<'PY'
import sys
p = sys.argv[1]
data = bytearray(open(p, 'rb').read())
if len(data) < 2048:
    sys.exit(0)
mid = len(data) // 2
data[mid:mid + 512] = b"\x00" * 512
open(p, 'wb').write(bytes(data))
PY

# ③ 非容器明文文件：直接用明文 mp4（Go 会走 http.ServeFile 分支）
PLAIN_REMOTE_NAME="plain-$(basename "$PLAIN")"

$ADB shell mkdir -p "$REMOTE_DIR" >/dev/null 2>&1
$ADB push "$WRONG"   "$REMOTE_DIR/wrong-key.sccgv"     >/dev/null 2>&1 || { echo "❌ push 错密码容器失败"; exit 1; }
$ADB push "$CORRUPT" "$REMOTE_DIR/corrupt.4pm.sccgv"   >/dev/null 2>&1 || { echo "❌ push 损坏容器失败"; exit 1; }
$ADB push "$PLAIN"   "$REMOTE_DIR/$PLAIN_REMOTE_NAME"  >/dev/null 2>&1 || { echo "❌ push 明文文件失败"; exit 1; }

# 虚拟路径前缀（mount root 现算，不写死）
VPATH="$(curl -s -m 10 "$API/api/mounts" | python3 -c '
import json,sys
try: data=json.load(sys.stdin)
except Exception: sys.exit(0)
cands=data.get("mounts", data if isinstance(data,list) else [])
def walk(o):
    if isinstance(o,dict):
        if str(o.get("name") or o.get("id") or "").lower()=="primary":
            for k in ("resolved_root","root_path","root","path","folder","dir","source"):
                if o.get(k): return o[k]
        for v in o.values():
            r=walk(v)
            if r: return r
    elif isinstance(o,list):
        for v in o:
            r=walk(v)
            if r: return r
    return ""
print(walk(cands))
' 2>/dev/null)"
vpath() { # vpath <模拟器绝对路径>
  if [ -n "$VPATH" ]; then printf '/d/primary%s' "${1#"$VPATH"}"; else printf '/d/primary%s' "$1"; fi
}
V_WRONG="$(vpath "$REMOTE_DIR/wrong-key.sccgv")"
V_CORRUPT="$(vpath "$REMOTE_DIR/corrupt.4pm.sccgv")"
V_PLAIN="$(vpath "$REMOTE_DIR/$PLAIN_REMOTE_NAME")"
V_OK="$(vpath "$REMOTE_DIR/$(basename "$SAMPLE")")"
echo "  mount root=${VPATH:-unknown}"

# ---------- 1) 错密码 ----------
head1 "1) 密码错误的容器 → 403 + wrong_password（前端靠它提示用户）"
c1="$(code_of --data-urlencode "path=$V_WRONG" "$API/stream")"
[ "$c1" = "403" ] && ok "错密码 → 403" || bad "错密码 → $c1（期望 403；若 200 说明拿错误密钥解出了垃圾）"
if grep -q 'wrong_password' "$TMP/body.bin" 2>/dev/null; then
  ok "body 含 wrong_password 标记"
else
  bad "body 不含 wrong_password：$(head -c 120 "$TMP/body.bin" 2>/dev/null)"
fi

# ---------- 2) 损坏容器 ----------
head1 "2) 损坏的容器 → 必须显式失败；若仍 200，内容必须与明文一致（否则=静默乱码）"
c2="$(code_of --data-urlencode "path=$V_CORRUPT" "$API/stream")"
case "$c2" in
  200|206)
    cp "$TMP/body.bin" "$TMP/corrupt-out.bin"
    if cmp -s "$TMP/corrupt-out.bin" "$PLAIN"; then
      ok "损坏容器返回 $c2，但内容与明文一致（本次篡改未影响实际数据）"
    else
      bad "损坏容器返回 $c2 且内容与明文不一致 —— **静默乱码**（长度 $(stat -c %s "$TMP/corrupt-out.bin") 与明文一致、无报错；真机表现：文件能列能下，就是播不出来/打开是花屏，且没有任何错误提示）"
    fi ;;
  *) ok "损坏容器返回 $c2（显式失败）" ;;
esac

# ---------- 3) 非容器明文文件 ----------
head1 "3) 非容器文件（明文 mp4）→ 按普通文件伺服，Range 也要对"
c3="$(code_of --data-urlencode "path=$V_PLAIN" "$API/stream")"
[ "$c3" = "200" ] && ok "非容器 → 200" || bad "非容器 → $c3（期望 200）"
off=$(( PSIZE / 2 )); len=4096
code3="$(curl -s -m 120 -G -r "$off-$(( off + len - 1 ))" --data-urlencode "path=$V_PLAIN" "$API/stream" -o "$TMP/plain-seg.bin" -w '%{http_code}' 2>/dev/null)"
tail -c +$(( off + 1 )) "$PLAIN" | head -c "$len" > "$TMP/plain-seg-plain.bin"
if [ "$code3" = "206" ] && cmp -s "$TMP/plain-seg.bin" "$TMP/plain-seg-plain.bin"; then
  ok "非容器 Range 中段字节一致（206）"
else
  bad "非容器 Range 中段异常（HTTP $code3，$(stat -c %s "$TMP/plain-seg.bin" 2>/dev/null) 字节）"
fi

# ---------- 4) /decrypt 与 /stream 同语义 ----------
head1 "4) /decrypt 端点（前端另一条入口）语义必须与 /stream 一致"
code4="$(curl -s -m 120 -G -r "0-4095" --data-urlencode "path=$V_OK" "$API/decrypt" -o "$TMP/decrypt.bin" -w '%{http_code}' 2>/dev/null)"
tail -c +1 "$PLAIN" | head -c 4096 > "$TMP/decrypt-plain.bin"
if [ "$code4" = "206" ] && cmp -s "$TMP/decrypt.bin" "$TMP/decrypt-plain.bin"; then
  ok "/decrypt Range 0-4095 → 206 且字节一致"
else
  bad "/decrypt 异常（HTTP $code4，$(stat -c %s "$TMP/decrypt.bin" 2>/dev/null) 字节；$(head -c 60 "$TMP/decrypt.bin" 2>/dev/null)）"
fi

# ---------- 5) 存量容器向后兼容 ----------
if [ -n "$LEGACY" ] && [ -f "$LEGACY" ]; then
  head1 "5) 存量容器（写入端改造前产出、无 CRC 元数据）必须仍可读"
  $ADB push "$LEGACY" "$REMOTE_DIR/legacy-nocrc.sccgv" >/dev/null 2>&1 || bad "push 存量容器失败"
  V_LEGACY="$(vpath "$REMOTE_DIR/legacy-nocrc.sccgv")"
  c5="$(curl -s -m 120 -G --data-urlencode "path=$V_LEGACY" "$API/stream" -o "$TMP/legacy.bin" -w '%{http_code}' 2>/dev/null)"
  [ "$c5" = "200" ] && ok "存量容器 → 200" || bad "存量容器 → $c5（向后兼容被破坏！）"
  if cmp -s "$TMP/legacy.bin" "$PLAIN"; then
    ok "存量容器全量字节一致（老容器不受新校验影响）"
  else
    bad "存量容器内容不一致（$(stat -c %s "$TMP/legacy.bin" 2>/dev/null) 字节）"
  fi
else
  echo; echo "ℹ️  未指定 --legacy，跳过向后兼容断言"
fi

# ---------- 6) 大文件（流式分支）损坏必须被发现 ----------
# 流式分支（>3MB）不会先把整片读进内存，HTTP 侧 io.Copy 又是靠
# io.LimitReader(contentLength) 收尾 ⇒ 读满即停，**不会**再触发底层那次 io.EOF。
# 所以校验必须在"读满整片"时就判定（crcGuardReader 的判定 2），否则后端会
# 若无其事地记一条 "Successfully served N bytes"，把损坏数据当成功发出去。
if [ -n "$BIG" ] && [ -f "$BIG" ]; then
  head1 "6) 大文件（流式分支）损坏必须被**发现**（看后端日志，不能 Successfully served）"
  cp "$BIG" "$TMP/corrupt-big.sccgv"
  python3 - "$TMP/corrupt-big.sccgv" <<'PY'
import sys
p = sys.argv[1]
d = bytearray(open(p, 'rb').read())
if len(d) >= 2048:
    m = len(d) // 2
    d[m:m + 512] = b"\x00" * 512
open(p, 'wb').write(bytes(d))
PY
  $ADB push "$TMP/corrupt-big.sccgv" "$REMOTE_DIR/corrupt-big.sccgv" >/dev/null 2>&1 || bad "push 大容器失败"
  V_BIG="$(vpath "$REMOTE_DIR/corrupt-big.sccgv")"
  # 用**字节偏移**切出"本次请求新增的日志"：按行数切（toybox wc -l 解析）不可靠，
  # 按固定尾部行数又会串进上一条（正常大文件）请求的 Successfully served。
  timeout 30 $ADB shell cat "$BACKEND_LOG" > "$TMP/before.log" 2>/dev/null
  bsz=$(stat -c %s "$TMP/before.log" 2>/dev/null || echo 0)

  # 【状态码】不带 Range 的全量请求 ⇒ 必须在吐第一个字节前就判坏（422），
  #   而不是 200 + 截断流（分块 CRC 只能做到后者，因为响应头已经发出去了）。
  c6="$(curl -s -m 300 -G --data-urlencode "path=$V_BIG" "$API/stream" -o "$TMP/big-out.bin" -w '%{http_code}' 2>/dev/null)"
  [ "$c6" = "422" ] && ok "损坏的大文件（全量请求）→ 422 data_corrupted（吐字节前就判坏了）" \
    || bad "损坏的大文件（全量请求）→ HTTP $c6（期望 422；200 表示仍是'发了头才发现坏'）"
  sleep 2

  timeout 30 $ADB shell cat "$BACKEND_LOG" > "$TMP/after.log" 2>/dev/null
  tail -c +$(( bsz + 1 )) "$TMP/after.log" > "$TMP/backend-new.log" 2>/dev/null
  if grep -q '完整性校验失败' "$TMP/backend-new.log" 2>/dev/null; then
    ok "流式分支也发现了损坏（后端日志出现完整性校验失败）"
  else
    bad "流式分支**没**发现损坏（日志无完整性校验失败）—— 大文件仍会静默吐乱码"
  fi
  # 只看**大文件那一条**请求的日志（尾部还可能有别的请求的正常 Successfully served）
  # 大文件的原始名是 big.mp4；按它过滤，避免误判成失败
  if grep 'Successfully served' "$TMP/backend-new.log" 2>/dev/null | grep -q 'big.mp4'; then
    bad "后端把损坏数据记为 Successfully served（假装成功）"
  else
    ok "未把损坏数据记为 Successfully served（同一条日志里没有大文件的成功记录）"
  fi

  # ---------- 6b) 带 Range 的请求：不预校验，靠**分块 CRC**在损坏块处中止 ----------
  # 真机场景：播放器拖动/并发分段都会带 Range；这类请求不能为 1KB 去读整片，
  # 所以由分块 CRC 兜底 —— 期望"读到损坏块就停"，而不是把整片吐完。
  BPSIZE="$(stat -c %s "$BIG")"
  timeout 30 $ADB shell cat "$BACKEND_LOG" > "$TMP/before2.log" 2>/dev/null
  bsz2=$(stat -c %s "$TMP/before2.log" 2>/dev/null || echo 0)
  c6b="$(curl -s -m 300 -G -r "0-$(( BPSIZE - 1 ))" --data-urlencode "path=$V_BIG" "$API/stream" -o "$TMP/big-range.bin" -w '%{http_code}' 2>/dev/null)"
  sleep 2
  timeout 30 $ADB shell cat "$BACKEND_LOG" > "$TMP/after2.log" 2>/dev/null
  tail -c +$(( bsz2 + 1 )) "$TMP/after2.log" > "$TMP/backend-new2.log" 2>/dev/null

  # 【早期发现】分块 CRC 的意义：报错位置必须靠前，不能"读完整片才知道"
  early=$(python3 - "$TMP/backend-new2.log" <<'PY'
import re, sys
log = open(sys.argv[1], 'rb').read().decode('utf-8', 'ignore')
best = None
for m in re.finditer(r'起点\s+(\d+)', log):
    s = int(m.group(1))
    best = s if best is None else min(best, s)
print("none" if best is None else str(best))
PY
)
  if [ "$early" = "none" ]; then
    bad "带 Range 请求没有触发块级校验失败（日志无'起点'）—— 分块 CRC 没生效"
  else
    if [ "$early" -lt $(( BPSIZE / 2 )) ]; then
      ok "带 Range 请求在文件前段就中止（块校验起点 $early / 文件 $BPSIZE —— 早于中点）"
    else
      bad "发现得太晚（块校验起点 $early / 文件 $BPSIZE）—— 退回到了整片校验"
    fi
  fi
  # 实体体必须被截断（不能把整片都吐出去）
  got=$(stat -c %s "$TMP/big-range.bin" 2>/dev/null || echo 0)
  if [ "$got" -lt "$BPSIZE" ]; then
    ok "带 Range 请求的实体体被截断（$got < $BPSIZE 字节）—— 没把损坏数据全吐出去"
  else
    bad "带 Range 请求仍返回完整 $got 字节 —— 分块 CRC 没拦住"
  fi
  info "带 Range 请求状态码：$c6b（200 + 截断是预期的，因为响应头已经发出去了）"
else
  echo; echo "ℹ️  未指定 --big，跳过大文件（流式分支）损坏断言"
fi

echo
echo "═══ 结果：${PASS} PASS / ${FAIL} FAIL ═══"
[ "$FAIL" = "0" ] || exit 1
exit 0
