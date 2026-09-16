"""i18n key 迁移 - 跨字典 / app → shared 的一次性或批量移动。

设计目标：
- 复用 loader（shared 去重、缓存）与 addkey（结构化写字典），不重复造轮子。
- 幂等：目标已存在则跳过；--keep 控制是否保留源。
- --dry-run 仅规划、不落盘，可安全用于 benchmark 与预检。
- 双 locale 对齐；prefix.（结尾）表示子树批量。
"""
from __future__ import annotations

import re
from pathlib import Path

from .config import get_app_config, resolve_path, PROJECT_ROOT
from .loader import load_all_dicts
from .addkey import add_key
from .perf import perf_tracker

SHARED_I18N_DIR = PROJECT_ROOT / "app" / "packages" / "shared-components" / "src" / "i18n"


def _match_keys(spec: str, dicts: dict[str, dict[str, str]]) -> list[str]:
    """匹配 key。三种语义：
    - 尾星 `tasks.report*` → 字符串前缀（匹配 tasks.reportTitle 等驼峰 key）
    - 尾点 `tasks.performance.` → 点边界子树（匹配 tasks.performance.xxx）
    - 否则 → 精确匹配单个 key
    """
    keys = set(dicts.get("zh-CN", {})) | set(dicts.get("en", {}))
    if spec.endswith("*"):
        p = spec[:-1]
        return sorted(k for k in keys if k.startswith(p))
    if spec.endswith("."):
        p = spec[:-1]
        return sorted(k for k in keys if k == p or k.startswith(p + "."))
    return [spec] if spec in keys else []


def _target_file_for(to_target: str, key: str, from_app: str) -> str:
    """返回目标文件的『相对路径』（供 resolve_path / add_key 使用）。"""
    prefix = key.split(".")[0]
    if to_target == "shared":
        tgt = SHARED_I18N_DIR / f"{prefix}.ts"
        return str(tgt.relative_to(PROJECT_ROOT))
    app_cfg = get_app_config(from_app)
    for f in app_cfg.i18n_files:
        if Path(f).stem == prefix:
            return f
    return app_cfg.i18n_files[0]


def _remove_key_from_file(key: str, filepath: str) -> bool:
    p = resolve_path(filepath)
    if not p.exists():
        return False
    lines = p.read_text(encoding="utf-8").split("\n")
    pat = re.compile(rf'["\']{re.escape(key)}["\']\s*:')
    new_lines = [ln for ln in lines if not pat.search(ln)]
    if len(new_lines) != len(lines):
        p.write_text("\n".join(new_lines), encoding="utf-8")
        return True
    return False


def _register_shared_module(module_name: str, index_rel: str) -> bool:
    """把 module 注册进 shared i18n/index.ts 的 sharedI18nModules。返回是否改动。

    幂等：import 行与数组元素都已存在则跳过（避免重复注册，如 tasks 出现两次）。
    """
    idx_path = resolve_path(index_rel)
    if not idx_path.exists():
        return False
    content = idx_path.read_text(encoding="utf-8")
    import_line = f'import {module_name} from "./{module_name}";'

    array_match = re.search(
        r"export const sharedI18nModules: MessageModule\[\] = \[(.*?)\];", content, re.DOTALL
    )
    # 数组元素级匹配：module_name 作为独立元素（前接开头或逗号，后接逗号、] 或字符串结尾）
    already_registered = bool(array_match) and bool(
        re.search(rf'(^|,\s*){re.escape(module_name)}(\s*,|\s*\]|\s*$)', array_match.group(1))
    )

    changed = False
    if import_line not in content:
        content = content.replace(
            'import settings from "./settings";',
            f'import settings from "./settings";\n{import_line}',
        )
        changed = True
    if not already_registered:
        content = re.sub(
            r"export const sharedI18nModules: MessageModule\[\] = \[(.*?)\];",
            lambda m: f"export const sharedI18nModules: MessageModule[] = [{m.group(1)}, {module_name}];",
            content,
            count=1,
        )
        changed = True
    if changed:
        idx_path.write_text(content, encoding="utf-8")
    return changed


def move_key(
    spec: str,
    from_app: str = "encv-mobile",
    to_target: str = "shared",
    keep: bool = False,
    dry_run: bool = False,
    register: bool = False,
) -> dict:
    """
    把匹配 spec 的 key 从 from_app 迁移到 to_target（"shared" 或某 app 名）。

    Returns: {success, moved, skipped, removed, matched, dry_run, registered}
    """
    perf_tracker.start("Key迁移")
    src = load_all_dicts(from_app)
    matched = _match_keys(spec, src)

    if not matched:
        perf_tracker.end("Key迁移", 0, {"matched": 0, "dry_run": dry_run})
        return {
            "success": True, "moved": 0, "skipped": 0, "removed": 0,
            "matched": 0, "dry_run": dry_run, "registered": False,
        }

    moved = skipped = removed = 0
    target_stems: set[str] = set()
    app_cfg = get_app_config(from_app)

    for key in matched:
        zh = src.get("zh-CN", {}).get(key, "")
        en = src.get("en", {}).get(key, "")
        tgt_rel = _target_file_for(to_target, key, from_app)
        target_stems.add(Path(tgt_rel).stem)

        if dry_run:
            moved += 1
            continue

        res = add_key(key, zh, en, target_file=tgt_rel)
        if not res.get("success") and res.get("reason") == "already_exists":
            skipped += 1
        else:
            moved += 1
            if not keep:
                for sf in app_cfg.i18n_files:
                    if _remove_key_from_file(key, sf):
                        removed += 1
                        break

    registered = False
    if register and to_target == "shared" and not dry_run:
        for stem in target_stems:
            if _register_shared_module(stem, "app/packages/shared-components/src/i18n/index.ts"):
                registered = True

    perf_tracker.end("Key迁移", moved, {
        "matched": len(matched),
        "skipped": skipped,
        "removed": removed,
        "dry_run": dry_run,
        "registered": registered,
    })
    return {
        "success": True, "moved": moved, "skipped": skipped, "removed": removed,
        "matched": len(matched), "dry_run": dry_run, "registered": registered,
    }


def cmd_move_key(args) -> int:
    result = move_key(
        args.key,
        from_app=args.app or "encv-mobile",
        to_target=args.to,
        keep=args.keep,
        dry_run=args.dry_run,
        register=args.register,
    )
    if args.dry_run:
        print(f"🔍 [dry-run] 将迁移 {result['matched']} 个 key（不落盘）")
    else:
        print(f"✅ 迁移完成: moved={result['moved']} skipped={result['skipped']} "
              f"removed_from_source={result['removed']} registered={result['registered']}")
    return 0
