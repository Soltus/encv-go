#!/usr/bin/env python3
"""
i18n 工具箱 - 完整的 i18n 管理工具。

功能：
1. scan - 扫描使用的 key 与字典对比，找出缺失/多余的 key
2. dup - 检测近重复的翻译（相似值的不同 key）
3. en-check - 检查英文翻译完整度
4. sync - 同步中英文 key（确保两边 key 数量一致）
5. gen-types - 生成 TypeScript 类型定义（类型安全的 i18n key）
6. stats - 统计 i18n 字典的详细信息

用法：
    python3 scripts/i18n-tool.py scan
    python3 scripts/i18n-tool.py dup
    python3 scripts/i18n-tool.py en-check
    python3 scripts/i18n-tool.py gen-types
    python3 scripts/i18n-tool.py stats
"""
import re
import sys
from pathlib import Path
from collections import defaultdict
from difflib import SequenceMatcher

PROJECT_ROOT = Path("/workspace")

SRC_DIRS = [
    PROJECT_ROOT / "app/encv-mobile/src",
    PROJECT_ROOT / "app/packages/shared-components/src",
]

I18N_FILES = [
    # shared-components
    PROJECT_ROOT / "app/packages/shared-components/src/i18n/common.ts",
    PROJECT_ROOT / "app/packages/shared-components/src/i18n/devlogs.ts",
    PROJECT_ROOT / "app/packages/shared-components/src/i18n/errors.ts",
    PROJECT_ROOT / "app/packages/shared-components/src/i18n/settings.ts",
    # encv-mobile
    PROJECT_ROOT / "app/encv-mobile/src/i18n/agent.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/tasks.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/files.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/player.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/modals.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/extensions.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/simverse.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/settings.ts",
    PROJECT_ROOT / "app/encv-mobile/src/i18n/devlogs.ts",
]


def parse_i18n_file(filepath: Path) -> dict[str, dict[str, str]]:
    """解析 i18n 文件，返回 {"zh-CN": {key: value}, "en": {key: value}}。"""
    if not filepath.exists():
        return {"zh-CN": {}, "en": {}}

    content = filepath.read_text(encoding="utf-8", errors="ignore")
    result = {"zh-CN": {}, "en": {}}

    for locale in ["zh-CN", "en"]:
        locale_patterns = [
            rf'"{re.escape(locale)}"\s*:\s*\{{',
            rf'{re.escape(locale)}\s*:\s*\{{',
        ]
        start_match = None
        for pat in locale_patterns:
            m = re.search(pat, content)
            if m:
                start_match = m
                break
        if not start_match:
            continue

        start_pos = start_match.end()
        end_pos = len(content)

        for other in ["zh-CN", "en"]:
            if other == locale:
                continue
            other_patterns = [
                rf'^\s*"{re.escape(other)}"\s*:\s*\{{',
                rf'^\s*{re.escape(other)}\s*:\s*\{{',
            ]
            for pat in other_patterns:
                other_match = re.search(pat, content[start_pos:], re.MULTILINE)
                if other_match:
                    candidate = start_pos + other_match.start()
                    if candidate < end_pos:
                        end_pos = candidate

        block = content[start_pos:end_pos]
        last_close = block.rfind("\n  }")
        if last_close > 0:
            block = block[:last_close]

        lines = block.split("\n")
        i = 0
        current_key = None
        current_value_parts = []
        in_multiline = False

        while i < len(lines):
            line = lines[i]
            stripped = line.strip()

            if stripped.startswith("//") or stripped.startswith("/*"):
                i += 1
                continue

            if in_multiline:
                m = re.match(r'^["\'](.+?)["\'],?\s*$', stripped)
                if m:
                    current_value_parts.append(m.group(1))
                    result[locale][current_key] = "".join(current_value_parts)
                    in_multiline = False
                    current_key = None
                    current_value_parts = []
                else:
                    current_value_parts.append(stripped.rstrip(","))
                i += 1
                continue

            m = re.match(r'^"([^"]+)"\s*:\s*["\'](.+?)["\'],?\s*$', stripped)
            if m:
                key = m.group(1)
                value = m.group(2)
                result[locale][key] = value
                i += 1
                continue

            m = re.match(r'^"([^"]+)"\s*:\s*["\'](.*)$', stripped)
            if m:
                current_key = m.group(1)
                first_part = m.group(2)
                if first_part.endswith('",') or first_part.endswith("',"):
                    val = first_part[:-2]
                    result[locale][current_key] = val
                    current_key = None
                elif first_part.endswith('"') or first_part.endswith("'"):
                    val = first_part[:-1]
                    result[locale][current_key] = val
                    current_key = None
                else:
                    current_value_parts = [first_part]
                    in_multiline = True
                i += 1
                continue

            m = re.match(r'^"([^"]+)"\s*:\s*$', stripped)
            if m:
                current_key = m.group(1)
                current_value_parts = []
                in_multiline = True
                i += 1
                continue

            i += 1

    return result


def load_all_dicts() -> dict[str, dict[str, str]]:
    """加载所有字典，合并去重。"""
    all_zh: dict[str, str] = {}
    all_en: dict[str, str] = {}
    key_sources: dict[str, str] = {}

    for f in I18N_FILES:
        if not f.exists():
            continue
        parsed = parse_i18n_file(f)
        for k, v in parsed["zh-CN"].items():
            if k not in all_zh:
                all_zh[k] = v
                key_sources[k] = f.name
        for k, v in parsed["en"].items():
            if k not in all_en:
                all_en[k] = v

    return {"zh-CN": all_zh, "en": all_en, "_sources": key_sources}


def extract_used_keys() -> dict[str, list[str]]:
    """提取源码中使用的所有 i18n key。"""
    key_files: dict[str, list[str]] = defaultdict(list)

    pattern = re.compile(
        r'''(?<![a-zA-Z0-9_])t\(['"]([a-zA-Z][a-zA-Z0-9_.\-]+)['"]\)'''
    )

    for src_dir in SRC_DIRS:
        if not src_dir.exists():
            continue
        for filepath in src_dir.rglob("*"):
            if filepath.suffix not in (".ts", ".vue"):
                continue
            if "node_modules" in str(filepath) or "__tests__" in str(filepath):
                continue

            content = filepath.read_text(encoding="utf-8", errors="ignore")
            rel_path = str(filepath.relative_to(PROJECT_ROOT))

            for m in pattern.finditer(content):
                key = m.group(1)
                if "." in key and not key.startswith("@") and not key.startswith("./"):
                    if rel_path not in key_files[key]:
                        key_files[key].append(rel_path)

    return key_files


def cmd_scan():
    """扫描使用情况。"""
    print("🔍 扫描源码中使用的 i18n key...")
    used = extract_used_keys()
    print(f"   共找到 {len(used)} 个使用中的静态 key")

    print("\n📚 加载 i18n 字典...")
    dicts = load_all_dicts()
    zh_keys = set(dicts["zh-CN"].keys())
    en_keys = set(dicts["en"].keys())
    print(f"   zh-CN: {len(zh_keys)} 个 key")
    print(f"   en: {len(en_keys)} 个 key")

    # 缺失的 key
    missing = sorted(set(used.keys()) - zh_keys)
    print(f"\n❌ 缺失的 key（使用了但 zh-CN 没有）：{len(missing)} 个")
    for k in missing[:30]:
        files = ", ".join(used[k][:2])
        print(f"   - {k}  ({files})")
    if len(missing) > 30:
        print(f"   ... 还有 {len(missing) - 30} 个")

    # 多余的 key
    unused = sorted(zh_keys - set(used.keys()))
    print(f"\n💤 多余的 key（字典中有但没使用）：{len(unused)} 个")
    for k in unused[:20]:
        print(f"   - {k}  (来自 {dicts['_sources'].get(k, '?')})")
    if len(unused) > 20:
        print(f"   ... 还有 {len(unused) - 20} 个")

    # 中英文差异
    zh_only = zh_keys - en_keys
    en_only = en_keys - zh_keys
    print(f"\n🌐 中英文 key 数量差异：")
    print(f"   只有中文没有英文：{len(zh_only)} 个")
    print(f"   只有英文没有中文：{len(en_only)} 个")

    if zh_only:
        print("\n   只有中文没有英文的 key（前 20）：")
        for k in sorted(zh_only)[:20]:
            print(f"   - {k}")


def cmd_dup():
    """检测近重复的翻译。"""
    print("🔍 检测近重复的翻译（相似值的不同 key）...")
    dicts = load_all_dicts()
    zh = dicts["zh-CN"]

    # 按值分组，找出完全重复的值
    value_keys: dict[str, list[str]] = defaultdict(list)
    for k, v in zh.items():
        if len(v) > 5:  # 只检查较长的翻译，避免短值误报
            value_keys[v].append(k)

    exact_dups = {v: ks for v, ks in value_keys.items() if len(ks) > 1}
    print(f"\n📋 完全重复的值：{len(exact_dups)} 组")
    for v, ks in sorted(exact_dups.items(), key=lambda x: -len(x[1]))[:10]:
        print(f"\n   值: \"{v[:50]}{'...' if len(v) > 50 else ''}\"")
        for k in ks:
            print(f"   - {k}")

    # 近重复检测（相似度 > 0.85）
    print(f"\n🔄 近重复检测中（可能需要一点时间）...")
    items = list(zh.items())
    near_dups = []

    for i in range(len(items)):
        k1, v1 = items[i]
        if len(v1) < 10:  # 太短的跳过
            continue
        for j in range(i + 1, len(items)):
            k2, v2 = items[j]
            if len(v2) < 10:
                continue
            # 同一个前缀的跳过（可能是同模块的不同 key）
            if k1.split(".")[0] == k2.split(".")[0]:
                continue
            ratio = SequenceMatcher(None, v1, v2).ratio()
            if ratio > 0.85:
                near_dups.append((ratio, k1, v1, k2, v2))

    near_dups.sort(key=lambda x: -x[0])
    print(f"\n   近重复的翻译（相似度 > 85%）：{len(near_dups)} 组")
    for ratio, k1, v1, k2, v2 in near_dups[:15]:
        print(f"\n   相似度: {ratio:.0%}")
        print(f"   {k1}: \"{v1[:40]}{'...' if len(v1) > 40 else ''}\"")
        print(f"   {k2}: \"{v2[:40]}{'...' if len(v2) > 40 else ''}\"")


def cmd_en_check():
    """检查英文翻译完整度。"""
    print("🌐 检查英文翻译完整度...")
    dicts = load_all_dicts()
    zh = dicts["zh-CN"]
    en = dicts["en"]

    missing_en = sorted(set(zh.keys()) - set(en.keys()))
    print(f"\n❌ 缺少英文翻译的 key：{len(missing_en)} 个")
    for k in missing_en[:30]:
        print(f"   - {k}: \"{zh[k][:50]}{'...' if len(zh[k]) > 50 else ''}\"")
    if len(missing_en) > 30:
        print(f"   ... 还有 {len(missing_en) - 30} 个")

    # 按模块统计
    prefix_missing: dict[str, int] = defaultdict(int)
    for k in missing_en:
        prefix = k.split(".")[0]
        prefix_missing[prefix] += 1

    print(f"\n📊 按模块统计缺少的英文翻译：")
    for prefix, count in sorted(prefix_missing.items(), key=lambda x: -x[1]):
        total = sum(1 for k in zh if k.startswith(prefix + "."))
        print(f"   {prefix}: {count}/{total} 个缺失 ({(count/total*100):.0f}%)")


def cmd_gen_types():
    """生成 TypeScript 类型定义文件。"""
    print("🔧 生成 TypeScript 类型定义...")
    dicts = load_all_dicts()
    all_keys = sorted(set(dicts["zh-CN"].keys()) | set(dicts["en"].keys()))

    types_content = """// AUTO-GENERATED by scripts/i18n-tool.py gen-types
// DO NOT EDIT MANUALLY

export type I18nKey =
"""

    for i, key in enumerate(all_keys):
        if i < len(all_keys) - 1:
            types_content += f'  | "{key}"\n'
        else:
            types_content += f'  | "{key}";\n'

    types_content += """
export type Locale = "zh-CN" | "en";

export type MessageParams = Record<string, string>;

export type TFunction = (key: I18nKey, params?: MessageParams) => string;

export type MessageModule = {
  "zh-CN": Record<string, string>;
  en: Record<string, string>;
};
"""

    output_path = PROJECT_ROOT / "app/packages/shared-components/src/i18n/generated-types.ts"
    output_path.write_text(types_content, encoding="utf-8")
    print(f"   已生成 {len(all_keys)} 个 key 的类型定义")
    print(f"   输出文件: {output_path}")


def cmd_stats():
    """统计 i18n 字典的详细信息。"""
    print("📊 i18n 字典统计...")
    dicts = load_all_dicts()
    zh = dicts["zh-CN"]
    en = dicts["en"]

    print(f"\n   总 key 数: {len(zh)} (zh-CN) / {len(en)} (en)")

    prefix_stats: dict[str, dict[str, int]] = defaultdict(lambda: {"zh": 0, "en": 0})
    for k in zh:
        prefix = k.split(".")[0]
        prefix_stats[prefix]["zh"] += 1
    for k in en:
        prefix = k.split(".")[0]
        prefix_stats[prefix]["en"] += 1

    print(f"\n   按模块统计（前 20）:")
    sorted_prefixes = sorted(prefix_stats.items(), key=lambda x: -x[1]["zh"])
    for prefix, counts in sorted_prefixes[:20]:
        bar_len = int(counts["zh"] / max(1, sorted_prefixes[0][1]["zh"]) * 20)
        bar = "█" * bar_len
        print(f"   {prefix:20s} {counts['zh']:4d} {bar}  (en: {counts['en']})")

    total_chars_zh = sum(len(v) for v in zh.values())
    total_chars_en = sum(len(v) for v in en.values())
    print(f"\n   总字符数: {total_chars_zh} (zh-CN) / {total_chars_en} (en)")
    print(f"   平均长度: {total_chars_zh/len(zh):.1f} (zh-CN) / {total_chars_en/len(en):.1f} (en)")

    sources = dicts.get("_sources", {})
    source_counts: dict[str, int] = defaultdict(int)
    for src in sources.values():
        source_counts[src] += 1

    print(f"\n   按文件来源统计:")
    for src, count in sorted(source_counts.items(), key=lambda x: -x[1]):
        print(f"   {src:30s} {count:4d} keys")


def main():
    if len(sys.argv) < 2:
        print("用法: python3 scripts/i18n-tool.py <command>")
        print("")
        print("Commands:")
        print("  scan        扫描 key 使用情况和完整性")
        print("  dup         检测近重复的翻译")
        print("  en-check    检查英文翻译完整度")
        print("  gen-types   生成 TypeScript 类型定义")
        print("  stats       统计 i18n 字典信息")
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "scan":
        cmd_scan()
    elif cmd == "dup":
        cmd_dup()
    elif cmd == "en-check":
        cmd_en_check()
    elif cmd == "gen-types":
        cmd_gen_types()
    elif cmd == "stats":
        cmd_stats()
    else:
        print(f"未知命令: {cmd}")
        sys.exit(1)


if __name__ == "__main__":
    main()
