# Tasks

## Phase 1: 后端基础设施（pkg/tasksystem/ + SQLite）

- [ ] Task 1: 创建 `pkg/tasksystem/` 包，定义 Task/Store/RollbackManager/TrashManager/RollbackStrategy 接口 + 类型枚举
- [ ] Task 2: 创建 `pkg/tasksystem/store/sqlite/` SQLite 实现（glebarez/sqlite），建表 tasks/trash/rollback_snapshots + 索引 + WAL 模式
- [ ] Task 3: 实现 JSON → SQLite 迁移逻辑（启动时一次性，迁移后 JSON 重命名为 .migrated）

## Phase 2: 后端文件操作接入任务系统

- [ ] Task 4: TaskManager 改用 tasksystem.Store（移除 JSON 持久化），加 4 种 TaskType (move/copy/rename/delete) 到 processTask switch
- [ ] Task 5: 实现 `internal/service/file_task_handler.go`：move/copy/rename/delete 4 种任务处理器，执行前保存 pre_state 快照，完成后广播 file:change
- [ ] Task 6: 修改 4 个文件操作 handler（admin_handlers.go + mobile_api.go）改为创建 Task 返回 { taskId }

## Phase 3: 后端回收站 + 回滚

- [ ] Task 7: 实现 `internal/service/trash_manager.go`：MoveToTrash/Restore/List/Purge/Empty，trash 表 CRUD
- [ ] Task 8: 实现 `internal/service/rollback_manager.go` + 6 种 RollbackStrategy（encrypt/decrypt/move/copy/rename/delete），完备边界处理
- [ ] Task 9: 新增路由：GET/DELETE /api/trash、POST /api/trash/restore、POST /api/tasks/:id/rollback
- [ ] Task 10: AI agent delete_file 工具统一走 TrashManager（移除硬编码 .trash）

## Phase 4: 前端适配

- [ ] Task 11: api/encv.ts 扩展 TaskType union + EncvTask 加 rollbackOf/originalPath 字段 + 新增 rollbackTask/listTrash/restoreTrash/deleteTrash/emptyTrash API
- [ ] Task 12: taskPersistence.ts 降级为纯缓存（progress 不写 IndexedDB，LRU 200 条，移除 pruneTerminalTasks）
- [ ] Task 13: taskStore.ts applyTaskProgress 移除 putTaskThrottled 调用
- [ ] Task 14: useTaskFilter.ts 加 filterTriggeredBy + useTasksList.ts filteredTasks 加 triggeredBy 过滤 + Tasks.vue 加 triggeredBy chip
- [ ] Task 15: Files.vue 4 个文件操作改为创建 Task（移除 await loadFiles），依赖 file:change 增量更新
- [ ] Task 16: TaskDetailModal.vue 加回滚按钮 + RollbackConfirmDialog.vue + 任务卡片 rollback_* 图标/标签
- [ ] Task 17: i18n/tasks.ts 加 move/copy/rename/delete/rollback/trash/triggeredBy filter 翻译

## Phase 5: 测试

- [ ] Task 18: 实现 `useFileSystemTests` composable（8 个测试用例：move/copy/rename/delete + rollback + 4 个边界）
- [ ] Task 19: 开发者选项页加"文件系统任务测试"入口
- [ ] Task 20: 后端测试：SQLite Store CRUD + 迁移 + RollbackManager 各策略 + TrashManager

## Task Dependencies
- Task 2 依赖 Task 1
- Task 3 依赖 Task 2
- Task 4 依赖 Task 2
- Task 5 依赖 Task 4
- Task 6 依赖 Task 5
- Task 7 依赖 Task 2
- Task 8 依赖 Task 7
- Task 9 依赖 Task 7, Task 8
- Task 10 依赖 Task 7
- Task 11-17 依赖 Task 9
- Task 18-19 依赖 Task 11-17
- Task 20 依赖 Task 2, Task 8, Task 7
