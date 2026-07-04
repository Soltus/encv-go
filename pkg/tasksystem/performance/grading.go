package performance

// 默认阈值表（按 taskType）
var defaultThresholds = map[string]GradeThresholds{
	"encrypt": {ExcelThroughput: 200, GoodThroughput: 80, ExcelDurationMs: 1000, GoodDurationMs: 5000},
	"decrypt": {ExcelThroughput: 300, GoodThroughput: 100, ExcelDurationMs: 800, GoodDurationMs: 4000},
	"move":    {ExcelThroughput: 500, GoodThroughput: 200, ExcelDurationMs: 500, GoodDurationMs: 2000},
	"copy":    {ExcelThroughput: 300, GoodThroughput: 100, ExcelDurationMs: 1000, GoodDurationMs: 5000},
}

// CPUScore 校准系数
// fast (>=1.5): ×1.5
// medium (>=0.5): ×1.0
// slow (<0.5): ×0.6
func cpuScoreMultiplier(cpuScore float64) float64 {
	switch {
	case cpuScore >= 1.5:
		return 1.5
	case cpuScore >= 0.5:
		return 1.0
	default:
		return 0.6
	}
}

// GetThresholds 根据 taskType + pluginName + CPUScore 返回动态阈值。
// pluginName 当前未参与区分（预留），仅 taskType + CPUScore 决定。
func GetThresholds(taskType, pluginName string, cpuScore float64) GradeThresholds {
	base, ok := defaultThresholds[taskType]
	if !ok {
		// 未知 taskType，用 encrypt 默认值
		base = defaultThresholds["encrypt"]
	}
	mult := cpuScoreMultiplier(cpuScore)
	return GradeThresholds{
		ExcelThroughput: base.ExcelThroughput * mult,
		GoodThroughput:  base.GoodThroughput * mult,
		ExcelDurationMs: int64(float64(base.ExcelDurationMs) / mult), // CPU 越快，延迟要求越严
		GoodDurationMs:  int64(float64(base.GoodDurationMs) / mult),
	}
}

// CalculateGrade 计算评级。
// 评分公式（与 bench-report 一致）：
//   吞吐量分 = (实际 MB/s / ExcelMB) * 100，上限 100
//   延迟分 = (ExcelMs / 实际 ms) * 100，上限 100
//   综合分 = (吞吐量分 + 延迟分) / 2
//   excellent: 综合分 >= 80
//   good:      综合分 >= 50
//   warn:      综合分 < 50
// 对于不按吞吐评级的 taskType（rename/delete），仅按延迟评级。
func CalculateGrade(metrics PerformanceMetrics, thresholds GradeThresholds) (Grade, float64, string) {
	var throughputScore, durationScore float64
	var hasThroughput, hasDuration bool

	if thresholds.ExcelThroughput > 0 && metrics.AvgThroughput > 0 {
		throughputScore = metrics.AvgThroughput / thresholds.ExcelThroughput * 100
		if throughputScore > 100 {
			throughputScore = 100
		}
		hasThroughput = true
	}

	if thresholds.ExcelDurationMs > 0 && metrics.TotalDurationMs > 0 {
		durationScore = float64(thresholds.ExcelDurationMs) / float64(metrics.TotalDurationMs) * 100
		if durationScore > 100 {
			durationScore = 100
		}
		hasDuration = true
	}

	var combinedScore float64
	switch {
	case hasThroughput && hasDuration:
		combinedScore = (throughputScore + durationScore) / 2
	case hasThroughput:
		combinedScore = throughputScore
	case hasDuration:
		combinedScore = durationScore
	default:
		// 无阈值，默认 good
		return GradeGood, 50, "no thresholds defined, default to good"
	}

	var grade Grade
	var reason string
	switch {
	case combinedScore >= 80:
		grade = GradeExcellent
		reason = "throughput and duration well above thresholds"
	case combinedScore >= 50:
		grade = GradeGood
		reason = "meets thresholds"
	default:
		grade = GradeWarn
		reason = "below thresholds"
	}

	return grade, combinedScore, reason
}
