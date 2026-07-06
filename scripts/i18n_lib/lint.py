"""Lint 检查模块 - 增强的 i18n 错误检测与报告"""
from __future__ import annotations

import re
from dataclasses import dataclass, field
from collections import defaultdict

from .scanner import extract_used_keys
from .loader import load_all_dicts, extract_vars_from_value
from .search import find_near_duplicates_lsh
from .perf import perf_tracker
from .config import get_app_config


@dataclass
class I18nIssue:
    level: str  # error, warning, info
    code: str  # 错误码，如 MISSING_KEY, VAR_MISMATCH 等
    message: str
    key: str = ""
    locale: str = ""
    file: str = ""
    line: int = 0
    suggestion: str = ""
    details: dict = field(default_factory=dict)

    def format(self) -> str:
        location = ""
        if self.file:
            location = f"  {self.file}"
            if self.line:
                location += f":{self.line}"

        icon = {"error": "❌", "warning": "⚠️", "info": "ℹ️"}.get(self.level, "•")
        lines = [f"{icon} [{self.code}] {self.message}"]
        if self.key:
            lines.append(f"  key: {self.key}")
        if self.locale:
            lines.append(f"  locale: {self.locale}")
        if location:
            lines.append(location)
        if self.suggestion:
            lines.append(f"  💡 建议: {self.suggestion}")
        return "\n".join(lines)


def check_missing_keys(
    used_keys: dict[str, list[str]],
    dicts: dict[str, dict[str, str]],
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    zh_dict = dicts.get("zh-CN", {})
    en_dict = dicts.get("en", {})

    for key, files in used_keys.items():
        missing_zh = key not in zh_dict
        missing_en = key not in en_dict

        if missing_zh and missing_en:
            for f in files:
                parts = f.rsplit(":", 1)
                file_path = parts[0]
                line_num = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else 0
                issues.append(I18nIssue(
                    level="error",
                    code="MISSING_KEY",
                    message=f"翻译 key 缺失: '{key}' 在所有语言中都不存在",
                    key=key,
                    locale="all",
                    file=file_path,
                    line=line_num,
                    suggestion=f"在字典文件中添加 '{key}' 的中英文翻译",
                ))
        elif missing_zh:
            issues.append(I18nIssue(
                level="warning",
                code="MISSING_ZH",
                message=f"中文翻译缺失: '{key}'",
                key=key,
                locale="zh-CN",
                suggestion=f"为 key '{key}' 添加中文翻译，当前英文: {en_dict.get(key, '')[:50]}",
            ))
        elif missing_en:
            issues.append(I18nIssue(
                level="warning",
                code="MISSING_EN",
                message=f"英文翻译缺失: '{key}'",
                key=key,
                locale="en",
                suggestion=f"为 key '{key}' 添加英文翻译，当前中文: {zh_dict.get(key, '')[:50]}",
            ))

    return issues


def check_variable_consistency(
    dicts: dict[str, dict[str, str]],
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    zh_dict = dicts.get("zh-CN", {})
    en_dict = dicts.get("en", {})

    common_keys = set(zh_dict.keys()) & set(en_dict.keys())

    for key in sorted(common_keys):
        zh_vars = extract_vars_from_value(zh_dict[key])
        en_vars = extract_vars_from_value(en_dict[key])

        if zh_vars != en_vars:
            only_zh = zh_vars - en_vars
            only_en = en_vars - zh_vars

            details = []
            if only_zh:
                details.append(f"仅中文有: {', '.join(sorted(only_zh))}")
            if only_en:
                details.append(f"仅英文有: {', '.join(sorted(only_en))}")

            issues.append(I18nIssue(
                level="error",
                code="VAR_MISMATCH",
                message=f"变量不一致: '{key}' 的中英文参数不匹配",
                key=key,
                suggestion="检查翻译中的 {{变量}}，确保中英文使用相同的变量名和数量",
                details={"zh_vars": sorted(zh_vars), "en_vars": sorted(en_vars)},
            ))

    return issues


def check_near_duplicates(
    dicts: dict[str, dict[str, str]],
    locale: str = "zh-CN",
    threshold: float = 0.85,
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    target_dict = dicts.get(locale, {})

    items = [(k, v) for k, v in target_dict.items() if len(v) >= 10]
    dups = find_near_duplicates_lsh(items, locale, threshold)

    for ratio, k1, v1, k2, v2 in dups:
        issues.append(I18nIssue(
            level="warning",
            code="NEAR_DUPLICATE",
            message=f"近重复翻译 ({ratio*100:.1f}% 相似): '{k1}' vs '{k2}'",
            key=k1,
            locale=locale,
            suggestion="考虑合并或统一这两个翻译，减少维护成本",
            details={"other_key": k2, "similarity": ratio},
        ))

    return issues


def check_unused_keys(
    used_keys: dict[str, list[str]],
    dicts: dict[str, dict[str, str]],
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    zh_dict = dicts.get("zh-CN", {})

    for key in sorted(zh_dict.keys()):
        if key not in used_keys:
            issues.append(I18nIssue(
                level="info",
                code="UNUSED_KEY",
                message=f"未使用的翻译 key: '{key}'",
                key=key,
                suggestion="确认该 key 是否真的不需要，可以删除以减少字典体积",
            ))

    return issues


def check_duplicate_keys(
    dicts: dict[str, dict[str, str]],
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    for locale, dict_data in dicts.items():
        key_count: dict[str, int] = defaultdict(int)
        for key in dict_data:
            key_count[key] += 1
        for key, count in key_count.items():
            if count > 1:
                issues.append(I18nIssue(
                    level="error",
                    code="DUPLICATE_KEY",
                    message=f"重复 key: '{key}' 在 {locale} 中出现 {count} 次",
                    key=key,
                    locale=locale,
                    suggestion="删除重复的 key，只保留一个定义",
                    details={"count": count},
                ))
    return issues


def check_duplicate_values(
    dicts: dict[str, dict[str, str]],
    locale: str = "zh-CN",
    min_length: int = 5,
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    target_dict = dicts.get(locale, {})

    value_to_keys: dict[str, list[str]] = defaultdict(list)
    for key, value in target_dict.items():
        if len(value) >= min_length:
            value_to_keys[value].append(key)

    for value, keys in value_to_keys.items():
        if len(keys) > 1:
            issues.append(I18nIssue(
                level="warning",
                code="DUPLICATE_VALUE",
                message=f"重复 value: '{value[:50]}' 对应 {len(keys)} 个 key",
                key=keys[0],
                locale=locale,
                suggestion=f"考虑合并这些 key: {', '.join(keys[:5])}",
                details={"keys": keys, "value": value},
            ))

    return issues


def check_english_quality(
    dicts: dict[str, dict[str, str]],
) -> list[I18nIssue]:
    issues: list[I18nIssue] = []
    en_dict = dicts.get("en", {})

    common_mistakes = {
        r"\bthe the\b": "重复冠词 'the the'",
        r"\ba a\b": "重复冠词 'a a'",
        r"\bis is\b": "重复动词 'is is'",
        r"\bare are\b": "重复动词 'are are'",
        r"[A-Z]{3,}": "可能全大写，建议检查",
    }

    for key, value in en_dict.items():
        lower_val = value.lower()

        for pattern, desc in common_mistakes.items():
            if re.search(pattern, lower_val):
                issues.append(I18nIssue(
                    level="warning",
                    code="EN_QUALITY",
                    message=f"英文翻译可能有问题: '{desc}'",
                    key=key,
                    locale="en",
                    suggestion=f"检查英文翻译: '{value[:60]}'",
                ))
                break

    return issues


def run_all_checks(
    app_name: str | None = None,
    include_unused: bool = False,
    include_dup: bool = False,
    include_dup_value: bool = False,
    include_go: bool = True,
    include_kotlin: bool = True,
) -> dict:
    perf_tracker.start("Lint 检查")

    used_keys = extract_used_keys(app_name)

    if include_go:
        try:
            from .scanner_go import extract_go_i18n_keys
            app_cfg = get_app_config(app_name)
            if app_cfg.go_dirs:
                go_keys = extract_go_i18n_keys(app_cfg.go_dirs)
                for key, files in go_keys.items():
                    if key in used_keys:
                        used_keys[key].extend(files)
                    else:
                        used_keys[key] = files
        except Exception:
            pass

    if include_kotlin:
        try:
            from .scanner_kotlin import extract_kotlin_i18n_keys
            app_cfg = get_app_config(app_name)
            if app_cfg.kotlin_dirs:
                kt_keys = extract_kotlin_i18n_keys(app_cfg.kotlin_dirs)
                for key, files in kt_keys.items():
                    if key in used_keys:
                        used_keys[key].extend(files)
                    else:
                        used_keys[key] = files
        except Exception:
            pass

    dicts = load_all_dicts(app_name)

    all_issues: list[I18nIssue] = []

    all_issues.extend(check_missing_keys(used_keys, dicts))
    all_issues.extend(check_variable_consistency(dicts))
    all_issues.extend(check_duplicate_keys(dicts))
    all_issues.extend(check_english_quality(dicts))

    if include_dup:
        all_issues.extend(check_near_duplicates(dicts, "zh-CN"))
        all_issues.extend(check_near_duplicates(dicts, "en"))

    if include_dup_value:
        all_issues.extend(check_duplicate_values(dicts, "zh-CN"))
        all_issues.extend(check_duplicate_values(dicts, "en"))

    if include_unused:
        all_issues.extend(check_unused_keys(used_keys, dicts))

    errors = [i for i in all_issues if i.level == "error"]
    warnings = [i for i in all_issues if i.level == "warning"]
    infos = [i for i in all_issues if i.level == "info"]

    perf_tracker.end("Lint 检查", len(all_issues), {
        "errors": len(errors),
        "warnings": len(warnings),
        "infos": len(infos),
    })

    return {
        "issues": all_issues,
        "errors": errors,
        "warnings": warnings,
        "infos": infos,
        "total": len(all_issues),
        "used_keys_count": len(used_keys),
        "dict_keys_count": len(dicts.get("zh-CN", {})),
    }
