package performance

// CompareWithHistory 与同 plugin + taskType 的上一次运行对比。
// previous 为 nil 时返回零值对比。
func CompareWithHistory(current PerformanceMetrics, previous *PerformanceMetrics) HistoryComparison {
	cmp := HistoryComparison{
		Current:  current,
		Previous: previous,
	}
	if previous == nil {
		return cmp
	}

	// 吞吐量变化：正数=更快（更好），负数=更慢（更差）
	if previous.AvgThroughput > 0 {
		cmp.ThroughputPctChange = (current.AvgThroughput - previous.AvgThroughput) / previous.AvgThroughput * 100
	}

	// 耗时变化：正数=更慢（更差），负数=更快（更好）
	if previous.TotalDurationMs > 0 {
		cmp.DurationPctChange = float64(current.TotalDurationMs-previous.TotalDurationMs) / float64(previous.TotalDurationMs) * 100
	}

	// 评级是否变化
	cmp.GradeChanged = current.Grade != previous.Grade

	return cmp
}
