"""字典加载模块 - 用 Node.js 原生解析 i18n 字典文件，100% 准确"""
from __future__ import annotations

import json
import hashlib
import subprocess
import os
import tempfile
from pathlib import Path
from collections import defaultdict

from .config import CACHE_DIR, get_app_config, resolve_path
from .perf import perf_tracker

import threading

# shared 字典进程内只解析一次（跨 app 复用），避免每个 app 重复起 node 子进程
_shared_lock = threading.Lock()
_shared_cache: dict[str, dict] = {}
_mem_cache: dict[str, dict] = {}
SHARED_I18N_MARKER = "shared-components/src/i18n"

PARSER_VERSION = "3.0.0"
SCHEMA_VERSION = 2

EXTRACT_SCRIPT = Path(__file__).parent / "extract-i18n.mjs"


def _node_available() -> bool:
    try:
        subprocess.run(
            ["node", "--version"],
            capture_output=True,
            timeout=5,
            check=True,
        )
        return True
    except Exception:
        return False


def _compute_files_hash(filepaths: list[str]) -> str:
    hasher = hashlib.md5()
    for fp in sorted(filepaths):
        p = Path(fp)
        if p.exists():
            stat = p.stat()
            hasher.update(f"{fp}:{stat.st_size}:{stat.st_mtime_ns}".encode())
    return hasher.hexdigest()


def _cache_key(app_name: str) -> str:
    return f"dicts:{app_name}:v{SCHEMA_VERSION}:p{PARSER_VERSION}"


def _cache_path(app_name: str) -> tuple[Path, Path]:
    key = _cache_key(app_name)
    hashed = hashlib.md5(key.encode()).hexdigest()
    json_path = CACHE_DIR / f"{hashed}.json"
    lock_path = CACHE_DIR / f"{hashed}.lock"
    return json_path, lock_path


def _load_from_cache(app_name: str, files_hash: str) -> dict | None:
    json_path, lock_path = _cache_path(app_name)
    if lock_path.exists():
        try:
            lock_mtime = lock_path.stat().st_mtime
            import time
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
        if data.get("_parser_version") != PARSER_VERSION:
            return None
        if data.get("_schema_version") != SCHEMA_VERSION:
            return None
        return data
    except Exception:
        return None


def _save_to_cache(app_name: str, files_hash: str, data: dict) -> None:
    json_path, lock_path = _cache_path(app_name)
    try:
        lock_path.touch()
        save_data = {
            **data,
            "_files_hash": files_hash,
            "_parser_version": PARSER_VERSION,
            "_schema_version": SCHEMA_VERSION,
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


def parse_i18n_files(filepaths: list[str]) -> dict[str, dict[str, str]]:
    if not _node_available():
        raise RuntimeError(
            "Node.js is required for i18n dictionary parsing. "
            "Please install Node.js (>= 16)."
        )

    existing = [str(resolve_path(f)) for f in filepaths if resolve_path(f).exists()]
    if not existing:
        return {"zh-CN": {}, "en": {}}

    cmd = ["node", str(EXTRACT_SCRIPT)] + existing
    result = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=30,
        cwd=str(Path(__file__).parent.parent.parent),
    )

    if result.returncode != 0:
        raise RuntimeError(
            f"Failed to parse i18n files: {result.stderr.strip()}"
        )

    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON output from extractor: {e}") from e

    return data


def _split_shared(filepaths: list[str]) -> tuple[list[str], list[str]]:
    """把 i18n 文件列表拆成 (shared 文件, app 文件)。shared 文件只解析一次。"""
    shared, app = [], []
    for f in filepaths:
        norm = f.replace("\\", "/")
        if SHARED_I18N_MARKER in norm:
            shared.append(f)
        else:
            app.append(f)
    return shared, app


def _get_shared_dict(shared_files: list[str]) -> dict[str, dict[str, str]]:
    """解析 shared 字典，进程内仅解析一次（多线程共享，防缓存击穿）。

    双重检查锁定：第一个线程持锁解析，其余线程在锁上等待，
    醒来后命中缓存直接返回，避免多个 node 子进程重复解析 shared。
    """
    if not shared_files:
        return {"zh-CN": {}, "en": {}}
    h = _compute_files_hash([str(resolve_path(f)) for f in shared_files])
    perf_tracker.start("Shared字典加载")

    # 第一次检查（无锁快路径）
    cached = _shared_cache.get(h)
    if cached is not None:
        perf_tracker.end("Shared字典加载", len(cached.get("zh-CN", {})), {"cached": True})
        return cached

    with _shared_lock:
        # 第二次检查（持锁）：等待期间可能已被其它线程解析
        cached = _shared_cache.get(h)
        if cached is not None:
            perf_tracker.end("Shared字典加载", len(cached.get("zh-CN", {})), {"cached": "wait"})
            return cached
        # 持锁解析：保证进程内 shared 只冷解析一次
        parsed = parse_i18n_files(shared_files)
        result = {k: dict(v) for k, v in parsed.items() if not str(k).startswith("_")}
        _shared_cache[h] = result

    perf_tracker.end("Shared字典加载", len(result.get("zh-CN", {})), {"cached": False})
    return result


def load_all_dicts(
    app_name: str | None = None,
    use_cache: bool = True,
) -> dict[str, dict[str, str]]:
    perf_tracker.start("字典加载")
    app_config = get_app_config(app_name)

    existing_files = []
    for f in app_config.i18n_files:
        p = resolve_path(f)
        if p.exists():
            existing_files.append(str(p))

    files_hash = _compute_files_hash(existing_files)

    # 进程内内存缓存：同一进程内重复加载（benchmark 冷/热、并行 lint）直接命中
    if files_hash in _mem_cache:
        cached = _mem_cache[files_hash]
        perf_tracker.end("字典加载", len(cached.get("zh-CN", {})), {"cached": "mem", "parser": "node"})
        return {k: v for k, v in cached.items()}

    if use_cache:
        cached = _load_from_cache(app_config.name, files_hash)
        if cached:
            clean = {k: v for k, v in cached.items() if not str(k).startswith("_")}
            _mem_cache[files_hash] = clean
            perf_tracker.end("字典加载", len(clean.get("zh-CN", {})), {"cached": True, "parser": "node"})
            return clean

    # shared 字典仅解析一次（跨 app 复用）；app 专属文件各自解析
    shared_files, app_files = _split_shared(app_config.i18n_files)
    shared_dict = _get_shared_dict(shared_files)
    app_dict = parse_i18n_files(app_files) if app_files else {"zh-CN": {}, "en": {}}

    result: dict[str, dict[str, str]] = {}
    for locale in set(list(shared_dict.keys()) + list(app_dict.keys())):
        if str(locale).startswith("_"):
            continue
        merged = {}
        merged.update(app_dict.get(locale, {}))
        merged.update(shared_dict.get(locale, {}))  # shared 覆盖 app（保持 append-last 语义）
        result[locale] = merged

    if use_cache:
        _save_to_cache(app_config.name, files_hash, result)

    _mem_cache[files_hash] = result
    zh_count = len(result.get("zh-CN", {}))
    perf_tracker.end(
        "字典加载",
        zh_count,
        {"cached": False, "parser": "node", "shared_keys": len(shared_dict.get("zh-CN", {}))},
    )
    return result


def extract_vars_from_value(value: str) -> set[str]:
    import re
    pattern = re.compile(r"\{([^}:]+)(?::[^}]+)?\}")
    vars_set = set()
    for match in pattern.finditer(value):
        var_name = match.group(1).strip()
        if "|" in var_name:
            var_name = var_name.split("|")[1].strip()
        vars_set.add(var_name)
    return vars_set
