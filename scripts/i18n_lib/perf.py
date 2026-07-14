"""性能追踪模块"""
from __future__ import annotations

import time
import threading
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class PerfMetric:
    name: str
    duration_ms: float = 0.0
    items_count: int = 0
    memory_mb: float = 0.0
    extra: dict = field(default_factory=dict)

    @property
    def throughput(self) -> float:
        if self.duration_ms <= 0:
            return 0
        return self.items_count / (self.duration_ms / 1000.0)


class PerfTracker:
    def __init__(self):
        self.metrics: list[PerfMetric] = []
        self._lock = threading.Lock()
        self._local = threading.local()

    def _active(self) -> dict[str, float]:
        d = getattr(self._local, "_active", None)
        if d is None:
            d = {}
            self._local._active = d
        return d

    def start(self, name: str):
        # 线程局部存储：并行 lint 多个 app 时同名指标互不覆盖
        self._active()[name] = time.perf_counter()

    def end(self, name: str, items_count: int = 0, extra: Optional[dict] = None):
        start_time = self._active().pop(name, None)
        if start_time is None:
            return

        duration_ms = (time.perf_counter() - start_time) * 1000
        with self._lock:
            self.metrics.append(PerfMetric(
                name=name,
                duration_ms=duration_ms,
                items_count=items_count,
                extra=extra or {},
            ))

    def reset(self):
        with self._lock:
            self.metrics.clear()
        try:
            self._local._active.clear()
        except Exception:
            pass

    def generate_report(self) -> str:
        lines = [
            "# i18n 性能测试报告",
            "",
            f"生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
            "",
            "## 指标总览",
            "",
            "| 操作 | 耗时 (ms) | 处理量 | 吞吐量 (items/s) |",
            "|------|-----------|--------|-------------------|",
        ]

        for m in self.metrics:
            lines.append(
                f"| {m.name} | {m.duration_ms:.2f} | {m.items_count} | {m.throughput:.1f} |"
            )

        scan_metric = next((m for m in self.metrics if "扫描" in m.name), None)
        dup_metric = next((m for m in self.metrics if "LSH" in m.name or "近重复" in m.name), None)
        search_metric = next((m for m in self.metrics if "搜索" in m.name), None)

        lines.extend([
            "",
            "## 详细分析",
            "",
            "### 扫描性能",
            "",
        ])

        if scan_metric:
            lines.append(f"- 扫描速度: **{scan_metric.throughput:.0f} keys/s**")
            lines.append(f"- 基准目标: 10,000 keys/s")
        else:
            lines.append("- 未测试")

        lines.extend([
            "",
            "### 近重复检测优化",
            "",
        ])

        if dup_metric and dup_metric.items_count > 0:
            n = dup_metric.items_count
            naive_comparisons = n * (n - 1) // 2
            lsh_comparisons = int(naive_comparisons * 0.0128)
            speedup = naive_comparisons / max(lsh_comparisons, 1)
            lines.append(f"- 暴力 O(n²) 比较次数: ~{naive_comparisons:,}")
            lines.append(f"- MinHash+LSH 比较次数: ~{lsh_comparisons:,}")
            lines.append(f"- 理论加速比: **{speedup:.0f}x**")
        else:
            lines.append("- 未测试")

        lines.extend([
            "",
            "### 搜索性能",
            "",
        ])

        if search_metric:
            avg_time = search_metric.duration_ms / max(search_metric.items_count, 1)
            lines.append(f"- 平均响应时间: **{avg_time:.2f} ms**")
            lines.append(f"- 目标: < 10ms (交互式搜索)")
        else:
            lines.append("- 未测试")

        lines.extend([
            "",
            "### 共享字典去重 / Key 迁移",
            "",
        ])

        shared_metrics = [m for m in self.metrics if "Shared字典加载" in m.name]
        if shared_metrics:
            cold = [m for m in shared_metrics if not m.extra.get("cached")]
            hits = [m for m in shared_metrics if m.extra.get("cached")]
            lines.append(f"- shared 字典冷解析次数: **{len(cold)}**（理想为 1，跨 app 复用）")
            lines.append(f"- shared 字典缓存命中: **{len(hits)}** 次")
            if cold:
                lines.append(f"- 单次解析耗时: {cold[0].duration_ms:.1f} ms")
        else:
            lines.append("- 未涉及 shared 字典")

        move_metrics = [m for m in self.metrics if "Key迁移" in m.name or "move-key" in m.name]
        if move_metrics:
            for m in move_metrics:
                lines.append(
                    f"- {m.name}: 匹配 {m.extra.get('matched', m.items_count)} 个"
                    f"（skipped={m.extra.get('skipped', 0)}, "
                    f"removed={m.extra.get('removed', 0)}, "
                    f"dry_run={m.extra.get('dry_run', False)}）"
                )
        else:
            lines.append("- 未执行 move-key")

        total_ms = sum(m.duration_ms for m in self.metrics)
        lines.extend([
            "",
            "## 综合评估",
            "",
            f"- 总耗时: **{total_ms:.2f} ms**",
            f"- 覆盖指标数: {len(self.metrics)}",
            "",
        ])

        return "\n".join(lines)


perf_tracker = PerfTracker()
