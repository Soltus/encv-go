"""Go 源码 i18n 调用扫描器 - 提取 Go 代码中使用的 i18n key"""
from __future__ import annotations

import re
import os
from pathlib import Path
from collections import defaultdict

from .config import PROJECT_ROOT, get_app_config
from .perf import perf_tracker

GO_I18N_PATTERN = re.compile(
    r"""i18n\.\s*T(?:With)?\s*\(\s*['"`]([^'"`]+)['"`]"""
)

COMMENT_PATTERN = re.compile(r'^\s*//')


def extract_go_i18n_keys(
    go_dirs: list[str] | None = None,
    use_cache: bool = True,
) -> dict[str, list[str]]:
    """
    扫描 Go 源码，提取所有 i18n.T() / i18n.TWith() 调用的 key

    Args:
        go_dirs: 要扫描的 Go 目录列表，默认 ["internal", "pkg"]
        use_cache: 是否使用缓存

    Returns:
        {key: [file:line, ...]}
    """
    perf_tracker.start("扫描 Go i18n")

    if go_dirs is None:
        go_dirs = ["internal", "pkg"]

    used_keys: dict[str, list[str]] = defaultdict(list)

    for go_dir in go_dirs:
        dir_path = PROJECT_ROOT / go_dir
        if not dir_path.exists():
            continue

        for root, dirs, files in os.walk(dir_path):
            if "vendor" in dirs:
                dirs.remove("vendor")
            if ".git" in dirs:
                dirs.remove(".git")

            for fname in files:
                if not fname.endswith(".go"):
                    continue

                fpath = Path(root) / fname
                try:
                    with open(fpath, "r", encoding="utf-8", errors="ignore") as f:
                        lines = f.readlines()
                except Exception:
                    continue

                for line_no, line in enumerate(lines, 1):
                    stripped = line.lstrip()
                    if stripped.startswith("//"):
                        continue

                    for match in GO_I18N_PATTERN.finditer(line):
                        key = match.group(1)
                        rel_path = str(fpath.relative_to(PROJECT_ROOT))
                        used_keys[key].append(f"{rel_path}:{line_no}")

    perf_tracker.end("扫描 Go i18n", len(used_keys))
    return dict(used_keys)
