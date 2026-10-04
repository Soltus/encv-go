#!/usr/bin/env bash
# =============================================================================
# build-web-bundle.sh —— 产出**可云控下发**的主应用 SPA 包（I4 热更新用）
# -----------------------------------------------------------------------------
# 产物：<OUT>/web-<VERSION>.zip + .sha256（Hub 侧 bundle 仓库直接吃这两个文件）
#
# 为什么需要它：
#   主应用 SPA 热更新（I4）走的是 Capacitor 本地服务改指向
#   `<filesDir>/.encv/web-bundle`（见 MainActivity.applyHotWebBundleIfPresent），
#   该目录由云控通道（I2 的 bundle 安装器）写入 ⇒ 必须有"能下发的 zip"。
#   zip 内容就是 `pnpm build` 的 dist/（base 保持 '/'，因为托管在 https://localhost/ 根）。
#
# 用法：
#   bash scripts/build-web-bundle.sh                    # 版本 = git describe 或 日期
#   bash scripts/build-web-bundle.sh v0.0.1-hotfix      # 指定版本
#   bash scripts/build-web-bundle.sh v1 --no-build      # 跳过构建，只打包现有 dist/
#   OUT_DIR=/tmp/bundles bash scripts/build-web-bundle.sh v1
#
# 之后：
#   1) 把 zip + .sha256 放进 Hub 的 bundle 仓库（默认 ~/.local/share/encv-dev/bundles，
#      或 ENCV_BUNDLES_DIR 指定的目录）
#   2) POST /api/peerlink/bundle/push {peerId, name:"web"} 下发到设备
#   3) 设备端装好后，用 get_device_info 的 webBundle 字段远程确认生效版本
#      （Kotlin 侧要目录里同时有 index.html 与 version.json 才切换）
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_DIR="$REPO_ROOT/app/encv-mobile"
DIST_DIR="$APP_DIR/dist"
OUT_DIR="${OUT_DIR:-$HOME/.local/share/encv-dev/bundles}"

VERSION="${1:-}"
SKIP_BUILD=0
for arg in "$@"; do
  case "$arg" in
    --no-build) SKIP_BUILD=1 ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  VERSION="$(cd "$REPO_ROOT" && git describe --tags --always --dirty 2>/dev/null || true)"
  if [[ -z "$VERSION" ]]; then
    VERSION="$(date +%Y%m%d-%H%M%S)"
  fi
fi
# 版本号里允许连字符（Hub 侧按**第一个** '-' 切 name/version），但不能有空格或斜杠
VERSION="${VERSION//\//-}"
VERSION="${VERSION// /-}"

log()  { printf '\033[1;36m[web-bundle]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[web-bundle]\033[0m %s\n' "$*" >&2; }

command -v python3 >/dev/null 2>&1 || { err "需要 python3（容器里通常没有 zip 命令）"; exit 1; }

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  log "构建 SPA（vite build）…"
  ( cd "$APP_DIR" && pnpm build )
else
  log "跳过构建（--no-build），直接打包现有 dist/"
fi

if [[ ! -f "$DIST_DIR/index.html" ]]; then
  err "dist/index.html 不存在 —— 先构建成功再打包"
  exit 1
fi

mkdir -p "$OUT_DIR"
ZIP="$OUT_DIR/web-$VERSION.zip"

# 打包：dist 内的内容放在 zip **根**（安装器会剥掉唯一外层目录，放根更稳）
python3 - "$DIST_DIR" "$ZIP" <<'PY'
import os, sys, zipfile
src, out = sys.argv[1], sys.argv[2]
if os.path.exists(out):
    os.remove(out)
n = 0
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for root, _dirs, files in os.walk(src):
        for f in files:
            p = os.path.join(root, f)
            rel = os.path.relpath(p, src)
            z.write(p, rel)
            n += 1
print(n)
PY

python3 - "$ZIP" > "$ZIP.sha256" <<'PY'
import hashlib, sys
print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())
PY

log "产物：$ZIP ($(du -h "$ZIP" | cut -f1))"
log "摘要：$(cat "$ZIP.sha256")"
log "下一步：把这两个文件放进 Hub 的 bundle 仓库，然后 push {name:\"web\"} 到设备"
