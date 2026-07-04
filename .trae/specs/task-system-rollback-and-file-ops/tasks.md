# Tasks

## Phase 1: 后端基础设施（pkg/tasksystem/ + SQLite）

- [x] Task 1: 创建 `pkg/tasksystem/` 包，定义 Task/Store/RollbackManager/TrashManager/RollbackStrategy 接口 + 类型枚举
- [x] Task 2: 创建 `pkg/tasksystem/store/sqlite/` SQLite 实现（modernc.org/sqlite，pure-Go），建表 tasks/trash/rollback_snapshots + 索引 + WAL 模式
- [x] Task 3: 实现 JSON → SQLite 迁移逻辑（启动时一次性，迁移后 JSON 重命名为 .migrated）

## Phase 2: 后端文件操作接入任务系统

- [x] Task 4: TaskManager 改用 tasksystem.Store（双写策略：内存 map + SQLite Store），加 4 种 TaskType (move/copy/rename/delete) 到 processTask switch
- [x] Task 5: 实现 `internal/service/file_task_handler.go`：move/copy/rename/delete 4 种任务处理器，执行前保存 pre_state 快照，完成后广播 file:change
- [x] Task 6: 修改 4 个文件操作 handler（admin_handlers.go + mobile_api.go）改为创建 Task 返回 { taskId }

## Phase 3: 后端回收站 + 回滚

- [x] Task 7: 实现 `internal/service/trash_manager.go`：MoveToTrash/Restore/List/Purge/Empty，trash 表 CRUD
- [x] Task 8: 实现 `internal/service/rollback_manager.go` + 6 种 RollbackStrategy（encrypt/decrypt/move/copy/rename/delete），完备边界处理
- [x] Task 9: 新增路由：GET/DELETE /api/trash、POST /api/trash/restore、DELETE /api/trash/:id、POST /api/tasks/:id/rollback
- [x] Task 10: AI agent delete_file 工具统一走 TrashManager（移除硬编码 .trash，trash_adapter.go 解决类型不匹配）

## Phase 4: 前端适配

- [x] Task 11: api/encv.ts 扩展 TaskType union（12 种含 rollback_*）+ EncvTask 加 rollbackOf/originalPath 字段 + 新增 rollbackTask/listTrash/restoreTrash/purgeTrash/emptyTrash API + TrashItem interface
- [x] Task 12: taskPersistence.ts 降级为纯缓存（progress 不写 IndexedDB，LRU 200 条 ensureLRUCache，移除 pruneTerminalTasks）
- [x] Task 13: taskStore.ts applyTaskProgress 移除 putTaskThrottled 调用，hydrate 用 ensureLRUCache
- [x] Task 14: useTaskFilter.ts 加 filterTriggeredBy + toggleTriggeredByFilter + useTasksList.ts filteredTasks 加 triggeredBy 过滤
- [x] Task 15: Files.vue 4 个文件操作改为创建 Task（移除 await loadFiles），加密容器重命名仍走旧 PATCH API
- [x] Task 16: TaskDetailModal.vue 加回滚按钮（canRollback computed）+ 回滚确认对话框 + doRollback 函数
- [x] Task 17: i18n/tasks.ts 加 move/copy/rename/delete/rollback/trash/triggeredBy filter 翻译（中英文各 38 个键）

## Phase 5: 测试

- [x] Task 18: 实现 `useFileSystemTests` composable（8 个测试用例：move/copy/rename/delete + rollback + 4 个边界）
- [x] Task 19: 开发者选项页加"文件系统任务测试"入口（AutomationTestsHub.vue + FileSystemTestsDetail.vue + 路由 + i18n keys）
- [x] Task 20: 后端测试：SQLite Store CRUD（11 个测试）+ 迁移（4 个测试）全部通过

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

## 验证结果（2026-06-22）

- ✅ `go build ./...` 通过
- ✅ `go test ./pkg/tasksystem/...` 通过（15 个测试）
- ✅ `vue-tsc --noEmit` 通过
- ✅ `vite build` 通过（9.01s）
- ⚠️ `internal/service` 测试有预存在 build failure（`task_manager_crypto_params_test.go` 引用不存在的 `CreateWithCryptoParams` 方法），非本轮引入
- ⚠️ vitest 有 3 个预存在失败（useTaskTrigger.test.ts / encv.test.ts / useApiBaseProbe.test.ts），非本轮引入
