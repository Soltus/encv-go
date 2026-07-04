#!/usr/bin/env bash
# =============================================================================
# ci-check-no-nodejs-crypto.sh
# -----------------------------------------------------------------------------
# 硬性 lint：禁止在主源码树中用 Node.js crypto 加密 API Key。
#
# 背景（历史踩坑，2025-Q1 → 2026-Q2 移除）：
#   ① 历史上 scripts/agent-stub.js 曾用 node:crypto 的 scryptSync + AES-256-CBC
#      加密 OpenAI API Key。该文件已删除，但本 lint 保留作为**未来回归守卫**。
#   ② Node.js crypto 在手机 WebView / Capacitor 容器 / 纯前端环境下不可移植：
#      scryptSync 同步阻塞 WebView 主线程、固定 salt 易被反推、AES-CBC 格式
#      与 Go 端 AES-GCM 不兼容（跨端解密必然失败）。
#   ③ 因此本脚本检测到下列任一模式即 exit 1，CI 红灯。
#
# 检测模式（仅扫描 JS/TS/Vue 主源码，不查 Go）：
#   scryptSync                       同步密钥派生
#   \.scrypt(                        异步 scrypt 派生
#   pbkdf2Sync                       pbkdf2 派生
#   createCipheriv / createDecipheriv     AES primitives
#   createCipher( / createDecipher(       旧版 API
#   crypto\.createCipher\b / crypto\.createDecipher\b   旧版 API
#   \bencryptApiKey\b / \bdecryptApiKey\b                历史 agent-stub.js 函数名（仍保留检测，防回归）
#   node:crypto                      Node.js 内置 crypto 模块导入
#
# 修复指引（一旦红灯）：
#   ✅ 全部加解密走 Go 主后端：
#        internal/server/agent_api.go::EncryptApiKey(plaintext, deviceId...)
#        internal/server/agent_api.go::DecryptApiKey(stored,    deviceId...)
#      端点（已挂在 encv-go :2025，被 preview-gateway /agent-api 路由）：
#        POST /api/encrypt-key   {key, deviceId} → {encrypted}
#        POST /api/decrypt-key   {encrypted, deviceId} → {key}
#   ❌ 不要在 scripts/*.js、app/**/*.vue、preview-gateway 等里调 node:crypto。
#
# 本地运行：
#   bash scripts/ci-check-no-nodejs-crypto.sh
# =============================================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT" || exit 2

# 颜色（无 TTY 时自动降级）
if [ -t 1 ]; then
  R='\033[1;31m'; G='\033[1;32m'; Y='\033[1;33m'; B='\033[1;36m'; N='\033[0m'
else
  R=''; G=''; Y=''; B=''; N=''
fi
err()  { printf "${R}[FAIL]${N} %s\n" "$*" >&2; }
warn() { printf "${Y}[WARN]${N} %s\n" "$*" >&2; }
ok()   { printf "${G}[ OK ]${N} %s\n" "$*"; }
hdr()  { printf "${B}───${N} %s\n" "$*"; }

# ─── 1. 校验 grep 可用 ────────────────────────────────────────
if ! command -v grep >/dev/null 2>&1; then
  err "grep 未安装，无法运行本 lint"
  exit 2
fi

# ─── 2. 扫描范围 ──────────────────────────────────────────────
# 仅扫描主源码目录：scripts / app / agent / internal
# 排除：依赖、产物、git、AI agent skill 打包产物、PM2 dump 等。
SCAN_DIRS=(scripts app agent internal)

EXCLUDE_DIRS=(
  --exclude-dir=node_modules
  --exclude-dir=dist
  --exclude-dir=coverage
  --exclude-dir=.git
  --exclude-dir=.pm2
  --exclude-dir=.venv
  --exclude-dir=__pycache__
  --exclude-dir=vendor
  # AI agent skill 打包产物（.agents/skills/ 与 .trae/skills/ 下的 .mjs/.cjs bundle），
  # 内部引用 node:crypto 用于生成 trace ID、createHash 做 URL hash 等，
  # 属于 dev tool 自身能力，与 API Key 加密无关 → 排除。
  --exclude-dir=.agents
  --exclude-dir='skills'
)

INCLUDE_FILES=(
  --include='*.js'
  --include='*.cjs'
  --include='*.mjs'
  --include='*.ts'
  --include='*.tsx'
  --include='*.vue'
)

# ─── 3. 禁用模式表（按"风险类型"分组） ────────────────────────
# 每条模式 = 一行 grep -E 正则
PATTERNS=(
  # 模式 A：Node.js crypto 模块导入（历史 agent-stub.js 风格最显眼的标记）
  'node:crypto'

  # 模式 B：密钥派生
  'scryptSync'
  '\.scrypt\('
  'pbkdf2Sync'

  # 模式 C：AES primitives（与 Go 端 AES-GCM 不兼容的关键证据）
  'createCipheriv'
  'createDecipheriv'

  # 模式 D：旧版 AES API（Node 10+ 已废弃，但仍被 agent-stub 之类旧代码使用）
  'createCipher\('
  'createDecipher\('
  'crypto\.createCipher\b'
  'crypto\.createDecipher\b'

  # 模式 E：历史 agent-stub.js 专用函数名（最强信号，与 Node.js crypto 1:1 关联）
  '\bencryptApiKey\b'
  '\bdecryptApiKey\b'
)

# ─── 4. 扫描主循环 ────────────────────────────────────────────
FOUND=0
HITS_TOTAL=0

hdr "扫描范围: ${SCAN_DIRS[*]}"
hdr "排除目录: node_modules dist coverage .git .pm2 .venv vendor .agents skills"
hdr "禁用模式: ${#PATTERNS[@]} 条"

for pat in "${PATTERNS[@]}"; do
  # shellcheck disable=SC2086
  matches=$(grep -rEn \
    "${EXCLUDE_DIRS[@]}" \
    "${INCLUDE_FILES[@]}" \
    -e "$pat" \
    "${SCAN_DIRS[@]}" 2>/dev/null || true)

  if [ -n "$matches" ]; then
    # 统计行数
    hit_count=$(printf '%s\n' "$matches" | wc -l | tr -d ' ')
    HITS_TOTAL=$((HITS_TOTAL + hit_count))
    FOUND=1
    err "模式 '$pat' 命中 $hit_count 处："
    printf '%s\n' "$matches" | sed 's/^/    /' >&2
    echo "" >&2
  fi
done

# ─── 5. 输出结果 ──────────────────────────────────────────────
if [ "$FOUND" -ne 0 ]; then
  err "═══════════════════════════════════════════════════════════════"
  err " ❌  发现 Node.js crypto 加密 API Key 用法 — 禁止！"
  err "═══════════════════════════════════════════════════════════════"
  err ""
  err " 命中总数: ${HITS_TOTAL} 处"
  err ""
  err " 为什么禁止："
  err "   • mobile app / WebView / Capacitor 容器内无 Node.js 运行时"
  err "   • scryptSync 同步阻塞会卡死前端主线程"
  err "   • 固定 salt + AES-CBC 格式与 Go 端 AES-GCM 不兼容 → 跨端解密必然失败"
  err ""
  err " ✅ 正确做法：所有加解密走 Go 主后端（encv-go :2025）"
  err "   internal/server/agent_api.go"
  err "     EncryptApiKey(plaintext string, deviceId ...string) string"
  err "     DecryptApiKey(stored    string, deviceId ...string) string"
  err ""
  err " 端点（已被 preview-gateway /agent-api 路由到 encv-go）："
  err "   POST /api/encrypt-key   {key, deviceId} → {encrypted}"
  err "   POST /api/decrypt-key   {encrypted, deviceId} → {key}"
  err ""
  err " 删除或重写命中文件中的 Node.js crypto 引用后重跑本脚本即可。"
  err "═══════════════════════════════════════════════════════════════"
  exit 1
fi

ok "未发现 Node.js crypto 加密 API Key 用法（${#PATTERNS[@]} 条模式全过）"
exit 0
