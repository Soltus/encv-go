//go:build objectbox

package objectbox

/*
#cgo android,arm64 LDFLAGS: -L${SRCDIR}/libs/android_arm64
#cgo android,arm LDFLAGS: -L${SRCDIR}/libs/android_armv7
#cgo android,386 LDFLAGS: -L${SRCDIR}/libs/android_x86
#cgo android,amd64 LDFLAGS: -L${SRCDIR}/libs/android_x86_64
#cgo LDFLAGS: -lobjectbox
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
	objectbox "github.com/objectbox/objectbox-go/objectbox"
)

var ErrObjectBoxUnavailable = errors.New("objectbox: engine not available in this build (use -tags objectbox)")

type Store struct {
	ob        *objectbox.ObjectBox
	tasks     *TaskEntityBox
	snapshots *SnapshotEntityBox
	trash     *TrashEntityBox
	metrics   *PerformanceMetricsEntityBox
	calib     *CalibrationEntityBox
}

func New(dbDir string) (*Store, error) {
	ob, err := objectbox.NewBuilder().
		Directory(dbDir).
		MaxReaders(64).
		MaxSizeInKb(10 * 1024 * 1024).
		Model(ObjectBoxModel()).
		Build()
	if err != nil {
		return nil, fmt.Errorf("objectbox: open store: %w", err)
	}
	return &Store{
		ob:        ob,
		tasks:     BoxForTaskEntity(ob),
		snapshots: BoxForSnapshotEntity(ob),
		trash:     BoxForTrashEntity(ob),
		metrics:   BoxForPerformanceMetricsEntity(ob),
		calib:     BoxForCalibrationEntity(ob),
	}, nil
}

func (s *Store) EngineName() string { return "objectbox" }

func (s *Store) ConcurrencyHint() int { return 8 }

func (s *Store) Close() error {
	s.ob.Close()
	return nil
}

func (s *Store) CreateTask(task tasksystem.TaskData) error {
	entity := taskDataToEntity(task)
	_, err := s.tasks.Put(entity)
	return err
}

func (s *Store) GetTask(id string) (tasksystem.TaskData, error) {
	q := s.tasks.Query(TaskEntity_.TaskID.Equals(id, false))
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return tasksystem.TaskData{}, err
	}
	if len(results) == 0 {
		return tasksystem.TaskData{}, tasksystem.ErrNotFound
	}
	return taskEntityToData(results[0]), nil
}

func (s *Store) ListTasks(filter tasksystem.TaskFilter) ([]tasksystem.TaskData, error) {
	conditions := []objectbox.Condition{}

	if len(filter.Types) > 0 {
		var conds []objectbox.Condition
		for _, t := range filter.Types {
			conds = append(conds, TaskEntity_.Type.Equals(string(t), false))
		}
		conditions = append(conditions, objectbox.Any(conds...))
	}
	if len(filter.Statuses) > 0 {
		var conds []objectbox.Condition
		for _, st := range filter.Statuses {
			conds = append(conds, TaskEntity_.Status.Equals(string(st), false))
		}
		conditions = append(conditions, objectbox.Any(conds...))
	}
	if len(filter.TriggeredBy) > 0 {
		var conds []objectbox.Condition
		for _, tb := range filter.TriggeredBy {
			conds = append(conds, TaskEntity_.TriggeredBy.Equals(tb, false))
		}
		conditions = append(conditions, objectbox.Any(conds...))
	}
	if filter.RunID != "" {
		conditions = append(conditions, TaskEntity_.RunID.Equals(filter.RunID, false))
	}
	if filter.RollbackOf != "" {
		conditions = append(conditions, TaskEntity_.RollbackOf.Equals(filter.RollbackOf, false))
	}
	if filter.ServiceName != "" {
		conditions = append(conditions, TaskEntity_.ServiceName.Equals(filter.ServiceName, false))
	}
	if filter.TenantID != "" {
		conditions = append(conditions, TaskEntity_.TenantID.Equals(filter.TenantID, false))
	}

	var q *TaskEntityQuery
	if len(conditions) > 0 {
		q = s.tasks.Query(append(conditions, TaskEntity_.CreatedAt.OrderDesc())...)
	} else {
		q = s.tasks.Query(TaskEntity_.CreatedAt.OrderDesc())
	}
	defer q.Close()

	if filter.Limit > 0 {
		q.Limit(uint64(filter.Limit)).Offset(uint64(filter.Offset))
	}

	results, err := q.Find()
	if err != nil {
		return nil, err
	}

	tasks := make([]tasksystem.TaskData, len(results))
	for i, r := range results {
		tasks[i] = taskEntityToData(r)
	}
	return tasks, nil
}

func (s *Store) UpdateTask(task tasksystem.TaskData) error {
	return s.ob.RunInWriteTx(func() error {
		q := s.tasks.Query(TaskEntity_.TaskID.Equals(task.ID, false))
		results, err := q.Find()
		q.Close()
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return tasksystem.ErrNotFound
		}
		entity := taskDataToEntity(task)
		entity.Id = results[0].Id
		return s.tasks.Update(entity)
	})
}

func (s *Store) DeleteTask(id string) error {
	q := s.tasks.Query(TaskEntity_.TaskID.Equals(id, false))
	defer q.Close()
	ids, err := q.FindIds()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = s.tasks.Box.RemoveIds(ids...)
	return err
}

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
	q := s.snapshots.Query(
		SnapshotEntity_.TaskID.Equals(taskID, false),
		SnapshotEntity_.SnapshotType.OrderAsc(false),
		SnapshotEntity_.CreatedAt.OrderDesc(),
	)
	defer q.Close()
	q.Limit(1)

	results, err := q.Find()
	if err != nil {
		return tasksystem.Snapshot{}, err
	}
	if len(results) == 0 {
		return tasksystem.Snapshot{}, tasksystem.ErrNotFound
	}
	e := results[0]
	return tasksystem.Snapshot{
		ID:           fmt.Sprintf("%d", e.Id),
		TaskID:       e.TaskID,
		SnapshotType: e.SnapshotType,
		Data:         string(e.Data),
		CreatedAt:    time.UnixMilli(e.CreatedAt),
	}, nil
}

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
	q := s.trash.Query(TrashEntity_.TrashID.Equals(id, false))
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return tasksystem.TrashItem{}, err
	}
	if len(results) == 0 {
		return tasksystem.TrashItem{}, tasksystem.ErrNotFound
	}
	return trashEntityToItem(results[0]), nil
}

func (s *Store) GetTrashByTaskID(taskID string) (tasksystem.TrashItem, error) {
	q := s.trash.Query(TrashEntity_.TaskID.Equals(taskID, false))
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return tasksystem.TrashItem{}, err
	}
	if len(results) == 0 {
		return tasksystem.TrashItem{}, tasksystem.ErrNotFound
	}
	return trashEntityToItem(results[0]), nil
}

func (s *Store) ListTrash() ([]tasksystem.TrashItem, error) {
	q := s.trash.Query(TrashEntity_.DeletedAt.OrderDesc())
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return nil, err
	}
	items := make([]tasksystem.TrashItem, len(results))
	for i, r := range results {
		items[i] = trashEntityToItem(r)
	}
	return items, nil
}

func (s *Store) UpdateTrash(item tasksystem.TrashItem) error {
	q := s.trash.Query(TrashEntity_.TrashID.Equals(item.ID, false))
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return tasksystem.ErrNotFound
	}
	entity := trashItemToEntity(item)
	entity.Id = results[0].Id
	return s.trash.Update(entity)
}

func (s *Store) DeleteTrash(id string) error {
	q := s.trash.Query(TrashEntity_.TrashID.Equals(id, false))
	defer q.Close()
	ids, err := q.FindIds()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = s.trash.Box.RemoveIds(ids...)
	return err
}

func (s *Store) CountByRunId(runId string) (map[string]int, error) {
	if runId == "" {
		return map[string]int{}, nil
	}
	q := s.tasks.Query(TaskEntity_.RunID.Equals(runId, false))
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Status]++
	}
	return counts, nil
}

func (s *Store) ListRuns() ([]tasksystem.RunInfo, error) {
	q := s.tasks.Query(TaskEntity_.CreatedAt.OrderAsc())
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return nil, err
	}

	type runInfo struct {
		startedAt   time.Time
		triggeredBy string
	}
	runsMap := make(map[string]*runInfo)
	for _, r := range results {
		if r.RunID == "" {
			continue
		}
		if _, ok := runsMap[r.RunID]; !ok {
			runsMap[r.RunID] = &runInfo{
				startedAt:   time.UnixMilli(r.CreatedAt),
				triggeredBy: r.TriggeredBy,
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
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j].StartedAt.After(runs[i].StartedAt) {
				runs[i], runs[j] = runs[j], runs[i]
			}
		}
	}
	return runs, nil
}

func (s *Store) SaveMetrics(m performance.PerformanceMetrics) error {
	entity := metricsToEntity(m)
	_, err := s.metrics.Put(entity)
	return err
}

func (s *Store) GetMetrics(taskID string) (performance.PerformanceMetrics, error) {
	q := s.metrics.Query(PerformanceMetricsEntity_.TaskID.Equals(taskID, false))
	defer q.Close()
	results, err := q.Find()
	if err != nil {
		return performance.PerformanceMetrics{}, err
	}
	if len(results) == 0 {
		return performance.PerformanceMetrics{}, tasksystem.ErrNotFound
	}
	return entityToMetrics(results[0]), nil
}

func (s *Store) ListMetricsByPlugin(pluginName string, taskType string, limit int) ([]performance.PerformanceMetrics, error) {
	if limit <= 0 {
		limit = 10
	}
	q := s.metrics.Query(
		objectbox.All(
			PerformanceMetricsEntity_.PluginName.Equals(pluginName, false),
			PerformanceMetricsEntity_.TaskType.Equals(taskType, false),
		),
		PerformanceMetricsEntity_.CreatedAt.OrderDesc(),
	)
	defer q.Close()
	q.Limit(uint64(limit))
	results, err := q.Find()
	if err != nil {
		return nil, err
	}
	ms := make([]performance.PerformanceMetrics, len(results))
	for i, r := range results {
		ms[i] = entityToMetrics(r)
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

func (s *Store) SaveCalibration(cal performance.CalibrationResult) error {
	entity := &CalibrationEntity{
		CPUScore:      cal.CPUScore,
		AESThroughput: cal.AESThroughput,
		CPULabel:      cal.CPULabel,
		CalibratedAt:  cal.CalibratedAt.UnixMilli(),
		GoVersion:     cal.GoVersion,
		OS:            cal.OS,
		Arch:          cal.Arch,
		NumCPU:        cal.NumCPU,
	}
	existing, err := s.calib.Get(1)
	if err != nil {
		return err
	}
	if existing != nil {
		entity.Id = 1
		return s.calib.Update(entity)
	}
	id, err := s.calib.Put(entity)
	if err != nil {
		return err
	}
	if id != 1 {
		_ = s.calib.RemoveId(1)
		return fmt.Errorf("calibration id mismatch: got %d, expected 1", id)
	}
	return nil
}

func (s *Store) GetCalibration() (*performance.CalibrationResult, error) {
	e, err := s.calib.Get(1)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, nil
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

func (s *Store) ExportAll() (*tasksystem.DatabaseDump, error) {
	tasks, err := s.ListTasks(tasksystem.TaskFilter{Limit: 100000})
	if err != nil {
		return nil, fmt.Errorf("export tasks: %w", err)
	}

	trash, err := s.ListTrash()
	if err != nil {
		return nil, fmt.Errorf("export trash: %w", err)
	}

	var snapshots []tasksystem.Snapshot
	q := s.snapshots.Query()
	defer q.Close()
	snapResults, err := q.Find()
	if err != nil {
		return nil, fmt.Errorf("export snapshots: %w", err)
	}
	for _, r := range snapResults {
		snapshots = append(snapshots, tasksystem.Snapshot{
			ID:           fmt.Sprintf("%d", r.Id),
			TaskID:       r.TaskID,
			SnapshotType: r.SnapshotType,
			Data:         string(r.Data),
			CreatedAt:    time.UnixMilli(r.CreatedAt),
		})
	}

	var allMetrics []performance.PerformanceMetrics
	mq := s.metrics.Query()
	defer mq.Close()
	mResults, err := mq.Find()
	if err != nil {
		return nil, fmt.Errorf("export metrics: %w", err)
	}
	for _, r := range mResults {
		allMetrics = append(allMetrics, entityToMetrics(r))
	}

	cal, err := s.GetCalibration()
	if err != nil {
		return nil, fmt.Errorf("export calibration: %w", err)
	}

	return &tasksystem.DatabaseDump{
		Version:     1,
		Engine:      "objectbox",
		ExportedAt:  time.Now().UTC(),
		Tasks:       tasks,
		Trash:       trash,
		Snapshots:   snapshots,
		Metrics:     allMetrics,
		Calibration: cal,
	}, nil
}

func (s *Store) ImportAll(dump *tasksystem.DatabaseDump) error {
	_ = s.tasks.RemoveAll()
	_ = s.snapshots.RemoveAll()
	_ = s.trash.RemoveAll()
	_ = s.metrics.RemoveAll()
	_ = s.calib.RemoveAll()

	for _, t := range dump.Tasks {
		if err := s.CreateTask(t); err != nil {
			return fmt.Errorf("import task %s: %w", t.ID, err)
		}
	}

	for _, t := range dump.Trash {
		if err := s.CreateTrash(t); err != nil {
			return fmt.Errorf("import trash %s: %w", t.ID, err)
		}
	}

	for _, sn := range dump.Snapshots {
		if err := s.SaveSnapshot(sn); err != nil {
			return fmt.Errorf("import snapshot: %w", err)
		}
	}

	for _, m := range dump.Metrics {
		if err := s.SaveMetrics(m); err != nil {
			return fmt.Errorf("import metrics: %w", err)
		}
	}

	if dump.Calibration != nil {
		if err := s.SaveCalibration(*dump.Calibration); err != nil {
			return fmt.Errorf("import calibration: %w", err)
		}
	}

	return nil
}

func taskDataToEntity(t tasksystem.TaskData) *TaskEntity {
	e := &TaskEntity{
		TaskID:             t.ID,
		Type:               string(t.Type),
		Status:             string(t.Status),
		ServiceName:        t.ServiceName,
		MethodName:         t.MethodName,
		TenantID:           t.TenantID,
		TriggeredBy:        t.TriggeredBy,
		RunID:              t.RunID,
		SourcePath:         t.SourcePath,
		TargetPath:         t.TargetPath,
		OutputPath:         t.OutputPath,
		PluginName:         t.PluginName,
		Progress:           t.Progress,
		Phase:              t.Phase,
		Error:              t.Error,
		ErrorDetail:        t.ErrorDetail,
		Warning:            t.Warning,
		WarningDetail:      t.WarningDetail,
		ContainerVersion:   t.ContainerVersion,
		CipherMode:         t.CipherMode,
		CompressionMode:    t.CompressionMode,
		ExtraFields:        t.ExtraFields,
		Steps:              t.Steps,
		MountID:            t.MountID,
		MountSubPath:       t.MountSubPath,
		TargetMountID:      t.TargetMountID,
		TargetMountSubPath: t.TargetMountSubPath,
		Password:           t.Password,
		SecondaryPassword:  t.SecondaryPassword,
		DurationMs:         t.DurationMs,
		InputJSON:          t.InputJSON,
		OutputJSON:         t.OutputJSON,
		Attempts:           t.Attempts,
		Priority:           t.Priority,
		TagsJSON:           t.TagsJSON,
		CreatedAt:          t.CreatedAt.UnixMilli(),
		RollbackOf:         t.RollbackOf,
		OriginalPath:       t.OriginalPath,
	}
	if t.CompletedAt != nil {
		e.CompletedAt = t.CompletedAt.UnixMilli()
	}
	return e
}

func taskEntityToData(e *TaskEntity) tasksystem.TaskData {
	t := tasksystem.TaskData{
		ID:                 e.TaskID,
		Type:               tasksystem.TaskType(e.Type),
		Status:             tasksystem.TaskStatus(e.Status),
		ServiceName:        e.ServiceName,
		MethodName:         e.MethodName,
		TenantID:           e.TenantID,
		TriggeredBy:        e.TriggeredBy,
		RunID:              e.RunID,
		SourcePath:         e.SourcePath,
		TargetPath:         e.TargetPath,
		OutputPath:         e.OutputPath,
		PluginName:         e.PluginName,
		Progress:           e.Progress,
		Phase:              e.Phase,
		Error:              e.Error,
		ErrorDetail:        e.ErrorDetail,
		Warning:            e.Warning,
		WarningDetail:      e.WarningDetail,
		ContainerVersion:   e.ContainerVersion,
		CipherMode:         e.CipherMode,
		CompressionMode:    e.CompressionMode,
		ExtraFields:        e.ExtraFields,
		Steps:              e.Steps,
		MountID:            e.MountID,
		MountSubPath:       e.MountSubPath,
		TargetMountID:      e.TargetMountID,
		TargetMountSubPath: e.TargetMountSubPath,
		Password:           e.Password,
		SecondaryPassword:  e.SecondaryPassword,
		DurationMs:         e.DurationMs,
		InputJSON:          e.InputJSON,
		OutputJSON:         e.OutputJSON,
		Attempts:           e.Attempts,
		Priority:           e.Priority,
		TagsJSON:           e.TagsJSON,
		CreatedAt:          time.UnixMilli(e.CreatedAt),
		RollbackOf:         e.RollbackOf,
		OriginalPath:       e.OriginalPath,
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
		Metadata:        e.Metadata,
	}
}

func metricsToEntity(m performance.PerformanceMetrics) *PerformanceMetricsEntity {
	phaseTimingsJSON, _ := json.Marshal(m.PhaseTimings)
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
		PhaseTimingsJSON: string(phaseTimingsJSON),
		Grade:            string(m.Grade),
		GradeScore:       m.GradeScore,
		GradeReason:      m.GradeReason,
		CPUScore:         m.CPUScore,
		CPULabel:         m.CPULabel,
		CreatedAt:        m.CreatedAt.UnixMilli(),
	}
}

func entityToMetrics(e *PerformanceMetricsEntity) performance.PerformanceMetrics {
	m := performance.PerformanceMetrics{
		TaskID:           e.TaskID,
		TaskType:         e.TaskType,
		PluginName:       e.PluginName,
		ContainerVer:     e.ContainerVersion,
		CipherMode:       e.CipherMode,
		CompressionMode:  e.CompressionMode,
		SourceSize:       e.SourceSize,
		OutputSize:       e.OutputSize,
		SizeRatio:        e.SizeRatio,
		AvgThroughput:    e.AvgThroughput,
		PeakThroughput:   e.PeakThroughput,
		P50Throughput:    e.P50Throughput,
		P99Throughput:    e.P99Throughput,
		TotalDurationMs:  e.TotalDurationMs,
		Grade:            performance.Grade(e.Grade),
		GradeScore:       e.GradeScore,
		GradeReason:      e.GradeReason,
		CPUScore:         e.CPUScore,
		CPULabel:         e.CPULabel,
		CreatedAt:        time.UnixMilli(e.CreatedAt),
	}
	if e.PhaseTimingsJSON != "" {
		_ = json.Unmarshal([]byte(e.PhaseTimingsJSON), &m.PhaseTimings)
	}
	return m
}
