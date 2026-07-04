package performance

import "time"

// Grade 性能评级。
type Grade string

const (
	GradeExcellent Grade = "excellent"
	GradeGood      Grade = "good"
	GradeWarn      Grade = "warn"
)

// PhaseTiming 单个阶段的耗时与吞吐量统计。
type PhaseTiming struct {
	Phase          string  `json:"phase"`
	DurationMs     int64   `json:"durationMs"`
	BytesProcessed int64   `json:"bytesProcessed,omitempty"`
	ThroughputMBps float64 `json:"throughputMBps,omitempty"`
}

// PerformanceMetrics 一次任务的完整性能指标。
type PerformanceMetrics struct {
	TaskID          string        `json:"taskId"`
	TaskType        string        `json:"taskType"`
	PluginName      string        `json:"pluginName,omitempty"`
	ContainerVer    int           `json:"containerVersion,omitempty"`
	CipherMode      int           `json:"cipherMode,omitempty"`
	CompressionMode string        `json:"compressionMode,omitempty"`
	SourceSize      int64         `json:"sourceSize"`
	OutputSize      int64         `json:"outputSize,omitempty"`
	SizeRatio       float64       `json:"sizeRatio,omitempty"`
	AvgThroughput   float64       `json:"avgThroughput"`
	PeakThroughput  float64       `json:"peakThroughput"`
	P50Throughput   float64       `json:"p50Throughput"`
	P99Throughput   float64       `json:"p99Throughput"`
	PhaseTimings    []PhaseTiming `json:"phaseTimings"`
	TotalDurationMs int64         `json:"totalDurationMs"`
	Grade           Grade         `json:"grade"`
	GradeScore      float64       `json:"gradeScore"`
	GradeReason     string        `json:"gradeReason,omitempty"`
	CPUScore        float64       `json:"cpuScore"`
	CPULabel        string        `json:"cpuLabel"`
	CreatedAt       time.Time     `json:"createdAt"`
}

// CalibrationResult 硬件校准结果。
type CalibrationResult struct {
	CPUScore      float64   `json:"cpuScore"`
	AESThroughput float64   `json:"aesThroughput"`
	CPULabel      string    `json:"cpuLabel"`
	CalibratedAt  time.Time `json:"calibratedAt"`
	GoVersion     string    `json:"goVersion"`
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	NumCPU        int       `json:"numCpu"`
}

// GradeThresholds 评级阈值表。
type GradeThresholds struct {
	ExcelThroughput float64
	GoodThroughput  float64
	ExcelDurationMs int64
	GoodDurationMs  int64
}

// HistoryComparison 与历史记录的对比结果。
type HistoryComparison struct {
	Current             PerformanceMetrics  `json:"current"`
	Previous            *PerformanceMetrics `json:"previous,omitempty"`
	ThroughputPctChange float64             `json:"throughputPctChange,omitempty"`
	DurationPctChange   float64             `json:"durationPctChange,omitempty"`
	GradeChanged        bool                `json:"gradeChanged"`
}
