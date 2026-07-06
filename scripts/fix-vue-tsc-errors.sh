#!/usr/bin/env bash
set -e

cd "$(dirname "$0")/../app/encv-mobile"

# Step 1: 收集所有 TS6133 的 _ 前缀变量
echo "收集需要修复的变量..."

# 从 vue-tsc 输出中提取信息
TMPFILE=$(mktemp)
pnpm exec vue-tsc --noEmit 2>&1 | grep -E "error TS(6133|2339):" > "$TMPFILE" || true

# 找出所有受影响的 .vue 文件
FILES=$(grep -E "error TS(6133|2339):" "$TMPFILE" | sed 's/([0-9]*,[0-9]*).*//' | sort -u)

echo "发现 $(echo "$FILES" | wc -l) 个受影响文件"

# 对每个文件进行修复
for file in $FILES; do
  [ -f "$file" ] || continue

  # 获取该文件的所有 TS6133: '_xxx' is declared but never read
  UNUSED_VARS=$(grep "$file" "$TMPFILE" | grep "TS6133" | grep -oP "'_\\K[^']+" | sort -u)

  if [ -z "$UNUSED_VARS" ]; then
    continue
  fi

  echo "修复: $file"
  echo "  变量: $(echo "$UNUSED_VARS" | tr '\n' ' ')"

  # 对每个变量，把脚本中的 _xxx 改为 xxx
  # 但要小心：只改 <script> 部分，不改模板
  # 并且只改变量定义，不改变量使用

  # 更简单的方法：在 script 部分，把 const _xxx / let _xxx / var _xxx / function _xxx 等的 _ 去掉
  # 用 python 来处理更可靠

  python3 << PYEOF
import re
import sys

filepath = "$file"
unused_vars = """$UNUSED_VARS""".strip().split('\n')

with open(filepath, 'r', encoding='utf-8') as f:
    content = f.read()

# 分离 template 和 script 部分
# 我们只修改 <script setup> 中的变量定义

# 找到 <script ...> 开始和结束位置
script_match = re.search(r'<script[^>]*>', content)
if not script_match:
    sys.exit(0)

script_start = script_match.end()
# 找 </script>
script_end_match = re.search(r'</script>', content[script_start:])
if not script_end_match:
    sys.exit(0)

script_end = script_start + script_end_match.start()

before_script = content[:script_start]
script_content = content[script_start:script_end]
after_script = content[script_end:]

# 对每个变量，替换定义处的 _xxx 为 xxx
# 匹配模式：
# const _varName = 
# let _varName = 
# var _varName = 
# function _varName(
# async function _varName(
# const _varName = computed(
# const _varName = ref(
# 等等

modified = script_content
for var_name in unused_vars:
    if not var_name:
        continue
    # 匹配变量定义：在 const/let/var/function/class/async function 后面的 _xxx
    # 用单词边界确保是完整变量名
    pattern = r'\b(const|let|var|function|class|async\s+function)\s+_' + re.escape(var_name) + r'\b'
    replacement = r'\1 ' + var_name
    modified = re.sub(pattern, replacement, modified)

    # 还要处理解构赋值中的 _xxx: xxx 形式？不，那是别名
    # 主要是直接定义的变量

# 写回
new_content = before_script + modified + after_script

if new_content != content:
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(new_content)
    print(f"  ✓ 已修复")
else:
    print(f"  ⚠ 未修改（可能需要手动处理）")

PYEOF

done

rm -f "$TMPFILE"

echo ""
echo "修复完成！"
