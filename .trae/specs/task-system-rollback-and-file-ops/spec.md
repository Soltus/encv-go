# 任务系统回滚特性 + 文件系统联动升级 Spec

## Why

当前任务系统存在以下架构缺陷，阻碍回滚特性落地与多前端复用：

1. **后端无数据库**：用 `.encv-tasks.json` 文件持久化，无事务/索引/历史快照能力。多前端共享时无隔离，并发写入靠 map mutex，无法支撑回滚所需的事务性快照存储。
2. **前端重度依赖 IndexedDB**：Dexie `encv_task_store_v6` 承担离线缓存+秒开+高频 progress 缓冲三大职责，与"后端数据库权威"原则相悖。一个后端服务多个前端时，IndexedDB 是 per-client 的，无法作为共享真相源。
3. **4 个文件操作未接入任务系统**：move/copy/rename/delete 是同步 HTTP，无进度/取消/回滚能力，大文件操作阻塞 HTTP 请求。
4. **无回收站**：删除是 `os.Remove` 硬删除，无法回滚。AI agent 工具有半成品 trash 模式但 mobile 前端无法访问，且无还原/列出/清空 API。
5. **无回滚机制**：只有对称操作（encrypt↔decrypt），不算回滚。回滚需要"撤销已完成的副作用"，边界状态复杂（源文件被覆盖/删除/移动）。
6. **文件操作不广播 file:change**：前端靠 `loadFiles()` 全量重载，与 v6 增量更新机制脱节。
7. **任务筛选不完整**：不支持按 triggeredBy 筛选，无法区分 user/automation/ai_agent 任务。
8. **架构耦合在应用层**：TaskManager 直接写在 `internal/service/`，无法被其他应用复用。

## What Changes

### 后端：引入 SQLite 数据库（glebarez/sqlite，pure-Go）

- **新增** `pkg/tasksystem/` 可复用包，定义 `Task` / `RollbackManager` / `TrashManager` / `Store` 接口
- **新增** `pkg/tasksystem/store/sqlite/` SQLite 实现（使用 `github.com/glebarez/sqlite`，符合 android.md §五 gomobile+sqlite 铁律）
- **新增** 3 张表：`tasks` / `trash` / `rollback_snapshots`
- **迁移** `.encv-tasks.json` → SQLite（启动时一次性迁移，迁移后 JSON 文件保留为只读备份）
- **BREAKING** `GET /api/tasks` 返回结构不变，但底层存储切换为 SQLite

### 后端：文件操作接入任务系统

- **新增** 4 种 TaskType：`move` / `copy` / `rename` / `delete`
- **修改** `processTask` switch 加 4 个新 case
- **修改** 4 个文件操作 handler：改为创建 Task 后异步执行，立即返回 taskID
- **修改** 文件操作完成后广播 `file:change` 事件（create/delete/modify）
- **BREAKING** `POST /api/file/move` / `POST /api/file/copy` / `POST /api/file/rename` / `DELETE /api/files` 返回值从直接结果改为 `{ taskId: string }`，前端需轮询或订阅 WS 事件获取结果

### 后端：回收站功能

- **新增** `<servingDir>/.trash/` 目录（与 AI agent 工具一致）
- **新增** trash 表记录 original_path / trash_path / is_directory / size / deleted_at / task_id
- **新增** 4 个 API：`GET /api/trash` / `POST /api/trash/restore` / `DELETE /api/trash/:id` / `DELETE /api/trash`
- **修改** delete 任务：改为移到 `.trash/` + 记录 trash 表，而非 `os.Remove`

### 后端：回滚机制

- **新增** `RollbackManager` 接口 + 实现
- **新增** `POST /api/tasks/:id/rollback` API
- **新增** rollback_snapshots 表存储任务执行前后的状态快照
- **新增** 6 种操作的回滚策略（encrypt/decrypt/move/copy/rename/delete）

### 前端：IndexedDB 降级为纯缓存

- **修改** `taskPersistence.ts`：移除 `pruneTerminalTasks`（后端权威），保留 LRU 缓存（最近 200 条）
- **修改** `taskStore.ts`：progress 不再写 IndexedDB（只读后端 WS），status 变更仍写缓存
- **修改** `hydrate`：先从 IndexedDB 秒开，再从后端 fetch 覆盖（已有逻辑，不变）

### 前端：文件操作 UI 适配任务系统

- **修改** `Files.vue`：4 个文件操作改为创建 Task + 订阅 WS 事件，移除 `await loadFiles()` 全量重载
- **修改** `Files.vue`：文件操作进度通过 task:progress 事件驱动，操作完成后 file:change 增量更新
- **新增** 文件操作任务卡片（TaskType=move/copy/rename/delete 的图标/颜色/标签）

### 前端：任务筛选完善

- **新增** `filterTriggeredBy` 筛选维度（user/automation/ai_agent）
- **新增** Tasks.vue triggeredBy ion-chip + popover
- **新增** `filterTypes` 扩展支持 move/copy/rename/delete

### 前端：回滚 UI

- **新增** TaskDetailModal 加"回滚"按钮（仅终态任务且 CanRollback=true 时显示）
- **新增** 回滚确认对话框（提示边界状态风险）
- **新增** 回滚任务卡片（rollback_of 字段关联原任务）

### 测试：开发者选项自动化测试增加文件系统任务测试

- **新增** `useFileSystemTests` composable：文件系统任务测试用例（move/copy/rename/delete + rollback 各边界）
- **修改** 开发者选项页加"文件系统任务测试"入口

## Impact

### 受影响代码

**后端（新增）**：
- `pkg/tasksystem/` — 可复用任务系统包（Task/Rollback/Trash 接口）
- `pkg/tasksystem/store/sqlite/` — SQLite 存储实现
- `internal/service/rollback_manager.go` — 回滚管理器实现
- `internal/service/trash_manager.go` — 回收站管理器实现
- `internal/service/file_task_handler.go` — 文件操作任务处理器

**后端（修改）**：
- `internal/service/task_manager.go` — 改用 SQLite Store，加 4 种 TaskType，加回滚快照
- `internal/service/mobile_service.go` — DeleteFile 改为移到回收站
- `internal/server/mobile_api.go` — 文件操作 handler 改为创建 Task
- `internal/server/admin_handlers.go` — move/copy/rename handler 改为创建 Task
- `internal/server/server.go` — 新增 trash / rollback 路由
- `internal/tools/edit_metadata.go` — AI agent delete_file 工具统一走 TrashManager

**前端（新增）**：
- `app/encv-mobile/src/composables/useFileSystemTests.ts` — 文件系统任务测试
- `app/encv-mobile/src/components/RollbackConfirmDialog.vue` — 回滚确认对话框

**前端（修改）**：
- `app/encv-mobile/src/api/encv.ts` — EncvTask 加 rollbackOf/originalPath 字段，新增 rollbackTask/listTrash/restoreTrash API
- `app/encv-mobile/src/lib/taskPersistence.ts` — 降级为纯缓存，移除 pruneTerminalTasks
- `app/encv-mobile/src/stores/taskStore.ts` — progress 不写 IndexedDB
- `app/encv-mobile/src/composables/useTaskFilter.ts` — 加 filterTriggeredBy
- `app/encv-mobile/src/composables/useTasksList.ts` — filteredTasks 加 triggeredBy 过滤
- `app/encv-mobile/src/views/Tasks.vue` — 加 triggeredBy chip + 回滚按钮
- `app/encv-mobile/src/views/Files.vue` — 4 个文件操作改为创建 Task
- `app/encv-mobile/src/components/TaskDetailModal.vue` — 加回滚按钮
- `app/encv-mobile/src/i18n/tasks.ts` — 加 move/copy/rename/delete/rollback/trash 翻译

### 受影响 specs
- `unify-workflow-task-service` — Phase 枚举化对齐
- `automation-workflow` 规则 — 4 件套订阅、动态工作流构建规范
- `android.md` 规则 — gomobile+sqlite 选型铁律（glebarez/sqlite）

## ADDED Requirements

### Requirement: pkg/tasksystem/ 可复用任务系统包

系统 SHALL 提供 `pkg/tasksystem/` 包，定义任务系统的核心接口，可被 encv 应用和其他 Go 应用复用。

```go
package tasksystem

// TaskType 任务类型枚举
type TaskType string
const (
    TaskTypeEncrypt TaskType = "encrypt"
    TaskTypeDecrypt TaskType = "decrypt"
    TaskTypeMove    TaskType = "move"
    TaskTypeCopy    TaskType = "copy"
    TaskTypeRename  TaskType = "rename"
    TaskTypeDelete  TaskType = "delete"
)

// TaskStatus 任务状态枚举
type TaskStatus string
const (
    StatusQueued     TaskStatus = "queued"
    StatusRunning    TaskStatus = "running"
    StatusCancelling TaskStatus = "cancelling"
    StatusCompleted  TaskStatus = "completed"
    StatusFailed     TaskStatus = "failed"
    StatusCancelled  TaskStatus = "cancelled"
)

// Task 任务核心接口
type Task interface {
    ID() string
    Type() TaskType
    Status() TaskStatus
    SourcePath() string
    TargetPath() string
    OutputPath() string
    TriggeredBy() string
    RunID() string
    Progress() int
    Phase() string
    Error() string
    CreatedAt() time.Time
    CompletedAt() *time.Time
    RollbackOf() string  // 回滚任务指向原任务 ID
    OriginalPath() string  // 原始路径（回滚用）
}

// Store 存储接口（可插拔，SQLite/PostgreSQL/内存均可实现）
type Store interface {
    CreateTask(task Task) error
    GetTask(id string) (Task, error)
    ListTasks(filter TaskFilter) ([]Task, error)
    UpdateTask(task Task) error
    DeleteTask(id string) error
    SaveSnapshot(taskID string, snapshot Snapshot) error
    GetSnapshot(taskID string) (Snapshot, error)
}

// RollbackManager 回滚管理器接口
type RollbackManager interface {
    CanRollback(taskID string) bool
    Rollback(taskID string) (newTaskID string, err error)
    GetRollbackStrategy(taskType TaskType) (RollbackStrategy, error)
}

// RollbackStrategy 回滚策略接口（每种 TaskType 一个实现）
type RollbackStrategy interface {
    PrepareRollback(task Task, snapshot Snapshot) (rollbackTask Task, err error)
    ExecuteRollback(task Task) error
    ValidateBoundary(task Task) error
}

// TrashManager 回收站管理器接口
type TrashManager interface {
    MoveToTrash(path string) (trashItem TrashItem, err error)
    Restore(trashID string, destPath string) error
    List() ([]TrashItem, error)
    Purge(trashID string) error
    Empty() error
}

// TrashItem 回收站条目
type TrashItem struct {
    ID           string
    OriginalPath string
    TrashPath    string
    IsDirectory  bool
    Size         int64
    DeletedAt    time.Time
    TaskID       string
}

// Snapshot 任务执行前后的状态快照
type Snapshot struct {
    TaskID      string
    Type        string  // 'pre_state' | 'post_state'
    Data        []byte  // JSON 编码的状态数据
    CreatedAt   time.Time
}

// TaskFilter 任务查询过滤
type TaskFilter struct {
    Types        []TaskType
    Statuses     []TaskStatus
    TriggeredBy  []string
    RunID        string
    Limit        int
    Offset       int
}
```

#### Scenario: encv 应用使用 pkg/tasksystem/
- **WHEN** encv 应用初始化任务系统
- **THEN** 调用 `tasksystem.NewSQLiteStore(dbPath)` 创建 SQLite 存储
- **AND** 调用 `tasksystem.NewRollbackManager(store)` 创建回滚管理器
- **AND** 调用 `tasksystem.NewTrashManager(trashDir, store)` 创建回收站管理器
- **AND** encv 的 TaskManager 实现 `tasksystem.Task` 接口适配

#### Scenario: 其他 Go 应用复用 pkg/tasksystem/
- **WHEN** 其他应用需要任务系统
- **THEN** `import "github.com/.../pkg/tasksystem"` 引入包
- **AND** 实现 `Store` 接口（或直接用 `store/sqlite` 子包）
- **AND** 注册自定义 `RollbackStrategy`（如有特殊操作类型）
- **AND** 不依赖 encv 的 `internal/service/` 任何代码

### Requirement: SQLite 数据库存储（modernc.org/sqlite）

系统 SHALL 使用 `modernc.org/sqlite`（pure-Go，CGO_ENABLED=0，不依赖 gorm）作为后端权威数据库，符合 android.md §五 gomobile+sqlite 铁律（禁止 mattn/go-sqlite3 CGO 驱动）。

#### Scenario: 数据库初始化
- **WHEN** 后端启动
- **THEN** 在 `<servingDir>/.encv-tasks.db` 创建 SQLite 数据库
- **AND** 启用 WAL 模式（`PRAGMA journal_mode=WAL`）提升并发读写性能
- **AND** 创建 `tasks` / `trash` / `rollback_snapshots` 三张表
- **AND** 创建必要索引（status / run_id / triggered_by / created_at / rollback_of / deleted_at）

#### Scenario: 从 JSON 迁移到 SQLite
- **WHEN** 后端启动且检测到 `.encv-tasks.json` 存在且 `.encv-tasks.db` 不存在
- **THEN** 读取 JSON 文件，逐条插入 SQLite
- **AND** 迁移完成后将 JSON 文件重命名为 `.encv-tasks.json.migrated`（保留为备份）
- **AND** 迁移失败时回滚 SQLite 事务，保留 JSON 文件，启动失败并报错

#### Scenario: tasks 表 schema
```sql
CREATE TABLE tasks (
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
    container_version INTEGER,
    cipher_mode INTEGER,
    compression_mode TEXT,
    extra_fields TEXT,  -- JSON
    steps TEXT,  -- JSON
    mount_id TEXT,
    mount_sub_path TEXT,
    target_mount_id TEXT,
    target_mount_sub_path TEXT,
    created_at DATETIME NOT NULL,
    completed_at DATETIME,
    rollback_of TEXT,  -- 回滚任务指向原任务 ID
    original_path TEXT,  -- 原始路径（回滚用）
    password TEXT,  -- 加密任务密码（向后兼容）
    secondary_password TEXT
);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_run_id ON tasks(run_id);
CREATE INDEX idx_tasks_triggered_by ON tasks(triggered_by);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
CREATE INDEX idx_tasks_rollback_of ON tasks(rollback_of);
```

#### Scenario: trash 表 schema
```sql
CREATE TABLE trash (
    id TEXT PRIMARY KEY,
    original_path TEXT NOT NULL,
    trash_path TEXT NOT NULL,
    is_directory INTEGER DEFAULT 0,
    size INTEGER DEFAULT 0,
    deleted_at DATETIME NOT NULL,
    task_id TEXT,
    restore_task_id TEXT,
    metadata TEXT  -- JSON，存文件原属性（mtime/mode 等）
);
CREATE INDEX idx_trash_deleted_at ON trash(deleted_at);
CREATE INDEX idx_trash_original_path ON trash(original_path);
```

#### Scenario: rollback_snapshots 表 schema
```sql
CREATE TABLE rollback_snapshots (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    snapshot_type TEXT NOT NULL,  -- 'pre_state' | 'post_state'
    snapshot_data TEXT NOT NULL,  -- JSON
    created_at DATETIME NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX idx_snapshots_task_id ON rollback_snapshots(task_id);
```

### Requirement: 文件操作接入任务系统

系统 SHALL 将 move/copy/rename/delete 4 种文件操作接入任务系统，统一走 Task 队列，有进度/取消/回滚能力。

#### Scenario: 文件移动（move）作为任务
- **WHEN** 用户调用 `POST /api/file/move` body `{ srcPath, destPath }`
- **THEN** 后端创建 Task(type=move, sourcePath=srcPath, targetPath=destPath, triggeredBy=user)
- **AND** 立即返回 `{ taskId: "<id>" }`（202 Accepted）
- **AND** 异步执行：优先 `os.Rename`（同设备原子），失败 fallback 到 `io.Copy` + `os.Remove`（跨设备）
- **AND** 执行前保存 pre_state 快照（原文件路径、大小、mtime）
- **AND** 执行完成后广播 `file:change` 事件（action=delete for srcPath, action=create for destPath）
- **AND** 任务状态推送 task:progress / task:completed

#### Scenario: 文件复制（copy）作为任务
- **WHEN** 用户调用 `POST /api/file/copy` body `{ srcPath, destPath }`
- **THEN** 后端创建 Task(type=copy, sourcePath=srcPath, targetPath=destPath)
- **AND** 立即返回 `{ taskId: "<id>" }`
- **AND** 异步执行 `io.Copy`，进度按字节推送
- **AND** 执行完成后广播 `file:change` 事件（action=create for destPath，srcPath 不变）

#### Scenario: 文件重命名（rename）作为任务
- **WHEN** 用户调用 `POST /api/file/rename` body `{ oldPath, newName }`
- **THEN** 后端创建 Task(type=rename, sourcePath=oldPath, targetPath=newPath, originalPath=oldPath)
- **AND** 立即返回 `{ taskId: "<id>" }`
- **AND** 异步执行：普通文件 `os.Rename`；加密容器走 `MobileService.RenameFile`（元数据重命名）
- **AND** 执行前保存 pre_state 快照（原文件名）
- **AND** 执行完成后广播 `file:change` 事件（action=delete for oldPath, action=create for newPath）

#### Scenario: 文件删除（delete）作为任务
- **WHEN** 用户调用 `DELETE /api/files?path=<encoded>`
- **THEN** 后端创建 Task(type=delete, sourcePath=path, originalPath=path)
- **AND** 立即返回 `{ taskId: "<id>" }`
- **AND** 异步执行：调用 `TrashManager.MoveToTrash(path)` 移到 `<servingDir>/.trash/<timestamp>_<name>`
- **AND** trash 表记录 original_path / trash_path / is_directory / size / task_id
- **AND** 执行完成后广播 `file:change` 事件（action=delete for path）

#### Scenario: 文件操作进度推送
- **WHEN** move/copy 任务执行中
- **THEN** 按已传输字节 / 总字节比例推送 task:progress 事件
- **AND** phase 为 `moving` / `copying`
- **AND** rename/delete 任务因执行快，progress 直接 0→100

### Requirement: 回收站功能

系统 SHALL 提供回收站功能，删除操作改为移到回收站而非永久删除。

#### Scenario: 移到回收站
- **WHEN** delete 任务执行
- **THEN** 文件/目录移到 `<servingDir>/.trash/<timestamp>_<original_name>`
- **AND** trash 表记录 original_path / trash_path / is_directory / size / deleted_at / task_id
- **AND** 保留文件原属性（mtime/mode）到 metadata JSON 字段

#### Scenario: 列出回收站
- **WHEN** 用户调用 `GET /api/trash`
- **THEN** 返回 `{ items: TrashItem[] }`，按 deleted_at 倒序
- **AND** 每个 item 含 id / originalPath / trashPath / isDirectory / size / deletedAt

#### Scenario: 从回收站还原
- **WHEN** 用户调用 `POST /api/trash/restore` body `{ trashId, destPath? }`
- **THEN** 创建 Task(type=restore, sourcePath=trashPath, targetPath=destPath||originalPath, triggeredBy=user)
- **AND** 异步执行：`os.Rename(trashPath, destPath)` 移回原位
- **AND** trash 表记录 restore_task_id
- **AND** 执行完成后广播 `file:change` 事件（action=create for destPath）
- **AND** 从 trash 表删除该条目

#### Scenario: 永久删除回收站条目
- **WHEN** 用户调用 `DELETE /api/trash/:id`
- **THEN** `os.Remove(trashPath)` 永久删除文件
- **AND** 从 trash 表删除该条目
- **AND** 不广播 file:change（回收站内部操作）

#### Scenario: 清空回收站
- **WHEN** 用户调用 `DELETE /api/trash`
- **THEN** 遍历 trash 表所有条目，逐个 `os.Remove(trashPath)`
- **AND** 清空 trash 表
- **AND** 不广播 file:change

#### Scenario: 回收站自动清理（可选）
- **WHEN** 后端启动且回收站有条目超过 30 天
- **THEN** 自动永久删除过期条目（可配置，默认关闭）
- **AND** 记录日志

### Requirement: 回滚机制

系统 SHALL 提供回滚机制，支持 6 种操作（encrypt/decrypt/move/copy/rename/delete）的回滚，完备处理边界状态。

#### Scenario: 加密任务回滚
- **WHEN** 用户对 encrypt 任务调用 `POST /api/tasks/:id/rollback`
- **AND** 任务状态为 completed
- **THEN** 创建 Task(type=rollback_encrypt, rollbackOf=原任务ID, triggeredBy=user)
- **AND** 执行：删除 outputPath（加密产物）
- **AND** 不删除 sourcePath（原文件保留）
- **AND** 边界处理：outputPath 不存在=视为已回滚，标记 completed；outputPath 被占用=报错 failed

#### Scenario: 解密任务回滚
- **WHEN** 用户对 decrypt 任务调用回滚
- **THEN** 创建 Task(type=rollback_decrypt, rollbackOf=原任务ID)
- **AND** 执行：删除 outputPath（解密产物）
- **AND** 边界处理同加密回滚

#### Scenario: 移动任务回滚
- **WHEN** 用户对 move 任务调用回滚
- **THEN** 创建 Task(type=rollback_move, rollbackOf=原任务ID, sourcePath=targetPath, targetPath=originalPath)
- **AND** 执行：`os.Rename(targetPath, originalPath)` 移回原位
- **AND** 边界处理：
    - targetPath 不存在=报错 failed（无法回滚已丢失的文件）
    - originalPath 已被占用=报错 failed（需用户手动处理）
    - 跨设备移动= fallback 到 io.Copy + os.Remove

#### Scenario: 复制任务回滚
- **WHEN** 用户对 copy 任务调用回滚
- **THEN** 创建 Task(type=rollback_copy, rollbackOf=原任务ID, sourcePath=targetPath)
- **AND** 执行：`os.Remove(targetPath)` 删除副本
- **AND** 边界处理：targetPath 不存在=视为已回滚，标记 completed

#### Scenario: 重命名任务回滚
- **WHEN** 用户对 rename 任务调用回滚
- **THEN** 创建 Task(type=rollback_rename, rollbackOf=原任务ID, sourcePath=targetPath, targetPath=originalPath)
- **AND** 执行：`os.Rename(targetPath, originalPath)` 改回原名
- **AND** 边界处理：
    - targetPath 不存在=报错 failed
    - originalPath 已被占用=报错 failed
    - 加密容器元数据重命名=走 `MobileService.RenameFile` 反向操作

#### Scenario: 删除任务回滚（从回收站还原）
- **WHEN** 用户对 delete 任务调用回滚
- **THEN** 查找 trash 表中 task_id = 原任务ID 的条目
- **AND** 创建 Task(type=rollback_delete, rollbackOf=原任务ID, sourcePath=trashPath, targetPath=originalPath)
- **AND** 执行：`os.Rename(trashPath, originalPath)` 从回收站还原
- **AND** 边界处理：
    - trash 条目不存在（回收站已清）=报错 failed
    - originalPath 已被占用=报错 failed
    - 还原成功后从 trash 表删除条目

#### Scenario: 回滚前置校验
- **WHEN** 用户调用回滚 API
- **THEN** `RollbackManager.CanRollback(taskID)` 校验：
    - 任务状态必须为 completed（failed/cancelled/running 不可回滚）
    - 任务未被回滚过（rollback_of 字段为空，且无其他任务 rollback_of 指向它）
    - 回滚所需的快照/资源存在（如 trash 条目、原路径可访问）
- **AND** 校验失败返回 400 Bad Request + 错误原因

#### Scenario: 回滚任务本身不可再回滚
- **WHEN** 用户对 rollback_* 类型任务调用回滚
- **THEN** 拒绝（400 Bad Request），回滚任务不支持二次回滚

### Requirement: 任务筛选按 triggeredBy

系统 SHALL 支持按 triggeredBy 维度筛选任务。

#### Scenario: 前端筛选 UI
- **WHEN** 用户在 Tasks 页面点击 triggeredBy chip
- **THEN** 弹出 popover 显示 3 个 checkbox：user / automation / ai_agent
- **AND** 多选，选中后立即应用筛选
- **AND** chip 上显示选中数量（如"触发者 2"）

#### Scenario: 筛选逻辑
- **WHEN** filterTriggeredBy = ['automation', 'ai_agent']
- **THEN** filteredTasks 只显示 triggeredBy in ['automation', 'ai_agent'] 的任务
- **AND** filterTriggeredBy 为空数组时不筛选（显示全部）

### Requirement: 前端 IndexedDB 降级为纯缓存

系统 SHALL 将前端 IndexedDB 降级为纯缓存，后端 SQLite 为权威数据源。

#### Scenario: progress 不写 IndexedDB
- **WHEN** task:progress 事件到达前端
- **THEN** 更新内存 store 的 task.progress
- **AND** 不调用 `putTaskThrottled`（progress 是高频 transient 状态，后端 SQLite 已持久化）
- **AND** status 变更仍写 IndexedDB 缓存（用于离线秒开）

#### Scenario: IndexedDB LRU 缓存
- **WHEN** IndexedDB 任务数超过 200
- **THEN** 按 createdAt 倒序保留最新 200 条，删除多余的
- **AND** 不区分终态/非终态（后端权威，前端只缓存最近访问的）

#### Scenario: hydrate 优先 IndexedDB 秒开
- **WHEN** 应用冷启动
- **THEN** 先从 IndexedDB 加载缓存任务（秒开 UI）
- **AND** 异步从后端 `GET /api/tasks` 拉取最新，bulkSetTasks 覆盖
- **AND** 后端数据覆盖 IndexedDB（后端权威）

### Requirement: 文件操作 UI 适配任务系统

系统 SHALL 将前端 4 个文件操作改为创建 Task + 订阅 WS 事件，移除 `await loadFiles()` 全量重载。

#### Scenario: 文件操作创建 Task
- **WHEN** 用户在 Files.vue 长按文件选择"移动/复制/重命名/删除"
- **THEN** 调用对应 API（moveFile/copyFile/renameFile/deleteFile）
- **AND** API 返回 `{ taskId }`，前端显示 toast "任务已创建"
- **AND** 不调用 `await loadFiles()`（依赖 file:change 增量更新）

#### Scenario: 文件操作进度反馈
- **WHEN** move/copy 大文件任务执行中
- **THEN** 前端通过 task:progress 事件更新进度
- **AND** 可在 Tasks 页面看到任务卡片进度条
- **AND** 用户可点击"取消"取消任务

#### Scenario: 文件操作完成增量更新
- **WHEN** 文件操作任务完成，后端广播 file:change
- **THEN** Files.vue 的 onFileChange 增量更新 files.value 数组
- **AND** delete → splice 移除；create → getFileInfo + push；modify → getFileInfo + 替换
- **AND** 不全量 reload

### Requirement: 回滚 UI

系统 SHALL 在任务详情弹窗提供回滚按钮，并提示边界状态风险。

#### Scenario: 回滚按钮显示条件
- **WHEN** 用户打开任务详情弹窗
- **AND** 任务状态为 completed
- **AND** 任务类型为 encrypt/decrypt/move/copy/rename/delete（非 rollback_*）
- **AND** 任务未被回滚过
- **THEN** 显示"回滚"按钮
- **AND** rollback_* 类型任务不显示回滚按钮

#### Scenario: 回滚确认对话框
- **WHEN** 用户点击"回滚"按钮
- **THEN** 弹出确认对话框，提示：
    - 回滚操作内容（如"删除输出文件 sample.encv"）
    - 边界风险（如"如果输出文件已被移动/删除，回滚将失败"）
    - "确认回滚" / "取消"按钮
- **AND** 用户确认后调用 `POST /api/tasks/:id/rollback`

#### Scenario: 回滚任务卡片
- **WHEN** 回滚任务创建
- **THEN** 任务卡片显示"回滚"标签 + 关联的原任务 ID（后 8 位）
- **AND** 卡片图标为 undo 图标
- **AND** 回滚任务状态推送与其他任务一致（task:created/update/progress/completed）

### Requirement: 开发者选项自动化测试增加文件系统任务测试

系统 SHALL 在开发者选项中增加文件系统任务测试，覆盖 move/copy/rename/delete + rollback 各边界状态。

#### Scenario: 文件系统任务测试入口
- **WHEN** 用户在开发者选项页面点击"文件系统任务测试"
- **THEN** 调用 `useFileSystemTests` composable 运行测试
- **AND** 测试结果以测试报告树展示（复用 UnifiedTreeView）

#### Scenario: 测试用例覆盖
- **WHEN** 文件系统任务测试运行
- **THEN** 执行以下测试用例：
    1. 创建临时文件 → move → 验证原位无文件、新位有文件 → rollback → 验证原位有文件、新位无文件
    2. 创建临时文件 → copy → 验证原位和新位都有文件 → rollback → 验证新位文件已删
    3. 创建临时文件 → rename → 验证旧名无文件、新名有文件 → rollback → 验证旧名有文件、新名无文件
    4. 创建临时文件 → delete → 验证原位无文件 → rollback → 验证原位有文件（从回收站还原）
    5. 边界：move 到已存在目标 → 期望 failed
    6. 边界：rollback 已被回滚的任务 → 期望 400
    7. 边界：rollback 已被删除的文件（trash 已清）→ 期望 failed
    8. 边界：rollback 时原位已被占用 → 期望 failed
- **AND** 每个用例断言任务状态 + 文件系统实际状态

#### Scenario: 测试隔离
- **WHEN** 文件系统任务测试运行
- **THEN** 在 `<servingDir>/.encv-test-fs/` 临时目录执行
- **AND** 测试完成后清理临时目录
- **AND** 不影响用户真实文件

## MODIFIED Requirements

### Requirement: TaskManager 持久化层

`TaskManager` SHALL 改用 `tasksystem.Store`（SQLite 实现）作为持久化层，移除 `.encv-tasks.json` JSON 文件直接写入。

#### Scenario: TaskManager 初始化
- **WHEN** `NewTaskManager` 构造
- **THEN** 接收 `store tasksystem.Store` 参数（依赖注入）
- **AND** 不再接收 `persistPath string` 参数
- **AND** 所有 `saveTasks()` / `loadTasks()` 调用改为 `store.CreateTask` / `store.ListTasks`

#### Scenario: 任务状态变更持久化
- **WHEN** 任务状态变更（created/running/completed/failed/cancelled）
- **THEN** 调用 `store.UpdateTask(task)` 持久化到 SQLite
- **AND** 不再写 JSON 文件

### Requirement: GET /api/tasks 返回结构

`GET /api/tasks` SHALL 返回与当前一致的结构，但底层从 SQLite 查询。

#### Scenario: 查询全部任务
- **WHEN** `GET /api/tasks` 无 query 参数
- **THEN** 返回 `{ tasks: EncvTask[] }`，按 createdAt 倒序
- **AND** 底层 `store.ListTasks(TaskFilter{Limit: 1000})` 查询 SQLite

#### Scenario: 查询支持分页（新增）
- **WHEN** `GET /api/tasks?limit=50&offset=100`
- **THEN** 返回 `{ tasks: EncvTask[], total: number }`
- **AND** 底层 `store.ListTasks(TaskFilter{Limit: 50, Offset: 100})`

### Requirement: TaskType 枚举扩展

`TaskType` SHALL 从 `encrypt | decrypt` 扩展为 `encrypt | decrypt | move | copy | rename | delete | rollback_*`。

#### Scenario: 前端 TaskType union
- **WHEN** 前端定义 TaskType
- **THEN** `type TaskType = 'encrypt' | 'decrypt' | 'move' | 'copy' | 'rename' | 'delete' | 'rollback_encrypt' | 'rollback_decrypt' | 'rollback_move' | 'rollback_copy' | 'rollback_rename' | 'rollback_delete'`

#### Scenario: 任务图标/颜色/标签映射
- **WHEN** UI 渲染任务卡片
- **THEN** move → move icon + 蓝色
- **AND** copy → copy icon + 蓝色
- **AND** rename → text icon + 蓝色
- **AND** delete → trash icon + 红色
- **AND** rollback_* → undo icon + 紫色

### Requirement: 文件操作 handler 返回值

4 个文件操作 handler SHALL 返回 `{ taskId: string }` 而非直接结果。

#### Scenario: 返回值变更
- **WHEN** `POST /api/file/move` / `POST /api/file/copy` / `POST /api/file/rename` / `DELETE /api/files`
- **THEN** 返回 `202 Accepted` + `{ taskId: "<id>" }`
- **AND** 不再返回直接操作结果
- **AND** 前端通过 task:completed 事件获取结果

### Requirement: AI agent delete_file 工具统一走 TrashManager

`internal/tools/edit_metadata.go` 的 `delete_file` 工具 SHALL 统一走 `TrashManager`，移除硬编码 `.trash` 逻辑。

#### Scenario: AI agent trash 模式
- **WHEN** AI agent 调用 `delete_file` with `mode: "trash"`
- **THEN** 调用 `TrashManager.MoveToTrash(path)`
- **AND** 不再硬编码 `filepath.Join(rootAbs, ".trash")`
- **AND** trash 表记录条目，可通过 `GET /api/trash` 列出

## REMOVED Requirements

### Requirement: .encv-tasks.json JSON 持久化
**Reason**: 引入 SQLite 数据库作为权威存储，JSON 文件无事务/索引/并发能力
**Migration**: 启动时一次性迁移 `.encv-tasks.json` → SQLite，迁移后 JSON 重命名为 `.encv-tasks.json.migrated`

### Requirement: 前端 IndexedDB 作为任务权威存储
**Reason**: 一个后端服务多个前端时，IndexedDB 是 per-client 的，无法作为共享真相源；回滚特性需要后端数据库权威
**Migration**: IndexedDB 降级为纯缓存（LRU 200 条），后端 SQLite 为权威；hydrate 时 IndexedDB 秒开后端覆盖

### Requirement: 文件操作同步 HTTP 直接返回结果
**Reason**: 大文件操作阻塞 HTTP 请求，无进度/取消/回滚能力
**Migration**: 4 个文件操作改为创建 Task 异步执行，返回 `{ taskId }`，前端订阅 WS 事件

### Requirement: 文件操作不广播 file:change
**Reason**: 前端靠 `loadFiles()` 全量重载，与 v6 增量更新机制脱节
**Migration**: 4 个文件操作完成后广播 file:change 事件，前端增量更新

### Requirement: AI agent delete_file 工具硬编码 .trash 目录
**Reason**: 与 mobile 前端回收站功能重复实现，且描述与实现不一致（描述说读 mount.trash_path，实际硬编码）
**Migration**: 统一走 `TrashManager`，trash 表记录所有回收站条目
