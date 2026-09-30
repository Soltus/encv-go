#!/usr/bin/env bash
# =============================================================================
# android-common.sh — 本地构建脚本与 GitHub Actions 共用的 Android 构建逻辑
# -----------------------------------------------------------------------------
# 单一真相来源（single source of truth），同时被以下两者调用，避免维护冲突：
#   - scripts/build-android.sh        （本地：source 后直接调用函数）
#   - .github/workflows/android.yml （CI：run: bash scripts/android-common.sh <cmd>）
#
# 子命令（直接执行时）：
#   build-go [version] [--abi arm64|x86_64]
#                             CGO + NDK 交叉编译 encv-go → jniLibs/<abi>/libencv-go.so
#                             （含 libsql/objectbox 链接与 .so 拷贝到 jniLibs；
#                               默认 arm64；x86_64 供无 KVM 的 x86_64 模拟器原生运行，
#                               需配合 EMU_X86_64=1 让 Gradle 把该 ABI 打进 APK）
#   build-emu-backend [out]   编 ./cmd/encv 为 android/amd64 **可执行**，
#                             供 hybrid-e2e.sh 推到模拟器里跑真实后端
#   ensure-keystore <dir>     生成 release keystore（若不存在）+ 写入 android/keystore.properties
#
# 环境变量（可选覆盖）：
#   ROOT_DIR              仓库根（默认：脚本所在目录的上一级）
#   MONOREPO_MOBILE_DIR  encv-mobile 相对路径（默认：app/encv-mobile）
#   ANDROID_NDK          NDK 根目录（默认：ANDROID_HOME/ndk/<最新版>）
#   LIBSQL_READY         "1" 启用 libsql 链接（默认 0）
#   OBJECTBOX_READY      "1" 启用 objectbox 链接（默认 0）
# =============================================================================

# ---- 路径解析（仅当未由调用方设置时套用默认）----
ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
MONOREPO_MOBILE_DIR="${MONOREPO_MOBILE_DIR:-app/encv-mobile}"

# 解析 NDK 内的 clang（min API 24，与项目一致）
# 用法：cc="$(android_cc [arm64|x86_64])" || return 1
android_cc() {
  local abi="${1:-arm64}"
  local ndk="${ANDROID_NDK:-}"
  if [[ -z "$ndk" ]]; then
    ndk="$(ls -d "${ANDROID_HOME:-/opt/android-sdk}"/ndk/*/ 2>/dev/null | sort -V | tail -1)"
  fi
  [[ -n "$ndk" ]] || { echo "❌ 无法定位 NDK（请设置 ANDROID_NDK 或 ANDROID_HOME）" >&2; return 1; }
  case "$abi" in
    arm64)  echo "$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang" ;;
    x86_64) echo "$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android24-clang" ;;
    *)      echo "❌ 不支持的 ABI: $abi（只支持 arm64 / x86_64）" >&2; return 1 ;;
  esac
}

# build_go_binary [version] [--abi arm64|x86_64]
# 编译 ./cmd/encv-mobile → <mobile>/encv-go-<abi>，并拷贝为
# android/app/src/main/jniLibs/<abi-v8a|x86_64>/libencv-go.so
# （libsql/objectbox 就绪时按 -tags 链接并拷贝其 .so；这两个原生库目前只有
#  android_arm64 预编译，x86_64 构建会明确跳过并提示降级）
build_go_binary() {
  local version="${1:-dev}"; shift || true
  local abi="arm64"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --abi) abi="${2:-arm64}"; shift 2 ;;
      *) shift ;;
    esac
  done

  # ABI → Go arch / jniLibs 子目录 / 原生库目录后缀
  local goarch jni_dir native_dir
  case "$abi" in
    arm64)  goarch="arm64"; jni_dir="arm64-v8a"; native_dir="android_arm64" ;;
    x86_64) goarch="amd64"; jni_dir="x86_64";    native_dir="android_x86_64" ;;
    *) echo "❌ 不支持的 ABI: $abi（只支持 arm64 / x86_64）" >&2; return 1 ;;
  esac

  local mobile="$ROOT_DIR/$MONOREPO_MOBILE_DIR"
  local android="$mobile/android"
  local jni="$android/app/src/main/jniLibs/$jni_dir"
  mkdir -p "$jni"

  local cc; cc="$(android_cc "$abi")" || return 1
  local tags="" ldflags=""
  if [[ "${LIBSQL_READY:-0}" == "1" ]]; then
    if [[ -d "$ROOT_DIR/pkg/libsql/libs/$native_dir" ]]; then
      tags="${tags:+$tags,}libsql"
      ldflags="$ldflags -L$ROOT_DIR/pkg/libsql/libs/$native_dir"
    else
      echo "⚠️  libsql 无 $native_dir 预编译库，跳过 libsql 链接（功能降级为 SQLite-only）"
    fi
  fi
  if [[ "${OBJECTBOX_READY:-0}" == "1" ]]; then
    if [[ -d "$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/$native_dir" ]]; then
      tags="${tags:+$tags,}objectbox"
      ldflags="$ldflags -L$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/$native_dir"
    else
      echo "⚠️  objectbox 无 $native_dir 预编译库，跳过 objectbox 链接（功能降级为 SQLite）"
    fi
  fi
  [[ -n "$tags" ]] && tags="-tags $tags"

  echo "═══ Build Go binary (Android $abi / GOARCH=$goarch) ═══"
  echo "  version     : $version"
  echo "  CC          : $cc"
  echo "  BUILD_TAGS  : ${tags#-tags }"
  echo "  CGO_LDFLAGS : ${ldflags# }"

  ( cd "$ROOT_DIR" && \
    CGO_ENABLED=1 GOOS=android GOARCH="$goarch" \
    CC="$cc" CGO_LDFLAGS="$ldflags" GOFLAGS="-mod=mod" \
    go build $tags -ldflags="-s -w -X main.version=$version" \
    -o "$mobile/encv-go-$abi" ./cmd/encv-mobile ) || return 1

  cp "$mobile/encv-go-$abi" "$jni/libencv-go.so"
  echo "✅ libencv-go.so → $jni"

  if [[ "${LIBSQL_READY:-0}" == "1" ]] && [[ -f "$ROOT_DIR/pkg/libsql/libs/$native_dir/libsql_experimental.so" ]]; then
    cp "$ROOT_DIR/pkg/libsql/libs/$native_dir/libsql_experimental.so" "$jni/"
    echo "✅ libsql_experimental.so → $jni"
  fi
  if [[ "${OBJECTBOX_READY:-0}" == "1" ]] && [[ -f "$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/$native_dir/libobjectbox-jni.so" ]]; then
    cp "$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/$native_dir/libobjectbox-jni.so" "$jni/"
    echo "✅ libobjectbox-jni.so → $jni"
  fi
}

# build_emu_backend_binary [out_path]
# 编 ./cmd/encv 为 android/amd64 **可执行**（不是 .so），推到模拟器里可直接跑：
#   adb push <out> /data/local/tmp/encv-x64 && adb shell /data/local/tmp/encv-x64 start
# 用途：hybrid-e2e.sh 在模拟器内起真实后端（Android 文件/权限/mount 语义 = 真机），
#       宿主机 Chromium 跑前端（模拟器内 WebView 会崩，见 memory 2026-10-01 §8-10）。
build_emu_backend_binary() {
  local out="${1:-/tmp/encv-x64}"
  local cc; cc="$(android_cc x86_64)" || return 1
  echo "═══ Build Android x86_64 backend binary (./cmd/encv) ═══"
  echo "  out : $out"
  echo "  CC  : $cc"
  ( cd "$ROOT_DIR" && \
    CGO_ENABLED=1 GOOS=android GOARCH=amd64 \
    CC="$cc" GOFLAGS="-mod=mod" \
    go build -o "$out" ./cmd/encv ) || return 1
  echo "✅ $out"
}

# ensure_android_keystore <mobile_dir_relative>
# 生成 <mobile>/keystore/release.jks（若不存在），并写入
# <mobile>/android/keystore.properties 供 Gradle 签名读取。
ensure_android_keystore() {
  local mobile="$ROOT_DIR/${1:-$MONOREPO_MOBILE_DIR}"
  local ksdir="$mobile/keystore"
  local ks="$ksdir/release.jks"
  mkdir -p "$ksdir"
  if [[ ! -f "$ks" ]]; then
    echo "生成 release keystore: $ks"
    keytool -genkeypair -v -keystore "$ks" \
      -storepass encv2025 -alias encvrelease \
      -keypass encv2025 \
      -keyalg RSA -keysize 2048 -validity 10000 \
      -dname "CN=ENCV-go, OU=Personal, O=ENCV, L=Unknown, ST=Unknown, C=CN"
  fi
  cat > "$mobile/android/keystore.properties" <<EOF
storeFile=$ks
storePassword=encv2025
keyAlias=encvrelease
keyPassword=encv2025
EOF
  echo "✅ keystore.properties → $mobile/android/keystore.properties"
}

main() {
  local cmd="${1:-}"; shift || true
  case "$cmd" in
    build-go)          build_go_binary "$@" ;;
    build-emu-backend) build_emu_backend_binary "$@" ;;
    ensure-keystore)   ensure_android_keystore "$@" ;;
    *) echo "usage: android-common.sh {build-go|build-emu-backend|ensure-keystore} ..." >&2; exit 1 ;;
  esac
}

# 仅当直接执行（非 source）时进入 main
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -euo pipefail
  main "$@"
fi
