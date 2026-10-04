#!/usr/bin/env bash
# =============================================================================
# emu-backend-large.sh —— 模拟器内后端「大文件流式分支」的 seek 正确性断言
# -----------------------------------------------------------------------------
# 为什么要单独一个脚本（见 .codebuddy/memory/2026-10-01.md §13）：
#   LocalFileProvider.shouldCacheInMemory() 有两条分支：
#     · 小文件（≤3MB）→ 全量读进内存（cachedData），GetReader/GetSeeker 共用
#       同一个 *cachedReadCloser（2026-10-01 修的就是这里：曾各 new 一个 reader，
#       导致 Seek 不作用在真正被拷贝的 reader 上 → Range 静默失效）
#     · 大文件（>3MB）→ 直接流式走 VirtualSeekableDecryptReader（另一条代码路径！）
#   上一轮只验证了小文件分支；**大文件分支在真机通路上从未被字节级验证过**，
#   而它恰恰是真实视频（几十 MB~GB）走的分支。本脚本专门补这块。
#
# 断言（任一失败 exit 1）：
#   1) 样例明文 > 3MB（确保真的走流式分支，否则测试无意义 → 直接判 FAIL 提示换样例）
#   2) 全量下载与明文逐字节一致
#   3) 多偏移 Range（含非块对齐偏移）206 + 字节一致
#   4) suffix Range（末尾 1MB）字节一致
#   5) 并发 3 路不同 Range 字节一致
#
# 用法：
#   bash scripts/emu-backend-large.sh [选项]
#     --api    <base>      默认 http://127.0.0.1:12025
#     --plain  <明文文件>   默认 /tmp/src/big.mp4
#     --sample <加密容器>   默认 /tmp/src/bigout/big.4pm.sccgv
#     --remote <模拟器目录> 默认 /data/local/tmp/out
# =============================================================================
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADB=/opt/android-sdk/platform-tools/adb
API="${EMU_API:-http://127.0.0.1:12025}"
PLAIN="${BIG_PLAIN:-/tmp/src/big.mp4}"
SAMPLE="${BIG_SAMPLE:-/tmp/src/bigout/big.4pm.sccgv}"
REMOTE_DIR="${BIG_REMOTE:-/data/local/tmp/out}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --api)    API="$2";        shift 2 ;;
    --plain)  PLAIN="$2";      shift 2 ;;
    --sample) SAMPLE="$2";     shift 2 ;;
    --remote) REMOTE_DIR="$2"; shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

TMP="$(mktemp -d /tmp/emu-backend-large.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()   { printf '  ✅ %s\n' "$*"; PASS=$((PASS + 1)); }
bad()  { printf '  ❌ %s\n' "$*"; FAIL=$((FAIL + 1)); }
head1() { printf '\n── %s ──\n' "$*"; }

# ---------- 0) 样例（缺则现造：ffmpeg 噪声视频 + encrypt-v2） ----------
if [ ! -f "$PLAIN" ] || [ "$(stat -c %s "$PLAIN")" -le $(( 3 * 1024 * 1024 )) ]; then
  echo "ℹ️  缺少 >3MB 明文样例（$PLAIN）—— 现造…"
  command -v ffmpeg >/dev/null 2>&1 || { echo "❌ 缺 ffmpeg，无法生成样例"; exit 1; }
  mkdir -p "$(dirname "$PLAIN")" "$(dirname "$SAMPLE")"
  ffmpeg -y -loglevel error \
    -f lavfi -i "color=c=gray:s=640x480:d=20" \
    -f lavfi -i sine=frequency=440:duration=20 \
    -vf "noise=alls=100:allf=t+u" \
    -c:v libx264 -crf 18 -preset veryfast -pix_fmt yuv420p -c:a aac \
    -movflags +frag_keyframe+empty_moov+default_base_moof -shortest "$PLAIN" \
    || { echo "❌ ffmpeg 生成大样例失败"; exit 1; }
  ( cd "$ROOT" && go run ./cmd/encv encrypt-v2 "$PLAIN" -p my-encv_key -o "$(dirname "$SAMPLE")" ) \
    || { echo "❌ encrypt-v2 生成大容器失败"; exit 1; }
fi

PSIZE="$(stat -c %s "$PLAIN")"
echo "═══ 模拟器内后端 · 大文件流式分支检查 ═══"
echo "  API  : $API"
echo "  明文 : $PLAIN（$PSIZE 字节）"
echo "  容器 : $SAMPLE"

head1 "1) 样例必须 > 3MB（否则走的是内存缓存分支，本脚本无意义）"
if [ "$PSIZE" -gt $(( 3 * 1024 * 1024 )) ]; then
  ok "明文 $(( PSIZE / 1024 / 1024 ))MB > 3MB → 走流式分支"
else
  bad "明文仅 $PSIZE 字节（≤3MB）→ 会走内存缓存分支，本脚本测不到流式分支（换更大的样例）"
  exit 1
fi

c="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "$API/ping")"
[ "$c" = "200" ] || { echo "❌ 后端未就绪（/ping → $c）"; exit 1; }

# ---------- 推样例进模拟器 + 算虚拟路径 ----------
$ADB devices | grep -q emulator || { echo "❌ 模拟器未启动"; exit 1; }
$ADB shell mkdir -p "$REMOTE_DIR" >/dev/null 2>&1
$ADB push "$SAMPLE" "$REMOTE_DIR/$(basename "$SAMPLE")" >/dev/null 2>&1 \
  || { echo "❌ push 大容器失败"; exit 1; }
REMOTE="$REMOTE_DIR/$(basename "$SAMPLE")"

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
if [ -n "$VPATH" ]; then VFILE="/d/primary${REMOTE#"$VPATH"}"; else VFILE="/d/primary/out/$(basename "$SAMPLE")"; fi
echo "  虚拟路径: $VFILE（primary root=${VPATH:-unknown}）"

# ---------- 2) 全量 ----------
head1 "2) 全量解密（逐字节）"
# ⚠️ -G 不能省：curl 带 --data-urlencode 默认是 POST，/stream 只认 GET，
#    POST 会拿到 19 字节的 "404 page not found" —— 那是脚本的假阳性。
curl -s -m 300 -G --data-urlencode "path=$VFILE" "$API/stream" -o "$TMP/full.bin"
FSIZE="$(stat -c %s "$TMP/full.bin" 2>/dev/null || echo 0)"
if [ "$FSIZE" = "$PSIZE" ]; then
  ok "全量长度一致（$FSIZE）"
  cmp -s "$TMP/full.bin" "$PLAIN" && ok "全量逐字节一致" || bad "全量字节不一致"
else
  bad "全量长度 $FSIZE ≠ 明文 $PSIZE"
fi

# ---------- 3) 多偏移 seek（含非块对齐） ----------
head1 "3) 多偏移 Range seek（流式分支 · 逐字节）"
OFFS=(0 1 4095 65536 1048577 $(( PSIZE / 2 )) $(( PSIZE - 8193 )))
for off in "${OFFS[@]}"; do
  len=8192
  [ $(( off + len )) -gt "$PSIZE" ] && len=$(( PSIZE - off ))
  [ "$len" -le 0 ] && continue
  code="$(curl -s -m 120 -G -o "$TMP/seg.bin" -w '%{http_code}' -r "$off-$(( off + len - 1 ))" --data-urlencode "path=$VFILE" "$API/stream")"
  tail -c +$(( off + 1 )) "$PLAIN" | head -c "$len" > "$TMP/seg-plain.bin"
  if [ "$code" = "206" ] && cmp -s "$TMP/seg.bin" "$TMP/seg-plain.bin"; then
    ok "偏移 $off（$len 字节）一致"
  else
    bad "偏移 $off 不一致（HTTP $code，$(stat -c %s "$TMP/seg.bin" 2>/dev/null) 字节）"
  fi
done

# ---------- 4) suffix Range ----------
head1 "4) suffix Range（末尾 1MB）"
SUF=1048576
[ "$PSIZE" -lt "$SUF" ] && SUF="$PSIZE"
code="$(curl -s -m 300 -G -o "$TMP/suf.bin" -w '%{http_code}' -H "Range: bytes=-$SUF" --data-urlencode "path=$VFILE" "$API/stream")"
tail -c +$(( PSIZE - SUF + 1 )) "$PLAIN" | head -c "$SUF" > "$TMP/suf-plain.bin"
if [ "$code" = "206" ] && cmp -s "$TMP/suf.bin" "$TMP/suf-plain.bin"; then
  ok "末尾 ${SUF} 字节一致"
else
  bad "末尾 ${SUF} 字节不一致（HTTP $code）"
fi

# ---------- 5) 并发（多轮！） ----------
# ⚠️ 必须多轮：这类"共享句柄引用计数被多减一次"的 bug 是**概率性**的
#    （2026-10-02 修前实测：8 轮 × 3 路里有 3 条被截断，单轮很可能侥幸全绿）。
head1 "5) 并发 3 路 Range × 3 轮"
conc_fail=0
for round in 1 2 3; do
  pids=()
  for i in 0 1 2; do
    case $i in
      0) off=0;                     len=1048576 ;;
      1) off=$(( PSIZE / 3 ));      len=1048576 ;;
      2) off=$(( PSIZE - 524288 )); len=524288 ;;
    esac
    ( curl -s -m 300 -G -r "$off-$(( off + len - 1 ))" --data-urlencode "path=$VFILE" "$API/stream" -o "$TMP/c-$round-$i.bin"
      tail -c +$(( off + 1 )) "$PLAIN" | head -c "$len" > "$TMP/c-plain-$round-$i.bin" ) &
    pids+=($!)
  done
  wait "${pids[@]}" 2>/dev/null
  for i in 0 1 2; do
    if cmp -s "$TMP/c-$round-$i.bin" "$TMP/c-plain-$round-$i.bin"; then
      :
    else
      bad "第 $round 轮 并发 #$i 字节不一致（$(stat -c %s "$TMP/c-$round-$i.bin" 2>/dev/null) 字节 —— 实体体被截断？）"
      conc_fail=$(( conc_fail + 1 ))
    fi
  done
done
[ "$conc_fail" = "0" ] && ok "3 轮 × 3 路并发（共 9 条 1MB 级 Range）全部字节一致" \
  || bad "并发共 $conc_fail/9 条失败"

echo
echo "═══ 结果：${PASS} PASS / ${FAIL} FAIL ═══"
[ "$FAIL" = "0" ] || exit 1
exit 0
