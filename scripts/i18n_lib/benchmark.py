"""性能基准测试模块 - 全指标性能测试"""
from __future__ import annotations

import random
import sys

from .config import PERF_REPORT_PATH
from .perf import perf_tracker
from .scanner import extract_used_keys
from .loader import load_all_dicts
from .db import init_db
from .search import (
    minhash_signature,
    find_near_duplicates_lsh,
    build_ngram_index,
    search_ngram,
    tokenize,
)


def random_text(locale: str, length: int) -> str:
    if locale.startswith("zh"):
        chars = "的一是不了人我在有他这为之大来以个中上们到说国地也子时出就而要下你天年生自会那后能对着事其景"
        return "".join(random.choice(chars) for _ in range(length))
    else:
        words = [
            "file", "task", "setting", "user", "data", "system", "config",
            "network", "storage", "memory", "process", "thread", "buffer",
            "cache", "index", "query", "result", "error", "warning", "success",
        ]
        return " ".join(random.choice(words) for _ in range(length // 5))


def cmd_benchmark(app_name: str | None = None):
    print("🚀 i18n 工具性能基准测试")
    print("=" * 60)

    print("\n📦 阶段 1: 冷启动（无缓存）")
    print("-" * 60)

    from .config import CACHE_DIR, DB_PATH
    for f in CACHE_DIR.glob("*.pkl"):
        f.unlink()
    if DB_PATH.exists():
        DB_PATH.unlink()

    extract_used_keys(app_name, use_cache=False)
    load_all_dicts(app_name, use_cache=False)
    init_db(app_name)

    perf_tracker.start("搜索（数据库）")
    from .db import search_db
    results = search_db("文件", "zh-CN", 50)
    perf_tracker.end("搜索（数据库）", len(results))

    print(f"\n   找到 {len(results)} 条匹配")

    print("\n📦 阶段 2: 热启动（有缓存）")
    print("-" * 60)

    extract_used_keys(app_name, use_cache=True)
    load_all_dicts(app_name, use_cache=True)

    print("\n📦 阶段 3: 大规模压力测试（2万值级别）")
    print("-" * 60)

    stress_count = 20000
    perf_tracker.start("大规模压力测试")

    stress_items_zh = [
        (f"stress.key.{i}", random_text("zh", random.randint(5, 50)))
        for i in range(stress_count)
    ]
    stress_items_en = [
        (f"stress.key.{i}", random_text("en", random.randint(5, 50)))
        for i in range(stress_count)
    ]

    perf_tracker.end("大规模压力测试", stress_count, {"阶段": "数据生成"})

    perf_tracker.start("MinHash 签名生成（2万条）")
    sigs_zh = [minhash_signature(tokenize(v, "zh-CN")) for _, v in stress_items_zh]
    perf_tracker.end("MinHash 签名生成（2万条）", stress_count)

    perf_tracker.start("LSH 近重复检测（2万条）")
    dups = find_near_duplicates_lsh(stress_items_zh, "zh-CN", 0.85)
    perf_tracker.end("LSH 近重复检测（2万条）", stress_count, {"找到近重复": len(dups)})

    perf_tracker.start("n-gram 索引构建（2万条）")
    texts_zh = [v for _, v in stress_items_zh]
    ng_index = build_ngram_index(texts_zh, "zh-CN", 2)
    perf_tracker.end("n-gram 索引构建（2万条）", len(ng_index))

    perf_tracker.start("n-gram 搜索（100次）")
    for _ in range(100):
        q = random_text("zh", 3)
        search_ngram(q, texts_zh, ng_index, "zh-CN", 20)
    perf_tracker.end("n-gram 搜索（100次）", 100)

    perf_tracker.start("20万值级别扫描估算")
    estimated_time = 0
    for m in perf_tracker.metrics:
        if "LSH 近重复检测" in m.name and "2万" in m.name:
            estimated_time = m.duration_ms * 10
            break
    perf_tracker.end("20万值级别扫描估算", 200000, {"估算耗时_ms": estimated_time})

    print("\n📊 性能报告")
    print("=" * 60)

    report = perf_tracker.generate_report()
    print(report)

    PERF_REPORT_PATH.write_text(report, encoding="utf-8")
    print(f"\n📄 完整报告已保存: {PERF_REPORT_PATH}")

    total_dup = next(
        (m for m in perf_tracker.metrics if "LSH 近重复检测（2万条）" in m.name),
        None,
    )
    total_search = next(
        (m for m in perf_tracker.metrics if "n-gram 搜索" in m.name),
        None,
    )
    est_200k = next(
        (m for m in perf_tracker.metrics if "20万" in m.name),
        None,
    )

    print("\n🎯 优化效果估算")
    print("-" * 60)

    if total_dup and total_dup.items_count > 0:
        n = total_dup.items_count
        naive_time = (n * n / 2) * 0.001
        optimized_time = total_dup.duration_ms
        if optimized_time > 0:
            speedup = naive_time / optimized_time
            print(f"近重复检测加速比: **{speedup:.0f}x**")
            print(f"  - 暴力 O(n²): ~{naive_time:,.0f} ms (估算)")
            print(f"  - MinHash+LSH: {optimized_time:,.2f} ms")

    if total_search and total_search.items_count > 0:
        avg_search = total_search.duration_ms / total_search.items_count
        print(f"\n搜索平均响应: **{avg_search:.3f} ms/次**")

    if est_200k and est_200k.extra.get("估算耗时_ms", 0) > 0:
        t = est_200k.extra["估算耗时_ms"]
        naive_200k = (200000 * 200000 / 2) * 0.001
        speedup_200k = naive_200k / t
        print(f"\n20万值级别估算:")
        print(f"  - 暴力 O(n²): ~{naive_200k:,.0f} ms (~{naive_200k/1000/60/60:.1f} 小时)")
        print(f"  - MinHash+LSH: ~{t:,.0f} ms (~{t/1000:.1f} 秒)")
        print(f"  - 综合加速比: **{speedup_200k:.0f}x**")

    print("\n✅ 性能测试完成！")
