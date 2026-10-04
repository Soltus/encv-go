#!/usr/bin/env bash
# =============================================================================
# emu-smoke.sh —— APK 真机冒烟（装 → AOT → 冷启动 → 存活/崩溃判定 → 截图）
# -----------------------------------------------------------------------------
# 这是「模拟器里能不能真跑起来主应用」的唯一自动化闸门。
# 2026-10-01 实测（见 .codebuddy/memory/2026-10-01.md §8-11）：
#   - 无 KVM 的模拟器上，装完不做 AOT 必被 AM 判超时杀掉（已由 emuctl install 自动 AOT 解决）
#   - 过了 ANR 关之后仍会在 WebView/Chromium 初始化处崩：
#     logcat 出现 `F/crashpad ... CRASHPAD MINIDUMP` + `Fatal signal 5 (SIGTRAP),
#     SI_KERNEL`、pc=0、无 tombstone（ART 主动 abort）→ 属**模拟器环境限制**，
#     不是 app 代码问题（Go 后端单独在模拟器里跑完全正常，已二分证明）。
#   ⇒ 因此本脚本当前会 FAIL，并明确打印「已知环境限制」。
#     这正是它的价值：镜像/环境一旦改善（有 KVM、换 WebView、换系统镜像），
#     它会自己转绿，而不是靠人肉判断。
#
# 退出码：0=应用存活且无崩溃/ANR；1=未通过（含崩溃/ANR/未存活）；2=用法或前置错误
#
# 用法：
#   bash scripts/emu-smoke.sh [--apk <path>] [--pkg <包名>] [--wait <秒>] [--build]
#     --build    APK 不存在时先构建（EMU_X86_64=1，x86_64 原生，不进 Berberis）
# =============================================================================
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADB=/opt/android-sdk/platform-tools/adb

PKG="${EMU_PKG:-com.encvgo.app}"
WAIT="${EMU_SMOKE_WAIT:-180}"
BUILD=0
APK=""
OUT_DIR="${EMU_OUT_DIR:-/tmp/emutest}"
SHOT="$OUT_DIR/smoke-$(date +%H%M%S).png"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apk)   APK="$2";   shift 2 ;;
    --pkg)   PKG="$2";   shift 2 ;;
    --wait)  WAIT="$2";  shift 2 ;;
    --build) BUILD=1;    shift ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

mkdir -p "$OUT_DIR"
info() { printf 'ℹ️  %s\n' "$*"; }
ok()   { printf '✅ %s\n' "$*"; }
bad()  { printf '❌ %s\n' "$*"; }

# 1) 定位 APK
if [[ -z "$APK" ]]; then
  APK="$(ls -t "$ROOT"/app/encv-mobile/android/app/build/outputs/apk/*/*.apk 2>/dev/null | head -1)"
fi
if [[ -z "$APK" || ! -f "$APK" ]]; then
  if [ "$BUILD" = "1" ]; then
    info "构建 x86_64 模拟器 APK（EMU_X86_64=1）…"
    ( cd "$ROOT" && EMU_X86_64=1 bash scripts/build-android.sh ) || { bad "APK 构建失败"; exit 2; }
    APK="$(ls -t "$ROOT"/app/encv-mobile/android/app/build/outputs/apk/*/*.apk 2>/dev/null | head -1)"
  fi
fi
[[ -n "$APK" && -f "$APK" ]] || { bad "找不到 APK（用 --apk 指定，或加 --build 现编）"; exit 2; }
ok "APK: $APK（$(du -h "$APK" | cut -f1)）"

# 2) 模拟器就绪
$ADB devices | grep -q 'emulator' || { bad "模拟器未启动（先 emuctl start）"; exit 2; }

# 3) 安装（emuctl install 内含 AOT，慢环境下必须）
info "安装 + AOT 编译…"
emuctl install "$APK" || { bad "安装失败"; exit 1; }

# 4) 冷启动（清日志后启动，保证 logcat 只含本次）
$ADB logcat -c >/dev/null 2>&1
pkg_dump="$($ADB shell pm dump "$PKG" 2>/dev/null | grep -m1 'primaryCpuAbi' | tr -d '\r')"
info "ABI: ${pkg_dump:-unknown}"
info "冷启动 $PKG …"
timeout 120 $ADB shell monkey -p "$PKG" -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1

# 5) 采样存活（慢环境下 ART/WebView 初始化很慢，给足时间）
alive=0
for i in $(seq 1 $(( WAIT / 5 ))); do
  if [ -n "$($ADB shell pidof "$PKG" 2>/dev/null | tr -d '\r\n')" ]; then alive=1; break; fi
  sleep 5
done
[ "$alive" = "1" ] && ok "应用进程存活" || bad "应用在 ${WAIT}s 内未存活（或已退出）"

# 6) 是否真的到前台
resumed="$($ADB shell dumpsys activity activities 2>/dev/null | tr -d '\r' | grep -m1 'mResumedActivity' )"
if grep -q "$PKG" <<<"$resumed"; then ok "已到前台: ${resumed##* }"; else bad "未到前台（${resumed:-无 mResumedActivity}）"; fi

# 7) 崩溃/ANR 取证
log="$OUT_DIR/smoke-logcat.txt"
timeout 60 $ADB logcat -d > "$log" 2>&1
crash="$(grep -Ei 'SIGTRAP|Fatal signal|crashpad|CRASHPAD MINIDUMP|FATAL EXCEPTION' "$log" | head -5)"
anr="$(grep -Ei 'ANR in|am_anr|ActivityManager: Killing' "$log" | head -5)"
if [ -n "$crash" ]; then
  bad "检测到崩溃日志："; printf '%s\n' "$crash" | sed 's/^/      /'
fi
if [ -n "$anr" ]; then
  bad "检测到 ANR/被杀："; printf '%s\n' "$anr" | sed 's/^/      /'
fi
[ -z "$crash$anr" ] && ok "无崩溃/ANR 日志"

# 8) 截图取证
$ADB exec-out screencap -p > "$SHOT" 2>/dev/null
[ -s "$SHOT" ] && ok "截图: $SHOT" || bad "截图失败"

# 9) 判定
echo
if [ "$alive" = "1" ] && [ -z "$crash$anr" ] && grep -q "$PKG" <<<"$resumed"; then
  echo "✅ APK 真机冒烟通过"
  exit 0
fi
echo "❌ APK 真机冒烟失败（日志: $log）"
if grep -qiE 'crashpad|SIGTRAP' "$log"; then
  echo "   ↳ 命中已知环境限制：模拟器无 KVM，APK 内 WebView(Chromium) 初始化即崩；"
  echo "     Go 后端/原生层在模拟器上完全正常（已二分证明）。换有 KVM 的宿主或新 WebView 后本脚本应转绿。"
fi
exit 1
