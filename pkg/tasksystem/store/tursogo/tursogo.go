// Package tursogo 提供 tasksystem.Store 的 Turso 正统引擎实现。
//
// Turso Database 是从底层重写的 SQLite 兼容引擎，核心特性：
//   - MVCC 并发写（多 writer 不阻塞，告别 "database is locked"）
//   - 异步 I/O
//   - Turso Sync（本地 + 云端 push/pull 同步）
//   - 纯 Go + purego，无 CGO 依赖
//
// 本实现支持三种模式：
//  1. 本地文件模式（`turso://path/to/file.db`）
//  2. 内存模式（`:memory:`）
//  3. Turso Sync 模式（本地 + 云端同步，支持 Push/Pull）
//
// 使用方式：
//
//	// 本地文件模式
//	store, err := tursogo.NewLocal("app.db")
//
//	// Turso Sync 模式（推荐：本地读写 + 云端同步）
//	store, err := tursogo.NewSync(tursogo.SyncConfig{
//	    Path:       "local.db",
//	    RemoteURL:  "libsql://your-db.turso.io",
//	    AuthToken:  "your-auth-token",
//	})
//
//	// 手动同步
//	store.Push(ctx)  // 本地 → 云端
//	store.Pull(ctx)  // 云端 → 本地
package tursogo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	turso "turso.tech/database/tursogo"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

// Store Turso 正统引擎存储实现。
//
// 相比 libsql 兼容层，tursogo 提供：
//   - MVCC 并发写（多 goroutine 同时写不锁库）
//   - 更高的读性能（异步 I/O）
//   - Turso Sync（显式 push/pull）
//   - 更活跃的开发迭代
type Store struct {
	db     *sql.DB
	syncDb *turso.TursoSyncDb // 仅 Sync 模式下非 nil
}

// SyncConfig Turso Sync 模式配置。
type SyncConfig struct {
	Path      string // 本地数据库文件路径
	RemoteURL string // Turso Cloud 数据库 URL（libsql://...）
	AuthToken string // 认证 token
}

// NewLocal 创建本地文件模式的 Turso Store（无云端同步）。
//
// path: 本地数据库文件路径，使用 `:memory:` 可创建内存数据库。
func NewLocal(path string) (*Store, error) {
	db, err := sql.Open("turso", path)
	if err != nil {
		return nil, fmt.Errorf("open turso local: %w", err)
	}

	// Turso 支持 MVCC 并发写，可安全地增加连接数
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(10 * time.Minute)

	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return store, nil
}

// NewSync 创建带 Turso Sync 的 Store（本地读写 + 云端同步）。
//
// 所有读写均走本地数据库（低延迟、离线可用），
// 通过 Push()/Pull() 显式与云端同步。
func NewSync(cfg SyncConfig) (*Store, error) {
	ctx := context.Background()

	syncDb, err := turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
		Path:      cfg.Path,
		RemoteUrl: cfg.RemoteURL,
		AuthToken: cfg.AuthToken,
	})
	if err != nil {
		return nil, fmt.Errorf("create turso sync db: %w", err)
	}

	db, err := syncDb.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect turso sync db: %w", err)
	}

	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(10 * time.Minute)

	store := &Store{db: db, syncDb: syncDb}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return store, nil
}

// Push 将本地变更推送到 Turso Cloud。
// 仅 Sync 模式下有效；本地模式返回 nil（no-op）。
func (s *Store) Push(ctx context.Context) error {
	if s.syncDb == nil {
		return nil
	}
	return s.syncDb.Push(ctx)
}

// Pull 从 Turso Cloud 拉取远程变更到本地。
// 仅 Sync 模式下有效；本地模式返回 nil（no-op）。
func (s *Store) Pull(ctx context.Context) error {
	if s.syncDb == nil {
		return nil
	}
	_, err := s.syncDb.Pull(ctx)
	return err
}

// IsSyncMode 返回是否为 Sync 模式。
func (s *Store) IsSyncMode() bool {
	return s.syncDb != nil
}

// initSchema 创建表和索引（IF NOT EXISTS，幂等）。
func (s *Store) initSchema() error {
	_, err := s.db.Exec(schemaSQL)
	return err
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    source_path TEXT,
    target_path TEXT,
    output_path TEXT,
    plugin_name TEXT,
    triggered_by TEXT DEFAULT 'user',
    run_id TEXT,
    progress INTEGER DEFAULT 0,
    phase TEXT,
    error TEXT,
    error_detail TEXT,
    warning TEXT,
    warning_detail TEXT,
    container_version INTEGER,
    cipher_mode INTEGER,
    compression_mode TEXT,
    extra_fields TEXT,
    steps TEXT,
    mount_id TEXT,
    mount_sub_path TEXT,
    target_mount_id TEXT,
    target_mount_sub_path TEXT,
    password TEXT,
    secondary_password TEXT,
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    rollback_of TEXT,
    original_path TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_run_id ON tasks(run_id);
CREATE INDEX IF NOT EXISTS idx_tasks_triggered_by ON tasks(triggered_by);
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
CREATE INDEX IF NOT EXISTS idx_tasks_rollback_of ON tasks(rollback_of);

CREATE TABLE IF NOT EXISTS trash (
    id TEXT PRIMARY KEY,
    original_path TEXT NOT NULL,
    trash_path TEXT NOT NULL,
    is_directory INTEGER DEFAULT 0,
    size INTEGER DEFAULT 0,
    deleted_at DATETIME NOT NULL,
    task_id TEXT,
    restore_task_id TEXT,
    metadata TEXT
);
CREATE INDEX IF NOT EXISTS idx_trash_deleted_at ON trash(deleted_at);
CREATE INDEX IF NOT EXISTS idx_trash_original_path ON trash(original_path);
CREATE INDEX IF NOT EXISTS idx_trash_task_id ON trash(task_id);

CREATE TABLE IF NOT EXISTS rollback_snapshots (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    snapshot_type TEXT NOT NULL,
    snapshot_data TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_task_id ON rollback_snapshots(task_id);

CREATE TABLE IF NOT EXISTS performance_metrics (
    task_id TEXT PRIMARY KEY,
    task_type TEXT NOT NULL,
    plugin_name TEXT,
    container_version INTEGER,
    cipher_mode INTEGER,
    compression_mode TEXT,
    source_size INTEGER,
    output_size INTEGER,
    size_ratio REAL,
    avg_throughput REAL,
    peak_throughput REAL,
    p50_throughput REAL,
    p99_throughput REAL,
    total_duration_ms INTEGER,
    phase_timings_json TEXT,
    grade TEXT,
    grade_score REAL,
    grade_reason TEXT,
    cpu_score REAL,
    cpu_label TEXT,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_perf_plugin_type ON performance_metrics(plugin_name, task_type);
CREATE INDEX IF NOT EXISTS idx_perf_created_at ON performance_metrics(created_at);
CREATE INDEX IF NOT EXISTS idx_perf_grade ON performance_metrics(grade);

CREATE TABLE IF NOT EXISTS calibration (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    cpu_score REAL NOT NULL,
    aes_throughput REAL NOT NULL,
    cpu_label TEXT NOT NULL,
    calibrated_at DATETIME NOT NULL,
    go_version TEXT,
    os TEXT,
    arch TEXT,
    num_cpu INTEGER
);
`

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// ConcurrencyHint 返回推荐的并发写入协程数。
// Turso MVCC 支持多 writer 并发写，返回 8。
func (s *Store) ConcurrencyHint() int {
	return 8
}

// CreateTask 创建任务。
func (s *Store) CreateTask(task tasksystem.TaskData) error {
	_, err := s.db.Exec(`
		INSERT INTO tasks (
			id, type, status, source_path, target_path, output_path,
			plugin_name, triggered_by, run_id, progress, phase,
			error, error_detail, warning, warning_detail,
			container_version, cipher_mode, compression_mode,
			extra_fields, steps, mount_id, mount_sub_path,
			target_mount_id, target_mount_sub_path,
			password, secondary_password,
			created_at, completed_at, rollback_of, original_path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, string(task.Type), string(task.Status),
		task.SourcePath, task.TargetPath, task.OutputPath,
		task.PluginName, task.TriggeredBy, task.RunID, task.Progress, task.Phase,
		task.Error, task.ErrorDetail, task.Warning, task.WarningDetail,
		task.ContainerVersion, task.CipherMode, task.CompressionMode,
		task.ExtraFields, task.Steps, task.MountID, task.MountSubPath,
		task.TargetMountID, task.TargetMountSubPath,
		task.Password, task.SecondaryPassword,
		task.CreatedAt, task.CompletedAt, task.RollbackOf, task.OriginalPath,
	)
	return err
}

// GetTask 按 ID 查询任务。
func (s *Store) GetTask(id string) (tasksystem.TaskData, error) {
	row := s.db.QueryRow(tasksSelectAll+" WHERE id = ?", id)
	return scanTask(row)
}

// ListTasks 按过滤条件查询任务列表，按 CreatedAt 倒序。
func (s *Store) ListTasks(filter tasksystem.TaskFilter) ([]tasksystem.TaskData, error) {
	var (
		where []string
		args  []interface{}
	)
	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		where = append(where, "type IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for i, st := range filter.Statuses {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		where = append(where, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(filter.TriggeredBy) > 0 {
		placeholders := make([]string, len(filter.TriggeredBy))
		for i, tb := range filter.TriggeredBy {
			placeholders[i] = "?"
			args = append(args, tb)
		}
		where = append(where, "triggered_by IN ("+strings.Join(placeholders, ",")+")")
	}
	if filter.RunID != "" {
		where = append(where, "run_id = ?")
		args = append(args, filter.RunID)
	}
	if filter.RollbackOf != "" {
		where = append(where, "rollback_of = ?")
		args = append(args, filter.RollbackOf)
	}

	query := tasksSelectAll
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []tasksystem.TaskData
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

// UpdateTask 更新任务（全字段更新）。
func (s *Store) UpdateTask(task tasksystem.TaskData) error {
	_, err := s.db.Exec(`
		UPDATE tasks SET
			type = ?, status = ?, source_path = ?, target_path = ?, output_path = ?,
			plugin_name = ?, triggered_by = ?, run_id = ?, progress = ?, phase = ?,
			error = ?, error_detail = ?, warning = ?, warning_detail = ?,
			container_version = ?, cipher_mode = ?, compression_mode = ?,
			extra_fields = ?, steps = ?, mount_id = ?, mount_sub_path = ?,
			target_mount_id = ?, target_mount_sub_path = ?,
			password = ?, secondary_password = ?,
			completed_at = ?, rollback_of = ?, original_path = ?
		WHERE id = ?`,
		string(task.Type), string(task.Status),
		task.SourcePath, task.TargetPath, task.OutputPath,
		task.PluginName, task.TriggeredBy, task.RunID, task.Progress, task.Phase,
		task.Error, task.ErrorDetail, task.Warning, task.WarningDetail,
		task.ContainerVersion, task.CipherMode, task.CompressionMode,
		task.ExtraFields, task.Steps, task.MountID, task.MountSubPath,
		task.TargetMountID, task.TargetMountSubPath,
		task.Password, task.SecondaryPassword,
		task.CompletedAt, task.RollbackOf, task.OriginalPath,
		task.ID,
	)
	return err
}

// DeleteTask 按 ID 删除任务（硬删除）。
func (s *Store) DeleteTask(id string) error {
	_, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

// CountByRunId 按 runId 统计各状态的任务数。
func (s *Store) CountByRunId(runId string) (map[string]int, error) {
	if runId == "" {
		return map[string]int{}, nil
	}
	rows, err := s.db.Query(
		"SELECT status, COUNT(*) FROM tasks WHERE run_id = ? GROUP BY status",
		runId,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

// ListRuns 列出所有 run（去重 runId），带最早创建时间和 triggeredBy。
func (s *Store) ListRuns() ([]tasksystem.RunInfo, error) {
	rows, err := s.db.Query(`
		SELECT run_id, MIN(created_at), triggered_by
		FROM tasks
		WHERE run_id != ''
		GROUP BY run_id
		ORDER BY MIN(created_at) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []tasksystem.RunInfo
	for rows.Next() {
		var info tasksystem.RunInfo
		var startedAtStr string
		if err := rows.Scan(&info.RunID, &startedAtStr, &info.TriggeredBy); err != nil {
			return nil, err
		}
		parsed, err := parseTime(startedAtStr)
		if err != nil {
			return nil, fmt.Errorf("ListRuns: parse startedAt %q: %w", startedAtStr, err)
		}
		info.StartedAt = parsed
		result = append(result, info)
	}
	return result, rows.Err()
}

// parseTime 解析时间字符串（兼容多种格式）。
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05-07:00", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

// SaveSnapshot 保存任务快照。
func (s *Store) SaveSnapshot(snapshot tasksystem.Snapshot) error {
	_, err := s.db.Exec(`
		INSERT INTO rollback_snapshots (id, task_id, snapshot_type, snapshot_data, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		snapshot.ID, snapshot.TaskID, snapshot.SnapshotType, snapshot.Data, snapshot.CreatedAt,
	)
	return err
}

// GetSnapshot 查询任务的快照（pre_state 优先）。
func (s *Store) GetSnapshot(taskID string) (tasksystem.Snapshot, error) {
	row := s.db.QueryRow(`
		SELECT id, task_id, snapshot_type, snapshot_data, created_at
		FROM rollback_snapshots
		WHERE task_id = ?
		ORDER BY CASE snapshot_type WHEN 'pre_state' THEN 0 ELSE 1 END, created_at DESC
		LIMIT 1`, taskID)
	var snap tasksystem.Snapshot
	var snapshotType string
	err := row.Scan(&snap.ID, &snap.TaskID, &snapshotType, &snap.Data, &snap.CreatedAt)
	if err != nil {
		return tasksystem.Snapshot{}, err
	}
	snap.SnapshotType = snapshotType
	return snap, nil
}

// CreateTrash 创建回收站条目。
func (s *Store) CreateTrash(item tasksystem.TrashItem) error {
	isDir := 0
	if item.IsDirectory {
		isDir = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO trash (id, original_path, trash_path, is_directory, size, deleted_at, task_id, restore_task_id, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.OriginalPath, item.TrashPath, isDir, item.Size,
		item.DeletedAt, item.TaskID, item.RestoreTaskID, item.Metadata,
	)
	return err
}

// GetTrash 按 ID 查询回收站条目。
func (s *Store) GetTrash(id string) (tasksystem.TrashItem, error) {
	row := s.db.QueryRow(`
		SELECT id, original_path, trash_path, is_directory, size, deleted_at, task_id, restore_task_id, metadata
		FROM trash WHERE id = ?`, id)
	return scanTrash(row)
}

// GetTrashByTaskID 按任务 ID 查询回收站条目。
func (s *Store) GetTrashByTaskID(taskID string) (tasksystem.TrashItem, error) {
	row := s.db.QueryRow(`
		SELECT id, original_path, trash_path, is_directory, size, deleted_at, task_id, restore_task_id, metadata
		FROM trash WHERE task_id = ?`, taskID)
	return scanTrash(row)
}

// ListTrash 查询回收站列表，按 DeletedAt 倒序。
func (s *Store) ListTrash() ([]tasksystem.TrashItem, error) {
	rows, err := s.db.Query(`
		SELECT id, original_path, trash_path, is_directory, size, deleted_at, task_id, restore_task_id, metadata
		FROM trash ORDER BY deleted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []tasksystem.TrashItem
	for rows.Next() {
		item, err := scanTrash(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// UpdateTrash 更新回收站条目。
func (s *Store) UpdateTrash(item tasksystem.TrashItem) error {
	isDir := 0
	if item.IsDirectory {
		isDir = 1
	}
	_, err := s.db.Exec(`
		UPDATE trash SET
			original_path = ?, trash_path = ?, is_directory = ?, size = ?,
			deleted_at = ?, task_id = ?, restore_task_id = ?, metadata = ?
		WHERE id = ?`,
		item.OriginalPath, item.TrashPath, isDir, item.Size,
		item.DeletedAt, item.TaskID, item.RestoreTaskID, item.Metadata,
		item.ID,
	)
	return err
}

// DeleteTrash 按 ID 删除回收站条目（硬删除，不删文件）。
func (s *Store) DeleteTrash(id string) error {
	_, err := s.db.Exec("DELETE FROM trash WHERE id = ?", id)
	return err
}

// tasksSelectAll 任务查询的公共字段部分。
const tasksSelectAll = `
	SELECT id, type, status, source_path, target_path, output_path,
		plugin_name, triggered_by, run_id, progress, phase,
		error, error_detail, warning, warning_detail,
		container_version, cipher_mode, compression_mode,
		extra_fields, steps, mount_id, mount_sub_path,
		target_mount_id, target_mount_sub_path,
		password, secondary_password,
		created_at, completed_at, rollback_of, original_path
	FROM tasks`

// scanner 兼容 *sql.Row 和 *sql.Rows 的 Scan 方法。
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanTask 扫描任务行。
func scanTask(s scanner) (tasksystem.TaskData, error) {
	var (
		task        tasksystem.TaskData
		taskType    string
		status      string
		completedAt sql.NullTime
	)
	err := s.Scan(
		&task.ID, &taskType, &status,
		&task.SourcePath, &task.TargetPath, &task.OutputPath,
		&task.PluginName, &task.TriggeredBy, &task.RunID, &task.Progress, &task.Phase,
		&task.Error, &task.ErrorDetail, &task.Warning, &task.WarningDetail,
		&task.ContainerVersion, &task.CipherMode, &task.CompressionMode,
		&task.ExtraFields, &task.Steps, &task.MountID, &task.MountSubPath,
		&task.TargetMountID, &task.TargetMountSubPath,
		&task.Password, &task.SecondaryPassword,
		&task.CreatedAt, &completedAt, &task.RollbackOf, &task.OriginalPath,
	)
	if err != nil {
		return tasksystem.TaskData{}, err
	}
	task.Type = tasksystem.TaskType(taskType)
	task.Status = tasksystem.TaskStatus(status)
	if completedAt.Valid {
		t := completedAt.Time
		task.CompletedAt = &t
	}
	return task, nil
}

// scanTrash 扫描回收站条目行。
func scanTrash(s scanner) (tasksystem.TrashItem, error) {
	var (
		item          tasksystem.TrashItem
		isDir         int
		taskID        sql.NullString
		restoreTaskID sql.NullString
		metadata      sql.NullString
	)
	err := s.Scan(
		&item.ID, &item.OriginalPath, &item.TrashPath, &isDir, &item.Size,
		&item.DeletedAt, &taskID, &restoreTaskID, &metadata,
	)
	if err != nil {
		return tasksystem.TrashItem{}, err
	}
	item.IsDirectory = isDir != 0
	if taskID.Valid {
		item.TaskID = taskID.String
	}
	if restoreTaskID.Valid {
		item.RestoreTaskID = restoreTaskID.String
	}
	if metadata.Valid {
		item.Metadata = metadata.String
	}
	return item, nil
}

// 确保 *Store 实现 tasksystem.Store 接口。
var _ tasksystem.Store = (*Store)(nil)

// ========== Performance Metrics ==========

// SaveMetrics 保存性能指标。
func (s *Store) SaveMetrics(m performance.PerformanceMetrics) error {
	phaseTimingsJSON, err := json.Marshal(m.PhaseTimings)
	if err != nil {
		return fmt.Errorf("marshal phase timings: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO performance_metrics (
			task_id, task_type, plugin_name, container_version, cipher_mode, compression_mode,
			source_size, output_size, size_ratio,
			avg_throughput, peak_throughput, p50_throughput, p99_throughput,
			total_duration_ms, phase_timings_json,
			grade, grade_score, grade_reason,
			cpu_score, cpu_label, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TaskID, m.TaskType, m.PluginName, m.ContainerVer, m.CipherMode, m.CompressionMode,
		m.SourceSize, m.OutputSize, m.SizeRatio,
		m.AvgThroughput, m.PeakThroughput, m.P50Throughput, m.P99Throughput,
		m.TotalDurationMs, string(phaseTimingsJSON),
		string(m.Grade), m.GradeScore, m.GradeReason,
		m.CPUScore, m.CPULabel, m.CreatedAt,
	)
	return err
}

// GetMetrics 按 task_id 查询性能指标。
func (s *Store) GetMetrics(taskID string) (performance.PerformanceMetrics, error) {
	row := s.db.QueryRow(`
		SELECT task_id, task_type, plugin_name, container_version, cipher_mode, compression_mode,
			source_size, output_size, size_ratio,
			avg_throughput, peak_throughput, p50_throughput, p99_throughput,
			total_duration_ms, phase_timings_json,
			grade, grade_score, grade_reason,
			cpu_score, cpu_label, created_at
		FROM performance_metrics WHERE task_id = ?`, taskID)
	return scanMetrics(row)
}

// ListMetricsByPlugin 按 plugin + taskType 查询历史性能指标（按 created_at 倒序，limit 条）。
func (s *Store) ListMetricsByPlugin(pluginName string, taskType string, limit int) ([]performance.PerformanceMetrics, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT task_id, task_type, plugin_name, container_version, cipher_mode, compression_mode,
			source_size, output_size, size_ratio,
			avg_throughput, peak_throughput, p50_throughput, p99_throughput,
			total_duration_ms, phase_timings_json,
			grade, grade_score, grade_reason,
			cpu_score, cpu_label, created_at
		FROM performance_metrics
		WHERE plugin_name = ? AND task_type = ?
		ORDER BY created_at DESC
		LIMIT ?`, pluginName, taskType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []performance.PerformanceMetrics
	for rows.Next() {
		m, err := scanMetrics(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetLatestMetrics 获取同 plugin + taskType 的上一次运行（用于历史对比）。
func (s *Store) GetLatestMetrics(pluginName string, taskType string) (*performance.PerformanceMetrics, error) {
	rows, err := s.db.Query(`
		SELECT task_id, task_type, plugin_name, container_version, cipher_mode, compression_mode,
			source_size, output_size, size_ratio,
			avg_throughput, peak_throughput, p50_throughput, p99_throughput,
			total_duration_ms, phase_timings_json,
			grade, grade_score, grade_reason,
			cpu_score, cpu_label, created_at
		FROM performance_metrics
		WHERE plugin_name = ? AND task_type = ?
		ORDER BY created_at DESC
		LIMIT 1`, pluginName, taskType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	m, err := scanMetrics(rows)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// scanMetrics 从 sql.Row 或 sql.Rows 扫描性能指标。
func scanMetrics(scanner interface{ Scan(dest ...interface{}) error }) (performance.PerformanceMetrics, error) {
	var m performance.PerformanceMetrics
	var phaseTimingsJSON string
	var grade string
	err := scanner.Scan(
		&m.TaskID, &m.TaskType, &m.PluginName, &m.ContainerVer, &m.CipherMode, &m.CompressionMode,
		&m.SourceSize, &m.OutputSize, &m.SizeRatio,
		&m.AvgThroughput, &m.PeakThroughput, &m.P50Throughput, &m.P99Throughput,
		&m.TotalDurationMs, &phaseTimingsJSON,
		&grade, &m.GradeScore, &m.GradeReason,
		&m.CPUScore, &m.CPULabel, &m.CreatedAt,
	)
	if err != nil {
		return m, err
	}
	m.Grade = performance.Grade(grade)
	if phaseTimingsJSON != "" {
		if err := json.Unmarshal([]byte(phaseTimingsJSON), &m.PhaseTimings); err != nil {
			return m, fmt.Errorf("unmarshal phase timings: %w", err)
		}
	}
	return m, nil
}

// ========== Calibration ==========

// SaveCalibration 保存硬件校准结果（单行表，id=1，INSERT OR REPLACE）。
func (s *Store) SaveCalibration(cal performance.CalibrationResult) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO calibration (
			id, cpu_score, aes_throughput, cpu_label, calibrated_at,
			go_version, os, arch, num_cpu
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cal.CPUScore, cal.AESThroughput, cal.CPULabel, cal.CalibratedAt,
		cal.GoVersion, cal.OS, cal.Arch, cal.NumCPU,
	)
	return err
}

// GetCalibration 获取硬件校准结果。返回 nil 表示尚未校准。
func (s *Store) GetCalibration() (*performance.CalibrationResult, error) {
	row := s.db.QueryRow(`
		SELECT cpu_score, aes_throughput, cpu_label, calibrated_at,
			go_version, os, arch, num_cpu
		FROM calibration WHERE id = 1`)
	var cal performance.CalibrationResult
	err := row.Scan(
		&cal.CPUScore, &cal.AESThroughput, &cal.CPULabel, &cal.CalibratedAt,
		&cal.GoVersion, &cal.OS, &cal.Arch, &cal.NumCPU,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cal, nil
}

// ========== 导入 / 导出（跨引擎迁移） ==========

// ExportAll 导出全部数据为通用 DatabaseDump 格式。
func (s *Store) ExportAll() (*tasksystem.DatabaseDump, error) {
	dump := &tasksystem.DatabaseDump{
		Version:    1,
		Engine:     "turso",
		ExportedAt: time.Now(),
	}

	// 导出 tasks
	tasks, err := s.ListTasks(tasksystem.TaskFilter{})
	if err != nil {
		return nil, fmt.Errorf("export tasks: %w", err)
	}
	dump.Tasks = tasks

	// 导出 trash
	trash, err := s.ListTrash()
	if err != nil {
		return nil, fmt.Errorf("export trash: %w", err)
	}
	dump.Trash = trash

	// 导出 snapshots：遍历所有 task 查快照
	var allSnapshots []tasksystem.Snapshot
	for _, t := range tasks {
		snap, err := s.GetSnapshot(t.ID)
		if err == nil {
			allSnapshots = append(allSnapshots, snap)
		}
	}
	dump.Snapshots = allSnapshots

	// 导出 metrics：全量查
	metrics, err := s.exportAllMetrics()
	if err != nil {
		return nil, fmt.Errorf("export metrics: %w", err)
	}
	dump.Metrics = metrics

	// 导出 calibration
	cal, err := s.GetCalibration()
	if err != nil {
		return nil, fmt.Errorf("export calibration: %w", err)
	}
	dump.Calibration = cal

	return dump, nil
}

// exportAllMetrics 导出所有性能指标。
func (s *Store) exportAllMetrics() ([]performance.PerformanceMetrics, error) {
	rows, err := s.db.Query(`
		SELECT task_id, task_type, plugin_name, container_version, cipher_mode, compression_mode,
			source_size, output_size, size_ratio,
			avg_throughput, peak_throughput, p50_throughput, p99_throughput,
			total_duration_ms, phase_timings_json,
			grade, grade_score, grade_reason,
			cpu_score, cpu_label, created_at
		FROM performance_metrics
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []performance.PerformanceMetrics
	for rows.Next() {
		m, err := scanMetrics(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// ImportAll 导入全部数据（全量替换）。
// 导入前会清空所有现有数据。
func (s *Store) ImportAll(dump *tasksystem.DatabaseDump) error {
	if dump == nil {
		return fmt.Errorf("dump is nil")
	}

	// 开启事务，确保原子性
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 清空所有表
	tables := []string{"tasks", "trash", "rollback_snapshots", "performance_metrics", "calibration"}
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("clear table %s: %w", table, err)
		}
	}

	// 导入 tasks
	for _, task := range dump.Tasks {
		if err := s.createTaskTx(tx, task); err != nil {
			return fmt.Errorf("import task %s: %w", task.ID, err)
		}
	}

	// 导入 trash
	for _, item := range dump.Trash {
		if err := s.createTrashTx(tx, item); err != nil {
			return fmt.Errorf("import trash %s: %w", item.ID, err)
		}
	}

	// 导入 snapshots
	for _, snap := range dump.Snapshots {
		if err := s.saveSnapshotTx(tx, snap); err != nil {
			return fmt.Errorf("import snapshot %s: %w", snap.ID, err)
		}
	}

	// 导入 metrics
	for _, m := range dump.Metrics {
		if err := s.saveMetricsTx(tx, m); err != nil {
			return fmt.Errorf("import metrics %s: %w", m.TaskID, err)
		}
	}

	// 导入 calibration
	if dump.Calibration != nil {
		if err := s.saveCalibrationTx(tx, *dump.Calibration); err != nil {
			return fmt.Errorf("import calibration: %w", err)
		}
	}

	return tx.Commit()
}

// createTaskTx 在事务中创建任务（复用 CreateTask 的 SQL）。
func (s *Store) createTaskTx(tx *sql.Tx, task tasksystem.TaskData) error {
	_, err := tx.Exec(`
		INSERT INTO tasks (
			id, type, status, source_path, target_path, output_path,
			plugin_name, triggered_by, run_id, progress, phase,
			error, error_detail, warning, warning_detail,
			container_version, cipher_mode, compression_mode,
			extra_fields, steps, mount_id, mount_sub_path,
			target_mount_id, target_mount_sub_path,
			password, secondary_password,
			created_at, completed_at, rollback_of, original_path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, string(task.Type), string(task.Status),
		task.SourcePath, task.TargetPath, task.OutputPath,
		task.PluginName, task.TriggeredBy, task.RunID, task.Progress, task.Phase,
		task.Error, task.ErrorDetail, task.Warning, task.WarningDetail,
		task.ContainerVersion, task.CipherMode, task.CompressionMode,
		task.ExtraFields, task.Steps, task.MountID, task.MountSubPath,
		task.TargetMountID, task.TargetMountSubPath,
		task.Password, task.SecondaryPassword,
		task.CreatedAt, task.CompletedAt, task.RollbackOf, task.OriginalPath,
	)
	return err
}

// createTrashTx 在事务中创建回收站条目。
func (s *Store) createTrashTx(tx *sql.Tx, item tasksystem.TrashItem) error {
	isDir := 0
	if item.IsDirectory {
		isDir = 1
	}
	_, err := tx.Exec(`
		INSERT INTO trash (id, original_path, trash_path, is_directory, size, deleted_at, task_id, restore_task_id, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.OriginalPath, item.TrashPath, isDir, item.Size,
		item.DeletedAt, item.TaskID, item.RestoreTaskID, item.Metadata,
	)
	return err
}

// saveSnapshotTx 在事务中保存快照。
func (s *Store) saveSnapshotTx(tx *sql.Tx, snapshot tasksystem.Snapshot) error {
	_, err := tx.Exec(`
		INSERT INTO rollback_snapshots (id, task_id, snapshot_type, snapshot_data, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		snapshot.ID, snapshot.TaskID, snapshot.SnapshotType, snapshot.Data, snapshot.CreatedAt,
	)
	return err
}

// saveMetricsTx 在事务中保存性能指标。
func (s *Store) saveMetricsTx(tx *sql.Tx, m performance.PerformanceMetrics) error {
	phaseTimingsJSON, err := json.Marshal(m.PhaseTimings)
	if err != nil {
		return fmt.Errorf("marshal phase timings: %w", err)
	}
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO performance_metrics (
			task_id, task_type, plugin_name, container_version, cipher_mode, compression_mode,
			source_size, output_size, size_ratio,
			avg_throughput, peak_throughput, p50_throughput, p99_throughput,
			total_duration_ms, phase_timings_json,
			grade, grade_score, grade_reason,
			cpu_score, cpu_label, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TaskID, m.TaskType, m.PluginName, m.ContainerVer, m.CipherMode, m.CompressionMode,
		m.SourceSize, m.OutputSize, m.SizeRatio,
		m.AvgThroughput, m.PeakThroughput, m.P50Throughput, m.P99Throughput,
		m.TotalDurationMs, string(phaseTimingsJSON),
		string(m.Grade), m.GradeScore, m.GradeReason,
		m.CPUScore, m.CPULabel, m.CreatedAt,
	)
	return err
}

// saveCalibrationTx 在事务中保存校准结果。
func (s *Store) saveCalibrationTx(tx *sql.Tx, cal performance.CalibrationResult) error {
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO calibration (
			id, cpu_score, aes_throughput, cpu_label, calibrated_at,
			go_version, os, arch, num_cpu
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cal.CPUScore, cal.AESThroughput, cal.CPULabel, cal.CalibratedAt,
		cal.GoVersion, cal.OS, cal.Arch, cal.NumCPU,
	)
	return err
}
