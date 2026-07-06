"""字典加载模块 - 加载和解析 i18n 字典文件"""
from __future__ import annotations

import re
import pickle
import hashlib
from collections import defaultdict

from .config import CACHE_DIR, get_app_config, resolve_path
from .perf import perf_tracker
from .tokenizer import file_hash

DICT_KEY_PATTERN = re.compile(r'''["']([^"']+)["']\s*:\s*["']((?:[^"\\]|\\.)*)["']''')


def parse_i18n_file(filepath: str) -> dict[str, dict[str, str]]:
    result = {"zh-CN": {}, "en": {}}
    current_locale = None

    try:
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
    except Exception:
        return result

    lines = content.split("\n")
    for line_num, line in enumerate(lines, 1):
        stripped = line.strip()

        if '"zh-CN"' in stripped or "'zh-CN'" in stripped:
            if "{" in stripped:
                current_locale = "zh-CN"
                continue
        if re.match(r'^["\']?en["\']?\s*:\s*\{', stripped):
            if "zh-CN" not in stripped:
                current_locale = "en"
                continue

        if current_locale and ":" in stripped and '"' in stripped:
            match = DICT_KEY_PATTERN.search(stripped)
            if match:
                key = match.group(1)
                value = match.group(2)
                value = value.replace('\\"', '"').replace("\\'", "'").replace("\\\\", "\\")
                if not key.startswith("//"):
                    result[current_locale][key] = value

    return result


def load_cache(key: str) -> dict | None:
    cache_file = CACHE_DIR / f"{hashlib.md5(key.encode()).hexdigest()}.pkl"
    if cache_file.exists():
        try:
            with open(cache_file, "rb") as f:
                return pickle.load(f)
        except Exception:
            return None
    return None


def save_cache(key: str, data: dict):
    cache_file = CACHE_DIR / f"{hashlib.md5(key.encode()).hexdigest()}.pkl"
    try:
        with open(cache_file, "wb") as f:
            pickle.dump(data, f)
    except Exception:
        pass


def load_all_dicts(
    app_name: str | None = None,
    use_cache: bool = True,
) -> dict[str, dict[str, str]]:
    perf_tracker.start("字典加载")
    app_config = get_app_config(app_name)

    cache_key = f"dicts:{app_name or 'default'}"
    file_hash_map = {}
    for f in app_config.i18n_files:
        p = resolve_path(f)
        if p.exists():
            file_hash_map[f] = file_hash(str(p))

    cache_version = hashlib.md5(
        "|".join(f"{f}:{h}" for f, h in sorted(file_hash_map.items())).encode()
    ).hexdigest()

    if use_cache:
        cached = load_cache(cache_key)
        if cached and cached.get("_version") == cache_version:
            perf_tracker.end("字典加载", len(cached.get("zh-CN", {})), {"cached": True})
            return {k: v for k, v in cached.items() if k != "_version"}

    merged: dict[str, dict[str, str]] = {"zh-CN": {}, "en": {}}
    file_source: dict[str, str] = {}

    for file_rel in app_config.i18n_files:
        filepath = resolve_path(file_rel)
        if not filepath.exists():
            continue
        parsed = parse_i18n_file(str(filepath))
        basename = filepath.name
        for locale in ["zh-CN", "en"]:
            for key, value in parsed[locale].items():
                merged[locale][key] = value
                if key not in file_source:
                    file_source[key] = basename

    result = {**merged, "_file_source": file_source}

    if use_cache:
        save_data = {k: v for k, v in result.items()}
        save_data["_version"] = cache_version
        save_cache(cache_key, save_data)

    perf_tracker.end("字典加载", len(merged["zh-CN"]))
    return result


def extract_vars_from_value(value: str) -> set[str]:
    pattern = re.compile(r"\{([^}:]+)(?::[^}]+)?\}")
    vars_set = set()
    for match in pattern.finditer(value):
        var_name = match.group(1).strip()
        if "|" in var_name:
            var_name = var_name.split("|")[1].strip()
        vars_set.add(var_name)
    return vars_set
