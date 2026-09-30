#!/usr/bin/env bash
# =============================================================================
# emu-backend-check.sh —— 模拟器内真实后端的 HTTP 契约断言（真机语义回归）
# -----------------------------------------------------------------------------
# 为什么单独成脚本（见 .codebuddy/memory/2026-10-01.md §10）：
#   后端跑在模拟器里 ⇒ 走的是 Android 的文件系统/权限/mount 语义，与真机等价；
#   宿主机只做 HTTP 断言（adb forward 打通）。这条通路不依赖 WebView，
#   因此在「模拟器 WebView 会崩」的环境限制下依然稳定可用，是最有价值的真机级闸门。
#
# 断言（任一失败 exit 1）：
#   1) /ping                        → 200
#   2) /api/mounts                  → 200 且含 primary（mount 语义生效）
#   3) /api/files?path=<父目录>      → 200 且含目标容器（Android 目录列举生效）
#   4) /stream 全量                  → 200，字节与明文逐字节相同（解密完整性）
#   5) /stream Range 0-1023          → 206 + Content-Range 总长=明文长度 + 字节一致
#   6) /stream Range 中间偏移（seek） → 206 + 字节一致（随机读/拖动正确性）
#   7) /stream 不存在的路径           → 非 200/206（失败必须显式失败，不能静默 200）
#
# 前置：模拟器内后端已起 + adb forward（一般由 scripts/hybrid-e2e.sh 编排）
# 用法：
#   bash scripts/emu-backend-check.sh [选项]
#     --api   <base>      默认 http://127.0.0.1:12025
#     --path  <虚拟路径>   默认 /d/primary/out/sample.4pm.sccgv
#     --plain <明文文件>   默认 /tmp/src/sample.mp4（用于逐字节比对）
#     --no-bytes          跳过 4/5/6 的字节比对（只在 HTTP 层断言）
# =============================================================================
set -uo pipefail

API="${EMU_API:-http://127.0.0.1:12025}"
FILE="${EMU_PATH:-/d/primary/out/sample.4pm.sccgv}"
PLAIN="${EMU_PLAIN:-/tmp/src/sample.mp4}"
BYTES=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --api)   API="$2";   shift 2 ;;
    --path)  FILE="$2";  shift 2 ;;
    --plain) PLAIN="$2"; shift 2 ;;
    --no-bytes) BYTES=0; shift ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

TMP="$(mktemp -d /tmp/emu-backend-check.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()   { printf '  ✅ %s\n' "$*"; PASS=$((PASS + 1)); }
bad()  { printf '  ❌ %s\n' "$*"; FAIL=$((FAIL + 1)); }
head1() { printf '\n── %s ──\n' "$*"; }

# 统一带超时、带 URL 编码地请求（path 参数交给 curl 编码，避免手写 %2F 出错）
req() { # req <extra curl args...>
  curl -s -m 60 -G "$@" 2>/dev/null
}
code_of() { # code_of <extra curl args...>
  curl -s -m 60 -o /dev/null -w '%{http_code}' -G "$@" 2>/dev/null
}

PARENT="$(dirname "$FILE")"
NAME="$(basename "$FILE")"

echo "═══ 模拟器内后端契约检查 ═══"
echo "  API  : $API"
echo "  容器 : $FILE"
echo "  明文 : $PLAIN"

# ---------- 1) 存活 ----------
head1 "1) /ping"
c="$(code_of "$API/ping")"
[ "$c" = "200" ] && ok "/ping → 200" || bad "/ping → $c（期望 200）"
if [ "$c" != "200" ]; then
  echo; echo "❌ 后端未就绪，后续断言无意义，提前退出"; exit 1
fi

# ---------- 2) mount 语义 ----------
head1 "2) /api/mounts（Android mount 语义）"
mounts="$(req "$API/api/mounts")"
if [ -n "$mounts" ] && grep -qi 'primary' <<<"$mounts"; then
  ok "/api/mounts 含 primary"
else
  bad "/api/mounts 未返回 primary（${mounts:0:120}）"
fi

# ---------- 3) 目录列举 ----------
head1 "3) /api/files（Android 目录列举）"
listing="$(req --data-urlencode "path=$PARENT" "$API/api/files")"
if grep -q "$NAME" <<<"$listing"; then
  ok "/api/files?path=$PARENT 含 $NAME"
else
  bad "/api/files?path=$PARENT 未列出 $NAME（${listing:0:160}）"
fi

# ---------- 4~6) 解密流（HTTP + 字节） ----------
if [ "$BYTES" = "1" ]; then
  if [ ! -f "$PLAIN" ]; then
    bad "缺少明文文件 $PLAIN（无法做字节比对；加 --no-bytes 只验 HTTP）"
  else
    PSIZE="$(stat -c %s "$PLAIN")"
    echo
    echo "  明文长度: $PSIZE"

    head1 "4) /stream 全量解密（逐字节）"
    req --data-urlencode "path=$FILE" "$API/stream" -o "$TMP/full.bin"
    FSIZE="$(stat -c %s "$TMP/full.bin" 2>/dev/null || echo 0)"
    if [ "$FSIZE" = "$PSIZE" ]; then
      ok "全量长度一致（$FSIZE）"
      if cmp -s "$TMP/full.bin" "$PLAIN"; then ok "全量逐字节一致"; else bad "全量字节不一致"; fi
    else
      bad "全量长度 $FSIZE ≠ 明文 $PSIZE"
    fi

    head1 "5) /stream Range 0-1023（边下边播首段）"
    cr="$(req -D - -r 0-1023 --data-urlencode "path=$FILE" "$API/stream" -o "$TMP/head.bin" | tr -d '\r' | grep -i '^content-range:' | head -1)"
    c5="$(code_of -r 0-1023 --data-urlencode "path=$FILE" "$API/stream")"
    [ "$c5" = "206" ] && ok "Range → 206" || bad "Range → $c5（期望 206）"
    if grep -qi "bytes 0-1023/$PSIZE" <<<"$cr"; then
      ok "Content-Range 总长为明文长度：$cr"
    else
      bad "Content-Range 不含 /$PSIZE：$cr"
    fi
    dd if="$PLAIN" of="$TMP/plain-head.bin" bs=1024 count=1 status=none
    if cmp -s "$TMP/head.bin" "$TMP/plain-head.bin"; then ok "首段字节一致"; else bad "首段字节不一致"; fi

    head1 "6) /stream Range 中间偏移（seek / 拖动）"
    OFF=$(( PSIZE / 2 )); LEN=4096
    [ $(( OFF + LEN )) -gt "$PSIZE" ] && LEN=$(( PSIZE - OFF ))
    if [ "$LEN" -le 0 ]; then
      bad "明文太短，无法做 seek 断言"
    else
      c6="$(code_of -r "$OFF-$(( OFF + LEN - 1 ))" --data-urlencode "path=$FILE" "$API/stream")"
      [ "$c6" = "206" ] && ok "seek Range → 206" || bad "seek Range → $c6（期望 206）"
      req -r "$OFF-$(( OFF + LEN - 1 ))" --data-urlencode "path=$FILE" "$API/stream" -o "$TMP/mid.bin"
      dd if="$PLAIN" of="$TMP/plain-mid.bin" bs=1 skip="$OFF" count="$LEN" status=none
      if cmp -s "$TMP/mid.bin" "$TMP/plain-mid.bin"; then
        ok "偏移 $OFF 起 $LEN 字节与明文一致"
      else
        bad "偏移 $OFF 起 $LEN 字节与明文不一致（解密偏移错位？）"
      fi
    fi
  fi
else
  echo; echo "ℹ️  --no-bytes：跳过 4/5/6 的字节比对"
fi

# ---------- 7) 失败路径必须显式失败 ----------
head1 "7) 不存在的路径（不能静默 200）"
c7="$(code_of --data-urlencode "path=$PARENT/__nope__.sccgv" "$API/stream")"
case "$c7" in
  200|206) bad "不存在的路径返回 $c7（期望 4xx/5xx）" ;;
  *)       ok "不存在的路径返回 $c7" ;;
esac

echo
echo "═══ 结果：${PASS} PASS / ${FAIL} FAIL ═══"
[ "$FAIL" = "0" ] || exit 1
exit 0
