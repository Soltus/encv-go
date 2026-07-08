#!/usr/bin/env bash
# =============================================================================
# setup-sandbox-env.sh
# -----------------------------------------------------------------------------
# 一键准备 ENCV / OpenList / encv-mobile 沙箱 dev 环境的**静态依赖**。
#
# 本脚本**只做环境准备**（装工具/clone 仓库/构建/装依赖），**不拉起任何服务**。
# 服务启动请用：
#   - bash scripts/start-preview.sh     (encv-go + Vite encv-mobile)
#   - bash scripts/dev-openlist.sh      (OpenList 真实 fork on 5244)
#   - bash scripts/dev-openlist-web.sh  (plugin-openlist Vite on 5174)
#
# 包含步骤：
#   0/6  前置检查（go / node / pnpm / git / curl / cmake）
#   1/6  装 Go 工具链 (air-verse/air live reload)
#   2/6  装 Kotlin 工具链 (运行 .trae/scripts/setup-kotlinc.sh)
#   3/7  clone OpenList 双 fork
#         - 后端: app/openlist/Hi-Sillot-OpenList/   (dev 分支)
#         - 前端: app/openlist/Hi-Sillot-OpenList-Frontend/ (main 分支)
#   3b/7 clone ComboLite fork (K-Sillot/ComboLite)
#   4/7  构建前端 fork 的 dist (Hi-Sillot-OpenList-Frontend/dist/)
#   5/6  pnpm install encv-mobile 主 app + plugin-openlist/web
#   6/6  构建 preview-gateway 网关（app/preview-gateway/）
#
# 退出码:
#   0  = 全部就绪
#   1  = 前置依赖缺失
#   2  = 网络/克隆失败
#   3  = pnpm install / fork build 失败
# =============================================================================
set -uo pipefail   # 不开 -e（pnpm install / go install 非零但仍可继续）

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MOBILE_DIR="${REPO_ROOT}/app/encv-mobile"
OPENLIST_ROOT="${REPO_ROOT}/app/openlist"
FRONTEND_FORK_DIR="${OPENLIST_ROOT}/Hi-Sillot-OpenList-Frontend"
BACKEND_FORK_DIR="${OPENLIST_ROOT}/Hi-Sillot-OpenList"

# ---- 颜色 ----
R='\033[1;31m'; G='\033[1;32m'; Y='\033[1;33m'; C='\033[1;36m'; N='\033[0m'
log()  { printf "${C}[setup]${N} %s\n" "$*"; }
ok()   { printf "${G}[setup]${N} %s\n" "$*"; }
warn() { printf "${Y}[setup]${N} %s\n" "$*" >&2; }
err()  { printf "${R}[setup]${N} %s\n" "$*" >&2; }
step() { echo ""; printf "${C}==>${N} %s\n" "$*"; }

FAILED=0

# ============================================================================
# 步骤 pre-0: 安装 Cypress / Electron 系统依赖（沙箱无 GUI 需 xvfb + GTK 库）
# ============================================================================
step "pre-0/6 安装 Cypress/Electron 系统依赖（xvfb + GTK + ATK）"

CYPRESS_DEPS=(
  xvfb
  libatk1.0-0
  libatk-bridge2.0-0
  libgtk-3-0
  libgbm1
  libxdamage1
  libxcomposite1
  libxcursor1
  libxi6
  libxtst6
  libxrandr2
  libxss1
  libasound2
  libpango-1.0-0
  libcairo2
  libdbus-1-3
  libdbus-glib-1-2
  libnss3
  libx11-xcb1
)

# 检查哪些缺
MISSING_DEPS=()
for pkg in "${CYPRESS_DEPS[@]}"; do
  if ! dpkg -l "$pkg" >/dev/null 2>&1; then
    MISSING_DEPS+=("$pkg")
  fi
done

if [[ ${#MISSING_DEPS[@]} -eq 0 ]]; then
  ok "Cypress/Electron 系统依赖全部就绪（${#CYPRESS_DEPS[@]} 个包）"
else
  log "缺失 ${#MISSING_DEPS[@]} 个包，apt-get install ..."
  log "  缺失: ${MISSING_DEPS[*]}"
  if apt-get update -qq 2>&1 | tail -3; then
    if DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${MISSING_DEPS[@]}" 2>&1 | tail -10; then
      ok "Cypress/Electron 系统依赖安装完成"
    else
      warn "Cypress 系统依赖安装失败（测试可能跑不起来）"
      FAILED=$((FAILED+1))
    fi
  else
    warn "apt-get update 失败（跳过 Cypress 系统依赖安装）"
    FAILED=$((FAILED+1))
  fi
fi

# ============================================================================
# 步骤 pre-0: 清理沙箱内残留 orphan 进程（必须）
# ============================================================================
# 沙箱特有：上次 preview 跑过留下 zombie air / go build / vite 进程
# 这些进程不抢 CPU 看起来无害，但：
#   1. go build 卡 Sl 状态会无限占着 /tmp/go-build-XXX 临时目录
#   2. preview-gateway / air 占着 :16666 / :2025 端口导致本脚本构建后无法启 pm2
#   3. 多 zombie 累积会让 go test / go build 性能下降
# 杀干净后才能进入正常的 install / build 流程
if [[ -x "${SCRIPT_DIR}/kill-orphan-children.sh" ]]; then
  log "清理沙箱 orphan 进程（kill-orphan-children.sh）..."
  bash "${SCRIPT_DIR}/kill-orphan-children.sh" || true
fi

# ============================================================================
# 步骤 0/5: 前置检查
# ============================================================================
step "0/5 前置检查（go / node / pnpm / git / curl / cmake / java）"
for cmd in go node npm pnpm git curl cmake java; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    err "❌ 缺少命令: $cmd"
    exit 1
  fi
done
ok "基础工具链全部就绪"
log "  go=$(go version)  node=$(node --version)  pnpm=$(pnpm --version)"

# pm2 兜底安装（previews.sh 强依赖，但不在系统基础包内）
if command -v pm2 >/dev/null 2>&1; then
  ok "pm2 已安装: $(command -v pm2)"
else
  log "pm2 缺失，npm i -g pm2 ..."
  if npm install -g pm2 --no-audit --no-fund 2>&1 | tail -5; then
    if command -v pm2 >/dev/null 2>&1; then
      ok "pm2 安装成功: $(command -v pm2)"
    else
      warn "npm i -g pm2 返回 0 但 pm2 仍不在 PATH（previews.sh 将无法运行）"
      FAILED=$((FAILED+1))
    fi
  else
    warn "pm2 安装失败（previews.sh 将无法运行）"
    FAILED=$((FAILED+1))
  fi
fi

# ============================================================================
# 步骤 1/5: 装 Go 工具链 (air)
# ============================================================================
step "1/6 装 Go 工具链（air-verse/air，用于 start-preview.sh live reload）"

# air 已迁移：github.com/cosmtrek/air → github.com/air-verse/air
if command -v air >/dev/null 2>&1; then
  ok "air 已安装: $(command -v air)"
else
  # 把 GOBIN 和 mise Go bin 都加到 PATH，找 air
  GOBIN="$(go env GOPATH)/bin"
  MISE_GO_BIN="/root/.local/share/mise/installs/go/1.25.1/bin"
  export PATH="${GOBIN}:${MISE_GO_BIN}:${PATH}"

  if command -v air >/dev/null 2>&1; then
    ok "air 在 PATH 中: $(command -v air)"
  else
    log "go install github.com/air-verse/air@latest ..."
    if go install github.com/air-verse/air@latest 2>&1 | tail -5; then
      if [[ -x "${GOBIN}/air" ]]; then
        ok "air 安装成功: ${GOBIN}/air"
      else
        warn "go install 返回 0 但 ${GOBIN}/air 不存在"
        FAILED=$((FAILED+1))
      fi
    else
      warn "air 安装失败（start-preview.sh 需要它）"
      FAILED=$((FAILED+1))
    fi
  fi
fi

# ============================================================================
# 步骤 2/5: 装 Kotlin 工具链
# ============================================================================
step "2/5 装 Kotlin 工具链（运行 .trae/scripts/setup-kotlinc.sh）"

if command -v kotlinc >/dev/null 2>&1; then
  ok "kotlinc 已安装: $(command -v kotlinc)"
else
  if [[ -x "${REPO_ROOT}/.trae/scripts/setup-kotlinc.sh" ]]; then
    log "运行 setup-kotlinc.sh ..."
    if bash "${REPO_ROOT}/.trae/scripts/setup-kotlinc.sh" 2>&1 | tail -8; then
      ok "kotlinc 安装完成"
    else
      warn "kotlinc 安装失败"
      FAILED=$((FAILED+1))
    fi
  else
    warn "未找到 .trae/scripts/setup-kotlinc.sh"
    FAILED=$((FAILED+1))
  fi
fi

# ============================================================================
# 步骤 3/5: clone OpenList 双 fork
# ============================================================================
step "3/5 clone OpenList 双 fork（后端 dev 分支 + 前端 main 分支）"

# 3a. 后端 fork
if [[ -d "${BACKEND_FORK_DIR}/.git" ]]; then
  ok "后端 fork 已存在: ${BACKEND_FORK_DIR}"
else
  log "git clone Hi-Sillot/OpenList (dev) ..."
  cd "${OPENLIST_ROOT}"
  if git clone --depth 1 --branch dev \
       https://github.com/Hi-Sillot/OpenList.git \
       Hi-Sillot-OpenList 2>&1 | tail -3; then
    ok "后端 fork clone 完成"
  else
    err "后端 fork clone 失败"
    FAILED=$((FAILED+1))
  fi
fi

# 3b. 前端 fork
if [[ -d "${FRONTEND_FORK_DIR}/.git" ]]; then
  ok "前端 fork 已存在: ${FRONTEND_FORK_DIR}"
else
  log "git clone Hi-Sillot/OpenList-Frontend (main) ..."
  cd "${OPENLIST_ROOT}"
  if git clone --depth 1 --branch main \
       https://github.com/Hi-Sillot/OpenList-Frontend.git \
       Hi-Sillot-OpenList-Frontend 2>&1 | tail -3; then
    ok "前端 fork clone 完成"
  else
    err "前端 fork clone 失败"
    FAILED=$((FAILED+1))
  fi
fi

# ============================================================================
# 步骤 3b/7: clone ComboLite fork (K-Sillot/ComboLite)
# ============================================================================
step "3b/7 clone ComboLite fork (K-Sillot/ComboLite)"

COMBOLITE_DIR="${REPO_ROOT}/app/combolite"
COMBOLITE_FORK_DIR="${COMBOLITE_DIR}/ComboLite"

if [[ -d "${COMBOLITE_FORK_DIR}/.git" ]]; then
  ok "ComboLite fork 已存在: ${COMBOLITE_FORK_DIR}"
else
  log "git clone K-Sillot/ComboLite ..."
  mkdir -p "${COMBOLITE_DIR}"
  cd "${COMBOLITE_DIR}"
  if git clone --depth 1 \
       https://github.com/K-Sillot/ComboLite.git \
       ComboLite 2>&1 | tail -3; then
    ok "ComboLite fork clone 完成"
  else
    warn "ComboLite fork clone 失败（不影响主构建，仅用于源码参考）"
  fi
fi

# ============================================================================
# 步骤 4/7: 构建前端 fork 的 dist
# ============================================================================
step "4/6 构建前端 fork 的 dist (Hi-Sillot-OpenList-Frontend/dist/)"

if [[ -f "${FRONTEND_FORK_DIR}/dist/index.html" ]]; then
  ok "前端 dist 已构建: ${FRONTEND_FORK_DIR}/dist/"
else
  if [[ ! -d "${FRONTEND_FORK_DIR}/node_modules" ]]; then
    log "pnpm install (前端 fork) ..."
    cd "${FRONTEND_FORK_DIR}"
    if pnpm install --prefer-offline 2>&1 | tail -5; then
      ok "前端 fork 依赖安装完成"
    else
      warn "前端 fork pnpm install 失败（dev-openlist.sh 会 fallback 到 release tarball）"
      FAILED=$((FAILED+1))
    fi
  fi
  if [[ ! -f "${FRONTEND_FORK_DIR}/dist/index.html" ]]; then
    log "构建前端 dist (pnpm build) ..."
    cd "${FRONTEND_FORK_DIR}"
    if pnpm build 2>&1 | tail -8; then
      if [[ -f "${FRONTEND_FORK_DIR}/dist/index.html" ]]; then
        ok "前端 dist 构建完成"
      else
        warn "构建退出 0 但 dist/index.html 不存在"
        FAILED=$((FAILED+1))
      fi
    else
      warn "前端 dist 构建失败（dev-openlist.sh 会 fallback 到 release tarball）"
      FAILED=$((FAILED+1))
    fi
  fi
fi

# ============================================================================
# 步骤 5/6: pnpm install encv-mobile
# ============================================================================
step "5/6 pnpm install encv-mobile + plugin-openlist/web + plugin-simverse/web"

# 5a. 主 app
if [[ -d "${MOBILE_DIR}/node_modules/vite" ]]; then
  ok "encv-mobile node_modules 已就绪"
else
  log "pnpm install encv-mobile ..."
  cd "${MOBILE_DIR}"
  if pnpm install --prefer-offline 2>&1 | tail -5; then
    ok "encv-mobile 依赖安装完成"
  else
    err "encv-mobile pnpm install 失败"
    FAILED=$((FAILED+1))
  fi
fi

# 5b. plugin web
if [[ -d "${MOBILE_DIR}/plugin-openlist/web/node_modules/vite" ]]; then
  ok "plugin-openlist/web node_modules 已就绪"
else
  log "pnpm install plugin-openlist/web ..."
  cd "${MOBILE_DIR}"
  if pnpm install --prefer-offline 2>&1 | tail -5; then
    ok "plugin-openlist/web 依赖安装完成"
  else
    warn "plugin-openlist/web pnpm install 失败（5174 Vite 跑不起来）"
    FAILED=$((FAILED+1))
  fi
fi

# 5c. plugin-simverse web
if [[ -d "${MOBILE_DIR}/plugin-simverse/web/node_modules/vite" ]]; then
  ok "plugin-simverse/web node_modules 已就绪"
else
  log "pnpm install plugin-simverse/web ..."
  cd "${MOBILE_DIR}"
  if pnpm install --prefer-offline 2>&1 | tail -5; then
    ok "plugin-simverse/web 依赖安装完成"
  else
    warn "plugin-simverse/web pnpm install 失败"
    FAILED=$((FAILED+1))
  fi
fi

# ============================================================================
# 步骤 6/6: 构建 preview-gateway 网关
# ============================================================================
GATEWAY_DIR="${REPO_ROOT}/app/preview-gateway"

step "6/6 构建 preview-gateway 网关（app/preview-gateway/）"

if [[ -d "${GATEWAY_DIR}/node_modules" ]]; then
  ok "preview-gateway node_modules 已就绪"
else
  log "pnpm install preview-gateway ..."
  cd "${GATEWAY_DIR}"
  if pnpm install --prefer-offline 2>&1 | tail -5; then
    ok "preview-gateway 依赖安装完成"
  else
    err "preview-gateway pnpm install 失败（网关跑不起来）"
    FAILED=$((FAILED+1))
  fi
fi

if [[ -f "${GATEWAY_DIR}/dist/server.js" ]]; then
  ok "preview-gateway dist/server.js 已构建"
else
  log "构建 preview-gateway (pnpm build) ..."
  cd "${GATEWAY_DIR}"
  if pnpm build 2>&1 | tail -8; then
    if [[ -f "${GATEWAY_DIR}/dist/server.js" ]]; then
      ok "preview-gateway 构建完成"
    else
      warn "构建退出 0 但 dist/server.js 不存在"
      FAILED=$((FAILED+1))
    fi
  else
    err "preview-gateway 构建失败（pm2 启动会失败）"
    FAILED=$((FAILED+1))
  fi
fi

# ============================================================================
# 环境就绪报告（只展示静态资源状态，不拉起任何服务）
# ============================================================================
step "✅ 环境准备完成"
cat <<EOF

========================================
📦 沙箱环境状态（静态资源）

工具链：
EOF
for cmd in go node pnpm pm2 git cmake java kotlinc air; do
  if command -v "$cmd" >/dev/null 2>&1; then
    ver=$("$cmd" --version 2>&1 | head -1 | sed 's/^/  /')
    echo "  ✅ $cmd $ver"
  else
    echo "  ❌ $cmd (missing)"
  fi
done

cat <<EOF

仓库：
EOF
for d in "${BACKEND_FORK_DIR}" "${FRONTEND_FORK_DIR}" "${MOBILE_DIR}/node_modules/vite" "${FRONTEND_FORK_DIR}/dist/index.html" "${GATEWAY_DIR}/dist/server.js" "${SIMVERSE_DIR}/node_modules/cypress"; do
  if [[ -e "$d" ]]; then
    if [[ -d "$d/.git" ]]; then
      branch=$(cd "$d" && git rev-parse --abbrev-ref HEAD 2>/dev/null)
      echo "  ✅ $d   (branch: $branch)"
    else
      echo "  ✅ $d"
    fi
  else
    echo "  ❌ $d   (missing)"
  fi
done

cat <<EOF

下一步（手动拉起服务，本脚本不负责）：
  bash scripts/previews.sh start
                → preview-gateway   on :16666  (统一对外预览入口)
                → start-preview     on :2025 + :8100  (encv-go + Vite encv-mobile)
                → openlist          on :5244  (OpenList Go fork)
                → plugin-openlist-vite on :5174  (Vite plugin 管理 UI)

  浏览器访问：http://localhost:16666/

  ⚠️ 首次访问 :16666 时，agent-tool-host 内部的 preview-proxy 会自动
     把 :16666 注册到 OpenPreview 外网白名单，之后才能用 OpenPreview 工具
     激活外网预览（agent-browser navigate :16666 触发自动注册）。

========================================
EOF

if [[ $FAILED -gt 0 ]]; then
  warn "⚠️  共 ${FAILED} 步非致命失败，请查看上方 WARN 行"
fi

# 报告前最后再清一次 orphan（install/build 过程中可能 spawn 子进程）
if [[ -x "${SCRIPT_DIR}/kill-orphan-children.sh" ]]; then
  bash "${SCRIPT_DIR}/kill-orphan-children.sh" --report || true
fi

ok "🎉 沙箱环境就绪"
