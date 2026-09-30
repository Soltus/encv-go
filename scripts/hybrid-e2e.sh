#!/usr/bin/env bash
# =============================================================================
# hybrid-e2e.sh —— 「加密视频预览」端到端回归（混合通路，一键 / 自包含 / 可重复）
# =============================================================================
# 背景（.codebuddy/memory/2026-10-01.md §8-11）：
#   无 KVM 的模拟器上，APK 内 WebView（Chromium 133）初始化即崩（crashpad +
#   SIGTRAP pc=0），属模拟器环境限制；但 Go 后端（x86_64 原生）完全正常。
#   所以真机级验证拆成两半：
#     · 后端跑在模拟器里（Android 文件/权限/mount 语义 = 真机）
#     · 前端跑在宿主机真实 Chromium（替代 WebView）
#     · adb forward 打通
# 三阶段断言：
#   ① 后端契约（scripts/emu-backend-check.sh）：/ping、mount 语义、目录列举、
#      全量解密逐字节、Range 首段、中间偏移 seek、失败路径不静默 200
#   ② 前端播放（scripts/hybrid-play.ts）：/stream 206 + artplayer playing + 无错误卡片
#   ③ 证据落盘：截图 + logcat 片段
#
# 用法：
#   bash scripts/hybrid-e2e.sh [选项]
#     --only-backend   只跑 ①（快，不需要 dist/浏览器）
#     --keep           结束后保留后端与端口转发（便于手工排查）
#     --no-build       缺 dist 时不自动构建（直接失败并提示）
#     --sample <path>  指定加密样例容器（默认 /tmp/src/out/sample.4pm.sccgv）
# =============================================================================
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADB=/opt/android-sdk/platform-tools/adb
API_PORT=12025
SAMPLE="${HYBRID_SAMPLE:-/tmp/src/out/sample.4pm.sccgv}"
PLAIN="${HYBRID_PLAIN:-/tmp/src/sample.mp4}"
OUT_DIR=/tmp/emutest
KEEP=0
ONLY_BACKEND=0
NO_BUILD=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --only-backend) ONLY_BACKEND=1; shift ;;
    --keep)         KEEP=1;         shift ;;
    --no-build)     NO_BUILD=1;     shift ;;
    --sample)       SAMPLE="$2";    shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

mkdir -p "$OUT_DIR"
fail() { echo "❌ $*" >&2; exit 1; }
info() { echo "ℹ️  $*"; }
ok()   { echo "✅ $*"; }

SERVE_PID=""
cleanup() {
  if [ "$KEEP" = "1" ]; then info "--keep：保留后端/转发/前端伺服"; return; fi
  [ -n "$SERVE_PID" ] && kill "$SERVE_PID" 2>/dev/null
  # ⚠️ adb shell pkill 会匹配会话自身导致挂起 → 必须 timeout
  timeout 20 $ADB shell "pkill -f 'encv-x64 start'" >/dev/null 2>&1
  $ADB forward --remove tcp:$API_PORT >/dev/null 2>&1
  rm -f /tmp/hybrid-serve.ts
}
trap cleanup EXIT

# ---------------- 0) 样例容器（缺则现造，保证换会话也能一键跑） ----------------
if [ ! -f "$SAMPLE" ]; then
  info "缺少加密样例 $SAMPLE —— 现造（ffmpeg + encv encrypt-v2）…"
  command -v ffmpeg >/dev/null || fail "缺 ffmpeg，无法生成样例（也可手工放一个 fMP4 加密容器并用 --sample 指定）"
  mkdir -p "$(dirname "$PLAIN")" "$(dirname "$SAMPLE")"
  ffmpeg -y -loglevel error \
    -f lavfi -i testsrc=size=320x240:rate=25:duration=3 \
    -f lavfi -i sine=frequency=440:duration=3 \
    -c:v libx264 -pix_fmt yuv420p -c:a aac \
    -movflags +frag_keyframe+empty_moov+default_base_moof -shortest "$PLAIN" \
    || fail "ffmpeg 生成样例视频失败"
  ( cd "$ROOT" && go run ./cmd/encv encrypt-v2 "$PLAIN" -p my-encv_key -o "$(dirname "$SAMPLE")" ) \
    || fail "encrypt-v2 生成加密容器失败"
fi
ok "加密样例: $SAMPLE（明文 $PLAIN，$(stat -c %s "$PLAIN") 字节）"

if [ "$ONLY_BACKEND" = "0" ]; then
  if [ ! -f "$ROOT/app/encv-mobile/dist/index.html" ]; then
    [ "$NO_BUILD" = "1" ] && fail "缺少前端 dist（先 pnpm --dir app/encv-mobile build）"
    info "缺少前端 dist —— 现构建（pnpm build，约数分钟）…"
    ( cd "$ROOT/app/encv-mobile" && timeout 1500 pnpm build ) || fail "前端构建失败"
  fi
  ok "前端 dist 就绪"
fi

# ---------------- 1) 模拟器 + x86_64 Go 后端 ----------------
$ADB devices | grep -q emulator || fail "模拟器未启动（先 emuctl start）"
BIN=/tmp/encv-x64
if [ ! -x "$BIN" ]; then
  info "编译 x86_64 Go 后端（android-common.sh build-emu-backend）…"
  bash "$ROOT/scripts/android-common.sh" build-emu-backend "$BIN" || fail "Go 交叉编译失败"
fi
ok "后端二进制: $BIN"

python3 - <<'PY'
import json
d = json.load(open('/workspace/config.user.json'))
d.setdefault('mobile', {}).setdefault('server', {})['dir'] = '/data/local/tmp/out'
d['log'] = {'file': '', 'level': 'info'}
json.dump(d, open('/tmp/encvrepro.json', 'w'))
PY
$ADB push "$BIN" /data/local/tmp/encv-x64 >/dev/null
$ADB push /tmp/encvrepro.json /data/local/tmp/encvrepro.json >/dev/null
$ADB shell mkdir -p /data/local/tmp/out
$ADB push "$SAMPLE" /data/local/tmp/out/"$(basename "$SAMPLE")" >/dev/null
$ADB shell chmod 755 /data/local/tmp/encv-x64
ok "配置/样例/后端已 push 到模拟器"

# 起后端（清掉旧的）+ 端口转发
# ⚠️ 两条 adb shell 都必须加 timeout + nohup/重定向：
#    - pkill 的模式会匹配会话自身导致挂起
#    - 后台进程持有 stdout 时 adb shell 不返回
timeout 30 $ADB shell "pkill -f 'encv-x64 start'" >/dev/null 2>&1
sleep 2
# ⚠️ 后端端口是**自选**的（2025 起递增，2025..2034）：旧实例没退干净或自检失败时
#    它会跳到下一个端口（实测日志：addr=[::]:2025 → Self-check failed → 2026）。
#    真机上的同款疑点：前端 getApiBaseUrl() 用常量 http://127.0.0.1:2025，
#    端口一漂就永远连不上（只有 useApiBaseProbe 成功才写回 localStorage）。
#    ⇒ 这里**从日志里读真实端口**再转发，不假设 2025。
timeout 30 $ADB shell "cd /data/local/tmp && nohup env ENCV_CONFIG_PATH=/data/local/tmp/encvrepro.json /data/local/tmp/encv-x64 start > /data/local/tmp/backend.log 2>&1 &"
$ADB forward --remove tcp:$API_PORT >/dev/null 2>&1
API="http://127.0.0.1:$API_PORT"
code=""
BACKEND_PORT=""
for _ in $(seq 1 25); do
  sleep 2
  # 把日志拉到宿主再解析（模拟器里的 toybox grep 不支持 -o，别在 adb shell 里做正则）
  timeout 20 $ADB shell cat /data/local/tmp/backend.log > "$OUT_DIR/backend.log" 2>/dev/null
  # 日志里 addr= 后面夹了 ANSI 色码（\e[0m），不能按 addr=\[::\] 匹配，直接截 [::]: 之后
  BACKEND_PORT="$(grep 'successfully started' "$OUT_DIR/backend.log" 2>/dev/null \
    | sed 's/.*\[::\]://; s/[^0-9].*$//' | grep -E '^[0-9]+$' | tail -1)"
  if [ -n "$BACKEND_PORT" ]; then
    $ADB forward tcp:$API_PORT tcp:$BACKEND_PORT >/dev/null 2>&1
    code=$(curl -s -m 5 -o /dev/null -w '%{http_code}' "$API/ping")
    [ "$code" = "200" ] && break
  fi
done
[ "$code" = "200" ] || fail "模拟器内后端未就绪（看 adb shell cat /data/local/tmp/backend.log）"
[ "$BACKEND_PORT" = "2025" ] || info "⚠️  后端自选端口是 $BACKEND_PORT（不是 2025）—— 真机上 getApiBaseUrl() 的常量 2025 会同款连不上"
ok "后端就绪（模拟器内 :$BACKEND_PORT → 宿主 :$API_PORT）"

# ---------------- 2) 容器虚拟路径：按 mount root 现算（不再写死） ----------------
# ⚠️ 坑（memory §10）：primary mount 的 root 取自 mounts.json，**不是** config 的
#     mobile.server.dir。写死 /d/primary/out/... 会在换环境后静默失效。
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
REMOTE="/data/local/tmp/out/$(basename "$SAMPLE")"
if [ -n "$VPATH" ]; then
  VFILE="/d/primary${REMOTE#"$VPATH"}"
else
  VFILE="/d/primary/out/$(basename "$SAMPLE")"
fi
ok "容器虚拟路径: $VFILE（primary root=${VPATH:-unknown}）"

# ---------------- 3) 阶段①：后端契约 ----------------
info "阶段①：模拟器内后端契约断言"
bash "$ROOT/scripts/emu-backend-check.sh" --api "$API" --path "$VFILE" --plain "$PLAIN" || fail "后端契约断言失败"

[ "$ONLY_BACKEND" = "1" ] && { echo; echo "✅ --only-backend：后端契约全通过"; exit 0; }

# ---------------- 4) 阶段②：宿主机 Chromium 跑前端 ----------------
# ⚠️ 必须是「薄反代」而不是纯静态伺服：真机上 WebView 的页面就是**由 Go 后端本身**提供的，
#    所以前端用的是相对路径 /stream、/api —— 打到自己 origin 上。这里复刻同样的拓扑：
#    /stream|/decrypt|/api|/preview* → 模拟器内后端，其余 → 生产 dist 静态文件。
#    （纯静态伺服会把 /stream 兜底成 index.html，播放器拿到 HTML 当视频解 → 必然失败）
cat > /tmp/hybrid-serve.ts <<'TS'
const API = process.env.HYBRID_API ?? "http://127.0.0.1:12025";
const DIST = "/workspace/app/encv-mobile/dist";
const PROXY_PREFIXES = ["/stream", "/decrypt", "/api", "/preview", "/themes", "/ping"];
Bun.serve({
  port: 18081,
  async fetch(req) {
    const url = new URL(req.url);
    if (PROXY_PREFIXES.some((p) => url.pathname === p || url.pathname.startsWith(p + "/"))) {
      const target = API + url.pathname + url.search;
      const headers = new Headers(req.headers);
      headers.set("host", new URL(API).host);
      try {
        return await fetch(target, {
          method: req.method,
          headers,
          body: req.method === "GET" || req.method === "HEAD" ? undefined : req.body,
        });
      } catch (e) {
        return new Response("proxy error: " + e, { status: 502 });
      }
    }
    let f = Bun.file(DIST + url.pathname);
    if (!(await f.exists())) f = Bun.file(DIST + "/index.html");
    return new Response(f);
  },
});
TS
export HYBRID_API="$API"
setsid bash -c 'bun /tmp/hybrid-serve.ts > /tmp/emutest/hybrid-serve.log 2>&1' </dev/null >/dev/null 2>&1 &
sleep 2
SERVE_PID="$(pgrep -f 'bun /tmp/hybrid-serve.ts' | head -1)"
code=""
for _ in $(seq 1 15); do
  sleep 1
  code=$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/)
  [ "$code" = "200" ] && break
done
[ "$code" = "200" ] || fail "前端静态伺服未就绪（看 /tmp/emutest/hybrid-serve.log）"
ok "前端就绪（:18081，生产 dist）"

info "阶段②：浏览器端到端断言"
(
  cd "$ROOT/app/encv-mobile" && \
  HYBRID_API="$API" HYBRID_BASE="http://127.0.0.1:18081" HYBRID_PATH="$VFILE" \
  HYBRID_SHOT="$OUT_DIR/hybrid-player.png" \
  bun "$ROOT/scripts/hybrid-play.ts"
) || fail "端到端验证失败（截图 $OUT_DIR/hybrid-player.png）"

echo
echo "✅ hybrid-e2e 全通过：模拟器内真实后端 + 真实浏览器播放加密视频"
