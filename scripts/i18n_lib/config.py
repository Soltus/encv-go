"""配置管理模块 - 支持自动发现 i18n app"""
from __future__ import annotations

import json
from pathlib import Path
from dataclasses import dataclass, field

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
DB_PATH = PROJECT_ROOT / ".i18n-cache.db"
CACHE_DIR = PROJECT_ROOT / ".i18n-cache"
PERF_REPORT_PATH = PROJECT_ROOT / "i18n-perf-report.md"

CACHE_DIR.mkdir(exist_ok=True)

I18N_DICT_PATTERNS = {"zh-CN", "en"}


def _discover_apps() -> dict:
    """
    自动发现项目中所有使用 i18n 的 app。

    发现规则：
    1. 查找所有包含 i18n/ 目录且目录下有 .ts 文件的项目目录
    2. 项目目录通常位于 app/ 下，或 app/*/web/ 下（插件内嵌 webview）
    3. 自动检测 src_dirs / i18n_files / go_dirs / kotlin_dirs
    """
    discovered = {}
    search_roots = [
        PROJECT_ROOT / "app",
    ]

    candidate_dirs = []
    for root in search_roots:
        if not root.exists():
            continue
        for child in root.iterdir():
            if not child.is_dir():
                continue
            if child.name.startswith(".") or child.name.startswith("_"):
                continue
            if child.name in {"node_modules", "build", "dist", ".git", "target"}:
                continue
            candidate_dirs.append(child)
            web_dir = child / "web"
            if web_dir.exists() and web_dir.is_dir():
                candidate_dirs.append(web_dir)

    for proj_dir in candidate_dirs:
        i18n_dir = proj_dir / "src" / "i18n"
        if not (i18n_dir.exists() and i18n_dir.is_dir()):
            continue

        dict_files = []
        for ts_file in sorted(i18n_dir.glob("*.ts")):
            if ts_file.name in {"index.ts", "init.ts"}:
                continue
            try:
                content = ts_file.read_text(encoding="utf-8", errors="ignore")
                has_zh = '"zh-CN"' in content or "'zh-CN'" in content
                has_en = "en:" in content or '"en"' in content or "'en'" in content
                if has_zh or has_en:
                    rel = ts_file.relative_to(PROJECT_ROOT).as_posix()
                    dict_files.append(rel)
            except Exception:
                continue

        if not dict_files:
            continue

        src_dirs = []
        for sub in ["src", "views", "components", "composables"]:
            d = proj_dir / sub
            if d.exists() and d.is_dir():
                rel = d.relative_to(PROJECT_ROOT).as_posix()
                src_dirs.append(rel)

        go_dirs = []
        go_mod = PROJECT_ROOT / "go.mod"
        if go_mod.exists() and proj_dir.name == "encv-mobile":
            go_dirs = ["internal", "pkg"]

        kotlin_dirs = []
        android_kotlin = proj_dir / "android" / "app" / "src" / "main" / "java"
        if android_kotlin.exists():
            rel = android_kotlin.relative_to(PROJECT_ROOT).as_posix()
            kotlin_dirs.append(rel)

        app_name = proj_dir.name
        if proj_dir.parent.name == "web" and proj_dir.parent.parent.name in {"plugin-openlist"}:
            app_name = proj_dir.parent.parent.name

        existing = discovered.get(app_name, {})
        discovered[app_name] = {
            "src_dirs": sorted(set(existing.get("src_dirs", []) + src_dirs)),
            "i18n_files": sorted(set(existing.get("i18n_files", []) + dict_files)),
            "types_output": existing.get("types_output", ""),
            "go_dirs": sorted(set(existing.get("go_dirs", []) + go_dirs)),
            "kotlin_dirs": sorted(set(existing.get("kotlin_dirs", []) + kotlin_dirs)),
        }

    shared_i18n = PROJECT_ROOT / "app" / "packages" / "shared-components" / "src" / "i18n"
    if shared_i18n.exists():
        shared_files = []
        for ts_file in sorted(shared_i18n.glob("*.ts")):
            if ts_file.name in {"index.ts", "init.ts", "generated-types.ts"}:
                continue
            rel = ts_file.relative_to(PROJECT_ROOT).as_posix()
            shared_files.append(rel)

        for name in discovered:
            for sf in shared_files:
                if sf not in discovered[name]["i18n_files"]:
                    discovered[name]["i18n_files"].append(sf)
            shared_src = "app/packages/shared-components/src"
            if shared_src not in discovered[name]["src_dirs"]:
                discovered[name]["src_dirs"].append(shared_src)
            discovered[name]["i18n_files"].sort()
            discovered[name]["src_dirs"].sort()

    if "encv-mobile" in discovered and not discovered["encv-mobile"].get("types_output"):
        discovered["encv-mobile"]["types_output"] = "app/packages/shared-components/src/i18n/generated-types.ts"

    return discovered


DEFAULT_CONFIG: dict = {
    "apps": {},
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

    config = dict(DEFAULT_CONFIG)

    config_path = PROJECT_ROOT / "i18n.config.json"
    file_config = {}
    if config_path.exists():
        try:
            with open(config_path, "r", encoding="utf-8") as f:
                file_config = json.load(f)
        except Exception:
            pass

    discovered = _discover_apps()

    merged_apps = {}
    for name, auto_cfg in discovered.items():
        merged_apps[name] = dict(auto_cfg)
    if "apps" in file_config:
        for name, cfg in file_config["apps"].items():
            if name in merged_apps:
                for k, v in cfg.items():
                    if isinstance(v, list):
                        merged_apps[name][k] = sorted(set(merged_apps[name].get(k, []) + v))
                    else:
                        merged_apps[name][k] = v
            else:
                merged_apps[name] = dict(cfg)

    config["apps"] = merged_apps
    if "default_app" in file_config:
        config["default_app"] = file_config["default_app"]
    if "locales" in file_config:
        config["locales"] = file_config["locales"]

    _config_cache = config
    return config


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


def get_all_app_names() -> list[str]:
    config = load_config()
    return list(config.get("apps", {}).keys())


def resolve_path(relative_path: str) -> Path:
    return PROJECT_ROOT / relative_path
