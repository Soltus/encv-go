#!/usr/bin/env python3
"""
消除 `.codebuddy/skill-registry.json` 的 **updatedAt 时间戳噪音**（不忽略该文件）。

背景
────
IDE / MCP 客户端每次扫描技能目录都会重写本文件，把全部条目的 `updatedAt` 刷成同一时刻，
但内容哈希 `hash` 一个都没变 ⇒ `git status` 常年挂红，纯噪音。
用户要求：**文件必须继续被 git 跟踪、真实变更必须看得见**，所以不能用
`.gitignore` / `assume-unchanged` / `skip-worktree`（那些会让真变更一起消失）。

策略
────
比较「去掉 updatedAt 后的规范化内容」：
  · 无实质变化（各技能 path/hash/description/source 都相同）
        ⇒ 把每个条目的 `updatedAt` **回填成 HEAD 里的原值** ⇒ 工作区与 HEAD 一致，噪音归零。
  · 有实质变化（新增/删除技能、内容变 => hash 变）
        ⇒ **保留新时间戳不动**，并打印 REAL-CHANGE 提示 ⇒ 真变更照样进 git status/commit。

幂等：多次运行结果一致。

用法
────
    python3 scripts/normalize-skill-registry.py           # 就地归一化（默认）
    python3 scripts/normalize-skill-registry.py --check   # 只检查，不改文件（CI 可用）

退出码
────
    0 = 已干净 / 已归一化成功
    1 = 检测到真实变更（需要人看一眼）
    2 = 用法或 IO 错误
"""

import io
import json
import subprocess
import sys

TARGET = ".codebuddy/skill-registry.json"


def read_file(path):
    """读工作区文件。"""
    with io.open(path, encoding="utf-8") as f:
        return f.read()


def read_head(path):
    """读 HEAD 版本；文件尚未入库时返回 None。"""
    try:
        out = subprocess.check_output(
            ["git", "show", "HEAD:{}".format(path)],
            stderr=subprocess.DEVNULL,
        )
        return out.decode("utf-8")
    except Exception:
        return None


def strip_updated_at(obj):
    """深拷贝并剔除所有 updatedAt，得到内容指纹。"""
    if isinstance(obj, dict):
        return {k: strip_updated_at(v) for k, v in obj.items() if k != "updatedAt"}
    if isinstance(obj, list):
        return [strip_updated_at(v) for v in obj]
    return obj


def canonical(text):
    """规范化 JSON：排序 key、去尾空格，用于稳定比较。"""
    return json.dumps(strip_updated_at(json.loads(text)), sort_keys=True, ensure_ascii=False, indent=2)


def collect_updated_at_map(text):
    """提取 {技能名: updatedAt}（同时支持顶层字段未来也带时间戳的情况）。"""
    doc = json.loads(text)
    out = {}
    for name, body in (doc.get("skills") or {}).items():
        if isinstance(body, dict) and "updatedAt" in body:
            out[("skills", name)] = body["updatedAt"]
    if isinstance(doc, dict) and "updatedAt" in doc:
        out[("root",)] = doc["updatedAt"]
    return out


def main():
    check_only = "--check" in sys.argv

    try:
        work = read_file(TARGET)
    except Exception as e:
        print("[normalize-skill-registry] read failed: {}".format(e))
        return 2

    head = read_head(TARGET)
    if head is None:
        print("[normalize-skill-registry] {} not tracked yet — nothing to normalize".format(TARGET))
        return 0

    if canonical(work) == canonical(head):
        # 内容完全一致，只有时间戳在抖 ⇒ 回填 HEAD 的时间戳
        head_map = collect_updated_at_map(head)
        doc = json.loads(work)
        changed = 0
        for name, body in (doc.get("skills") or {}).items():
            old = head_map.get(("skills", name))
            if old is not None and isinstance(body, dict) and body.get("updatedAt") != old:
                body["updatedAt"] = old
                changed += 1
        if ("root",) in head_map and "updatedAt" in doc and doc["updatedAt"] != head_map[("root",)]:
            doc["updatedAt"] = head_map[("root",)]
            changed += 1

        if changed:
            if check_only:
                print("[normalize-skill-registry] NOISE: {} 条 updatedAt 抖动（--check，未修改）".format(changed))
                return 0
            with io.open(TARGET, "w", encoding="utf-8") as f:
                # 保持原文件的 2 空格缩进与结尾换行习惯
                json.dump(doc, f, ensure_ascii=False, indent=2)
                f.write("\n")
            print("[normalize-skill-registry] 归一化 {} 条 updatedAt → 与 HEAD 一致（内容无变化）".format(changed))
        else:
            print("[normalize-skill-registry] 已干净：无 updatedAt 抖动")
        return 0

    print("[normalize-skill-registry] REAL-CHANGE：{} 有实质变更（hash/条目结构），保留新时间戳".format(TARGET))
    return 1


if __name__ == "__main__":
    sys.exit(main())
