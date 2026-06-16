#!/usr/bin/env bash
# scripts/create-ci-labels.sh
# =====================================================
# 一次性创建 CI 触发标签 (ci:full / ci:e2e / ci:skip)
# 配合 ci-workflow 改造使用 — 仓库初始化时跑一次
#
# 标签语义（详见 .trae/rules/ci-workflow.md §1.3）：
#   ci:full  → 触发 full-regression.yml (Layer 2 全包 regression)
#   ci:e2e   → 触发 e2e-integration.yml (Layer 3 加密 roundtrip E2E)
#   ci:skip  → 预留：maintainer 显式跳过 CI（暂未实现 workflow 端逻辑）
#
# 颜色编码（沿用 GitHub label 调色板）：
#   绿 #0E8A16 = OK
#   黄 #FBCA04 = 注意
#   红 #D93F0B = 警告
#
# 用法：
#   bash scripts/create-ci-labels.sh                 # 用 gh CLI 默认 repo
#   GITHUB_REPOSITORY=owner/repo bash scripts/create-ci-labels.sh  # 指定 repo
#   gh auth login 必装 + 仓库 admin 权限
# =====================================================

set -euo pipefail

# 解析 repo（优先 GITHUB_REPOSITORY env，否则从 gh CLI 当前仓库读）
REPO="${GITHUB_REPOSITORY:-}"
if [ -z "$REPO" ]; then
  if command -v gh >/dev/null 2>&1; then
    REPO=$(gh repo view --json nameWithOwner --jq '.nameWithOwner' 2>/dev/null || echo "")
  fi
fi

if [ -z "$REPO" ]; then
  echo "❌ No repo detected. Set GITHUB_REPOSITORY=owner/repo or run from a git repo with gh auth."
  exit 1
fi

echo "Repo: $REPO"
echo ""

# 工具守卫
if ! command -v gh >/dev/null 2>&1; then
  echo "❌ gh CLI not found. Install: https://cli.github.com/"
  exit 1
fi

# ── 工具函数：create-if-missing ──
create_label_if_missing() {
  local name="$1"
  local color="$2"
  local desc="$3"
  if gh label list --repo "$REPO" --json name --jq '.[].name' 2>/dev/null | grep -qx "$name"; then
    echo "✅ $name already exists"
    return 0
  fi
  echo "  Creating $name ..."
  if gh label create "$name" --repo "$REPO" --color "$color" --description "$desc" >/dev/null 2>&1; then
    echo "  ✅ Created $name"
  else
    echo "  ❌ Failed to create $name (need admin scope)"
    return 1
  fi
}

# ── 创建 3 个 CI 标签 ──
create_label_if_missing \
  "ci:full" \
  "0E8A16" \
  "Run full regression suite (Layer 2: go full test + frontend coverage)"

create_label_if_missing \
  "ci:e2e" \
  "FBCA04" \
  "Run E2E integration tests (Layer 3: encryption roundtrip)"

create_label_if_missing \
  "ci:skip" \
  "D93F0B" \
  "Skip CI checks (maintainer override, use with caution)"

echo ""
echo "✅ All CI labels ready"
echo ""
echo "用法："
echo "  # 给 PR 加 ci:full 标签（触发 Layer 2）"
echo "  gh pr edit <PR-number> --add-label ci:full --repo $REPO"
echo ""
echo "  # 同时加 ci:full + ci:e2e（重大变更，触发 Layer 2 + Layer 3）"
echo "  gh pr edit <PR-number> --add-label ci:full,ci:e2e --repo $REPO"
echo ""
echo "  # 列出当前所有 ci:* 标签"
echo "  gh label list --repo $REPO --search 'ci:'"
