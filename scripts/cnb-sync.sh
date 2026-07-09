#!/bin/bash
# ============================================================
# cnb-sync.sh — 通用 cnb.cool ↔ GitHub 双向同步脚本
#
# 用法:
#   ./cnb-sync.sh sync          # 双向同步
#   ./cnb-sync.sh push          # 单向推送 (cnb→GitHub)
#   ./cnb-sync.sh pull          # 单向拉取 (GitHub→cnb)
#   ./cnb-sync.sh status        # 查看同步状态
#
# 环境变量 (必填):
#   GITHUB_TOKEN  — GitHub PAT (repo 权限)
#   CNB_REPO_SLUG — cnb 仓库路径（如 sc.hwd/sono）
#
# 环境变量 (可选):
#   GITHUB_USER   — GitHub 用户名（默认同 cnb owner）
#   SYNC_BRANCHES — 要同步的分支列表，空格分隔（默认: 所有分支）
#   DRY_RUN       — true=预览不执行
#   FORCE         — true=强制覆盖推送
#   GITHUB_REPO_OVERRIDE — 覆盖默认 GitHub 仓库（格式: owner/repo）
# ============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCK_FILE="${SCRIPT_DIR}/.sync.lock"

GITHUB_TOKEN="${GITHUB_TOKEN:?❌ 请设置 GITHUB_TOKEN}"
CNB_REPO_SLUG="${CNB_REPO_SLUG:?❌ 请设置 CNB_REPO_SLUG}"

# 默认 GitHub 仓库（与 cnb 同名）
# 可通过 GITHUB_USER 环境变量覆盖
# 也可通过按钮参数 github_repo 覆盖（格式: owner/repo）
GITHUB_USER="${GITHUB_USER:-$(echo "$CNB_REPO_SLUG" | cut -d/ -f1)}"
GITHUB_REPO="https://oauth2:${GITHUB_TOKEN}@github.com/${GITHUB_USER}/$(echo "$CNB_REPO_SLUG" | cut -d/ -f2).git"
GITHUB_REMOTE="github"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }
error() { log "❌ ERROR: $*"; exit 1; }
info() { log "ℹ️  INFO: $*"; }
ok() { log "✅ $*"; }

acquire_lock() {
  if [ -f "$LOCK_FILE" ]; then
    local pid
    pid=$(cat "$LOCK_FILE" 2>/dev/null)
    if kill -0 "$pid" 2>/dev/null; then
      error "同步已在进行中 (PID: $pid)。请先等待完成或手动删除 $LOCK_FILE"
    fi
    rm -f "$LOCK_FILE"
    info "清理过期锁文件"
  fi
  echo $$ > "$LOCK_FILE"
  trap 'rm -f "$LOCK_FILE"' EXIT
}

# 获取目标 GitHub 仓库 URL（支持按钮参数覆盖）
get_github_repo_url() {
  local CUSTOM_REPO="${GITHUB_REPO_OVERRIDE:-}"
  if [ -n "$CUSTOM_REPO" ]; then
    local OWNER REPO
    OWNER="$(echo "$CUSTOM_REPO" | cut -d/ -f1)"
    REPO="$(echo "$CUSTOM_REPO" | cut -d/ -f2)"
    echo "https://oauth2:${GITHUB_TOKEN}@github.com/${OWNER}/${REPO}.git"
  else
    echo "$GITHUB_REPO"
  fi
}

# 添加/更新 GitHub remote
setup_remote() {
  local TARGET_URL="${1:-$GITHUB_REPO}"
  if git remote get-url "$GITHUB_REMOTE" >/dev/null 2>&1; then
    info "更新已有 remote: $GITHUB_REMOTE"
    git remote set-url "$GITHUB_REMOTE" "$TARGET_URL"
  else
    info "添加新 remote: $GITHUB_REMOTE"
    git remote add "$GITHUB_REMOTE" "$TARGET_URL"
  fi
}

get_branches() {
  if [ -n "${SYNC_BRANCHES:-}" ]; then
    echo "$SYNC_BRANCHES"
  else
    git branch --format='%(refname:short)'
  fi
}

# 单向推送: cnb → GitHub
do_push() {
  log "📤 开始推送: cnb(${CNB_REPO_SLUG}) → GitHub"
  
  local TARGET_URL
  TARGET_URL=$(get_github_repo_url)
  setup_remote "$TARGET_URL"
  
  local branches
  branches=$(get_branches)
  
  while IFS= read -r branch; do
    [ -z "$branch" ] && continue
    log "  推送分支: $branch"
    if [ "${DRY_RUN:-false}" = "true" ]; then
      info "  [DRY RUN] git push $GITHUB_REMOTE $branch"
    else
      local PUSH_FLAGS=""
      [ "${FORCE:-false}" = "true" ] && PUSH_FLAGS="--force"
      git push $PUSH_FLAGS "$GITHUB_REMOTE" "$branch" 2>&1 || error "分支 $branch 推送失败"
    fi
  done <<< "$branches"
  
  log "  推送所有标签..."
  if [ "${DRY_RUN:-false}" != "true" ]; then
    git push "$GITHUB_REMOTE" --tags 2>&1 || error "标签推送失败"
  fi
  
  ok "推送完成"
}

# 单向拉取: GitHub → cnb（拉取后推回 cnb 远端，使同步持久化）
do_pull() {
  log "📥 开始拉取: GitHub → cnb(${CNB_REPO_SLUG})"
  
  # cnb 远端（本地仓库克隆自 cnb，默认 remote 名为 origin）
  local CNB_REMOTE="origin"
  if ! git remote get-url "$CNB_REMOTE" >/dev/null 2>&1; then
    error "未找到 cnb 远端 ($CNB_REMOTE)，无法将拉取结果推回 cnb"
  fi
  
  local TARGET_URL
  TARGET_URL=$(get_github_repo_url)
  setup_remote "$TARGET_URL"
  
  # 先抓取全部远程引用，建立远程跟踪分支（供后续存在性校验与更新使用）
  log "  获取 GitHub 远程引用..."
  git fetch "$GITHUB_REMOTE" --prune 2>&1 || error "无法连接 GitHub 或 fetch 失败"
  
  local branches
  branches=$(get_branches)
  
  local current_branch=""
  current_branch=$(git symbolic-ref --short HEAD 2>/dev/null || echo "")
  
  # 记录实际被更新的本地分支，最后统一推回 cnb 远端
  local updated_branches=""
  
  while IFS= read -r branch; do
    [ -z "$branch" ] && continue
    log "  处理分支: $branch"
    
    # 1. 校验远程分支是否存在（不存在必须报错，不再静默视为成功）
    if ! git show-ref --verify --quiet "refs/remotes/${GITHUB_REMOTE}/${branch}"; then
      error "分支 '$branch' 在 GitHub 上不存在，拉取失败"
    fi
    
    if [ "${DRY_RUN:-false}" = "true" ]; then
      info "  [DRY RUN] 将把本地分支 $branch 更新为 ${GITHUB_REMOTE}/${branch} 并推送到 cnb 远端"
      continue
    fi
    
    # 2. 本地分支不存在则直接基于远程跟踪分支创建
    if ! git show-ref --verify --quiet "refs/heads/${branch}"; then
      git branch "$branch" "${GITHUB_REMOTE}/${branch}"
      ok "  ✅ 已创建本地分支 $branch"
      updated_branches="$updated_branches $branch"
      continue
    fi
    
    # 3. 将本地分支更新到与远程一致
    if [ "$branch" = "$current_branch" ]; then
      # 当前分支：通过快进合并更新工作区与引用
      if [ "${FORCE:-false}" = "true" ]; then
        git reset --hard "${GITHUB_REMOTE}/${branch}" 2>&1 \
          || error "分支 $branch 强制重置失败"
      else
        git merge --ff-only "${GITHUB_REMOTE}/${branch}" 2>&1 \
          || error "分支 $branch 合并失败（本地与远程存在分叉，可开启 FORCE 覆盖）"
      fi
    else
      # 非当前分支：直接快进更新本地引用（不会触碰工作区）
      if [ "${FORCE:-false}" = "true" ]; then
        git fetch "${GITHUB_REMOTE}" "+${branch}:${branch}" 2>&1 \
          || error "分支 $branch 强制更新失败"
      else
        git fetch "${GITHUB_REMOTE}" "${branch}:${branch}" 2>&1 \
          || error "分支 $branch 更新失败（非快进，可能存在分叉，可开启 FORCE）"
      fi
    fi
    ok "  ✅ 分支 $branch 已更新"
    updated_branches="$updated_branches $branch"
  done <<< "$branches"
  
  log "  拉取标签..."
  git fetch "$GITHUB_REMOTE" --tags 2>&1 || info "  ⚠️ 标签拉取失败"
  
  # 4. 将更新后的分支推回 cnb 远端，使 GitHub → cnb 的同步真正持久化
  if [ "${DRY_RUN:-false}" = "true" ]; then
    info "  [DRY RUN] 将把更新后的分支推送到 cnb 远端 ($CNB_REMOTE)"
  elif [ -n "$updated_branches" ]; then
    local PUSH_FLAGS=""
    [ "${FORCE:-false}" = "true" ] && PUSH_FLAGS="--force"
    log "  推送更新到 cnb 远端 ($CNB_REMOTE)..."
    for branch in $updated_branches; do
      git push $PUSH_FLAGS "$CNB_REMOTE" "$branch" 2>&1 \
        || error "分支 $branch 推送回 cnb 失败"
    done
    git push $PUSH_FLAGS "$CNB_REMOTE" --tags 2>&1 || info "  ⚠️ 标签推送回 cnb 失败"
  fi
  
  ok "拉取完成（已同步至 cnb 远端）"
}

# 双向同步
do_sync() {
  log "🔄 开始双向同步"
  do_push
  do_pull
  ok "双向同步完成"
}

# 查看同步状态
do_status() {
  log "📊 同步状态"
  local TARGET_URL
  TARGET_URL=$(get_github_repo_url)
  setup_remote "$TARGET_URL"
  
  local current_branch
  current_branch=$(git symbolic-ref --short HEAD)
  
  echo ""
  echo "  当前分支: $current_branch"
  echo "  cnb:      https://cnb.cool/${CNB_REPO_SLUG}"
  echo "  github:   https://github.com/${GITHUB_USER}/$(echo "$CNB_REPO_SLUG" | cut -d/ -f2)"
  echo ""
  
  log "  比较 $current_branch 分支差异..."
  if [ "${DRY_RUN:-false}" != "true" ]; then
    local cnb_sha github_sha
    cnb_sha=$(git rev-parse HEAD)
    github_sha=$(git ls-remote "$GITHUB_REMOTE" "refs/heads/$current_branch" | awk '{print $1}')
    
    if [ "$cnb_sha" = "$github_sha" ]; then
      ok "分支 $current_branch 已同步"
    elif [ -n "$github_sha" ]; then
      info "分支 $current_branch 有差异"
      echo "  cnb SHA:    $cnb_sha"
      echo "  github SHA: $github_sha"
    else
      info "GitHub 上尚无此分支"
    fi
  fi
}

# ---------- 主入口 ----------
acquire_lock

FORCE="${FORCE:-false}"
GITHUB_REPO_OVERRIDE="${GITHUB_REPO_OVERRIDE:-}"

case "${1:-push}" in
  sync)   do_sync ;;
  push)   do_push ;;
  pull)   do_pull ;;
  status) do_status ;;
  *)
    error "未知命令: $1
用法: $0 {sync|push|pull|status}"
    ;;
esac
