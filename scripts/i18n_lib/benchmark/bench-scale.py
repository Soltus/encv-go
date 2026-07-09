#!/usr/bin/env python3
"""
大规模 i18n 性能基准测试
测试不同规模下的各阶段耗时，找出瓶颈
"""
import time
import subprocess
import json
import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from i18n_lib.loader import parse_i18n_files, _node_available, SCHEMA_VERSION, PARSER_VERSION
from i18n_lib.scanner import _file_fingerprint

def bench_node_parse(data_dir, label):
    """测试 Node.js 解析阶段"""
    files = sorted(Path(data_dir).glob("*.ts"))
    print(f"\n{'='*60}")
    print(f"📊 {label}: {len(files)} 个文件")
    print(f"{'='*60}")

    # ---- 阶段1: 文件系统遍历（找文件） ----
    t0 = time.time()
    file_list = [str(f) for f in files]
    t_files = time.time() - t0
    total_keys_estimate = 0
    for f in file_list:
        stat = Path(f).stat()
        total_keys_estimate += stat.st_size
    print(f"  阶段1-文件遍历: {t_files*1000:.1f}ms ({len(file_list)} files, {total_keys_estimate/1024/1024:.1f} MB)")

    # ---- 阶段2: Node.js 解析（冷启动 + import + JSON序列化 + stdout传输） ----
    t0 = time.time()
    result = parse_i18n_files(file_list)
    t_parse = time.time() - t0

    total_keys = sum(len(v) for v in result.values())
    lang_count = len(result)
    print(f"  阶段2-Node.js解析: {t_parse*1000:.1f}ms")
    print(f"    - 语言数: {lang_count}")
    print(f"    - 总key数: {total_keys:,}")
    print(f"    - 单位速度: {total_keys/t_parse/1000:.1f} k keys/秒")

    # ---- 阶段3: JSON 序列化（模拟缓存写入） ----
    t0 = time.time()
    cache_data = {
        "schema_version": SCHEMA_VERSION,
        "parser_version": PARSER_VERSION,
        "files_hash": "fakehash",
        "data": result,
        "parsed_at": time.time(),
    }
    json_str = json.dumps(cache_data)
    t_json_dump = time.time() - t0
    print(f"  阶段3-JSON序列化: {t_json_dump*1000:.1f}ms ({len(json_str)/1024/1024:.1f} MB)")

    # ---- 阶段4: JSON 反序列化（模拟缓存读取） ----
    t0 = time.time()
    loaded = json.loads(json_str)
    t_json_load = time.time() - t0
    print(f"  阶段4-JSON反序列化: {t_json_load*1000:.1f}ms")

    # ---- 阶段5: 文件指纹计算（缓存有效性检查） ----
    t0 = time.time()
    for f in file_list:
        _file_fingerprint(f)
    t_fingerprint = time.time() - t0
    print(f"  阶段5-文件指纹计算: {t_fingerprint*1000:.1f}ms")

    # ---- 汇总 ----
    total_no_cache = t_parse + t_files
    total_with_cache = t_files + t_json_load + t_fingerprint
    print(f"\n  📈 无缓存总耗时: {total_no_cache*1000:.1f}ms")
    print(f"  📈 有缓存总耗时: {total_with_cache*1000:.1f}ms")
    print(f"  📈 缓存加速比: {total_no_cache/total_with_cache:.1f}x")

    return {
        "file_count": len(file_list),
        "total_keys": total_keys,
        "lang_count": lang_count,
        "t_files": t_files,
        "t_parse": t_parse,
        "t_json_dump": t_json_dump,
        "t_json_load": t_json_load,
        "t_fingerprint": t_fingerprint,
        "total_no_cache": total_no_cache,
        "total_with_cache": total_with_cache,
    }


def main():
    if not _node_available():
        print("❌ Node.js 不可用")
        return

    # 小规模预热
    print("🔥 预热...")
    bench_node_parse("/tmp/i18n-bench-200k", "预热")

    # 20万key × 20语言
    result_200k = bench_node_parse("/tmp/i18n-bench-200k", "20万 key × 20 语言")

    # 分析瓶颈
    print(f"\n{'='*60}")
    print(f"🔍 瓶颈分析")
    print(f"{'='*60}")

    stages = [
        ("Node.js解析", result_200k["t_parse"]),
        ("JSON序列化", result_200k["t_json_dump"]),
        ("JSON反序列化", result_200k["t_json_load"]),
        ("文件指纹", result_200k["t_fingerprint"]),
        ("文件遍历", result_200k["t_files"]),
    ]

    stages.sort(key=lambda x: x[1], reverse=True)
    total = sum(s[1] for s in stages)

    for name, t in stages:
        pct = t / total * 100
        bar = "█" * int(pct / 2)
        print(f"  {name:15s}: {t*1000:8.1f}ms ({pct:5.1f}%) {bar}")

    print(f"\n  无缓存总耗时: {result_200k['total_no_cache']*1000:.1f}ms")
    print(f"  5秒目标: {'✅ 达成' if result_200k['total_no_cache'] < 5 else '❌ 未达成'}")

    # 预测100万key
    print(f"\n{'='*60}")
    print(f"📈 扩展性预测（线性外推）")
    print(f"{'='*60}")
    for keys in [500000, 1000000, 2000000]:
        scale = keys / 200000
        pred = result_200k["total_no_cache"] * scale
        status = "✅" if pred < 5 else "⚠️" if pred < 10 else "❌"
        print(f"  {status} {keys/1000:.0f}k keys: 预计 {pred*1000:.0f}ms ({scale:.0f}x)")

    # 给出优化建议
    print(f"\n{'='*60}")
    print(f"💡 优化建议")
    print(f"{'='*60}")
    print(optimization_suggestions(result_200k))


def optimization_suggestions(r):
    """根据瓶颈比例给出优化建议"""
    suggestions = []
    total = r["total_no_cache"]

    # 方案A: 缓存为主（最有效，成本最低）
    speedup_cache = r["total_no_cache"] / r["total_with_cache"]
    suggestions.append(f"""
方案A: 缓存优先（推荐 ⭐⭐⭐⭐⭐）
  原理: 充分利用缓存，只有文件变更时才重新解析
  预期效果: 有缓存时 {r['total_with_cache']*1000:.0f}ms（加速 {speedup_cache:.1f}x）
  成本: 低（已实现）
  适用场景: 日常开发 / CI 增量构建
""")

    # 方案B: 并行解析 Node.js worker
    if r["t_parse"] / total > 0.5:
        speedup_node_worker = min(4, 8)  # 假设文件并行，受CPU核心限制
        new_parse = r["t_parse"] / speedup_node_worker + 0.1  # 加启动开销
        new_total = new_parse + r["t_files"] + r["t_json_dump"]
        suggestions.append(f"""
方案B: Node.js Worker 线程池并行解析
  原理: 多个 Node worker 并行解析文件，用 worker_threads
  预期效果: 解析阶段加速约 {speedup_node_worker}x，总耗时约 {new_total*1000:.0f}ms
  成本: 中（需改造 extract 脚本为 worker 模式）
  适用场景: 超大字典首次加载
""")

    # 方案C: 预编译的二进制缓存
    if r["t_json_load"] / total > 0.1:
        suggestions.append(f"""
方案C: msgpack / 二进制格式替代 JSON
  原理: 用 msgpack 等二进制格式替代 JSON，序列化/反序列化更快
  预期效果: JSON阶段加速 2-3x，但整体收益有限（JSON占比不大）
  成本: 低
  适用场景: 极端追求缓存读取速度
""")

    # 方案D: 增量解析
    suggestions.append(f"""
方案D: 增量解析（只重新解析变更文件）
  原理: 按文件粒度缓存，只重新解析 mtime 变化的文件
  预期效果: 单文件变更时 < 50ms（几乎无感）
  成本: 中（需改造 loader 为按文件缓存 + merge）
  适用场景: 日常开发高频修改
""")

    # 方案E: 内置 SQLite / 数据库
    suggestions.append(f"""
方案E: SQLite 存储字典
  原理: 字典存 SQLite，按需查询，不全部加载到内存
  预期效果: 启动快（<100ms），查询有索引
  成本: 高（架构改动大）
  适用场景: 百万级 key 且不需要全量遍历
""")

    return "\n".join(suggestions)


if __name__ == "__main__":
    main()
