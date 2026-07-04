# Checklist

## Phase 1: 后端基础设施
- [x] pkg/tasksystem/ 包定义 Task/Store/RollbackManager/TrashManager/RollbackStrategy 接口
- [x] pkg/tasksystem/store/sqlite/ 实现 SQLite Store（modernc.org/sqlite, pure-Go, CGO_ENABLED=0）
- [x] tasks/trash/rollback_snapshots 三张表 + 索引创建
- [x] WAL 模式启用
- [x] JSON → SQLite 迁移逻辑（迁移后 JSON 重命名 .migrated，失败回滚）

## Phase 2: 文件操作接入任务系统
- [x] TaskManager 改用 tasksystem.Store（双写策略：内存 map + SQLite Store，store=nil 走旧 JSON）
- [x] processTask switch 加 move/copy/rename/delete 4 个 case
- [x] file_task_handler.go 实现 4 种处理器（执行前保存 pre_state 快照）
- [x] 4 种文件操作完成后广播 file:change（create/delete/modify）
- [x] 4 个 handler 返回 { taskId } (202 Accepted)

## Phase 3: 回收站 + 回滚
- [x] TrashManager 实现 MoveToTrash/Restore/List/Purge/Empty
- [x] trash 表 CRUD
- [x] RollbackManager 实现 CanRollback/Rollback
- [x] 6 种 RollbackStrategy 实现（encrypt/decrypt/move/copy/rename/delete）
- [x] 边界处理：源文件不存在/被占用/跨设备/trash 已清
- [x] POST /api/tasks/:id/rollback API
- [x] GET/DELETE /api/trash + POST /api/trash/restore + DELETE /api/trash/:id API
- [x] AI agent delete_file 工具统一走 TrashManager（trash_adapter.go 解决类型不匹配）

## Phase 4: 前端适配
- [x] TaskType union 扩展（move/copy/rename/delete/rollback_*）
- [x] EncvTask 加 rollbackOf/originalPath 字段
- [x] rollbackTask/listTrash/restoreTrash/purgeTrash/emptyTrash API + TrashItem interface
- [x] IndexedDB 降级为纯缓存（progress 不写，LRU 200 条 ensureLRUCache）
- [x] filterTriggeredBy 筛选维度 + useTasksList filteredTasks 过滤
- [x] Files.vue 4 个文件操作改为创建 Task（移除 loadFiles）
- [x] TaskDetailModal 回滚按钮 + 回滚确认对话框
- [x] i18n 翻译完整（中英文各 38 个键 + fsTests 14 个键）

## Phase 5: 测试
- [x] useFileSystemTests composable（8 个用例：4 正向 + 4 边界）
- [x] 开发者选项"文件系统任务测试"入口（AutomationTestsHub + FileSystemTestsDetail + 路由）
- [x] 后端 SQLite Store CRUD 测试（11 个测试通过）
- [x] 后端迁移测试（4 个测试通过）
- [ ] 后端 RollbackManager 各策略测试（未实现，依赖完整 TaskManager 集成测试环境）
- [ ] 后端 TrashManager 测试（未实现，依赖完整文件系统环境）

## 验证（2026-06-22）
- [x] vue-tsc 通过
- [x] vite build 通过（9.01s）
- [x] go build 通过
- [x] go test ./pkg/tasksystem/... 通过（15 个测试）
- [ ] go test ./internal/... 通过（预存在 build failure：task_manager_crypto_params_test.go 引用不存在的 CreateWithCryptoParams，非本轮引入）
- [ ] vitest 相关测试通过（3 个预存在失败，非本轮引入）
- [x] 性能不退化（taskStore Map 索引保留 + IndexedDB 降级为纯缓存减少写入开销）
