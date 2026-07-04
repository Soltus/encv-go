# 修复三大运行时缺陷 Spec

## Why

CI 构建通过后，用户实际使用暴露了 3 个严重运行时缺陷：
1. **插件加载架构矛盾**：MainActivity 自动扫描 `{filesDir}/plugins/*.apk` 目录安装插件，但前端已有完整的「用户选择 APK → 安装」流程（PluginSettings.vue + GoProcessPlugin.pickAndInstallPlugin）。两者冲突导致插件卡在「安装中」状态无任何报错反馈。
2. **加密 MP4 失败 `outputDir is empty`**：`EncryptFileWithPlugin()` 调用顺序错误 — `ProcessFileWithPlugin()` 内部调用 `GetContentPreprocessor().Preprocess()` 需要 `outputDir`，但 `outputDir` 要等到之后的 `PreEncryptProcessor()` 才被赋值。之前的 mock 测试未覆盖此执行顺序。
3. **任务左滑移除不持久化**：`handleRemoveTask()` 仅过滤内存中的响应式数组，未调用后端删除 API，刷新后从持久化存储恢复。

## What Changes

- **移除 MainActivity 自动扫描插件逻辑**，插件安装完全由前端用户操作驱动
- **修复 EncryptFileWithPlugin 执行顺序**，确保 outputDir 在 Preprocess 前就位
- **新增任务删除全链路**：后端 API + 前端函数 + 持久化写入
- **补充缺失的 mock 测试**覆盖以上 3 个场景

## Impact

- Affected code:
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt` — 移除 loadPlugins() 自动扫描
  - `internal/v2/plugins/registry.go` — 修复 outputDir 时序
  - `internal/service/task_manager.go` — 新增 RemoveTask 方法
  - `internal/service/mobile_service.go` — 新增 DELETE /tasks/:id 路由
  - `app/encv-mobile/src/api/encv.ts` — 新增 removeTask 函数
  - `app/encv-mobile/src/views/Tasks.vue` — handleRemoveTask 改为调用 API

## ADDED Requirements

### Requirement: 插件安装仅由前端驱动

系统 SHALL NOT 在应用启动时自动扫描磁盘目录安装插件。插件安装 SHALL 只通过前端用户操作触发：

#### Scenario: 应用启动不自动安装插件
- **WHEN** 应用启动
- **THEN** MainActivity 不扫描任何目录、不调用 installPlugin
- **AND** 插件列表由 ComboLite PluginManager 从已安装记录加载

#### Scenario: 用户通过前端选择并安装插件
- **WHEN** 用户在 PluginSettings 页面点击选择 APK 文件
- **THEN** 调用 pickAndInstallPlugin → 系统文件选择器打开 → 用户选中后复制到临时目录 → 调用 ComboLite installerManager.installPlugin() → 返回成功/失败结果给前端显示

### Requirement: 加密流程 outputDir 正确初始化

VideoContentPreprocessor.Preprocess() 调用时 outputDir SHALL 已被正确设置：

#### Scenario: MP4 文件加密预处理
- **WHEN** 对 MP4 文件执行加密且需要 fast-start remux
- **THEN** outputDir 在 Preprocess() 执行前已被赋值
- **AND** ensureOutputDir() 不再返回 "outputDir is empty" 错误

### Requirement: 任务删除持久化

任务删除 SHALL 同时更新内存和持久化存储：

#### Scenario: 左滑移除已完成/失败任务
- **WHEN** 用户在 Tasks 页面左滑点击「移除」按钮
- **THEN** 调用后端 DELETE /tasks/:id 接口
- **AND** 后端从 .encv-tasks.json 持久化文件中移除该任务
- **AND** 前端重新加载任务列表
- **WHEN** 之后刷新页面
- **THEN** 被移除的任务不再出现

## MODIFIED Requirements

### Requirement: EncryptFileWithPlugin 执行顺序

`EncryptFileWithPlugin()` SHALL 在调用 `ProcessFileWithPlugin()` 之前设置 `p.outputDir = outputDir`，确保 ContentPreprocessor 创建时携带有效的输出目录路径。

### Requirement: handleRemoveTask 行为变更

`handleRemoveTask(id)` SHALL 从纯前端过滤改为：
1. 调用 `removeTask(id)` API 发送 DELETE 请求到后端
2. 成功后调用 `loadTasks()` 重新从后端获取最新列表
3. 失败时显示 toast 错误提示

## REMOVED Requirements

### Requirement: MainActivity 自动扫描插件目录

**原因**：与前端驱动的插件选择架构矛盾，导致重复安装和卡死状态
**迁移**：完全移除，插件生命周期完全由 ComboLite PluginManager 管理
