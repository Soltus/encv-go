"""统计信息模块 - i18n 字典统计与分析"""
from __future__ import annotations

from collections import Counter, defaultdict

from .scanner import extract_used_keys
from .loader import load_all_dicts, extract_vars_from_value
from .perf import perf_tracker


def cmd_stats(app_name: str | None = None):
    perf_tracker.start("统计")
    dicts = load_all_dicts(app_name)
    used_keys = extract_used_keys(app_name)

    zh_dict = dicts.get("zh-CN", {})
    en_dict = dicts.get("en", {})
    file_source = dicts.get("_file_source", {})

    print("📊 i18n 字典统计...")
    print()
    print(f"   总 key 数: {len(zh_dict)} (zh-CN) / {len(en_dict)} (en)")

    module_counter: Counter = Counter()
    module_en_counter: Counter = Counter()
    for key in zh_dict.keys():
        mod = key.split(".")[0]
        module_counter[mod] += 1
    for key in en_dict.keys():
        mod = key.split(".")[0]
        module_en_counter[mod] += 1

    print()
    print("   按模块统计（前 20）:")
    for mod, count in module_counter.most_common(20):
        en_count = module_en_counter.get(mod, 0)
        bar_len = int(count / max(module_counter.values()) * 20)
        bar = "█" * bar_len
        print(f"   {mod:<20} {count:>4} {bar}  (en: {en_count})")

    zh_chars = sum(len(v) for v in zh_dict.values())
    en_chars = sum(len(v) for v in en_dict.values())

    print()
    print(f"   总字符数: {zh_chars} (zh-CN) / {en_chars} (en)")
    print(f"   平均长度: {zh_chars/max(len(zh_dict),1):.1f} (zh-CN) / {en_chars/max(len(en_dict),1):.1f} (en)")

    file_counter: Counter = Counter()
    for key, source in file_source.items():
        file_counter[source] += 1

    print()
    print("   按文件来源统计:")
    for fname, count in sorted(file_counter.items(), key=lambda x: -x[1]):
        print(f"   {fname:<30} {count:>4} keys")

    var_count = 0
    for key, value in zh_dict.items():
        if extract_vars_from_value(value):
            var_count += 1

    print()
    print(f"   包含变量的翻译: {var_count} (zh-CN) / {var_count} (en)")

    missing_in_zh = len(set(en_dict.keys()) - set(zh_dict.keys()))
    missing_in_en = len(set(zh_dict.keys()) - set(en_dict.keys()))
    unused = len(set(zh_dict.keys()) - set(used_keys.keys()))

    print()
    print("📈 使用情况:")
    print(f"   源码使用的 key: {len(used_keys)}")
    print(f"   字典中的 key: {len(zh_dict)}")
    print(f"   未使用的 key: {unused}")
    print(f"   中文缺失: {missing_in_zh}")
    print(f"   英文缺失: {missing_in_en}")

    perf_tracker.end("统计", len(zh_dict))
