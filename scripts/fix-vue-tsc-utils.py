#!/usr/bin/env python3
"""批量给 Vue SFC 添加缺失的工具函数导入。"""
import re
import subprocess
from pathlib import Path

PROJECT_DIR = Path("/workspace/app/encv-mobile")

# 工具函数 -> 导入路径映射
UTIL_IMPORTS = {
    "formatFileSize": {
        "module": "@/api/encv_files",
        "is_type": False,
    },
    "formatDateTime": {
        "module": "@/composables/useDateFormat",
        "is_type": False,
    },
    "formatContainerVersion": {
        "module": "@/constants/containerVersion",
        "is_type": False,
    },
    "isImageFile": {
        "module": "@/composables/useFileList",
        "is_type": False,
    },
    "getFileIcon": {
        "module": "@/composables/useFileList",
        "is_type": False,
    },
    "getFileIconColor": {
        "module": "@/composables/useFileList",
        "is_type": False,
    },
}

def get_missing_utils():
    """从 vue-tsc 错误中提取每个文件缺失的工具函数。"""
    result = subprocess.run(
        ["pnpm", "exec", "vue-tsc", "--noEmit"],
        cwd=str(PROJECT_DIR),
        capture_output=True,
        text=True
    )
    lines = (result.stdout + result.stderr).split("\n")

    file_utils = {}  # filepath -> set of util names

    pattern = re.compile(r"^([^(]+)\([0-9]+,[0-9]+\): error TS(2339|2551): Property '([^']+)' does not exist")

    for line in lines:
        m = pattern.match(line.strip())
        if not m:
            continue
        fpath = m.group(1).strip()
        prop = m.group(3)
        if prop in UTIL_IMPORTS:
            if fpath not in file_utils:
                file_utils[fpath] = set()
            file_utils[fpath].add(prop)

    return file_utils

def add_imports(filepath: Path, utils: set):
    """给文件添加导入。"""
    if not filepath.exists():
        return False

    content = filepath.read_text(encoding="utf-8")

    # 按模块分组
    by_module = {}  # module -> set of names
    for util in utils:
        info = UTIL_IMPORTS[util]
        mod = info["module"]
        if mod not in by_module:
            by_module[mod] = set()
        by_module[mod].add(util)

    script_match = re.search(r"<script[^>]*>", content)
    if not script_match:
        return False

    insert_pos = script_match.end()
    added = 0

    for mod, names in by_module.items():
        # 检查是否已有该模块的导入
        import_pattern = re.compile(rf"import\s*\{{([^}}]*)\}}\s*from\s*['\"]{re.escape(mod)}['\"]")
        existing_match = import_pattern.search(content)

        sorted_names = sorted(names)

        if existing_match:
            existing_names = [n.strip() for n in existing_match.group(1).split(",") if n.strip()]
            new_names = [n for n in sorted_names if n not in existing_names]
            if not new_names:
                continue
            all_names = sorted(set(existing_names + new_names))
            new_import = "import { " + ", ".join(all_names) + " } from \"" + mod + "\";"
            content = content[:existing_match.start()] + new_import + content[existing_match.end():]
        else:
            import_stmt = "\nimport { " + ", ".join(sorted_names) + " } from \"" + mod + "\";"
            content = content[:insert_pos] + import_stmt + content[insert_pos:]
            insert_pos += len(import_stmt)

        added += len(sorted_names)

    filepath.write_text(content, encoding="utf-8")
    return added > 0

def main():
    file_utils = get_missing_utils()
    print(f"发现 {len(file_utils)} 个文件缺失工具函数导入")

    fixed = 0
    total_added = 0

    for fpath_str, utils in sorted(file_utils.items()):
        fpath = Path(fpath_str)
        if not fpath.is_absolute():
            candidate = PROJECT_DIR / fpath_str
            if candidate.exists():
                fpath = candidate
            else:
                continue

        if add_imports(fpath, utils):
            print(f"  ✓ {fpath.name}: 添加 {len(utils)} 个导入 ({', '.join(sorted(utils))})")
            fixed += 1
            total_added += len(utils)

    print(f"\n修复了 {fixed} 个文件，添加了 {total_added} 个导入")

    # 再次验证
    print("\n再次运行 vue-tsc...")
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
