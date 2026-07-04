# Tasks

- [x] Task 1: 移除 MainActivity 自动扫描插件逻辑
  - [x] 1.1 删除 MainActivity.kt 中 loadPlugins() 方法的全部自动扫描/安装代码（扫描 filesDir/plugins、installPlugin、loadEnabledPlugins、launchPlugin）
  - [x] 1.2 保留 onCreate() 中对 loadPlugins() 的调用但改为空实现（仅日志输出，不 crash）
  - [x] 1.3 确认 GoProcessPlugin.pickAndInstallPlugin() + installPlugin() 前端驱动流程完整可用（文件选择 → 复制到 cacheDir/plugin_install → 调用 installerManager.installPlugin）

- [x] Task 2: 修复 EncryptFileWithPlugin outputDir 时序错误
  - [x] 2.1 在 registry.go 的 `EncryptFileWithPlugin()` 函数中，在调用 `ProcessFileWithPlugin()` **之前**，通过类型断言 `plugin.(*VideoPlugin)` 调用新增的 `SetOutputDir(outputDir)` 方法
  - [x] 2.2 验证 GetContentPreprocessor() 创建的 VideoContentPreprocessor 携带非空 outputDir

- [x] Task 3: 新增任务删除持久化全链路
  - [x] 3.1 task_manager.go 新增 `RemoveTask(id string)` 方法：从 tasks map 删除 + saveTasks() 持久化 + broadcaster 事件
  - [x] 3.2 mobile_service.go (mobile_api.go) 新增 handleRemoveTaskGin handler + server.go 注册 DELETE /api/tasks/:id
  - [x] 3.3 encv.ts 新增 `removeTask(id: string): Promise<void>` API 函数，DELETE /tasks/:id
  - [x] 3.4 Tasks.vue handleRemoveTask 改为 async 函数：removeTask(id) → loadTasks() → catch toast

- [x] Task 4: 补充 mock 测试覆盖 3 个缺陷场景
  - [x] 4.1 content_preprocessor_test.go: TestOutputDirSetBeforePreprocess — SetOutputDir 后 outputDir 非空且 ensureOutputDir 不报错
  - [x] 4.2 task_remove_persistence_test.go: TestRemoveTask_PersistenceAfterReload — 删除后重载不恢复（3 个子测试）
  - [x] 4.3 registry_test.go: TestRegistry_NoAutoInstallBehavior — registry 层无 install/PluginManager 等符号（6 个子测试）

# Bonus Fix
- [x] RemoveTask RWMutex 死锁修复：mu.Lock() 内调用 saveTasks()（含 mu.RLock()）→ 改为手动 Unlock 后再调用

# Task Dependencies
- [Task 4.1] depends on [Task 2] ✅
- [Task 4.2] depends on [Task 3.1] ✅
- [Task 4.3] depends on [Task 1] ✅
