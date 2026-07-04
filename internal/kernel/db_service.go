package kernel

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// DBService 数据库微服务（封装 tasksystem.Store 为 kernel.Service）。
//
// 定位：
//   - 将 DB 引擎作为微内核中的一等服务，按需激活/停用
//   - 支持多引擎（sqlite / libsql / turso），通过 factory 注入
//   - 所有微服务的 DB 操作统一走此服务（方便未来切换引擎、加缓存、加审计）
//
// 注意：
//   - 此服务是薄封装，不做业务逻辑，只做方法调度
//   - 实际的 Store 实现由工厂函数提供（SQLite / libsql / turso）
type DBService struct {
	store  tasksystem.Store
	engine string
}

// DBServiceConfig 数据库服务配置
type DBServiceConfig struct {
	Engine  string                // "sqlite" | "libsql" | "turso"
	Factory func() (tasksystem.Store, error)
}

// NewDBService 创建数据库服务（延迟初始化，Init 时才打开连接）。
func NewDBService(cfg DBServiceConfig) (*DBService, error) {
	if cfg.Factory == nil {
		return nil, errors.New("kernel/db: factory is nil")
	}
	return &DBService{engine: cfg.Engine}, nil
}

// Name 实现 Service 接口
func (s *DBService) Name() string { return "db" }

// Init 实现 Service 接口（打开数据库连接）
func (s *DBService) Init(ctx ServiceContext) error {
	return nil
}

// Health 实现 Service 接口
func (s *DBService) Health(ctx ServiceContext) error {
	if s.store == nil {
		return errors.New("kernel/db: not initialized")
	}
	return nil
}

// Call 实现 Service 接口（JSON 方法调度）
func (s *DBService) Call(ctx ServiceContext, method string, payload json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "engine":
		return json.Marshal(map[string]string{"engine": s.engine})
	case "stats":
		return s.callStats(ctx, payload)
	case "list_tasks":
		return s.callListTasks(ctx, payload)
	case "get_task":
		return s.callGetTask(ctx, payload)
	case "create_task":
		return s.callCreateTask(ctx, payload)
	case "update_task":
		return s.callUpdateTask(ctx, payload)
	case "delete_task":
		return s.callDeleteTask(ctx, payload)
	default:
		return nil, fmt.Errorf("%w: db.%s", ErrMethodNotFound, method)
	}
}

// Store 返回底层 tasksystem.Store（供内部直接使用，不走 JSON 序列化）。
func (s *DBService) Store() tasksystem.Store {
	return s.store
}

// SetStore 设置底层 Store（用于延迟初始化或热切换引擎）。
func (s *DBService) SetStore(store tasksystem.Store) {
	s.store = store
}

func (s *DBService) callStats(ctx ServiceContext, payload json.RawMessage) (json.RawMessage, error) {
	stats := map[string]any{
		"engine": s.engine,
		"ready":  s.store != nil,
	}
	return json.Marshal(stats)
}

func (s *DBService) callListTasks(ctx ServiceContext, payload json.RawMessage) (json.RawMessage, error) {
	if s.store == nil {
		return nil, errors.New("kernel/db: store not initialized")
	}
	var filter tasksystem.TaskFilter
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &filter); err != nil {
			return nil, fmt.Errorf("kernel/db: list_tasks unmarshal: %w", err)
		}
	}
	tasks, err := s.store.ListTasks(filter)
	if err != nil {
		return nil, err
	}
	return json.Marshal(tasks)
}

func (s *DBService) callGetTask(ctx ServiceContext, payload json.RawMessage) (json.RawMessage, error) {
	if s.store == nil {
		return nil, errors.New("kernel/db: store not initialized")
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("kernel/db: get_task unmarshal: %w", err)
	}
	task, err := s.store.GetTask(req.ID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(task)
}

func (s *DBService) callCreateTask(ctx ServiceContext, payload json.RawMessage) (json.RawMessage, error) {
	if s.store == nil {
		return nil, errors.New("kernel/db: store not initialized")
	}
	var task tasksystem.TaskData
	if err := json.Unmarshal(payload, &task); err != nil {
		return nil, fmt.Errorf("kernel/db: create_task unmarshal: %w", err)
	}
	if err := s.store.CreateTask(task); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": task.ID})
}

func (s *DBService) callUpdateTask(ctx ServiceContext, payload json.RawMessage) (json.RawMessage, error) {
	if s.store == nil {
		return nil, errors.New("kernel/db: store not initialized")
	}
	var task tasksystem.TaskData
	if err := json.Unmarshal(payload, &task); err != nil {
		return nil, fmt.Errorf("kernel/db: update_task unmarshal: %w", err)
	}
	if err := s.store.UpdateTask(task); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

func (s *DBService) callDeleteTask(ctx ServiceContext, payload json.RawMessage) (json.RawMessage, error) {
	if s.store == nil {
		return nil, errors.New("kernel/db: store not initialized")
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("kernel/db: delete_task unmarshal: %w", err)
	}
	if err := s.store.DeleteTask(req.ID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}
