#!/usr/bin/env python3
"""
全面修复 _ 前缀变量问题。

策略：
1. 扫描所有 .vue 文件
2. 在 script 部分找出所有以 _ 开头的变量/函数定义（const _xxx = / function _xxx(）
3. 检查模板中是否有对应的不带 _ 前缀的引用（xxx）
4. 如果有，就把 script 中的 _ 前缀移除
"""
import re
from pathlib import Path

PROJECT_DIR = Path("/workspace/app/encv-mobile/src")
SHARED_DIR = Path("/workspace/app/packages/shared-components/src")

def find_all_vue_files():
    files = list(PROJECT_DIR.glob("**/*.vue"))
    files += list(SHARED_DIR.glob("**/*.vue"))
    return files

def extract_script_vars(script_content: str) -> list[str]:
    """从 script 中提取所有以 _ 开头的变量/函数名。"""
    names = set()

    # const _xxx = / let _xxx = / var _xxx =
    for m in re.finditer(r'\b(?:const|let|var)\s+(_[a-zA-Z_][a-zA-Z0-9_]*)\s*[=:]', script_content):
        names.add(m.group(1))

    # function _xxx(
    for m in re.finditer(r'\bfunction\s+(_[a-zA-Z_][a-zA-Z0-9_]*)\s*\(', script_content):
        names.add(m.group(1))

    return sorted(names)

def check_template_uses(template_content: str, name_without_prefix: str) -> bool:
    """检查模板中是否使用了某个变量名。"""
    # 简单检查：变量名作为独立单词出现
    pattern = r'\b' + re.escape(name_without_prefix) + r'\b'
    return bool(re.search(pattern, template_content))

def fix_file(filepath: Path):
    """修复单个文件。"""
    content = filepath.read_text(encoding="utf-8")

    # 分离 template 和 script
    # 注意：根级别的 <template> 和 <script> 在顶层，不在其他元素内部
    # 简单做法：找到第一个 <template> 和最后一个 </template>
    template_start = content.find("<template>")
    if template_start == -1:
        return 0
    # 找对应的 </template> — 根级别的，不是内部 v-for/v-if 的
    # 简单做法：从文件末尾找 </template>
    template_end = content.rfind("</template>")
    if template_end == -1:
        return 0

    script_match = re.search(r'<script[^>]*>(.*?)</script>', content, re.DOTALL)

    if not script_match:
        return 0

    template_content = content[template_start:template_end + len("</template>")]
    script_content = script_match.group(1)
    script_start = script_match.start(1)
    script_end = script_match.end(1)

    # 找出 script 中所有 _ 开头的变量
    underscore_vars = extract_script_vars(script_content)

    fixed_count = 0
    new_script = script_content

    for var in underscore_vars:
        name_without = var[1:]  # 移除 _ 前缀
        # 检查模板中是否使用了不带前缀的版本
        if check_template_uses(template_content, name_without):
            # 替换变量定义处的前缀
            # 注意：只替换定义，不替换所有引用（因为内部引用可能是故意的）
            # 实际上，我们需要把整个 script 中的 _var 替换为 var
            # 但要小心不要误替换字符串中的内容

            # 安全替换：作为标识符的 _var → var
            pattern = r'\b' + re.escape(var) + r'\b'
            new_script = re.sub(pattern, name_without, new_script)
            fixed_count += 1

    if fixed_count > 0:
        new_content = content[:script_start] + new_script + content[script_end:]
        filepath.write_text(new_content, encoding="utf-8")

    return fixed_count

def main():
    files = find_all_vue_files()
    print(f"扫描 {len(files)} 个 .vue 文件...")

    total_fixed = 0
    total_files = 0

    for f in sorted(files):
        n = fix_file(f)
        if n > 0:
            print(f"  ✓ {f.name}: 修复 {n} 个 _ 前缀变量")
            total_fixed += n
            total_files += 1

    print(f"\n总计：修复 {total_files} 个文件，{total_fixed} 个变量")

if __name__ == "__main__":
    main()
