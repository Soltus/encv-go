package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// MigrateFromJSON 从 .encv-tasks.json 文件迁移任务到 SQLite。
//
// 行为：
//   - 若 jsonPath 不存在：返回 nil（无迁移需要）
//   - 若 dbPath 已存在：返回 nil（已迁移过）
//   - 否则读取 JSON，逐条插入 SQLite，迁移成功后将 JSON 重命名为 .migrated
//   - 迁移失败时返回错误，不修改 JSON 文件
//
// JSON 格式：[]*MobileTask（与 internal/service/task_manager.go 的 saveTasks 输出一致）。
// 为避免循环依赖，这里用本地 struct 解析 JSON，字段与 MobileTask 对齐。
func MigrateFromJSON(store *Store, jsonPath string) error {
	// JSON 文件不存在，无迁移需要
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat json file: %w", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read json file: %w", err)
	}

	// 解析 JSON（格式与 MobileTask JSON tag 对齐）
	var tasks []migrateTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}

	// 逐条迁移
	for _, mt := range tasks {
		// 运行中/排队中的任务标记为 failed（与旧 loadTasks 行为一致）
		status := mt.Status
		if status == "running" || status == "queued" {
			status = "failed"
			mt.Error = "interrupted by restart"
			if mt.CompletedAt == nil {
				now := time.Now().UTC()
				mt.CompletedAt = &now
			}
		} else if status == "cancelling" {
			status = "cancelled"
			if mt.CompletedAt == nil {
				now := time.Now().UTC()
				mt.CompletedAt = &now
			}
		}

		task := tasksystem.TaskData{
			ID:                 mt.ID,
			Type:               tasksystem.TaskType(mt.Type),
			Status:             tasksystem.TaskStatus(status),
			SourcePath:         mt.SourcePath,
			TargetPath:         mt.TargetPath,
			OutputPath:         mt.OutputPath,
			PluginName:         mt.PluginName,
			TriggeredBy:        mt.TriggeredBy,
			RunID:              mt.RunID,
			Progress:           mt.Progress,
			Phase:              mt.Phase,
			Error:              mt.Error,
			ErrorDetail:        mt.ErrorDetail,
			Warning:            mt.Warning,
			WarningDetail:      mt.WarningDetail,
			ContainerVersion:   mt.ContainerVersion,
			CipherMode:         mt.CipherMode,
			CompressionMode:    mt.CompressionMode,
			ExtraFields:        mt.ExtraFieldsJSON,
			Steps:              mt.StepsJSON,
			MountID:            mt.MountID,
			MountSubPath:       mt.MountSubPath,
			TargetMountID:      mt.TargetMountID,
			TargetMountSubPath: mt.TargetMountSubPath,
			Password:           mt.Password,
			SecondaryPassword:  mt.SecondaryPassword,
			CreatedAt:          mt.CreatedAt,
			CompletedAt:        mt.CompletedAt,
			RollbackOf:         mt.RollbackOf,
			OriginalPath:       mt.OriginalPath,
		}

		// 确保 TriggeredBy 有默认值
		if task.TriggeredBy == "" {
			task.TriggeredBy = "user"
		}

		if err := store.CreateTask(task); err != nil {
			return fmt.Errorf("create task %s: %w", task.ID, err)
		}
	}

	// 迁移成功，重命名 JSON 文件为 .migrated
	migratedPath := jsonPath + ".migrated"
	if err := os.Rename(jsonPath, migratedPath); err != nil {
		// 重命名失败不回滚数据（数据已入库），只记录警告
		return fmt.Errorf("rename json to .migrated (data already migrated): %w", err)
	}

	return nil
}

// migrateTask 迁移用的任务结构，字段与 MobileTask JSON tag 对齐。
// 不直接 import internal/service 避免循环依赖。
type migrateTask struct {
	ID                 string     `json:"id"`
	Type               string     `json:"type"`
	SourcePath         string     `json:"sourcePath"`
	TargetPath         string     `json:"targetPath,omitempty"`
	Password           string     `json:"password,omitempty"`
	SecondaryPassword  string     `json:"secondaryPassword,omitempty"`
	ExtraFieldsJSON    string     `json:"extraFields,omitempty"` // 原为 map，迁移时重新编码
	PluginName         string     `json:"pluginName,omitempty"`
	Status             string     `json:"status"`
	Progress           int        `json:"progress"`
	Phase              string     `json:"phase,omitempty"`
	Error              string     `json:"error,omitempty"`
	ErrorDetail        string     `json:"errorDetail,omitempty"`
	Warning            string     `json:"warning,omitempty"`
	WarningDetail      string     `json:"warningDetail,omitempty"`
	ContainerVersion   int        `json:"containerVersion,omitempty"`
	OutputPath         string     `json:"outputPath,omitempty"`
	StepsJSON          string     `json:"steps,omitempty"` // 原为 []TaskStep，迁移时重新编码
	CipherMode         int        `json:"cipherMode,omitempty"`
	CompressionMode    string     `json:"compressionMode,omitempty"`
	RunID              string     `json:"runId,omitempty"`
	TriggeredBy        string     `json:"triggeredBy,omitempty"`
	MountID            string     `json:"mountId,omitempty"`
	MountSubPath       string     `json:"mountSubPath,omitempty"`
	TargetMountID      string     `json:"targetMountId,omitempty"`
	TargetMountSubPath string     `json:"targetMountSubPath,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	RollbackOf         string     `json:"rollbackOf,omitempty"`
	OriginalPath       string     `json:"originalPath,omitempty"`
}

// MigrateIfNeeded 检查并执行迁移（便捷函数）。
//
// 参数：
//   - dbPath: SQLite 数据库文件路径（必落应用数据目录，绝不进 servingDir）
//   - servingDir: serving 目录（仅用于向后兼容查找 Legacy 的 .encv-tasks.json）
//
// 续43 脉络：任务持久化已从 servingDir 迁到 config.AppDataDir("tasks")。本函数同时
// 在「遗留 servingDir」与「数据目录」查找旧 json，任一存在即迁移（避免迁移顺序变化
// 导致漏迁）。绝不把任何新数据写回 servingDir。
//
// 若 dbPath 不存在且 jsonPath 存在，执行迁移。
func MigrateIfNeeded(dbPath, servingDir string) error {
	// 候选旧 json：先遗留 servingDir（旧版误放），再数据目录（migrateLegacyAppData 已迁出）。
	candidates := []string{
		filepath.Join(servingDir, ".encv-tasks.json"),
		filepath.Join(config.AppDataDir("tasks"), ".encv-tasks.json"),
	}
	jsonPath := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			jsonPath = c
			break
		}
	}
	if jsonPath == "" {
		// 无旧 json，无需迁移（db 可能已存在或首次启动）。
		return nil
	}

	// dbPath 已存在，检查是否已迁移
	if _, err := os.Stat(dbPath); err == nil {
		// db 已存在，但 JSON 也存在（未重命名为 .migrated），说明上次迁移中断
		store, err := New(dbPath)
		if err != nil {
			return fmt.Errorf("open store for re-migrate: %w", err)
		}
		defer store.Close()
		return MigrateFromJSON(store, jsonPath)
	}

	// dbPath 不存在，创建新 store 并迁移
	store, err := New(dbPath)
	if err != nil {
		return fmt.Errorf("create store for migrate: %w", err)
	}
	defer store.Close()
	return MigrateFromJSON(store, jsonPath)
}
