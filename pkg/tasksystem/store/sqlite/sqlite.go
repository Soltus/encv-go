// Package sqlite 提供 tasksystem.Store 的 SQLite 实现。
//
// 使用 modernc.org/sqlite（pure-Go，CGO_ENABLED=0），符合 android.md 规则。
// 启用 WAL 模式提升并发读写性能。
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// Store SQLite 存储实现。
type Store struct {
	db *sql.DB
}

// New 创建 SQLite Store。
// dbPath 为数据库文件路径（如 /storage/emulated/0/.encv-tasks.db）。
func New(dbPath string) (*Store, error) {
	// _pragma=foreign_keys(1) 启用外键约束
	// _pragma=journal_mode(WAL) 启用 WAL 模式提升并发
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// 单连接，避免 SQLite 并发写冲突（WAL 模式下读可并发）
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return store, nil
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
    created_at DATETIME NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_snapshots_task_id ON rollback_snapshots(task_id);
`

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
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
		task       tasksystem.TaskData
		taskType   string
		status     string
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
		item    tasksystem.TrashItem
		isDir   int
		taskID  sql.NullString
		restoreTaskID sql.NullString
		metadata sql.NullString
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

// Now 返回当前时间（方便测试 mock）。
func Now() time.Time {
	return time.Now().UTC()
}
