"""Kotlin 源码 i18n 调用扫描器 - 提取 Android Kotlin 代码中使用的 i18n key"""
from __future__ import annotations

import re
import os
from pathlib import Path
from collections import defaultdict

from .config import PROJECT_ROOT
from .perf import perf_tracker

KOTLIN_GETSTRING_PATTERN = re.compile(
    r"""(?:getString|getText|getStringArray|getQuantityString)\s*\(\s*R\.string\.([a-zA-Z0-9_]+)"""
)

KOTLIN_RSTRING_PATTERN = re.compile(
    r"""R\.string\.([a-zA-Z0-9_]+)"""
)

QUICK_CHECK_BYTES = b"R.string."


def _android_res_to_key(res_name: str) -> str:
    """将 Android 资源名转回 TS key（_ → .）"""
    return res_name.replace('_', '.')


def extract_kotlin_i18n_keys(
    kotlin_dirs: list[str] | None = None,
) -> dict[str, list[str]]:
    """
    扫描 Kotlin 源码，提取所有使用的 i18n key

    支持模式:
    - getString(R.string.xxx)
    - context.getText(R.string.xxx)
    - R.string.xxx（直接引用）

    Args:
        kotlin_dirs: 要扫描的目录列表，默认 ["app/encv-mobile/android"]

    Returns:
        {key: [file:line, ...]}
    """
    perf_tracker.start("扫描 Kotlin i18n")

    if kotlin_dirs is None:
        kotlin_dirs = ["app/encv-mobile/android"]

    used_keys: dict[str, list[str]] = defaultdict(list)
    found_res_names: set[str] = set()

    for kot_dir in kotlin_dirs:
        dir_path = PROJECT_ROOT / kot_dir
        if not dir_path.exists():
            continue

        for root, dirs, files in os.walk(dir_path):
            if "build" in dirs:
                dirs.remove("build")
            if ".gradle" in dirs:
                dirs.remove(".gradle")

            for fname in files:
                if not fname.endswith(".kt") and not fname.endswith(".java"):
                    continue

                fpath = Path(root) / fname
                try:
                    with open(fpath, "rb") as f:
                        content_bytes = f.read()

                    if QUICK_CHECK_BYTES not in content_bytes:
                        continue

                    content = content_bytes.decode("utf-8", errors="ignore")
                    lines = content.split("\n")
                except Exception:
                    continue

                for line_no, line in enumerate(lines, 1):
                    stripped = line.lstrip()
                    if stripped.startswith("//"):
                        continue
                    if stripped.startswith("*"):
                        continue

                    for pattern in [KOTLIN_GETSTRING_PATTERN, KOTLIN_RSTRING_PATTERN]:
                        for match in pattern.finditer(line):
                            res_name = match.group(1)
                            if res_name in found_res_names:
                                continue
                            found_res_names.add(res_name)

                            key = _android_res_to_key(res_name)
                            rel_path = str(fpath.relative_to(PROJECT_ROOT))
                            used_keys[key].append(f"{rel_path}:{line_no}")

    perf_tracker.end("扫描 Kotlin i18n", len(used_keys))
    return dict(used_keys)

