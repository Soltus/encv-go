#!/usr/bin/env python3
"""
scripts/merge-test-reports.py

合并 .test-runs/ 下所有 reports-*.json → 单个 report-all.json。
同时附带 crash 堆栈清单和 probe 资源摘要。

输出（stdout）：

{
  "total": N,
  "passed": N,
  "failed": N,
  "skipped": N,
  "failures": [{"name": ..., "error_msg": ...}, ...],
  "crashes": [".test-runs/crashes/.../*.stack", ...],
  "peak_rss_mb": N,
  "peak_goroutine": N
}

用法：
  python3 merge-test-reports.py .test-runs/ > .test-runs/report-all.json
"""
import json
import os
import sys
import glob


def main():
    if len(sys.argv) < 2:
        print("usage: merge-test-reports.py <log_root>", file=sys.stderr)
        sys.exit(1)
    log_root = sys.argv[1].rstrip("/")
    if not os.path.isdir(log_root):
        print(json.dumps({"total": 0, "passed": 0, "failed": 0, "skipped": 0, "failures": []}))
        return

    # 1) 合并 reports-*.json (递归)
    all_reports = []
    seen = set()
    for pattern in [
        os.path.join(log_root, "**/reports-*.json"),
        os.path.join(log_root, "reports-*.json"),  # 顶层
    ]:
        for fp in glob.glob(pattern, recursive=True):
            if fp in seen:
                continue
            seen.add(fp)
            try:
                with open(fp) as f:
                    data = json.load(f)
                if isinstance(data, list):
                    all_reports.extend(data)
            except (json.JSONDecodeError, OSError) as e:
                print(f"[merge] skip {fp}: {e}", file=sys.stderr)

    # 2) 统计
    total = len(all_reports)
    passed = sum(1 for r in all_reports if r.get("status") == "pass")
    failed = sum(1 for r in all_reports
                 if r.get("status") in ("fail", "panic", "timeout", "aborted"))
    skipped = sum(1 for r in all_reports if r.get("status") == "skip")

    failures = [
        {
            "name": r.get("name", "?"),
            "package": r.get("package", ""),
            "status": r.get("status", ""),
            "error_msg": r.get("error_msg", ""),
            "stack_file": r.get("stack_file", ""),
        }
        for r in all_reports
        if r.get("status") in ("fail", "panic", "timeout", "aborted")
    ]

    # 3) crash 堆栈清单
    crash_dir = os.path.join(log_root, "crashes")
    crashes = []
    if os.path.isdir(crash_dir):
        for root, _, files in os.walk(crash_dir):
            for fn in files:
                if fn.endswith(".stack"):
                    crashes.append(os.path.join(root, fn))
    crashes.sort()

    # 4) probe 资源峰值
    peak_rss_mb = 0
    peak_goroutine = 0
    peak_fds = 0
    probe_seen = set()
    for pattern in [
        os.path.join(log_root, "**/probe-*.json"),
        os.path.join(log_root, "probe-*.json"),  # 顶层
    ]:
        for fp in glob.glob(pattern, recursive=True):
            if fp in probe_seen:
                continue
            probe_seen.add(fp)
            try:
                with open(fp) as f:
                    p = json.load(f)
                peak_rss_mb = max(peak_rss_mb, p.get("end_rss_mb", 0))
                peak_goroutine = max(peak_goroutine, p.get("end_goroutine", 0))
                peak_fds = max(peak_fds, p.get("end_fds", 0))
            except (json.JSONDecodeError, OSError, ValueError):
                pass

    out = {
        "total": total,
        "passed": passed,
        "failed": failed,
        "skipped": skipped,
        "failures": failures,
        "crashes": crashes,
        "peak_rss_mb": peak_rss_mb,
        "peak_goroutine": peak_goroutine,
        "peak_fds": peak_fds,
    }
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
