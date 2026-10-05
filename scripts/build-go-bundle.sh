#!/usr/bin/env bash
# =============================================================================
# build-go-bundle.sh —— 产出**可云控下发**的 Go 二进制热更包（I3）
# -----------------------------------------------------------------------------
# 为什么必须有这个脚本（2026-10-06 真机踩坑，血的教训）：
#
#   手工打包时两次"我以为"让真机连续报错，用户在设备上陪着踩坑：
#     ① ABI 填了 `arm64`（**Go 的架构名**）
#        ⇒ 执行端要的是 **Android ABI 名** `arm64-v8a` ⇒ `missing_abi`
#     ② zip 内文件名填了 `encv`
#        ⇒ 安装器要求 `encv-go` ⇒ `missing required files: [encv-go]`
#
#   根因不是"手滑"，是**约定靠人记**：这些值在代码里都有唯一定义，
#   却允许自由填写 ⇒ 必然写错，且只能在真机上暴露。
#
#   ⇒ 本脚本把约定**固化**：ABI 只能从枚举里选（默认 arm64-v8a，非法值直接
#     拒绝并列出可选值），包内文件名取自代码常量，出包后**自检**再入库。
#
# 约定来源（代码改了请同步改这里，别各写一份）：
#   - 包内文件名 `encv-go`：internal/bundle 的 ApplyFile spec.Required[0]
#     （见 internal/bundle/apply_file_test.go、applyfile_required_test.go）
#   - ABI 合法值：internal/peerlink/bundle.go 的 ABI 字段注释
#     （arm64-v8a / armeabi-v7a / x86_64…），以及 internal/bundle/apply_file_test.go
#
# 产物：
#   <OUT>/go-binary-<VERSION>.zip          zip 根含且仅含一个文件 encv-go
#   <OUT>/go-binary-<VERSION>.zip.sha256   摘要
#   <OUT>/go-binary-<VERSION>.zip.abi      Android ABI 名（Hub manifest 靠它声明 abi）
#
# 用法：
#   bash scripts/build-go-bundle.sh                       # 默认 ABI=arm64-v8a，版本=时间戳
#   bash scripts/build-go-bundle.sh v0.0.2                # 指定版本
#   bash scripts/build-go-bundle.sh --abi armeabi-v7a v1  # 指定 ABI（必须是枚举值）
#   bash scripts/build-go-bundle.sh --list-abi            # 列出可选 ABI
#   OUT_DIR=/tmp/bundles bash scripts/build-go-bundle.sh v1
#
# 之后：POST /api/peerlink/bundle/push {peerId, name:"go-binary"} 下发
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$HOME/.local/share/encv-dev/bundles}"
NDK="${ANDROID_NDK:-${ANDROID_HOME:-/opt/android-sdk}/ndk/latest}"

# ── 约定常量（**不要自由填写**，改这里等于改契约） ─────────────────────────
# 包内文件名：来自 internal/bundle 的 Required[0]
readonly BUNDLE_FILE_NAME="encv-go"
# 包名（Hub 侧按第一个 '-' 切 name/version ⇒ 必须是 go-binary）
readonly BUNDLE_NAME="go-binary"

# Android ABI 枚举（顺序即 --list-abi 输出顺序）
readonly ABI_LIST=(arm64-v8a armeabi-v7a x86_64)
# 默认 ABI：真机主流架构
readonly DEFAULT_ABI="arm64-v8a"

# ABI → (GOARCH, NDK clang 前缀)
abi_goarch() {
  case "$1" in
    arm64-v8a)   echo "arm64" ;;
    armeabi-v7a) echo "arm" ;;
    x86_64)      echo "amd64" ;;
    *)           return 1 ;;
  esac
}

abi_clang() {
  case "$1" in
    arm64-v8a)   echo "aarch64-linux-android21-clang" ;;
    armeabi-v7a) echo "armv7a-linux-androideabi21-clang" ;;
    x86_64)      echo "x86_64-linux-android21-clang" ;;
    *)           return 1 ;;
  esac
}

log()  { printf '\033[1;36m[go-bundle]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[go-bundle]\033[0m %s\n' "$*" >&2; }

# ── 参数解析：ABI 只能选，不允许自由填写 ───────────────────────────────────
ABI="$DEFAULT_ABI"
VERSION=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --list-abi)
      printf '%s\n' "${ABI_LIST[@]}"
      exit 0
      ;;
    --abi)
      shift
      [[ $# -gt 0 ]] || { err "--abi 需要取值；可选值：${ABI_LIST[*]}"; exit 2; }
      ABI="$1"; shift
      ;;
    --abi=*)
      ABI="${1#--abi=}"; shift
      ;;
    -h|--help)
      sed -n '2,40p' "$0"
      exit 0
      ;;
    *)
      VERSION="$1"; shift
      ;;
  esac
done

# 枚举校验：非法值直接拒绝并列出可选值（fail fast，绝不拿去真机上试）
if ! abi_goarch "$ABI" >/dev/null 2>&1; then
  err "非法的 ABI: '$ABI'"
  err "可选值（只能用这些，不允许自由填写）："
  for a in "${ABI_LIST[@]}"; do err "  - $a"; done
  exit 2
fi

if [[ -z "$VERSION" ]]; then
  VERSION="$(cd "$REPO_ROOT" && git describe --tags --always --dirty 2>/dev/null || true)"
  [[ -z "$VERSION" ]] && VERSION="$(date +%Y%m%d-%H%M%S)"
fi
VERSION="${VERSION//\//-}"
VERSION="${VERSION// /-}"

GOARCH="$(abi_goarch "$ABI")"
CLANG="$(abi_clang "$ABI")"
CC_BIN="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/$CLANG"

[[ -x "$CC_BIN" ]] || { err "NDK 编译器不存在: $CC_BIN（检查 ANDROID_NDK / ANDROID_HOME）"; exit 1; }
command -v go >/dev/null 2>&1 || { err "需要 go"; exit 1; }
command -v python3 >/dev/null 2>&1 || { err "需要 python3（打包用）"; exit 1; }

log "ABI=$ABI（枚举内，默认 $DEFAULT_ABI） GOARCH=$GOARCH"
log "CC=$CC_BIN"

TMP_BIN="$(mktemp -d)/encv-go"
trap 'rm -rf "$(dirname "$TMP_BIN")"' EXIT

log "交叉编译 Go 二进制（CGO + NDK）…"
# ⚠️ 注入版本号：让 `get_device_info` 能**远端确证**设备跑的是哪一个包。
#    没有它，version 恒为 "dev" ⇒ 根本分不清跑的是 APK 内置还是热更的
#    （2026-10-06 就是分不清，误以为热更已生效，白推三次）。
#
#    变量名必须与项目一致：scripts/android-common.sh 用的是 **main.version**（小写 v），
#    写成 main.Version 会静默失效（不报错，但版本永远是 dev）。
#
# BUILD_TAGS：与 APK 构建对齐（本机 libsql/objectbox 就绪时可传，如 BUILD_TAGS=libsql）。
# 不带时功能可能少于 APK 内置二进制，但仍应能启动 —— 启动不了会被 Kotlin
# 的 rollbackHotBinaryIfBroken 自动作废并回退（不会让设备起不来后端）。
LDFLAGS="-s -w -X main.version=$VERSION"
BUILD_FLAGS=""
if [[ -n "${BUILD_TAGS:-}" ]]; then
  BUILD_FLAGS="-tags $BUILD_TAGS"
  log "BUILD_TAGS=$BUILD_TAGS（与 APK 构建对齐）"
fi
( cd "$REPO_ROOT" && \
  CGO_ENABLED=1 GOOS=android GOARCH="$GOARCH" CC="$CC_BIN" \
  go build $BUILD_FLAGS -ldflags "$LDFLAGS" -o "$TMP_BIN" ./cmd/encv )

[[ -f "$TMP_BIN" ]] || { err "编译未产出二进制"; exit 1; }
log "二进制：$(du -h "$TMP_BIN" | cut -f1)"

mkdir -p "$OUT_DIR"
ZIP="$OUT_DIR/$BUNDLE_NAME-$VERSION.zip"

# 打包：zip 根放**约定文件名**（不是构建产物名）
python3 - "$TMP_BIN" "$ZIP" "$BUNDLE_FILE_NAME" <<'PY'
import os, sys, zipfile
src, out, name = sys.argv[1], sys.argv[2], sys.argv[3]
if os.path.exists(out):
    os.remove(out)
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    z.write(src, name)
print(name)
PY

# sidecar：abi + sha256（Hub manifest 靠 .abi 声明架构；执行端据此校验）
printf '%s' "$ABI" > "$ZIP.abi"
python3 -c "import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],'rb').read()).hexdigest())" "$ZIP" > "$ZIP.sha256"

# ── 出包自检：不让错误包进仓库（这两条就是踩过的坑） ───────────────────────
python3 - "$ZIP" "$BUNDLE_FILE_NAME" "$ABI" "$ZIP.abi" "$ZIP.sha256" <<'PY'
import sys, zipfile
zip_path, want_name, want_abi, abi_path, sha_path = sys.argv[1:6]
names = zipfile.ZipFile(zip_path).namelist()
if names != [want_name]:
    sys.exit(f"自检失败：zip 内容必须是且仅是 [{want_name}]，实际 {names}")
if open(abi_path).read().strip() != want_abi:
    sys.exit(f"自检失败：.abi 应为 {want_abi}")
if len(open(sha_path).read().strip()) != 64:
    sys.exit("自检失败：.sha256 不是合法 sha256")
print("selfcheck ok")
PY

log "产物：$ZIP ($(du -h "$ZIP" | cut -f1))"
log "ABI  ：$(cat "$ZIP.abi")"
log "摘要 ：$(cat "$ZIP.sha256")"
log "自检 ：通过（zip 内含且仅含 $BUNDLE_FILE_NAME，.abi/.sha256 齐备）"
log "下一步：POST /api/peerlink/bundle/push {peerId, name:\"$BUNDLE_NAME\"}"
