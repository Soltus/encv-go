"""源码扫描模块 - 提取 i18n key 使用情况"""
from __future__ import annotations

import re
import pickle
import hashlib
from pathlib import Path
from collections import defaultdict

from .config import CACHE_DIR, get_app_config, resolve_path
from .perf import perf_tracker
from .tokenizer import file_hash

I18N_PATTERNS = [
    re.compile(r"""\bt\(\s*['"`]([^'"`]+)['"`]"""),
    re.compile(r"""\btField\(\s*['"`]([^'"`]+)['"`]"""),
    re.compile(r"""\btSectionTitle\(\s*['"`]([^'"`]+)['"`]"""),
    re.compile(r"""\$t\(\s*['"`]([^'"`]+)['"`]"""),
]


def scan_file(filepath: str) -> tuple[dict[str, list[str]], str]:
    local_keys: dict[str, list[str]] = defaultdict(list)
    try:
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
    except Exception:
        return dict(local_keys), ""

    rel_path = filepath
    for line_num, line in enumerate(content.split("\n"), 1):
        for pattern in I18N_PATTERNS:
            for match in pattern.finditer(line):
                key = match.group(1)
                local_keys[key].append(f"{rel_path}:{line_num}")

    return dict(local_keys), file_hash(filepath)


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


def extract_used_keys(
    app_name: str | None = None,
    use_cache: bool = True,
) -> dict[str, list[str]]:
    perf_tracker.start("源码扫描")
    app_config = get_app_config(app_name)

    all_files: list[str] = []
    for src_dir_rel in app_config.src_dirs:
        src_dir = resolve_path(src_dir_rel)
        if not src_dir.exists():
            continue
        for filepath in src_dir.rglob("*"):
            if filepath.suffix not in (".ts", ".vue"):
                continue
            if "node_modules" in str(filepath) or "__tests__" in str(filepath):
                continue
            all_files.append(str(filepath))

    cache_key = f"used_keys:{app_name or 'default'}"
    file_hash_map = {}
    for f in all_files:
        p = Path(f)
        if p.exists():
            file_hash_map[f] = file_hash(f)

    cache_version = hashlib.md5(
        "|".join(f"{f}:{h}" for f, h in sorted(file_hash_map.items())).encode()
    ).hexdigest()

    if use_cache:
        cached = load_cache(cache_key)
        if cached and cached.get("_version") == cache_version:
            perf_tracker.end("源码扫描", len(cached.get("keys", {})), {"cached": True})
            return cached.get("keys", {})

    key_files: dict[str, list[str]] = defaultdict(list)

    for f in all_files:
        local_keys, _ = scan_file(f)
        for k, files in local_keys.items():
            key_files[k].extend(files)

    result = dict(key_files)

    if use_cache:
        save_cache(cache_key, {"keys": result, "_version": cache_version})

    perf_tracker.end("源码扫描", len(result), {"files": len(all_files)})
    return result
