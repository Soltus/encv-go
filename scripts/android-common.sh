#!/usr/bin/env bash
# =============================================================================
# android-common.sh — 本地构建脚本与 GitHub Actions 共用的 Android 构建逻辑
# -----------------------------------------------------------------------------
# 单一真相来源（single source of truth），同时被以下两者调用，避免维护冲突：
#   - scripts/build-android.sh        （本地：source 后直接调用函数）
#   - .github/workflows/android.yml （CI：run: bash scripts/android-common.sh <cmd>）
#
# 子命令（直接执行时）：
#   build-go [version]        CGO + NDK 交叉编译 encv-go → libencv-go.so
#                               （含 libsql/objectbox 链接与 .so 拷贝到 jniLibs）
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

# 解析 NDK 内的 aarch64 clang（min API 24，与项目一致）
# 用法：cc="$(android_cc)" || return 1
android_cc() {
  local ndk="${ANDROID_NDK:-}"
  if [[ -z "$ndk" ]]; then
    ndk="$(ls -d "${ANDROID_HOME:-/opt/android-sdk}"/ndk/*/ 2>/dev/null | sort -V | tail -1)"
  fi
  [[ -n "$ndk" ]] || { echo "❌ 无法定位 NDK（请设置 ANDROID_NDK 或 ANDROID_HOME）" >&2; return 1; }
  echo "$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang"
}

# build_go_binary [version]
# 编译 ./cmd/encv-mobile → <mobile>/encv-go-arm64，并拷贝为
# android/app/src/main/jniLibs/arm64-v8a/libencv-go.so
# （libsql/objectbox 就绪时按 -tags 链接并拷贝其 .so）
build_go_binary() {
  local version="${1:-dev}"
  local mobile="$ROOT_DIR/$MONOREPO_MOBILE_DIR"
  local android="$mobile/android"
  local jni="$android/app/src/main/jniLibs/arm64-v8a"
  mkdir -p "$jni"

  local cc; cc="$(android_cc)" || return 1
  local tags="" ldflags=""
  if [[ "${LIBSQL_READY:-0}" == "1" ]]; then
    tags="${tags:+$tags,}libsql"
    ldflags="$ldflags -L$ROOT_DIR/pkg/libsql/libs/android_arm64"
  fi
  if [[ "${OBJECTBOX_READY:-0}" == "1" ]]; then
    tags="${tags:+$tags,}objectbox"
    ldflags="$ldflags -L$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/android_arm64"
  fi
  [[ -n "$tags" ]] && tags="-tags $tags"

  echo "═══ Build Go binary (Android arm64) ═══"
  echo "  version     : $version"
  echo "  CC          : $cc"
  echo "  BUILD_TAGS  : ${tags#-tags }"
  echo "  CGO_LDFLAGS : ${ldflags# }"

  ( cd "$ROOT_DIR" && \
    CGO_ENABLED=1 GOOS=android GOARCH=arm64 \
    CC="$cc" CGO_LDFLAGS="$ldflags" GOFLAGS="-mod=mod" \
    go build $tags -ldflags="-s -w -X main.version=$version" \
    -o "$mobile/encv-go-arm64" ./cmd/encv-mobile )

  cp "$mobile/encv-go-arm64" "$jni/libencv-go.so"
  echo "✅ libencv-go.so → $jni"

  if [[ "${LIBSQL_READY:-0}" == "1" ]] && [[ -f "$ROOT_DIR/pkg/libsql/libs/android_arm64/libsql_experimental.so" ]]; then
    cp "$ROOT_DIR/pkg/libsql/libs/android_arm64/libsql_experimental.so" "$jni/"
    echo "✅ libsql_experimental.so → $jni"
  fi
  if [[ "${OBJECTBOX_READY:-0}" == "1" ]] && [[ -f "$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/android_arm64/libobjectbox-jni.so" ]]; then
    cp "$ROOT_DIR/pkg/tasksystem/store/objectbox/libs/android_arm64/libobjectbox-jni.so" "$jni/"
    echo "✅ libobjectbox-jni.so → $jni"
  fi
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
    build-go)        build_go_binary "$@" ;;
    ensure-keystore) ensure_android_keystore "$@" ;;
    *) echo "usage: android-common.sh {build-go|ensure-keystore} ..." >&2; exit 1 ;;
  esac
}

# 仅当直接执行（非 source）时进入 main
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -euo pipefail
  main "$@"
fi
