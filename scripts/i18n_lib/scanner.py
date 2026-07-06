"""源码扫描模块 - 提取 i18n key 使用情况（高性能 v3）"""
from __future__ import annotations

import re
import json
import hashlib
import tempfile
import os
from pathlib import Path
from collections import defaultdict

from .config import CACHE_DIR, get_app_config, resolve_path
from .perf import perf_tracker

CACHE_SCHEMA_VERSION = 2

COMBINED_PATTERN = re.compile(
    r"""(?<![\w$.])(?:t|\$t|tField|tSectionTitle)\(\s*['"`]([^'"`]+)['"`]""",
)

DYNAMIC_KEY_MARKERS = ("${", "{", "`", "$", "+")

_SKIP_SUFFIXES = {".d.ts"}
_SKIP_DIRS = {"node_modules", "__tests__", "test", "tests", "dist", ".nuxt", ".next"}


def is_dynamic_key(key: str) -> bool:
    return any(m in key for m in DYNAMIC_KEY_MARKERS)


def _strip_comments(line: str) -> str:
    in_string = False
    string_char = ""
    i = 0
    while i < len(line):
        c = line[i]
        if in_string:
            if c == "\\" and i + 1 < len(line):
                i += 2
                continue
            if c == string_char:
                in_string = False
        else:
            if c in ('"', "'", "`"):
                in_string = True
                string_char = c
            elif c == "/" and i + 1 < len(line) and line[i + 1] == "/":
                return line[:i]
        i += 1
    return line


def scan_file(filepath: str) -> tuple[dict[str, list[str]], str]:
    direct_keys: dict[str, list[str]] = defaultdict(list)
    try:
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
    except Exception:
        return {}, ""

    rel_path = filepath
    for line_num, line in enumerate(content.split("\n"), 1):
        if not line.strip():
            continue
        if "t(" not in line and "$t" not in line and "tField" not in line and "tSection" not in line:
            continue

        clean_line = _strip_comments(line)
        if not clean_line:
            continue

        for match in COMBINED_PATTERN.finditer(clean_line):
            full_match = match.group(0)
            key = match.group(1)
            if is_dynamic_key(key):
                continue
            call_type = full_match.split("(")[0].strip()
            if call_type in ("t", "$t"):
                direct_keys[key].append(f"{rel_path}:{line_num}")

    return dict(direct_keys), _file_fingerprint(filepath)


def _file_fingerprint(filepath: str) -> str:
    try:
        p = Path(filepath)
        stat = p.stat()
        return f"{stat.st_size}:{stat.st_mtime_ns}"
    except Exception:
        return ""


def _collect_files(app_config) -> list[str]:
    all_files: list[str] = []
    seen = set()
    for src_dir_rel in app_config.src_dirs:
        src_dir = resolve_path(src_dir_rel)
        if not src_dir.exists():
            continue
        for filepath in src_dir.rglob("*"):
            if filepath.suffix not in (".ts", ".vue", ".tsx"):
                continue
            if filepath.suffix in _SKIP_SUFFIXES:
                continue
            parts = set(filepath.parts)
            if any(d in parts for d in _SKIP_DIRS):
                continue
            fp = str(filepath)
            if fp not in seen:
                seen.add(fp)
                all_files.append(fp)
    return all_files


def _cache_key(app_name: str) -> str:
    return f"used_keys:{app_name}:v{CACHE_SCHEMA_VERSION}"


def _cache_path(app_name: str) -> tuple[Path, Path]:
    key = _cache_key(app_name)
    hashed = hashlib.md5(key.encode()).hexdigest()
    json_path = CACHE_DIR / f"{hashed}.json"
    lock_path = CACHE_DIR / f"{hashed}.lock"
    return json_path, lock_path


def _compute_files_hash(filepaths: list[str]) -> str:
    hasher = hashlib.md5()
    for fp in sorted(filepaths):
        fp_str = str(fp)
        finger = _file_fingerprint(fp_str)
        hasher.update(f"{fp_str}:{finger}".encode())
    return hasher.hexdigest()


def _load_from_cache(app_name: str, files_hash: str) -> dict | None:
    json_path, lock_path = _cache_path(app_name)
    if lock_path.exists():
        try:
            import time
            lock_mtime = lock_path.stat().st_mtime
            if time.time() - lock_mtime > 30:
                lock_path.unlink()
            else:
                return None
        except Exception:
            pass

    if not json_path.exists():
        return None

    try:
        with open(json_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        if data.get("_files_hash") != files_hash:
            return None
        if data.get("_schema_version") != CACHE_SCHEMA_VERSION:
            return None
        return data
    except Exception:
        return None


def _save_to_cache(app_name: str, files_hash: str, data: dict) -> None:
    json_path, lock_path = _cache_path(app_name)
    try:
        lock_path.touch()
        save_data = {
            "keys": data,
            "_files_hash": files_hash,
            "_schema_version": CACHE_SCHEMA_VERSION,
        }
        tmp_fd, tmp_path = tempfile.mkstemp(dir=str(CACHE_DIR), suffix=".tmp")
        try:
            with os.fdopen(tmp_fd, "w", encoding="utf-8") as f:
                json.dump(save_data, f, ensure_ascii=False)
            os.replace(tmp_path, json_path)
        finally:
            if os.path.exists(tmp_path):
                try:
                    os.unlink(tmp_path)
                except Exception:
                    pass
    except Exception:
        pass
    finally:
        try:
            if lock_path.exists():
                lock_path.unlink()
        except Exception:
            pass


def extract_used_keys(
    app_name: str | None = None,
    use_cache: bool = True,
) -> dict[str, list[str]]:
    perf_tracker.start("源码扫描")
    app_config = get_app_config(app_name)

    all_files = _collect_files(app_config)
    files_hash = _compute_files_hash(all_files)

    if use_cache:
        cached = _load_from_cache(app_config.name, files_hash)
        if cached:
            perf_tracker.end(
                "源码扫描",
                len(cached.get("keys", {})),
                {"cached": True, "files": len(all_files)},
            )
            return cached.get("keys", {})

    result: dict[str, list[str]] = defaultdict(list)

    for f in all_files:
        keys, _ = scan_file(f)
        for k, files in keys.items():
            result[k].extend(files)

    final_result = dict(result)

    if use_cache:
        _save_to_cache(app_config.name, files_hash, final_result)

    perf_tracker.end(
        "源码扫描",
        len(final_result),
        {"cached": False, "files": len(all_files)},
    )
    return final_result
