#!/usr/bin/env bash
# =============================================================================
# dev-openlist.sh
# -----------------------------------------------------------------------------
# 一键启动 OpenList(5244) dev 模式：让 encv-mobile 在浏览器里跑通完整 stack。
#
# 不依赖 fork Go 代码改动（Hi-Sillot 已有 cobra + conf.Conf.DistDir 切换）。
#
# 用法：
#   bash scripts/dev-openlist.sh                     # 默认 fork 路径 + 默认端口
#   bash scripts/dev-openlist.sh --port 5245         # 自定义端口
#   bash scripts/dev-openlist.sh --data /tmp/odata   # 自定义 data 目录
#   bash scripts/dev-openlist.sh --fork /opt/OpenList  # 自定义 fork checkout
#   bash scripts/dev-openlist.sh --frontend-version 4.1.8  # 重下 dist
#
# 配合 Vite 沙箱 dev 模式（cv-mobile 端）：
#   Terminal 1: bash scripts/dev-openlist.sh
#   Terminal 2: pnpm dev
#   Browser:    http://localhost:8100/openlist-ui/
# =============================================================================

set -euo pipefail

# ---- 路径与默认配置 ----
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MOBILE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${MOBILE_DIR}/../.." && pwd)"

PORT="${OPENLIST_PORT:-5244}"
DATA_DIR="${OPENLIST_DATA:-$(pwd)/data}"
FORK_DIR="${OPENLIST_FORK:-${REPO_ROOT}/app/openlist/Hi-Sillot-OpenList}"
OPENLIST_VERSION="${OPENLIST_VERSION:-4.1.8}"
WEB_VERSION="${OPENLIST_WEB_VERSION:-v${OPENLIST_VERSION}}"
FRONTEND_TARBALL_URL="https://github.com/OpenListTeam/OpenList-Frontend/releases/download/${WEB_VERSION}/openlist-frontend-dist-${OPENLIST_VERSION}.tar.gz"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --port               <port>    HTTP port                       (default: 5244)
  --data               <dir>     Data directory (SQLite + config) (default: ./data)
  --fork               <dir>     Hi-Sillot fork checkout path    (default: \${REPO_ROOT}/app/openlist/Hi-Sillot-OpenList)
  --frontend-version   <vX.Y.Z>  Re-download frontend dist version  (default: ${OPENLIST_VERSION})
  --no-config          Skip writing config.json (use existing)
  -h, --help                      Show this help

Environment:
  OPENLIST_PORT          Override --port
  OPENLIST_DATA          Override --data
  OPENLIST_FORK          Override --fork
  OPENLIST_VERSION       Override --frontend-version
  OPENLIST_WEB_VERSION   Override web release tag (default: v\${OPENLIST_VERSION})

Examples:
  bash scripts/dev-openlist.sh --port 5244 --data ./openlist-data
  bash scripts/dev-openlist.sh --frontend-version 4.1.8
EOF
}

NO_CONFIG=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --port)               PORT="$2"; shift 2 ;;
    --data)               DATA_DIR="$2"; shift 2 ;;
    --fork)               FORK_DIR="$2"; shift 2 ;;
    --frontend-version)   OPENLIST_VERSION="$2"; WEB_VERSION="v${2}"; FRONTEND_TARBALL_URL="https://github.com/OpenListTeam/OpenList-Frontend/releases/download/v${2}/openlist-frontend-dist-${2}.tar.gz"; shift 2 ;;
    --no-config)          NO_CONFIG=1; shift ;;
    -h|--help)            usage; exit 0 ;;
    *) echo "ERROR: unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

log() { printf '\033[1;36m[dev-openlist]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[dev-openlist]\033[0m %s\n' "$*" >&2; }

# ---- 前置检查 ----
if ! command -v go >/dev/null 2>&1; then
  err "go 命令未找到。请安装 Go 1.21+ 后重试。"
  err "沙箱环境无 Go 工具链时，本脚本会失败；本地用户请在自己的机器上跑。"
  exit 1
fi

if [[ ! -d "${FORK_DIR}" ]]; then
  err "fork 目录不存在: ${FORK_DIR}"
  err "请先 clone："
  err "  git clone --branch dev --depth 1 https://github.com/Hi-Sillot/OpenList.git ${FORK_DIR}"
  exit 1
fi

if [[ ! -d "${FORK_DIR}/cmd" ]] || [[ ! -f "${FORK_DIR}/main.go" ]]; then
  err "fork 目录结构异常（缺 main.go 或 cmd/）：${FORK_DIR}"
  exit 1
fi

# ---- Step 1: 进入 fork 目录 ----
cd "${FORK_DIR}"
log "Working dir: $(pwd)"

# ---- Step 2: 确保 data 目录 ----
mkdir -p "${DATA_DIR}"
log "Data dir: ${DATA_DIR}"

# ---- Step 3: 确保 public/dist 存在（dev 模式的核心资产） ----
# 优先使用本地构建的 dist（来自 Hi-Sillot-OpenList-Frontend）→ 真正的热更新工作流
# fallback：下载 OpenListTeam/OpenList-Frontend release tarball
NEED_DOWNLOAD=0
LOCAL_FRONTEND_DIR="${REPO_ROOT}/app/openlist/Hi-Sillot-OpenList-Frontend"

if [[ -d "${LOCAL_FRONTEND_DIR}/dist" && -f "${LOCAL_FRONTEND_DIR}/dist/index.html" ]]; then
  log "使用本地构建的 dist（来自 Hi-Sillot-OpenList-Frontend）"
  log "  source: ${LOCAL_FRONTEND_DIR}/dist"
  rm -rf public/dist
  mkdir -p public/dist
  cp -a "${LOCAL_FRONTEND_DIR}/dist/." public/dist/
  # 用 Hi-Sillot-OpenList-Frontend 的 package.json 版本作为 VERSION 标记
  LOCAL_VERSION="v$(grep -oE '"version"[[:space:]]*:[[:space:]]*"[^"]+"' "${LOCAL_FRONTEND_DIR}/package.json" | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')"
  echo "${LOCAL_VERSION:-local}-encv" > public/dist/VERSION
  log "  done: $(du -sh public/dist/ | cut -f1) ($(cat public/dist/VERSION))"
elif [[ ! -f "public/dist/index.html" ]]; then
  log "public/dist/ 不存在，准备下载 OpenList-Frontend ${WEB_VERSION} ..."
  NEED_DOWNLOAD=1
elif [[ "${OPENLIST_VERSION}" != "4.1.8" ]] && [[ -n "${OPENLIST_VERSION:-}" ]]; then
  log "前端版本被指定为 ${OPENLIST_VERSION}（非默认 4.1.8），重新下载..."
  NEED_DOWNLOAD=1
fi

if [[ "${NEED_DOWNLOAD}" -eq 1 ]]; then
  if ! command -v curl >/dev/null 2>&1; then
    err "curl 未找到，无法下载 frontend dist"
    err "提示：也可以手动从 ${LOCAL_FRONTEND_DIR} 跑 'bun run build' 后重跑"
    exit 1
  fi
  if ! command -v tar >/dev/null 2>&1; then
    err "tar 未找到，无法解压 frontend dist"
    exit 1
  fi

  TMP="$(mktemp -d)"
  log "下载 ${FRONTEND_TARBALL_URL}"
  if ! curl -fsSL --retry 3 -o "${TMP}/frontend.tar.gz" "${FRONTEND_TARBALL_URL}"; then
    err "下载失败：${FRONTEND_TARBALL_URL}"
    err "检查：网络/版本号/防火墙"
    rm -rf "${TMP}"
    exit 1
  fi

  log "解压 frontend dist"
  tar -xzf "${TMP}/frontend.tar.gz" -C "${TMP}"

  # 替换 dist（保留 VERSION 标记）
  rm -rf public/dist
  mkdir -p public/dist
  if [[ -d "${TMP}/dist" ]]; then
    # tar 包内是 dist/ 目录
    cp -r "${TMP}/dist/." public/dist/
  else
    # tar 包内是散文件（少数 release 的格式）
    cp -r "${TMP}/." public/dist/
  fi
  echo "${WEB_VERSION}-encv" > public/dist/VERSION
  log "已写入 public/dist/ ($(du -sh public/dist/ | cut -f1))"
  rm -rf "${TMP}"
else
  log "public/dist/ 已存在 ($(du -sh public/dist/ | cut -f1))，跳过下载"
fi

# ---- Step 4: 写 config.json 启用 dist_dir + 端口 ----
# OpenList 启动时默认从 ${DATA_DIR}/config.json 读配置（也可显式 --config 指定）
# 我们写一份到 data dir，用绝对路径避免相对路径解析问题
if [[ "${NO_CONFIG}" -eq 0 ]]; then
  CONFIG_FILE="${DATA_DIR}/config.json"
  ABS_DIST_DIR="$(cd "${FORK_DIR}/public/dist" && pwd)"
  log "写入 ${CONFIG_FILE} (dist_dir=${ABS_DIST_DIR}, http_port=${PORT})"
  cat > "${CONFIG_FILE}" <<EOF
{
  "dist_dir": "${ABS_DIST_DIR}",
  "scheme": {
    "address": "0.0.0.0",
    "http_port": ${PORT},
    "https_port": -1,
    "force_https": false,
    "cert_file": "",
    "key_file": ""
  }
}
EOF
else
  log "--no-config：跳过 config.json 写入"
fi

# ---- Step 5: 启动 OpenList ----
log "启动 OpenList (port ${PORT}, data ${DATA_DIR})"
log "Ctrl+C 停止"
log "浏览器直访：http://127.0.0.1:${PORT}/  （或经由 Vite: http://localhost:8100/openlist-ui/）"
log "  (说明) 首次启动会用 go build 编译 OpenList，需要 30-60s；后续增量编译只需 2-5s"

# go run 模式下 go 会利用 build cache（${HOME}/.cache/go-build），增量编译很快
exec go run . server --data "${DATA_DIR}"
