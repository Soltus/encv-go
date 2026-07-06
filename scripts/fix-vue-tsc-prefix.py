#!/usr/bin/env python3
"""
批量修复 Vue SFC 中错误添加的 _ 前缀变量。

问题：有人为了绕过 noUnusedLocals 检查，给模板中使用的变量加了 _ 前缀，
但忘记同步模板中的引用，导致：
1. TS2339: 模板引用的属性不存在
2. TS6133: _前缀变量声明但未使用
3. 运行时功能也会异常（模板访问不到变量）

修复：把 <script> 部分中 _ 前缀的变量定义恢复为无前缀形式。
"""
import re
import sys
import os
import subprocess
from pathlib import Path

PROJECT_DIR = Path("/workspace/app/encv-mobile")
SHARED_DIR = Path("/workspace/app/packages/shared-components")

def get_tsc_errors():
    """运行 vue-tsc 并收集错误。"""
    print("运行 vue-tsc 收集错误...")
    result = subprocess.run(
        ["pnpm", "exec", "vue-tsc", "--noEmit"],
        cwd=str(PROJECT_DIR),
        capture_output=True,
        text=True
    )
    errors = result.stdout + result.stderr
    return errors.split("\n")

def parse_errors(lines):
    """解析错误行，返回 {filepath: {var_names}} 映射。"""
    ts2339_pattern = re.compile(r"^([^(]+)\([0-9]+,[0-9]+\): error TS2339: Property '([^']+)' does not exist")
    ts6133_pattern = re.compile(r"^([^(]+)\([0-9]+,[0-9]+\): error TS6133: '([^']+)' is declared but its value is never read")

    # 收集 TS2339 中提到的属性名（模板中使用的名字）
    # 和 TS6133 中提到的 _ 前缀变量名（脚本中定义的名字）
    file_vars = {}  # filepath -> set of var names to fix (without _ prefix)

    for line in lines:
        # TS2339: 模板中使用 xxx，但组件上没有 xxx 属性
        m = ts2339_pattern.search(line)
        if m:
            fpath = m.group(1).strip()
            prop_name = m.group(2)
            if fpath not in file_vars:
                file_vars[fpath] = set()
            # 这个 prop_name 可能是模板中使用的名字
            # 对应的脚本定义应该是 _prop_name
            file_vars[fpath].add(prop_name)

    return file_vars

def fix_file(filepath: Path, var_names: set):
    """修复单个文件中的 _ 前缀变量。"""
    if not filepath.exists():
        return False

    content = filepath.read_text(encoding="utf-8")

    # 找到 <script ...> 部分
    script_start_match = re.search(r'<script[^>]*>', content)
    if not script_start_match:
        return False

    script_start = script_start_match.end()
    script_end_match = re.search(r'</script>', content[script_start:])
    if not script_end_match:
        return False

    script_end = script_start + script_end_match.start()

    before = content[:script_start]
    script_content = content[script_start:script_end]
    after = content[script_end:]

    modified = False
    new_script = script_content

    for var_name in var_names:
        if not var_name or var_name.startswith("_"):
            continue
        # 要把 _var_name 替换为 var_name
        underscore_name = "_" + var_name

        # 匹配变量定义：
        # const _xxx = / let _xxx = / var _xxx =
        # function _xxx( / async function _xxx(
        # 还要确保是完整单词匹配

        # 模式：关键字 + 空格 + _varName + 单词边界/符号
        patterns = [
            # const/let/var
            (r'\b(const|let|var)\s+_' + re.escape(var_name) + r'\b', r'\1 ' + var_name),
            # function/async function
            (r'\b(async\s+function|function)\s+_' + re.escape(var_name) + r'\b', r'\1 ' + var_name),
        ]

        for pattern, replacement in patterns:
            new_script, n = re.subn(pattern, replacement, new_script)
            if n > 0:
                modified = True

    if modified:
        new_content = before + new_script + after
        filepath.write_text(new_content, encoding="utf-8")
        return True
    return False

def main():
    error_lines = get_tsc_errors()
    file_vars = parse_errors(error_lines)

    print(f"发现 {len(file_vars)} 个文件需要修复")

    fixed_count = 0
    for fpath_str, var_names in sorted(file_vars.items()):
        # 处理路径
        fpath = Path(fpath_str)
        if not fpath.is_absolute():
            # 相对路径，尝试相对于 encv-mobile 和 shared-components 解析
            for base in [PROJECT_DIR, SHARED_DIR, Path("/workspace/app")]:
                candidate = base / fpath_str
                if candidate.exists():
                    fpath = candidate
                    break

        if not fpath.exists():
            print(f"  ⚠  文件不存在: {fpath_str}")
            continue

        # 过滤：只处理确定是 _ 前缀导致的问题
        # 验证：脚本中确实有 _xxx 变量
        content = fpath.read_text(encoding="utf-8")
        actual_vars = set()
        for var_name in var_names:
            if re.search(r'\b(const|let|var|function)\s+_' + re.escape(var_name) + r'\b', content):
                actual_vars.add(var_name)

        if not actual_vars:
            continue

        print(f"  ✓ {fpath.name}: {len(actual_vars)} 个变量")
        if fix_file(fpath, actual_vars):
            fixed_count += 1

    print(f"\n修复了 {fixed_count} 个文件")

    # 再运行一次 vue-tsc 看看还剩多少错误
    print("\n再次运行 vue-tsc 验证...")
    result = subprocess.run(
        ["pnpm", "exec", "vue-tsc", "--noEmit"],
        cwd=str(PROJECT_DIR),
        capture_output=True,
        text=True
    )
    error_count = len([l for l in (result.stdout + result.stderr).split("\n") if "error TS" in l])
    print(f"剩余错误数: {error_count}")

if __name__ == "__main__":
    main()
