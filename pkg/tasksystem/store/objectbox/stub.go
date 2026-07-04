//go:build !objectbox

package objectbox

import (
	"errors"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

// ErrObjectBoxUnavailable 当前构建未启用 ObjectBox 引擎（编译时未加 -tags objectbox）。
var ErrObjectBoxUnavailable = errors.New("objectbox: engine not available in this build (use -tags objectbox)")

// Store 是 ObjectBox 存储的 stub 实现。
//
// 当构建时不带 objectbox tag 时，编译此文件，所有方法都返回
// ErrObjectBoxUnavailable，确保不引入 CGO 和 C 库依赖。
//
// 带 objectbox tag 构建时，编译 objectbox.go 中的真实实现。
type Store struct{}

// New 创建 stub store（始终返回错误）。
func New(dbDir string) (*Store, error) {
	return nil, ErrObjectBoxUnavailable
}

// EngineName 返回引擎名称。
func (s *Store) EngineName() string { return "objectbox" }

// ConcurrencyHint 返回推荐并发数。
func (s *Store) ConcurrencyHint() int { return 4 }

// Close 关闭 store。
func (s *Store) Close() error { return nil }

func (s *Store) CreateTask(task tasksystem.TaskData) error { return ErrObjectBoxUnavailable }
func (s *Store) GetTask(id string) (tasksystem.TaskData, error) {
	return tasksystem.TaskData{}, ErrObjectBoxUnavailable
}
func (s *Store) ListTasks(filter tasksystem.TaskFilter) ([]tasksystem.TaskData, error) {
	return nil, ErrObjectBoxUnavailable
}
func (s *Store) UpdateTask(task tasksystem.TaskData) error { return ErrObjectBoxUnavailable }
func (s *Store) DeleteTask(id string) error { return ErrObjectBoxUnavailable }

func (s *Store) SaveSnapshot(snapshot tasksystem.Snapshot) error {
	return ErrObjectBoxUnavailable
}
func (s *Store) GetSnapshot(taskID string) (tasksystem.Snapshot, error) {
	return tasksystem.Snapshot{}, ErrObjectBoxUnavailable
}

func (s *Store) CreateTrash(item tasksystem.TrashItem) error { return ErrObjectBoxUnavailable }
func (s *Store) GetTrash(id string) (tasksystem.TrashItem, error) {
	return tasksystem.TrashItem{}, ErrObjectBoxUnavailable
}
func (s *Store) GetTrashByTaskID(taskID string) (tasksystem.TrashItem, error) {
	return tasksystem.TrashItem{}, ErrObjectBoxUnavailable
}
func (s *Store) ListTrash() ([]tasksystem.TrashItem, error) { return nil, ErrObjectBoxUnavailable }
func (s *Store) UpdateTrash(item tasksystem.TrashItem) error { return ErrObjectBoxUnavailable }
func (s *Store) DeleteTrash(id string) error { return ErrObjectBoxUnavailable }

func (s *Store) CountByRunId(runId string) (map[string]int, error) {
	return nil, ErrObjectBoxUnavailable
}
func (s *Store) ListRuns() ([]tasksystem.RunInfo, error) { return nil, ErrObjectBoxUnavailable }

func (s *Store) SaveMetrics(m performance.PerformanceMetrics) error {
	return ErrObjectBoxUnavailable
}
func (s *Store) GetMetrics(taskID string) (performance.PerformanceMetrics, error) {
	return performance.PerformanceMetrics{}, ErrObjectBoxUnavailable
}
func (s *Store) ListMetricsByPlugin(pluginName string, taskType string, limit int) ([]performance.PerformanceMetrics, error) {
	return nil, ErrObjectBoxUnavailable
}
func (s *Store) GetLatestMetrics(pluginName string, taskType string) (*performance.PerformanceMetrics, error) {
	return nil, ErrObjectBoxUnavailable
}

func (s *Store) SaveCalibration(cal performance.CalibrationResult) error {
	return ErrObjectBoxUnavailable
}
func (s *Store) GetCalibration() (*performance.CalibrationResult, error) {
	return nil, ErrObjectBoxUnavailable
}

func (s *Store) ExportAll() (*tasksystem.DatabaseDump, error) {
	return nil, ErrObjectBoxUnavailable
}
func (s *Store) ImportAll(dump *tasksystem.DatabaseDump) error {
	return ErrObjectBoxUnavailable
}
