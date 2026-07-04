#!/usr/bin/env bash
# =============================================================================
# build-openlist-aar.sh
# -----------------------------------------------------------------------------
# Build the OpenList Android AAR (gomobile bind product) from the Hi-Sillot
# OpenList fork and drop it under --output/openlist.aar for ComboLite to
# consume via `app/encv-mobile/plugin-openlist/libs/openlist.aar`.
#
# Required environment:
#   - Go 1.25.x     (matches Hi-Sillot fork go.mod)
#   - NDK r25c+     (r26b / 26.3.11579264 recommended)
#   - Java 17       (Temurin / OpenJDK)
#   - cmake, git, tar, curl, sha256sum, jq (jq only required when fork ships
#     public/dist/i18n-overlay/)
#
# Configuration precedence (highest first):
#   1. CLI flags                                (--fork / --branch / ...)
#   2. scripts/openlist-fork.env.local          (gitignored personal override)
#   3. scripts/openlist-fork.env                (tracked default)
#   4. hard-coded fallback                      (only when above are absent)
# =============================================================================
# TODO: keep NDK version in sync with .github/workflows/build-mpv-lib.yml.
# TODO: Hi-Sillot fork must already contain `openlistlib/` (see
#       .trae/specs/integrate-openlist-as-combolite-plugin/spec.md §一).
# TODO: when adding new ENCV setting items in the fork, bump the version
#       recorded in Hi-Sillot/OpenList/frontend-pinned.txt.
# =============================================================================

set -euo pipefail

source "$(dirname "$0")/openlist-fork.env" 2>/dev/null || true
if [[ -f "$(dirname "$0")/openlist-fork.env.local" ]]; then
    # shellcheck disable=SC1091
    source "$(dirname "$0")/openlist-fork.env.local"
fi

NDK_DEFAULT="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}/ndk/26.3.11579264"
_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENCV_GO_ROOT_DEFAULT="$(cd "${_SCRIPT_DIR}/.." && pwd)"

FORK="${OPENLIST_FORK_URL:-https://github.com/Hi-Sillot/OpenList}"
BRANCH="${OPENLIST_FORK_BRANCH:-dev}"
NDK="${NDK_DEFAULT}"
ENCV_GO_ROOT="${ENCV_GO_ROOT_DEFAULT}"
OUTPUT=""
FRONTEND_VERSION_CLI=""
LOCAL_FRONTEND_DIST=""

usage() {
    cat <<EOF
Usage: $(basename "$0") --output <aar-dir> [options]

Options:
  --output                 <dir>    Output directory for openlist.aar (required)
  --fork                   <url>    Hi-Sillot fork URL       (default: ${FORK})
  --branch                 <name>   Git branch / tag          (default: ${BRANCH})
  --ndk                    <path>   Android NDK install path  (env: ANDROID_NDK_HOME)  (default: ${NDK_DEFAULT})
  --encv-go-root           <dir>    Local encv-go checkout    (default: ${ENCV_GO_ROOT_DEFAULT})
  --frontend-version       <vX.Y.Z> Pin OpenList-Frontend version (overrides env and frontend-pinned.txt)
  --local-frontend-dist    <dir>    Skip download, copy local frontend dist directly into public/dist/
  -h, --help                           Show this help

Defaults are loaded from scripts/openlist-fork.env:
  OPENLIST_FORK_URL / OPENLIST_FORK_BRANCH / OPENLIST_FRONTEND_VERSION
CLI flags always win over env values.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output)                OUTPUT="${2:-}"; shift 2 ;;
        --fork)                  FORK="${2:-}"; shift 2 ;;
        --branch)                BRANCH="${2:-}"; shift 2 ;;
        --ndk)                   NDK="${2:-}"; shift 2 ;;
        --encv-go-root)          ENCV_GO_ROOT="${2:-}"; shift 2 ;;
        --frontend-version)      FRONTEND_VERSION_CLI="${2:-}"; shift 2 ;;
        --local-frontend-dist)   LOCAL_FRONTEND_DIST="${2:-}"; shift 2 ;;
        -h|--help)               usage; exit 0 ;;
        *) echo "ERROR: unknown option: $1" >&2; usage; exit 2 ;;
    esac
done

if [[ -z "${OUTPUT}" ]]; then
    echo "ERROR: --output is required" >&2
    usage
    exit 2
fi

if [[ ! -d "${ENCV_GO_ROOT}" ]]; then
    echo "ERROR: encv-go root not found: ${ENCV_GO_ROOT}" >&2
    exit 2
fi

ENCV_GO_ROOT="$(cd "${ENCV_GO_ROOT}" && pwd)"
ENCV_GO_ROOT="${ENCV_GO_ROOT%/}"

mkdir -p "${OUTPUT}"
OUTPUT="$(cd "${OUTPUT}" && pwd)"

log() { printf '\033[1;36m[openlist-aar]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[openlist-aar]\033[0m %s\n' "$*" >&2; exit 1; }

log "== fork env =="
log "  OPENLIST_FORK_BRANCH=${OPENLIST_FORK_BRANCH:-<unset>}"
log "  OPENLIST_FRONTEND_VERSION=${OPENLIST_FRONTEND_VERSION:-<unset>}"

log "== Environment check =="
command -v go       >/dev/null 2>&1 || die "go not found in PATH"
command -v java     >/dev/null 2>&1 || die "java not found in PATH"
command -v git      >/dev/null 2>&1 || die "git not found in PATH"
command -v curl     >/dev/null 2>&1 || die "curl not found in PATH"
command -v tar      >/dev/null 2>&1 || die "tar not found in PATH"
command -v cmake    >/dev/null 2>&1 || die "cmake not found in PATH (NDK toolchain needs it)"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum not found in PATH"
command -v jq       >/dev/null 2>&1 || log "  (jq not found, will skip i18n overlay merge if fork ships it)"

if [[ ! -d "${NDK}" ]]; then
    if [[ -d "${ANDROID_HOME:-}/ndk/26.3.11579264" ]]; then
        NDK="${ANDROID_HOME}/ndk/26.3.11579264"
    elif [[ -d "${ANDROID_HOME:-}/ndk/25.2.9519653" ]]; then
        NDK="${ANDROID_HOME}/ndk/25.2.9519653"
    else
        die "NDK not found at: ${NDK}"
    fi
fi
NDK="$(cd "${NDK}" && pwd)"
[[ -x "${NDK}/ndk-build" ]] || die "ndk-build not executable under ${NDK}"

log "== Toolchain =="
log "  go         : $(go version)"
log "  java       : $(java -version 2>&1 | head -n 1)"
log "  NDK        : ${NDK}"
log "  encv-go    : ${ENCV_GO_ROOT}"
log "  fork       : ${FORK}@${BRANCH}"
log "  output dir : ${OUTPUT}"
log "  frontend-version CLI : ${FRONTEND_VERSION_CLI:-<none>}"
log "  local-frontend-dist  : ${LOCAL_FRONTEND_DIST:-<none>}"

# Default fork work dir: app/openlist/Hi-Sillot-OpenList/ under the encv-mobile
# repo root. This location matches the fork's own `go.mod` line
# `replace github.com/Soltus/encv-go => ../../../` so the relative replace
# resolves naturally to the encv-go root (no sed patching needed).
# Override with OPENLIST_FORK_WORK_DIR for CI runners that want to reuse a
# cached clone on a separate volume (e.g. /cache/fork).
if [[ -n "${OPENLIST_FORK_WORK_DIR:-}" ]]; then
    WORK_DIR="${OPENLIST_FORK_WORK_DIR}"
else
    _REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
    WORK_DIR="${_REPO_ROOT}/app/openlist/Hi-Sillot-OpenList"
fi
SRC_DIR="${WORK_DIR}"

log "== Workspace =="
log "  ${WORK_DIR}"
rm -rf "${SRC_DIR}"
mkdir -p "${WORK_DIR}"

log "== Clone Hi-Sillot fork (--depth 1) =="
# Build an auth-aware git clone when GITHUB_TOKEN is exported in the sandbox.
# Sandboxed shells do NOT auto-route env vars as git credentials, so a bare
# `git clone` would fail with "could not read Username for 'https://github.com':
# terminal prompts disabled". The reliable fix is to inject the token into the
# URL (`x-access-token` triggers HTTP Basic Auth, which GitHub PATs accept).
# `git -c http.extraHeader=Authorization: Bearer ...` works for clone in some
# Git versions but is rejected as "invalid credentials" on push, so we use the
# URL-injection form uniformly. See app/openlist/README.md §10 for the full
# rationale and the same pattern applied manually to `git push`.
CLONE_URL="${FORK}"
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    CLONE_URL="https://x-access-token:${GITHUB_TOKEN}@github.com/${FORK#https://github.com/}"
    TOKEN_PREFIX="${GITHUB_TOKEN:0:4}"
    log "  [INFO] using GITHUB_TOKEN for fork auth (${TOKEN_PREFIX}****)"
else
    log "  [WARN] GITHUB_TOKEN not set, falling back to anonymous clone (will fail on private repos)"
fi
git clone --depth 1 --branch "${BRANCH}" "${CLONE_URL}" "${SRC_DIR}"

GOMOD="${SRC_DIR}/go.mod"
[[ -f "${GOMOD}" ]] || die "go.mod not found in ${SRC_DIR}"

log "== Verify fork go.mod relative replace resolves correctly =="
# Fork is expected at app/openlist/Hi-Sillot-OpenList/, so go.mod's
# `replace github.com/Soltus/encv-go => ../../../` resolves to the encv-go
# root (parent of app/). If fork moves to a non-standard location, sed-patch
# the replace back to an absolute path. See D4 in
# .trae/documents/fork-clone-path-refactor-to-app-openlist.md.
_REL_REPLACE="$(grep -E '^replace[[:space:]]+github\.com/Soltus/encv-go[[:space:]]+=>' "${GOMOD}" 2>/dev/null | head -n 1 || true)"
case "${_REL_REPLACE}" in
    *../../../*|*"\${ENCV_GO_ROOT}"*)
        log "  (relative replace detected: ${_REL_REPLACE##* } → resolves from fork to encv-go root)"
        ;;
    *)
        if [[ -n "${_REL_REPLACE}" ]]; then
            log "  WARN: non-relative replace found, fork go.mod has been modified upstream:"
            log "        ${_REL_REPLACE}"
            log "        sed-patching back to absolute path '${ENCV_GO_ROOT}' as safety net"
            sed -i.bak -E "s|^replace[[:space:]]+github\\.com/Soltus/encv-go[[:space:]]+=>[[:space:]]+[^[:space:]]+|replace github.com/Soltus/encv-go => ${ENCV_GO_ROOT}|" "${GOMOD}"
            rm -f "${GOMOD}.bak"
        else
            log "  WARN: no encv-go replace line found at all, appending one"
            printf '\nreplace github.com/Soltus/encv-go => %s\n' "${ENCV_GO_ROOT}" >> "${GOMOD}"
        fi
        ;;
esac

# Patch 2: gomobile bind's pre-flight `build.Import("golang.org/x/mobile", ...)`
# walks the module's *package graph* and fails when the package is only
# declared via a Go 1.24 `tool` directive. The `tool` directive only
# registers the gobind binary; it does NOT make golang.org/x/mobile
# importable from module code, so the bind check still misses it.
#
# The gomobile error message suggests `go get -tool`, but that adds a
# `tool` directive — which is precisely the line the fork already has and
# which the bind check ignores. The actual fix is a regular `require`
# directive (go.dev/issue/77183; cmd/gomobile/bind.go in golang.org/x/mobile).
#
# This patch is idempotent: if the fork (or a future version of it) already
# ships a `require golang.org/x/mobile v...` line, the `go get` is a no-op.
if grep -qE 'golang\.org/x/mobile[[:space:]]+v[0-9]' "${GOMOD}"; then
    log "  (require golang.org/x/mobile already present in go.mod, no patch needed)"
else
    # Pin to the same pseudo-version gomobile itself downloads, so the
    # module graph matches the gobind binary we install a few lines below.
    PINNED_MOBILE_VERSION="v0.0.0-20260529142300-ecb4cd65260a"
    log "  adding require golang.org/x/mobile ${PINNED_MOBILE_VERSION}"
    log "  reason: gomobile bind needs the package in the module graph, not just the tool directive"
    ( cd "${SRC_DIR}" && go get "golang.org/x/mobile@${PINNED_MOBILE_VERSION}" ) \
        || die "go get golang.org/x/mobile@${PINNED_MOBILE_VERSION} failed"
    log "  current golang.org/x/mobile entries in go.mod:"
    grep -E 'golang\.org/x/mobile' "${GOMOD}" | while IFS= read -r line; do
        log "    ${line}"
    done
fi

log "== Resolve frontend version =="
DIST_DIR="${SRC_DIR}/public/dist"
mkdir -p "${DIST_DIR}"

FRONTEND_VERSION=""

if [[ -n "${LOCAL_FRONTEND_DIST}" ]]; then
    [[ -d "${LOCAL_FRONTEND_DIST}" ]] || die "--local-frontend-dist path not found: ${LOCAL_FRONTEND_DIST}"
    log "  source: local dist at ${LOCAL_FRONTEND_DIST}"
    rm -rf "${DIST_DIR}"
    mkdir -p "${DIST_DIR}"
    cp -a "${LOCAL_FRONTEND_DIST}/." "${DIST_DIR}/"
    [[ -f "${DIST_DIR}/index.html" ]] || die "local frontend dist missing index.html after copy"
    FRONTEND_VERSION="${FRONTEND_VERSION_CLI:-${OPENLIST_FRONTEND_VERSION:-local}}"
    log "  version: ${FRONTEND_VERSION} (label, not a real upstream tag)"
else
    # Single source of truth: CLI > env > fork's frontend-pinned.txt > latest
    if [[ -n "${FRONTEND_VERSION_CLI}" ]]; then
        FRONTEND_VERSION="${FRONTEND_VERSION_CLI}"
        log "  source: --frontend-version CLI"
    elif [[ -n "${OPENLIST_FRONTEND_VERSION:-}" ]]; then
        FRONTEND_VERSION="${OPENLIST_FRONTEND_VERSION}"
        log "  source: OPENLIST_FRONTEND_VERSION env"
    elif [[ -f "${SRC_DIR}/frontend-pinned.txt" ]]; then
        PINNED="$(cat "${SRC_DIR}/frontend-pinned.txt" 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?' | head -n 1 || true)"
        if [[ -n "${PINNED}" ]]; then
            FRONTEND_VERSION="${PINNED}"
            log "  source: fork frontend-pinned.txt"
        fi
    fi
    if [[ -z "${FRONTEND_VERSION}" ]]; then
        FRONTEND_VERSION="latest"
        echo "[WARN] no frontend pin, using latest" >&2
        log "  source: fallback (releases/latest) — set OPENLIST_FRONTEND_VERSION or --frontend-version to pin"
    fi
    log "  version: ${FRONTEND_VERSION}"

    if [[ "${FRONTEND_VERSION}" == "latest" ]]; then
        FE_API="https://api.github.com/repos/OpenListTeam/OpenList-Frontend/releases/latest"
    else
        FE_API="https://api.github.com/repos/OpenListTeam/OpenList-Frontend/releases/tags/${FRONTEND_VERSION}"
    fi

    RELEASE_INFO="$(curl -fsSL --max-time 15 -H 'Accept: application/vnd.github.v3+json' "${FE_API}" || true)"
    if [[ -z "${RELEASE_INFO}" ]]; then
        die "OpenList-Frontend ${FRONTEND_VERSION} not found (or API rate-limited)"
    fi

    if command -v jq >/dev/null 2>&1; then
        DL_URL="$(printf '%s' "${RELEASE_INFO}" \
            | jq -r '.assets[] | select(.browser_download_url | test("openlist-frontend-dist.*\\.tar\\.gz$")) | select(.browser_download_url | test("openlist-frontend-dist-lite") | not) | .browser_download_url' \
            | head -n 1)"
    else
        DL_URL="$(printf '%s' "${RELEASE_INFO}" \
            | grep -oE '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*openlist-frontend-dist[^"]*\.tar\.gz"' \
            | grep -v 'lite' | head -n 1 \
            | sed -E 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
    fi
    [[ -n "${DL_URL}" && "${DL_URL}" != "null" ]] || die "could not resolve frontend tarball URL for ${FRONTEND_VERSION}"
    log "  frontend: ${DL_URL}"

    TMP_TAR="${WORK_DIR}/openlist-frontend-dist.tar.gz"
    curl -fsSL --max-time 60 -o "${TMP_TAR}" "${DL_URL}"
    tar -xzf "${TMP_TAR}" -C "${DIST_DIR}" --strip-components=1
    rm -f "${TMP_TAR}"
    [[ -f "${DIST_DIR}/index.html" ]] || die "frontend dist extraction failed (no index.html)"
fi

log "== Apply i18n overlay =="
OVERLAY_DIR="${SRC_DIR}/public/dist/i18n-overlay"
if [[ -d "${OVERLAY_DIR}" ]]; then
    if ! command -v jq >/dev/null 2>&1; then
        die "i18n-overlay/ exists in fork but jq is not installed"
    fi
    ASSETS_DIR="${DIST_DIR}/assets"
    if [[ -d "${ASSETS_DIR}" ]]; then
        find "${OVERLAY_DIR}" -type f -name 'translation.json' | while read -r overlay_file; do
            rel="${overlay_file#${OVERLAY_DIR}/}"
            lang="${rel%%/*}"
            target="${ASSETS_DIR}/${lang}.json"
            if [[ -f "${target}" ]]; then
                tmp="$(mktemp)"
                if jq -s '.[0] * .[1]' "${target}" "${overlay_file}" > "${tmp}"; then
                    mv "${tmp}" "${target}"
                    log "  merged ${lang}: $(basename "${overlay_file}")"
                else
                    rm -f "${tmp}"
                    die "jq merge failed for ${lang}"
                fi
            else
                log "  skipped ${lang}: ${target} not present in frontend dist"
            fi
        done
    else
        log "  (no ${ASSETS_DIR} in frontend dist, skipping overlay merge)"
    fi
else
    log "  (no i18n-overlay/ in fork, nothing to merge)"
fi

log "== Write public/dist/VERSION =="
echo "${FRONTEND_VERSION}-encv" > "${DIST_DIR}/VERSION"
log "  ${DIST_DIR}/VERSION = $(cat "${DIST_DIR}/VERSION")"

log "== Set up NDK env =="
export ANDROID_HOME="${ANDROID_HOME:-$(dirname "$(dirname "${NDK}")")}"
export ANDROID_NDK_HOME="${NDK}"
log "  ANDROID_HOME=${ANDROID_HOME}"
log "  ANDROID_NDK_HOME=${ANDROID_NDK_HOME}"

log "== Install / update gomobile =="
GOPATH_BIN="$(go env GOPATH)/bin"
mkdir -p "${GOPATH_BIN}"
export PATH="${GOPATH_BIN}:${PATH}"
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
gomobile init

cd "${SRC_DIR}"
BIND_PKG=""
if [[ -d "openlistlib" ]] && ls openlistlib/*.go >/dev/null 2>&1; then
    BIND_PKG="./openlistlib"
elif [[ -d "cmd/openlistlib" ]] && ls cmd/openlistlib/*.go >/dev/null 2>&1; then
    BIND_PKG="./cmd/openlistlib"
else
    die "Hi-Sillot fork is missing openlistlib/ (see spec §一) and no fallback exists"
fi

# Sanity-check: fork must declare the gomobile tool directive (commit e3cd5b3+).
# This is what makes the `gobind` binary invocable from `go generate` and similar
# flows. The actual gomobile bind *package* dependency is added by the earlier
# `Patch 2` block (a regular `require golang.org/x/mobile ...` directive),
# because the bind pre-flight only consults the package graph, not the tool
# graph. See go.dev/issue/77183.
if ! grep -qE '^tool[[:space:]]+golang\.org/x/mobile/cmd/gobind' go.mod 2>/dev/null; then
    die "fork go.mod missing 'tool golang.org/x/mobile/cmd/gobind' directive (see go.dev/issue/77183)"
fi

# A2 fallback: Hi-Sillot/OpenList fork is supposed to ship `openlistlib/event.go`
# per spec §一 (Event + LogCallback interfaces for gobind). If the fork is
# behind and hasn't pushed it yet, we inject a minimal compatible event.go so
# `go build ./openlistlib` doesn't fail with `undefined: LogCallback`. Once the
# fork catches up, this block is a no-op. See
# .trae/documents/openlist-aar-sqlite-cgo-multi-solution.md §三 A2.
FORK_HEAD="$(git -C "${SRC_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
EVENT_GO="${SRC_DIR}/openlistlib/event.go"
if [[ ! -f "${EVENT_GO}" ]]; then
    log "  [A2] openlistlib/event.go missing in fork @ ${FORK_HEAD}, injecting fallback"
    cat > "${EVENT_GO}" <<'EOF'
// Code generated by scripts/build-openlist-aar.sh (A2 fallback). DO NOT EDIT
// in this script — the authoritative source lives in Hi-Sillot/OpenList fork.
// This file is overwritten on every build if the fork hasn't shipped it yet.
// Once the fork adds openlistlib/event.go, this injection becomes a no-op.
package openlistlib

type Event interface {
	OnStartError(eventType string, msg string)
	OnShutdown(eventType string)
	// gomobile maps Go `int` to Java `long` on 64-bit Android (linux/arm64
	// is our only target ABI). See genjava.go case types.Int64/types.UntypedInt
	// → java.Long. Writing `int` here would still produce Java `long`, but
	// for clarity (and to match what Hi-Sillot/OpenList@404daf0's event.go
	// uses) we keep `int64` to make the AAR regeneration identical.
	OnProcessExit(code int64)
}

type LogCallback interface {
	OnLog(level int16, time int64, log string)
}
EOF
    log "  [A2] injected: $(wc -l < "${EVENT_GO}") lines into ${EVENT_GO}"
else
    log "  [A2] openlistlib/event.go present in fork @ ${FORK_HEAD}, no injection needed"
fi

# B2 fallback: if the fork still uses gorm.io/driver/sqlite (which chains into
# mattn/go-sqlite3, a CGO package), we must give gomobile bind a working C
# cross-compiler pointing at the NDK. gomobile already sets CGO_ENABLED=1 for
# Android targets, but without CC/CXX it falls back to the host gcc, which
# can't produce android-arm64 ELF. We pre-set CC/CXX to the NDK clang.
#
# If the fork has switched to github.com/glebarez/sqlite (pure-Go, no CGO),
# these exports are harmless extra env vars — gomobile's `go build` will simply
# not invoke CGO. See .trae/documents/openlist-aar-sqlite-cgo-multi-solution.md
# §三 B2.
NDK_CLANG_DIR="${ANDROID_NDK_HOME}/toolchains/llvm/prebuilt/linux-x86_64/bin"
if [[ -x "${NDK_CLANG_DIR}/aarch64-linux-android21-clang" ]]; then
    export CC="${NDK_CLANG_DIR}/aarch64-linux-android21-clang"
    export CXX="${NDK_CLANG_DIR}/aarch64-linux-android21-clang++"
    log "  [B2] CGO toolchain pinned: CC=${CC}"
    log "  [B2] CXX=${CXX}"
    # Belt-and-suspenders: gomobile should already enable CGO for -target=android,
    # but some versions / paths silently fall back to CGO_ENABLED=0.
    export CGO_ENABLED=1
else
    log "  [B2] NDK clang not found at ${NDK_CLANG_DIR} (skipping CC/CXX pin);"
    log "       if build fails on mattn/go-sqlite3, install NDK r25c+ or switch fork to glebarez/sqlite"
fi

log "== gomobile bind (bind pkg: ${BIND_PKG}) =="

LDFLAGS="-s -w"
LDFLAGS="${LDFLAGS} -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.Version=${BRANCH}'"
LDFLAGS="${LDFLAGS} -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.WebVersion=${FRONTEND_VERSION}'"
LDFLAGS="${LDFLAGS} -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.BuiltAt=$(date +'%F %T %z')'"
LDFLAGS="${LDFLAGS} -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.GitAuthor=The OpenList Projects Contributors <noreply@openlist.team>'"
LDFLAGS="${LDFLAGS} -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.GitCommit=$(git -C "${SRC_DIR}" rev-parse --short HEAD)'"

# 16KB page size alignment (NDK 28+ requirement, also future-proofs older NDKs).
# See: https://developer.android.com/guide/practices/page-sizes
export CGO_CFLAGS="-O2"
export CGO_CXXFLAGS="-O2"
export CGO_LDFLAGS="-O2 -s -w -Wl,-z,max-page-size=16384"

cd "${SRC_DIR}"
gomobile bind \
    -ldflags "${LDFLAGS}" \
    -v \
    -androidapi 21 \
    -target="android/arm64" \
    -o "${OUTPUT}/openlist.aar" \
    "${BIND_PKG}"

[[ -s "${OUTPUT}/openlist.aar" ]] || die "openlist.aar was not produced"

log "== Checksum =="
( cd "${OUTPUT}" && sha256sum openlist.aar > openlist.aar.sha256 )
cat "${OUTPUT}/openlist.aar.sha256"

log "== Copy frontend dist to plugin assets (production path) =="
# C5 (spec §2.2): production runtime extracts dist from plugin-openlist APK assets/
# (instead of relying on gomobile's //go:embed of public/dist into libgojni.so).
# This makes frontend updates patchable without rebuilding the AAR.
PLUGIN_ASSETS_DIR="${ENCV_GO_ROOT}/app/encv-mobile/plugin-openlist/src/main/assets"
if [[ -d "${DIST_DIR}" && -f "${DIST_DIR}/index.html" ]]; then
  PLUGIN_DIST="${PLUGIN_ASSETS_DIR}/dist"
  log "  source: ${DIST_DIR}"
  log "  target: ${PLUGIN_DIST}"
  rm -rf "${PLUGIN_DIST}"
  mkdir -p "${PLUGIN_DIST}"
  cp -a "${DIST_DIR}/." "${PLUGIN_DIST}/"
  log "  done: $(du -sh "${PLUGIN_DIST}" | cut -f1) copied to ${PLUGIN_DIST}"
else
  log "  (no frontend dist available, skipping plugin assets copy)"
fi

log "== Done =="
log "  AAR  : ${OUTPUT}/openlist.aar"
log "  SIZE : $(du -h "${OUTPUT}/openlist.aar" | cut -f1)"
