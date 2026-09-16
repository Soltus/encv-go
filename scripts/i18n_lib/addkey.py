"""添加 key 到 TS 字典文件"""
from __future__ import annotations

import re
from pathlib import Path

from .config import get_app_config, resolve_path
from .loader import load_all_dicts


def _guess_target_file(key: str, app_name: str | None = None) -> str:
    """根据 key 前缀猜测应该放到哪个字典文件"""
    app_cfg = get_app_config(app_name)
    dicts = load_all_dicts(app_name)

    prefix = key.split(".")[0]

    for filepath in app_cfg.i18n_files:
        p = resolve_path(filepath)
        if not p.exists():
            continue
        filename = p.stem
        if filename == prefix:
            return filepath

    zh_dict = dicts.get("zh-CN", {})
    file_for_prefix: dict[str, str] = {}
    for k in zh_dict:
        k_prefix = k.split(".")[0]
        if k_prefix not in file_for_prefix:
            for filepath in app_cfg.i18n_files:
                fp = resolve_path(filepath)
                content = fp.read_text(encoding="utf-8")
                if f'"{k}"' in content:
                    file_for_prefix[k_prefix] = filepath
                    break

    if prefix in file_for_prefix:
        return file_for_prefix[prefix]

    return app_cfg.i18n_files[0]


def add_key(
    key: str,
    zh_value: str,
    en_value: str,
    app_name: str | None = None,
    target_file: str | None = None,
) -> dict:
    """
    添加一个翻译 key 到 TS 字典文件

    Args:
        key: 翻译 key
        zh_value: 中文翻译
        en_value: 英文翻译
        app_name: 应用名称
        target_file: 目标文件，不指定则自动猜测

    Returns:
        操作结果
    """
    if target_file is None:
        target_file = _guess_target_file(key, app_name)

    filepath = resolve_path(target_file)
    if not filepath.exists():
        raise FileNotFoundError(f"Dictionary file not found: {filepath}")

    content = filepath.read_text(encoding="utf-8")

    if f'"{key}"' in content:
        return {
            "success": False,
            "reason": "already_exists",
            "key": key,
            "file": str(filepath),
        }

    def insert_key_into_section(content: str, locale: str, key: str, value: str) -> str:
        # locale key 可能带引号（"zh-CN": {）或不带引号（en: {），两种都匹配
        pattern = re.compile(
            rf'(["\']?{re.escape(locale)}["\']?\s*:\s*\{{)',
        )
        match = pattern.search(content)
        if not match:
            return content

        insert_pos = match.end()
        # value 含双引号时改用单引号包裹，避免破坏 TS 字符串语法
        # （字典现有约定：含双引号的 value 用单引号包裹，如 '搜索 "{query}"'）
        if '"' in value:
            if "'" in value:
                # 同时含单双引号：转义双引号后仍以双引号包裹
                escaped = value.replace("\\", "\\\\").replace('"', '\\"')
                new_entry = f'\n    "{key}": "{escaped}",'
            else:
                new_entry = f'\n    "{key}": \'{value}\','
        else:
            new_entry = f'\n    "{key}": "{value}",'
        return content[:insert_pos] + new_entry + content[insert_pos:]

    content = insert_key_into_section(content, "zh-CN", key, zh_value)
    content = insert_key_into_section(content, "en", key, en_value)

    filepath.write_text(content, encoding="utf-8")

    return {
        "success": True,
        "key": key,
        "zh": zh_value,
        "en": en_value,
        "file": str(filepath),
    }
