# Checklist

## Phase 1: 后端基础设施
- [ ] pkg/tasksystem/ 包定义 Task/Store/RollbackManager/TrashManager/RollbackStrategy 接口
- [ ] pkg/tasksystem/store/sqlite/ 实现 SQLite Store（glebarez/sqlite, CGO_ENABLED=0）
- [ ] tasks/trash/rollback_snapshots 三张表 + 索引创建
- [ ] WAL 模式启用
- [ ] JSON → SQLite 迁移逻辑（迁移后 JSON 重命名 .migrated，失败回滚）

## Phase 2: 文件操作接入任务系统
- [ ] TaskManager 改用 tasksystem.Store（移除 JSON 持久化）
- [ ] processTask switch 加 move/copy/rename/delete 4 个 case
- [ ] file_task_handler.go 实现 4 种处理器（执行前保存 pre_state 快照）
- [ ] 4 种文件操作完成后广播 file:change（create/delete/modify）
- [ ] 4 个 handler 返回 { taskId } (202 Accepted)

## Phase 3: 回收站 + 回滚
- [ ] TrashManager 实现 MoveToTrash/Restore/List/Purge/Empty
- [ ] trash 表 CRUD
- [ ] RollbackManager 实现 CanRollback/Rollback
- [ ] 6 种 RollbackStrategy 实现（encrypt/decrypt/move/copy/rename/delete）
- [ ] 边界处理：源文件不存在/被占用/跨设备/trash 已清
- [ ] POST /api/tasks/:id/rollback API
- [ ] GET/DELETE /api/trash + POST /api/trash/restore API
- [ ] AI agent delete_file 工具统一走 TrashManager

## Phase 4: 前端适配
- [ ] TaskType union 扩展（move/copy/rename/delete/rollback_*）
- [ ] EncvTask 加 rollbackOf/originalPath 字段
- [ ] rollbackTask/listTrash/restoreTrash/deleteTrash/emptyTrash API
- [ ] IndexedDB 降级为纯缓存（progress 不写，LRU 200 条）
- [ ] filterTriggeredBy 筛选维度 + Tasks.vue triggeredBy chip
- [ ] Files.vue 4 个文件操作改为创建 Task（移除 loadFiles）
- [ ] TaskDetailModal 回滚按钮 + RollbackConfirmDialog
- [ ] 任务卡片 rollback_* 图标/标签
- [ ] i18n 翻译完整

## Phase 5: 测试
- [ ] useFileSystemTests composable（8 个用例）
- [ ] 开发者选项"文件系统任务测试"入口
- [ ] 后端 SQLite Store CRUD 测试
- [ ] 后端迁移测试
- [ ] 后端 RollbackManager 各策略测试
- [ ] 后端 TrashManager 测试

## 验证
- [ ] vue-tsc 通过
- [ ] vite build 通过
- [ ] go build 通过
- [ ] go test ./internal/... 通过
- [ ] vitest 相关测试通过
- [ ] 性能不退化（taskStore Map 索引 + 模板预计算 displayData 保留）
