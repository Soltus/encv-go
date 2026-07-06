#!/usr/bin/env python3
"""
i18n 工具箱 - 跨项目的 i18n 管理工具（基于 SQLite 的高级版）

功能：
1. scan       - 扫描使用的 key 与字典对比，找出缺失/多余的 key
2. dup        - 检测近重复的翻译（相似值的不同 key）
3. en-check   - 检查英文翻译完整度
4. sync       - 同步中英文 key（确保两边 key 数量一致）
5. gen-types  - 生成 TypeScript 类型定义（类型安全的 i18n key）
6. stats      - 统计 i18n 字典的详细信息
7. db-init    - 初始化 SQLite 数据库，导入所有翻译数据
8. db-query   - SQL 查询翻译数据库
9. var-check  - 检查翻译中的变量/参数一致性
10. find-key  - 模糊搜索 key 或翻译内容

用法：
    python3 scripts/i18n-tool.py scan [--app encv-mobile]
    python3 scripts/i18n-tool.py dup
    python3 scripts/i18n-tool.py en-check
    python3 scripts/i18n-tool.py gen-types
    python3 scripts/i18n-tool.py stats
    python3 scripts/i18n-tool.py db-init
    python3 scripts/i18n-tool.py db-query "SELECT * FROM translations WHERE locale='en' AND value LIKE '%file%' LIMIT 10"
    python3 scripts/i18n-tool.py var-check
    python3 scripts/i18n-tool.py find-key "search"

配置文件：i18n.config.json（可选，默认内置配置）
"""
import re
import sys
import json
import sqlite3
import hashlib
from pathlib import Path
from collections import defaultdict
from difflib import SequenceMatcher

PROJECT_ROOT = Path(__file__).resolve().parent.parent
DB_PATH = PROJECT_ROOT / ".i18n-cache.db"

DEFAULT_CONFIG = {
    "apps": {
        "encv-mobile": {
            "src_dirs": [
                "app/encv-mobile/src",
                "app/packages/shared-components/src",
            ],
            "i18n_files": [
                "app/packages/shared-components/src/i18n/common.ts",
                "app/packages/shared-components/src/i18n/devlogs.ts",
                "app/packages/shared-components/src/i18n/errors.ts",
                "app/packages/shared-components/src/i18n/settings.ts",
                "app/encv-mobile/src/i18n/agent.ts",
                "app/encv-mobile/src/i18n/tasks.ts",
                "app/encv-mobile/src/i18n/files.ts",
                "app/encv-mobile/src/i18n/player.ts",
                "app/encv-mobile/src/i18n/modals.ts",
                "app/encv-mobile/src/i18n/extensions.ts",
                "app/encv-mobile/src/i18n/simverse.ts",
            ],
            "types_output": "app/packages/shared-components/src/i18n/generated-types.ts",
        }
    },
    "default_app": "encv-mobile",
    "locales": ["zh-CN", "en"],
}


def load_config() -> dict:
    """加载配置文件，不存在则使用默认配置。"""
    config_path = PROJECT_ROOT / "i18n.config.json"
    if config_path.exists():
        with open(config_path, "r", encoding="utf-8") as f:
            user_config = json.load(f)
        config = {**DEFAULT_CONFIG, **user_config}
        return config
    return DEFAULT_CONFIG


def get_app_config(app_name: str | None = None) -> dict:
    """获取指定应用的配置。"""
    config = load_config()
    app = app_name or config.get("default_app", "encv-mobile")
    apps = config.get("apps", {})
    if app not in apps:
        print(f"❌ 未找到应用配置: {app}")
        print(f"   可用应用: {', '.join(apps.keys())}")
        sys.exit(1)
    return apps[app]


def resolve_path(rel_path: str) -> Path:
    """将相对路径解析为绝对路径。"""
    p = Path(rel_path)
    if p.is_absolute():
        return p
    return PROJECT_ROOT / rel_path


def parse_i18n_file(filepath: Path) -> dict[str, dict[str, str]]:
    """解析 i18n 文件，返回 {"zh-CN": {key: value}, "en": {key: value}}。"""
    if not filepath.exists():
        return {"zh-CN": {}, "en": {}}

    content = filepath.read_text(encoding="utf-8", errors="ignore")
    result = {"zh-CN": {}, "en": {}}
    locales = ["zh-CN", "en"]

    for locale in locales:
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

        for other in locales:
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


def load_all_dicts(app_name: str | None = None) -> dict:
    """加载所有字典，合并去重。"""
    app_config = get_app_config(app_name)
    all_zh: dict[str, str] = {}
    all_en: dict[str, str] = {}
    key_sources: dict[str, str] = {}

    for rel_path in app_config.get("i18n_files", []):
        f = resolve_path(rel_path)
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


def extract_used_keys(app_name: str | None = None) -> dict[str, list[str]]:
    """提取源码中使用的所有 i18n key。"""
    app_config = get_app_config(app_name)
    key_files: dict[str, list[str]] = defaultdict(list)

    pattern = re.compile(
        r'''(?<![a-zA-Z0-9_])t\(['"]([a-zA-Z][a-zA-Z0-9_.\-]+)['"]\)'''
    )

    for src_dir_rel in app_config.get("src_dirs", []):
        src_dir = resolve_path(src_dir_rel)
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


def extract_vars_from_value(value: str) -> set[str]:
    """从翻译值中提取变量名（格式：{varName}）。"""
    pattern = re.compile(r'\{([a-zA-Z_][a-zA-Z0-9_]*)\}')
    return set(pattern.findall(value))


def init_db(app_name: str | None = None) -> sqlite3.Connection:
    """初始化 SQLite 数据库，导入所有翻译数据。"""
    config = load_config()
    locales = config.get("locales", ["zh-CN", "en"])
    dicts = load_all_dicts(app_name)
    used = extract_used_keys(app_name)

    if DB_PATH.exists():
        DB_PATH.unlink()

    conn = sqlite3.connect(str(DB_PATH))
    conn.row_factory = sqlite3.Row

    conn.executescript("""
        CREATE TABLE translations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT NOT NULL,
            locale TEXT NOT NULL,
            value TEXT NOT NULL,
            source_file TEXT,
            value_hash TEXT,
            UNIQUE(key, locale)
        );

        CREATE TABLE key_usage (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT NOT NULL,
            file_path TEXT NOT NULL,
            UNIQUE(key, file_path)
        );

        CREATE TABLE translation_vars (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT NOT NULL,
            locale TEXT NOT NULL,
            var_name TEXT NOT NULL,
            UNIQUE(key, locale, var_name)
        );

        CREATE INDEX idx_translations_key ON translations(key);
        CREATE INDEX idx_translations_locale ON translations(locale);
        CREATE INDEX idx_translations_value ON translations(value);
        CREATE INDEX idx_key_usage_key ON key_usage(key);
    """)

    sources = dicts.get("_sources", {})
    for locale in locales:
        locale_dict = dicts.get(locale, {})
        for key, value in locale_dict.items():
            value_hash = hashlib.md5(value.encode("utf-8")).hexdigest()
            source = sources.get(key, "")
            conn.execute(
                "INSERT INTO translations (key, locale, value, source_file, value_hash) VALUES (?, ?, ?, ?, ?)",
                (key, locale, value, source, value_hash),
            )
            for var_name in extract_vars_from_value(value):
                conn.execute(
                    "INSERT OR IGNORE INTO translation_vars (key, locale, var_name) VALUES (?, ?, ?)",
                    (key, locale, var_name),
                )

    for key, files in used.items():
        for f in files:
            conn.execute(
                "INSERT OR IGNORE INTO key_usage (key, file_path) VALUES (?, ?)",
                (key, f),
            )

    conn.commit()
    print(f"✅ 数据库初始化完成: {DB_PATH}")
    print(f"   翻译条目: {conn.execute('SELECT COUNT(*) FROM translations').fetchone()[0]}")
    print(f"   唯一 key: {conn.execute('SELECT COUNT(DISTINCT key) FROM translations').fetchone()[0]}")
    print(f"   使用记录: {conn.execute('SELECT COUNT(*) FROM key_usage').fetchone()[0]}")

    return conn


def get_db(app_name: str | None = None, force_rebuild: bool = False) -> sqlite3.Connection:
    """获取数据库连接，不存在则初始化。"""
    if force_rebuild or not DB_PATH.exists():
        return init_db(app_name)
    conn = sqlite3.connect(str(DB_PATH))
    conn.row_factory = sqlite3.Row
    return conn


def cmd_scan(app_name: str | None = None):
    """扫描使用情况。"""
    print(f"🔍 扫描源码中使用的 i18n key (app: {app_name or 'default'})...")
    used = extract_used_keys(app_name)
    print(f"   共找到 {len(used)} 个使用中的静态 key")

    print("\n📚 加载 i18n 字典...")
    dicts = load_all_dicts(app_name)
    zh_keys = set(dicts["zh-CN"].keys())
    en_keys = set(dicts["en"].keys())
    print(f"   zh-CN: {len(zh_keys)} 个 key")
    print(f"   en: {len(en_keys)} 个 key")

    missing = sorted(set(used.keys()) - zh_keys)
    print(f"\n❌ 缺失的 key（使用了但 zh-CN 没有）：{len(missing)} 个")
    for k in missing[:30]:
        files = ", ".join(used[k][:2])
        print(f"   - {k}  ({files})")
    if len(missing) > 30:
        print(f"   ... 还有 {len(missing) - 30} 个")

    unused = sorted(zh_keys - set(used.keys()))
    print(f"\n💤 多余的 key（字典中有但没使用）：{len(unused)} 个")
    for k in unused[:20]:
        print(f"   - {k}  (来自 {dicts['_sources'].get(k, '?')})")
    if len(unused) > 20:
        print(f"   ... 还有 {len(unused) - 20} 个")

    zh_only = zh_keys - en_keys
    en_only = en_keys - zh_keys
    print(f"\n🌐 中英文 key 数量差异：")
    print(f"   只有中文没有英文：{len(zh_only)} 个")
    print(f"   只有英文没有中文：{len(en_only)} 个")

    if zh_only:
        print("\n   只有中文没有英文的 key（前 20）：")
        for k in sorted(zh_only)[:20]:
            print(f"   - {k}")

    if missing:
        sys.exit(1)


def cmd_dup(app_name: str | None = None):
    """检测近重复的翻译。"""
    print("🔍 检测近重复的翻译（相似值的不同 key）...")
    dicts = load_all_dicts(app_name)
    zh = dicts["zh-CN"]

    value_keys: dict[str, list[str]] = defaultdict(list)
    for k, v in zh.items():
        if len(v) > 5:
            value_keys[v].append(k)

    exact_dups = {v: ks for v, ks in value_keys.items() if len(ks) > 1}
    print(f"\n📋 完全重复的值：{len(exact_dups)} 组")
    for v, ks in sorted(exact_dups.items(), key=lambda x: -len(x[1]))[:10]:
        print(f"\n   值: \"{v[:50]}{'...' if len(v) > 50 else ''}\"")
        for k in ks:
            print(f"   - {k}")

    print(f"\n🔄 近重复检测中（可能需要一点时间）...")
    items = list(zh.items())
    near_dups = []

    for i in range(len(items)):
        k1, v1 = items[i]
        if len(v1) < 10:
            continue
        for j in range(i + 1, len(items)):
            k2, v2 = items[j]
            if len(v2) < 10:
                continue
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


def cmd_en_check(app_name: str | None = None):
    """检查英文翻译完整度。"""
    print("🌐 检查英文翻译完整度...")
    dicts = load_all_dicts(app_name)
    zh = dicts["zh-CN"]
    en = dicts["en"]

    missing_en = sorted(set(zh.keys()) - set(en.keys()))
    print(f"\n❌ 缺少英文翻译的 key：{len(missing_en)} 个")
    for k in missing_en[:30]:
        print(f"   - {k}: \"{zh[k][:50]}{'...' if len(zh[k]) > 50 else ''}\"")
    if len(missing_en) > 30:
        print(f"   ... 还有 {len(missing_en) - 30} 个")

    prefix_missing: dict[str, int] = defaultdict(int)
    for k in missing_en:
        prefix = k.split(".")[0]
        prefix_missing[prefix] += 1

    print(f"\n📊 按模块统计缺少的英文翻译：")
    for prefix, count in sorted(prefix_missing.items(), key=lambda x: -x[1]):
        total = sum(1 for k in zh if k.startswith(prefix + "."))
        if total > 0:
            print(f"   {prefix}: {count}/{total} 个缺失 ({(count/total*100):.0f}%)")

    if missing_en:
        sys.exit(1)


def cmd_gen_types(app_name: str | None = None):
    """生成 TypeScript 类型定义文件。"""
    print("🔧 生成 TypeScript 类型定义...")
    app_config = get_app_config(app_name)
    dicts = load_all_dicts(app_name)
    all_keys = sorted(set(dicts["zh-CN"].keys()) | set(dicts["en"].keys()))

    key_params: dict[str, list[str]] = {}
    for key in all_keys:
        zh_val = dicts["zh-CN"].get(key, "")
        en_val = dicts["en"].get(key, "")
        zh_vars = extract_vars_from_value(zh_val)
        en_vars = extract_vars_from_value(en_val)
        all_vars = sorted(set(zh_vars) | set(en_vars))
        if all_vars:
            key_params[key] = all_vars

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

export type MessageParamValue = string | number | boolean;

export type MessageParams = Record<string, MessageParamValue>;
"""

    if key_params:
        types_content += "\nexport interface I18nKeyParams {\n"
        for key in sorted(key_params.keys()):
            params = key_params[key]
            params_str = ", ".join(f'{p}: MessageParamValue' for p in params)
            types_content += f'  "{key}": {{ {params_str} }};\n'
        types_content += "}\n"
        types_content += "\nexport type I18nKeysWithParams = keyof I18nKeyParams;\n"
        types_content += "export type I18nKeysWithoutParams = Exclude<I18nKey, I18nKeysWithParams>;\n"

    types_content += """
export type TFunction = {
  (key: I18nKeysWithoutParams): string;
"""
    if key_params:
        types_content += "  <K extends I18nKeysWithParams>(key: K, params: I18nKeyParams[K]): string;\n"
    types_content += "  (key: string, params?: MessageParams): string;\n};\n"

    types_content += """
export type MessageModule = {
  "zh-CN": Record<string, string>;
  en: Record<string, string>;
};
"""

    output_rel = app_config.get("types_output", "app/packages/shared-components/src/i18n/generated-types.ts")
    output_path = resolve_path(output_rel)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(types_content, encoding="utf-8")
    print(f"   已生成 {len(all_keys)} 个 key 的类型定义")
    print(f"   其中 {len(key_params)} 个 key 带参数类型")
    print(f"   输出文件: {output_path}")


def cmd_stats(app_name: str | None = None):
    """统计 i18n 字典的详细信息。"""
    print("📊 i18n 字典统计...")
    dicts = load_all_dicts(app_name)
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
    max_zh = max((c["zh"] for c in prefix_stats.values()), default=1)
    for prefix, counts in sorted_prefixes[:20]:
        bar_len = int(counts["zh"] / max(max_zh, 1) * 20)
        bar = "█" * bar_len
        print(f"   {prefix:20s} {counts['zh']:4d} {bar}  (en: {counts['en']})")

    total_chars_zh = sum(len(v) for v in zh.values())
    total_chars_en = sum(len(v) for v in en.values())
    print(f"\n   总字符数: {total_chars_zh} (zh-CN) / {total_chars_en} (en)")
    avg_zh = total_chars_zh / len(zh) if zh else 0
    avg_en = total_chars_en / len(en) if en else 0
    print(f"   平均长度: {avg_zh:.1f} (zh-CN) / {avg_en:.1f} (en)")

    sources = dicts.get("_sources", {})
    source_counts: dict[str, int] = defaultdict(int)
    for src in sources.values():
        source_counts[src] += 1

    print(f"\n   按文件来源统计:")
    for src, count in sorted(source_counts.items(), key=lambda x: -x[1]):
        print(f"   {src:30s} {count:4d} keys")

    zh_with_vars = sum(1 for v in zh.values() if extract_vars_from_value(v))
    en_with_vars = sum(1 for v in en.values() if extract_vars_from_value(v))
    print(f"\n   包含变量的翻译: {zh_with_vars} (zh-CN) / {en_with_vars} (en)")


def cmd_db_init(app_name: str | None = None):
    """初始化数据库。"""
    init_db(app_name)


def cmd_db_query(query: str, app_name: str | None = None):
    """执行 SQL 查询。"""
    conn = get_db(app_name)
    try:
        cursor = conn.execute(query)
        rows = cursor.fetchall()
        if not rows:
            print("(无结果)")
            return

        cols = [d[0] for d in cursor.description]
        col_widths = {col: len(col) for col in cols}
        for row in rows:
            for col in cols:
                val = str(row[col])
                if len(val) > col_widths[col]:
                    col_widths[col] = min(len(val), 60)

        header = " | ".join(col.ljust(col_widths[col]) for col in cols)
        print(header)
        print("-" * len(header))

        for row in rows[:50]:
            vals = []
            for col in cols:
                val = str(row[col])
                if len(val) > 60:
                    val = val[:57] + "..."
                vals.append(val.ljust(col_widths[col]))
            print(" | ".join(vals))

        if len(rows) > 50:
            print(f"\n... 还有 {len(rows) - 50} 行")
    except Exception as e:
        print(f"❌ 查询错误: {e}")
    finally:
        conn.close()


def cmd_var_check(app_name: str | None = None):
    """检查翻译中的变量一致性。"""
    print("🔍 检查翻译变量/参数一致性...")
    dicts = load_all_dicts(app_name)
    zh = dicts["zh-CN"]
    en = dicts["en"]

    issues = []

    all_keys = set(zh.keys()) | set(en.keys())
    for key in all_keys:
        zh_vars = extract_vars_from_value(zh.get(key, ""))
        en_vars = extract_vars_from_value(en.get(key, ""))

        missing_in_en = zh_vars - en_vars
        missing_in_zh = en_vars - zh_vars

        if missing_in_en:
            issues.append({
                "key": key,
                "type": "missing_in_en",
                "vars": missing_in_en,
                "zh_value": zh.get(key, ""),
                "en_value": en.get(key, ""),
            })
        if missing_in_zh:
            issues.append({
                "key": key,
                "type": "missing_in_zh",
                "vars": missing_in_zh,
                "zh_value": zh.get(key, ""),
                "en_value": en.get(key, ""),
            })

    print(f"\n📊 变量一致性检查结果:")
    print(f"   检查的 key 总数: {len(all_keys)}")
    print(f"   发现问题: {len(issues)} 个")

    if issues:
        print(f"\n❌ 变量不一致的翻译（前 20）:")
        for issue in issues[:20]:
            print(f"\n   key: {issue['key']}")
            print(f"   类型: {issue['type']}")
            print(f"   变量: {', '.join(sorted(issue['vars']))}")
            zh_val = issue['zh_value'][:50] + ("..." if len(issue['zh_value']) > 50 else "")
            en_val = issue['en_value'][:50] + ("..." if len(issue['en_value']) > 50 else "")
            print(f"   zh-CN: \"{zh_val}\"")
            print(f"   en: \"{en_val}\"")

        if len(issues) > 20:
            print(f"\n   ... 还有 {len(issues) - 20} 个问题")

    if issues:
        sys.exit(1)
    else:
        print("\n✅ 所有翻译的变量完全一致！")


def cmd_find_key(query: str, app_name: str | None = None):
    """模糊搜索 key 或翻译内容。"""
    conn = get_db(app_name)
    try:
        print(f"🔍 搜索: \"{query}\"")

        rows = conn.execute(
            """
            SELECT t.key, t.locale, t.value, t.source_file
            FROM translations t
            WHERE t.key LIKE ? OR t.value LIKE ?
            ORDER BY t.key, t.locale
            LIMIT 50
            """,
            (f"%{query}%", f"%{query}%"),
        ).fetchall()

        if not rows:
            print("   未找到匹配项")
            return

        print(f"   找到 {len(rows)} 条匹配（最多显示 50 条）:\n")
        current_key = None
        for row in rows:
            if row["key"] != current_key:
                current_key = row["key"]
                print(f"📌 {current_key}  ({row['source_file']})")
            val = row["value"]
            if len(val) > 80:
                val = val[:77] + "..."
            print(f"   [{row['locale']}] {val}")
    finally:
        conn.close()


def main():
    if len(sys.argv) < 2:
        print("用法: python3 scripts/i18n-tool.py <command> [options]")
        print("")
        print("Commands:")
        print("  scan        扫描 key 使用情况和完整性")
        print("  dup         检测近重复的翻译")
        print("  en-check    检查英文翻译完整度")
        print("  gen-types   生成 TypeScript 类型定义")
        print("  stats       统计 i18n 字典信息")
        print("  var-check   检查翻译变量/参数一致性")
        print("  find-key    模糊搜索 key 或翻译内容")
        print("  db-init     初始化 SQLite 翻译数据库")
        print("  db-query    执行 SQL 查询翻译数据库")
        print("")
        print("Options:")
        print("  --app <name>  指定应用配置（默认: encv-mobile）")
        print("")
        print("示例:")
        print("  python3 scripts/i18n-tool.py scan")
        print("  python3 scripts/i18n-tool.py var-check")
        print("  python3 scripts/i18n-tool.py find-key search")
        print("  python3 scripts/i18n-tool.py db-query \"SELECT key, value FROM translations WHERE locale='en' LIMIT 5\"")
        sys.exit(1)

    cmd = sys.argv[1]

    app_name = None
    args = []
    i = 2
    while i < len(sys.argv):
        if sys.argv[i] == "--app" and i + 1 < len(sys.argv):
            app_name = sys.argv[i + 1]
            i += 2
        else:
            args.append(sys.argv[i])
            i += 1

    if cmd == "scan":
        cmd_scan(app_name)
    elif cmd == "dup":
        cmd_dup(app_name)
    elif cmd == "en-check":
        cmd_en_check(app_name)
    elif cmd == "gen-types":
        cmd_gen_types(app_name)
    elif cmd == "stats":
        cmd_stats(app_name)
    elif cmd == "db-init":
        cmd_db_init(app_name)
    elif cmd == "db-query":
        if not args:
            print("❌ 缺少 SQL 查询参数")
            sys.exit(1)
        cmd_db_query(args[0], app_name)
    elif cmd == "var-check":
        cmd_var_check(app_name)
    elif cmd == "find-key":
        if not args:
            print("❌ 缺少搜索关键词")
            sys.exit(1)
        cmd_find_key(args[0], app_name)
    else:
        print(f"未知命令: {cmd}")
        sys.exit(1)


if __name__ == "__main__":
    main()
