#!/usr/bin/env bash
# 预提交钩子：拒绝提交 ELF/Mach-O/PE 二进制
# 防止 2026 之前的「go build 产物误入 git」历史重演（见 .trae/rules/project_rules.md 「编译产物铁律」）
#
# 安装: cp scripts/git-hooks/check-no-binary.sh .git/hooks/pre-commit
#       chmod +x .git/hooks/pre-commit
#
# 紧急绕过: git commit --no-verify
set -e
staged=$(git diff --cached --name-only --diff-filter=ACMR 2>/dev/null || true)
forbidden='\.(elf|bin|exe|so|dll|dylib|o|a)$'
if echo "$staged" | grep -qE "$forbidden"; then
  echo "❌ 预提交钩子拒绝：检测到二进制文件"
  echo "$staged" | grep -E "$forbidden" | sed 's/^/  /'
  echo ""
  echo "可能原因：误把 go build 产物 / .a / .so / 编译产物 commit"
  echo "处理：git reset HEAD <file> 取消暂存，或确认 .gitignore 已覆盖该路径"
  echo "紧急绕过：git commit --no-verify"
  exit 1
fi
