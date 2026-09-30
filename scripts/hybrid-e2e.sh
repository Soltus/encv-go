#!/usr/bin/env bash
# =============================================================================
# hybrid-e2e.sh —— 「加密视频预览」端到端回归（混合通路，一键）
# =============================================================================
# 背景（.codebuddy/memory/2026-10-01.md §8-10）：
#   无 KVM 的模拟器上，APK 内 WebView（Chromium 133）初始化即崩（crashpad +
#   SIGTRAP pc=0），属模拟器环境限制；但 Go 后端（x86_64 原生）完全正常。
#   所以真机级验证拆成两半：
#     · 后端跑在模拟器里（Android 文件/权限/mount 语义 = 真机）
#     · 前端跑在宿主机真实 Chromium（替代 WebView）
#     · adb forward 打通
# 前置：
#   1) 模拟器已启动（emuctl start）
#   2) x86_64 Go 后端二进制（无则自动编译，需 NDK）
#   3) 加密样例容器 /tmp/src/out/sample.4pm.sccgv（生成方法见
#      app/encv-preview/README.md「样例容器」；视频必须 fMP4）
#   4) app/encv-mobile/dist 已构建（pnpm build）
# 用法：
#   bash scripts/hybrid-e2e.sh
# =============================================================================
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADB=/opt/android-sdk/platform-tools/adb
NDK=/opt/android-sdk/ndk/28.1.13356709
CC="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android24-clang"
SAMPLE="${HYBRID_SAMPLE:-/tmp/src/out/sample.4pm.sccgv}"
API_PORT=12025

fail() { echo "❌ $*" >&2; exit 1; }
info() { echo "ℹ️  $*"; }
ok()   { echo "✅ $*"; }

# 0) 前置
$ADB devices | grep -q emulator || fail "模拟器未启动（先 emuctl start）"
[ -f "$SAMPLE" ] || fail "缺少加密样例 $SAMPLE（见 app/encv-preview/README.md 生成）"
[ -f "$ROOT/app/encv-mobile/dist/index.html" ] || fail "缺少前端 dist（先 pnpm build）"

# 1) x86_64 Go 后端（有缓存，秒级）
BIN=/tmp/encv-x64
if [ ! -x "$BIN" ]; then
  info "编译 x86_64 Go 后端（android-common.sh build-emu-backend）…"
  bash "$ROOT/scripts/android-common.sh" build-emu-backend "$BIN" || fail "Go 交叉编译失败"
fi
ok "后端二进制: $BIN"

# 2) push 配置 + 样例 + 后端
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
$ADB push "$SAMPLE" /data/local/tmp/out/sample.4pm.sccgv >/dev/null
$ADB shell chmod 755 /data/local/tmp/encv-x64
ok "配置/样例/后端已 push 到模拟器"

# 3) 起后端（清掉旧的）+ 端口转发
# ⚠️ 两条 adb shell 都必须加 timeout + nohup/重定向：
#    - pkill 的模式会匹配会话自身导致挂起
#    - 后台进程持有 stdout 时 adb shell 不返回
timeout 30 $ADB shell "pkill -f 'encv-x64 start'" >/dev/null 2>&1
sleep 2
timeout 30 $ADB shell "cd /data/local/tmp && nohup env ENCV_CONFIG_PATH=/data/local/tmp/encvrepro.json /data/local/tmp/encv-x64 start > /data/local/tmp/backend.log 2>&1 &"
$ADB forward --remove tcp:$API_PORT >/dev/null 2>&1
$ADB forward tcp:$API_PORT tcp:2025
for i in $(seq 1 15); do
  sleep 2
  code=$(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:$API_PORT/ping")
  [ "$code" = "200" ] && break
done
[ "$code" = "200" ] || fail "模拟器内后端未就绪（看 adb shell cat /data/local/tmp/backend.log）"
ok "后端就绪（经 adb forward :$API_PORT）"

# 4) 起前端静态伺服（生产 dist）
pkill -f 'prod-serve.ts' >/dev/null 2>&1
cat > /tmp/hybrid-serve.ts <<'TS'
Bun.serve({
  port: 18081,
  async fetch(req) {
    const url = new URL(req.url);
    let f = Bun.file("/workspace/app/encv-mobile/dist" + url.pathname);
    if (!(await f.exists())) f = Bun.file("/workspace/app/encv-mobile/dist/index.html");
    return new Response(f);
  },
});
TS
setsid bash -c 'bun /tmp/hybrid-serve.ts > /tmp/emutest/hybrid-serve.log 2>&1' </dev/null >/dev/null 2>&1 &
for i in $(seq 1 10); do
  sleep 1
  code=$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/)
  [ "$code" = "200" ] && break
done
[ "$code" = "200" ] || fail "前端静态伺服未就绪"
ok "前端就绪（:18081，生产 dist）"

# 5) 端到端断言（playwright 驱动真实 Chromium）
info "跑端到端断言…"
( cd "$ROOT/app/encv-mobile" && bun "$ROOT/scripts/hybrid-play.ts" )
rc=$?
[ "$rc" = "0" ] && ok "端到端验证通过" || fail "端到端验证失败（截图 /tmp/hybrid-player.png）"
