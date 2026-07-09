#!/usr/bin/env python3
"""移除错误添加的 ionicons 导入（xxxIcon 形式的不是真正的 ionicons 图标）。"""
import re
import subprocess
from pathlib import Path

PROJECT_DIR = Path("/workspace/app/encv-mobile")

# 错误的图标名（项目内部变量，不是 ionicons 的）
BAD_ICONS = {
    "serverIcon", "playIcon", "refreshIcon", "stopIcon",
    "layersIcon", "closeIcon", "saveIcon", "copyIcon",
    "globeIcon", "homeIcon", "searchIcon",
    "settingsIcon", "arrowForwardIcon", "refreshCircleIcon",
    "checkboxIcon", "documentIcon", "documentTextIcon",
    "informationCircleIcon", "chevronIcon", "notificationsIcon",
    "checkmarkIcon",
}

def main():
    # 从 TS2305 和 TS2724 错误中提取
    result = subprocess.run(
        ["pnpm", "exec", "vue-tsc", "--noEmit"],
        cwd=str(PROJECT_DIR),
        capture_output=True,
        text=True
    )
    lines = (result.stdout + result.stderr).split("\n")

    file_bad_icons = {}  # filepath -> set of bad icon names

    for line in lines:
        # TS2305: Module '"ionicons/icons"' has no exported member 'xxx'.
        m = re.match(r"^([^(]+)\([0-9]+,[0-9]+\): error TS2305.*has no exported member '([^']+)'", line.strip())
        if m:
            fpath = m.group(1).strip()
            icon = m.group(2)
            if icon in BAD_ICONS:
                if fpath not in file_bad_icons:
                    file_bad_icons[fpath] = set()
                file_bad_icons[fpath].add(icon)
            continue
        # TS2724: '"ionicons/icons"' has no exported member named 'xxx'.
        m = re.match(r"^([^(]+)\([0-9]+,[0-9]+\): error TS2724.*has no exported member named '([^']+)'", line.strip())
        if m:
            fpath = m.group(1).strip()
            icon = m.group(2)
            if icon in BAD_ICONS:
                if fpath not in file_bad_icons:
                    file_bad_icons[fpath] = set()
                file_bad_icons[fpath].add(icon)
            continue

    print(f"发现 {len(file_bad_icons)} 个文件有错误的图标导入")

    fixed = 0
    for fpath_str, bad_icons in sorted(file_bad_icons.items()):
        fpath = Path(fpath_str)
        if not fpath.is_absolute():
            candidate = PROJECT_DIR / fpath_str
            if candidate.exists():
                fpath = candidate
            else:
                continue

        content = fpath.read_text(encoding="utf-8")

        # 找到 ionicons/icons 的 import 语句
        import_match = re.search(r"import\s*\{([^}]*)\}\s*from\s*['\"]ionicons/icons['\"]", content)
        if not import_match:
            continue

        imports_str = import_match.group(1)
        existing = [i.strip() for i in imports_str.split(",") if i.strip()]
        new_imports = [i for i in existing if i not in bad_icons]

        if len(new_imports) == len(existing):
            continue

        if not new_imports:
            # 移除整个 import
            new_import_stmt = ""
            # 还要移除整行 import
            content = content[:import_match.start()] + content[import_match.end():]
            # 清理可能的空行
            content = re.sub(r'\n\n+', '\n\n', content)
        else:
            new_import_stmt = "import {\n  " + ",\n  ".join(sorted(new_imports)) + ",\n} from \"ionicons/icons\""
            content = content[:import_match.start()] + new_import_stmt + content[import_match.end():]

        fpath.write_text(content, encoding="utf-8")
        print(f"  ✓ {fpath.name}: 移除 {len(bad_icons)} 个错误导入 ({', '.join(sorted(bad_icons))})")
        fixed += 1

    print(f"\n修复了 {fixed} 个文件")

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
