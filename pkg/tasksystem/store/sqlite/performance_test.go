package sqlite

import (
	"testing"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

func TestSaveAndGetMetrics(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// 先创建 task（FK 约束）
	task := tasksystem.TaskData{
		ID:         "task-perf-1",
		Type:       tasksystem.TaskTypeEncrypt,
		Status:     tasksystem.StatusCompleted,
		SourcePath: "/test/source.mp4",
		CreatedAt:  time.Now(),
	}
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// 保存性能指标
	m := performance.PerformanceMetrics{
		TaskID:         "task-perf-1",
		TaskType:       "encrypt",
		PluginName:     "mp4",
		SourceSize:     1024 * 1024 * 100,
		OutputSize:     1024 * 1024 * 105,
		SizeRatio:      1.05,
		AvgThroughput:  245.3,
		PeakThroughput: 312.7,
		P50Throughput:  240.0,
		P99Throughput:  298.0,
		PhaseTimings: []performance.PhaseTiming{
			{Phase: "encrypting", DurationMs: 312, BytesProcessed: 104857600, ThroughputMBps: 320.0},
			{Phase: "packing", DurationMs: 89},
		},
		TotalDurationMs: 524,
		Grade:           performance.GradeExcellent,
		GradeScore:      92.5,
		GradeReason:     "throughput and duration well above thresholds",
		CPUScore:        1.8,
		CPULabel:        "fast",
		CreatedAt:       time.Now(),
	}
	if err := store.SaveMetrics(m); err != nil {
		t.Fatalf("SaveMetrics: %v", err)
	}

	// 查询
	got, err := store.GetMetrics("task-perf-1")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if got.TaskID != m.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, m.TaskID)
	}
	if got.AvgThroughput != m.AvgThroughput {
		t.Errorf("AvgThroughput = %v, want %v", got.AvgThroughput, m.AvgThroughput)
	}
	if got.Grade != m.Grade {
		t.Errorf("Grade = %q, want %q", got.Grade, m.Grade)
	}
	if len(got.PhaseTimings) != 2 {
		t.Errorf("PhaseTimings len = %d, want 2", len(got.PhaseTimings))
	}
	if got.PhaseTimings[0].Phase != "encrypting" {
		t.Errorf("PhaseTimings[0].Phase = %q, want 'encrypting'", got.PhaseTimings[0].Phase)
	}
}

func TestListMetricsByPlugin(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// 创建 3 个 task + 3 个 metrics
	for i, id := range []string{"task-1", "task-2", "task-3"} {
		task := tasksystem.TaskData{
			ID:         id,
			Type:       tasksystem.TaskTypeEncrypt,
			Status:     tasksystem.StatusCompleted,
			SourcePath: "/test/source.mp4",
			CreatedAt:  time.Now(),
		}
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", id, err)
		}
		m := performance.PerformanceMetrics{
			TaskID:        id,
			TaskType:      "encrypt",
			PluginName:    "mp4",
			AvgThroughput: 200 + float64(i*10),
			Grade:         performance.GradeGood,
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Minute),
		}
		if err := store.SaveMetrics(m); err != nil {
			t.Fatalf("SaveMetrics %s: %v", id, err)
		}
	}

	// 查询 limit=2
	list, err := store.ListMetricsByPlugin("mp4", "encrypt", 2)
	if err != nil {
		t.Fatalf("ListMetricsByPlugin: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	// 倒序，应该是 task-3（最新）在前
	if list[0].TaskID != "task-3" {
		t.Errorf("list[0].TaskID = %q, want 'task-3'", list[0].TaskID)
	}
}

func TestGetLatestMetrics(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// 无数据时返回 nil
	got, err := store.GetLatestMetrics("mp4", "encrypt")
	if err != nil {
		t.Fatalf("GetLatestMetrics empty: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}

	// 创建 2 个 metrics
	for i, id := range []string{"task-a", "task-b"} {
		task := tasksystem.TaskData{
			ID:         id,
			Type:       tasksystem.TaskTypeEncrypt,
			Status:     tasksystem.StatusCompleted,
			SourcePath: "/test/source.mp4",
			CreatedAt:  time.Now(),
		}
		if err := store.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", id, err)
		}
		m := performance.PerformanceMetrics{
			TaskID:        id,
			TaskType:      "encrypt",
			PluginName:    "mp4",
			AvgThroughput: 200 + float64(i*50),
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Hour),
		}
		if err := store.SaveMetrics(m); err != nil {
			t.Fatalf("SaveMetrics %s: %v", id, err)
		}
	}

	// 查询最新
	latest, err := store.GetLatestMetrics("mp4", "encrypt")
	if err != nil {
		t.Fatalf("GetLatestMetrics: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil")
	}
	if latest.TaskID != "task-b" {
		t.Errorf("latest.TaskID = %q, want 'task-b'", latest.TaskID)
	}
}

func TestSaveAndGetCalibration(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// 无数据时返回 nil
	got, err := store.GetCalibration()
	if err != nil {
		t.Fatalf("GetCalibration empty: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}

	// 保存校准
	cal := performance.CalibrationResult{
		CPUScore:      1.8,
		AESThroughput: 5400.0,
		CPULabel:      "fast",
		CalibratedAt:  time.Now(),
		GoVersion:     "go1.24",
		OS:            "linux",
		Arch:          "amd64",
		NumCPU:        8,
	}
	if err := store.SaveCalibration(cal); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}

	// 查询
	gotCal, err := store.GetCalibration()
	if err != nil {
		t.Fatalf("GetCalibration: %v", err)
	}
	if gotCal == nil {
		t.Fatal("expected non-nil")
	}
	if gotCal.CPUScore != cal.CPUScore {
		t.Errorf("CPUScore = %v, want %v", gotCal.CPUScore, cal.CPUScore)
	}
	if gotCal.CPULabel != cal.CPULabel {
		t.Errorf("CPULabel = %q, want %q", gotCal.CPULabel, cal.CPULabel)
	}

	// 覆盖保存（INSERT OR REPLACE）
	cal2 := cal
	cal2.CPUScore = 0.6
	cal2.CPULabel = "medium"
	if err := store.SaveCalibration(cal2); err != nil {
		t.Fatalf("SaveCalibration overwrite: %v", err)
	}
	gotCal2, err := store.GetCalibration()
	if err != nil {
		t.Fatalf("GetCalibration after overwrite: %v", err)
	}
	if gotCal2.CPUScore != 0.6 {
		t.Errorf("CPUScore = %v, want 0.6", gotCal2.CPUScore)
	}
}
