#!/usr/bin/env bash
# =============================================================================
# emu-backend-edge.sh —— 模拟器内真实后端的「HTTP 协议边界」断言（真机语义回归 · 第二部分）
# -----------------------------------------------------------------------------
# 与 emu-backend-check.sh 的分工：
#   emu-backend-check.sh = 主链路契约（/ping、mount、列举、全量、Range 首段/中段、失败路径）
#   本脚本               = 协议边界与健壮性（416/截断/suffix/HEAD/目录/穿越/缺参/并发）
#
# 为什么要在模拟器里跑（见 .codebuddy/memory/2026-10-01.md §10）：
#   后端跑在模拟器里 ⇒ 走 Android 的文件系统/权限/mount 语义，与真机等价；
#   宿主机只做 HTTP 断言（adb forward 打通），不依赖会崩的 WebView。
#
# 断言（任一失败 exit 1）：
#   1) Range 越界（start >= size）      → 416，且**不带实体体**，且带 Content-Range: bytes */size
#   2) Range end 超界（end >> size）    → 206 且截断到 size-1，实体体长度 == size
#   3) suffix Range（bytes=-1024）      → 206，实体体 == 明文最后 1024 字节
#   4) HEAD /stream                    → 200，Content-Length == 明文长度，Accept-Ranges: bytes，无实体体
#   5) 目录路径                         → 非 200/206（不能把目录当文件吐出来）
#   6) 路径穿越（../../etc/passwd）      → 非 200/206，且不能泄出 passwd 内容
#   7) 缺 path 参数                     → 400
#   8) 206 响应                        → 带 Accept-Ranges: bytes
#   9) 并发 4 路不同 Range              → 每条都与明文对应区间逐字节一致
#
# 前置：模拟器内后端已起 + adb forward（一般由 scripts/hybrid-e2e.sh 编排）
# 用法：
#   bash scripts/emu-backend-edge.sh [选项]
#     --api   <base>      默认 http://127.0.0.1:12025
#     --path  <虚拟路径>   默认 /d/primary/out/sample.4pm.sccgv
#     --plain <明文文件>   默认 /tmp/src/sample.mp4（用于字节比对）
# =============================================================================
set -uo pipefail

API="${EMU_API:-http://127.0.0.1:12025}"
FILE="${EMU_PATH:-/d/primary/out/sample.4pm.sccgv}"
PLAIN="${EMU_PLAIN:-/tmp/src/sample.mp4}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --api)   API="$2";   shift 2 ;;
    --path)  FILE="$2";  shift 2 ;;
    --plain) PLAIN="$2"; shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

TMP="$(mktemp -d /tmp/emu-backend-edge.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()   { printf '  ✅ %s\n' "$*"; PASS=$((PASS + 1)); }
bad()  { printf '  ❌ %s\n' "$*"; FAIL=$((FAIL + 1)); }
head1() { printf '\n── %s ──\n' "$*"; }

# code_and_size <curl args...> → 输出 "<http_code> <下载字节数>"
code_and_size() { curl -s -m 60 -o "$TMP/body.bin" -w '%{http_code} %{size_download}' -G "$@" 2>/dev/null; }
# headers_only <curl args...> → 输出响应头
headers_only() { curl -s -m 60 -D - -o /dev/null -G "$@" 2>/dev/null | tr -d '\r'; }

echo "═══ 模拟器内后端 · 协议边界检查 ═══"
echo "  API  : $API"
echo "  容器 : $FILE"
echo "  明文 : $PLAIN"

if [ ! -f "$PLAIN" ]; then
  echo "❌ 缺少明文文件 $PLAIN（字节比对无法进行）"; exit 1
fi
PSIZE="$(stat -c %s "$PLAIN")"
echo "  明文长度: $PSIZE"

c="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "$API/ping")"
if [ "$c" != "200" ]; then echo "❌ 后端未就绪（/ping → $c）"; exit 1; fi

PARENT="$(dirname "$FILE")"

# ---------- 1) Range 越界：416 且不能带实体体 ----------
head1 "1) Range 越界（start >= size）→ 416 + Content-Range: bytes */size + 无实体体"
far=$(( PSIZE * 10 ))
read -r c1 s1 <<<"$(code_and_size -H "Range: bytes=$far-" --data-urlencode "path=$FILE" "$API/stream")"
[ "$c1" = "416" ] && ok "越界 Range → 416" || bad "越界 Range → $c1（期望 416）"
if [ "${s1:-0}" = "0" ]; then
  ok "416 无实体体（0 字节）"
else
  bad "416 竟然带实体体：$s1 字节（应为空；等于整文件则说明把全量数据白送了一遍）"
fi
h1="$(headers_only -H "Range: bytes=$far-" --data-urlencode "path=$FILE" "$API/stream")"
if grep -qi "content-range:[[:space:]]*bytes \*/$PSIZE" <<<"$h1"; then
  ok "416 带 Content-Range: bytes */$PSIZE（RFC 7233 要求）"
else
  bad "416 缺少 'Content-Range: bytes */$PSIZE'：$(grep -i 'content-range' <<<"$h1" | head -1 || echo '<无>')"
fi

# ---------- 2) Range end 超界：合法截断 ----------
head1 "2) Range end 超界 → 206 且截断到 size-1"
read -r c2 s2 <<<"$(code_and_size -H "Range: bytes=0-999999999" --data-urlencode "path=$FILE" "$API/stream")"
[ "$c2" = "206" ] && ok "end 超界 → 206" || bad "end 超界 → $c2（期望 206）"
[ "${s2:-0}" = "$PSIZE" ] && ok "截断后实体体长度 == 明文长度（$s2）" || bad "实体体长度 $s2 ≠ 明文长度 $PSIZE"
h2="$(headers_only -H "Range: bytes=0-999999999" --data-urlencode "path=$FILE" "$API/stream")"
grep -qi "content-range:[[:space:]]*bytes 0-$((PSIZE-1))/$PSIZE" <<<"$h2" \
  && ok "Content-Range: bytes 0-$((PSIZE-1))/$PSIZE" \
  || bad "Content-Range 未正确截断：$(grep -i 'content-range' <<<"$h2" | head -1 || echo '<无>')"

# ---------- 3) suffix Range（最后 N 字节） ----------
head1 "3) suffix Range（bytes=-1024）→ 206 + 末尾 1024 字节正确"
SUF=1024
[ "$PSIZE" -lt "$SUF" ] && SUF="$PSIZE"
read -r c3 s3 <<<"$(code_and_size -H "Range: bytes=-$SUF" --data-urlencode "path=$FILE" "$API/stream")"
[ "$c3" = "206" ] && ok "suffix Range → 206" || bad "suffix Range → $c3（期望 206）"
[ "${s3:-0}" = "$SUF" ] && ok "suffix 实体体长度 == $SUF" || bad "suffix 实体体长度 $s3 ≠ $SUF"
cp "$TMP/body.bin" "$TMP/suffix.bin"
dd if="$PLAIN" of="$TMP/plain-suffix.bin" bs=1 skip=$(( PSIZE - SUF )) count="$SUF" status=none
if cmp -s "$TMP/suffix.bin" "$TMP/plain-suffix.bin"; then ok "末尾 $SUF 字节与明文一致"; else bad "末尾 $SUF 字节与明文不一致（suffix 定位错位？）"; fi

# ---------- 4) HEAD ----------
head1 "4) HEAD /stream → 头正确且无实体体"
h4="$(curl -s -m 60 -I -G -o /dev/null -D - --data-urlencode "path=$FILE" "$API/stream" 2>/dev/null | tr -d '\r')"
c4="$(curl -s -m 60 -I -G -o /dev/null -w '%{http_code}' --data-urlencode "path=$FILE" "$API/stream" 2>/dev/null)"
[ "$c4" = "200" ] && ok "HEAD → 200" || bad "HEAD → $c4（期望 200）"
if grep -qi "^content-length:[[:space:]]*$PSIZE\$" <<<"$h4"; then
  ok "HEAD Content-Length == 明文长度 $PSIZE"
else
  bad "HEAD Content-Length ≠ $PSIZE：$(grep -i '^content-length' <<<"$h4" | head -1 || echo '<无>')"
fi
if grep -qi "^accept-ranges:[[:space:]]*bytes" <<<"$h4"; then
  ok "HEAD 带 Accept-Ranges: bytes"
else
  bad "HEAD 缺少 Accept-Ranges: bytes（播放器会认为不支持拖动）"
fi
# Go net/http 对 HEAD 的处理需要实测：若 ServeFile 未判断 r.Method，可能会真的把解密数据写出去
s4="$(curl -s -m 60 -I --data-urlencode "path=$FILE" "$API/stream" -o "$TMP/head-body.bin" -w '%{size_download}' 2>/dev/null)"
if [ "${s4:-0}" = "0" ]; then
  ok "HEAD 无实体体（0 字节）"
else
  bad "HEAD 竟然返回 $s4 字节实体体（协议违规 + 白白解密全文件）"
fi

# ---------- 5) 目录路径 ----------
head1 "5) 目录路径（不能当文件吐出来）"
c5="$(curl -s -m 60 -G -o "$TMP/dir.bin" -w '%{http_code}' --data-urlencode "path=$PARENT" "$API/stream" 2>/dev/null)"
case "$c5" in
  200|206) bad "目录路径返回 $c5（期望 4xx/5xx）" ;;
  *)       ok "目录路径返回 $c5" ;;
esac

# ---------- 6) 路径穿越 ----------
head1 "6) 路径穿越（../../etc/passwd）"
c6="$(curl -s -m 60 -G -o "$TMP/trav.bin" -w '%{http_code}' --data-urlencode "path=/d/primary/../../../../etc/passwd" "$API/stream" 2>/dev/null)"
case "$c6" in
  200|206)
    if grep -q 'root:' "$TMP/trav.bin" 2>/dev/null; then
      bad "路径穿越成功且泄露 /etc/passwd（$c6）—— 安全漏洞"
    else
      bad "路径穿越返回 $c6（未泄露内容但应拒绝）"
    fi ;;
  *) ok "路径穿越被拒（$c6）" ;;
esac

# ---------- 7) 缺 path 参数 ----------
head1 "7) 缺 path 参数"
c7="$(curl -s -m 60 -o /dev/null -w '%{http_code}' "$API/stream" 2>/dev/null)"
[ "$c7" = "400" ] && ok "缺 path → 400" || bad "缺 path → $c7（期望 400）"

# ---------- 8) Accept-Ranges ----------
head1 "8) 206 响应带 Accept-Ranges: bytes"
h8="$(headers_only -r 0-1023 --data-urlencode "path=$FILE" "$API/stream")"
grep -qi "^accept-ranges:[[:space:]]*bytes" <<<"$h8" \
  && ok "206 带 Accept-Ranges: bytes" || bad "206 缺少 Accept-Ranges: bytes"

# ---------- 9) 并发多 Range ----------
head1 "9) 并发多轮 Range（播放器会并发开流；这是 2026-10-01 抓到的间歇性失败点）"
# ⚠️ 必须加 -G：curl 带 --data-urlencode 默认是 POST，/stream 只认 GET，
#    POST 会返回 19 字节的 "404 page not found" —— 那是**测试脚本的假阳性**，不是 bug。
# ⚠️ 多轮而非单轮：并发句柄竞争是概率性的（修前实测约 25% 的请求失败），
#    单轮 4 条有可能侥幸全绿，5 轮 20 条才能稳定复现。
OFFS=(0 $(( PSIZE / 4 )) $(( PSIZE / 2 )) $(( PSIZE - 2048 )))
LEN=2048
conc_fail=0
for round in 1 2 3 4 5; do
  pids=()
  for i in 0 1 2 3; do
    off="${OFFS[$i]}"; len=$LEN
    [ $(( off + len )) -gt "$PSIZE" ] && len=$(( PSIZE - off ))
    ( curl -s -m 60 -G -r "$off-$(( off + len - 1 ))" --data-urlencode "path=$FILE" "$API/stream" -o "$TMP/conc-$round-$i.bin" 2>/dev/null ) &
    pids+=($!)
  done
  wait "${pids[@]}" 2>/dev/null
  for i in 0 1 2 3; do
    off="${OFFS[$i]}"; len=$LEN
    [ $(( off + len )) -gt "$PSIZE" ] && len=$(( PSIZE - off ))
    tail -c +$(( off + 1 )) "$PLAIN" | head -c "$len" > "$TMP/conc-plain-$i.bin"
    if cmp -s "$TMP/conc-$round-$i.bin" "$TMP/conc-plain-$i.bin"; then
      :
    else
      bad "第 $round 轮 偏移 $off 不一致（$(stat -c %s "$TMP/conc-$round-$i.bin" 2>/dev/null) 字节：$(head -c 40 "$TMP/conc-$round-$i.bin" 2>/dev/null)）"
      conc_fail=$(( conc_fail + 1 ))
    fi
  done
done
[ "$conc_fail" = "0" ] && ok "5 轮 × 4 路并发 Range（共 20 条）全部字节一致" \
  || bad "并发共 $conc_fail/20 条失败"

echo
echo "═══ 结果：${PASS} PASS / ${FAIL} FAIL ═══"
[ "$FAIL" = "0" ] || exit 1
exit 0
