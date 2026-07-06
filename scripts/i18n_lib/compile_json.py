"""TS 字典编译为 JSON - 供 Go/Python 等多语言复用"""
from __future__ import annotations

import json
import subprocess
import os
from pathlib import Path

from .config import get_app_config, PROJECT_ROOT, resolve_path
from .perf import perf_tracker

COMPILE_SCRIPT = Path(__file__).parent / "compile-json.mjs"


def compile_to_json(
    app_name: str | None = None,
    output_dir: str | None = None,
) -> dict:
    """
    将 TS 字典编译为 JSON 文件

    Args:
        app_name: 应用名称
        output_dir: 输出目录，默认 internal/i18n/locales

    Returns:
        编译结果信息
    """
    perf_tracker.start("编译 JSON")

    app_cfg = get_app_config(app_name)

    if output_dir is None:
        output_dir = str(PROJECT_ROOT / "internal" / "i18n" / "locales")

    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    existing = []
    for f in app_cfg.i18n_files:
        p = resolve_path(f)
        if p.exists():
            existing.append(str(p))

    if not existing:
        raise FileNotFoundError("No i18n dictionary files found")

    cmd = ["node", str(COMPILE_SCRIPT), str(output_path)] + existing

    result = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=60,
        cwd=str(PROJECT_ROOT),
    )

    if result.returncode != 0:
        raise RuntimeError(
            f"Node.js dictionary compilation failed:\n{result.stderr}"
        )

    locale_files = list(output_path.glob("*.json"))
    locale_count = len(locale_files)
    total_keys = 0
    for lf in locale_files:
        with open(lf, "r", encoding="utf-8") as f:
            data = json.load(f)
            total_keys += len(data)

    perf_tracker.end(
        "编译 JSON",
        total_keys,
        {"locales": locale_count, "output_dir": output_dir},
    )

    return {
        "output_dir": str(output_path),
        "locale_count": locale_count,
        "total_keys": total_keys,
        "files": [str(f) for f in sorted(locale_files)],
        "output": result.stdout,
    }
