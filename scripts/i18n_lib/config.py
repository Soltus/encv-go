"""配置管理模块"""
from __future__ import annotations

import json
from pathlib import Path
from dataclasses import dataclass, field

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
DB_PATH = PROJECT_ROOT / ".i18n-cache.db"
CACHE_DIR = PROJECT_ROOT / ".i18n-cache"
PERF_REPORT_PATH = PROJECT_ROOT / "i18n-perf-report.md"

CACHE_DIR.mkdir(exist_ok=True)

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
            "go_dirs": [
                "internal",
                "pkg",
            ],
            "kotlin_dirs": [
                "app/encv-mobile/android/app/src/main/java",
            ],
        }
    },
    "default_app": "encv-mobile",
    "locales": ["zh-CN", "en"],
}

EN_STOPWORDS = {
    "a", "an", "the", "is", "are", "was", "were", "be", "been", "being",
    "have", "has", "had", "do", "does", "did", "will", "would", "could",
    "should", "may", "might", "shall", "can", "need", "dare", "ought",
    "used", "to", "of", "in", "for", "on", "with", "at", "by", "from",
    "as", "into", "through", "during", "before", "after", "above", "below",
    "between", "out", "off", "over", "under", "again", "further", "then",
    "once", "here", "there", "when", "where", "why", "how", "all", "both",
    "each", "few", "more", "most", "other", "some", "such", "no", "nor",
    "not", "only", "own", "same", "so", "than", "too", "very", "just",
    "and", "but", "or", "if", "because", "until", "while",
}


@dataclass
class AppConfig:
    name: str
    src_dirs: list[str]
    i18n_files: list[str]
    types_output: str
    go_dirs: list[str] = field(default_factory=list)
    kotlin_dirs: list[str] = field(default_factory=list)


_config_cache: dict | None = None


def load_config() -> dict:
    global _config_cache
    if _config_cache is not None:
        return _config_cache

    config_path = PROJECT_ROOT / "i18n.config.json"
    if config_path.exists():
        try:
            with open(config_path, "r", encoding="utf-8") as f:
                _config_cache = json.load(f)
                return _config_cache
        except Exception:
            pass

    _config_cache = DEFAULT_CONFIG
    return _config_cache


def get_app_config(app_name: str | None = None) -> AppConfig:
    config = load_config()
    app_name = app_name or config.get("default_app", "encv-mobile")
    apps = config.get("apps", {})

    if app_name not in apps:
        raise ValueError(f"App '{app_name}' not found in config. Available: {list(apps.keys())}")

    app_cfg = apps[app_name]
    return AppConfig(
        name=app_name,
        src_dirs=app_cfg.get("src_dirs", []),
        i18n_files=app_cfg.get("i18n_files", []),
        types_output=app_cfg.get("types_output", ""),
        go_dirs=app_cfg.get("go_dirs", []),
        kotlin_dirs=app_cfg.get("kotlin_dirs", []),
    )


def resolve_path(relative_path: str) -> Path:
    return PROJECT_ROOT / relative_path
