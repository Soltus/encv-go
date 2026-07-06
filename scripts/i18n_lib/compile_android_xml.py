"""TS 字典编译为 Android strings.xml - 供 Kotlin 复用"""
from __future__ import annotations

import subprocess
from pathlib import Path

from .config import get_app_config, PROJECT_ROOT, resolve_path
from .perf import perf_tracker

COMPILE_SCRIPT = Path(__file__).parent / "compile-android-xml.mjs"


def compile_to_android_xml(
    app_name: str | None = None,
    output_dir: str | None = None,
) -> dict:
    """
    将 TS 字典编译为 Android strings.xml

    Args:
        app_name: 应用名称
        output_dir: 输出目录，默认 app/encv-mobile/android/app/src/main/res

    Returns:
        编译结果信息
    """
    perf_tracker.start("编译 Android XML")

    app_cfg = get_app_config(app_name)

    if output_dir is None:
        output_dir = str(
            PROJECT_ROOT
            / "app"
            / "encv-mobile"
            / "android"
            / "app"
            / "src"
            / "main"
            / "res"
        )

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
            f"Android XML compilation failed:\n{result.stderr}"
        )

    locale_dirs = [
        d for d in output_path.iterdir()
        if d.is_dir() and (d.name == "values" or d.name.startswith("values-"))
        and (d / "strings.xml").exists()
    ]

    locale_count = len(locale_dirs)
    total_keys = 0
    for ld in locale_dirs:
        xml_content = (ld / "strings.xml").read_text(encoding="utf-8")
        total_keys += xml_content.count("<string name=")

    perf_tracker.end(
        "编译 Android XML",
        total_keys,
        {"locales": locale_count, "output_dir": output_dir},
    )

    return {
        "output_dir": str(output_path),
        "locale_count": locale_count,
        "total_keys": total_keys,
        "files": [str(d / "strings.xml") for d in sorted(locale_dirs)],
        "output": result.stdout,
    }
