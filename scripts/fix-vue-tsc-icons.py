#!/usr/bin/env python3
"""
批量给 Vue SFC 添加缺失的 ionicons 导入。

策略：
1. 从 vue-tsc 错误中提取每个文件缺失的属性名
2. 判断哪些是 ionicons 图标（常见后缀：Outline, Sharp, Sharp 等，或匹配 ionicons 命名模式）
3. 给每个文件添加 import { xxx } from 'ionicons/icons'
"""
import re
import sys
import subprocess
from pathlib import Path

PROJECT_DIR = Path("/workspace/app/encv-mobile")

# 已知的 ionicons 图标后缀/模式
ICON_SUFFIXES = [
    "Outline", "Sharp", "Circle", "Outline", "Sharp",
    "Outline", "Filled",
]

# 常见的 ionicons 图标名前缀（用于判断是不是图标）
ICON_KEYWORDS = [
    "arrow", "checkmark", "close", "cloud", "chevron", "copy",
    "document", "folder", "heart", "home", "information", "list",
    "lock", "mail", "menu", "notifications", "pause", "play",
    "refresh", "search", "settings", "share", "star", "trash",
    "warning", "add", "alert", "apps", "archive", "back",
    "book", "bug", "bulb", "calendar", "caret", "cart",
    "cash", "chatbubbles", "checkbox", "clipboard", "cloud",
    "code", "cog", "color", "construct", "cube", "cut",
    "download", "ellipsis", "eye", "fast", "file", "film",
    "filter", "flash", "flask", "folder", "funnel", "gift",
    "git", "globe", "grid", "hammer", "hand", "happy",
    "headset", "heart", "help", "home", "hourglass", "image",
    "infinite", "journal", "key", "layers", "leaf", "library",
    "link", "list", "locate", "lock", "log", "mail", "male",
    "map", "medal", "medical", "mic", "moon", "musical",
    "navigate", "newspaper", "notifications", "nuclear",
    "nutrition", "paper", "pause", "pencil", "people",
    "person", "phone", "pin", "pint", "planet", "play",
    "podium", "power", "pricetag", "print", "qr", "radio",
    "rainy", "reader", "receipt", "recording", "refresh",
    "reorder", "repeat", "resize", "return", "ribbon",
    "rocket", "save", "scan", "search", "send", "server",
    "settings", "share", "shield", "shuffle", "skull",
    "sparkles", "speedometer", "square", "star", "stop",
    "store", "subway", "sunny", "swap", "sync", "tablet",
    "tag", "tear", "terminal", "text", "thermometer",
    "thumbs", "timer", "today", "toggle", "train", "trash",
    "triangle", "trophy", "trending", "umbrella", "unlink",
    "videocam", "volume", "walk", "wallet", "warning",
    "water", "wifi", "wine", "body", "cafe", "logo",
    "albums", "analytics", "bug", "layers", "sync",
    "flashlight",
]

def is_icon_name(name: str) -> bool:
    """判断一个变量名是否是 ionicons 图标名。"""
    # 先检查常见后缀
    for suffix in ICON_SUFFIXES:
        if name.endswith(suffix) and len(name) > len(suffix) + 2:
            return True

    # 检查常见前缀
    lower_name = name.lower()
    for kw in ICON_KEYWORDS:
        if lower_name.startswith(kw):
            return True

    # 一些特殊的图标名
    special_icons = [
        "add", "sync", "bug", "globe", "server", "copy",
        "search", "refresh", "save", "create", "trash",
        "grid", "list", "menu", "settings", "home",
        "arrowForward", "arrowBack", "chevronForward", "chevronDown",
        "checkmarkCircle", "closeCircle", "alertCircle",
        "documentText", "folderOpen", "lockClosed",
        "cloudOutline", "cloudDownloadOutline", "cloudUploadOutline",
        "playCircle", "pauseCircle",
        "logoAndroid", "logoVue", "logoIonic", "logoGoogle",
        "phonePortrait", "phonePortraitOutline",
        "shareSocial", "chatbubbles", "codeSlash",
        "helpCircle", "eye", "colorPalette", "gitNetwork",
        "speedometer", "terminal", "cube", "apps",
        "image", "construct", "layers", "flash", "flask",
        "cafe", "code", "key", "film", "text", "archive",
        "analytics", "albums",
    ]
    if name in special_icons:
        return True

    return False

def get_tsc_errors():
    print("运行 vue-tsc 收集错误...")
    result = subprocess.run(
        ["pnpm", "exec", "vue-tsc", "--noEmit"],
        cwd=str(PROJECT_DIR),
        capture_output=True,
        text=True
    )
    lines = (result.stdout + result.stderr).split("\n")

    # 解析 TS2339: Property 'xxx' does not exist
    file_props = {}  # filepath -> set of property names
    pattern = re.compile(r"^([^(]+)\([0-9]+,[0-9]+\): error TS2339: Property '([^']+)' does not exist")

    for line in lines:
        m = pattern.match(line.strip())
        if m:
            fpath = m.group(1).strip()
            prop = m.group(2)
            if fpath not in file_props:
                file_props[fpath] = set()
            file_props[fpath].add(prop)

    return file_props

def add_icons_import(filepath: Path, icons: set):
    """给文件添加 ionicons 导入。"""
    if not filepath.exists():
        return False

    content = filepath.read_text(encoding="utf-8")

    # 检查是否已经有 ionicons/icons 导入
    import_match = re.search(r"import\s*\{([^}]*)\}\s*from\s*['\"]ionicons/icons['\"]", content)

    sorted_icons = sorted(icons)

    if import_match:
        # 已有导入，添加新图标
        existing = import_match.group(1)
        existing_icons = [i.strip() for i in existing.split(",") if i.strip()]
        new_icons = [i for i in sorted_icons if i not in existing_icons]
        if not new_icons:
            return False
        all_icons = sorted(set(existing_icons + new_icons))
        new_import = "import {\n  " + ",\n  ".join(all_icons) + ",\n} from \"ionicons/icons\""
        content = content.replace(import_match.group(0), new_import)
    else:
        # 没有导入，在 script 开头添加
        script_match = re.search(r"<script[^>]*>", content)
        if not script_match:
            return False

        insert_pos = script_match.end()
        import_stmt = "\nimport {\n  " + ",\n  ".join(sorted_icons) + ",\n} from \"ionicons/icons\";\n"
        content = content[:insert_pos] + import_stmt + content[insert_pos:]

    filepath.write_text(content, encoding="utf-8")
    return True

def main():
    file_props = get_tsc_errors()
    print(f"发现 {len(file_props)} 个文件有 TS2339 错误")

    fixed_files = 0
    total_icons_added = 0

    for fpath_str, props in sorted(file_props.items()):
        # 解析路径
        fpath = Path(fpath_str)
        if not fpath.is_absolute():
            candidate = PROJECT_DIR / fpath_str
            if candidate.exists():
                fpath = candidate
            else:
                continue

        # 过滤出图标属性
        icon_props = {p for p in props if is_icon_name(p)}
        if not icon_props:
            continue

        # 过滤掉已经导入的
        content = fpath.read_text(encoding="utf-8")
        # 简单检查：看看脚本中有没有这些变量名的定义/导入
        script_match = re.search(r"<script[^>]*>(.*?)</script>", content, re.DOTALL)
        if script_match:
            script_content = script_match.group(1)
            icon_props = {p for p in icon_props if not re.search(r"\b" + re.escape(p) + r"\b", script_content)}

        if not icon_props:
            continue

        print(f"  ✓ {fpath.name}: 添加 {len(icon_props)} 个图标 ({', '.join(sorted(icon_props)[:5])}{'...' if len(icon_props) > 5 else ''})")
        if add_icons_import(fpath, icon_props):
            fixed_files += 1
            total_icons_added += len(icon_props)

    print(f"\n修复了 {fixed_files} 个文件，添加了 {total_icons_added} 个图标导入")

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
