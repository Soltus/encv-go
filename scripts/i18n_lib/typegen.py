"""类型生成模块 - 生成 TypeScript 类型定义"""
from __future__ import annotations

from .config import get_app_config, resolve_path
from .loader import load_all_dicts
from .perf import perf_tracker


def generate_types(app_name: str | None = None) -> str:
    perf_tracker.start("类型生成")
    app_config = get_app_config(app_name)
    dicts = load_all_dicts(app_name)
    zh_keys = sorted(dicts.get("zh-CN", {}).keys())

    type_lines = [
        "// AUTO-GENERATED FILE - DO NOT EDIT",
        "// 由 i18n-tool.py gen-types 自动生成",
        "",
        'export type Locale = "zh-CN" | "en";',
        "",
        "export type MessageParamValue = string | number | boolean;",
        "export type MessageParams = Record<string, MessageParamValue>;",
        "export type TFunction = (key: I18nKey, params?: MessageParams) => string;",
        "export type TFieldFunction = (key: string) => string;",
        "export type TSectionTitleFunction = (title: string) => string;",
        "",
        "export type MessageModule = { \"zh-CN\": Record<string, string>; en: Record<string, string> };",
        "",
        "export type I18nKey =",
    ]

    for i, key in enumerate(zh_keys):
        suffix = "," if i < len(zh_keys) - 1 else ";"
        type_lines.append(f'  | "{key}"{suffix}')

    type_lines.append("")

    perf_tracker.end("类型生成", len(zh_keys))
    return "\n".join(type_lines)


def cmd_gen_types(app_name: str | None = None) -> str:
    app_config = get_app_config(app_name)
    output_path = resolve_path(app_config.types_output)
    types_content = generate_types(app_name)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(types_content, encoding="utf-8")

    print(f"✅ 类型生成完成: {output_path}")
    print(f"   生成 {len(types_content.splitlines())} 行类型定义")
    return str(output_path)
