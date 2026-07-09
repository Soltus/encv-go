#!/usr/bin/env python3
"""
完整的 i18n 大规模性能分析
分阶段测试：Node.js 各阶段 / stdout传输 / JSON解析 / 扫描器
"""
import time
import subprocess
import json
import sys
import os
import re
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from i18n_lib.loader import _node_available
from i18n_lib.scanner import is_dynamic_key, COMBINED_PATTERN, _strip_comments

EXTRACT_SCRIPT = Path(__file__).parent.parent / "extract-i18n.mjs"
PROFILE_SCRIPT = Path(__file__).parent / "profile-node-parser.mjs"

def measure_python_parse(data_dir, label):
    """用 Python 子进程调用，测完整链路（含 stdout 传输 + JSON.parse）"""
    files = sorted(Path(data_dir).glob("*.ts"))
    file_list = [str(f) for f in files]
    total_size = sum(Path(f).stat().st_size for f in file_list)

    print(f"\n{'='*60}")
    print(f"📊 {label}: {len(file_list)} 个文件, {total_size/1024/1024:.1f} MB")
    print(f"{'='*60}")

    # ---- 阶段1: Node.js 纯解析（不含传输，从 stderr 读取耗时） ----
    t0 = time.time()
    result = subprocess.run(
        ["node", str(PROFILE_SCRIPT)] + file_list,
        capture_output=True,
        text=True,
        timeout=300,
    )
    t_total = time.time() - t0

    # 从 stderr 提取 Node 侧各阶段耗时
    stderr_lines = result.stderr.strip().split("\n")
    node_import = 0
    node_merge = 0
    node_json = 0
    total_keys = 0
    for line in stderr_lines:
        if "import" in line and "ms" in line:
            m = re.search(r"import:\s+([\d.]+)ms", line)
            if m: node_import = float(m.group(1))
        if "遍历合并" in line and "ms" in line:
            m = re.search(r"遍历合并:\s+([\d.]+)ms", line)
            if m: node_merge = float(m.group(1))
        if "JSON序列化\(紧凑\)" in line and "ms" in line:
            m = re.search(r"紧凑\):\s+([\d.]+)ms", line)
            if m: node_json = float(m.group(1))
        if "总 key 数" in line:
            m = re.search(r"总 key 数:\s+([\d,]+)", line)
            if m: total_keys = int(m.group(1).replace(",", ""))

    node_total = node_import + node_merge + node_json

    # ---- 阶段2: stdout 传输 + Python JSON.parse ----
    t_py_json = 0
    if result.stdout:
        t1 = time.time()
        data = json.loads(result.stdout)
        t_py_json = (time.time() - t1) * 1000

    transfer_time = t_total * 1000 - node_total - t_py_json

    print(f"\n  Node.js 侧:")
    print(f"    import:   {node_import:8.1f}ms ({node_import/(node_total+0.001)*100:.1f}%)")
    print(f"    合并:     {node_merge:8.1f}ms ({node_merge/(node_total+0.001)*100:.1f}%)")
    print(f"    JSON:     {node_json:8.1f}ms ({node_json/(node_total+0.001)*100:.1f}%)")
    print(f"    小计:     {node_total:8.1f}ms")
    print(f"\n  Python 侧:")
    print(f"    stdout传输: {transfer_time:8.1f}ms ({transfer_time/(t_total*1000)*100:.1f}%)")
    print(f"    JSON解析:   {t_py_json:8.1f}ms")
    print(f"\n  总计: {t_total*1000:.1f}ms")
    print(f"  总key数: {total_keys:,}")
    print(f"  吞吐量: {total_keys/t_total:.0f} keys/秒" if t_total > 0 else "")

    return {
        "total_keys": total_keys,
        "file_count": len(file_list),
        "node_import": node_import,
        "node_merge": node_merge,
        "node_json": node_json,
        "node_total": node_total,
        "transfer_ms": transfer_time,
        "py_json_ms": t_py_json,
        "total_ms": t_total * 1000,
    }


def bench_scanner(data_dir, label):
    """测试扫描器在大规模代码下的性能"""
    files = sorted(Path(data_dir).glob("*.ts"))
    all_content = ""
    for f in files[:5]:  # 只用前5个文件模拟
        all_content += Path(f).read_text(encoding="utf-8")

    # 构造类似源码的调用模式
    sample_code = """
import { t } from '@/i18n'

export function TestComponent() {
  const title = t('settings.title')
  const desc = t('settings.description')
  const name = t('user.name')
  const btn1 = t('common.save')
  const btn2 = t('common.cancel')
  const msg1 = t('errors.networkError')
  const msg2 = t('errors.timeout')
  const msg3 = t('warnings.lowSpace')
  const info1 = t('info.welcome')
  const info2 = t('info.goodbye')
"""
    # 复制 N 次模拟大规模代码
    n_copies = 2000
    big_code = sample_code * n_copies
    code_size = len(big_code)

    print(f"\n{'='*60}")
    print(f"📊 扫描器性能 - {label}")
    print(f"{'='*60}")
    print(f"  代码量: {code_size/1024/1024:.1f} MB")
    print(f"  预期调用数: {10 * n_copies:,}")

    t0 = time.time()
    direct_keys = set()
    for line in big_code.split("\n"):
        if not line.strip():
            continue
        if "t(" not in line and "$t" not in line and "tField" not in line and "tSection" not in line:
            continue
        clean_line = _strip_comments(line)
        if not clean_line:
            continue
        for match in COMBINED_PATTERN.finditer(clean_line):
            key = match.group(1)
            if is_dynamic_key(key):
                continue
            direct_keys.add(key)
    t_scan = (time.time() - t0) * 1000

    keys = direct_keys

    print(f"  扫描耗时: {t_scan:.1f}ms")
    print(f"  提取到 {len(keys)} 个唯一key")
    print(f"  扫描速度: {code_size/1024/1024/(t_scan/1000):.1f} MB/s")

    return {
        "code_size_mb": code_size / 1024 / 1024,
        "scan_ms": t_scan,
        "unique_keys": len(keys),
        "speed_mb_s": code_size / 1024 / 1024 / (t_scan / 1000),
    }


def main():
    if not _node_available():
        print("❌ Node.js 不可用")
        return

    # 扫描器基准
    scan_result = bench_scanner("/tmp/i18n-bench-20k", "模拟 2000 组件调用")

    # 多规模解析测试
    scales = [
        (2000, 20, "/tmp/i18n-bench-2k"),
        (10000, 20, "/tmp/i18n-bench-10k"),
        (50000, 20, "/tmp/i18n-bench-50k"),
        (200000, 20, "/tmp/i18n-bench-200k"),
    ]

    results = {}
    for keys, langs, outdir in scales:
        if not Path(outdir).exists():
            print(f"\n⏳ 生成 {keys} keys × {langs} langs 测试数据...")
            subprocess.run(
                ["node", str(Path(__file__).parent / "generate-large-data.mjs"),
                 str(keys), str(langs), outdir],
                capture_output=True
            )

        try:
            r = measure_python_parse(outdir, f"{keys/1000:.0f}k keys × {langs} langs")
            results[keys] = r
        except subprocess.TimeoutExpired:
            print(f"  ⚠️  超时跳过 (> 300s)")
            results[keys] = None

    # 汇总对比
    print(f"\n\n{'='*60}")
    print(f"📈 规模扩展对比")
    print(f"{'='*60}")
    print(f"{'规模':<20} {'总耗时':>10} {'Node侧':>10} {'传输':>10} {'JSON解析':>10}")
    print(f"{'-'*60}")
    for keys, r in sorted(results.items()):
        if r:
            label = f"{keys/1000:.0f}k × 20语"
            print(f"{label:<20} {r['total_ms']:>8.0f}ms {r['node_total']:>8.0f}ms {r['transfer_ms']:>8.0f}ms {r['py_json_ms']:>8.0f}ms")

    # 5秒目标判断
    print(f"\n{'='*60}")
    print(f"🎯 5秒目标可行性分析（20万key × 20语言）")
    print(f"{'='*60}")
    if 200000 in results and results[200000]:
        r = results[200000]
        current = r['total_ms']
        print(f"  当前无缓存: {current:.0f}ms")
        print(f"  目标: 5000ms")
        print(f"  差距: 需要提升 {(1 - 5000/current)*100:.0f}%")
        print()
        print(f"  瓶颈占比:")
        total = current
        for name, val in [
            ("Node import", r['node_import']),
            ("Node 合并遍历", r['node_merge']),
            ("Node JSON序列化", r['node_json']),
            ("stdout传输", r['transfer_ms']),
            ("Python JSON解析", r['py_json_ms']),
        ]:
            pct = val / total * 100
            bar = "█" * int(pct / 2)
            print(f"    {name:18s}: {val:7.0f}ms ({pct:5.1f}%) {bar}")


if __name__ == "__main__":
    main()
