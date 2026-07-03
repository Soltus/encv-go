//go:build objectbox

// Package objectbox 提供 tasksystem.Store 的 ObjectBox 实现。
//
// ObjectBox 是面向对象的 NoSQL 数据库，底层用 C 库（CGO），
// 官方支持 Android / iOS / Linux / Windows / macOS。
// 比 SQLite 更快（尤其是写入），原生支持对象关系和查询。
//
// 编译要求：
//   - 安装 ObjectBox C 库：bash <(curl -s https://raw.githubusercontent.com/objectbox/objectbox-go/main/install.sh)
//   - 生成代码：go generate ./pkg/tasksystem/store/objectbox
//   - 构建标签：go build -tags objectbox
//
// Android 集成：
//   - C 库需要交叉编译到 Android ABI（armeabi-v7a / arm64-v8a / x86 / x86_64）
//   - 通过 gomobile bind 时把 libobjectbox.so 打包进 AAR
//   - 与 libsql 的 jniLibs 模式类似
package objectbox

/*
#cgo LDFLAGS: -lobjectbox
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
	objectbox "github.com/objectbox/objectbox-go/objectbox"
)

// ─── Entity 定义 ───────────────────────────────────────────────
//
// ObjectBox 通过 struct + go:generate 生成 binding 代码。
// 这里只定义 struct，binding 代码由 `go generate` 生成。
//
// 注意：Id 字段必须是 uint64 类型，必须命名为 Id（首字母大写）。
//go:generate go run github.com/objectbox/objectbox-go/cmd/objectbox-gogen

// TaskEntity 任务实体（对应 tasksystem.TaskData）。
//
// 为了避免循环依赖和代码生成复杂性，这里定义独立的 Entity 结构体，
// Store 方法在 TaskData 和 TaskEntity 之间做转换。
type TaskEntity struct {
	Id uint64 `objectbox:"id"`

	// 任务标识
	TaskID          string `objectbox:"unique index"`
	Type            string `objectbox:"index"`
	Status          string `objectbox:"index"`
	ServiceName     string `objectbox:"index"`
	MethodName      string
	TenantID        string `objectbox:"index"`
	TriggeredBy     string `objectbox:"index"`
	RunID           string `objectbox:"index"`

	// 路径
	SourcePath  string
	TargetPath  string
	OutputPath  string
	PluginName  string

	// 进度
	Progress int
	Phase    string

	// 错误
	Error        string
	ErrorDetail  string
	Warning      string
	WarningDetail string

	// 加密参数
	ContainerVersion int
	CipherMode       int
	CompressionMode  string
	ExtraFields      string
	Steps            string

	// 挂载
	MountID             string
	MountSubPath        string
	TargetMountID       string
	TargetMountSubPath  string
	Password            string
	SecondaryPassword   string

	// 微服务扩展
	DurationMs int64
	InputJSON  string
	OutputJSON string
	Attempts   int
	Priority   int
	TagsJSON   string

	// 时间
	CreatedAt   int64 // UnixMilli
	CompletedAt int64 // UnixMilli, 0 = 未完成

	// 回滚
	RollbackOf    string
	OriginalPath  string
}

// SnapshotEntity 回滚快照实体。
type SnapshotEntity struct {
	Id           uint64 `objectbox:"id"`
	TaskID       string `objectbox:"index"`
	SnapshotType string
	Data         []byte
	CreatedAt    int64
}

// TrashEntity 回收站实体。
type TrashEntity struct {
	Id             uint64 `objectbox:"id"`
	TrashID        string `objectbox:"unique index"`
	OriginalPath   string
	TrashPath      string
	IsDirectory    bool
	Size           int64
	DeletedAt      int64
	TaskID         string `objectbox:"index"`
	RestoreTaskID  string
	Metadata       string
}

// PerformanceMetricsEntity 性能指标实体。
type PerformanceMetricsEntity struct {
	Id                uint64 `objectbox:"id"`
	TaskID            string `objectbox:"unique index"`
	TaskType          string `objectbox:"index"`
	PluginName        string `objectbox:"index"`
	ContainerVersion  int
	CipherMode        int
	CompressionMode   string
	SourceSize        int64
	OutputSize        int64
	SizeRatio         float64
	AvgThroughput     float64
	PeakThroughput    float64
	P50Throughput     float64
	P99Throughput     float64
	TotalDurationMs   int64
	PhaseTimingsJSON  string
	Grade             string
	GradeScore        float64
	GradeReason       string
	CPUScore          float64
	CPULabel          string
	CreatedAt         int64
}

// CalibrationEntity 硬件校准实体（单例，Id=1）。
type CalibrationEntity struct {
	Id             uint64  `objectbox:"id"`
	CPUScore       float64
	AESThroughput  float64
	CPULabel       string
	CalibratedAt   int64
	GoVersion      string
	OS             string
	Arch           string
	NumCPU         int
}

// Store ObjectBox 存储实现。
type Store struct {
	ob        *objectbox.ObjectBox
	tasks     *objectbox.Box
	snapshots *objectbox.Box
	trash     *objectbox.Box
	metrics   *objectbox.Box
	calib     *objectbox.Box
}

// New 创建 ObjectBox 存储。
// dbDir 是数据库目录路径（不是文件，ObjectBox 是多文件数据库）。
func New(dbDir string) (*Store, error) {
	ob, err := objectbox.NewBuilder().Directory(dbDir).Model(ObjectBoxModel()).Build()
	if err != nil {
		return nil, fmt.Errorf("objectbox: open store: %w", err)
	}
	return &Store{
		ob:        ob,
		tasks:     objectbox.BoxFor(ob, TaskEntity{}),
		snapshots: objectbox.BoxFor(ob, SnapshotEntity{}),
		trash:     objectbox.BoxFor(ob, TrashEntity{}),
		metrics:   objectbox.BoxFor(ob, PerformanceMetricsEntity{}),
		calib:     objectbox.BoxFor(ob, CalibrationEntity{}),
	}, nil
}

// EngineName 返回引擎名称。
func (s *Store) EngineName() string { return "objectbox" }

// ConcurrencyHint 返回推荐并发数。
// ObjectBox 支持 MVCC，多 writer 并发写性能好，给 8。
func (s *Store) ConcurrencyHint() int { return 8 }

// Close 关闭 ObjectBox store。
func (s *Store) Close() error {
	s.ob.Close()
	return nil
}

// ─── Task CRUD ─────────────────────────────────────────────────

func (s *Store) CreateTask(task tasksystem.TaskData) error {
	entity := taskDataToEntity(task)
	_, err := s.tasks.Put(entity)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) GetTask(id string) (tasksystem.TaskData, error) {
	qb := s.tasks.Query(objectbox.PropertyEqString(TaskEntity_.TaskID, id))
	defer qb.Close()
	results, err := qb.Find()
	if err != nil {
		return tasksystem.TaskData{}, err
	}
	if len(results) == 0 {
		return tasksystem.TaskData{}, tasksystem.ErrNotFound
	}
	return taskEntityToData(results[0].(*TaskEntity)), nil
}

func (s *Store) ListTasks(filter tasksystem.TaskFilter) ([]tasksystem.TaskData, error) {
	// ObjectBox 查询构建器
	conditions := []objectbox.Condition{}

	if len(filter.Types) > 0 {
		// ObjectBox 没有 IN，用 OR 链
		var orConds []objectbox.Condition
		for _, t := range filter.Types {
			orConds = append(orConds, objectbox.PropertyEqString(TaskEntity_.Type, string(t)))
		}
		conditions = append(conditions, objectbox.Or(orConds...))
	}
	if len(filter.Statuses) > 0 {
		var orConds []objectbox.Condition
		for _, st := range filter.Statuses {
			orConds = append(orConds, objectbox.PropertyEqString(TaskEntity_.Status, string(st)))
		}
		conditions = append(conditions, objectbox.Or(orConds...))
	}
	if len(filter.TriggeredBy) > 0 {
		var orConds []objectbox.Condition
		for _, tb := range filter.TriggeredBy {
			orConds = append(orConds, objectbox.PropertyEqString(TaskEntity_.TriggeredBy, tb))
		}
		conditions = append(conditions, objectbox.Or(orConds...))
	}
	if filter.RunID != "" {
		conditions = append(conditions, objectbox.PropertyEqString(TaskEntity_.RunID, filter.RunID))
	}
	if filter.RollbackOf != "" {
		conditions = append(conditions, objectbox.PropertyEqString(TaskEntity_.RollbackOf, filter.RollbackOf))
	}
	if filter.ServiceName != "" {
		conditions = append(conditions, objectbox.PropertyEqString(TaskEntity_.ServiceName, filter.ServiceName))
	}
	if filter.TenantID != "" {
		conditions = append(conditions, objectbox.PropertyEqString(TaskEntity_.TenantID, filter.TenantID))
	}

	var qb *objectbox.QueryBuilder
	if len(conditions) > 0 {
		qb = s.tasks.Query(objectbox.And(conditions...))
	} else {
		qb = s.tasks.Query(nil)
	}
	defer qb.Close()

	// 按 CreatedAt 倒序
	qb.OrderDescInt(TaskEntity_.CreatedAt)

	if filter.Limit > 0 {
		qb.Limit(filter.Limit, filter.Offset)
	}

	results, err := qb.Find()
	if err != nil {
		return nil, err
	}

	tasks := make([]tasksystem.TaskData, len(results))
	for i, r := range results {
		tasks[i] = taskEntityToData(r.(*TaskEntity))
	}
	return tasks, nil
}

func (s *Store) UpdateTask(task tasksystem.TaskData) error {
	// 先按 taskID 查内部 Id
	qb := s.tasks.Query(objectbox.PropertyEqString(TaskEntity_.TaskID, task.ID))
	defer qb.Close()
	results, err := qb.Find()
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return tasksystem.ErrNotFound
	}
	entity := taskDataToEntity(task)
	entity.Id = results[0].(*TaskEntity).Id
	_, err = s.tasks.Put(entity)
	return err
}

func (s *Store) DeleteTask(id string) error {
	qb := s.tasks.Query(objectbox.PropertyEqString(TaskEntity_.TaskID, id))
	defer qb.Close()
	ids, err := qb.FindIds()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.tasks.RemoveId(id); err != nil {
			return err
		}
	}
	return nil
}

// ─── Snapshot ─────────────────────────────────────────────────

func (s *Store) SaveSnapshot(snap tasksystem.Snapshot) error {
	entity := &SnapshotEntity{
		TaskID:       snap.TaskID,
		SnapshotType: string(snap.SnapshotType),
		Data:         []byte(snap.Data),
		CreatedAt:    snap.CreatedAt.UnixMilli(),
	}
	_, err := s.snapshots.Put(entity)
	return err
}

func (s *Store) GetSnapshot(taskID string) (tasksystem.Snapshot, error) {
	qb := s.snapshots.Query(objectbox.PropertyEqString(SnapshotEntity_.TaskID, taskID))
	defer qb.Close()
	// pre_state 优先
	qb.OrderAscString(SnapshotEntity_.SnapshotType)
	qb.OrderDescInt(SnapshotEntity_.CreatedAt)
	qb.Limit(1, 0)

	results, err := qb.Find()
	if err != nil {
		return tasksystem.Snapshot{}, err
	}
	if len(results) == 0 {
		return tasksystem.Snapshot{}, tasksystem.ErrNotFound
	}
	e := results[0].(*SnapshotEntity)
	return tasksystem.Snapshot{
		ID:           fmt.Sprintf("%d", e.Id),
		TaskID:       e.TaskID,
		SnapshotType: tasksystem.SnapshotType(e.SnapshotType),
		Data:         string(e.Data),
		CreatedAt:    time.UnixMilli(e.CreatedAt),
	}, nil
}

// ─── Trash ────────────────────────────────────────────────────

func (s *Store) CreateTrash(item tasksystem.TrashItem) error {
	entity := &TrashEntity{
		TrashID:       item.ID,
		OriginalPath:  item.OriginalPath,
		TrashPath:     item.TrashPath,
		IsDirectory:   item.IsDirectory,
		Size:          item.Size,
		DeletedAt:     item.DeletedAt.UnixMilli(),
		TaskID:        item.TaskID,
		RestoreTaskID: item.RestoreTaskID,
		Metadata:      string(item.Metadata),
	}
	_, err := s.trash.Put(entity)
	return err
}

func (s *Store) GetTrash(id string) (tasksystem.TrashItem, error) {
	qb := s.trash.Query(objectbox.PropertyEqString(TrashEntity_.TrashID, id))
	defer qb.Close()
	results, err := qb.Find()
	if err != nil {
		return tasksystem.TrashItem{}, err
	}
	if len(results) == 0 {
		return tasksystem.TrashItem{}, tasksystem.ErrNotFound
	}
	return trashEntityToItem(results[0].(*TrashEntity)), nil
}

func (s *Store) GetTrashByTaskID(taskID string) (tasksystem.TrashItem, error) {
	qb := s.trash.Query(objectbox.PropertyEqString(TrashEntity_.TaskID, taskID))
	defer qb.Close()
	results, err := qb.Find()
	if err != nil {
		return tasksystem.TrashItem{}, err
	}
	if len(results) == 0 {
		return tasksystem.TrashItem{}, tasksystem.ErrNotFound
	}
	return trashEntityToItem(results[0].(*TrashEntity)), nil
}

func (s *Store) ListTrash() ([]tasksystem.TrashItem, error) {
	qb := s.trash.Query(nil)
	defer qb.Close()
	qb.OrderDescInt(TrashEntity_.DeletedAt)
	results, err := qb.Find()
	if err != nil {
		return nil, err
	}
	items := make([]tasksystem.TrashItem, len(results))
	for i, r := range results {
		items[i] = trashEntityToItem(r.(*TrashEntity))
	}
	return items, nil
}

func (s *Store) UpdateTrash(item tasksystem.TrashItem) error {
	qb := s.trash.Query(objectbox.PropertyEqString(TrashEntity_.TrashID, item.ID))
	defer qb.Close()
	ids, err := qb.FindIds()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return tasksystem.ErrNotFound
	}
	entity := trashItemToEntity(item)
	entity.Id = ids[0]
	_, err = s.trash.Put(entity)
	return err
}

func (s *Store) DeleteTrash(id string) error {
	qb := s.trash.Query(objectbox.PropertyEqString(TrashEntity_.TrashID, id))
	defer qb.Close()
	ids, err := qb.FindIds()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.trash.RemoveId(id); err != nil {
			return err
		}
	}
	return nil
}

// ─── Count / Runs ─────────────────────────────────────────────

func (s *Store) CountByRunId(runId string) (map[string]int, error) {
	if runId == "" {
		return map[string]int{}, nil
	}
	qb := s.tasks.Query(objectbox.PropertyEqString(TaskEntity_.RunID, runId))
	defer qb.Close()
	results, err := qb.Find()
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, r := range results {
		status := r.(*TaskEntity).Status
		counts[status]++
	}
	return counts, nil
}

func (s *Store) ListRuns() ([]tasksystem.RunInfo, error) {
	// 先拿所有 runId 非空的任务，按 runId 分组，取最早 created_at
	qb := s.tasks.Query(nil)
	defer qb.Close()
	qb.OrderAscInt(TaskEntity_.CreatedAt)
	results, err := qb.Find()
	if err != nil {
		return nil, err
	}

	type runInfo struct {
		startedAt   time.Time
		triggeredBy string
	}
	runsMap := make(map[string]*runInfo)
	for _, r := range results {
		e := r.(*TaskEntity)
		if e.RunID == "" {
			continue
		}
		if _, ok := runsMap[e.RunID]; !ok {
			runsMap[e.RunID] = &runInfo{
				startedAt:   time.UnixMilli(e.CreatedAt),
				triggeredBy: e.TriggeredBy,
			}
		}
	}

	runs := make([]tasksystem.RunInfo, 0, len(runsMap))
	for id, info := range runsMap {
		runs = append(runs, tasksystem.RunInfo{
			RunID:       id,
			StartedAt:   info.startedAt,
			TriggeredBy: info.triggeredBy,
		})
	}
	// 按 startedAt 倒序
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j].StartedAt.After(runs[i].StartedAt) {
				runs[i], runs[j] = runs[j], runs[i]
			}
		}
	}
	return runs, nil
}

// ─── Performance Metrics ──────────────────────────────────────

func (s *Store) SaveMetrics(m performance.PerformanceMetrics) error {
	entity := metricsToEntity(m)
	_, err := s.metrics.Put(entity)
	return err
}

func (s *Store) GetMetrics(taskID string) (performance.PerformanceMetrics, error) {
	qb := s.metrics.Query(objectbox.PropertyEqString(PerformanceMetricsEntity_.TaskID, taskID))
	defer qb.Close()
	results, err := qb.Find()
	if err != nil {
		return performance.PerformanceMetrics{}, err
	}
	if len(results) == 0 {
		return performance.PerformanceMetrics{}, tasksystem.ErrNotFound
	}
	return entityToMetrics(results[0].(*PerformanceMetricsEntity)), nil
}

func (s *Store) ListMetricsByPlugin(pluginName string, taskType string, limit int) ([]performance.PerformanceMetrics, error) {
	if limit <= 0 {
		limit = 10
	}
	qb := s.metrics.Query(
		objectbox.And(
			objectbox.PropertyEqString(PerformanceMetricsEntity_.PluginName, pluginName),
			objectbox.PropertyEqString(PerformanceMetricsEntity_.TaskType, taskType),
		),
	)
	defer qb.Close()
	qb.OrderDescInt(PerformanceMetricsEntity_.CreatedAt)
	qb.Limit(limit, 0)
	results, err := qb.Find()
	if err != nil {
		return nil, err
	}
	ms := make([]performance.PerformanceMetrics, len(results))
	for i, r := range results {
		ms[i] = entityToMetrics(r.(*PerformanceMetricsEntity))
	}
	return ms, nil
}

func (s *Store) GetLatestMetrics(pluginName string, taskType string) (*performance.PerformanceMetrics, error) {
	list, err := s.ListMetricsByPlugin(pluginName, taskType, 1)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

// ─── Calibration ─────────────────────────────────────────────

func (s *Store) SaveCalibration(cal performance.CalibrationResult) error {
	entity := &CalibrationEntity{
		Id:            1,
		CPUScore:      cal.CPUScore,
		AESThroughput: cal.AESThroughput,
		CPULabel:      cal.CPULabel,
		CalibratedAt:  cal.CalibratedAt.UnixMilli(),
		GoVersion:     cal.GoVersion,
		OS:            cal.OS,
		Arch:          cal.Arch,
		NumCPU:        cal.NumCPU,
	}
	_, err := s.calib.Put(entity)
	return err
}

func (s *Store) GetCalibration() (*performance.CalibrationResult, error) {
	e := &CalibrationEntity{}
	err := s.calib.Get(1, e)
	if err != nil {
		if err == objectbox.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &performance.CalibrationResult{
		CPUScore:      e.CPUScore,
		AESThroughput: e.AESThroughput,
		CPULabel:      e.CPULabel,
		CalibratedAt:  time.UnixMilli(e.CalibratedAt),
		GoVersion:     e.GoVersion,
		OS:            e.OS,
		Arch:          e.Arch,
		NumCPU:        e.NumCPU,
	}, nil
}

// ─── Export / Import ─────────────────────────────────────────

func (s *Store) ExportAll() (*tasksystem.DatabaseDump, error) {
	// 任务
	tasks, err := s.ListTasks(tasksystem.TaskFilter{Limit: 100000})
	if err != nil {
		return nil, fmt.Errorf("export tasks: %w", err)
	}

	// 回收站
	trash, err := s.ListTrash()
	if err != nil {
		return nil, fmt.Errorf("export trash: %w", err)
	}

	// 快照（简单起见全部导出）
	var snapshots []tasksystem.Snapshot
	qb := s.snapshots.Query(nil)
	defer qb.Close()
	snapResults, err := qb.Find()
	if err != nil {
		return nil, fmt.Errorf("export snapshots: %w", err)
	}
	for _, r := range snapResults {
		e := r.(*SnapshotEntity)
		snapshots = append(snapshots, tasksystem.Snapshot{
			ID:           fmt.Sprintf("%d", e.Id),
			TaskID:       e.TaskID,
			SnapshotType: tasksystem.SnapshotType(e.SnapshotType),
			Data:         string(e.Data),
			CreatedAt:    time.UnixMilli(e.CreatedAt),
		})
	}

	// 性能指标
	var allMetrics []performance.PerformanceMetrics
	mq := s.metrics.Query(nil)
	defer mq.Close()
	mResults, err := mq.Find()
	if err != nil {
		return nil, fmt.Errorf("export metrics: %w", err)
	}
	for _, r := range mResults {
		allMetrics = append(allMetrics, entityToMetrics(r.(*PerformanceMetricsEntity)))
	}

	// 校准
	cal, err := s.GetCalibration()
	if err != nil {
		return nil, fmt.Errorf("export calibration: %w", err)
	}

	return &tasksystem.DatabaseDump{
		Version:   1,
		Engine:    "objectbox",
		ExportedAt: time.Now().UTC(),
		Tasks:     tasks,
		Trash:     trash,
		Snapshots: snapshots,
		Metrics:   allMetrics,
		Calibration: cal,
	}, nil
}

func (s *Store) ImportAll(dump *tasksystem.DatabaseDump) error {
	// 清空现有数据
	s.ob.RemoveAllObjects()

	// 导入任务
	for _, t := range dump.Tasks {
		if err := s.CreateTask(t); err != nil {
			return fmt.Errorf("import task %s: %w", t.ID, err)
		}
	}

	// 导入回收站
	for _, t := range dump.Trash {
		if err := s.CreateTrash(t); err != nil {
			return fmt.Errorf("import trash %s: %w", t.ID, err)
		}
	}

	// 导入快照
	for _, s := range dump.Snapshots {
		if err := s.SaveSnapshot(s); err != nil {
			return fmt.Errorf("import snapshot: %w", err)
		}
	}

	// 导入性能指标
	for _, m := range dump.Metrics {
		if err := s.SaveMetrics(m); err != nil {
			return fmt.Errorf("import metrics: %w", err)
		}
	}

	// 导入校准
	if dump.Calibration != nil {
		if err := s.SaveCalibration(*dump.Calibration); err != nil {
			return fmt.Errorf("import calibration: %w", err)
		}
	}

	return nil
}

// ─── 转换函数 ─────────────────────────────────────────────────

func taskDataToEntity(t tasksystem.TaskData) *TaskEntity {
	e := &TaskEntity{
		TaskID:          t.ID,
		Type:            string(t.Type),
		Status:          string(t.Status),
		ServiceName:     t.ServiceName,
		MethodName:      t.MethodName,
		TenantID:        t.TenantID,
		TriggeredBy:     t.TriggeredBy,
		RunID:           t.RunID,
		SourcePath:      t.SourcePath,
		TargetPath:      t.TargetPath,
		OutputPath:      t.OutputPath,
		PluginName:      t.PluginName,
		Progress:        t.Progress,
		Phase:           t.Phase,
		Error:           t.Error,
		ErrorDetail:     t.ErrorDetail,
		Warning:         t.Warning,
		WarningDetail:   t.WarningDetail,
		ContainerVersion: t.ContainerVersion,
		CipherMode:      t.CipherMode,
		CompressionMode: t.CompressionMode,
		ExtraFields:     t.ExtraFields,
		Steps:           t.Steps,
		MountID:          t.MountID,
		MountSubPath:     t.MountSubPath,
		TargetMountID:    t.TargetMountID,
		TargetMountSubPath: t.TargetMountSubPath,
		Password:         t.Password,
		SecondaryPassword: t.SecondaryPassword,
		DurationMs:      t.DurationMs,
		InputJSON:       t.InputJSON,
		OutputJSON:      t.OutputJSON,
		Attempts:        t.Attempts,
		Priority:        t.Priority,
		TagsJSON:        t.TagsJSON,
		CreatedAt:       t.CreatedAt.UnixMilli(),
		RollbackOf:      t.RollbackOf,
		OriginalPath:    t.OriginalPath,
	}
	if t.CompletedAt != nil {
		e.CompletedAt = t.CompletedAt.UnixMilli()
	}
	return e
}

func taskEntityToData(e *TaskEntity) tasksystem.TaskData {
	t := tasksystem.TaskData{
		ID:               e.TaskID,
		Type:             tasksystem.TaskType(e.Type),
		Status:           tasksystem.TaskStatus(e.Status),
		ServiceName:      e.ServiceName,
		MethodName:       e.MethodName,
		TenantID:         e.TenantID,
		TriggeredBy:      e.TriggeredBy,
		RunID:            e.RunID,
		SourcePath:       e.SourcePath,
		TargetPath:       e.TargetPath,
		OutputPath:       e.OutputPath,
		PluginName:       e.PluginName,
		Progress:         e.Progress,
		Phase:            e.Phase,
		Error:            e.Error,
		ErrorDetail:      e.ErrorDetail,
		Warning:          e.Warning,
		WarningDetail:    e.WarningDetail,
		ContainerVersion: e.ContainerVersion,
		CipherMode:       e.CipherMode,
		CompressionMode:  e.CompressionMode,
		ExtraFields:      e.ExtraFields,
		Steps:            e.Steps,
		MountID:          e.MountID,
		MountSubPath:     e.MountSubPath,
		TargetMountID:    e.TargetMountID,
		TargetMountSubPath: e.TargetMountSubPath,
		Password:         e.Password,
		SecondaryPassword: e.SecondaryPassword,
		DurationMs:       e.DurationMs,
		InputJSON:        e.InputJSON,
		OutputJSON:       e.OutputJSON,
		Attempts:         e.Attempts,
		Priority:         e.Priority,
		TagsJSON:         e.TagsJSON,
		CreatedAt:        time.UnixMilli(e.CreatedAt),
		RollbackOf:       e.RollbackOf,
		OriginalPath:     e.OriginalPath,
	}
	if e.CompletedAt > 0 {
		tm := time.UnixMilli(e.CompletedAt)
		t.CompletedAt = &tm
	}
	return t
}

func trashItemToEntity(t tasksystem.TrashItem) *TrashEntity {
	return &TrashEntity{
		TrashID:       t.ID,
		OriginalPath:  t.OriginalPath,
		TrashPath:     t.TrashPath,
		IsDirectory:   t.IsDirectory,
		Size:          t.Size,
		DeletedAt:     t.DeletedAt.UnixMilli(),
		TaskID:        t.TaskID,
		RestoreTaskID: t.RestoreTaskID,
		Metadata:      string(t.Metadata),
	}
}

func trashEntityToItem(e *TrashEntity) tasksystem.TrashItem {
	return tasksystem.TrashItem{
		ID:              e.TrashID,
		OriginalPath:    e.OriginalPath,
		TrashPath:       e.TrashPath,
		IsDirectory:     e.IsDirectory,
		Size:            e.Size,
		DeletedAt:       time.UnixMilli(e.DeletedAt),
		TaskID:          e.TaskID,
		RestoreTaskID:   e.RestoreTaskID,
		Metadata:        tasksystem.TrashMetadata(e.Metadata),
	}
}

func metricsToEntity(m performance.PerformanceMetrics) *PerformanceMetricsEntity {
	return &PerformanceMetricsEntity{
		TaskID:           m.TaskID,
		TaskType:         string(m.TaskType),
		PluginName:       m.PluginName,
		ContainerVersion: m.ContainerVer,
		CipherMode:       m.CipherMode,
		CompressionMode:  string(m.CompressionMode),
		SourceSize:       m.SourceSize,
		OutputSize:       m.OutputSize,
		SizeRatio:        m.SizeRatio,
		AvgThroughput:    m.AvgThroughput,
		PeakThroughput:   m.PeakThroughput,
		P50Throughput:    m.P50Throughput,
		P99Throughput:    m.P99Throughput,
		TotalDurationMs:  m.TotalDurationMs,
		PhaseTimingsJSON: m.PhaseTimingsJSON,
		Grade:            string(m.Grade),
		GradeScore:       m.GradeScore,
		GradeReason:      m.GradeReason,
		CPUScore:         m.CPUScore,
		CPULabel:         m.CPULabel,
		CreatedAt:        m.CreatedAt.UnixMilli(),
	}
}

func entityToMetrics(e *PerformanceMetricsEntity) performance.PerformanceMetrics {
	return performance.PerformanceMetrics{
		TaskID:           e.TaskID,
		TaskType:         performance.TaskType(e.TaskType),
		PluginName:       e.PluginName,
		ContainerVer:     e.ContainerVersion,
		CipherMode:       e.CipherMode,
		CompressionMode:  performance.CompressionMode(e.CompressionMode),
		SourceSize:       e.SourceSize,
		OutputSize:       e.OutputSize,
		SizeRatio:        e.SizeRatio,
		AvgThroughput:    e.AvgThroughput,
		PeakThroughput:   e.PeakThroughput,
		P50Throughput:    e.P50Throughput,
		P99Throughput:    e.P99Throughput,
		TotalDurationMs:  e.TotalDurationMs,
		PhaseTimingsJSON: e.PhaseTimingsJSON,
		Grade:            performance.Grade(e.Grade),
		GradeScore:       e.GradeScore,
		GradeReason:      e.GradeReason,
		CPUScore:         e.CPUScore,
		CPULabel:         e.CPULabel,
		CreatedAt:        time.UnixMilli(e.CreatedAt),
	}
}
