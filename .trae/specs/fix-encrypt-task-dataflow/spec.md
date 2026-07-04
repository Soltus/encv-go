# 修复 alist_encrypt 加密任务端到端数据流断点 Spec

## Why

alist_encrypt 加密任务从创建到执行存在多处数据流断点，导致用户填写密码后仍然报错 "encryption requires a password" 或 nil pointer dereference。核心原因是：插件单例复用导致 taskExtraFields 状态泄漏、前端 ExtraField label 未走 i18n 翻译。

## 已确认兼容的部分

插件系统**已经兼容** alist_encrypt 的独立加密流程：

1. **`EncryptFileWithPlugin` 双路径**（[registry.go:569-598](file:///workspace/internal/v2/plugins/registry.go#L569-L598)）：`needsPreprocessing` 判断正确，alist_encrypt 的 `GetMetadataExtractor()` 和 `GetContentPreprocessor()` 都返回 `nil`，走 `else` 分支直接 `os.Open` + `Encrypt(file)` ✅
2. **`FindAllEncryptingPlugins` 通用匹配**（[registry.go:477-502](file:///workspace/internal/v2/plugins/registry.go#L477-L502)）：阶段 3 把没有声明 MIME/扩展名的通用插件作为 P1 候选返回 ✅
3. **`PasswordIndependent` 策略**：`processEncrypt` 中 `isPasswordIndependent` 检查正确跳过全局密码空检查 ✅
4. **`SetTaskExtraFields` 调用时机**：在 `EncryptFileWithPlugin` 之前调用 ✅
5. **`PreEncryptProcessor` → `Encrypt` 数据流**：`outputDir` 和 `inputPath` 在 `PreEncryptProcessor` 中设置，`Encrypt` 中使用 ✅

## 仍需修复的断点

### 断点 1：插件单例 + taskExtraFields 状态泄漏（P0）

**现状**：`Plugins` 列表中的插件实例是全局单例（[registry.go:32-40](file:///workspace/internal/v2/plugins/registry.go#L32-L40)），`FindPluginByName` 返回同一个指针。

**问题**：`processEncrypt` 在每次任务执行时调用 `setter.SetTaskExtraFields(task.ExtraFields)`（[task_manager.go:506-508](file:///workspace/internal/service/task_manager.go#L506-L508)），但**任务完成后不会清除** `taskExtraFields`。如果：

1. 任务 A 设置了 `taskExtraFields = {"plugin_password": "abc"}`
2. 任务 B 没有设置 `plugin_password`（用户留空）
3. 任务 B 的 `resolvePasswordFromTask()` 会读到任务 A 残留的 `"abc"`

这是典型的单例状态泄漏。同理，`outputDir`、`inputPath` 等字段也存在类似问题。

### 断点 2：前端 ExtraField label 未走 i18n 翻译（P1）

**现状**：[NewTaskModal.vue:136](file:///workspace/app/encv-mobile/src/components/NewTaskModal.vue#L136) 中 ExtraField 的 label 直接使用 `field.label`：

```html
:label="field.label"
```

**问题**：后端 `GetTaskOptions()` 返回的 label 是 i18n key（如 `"tasks.pluginPassword"`），但前端没有调用 `t()` 翻译。用户看到的是原始 key 而不是翻译后的文字。

### 断点 3：调试日志残留（P3）

**现状**：[plugin.go:154](file:///workspace/internal/v2/plugins/alistencrypt/plugin.go#L154) 和 [registry.go:570](file:///workspace/internal/v2/plugins/registry.go#L570) 有 `slog.Info` 调试日志，应在生产环境中降级为 `slog.Debug`。

## What Changes

- **新增 `ResetTaskState()` 接口**：插件在任务完成后重置任务级状态（taskExtraFields、outputDir、inputPath 等）
- **processEncrypt / processDecrypt 在任务完成后调用 `ResetTaskState()`**
- **前端 ExtraField label 走 `t()` 翻译**
- **调试日志降级为 `slog.Debug`**

## Impact

- Affected code:
  - `internal/v2/plugins/interfaces/interfaces.go` — 新增 `TaskStateResetter` 接口
  - `internal/v2/plugins/alistencrypt/plugin.go` — 实现 `ResetTaskState()`，调试日志降级
  - `internal/v2/plugins/video/plugin.go` — 可能也需要实现 `ResetTaskState()`
  - `internal/service/task_manager.go` — 任务完成后调用 `ResetTaskState()`
  - `internal/v2/plugins/registry.go` — 调试日志降级
  - `app/encv-mobile/src/components/NewTaskModal.vue` — ExtraField label 走 `t()` 翻译

---

## ADDED Requirements

### REQ-1: TaskStateResetter 接口（P0）

系统 SHALL 提供 `TaskStateResetter` 接口，允许插件在任务完成后重置任务级状态。

```go
type TaskStateResetter interface {
    ResetTaskState()
}
```

#### Scenario: 任务完成后状态重置
- **WHEN** 一个加密/解密任务完成（无论成功或失败）
- **THEN** TaskManager SHALL 检查插件是否实现 `TaskStateResetter`
- **AND** 如果实现了，调用 `ResetTaskState()` 清除 taskExtraFields 等任务级状态

### REQ-2: AlistEncryptPlugin 实现 ResetTaskState（P0）

`AlistEncryptPlugin` SHALL 实现 `ResetTaskState()`，重置以下字段：
- `taskExtraFields` → `nil`
- `outputDir` → `""`
- `inputPath` → `""`

#### Scenario: 连续两个加密任务状态隔离
- **WHEN** 任务 A 设置 `taskExtraFields = {"plugin_password": "abc"}` 并完成
- **AND** 任务 B 不设置 `plugin_password`
- **THEN** 任务 B 的 `resolvePasswordFromTask()` SHALL 返回空字符串（回退到 DefaultPassword），而非任务 A 的残留值

### REQ-3: TaskManager 在任务完成后调用 ResetTaskState（P0）

`processEncrypt` 和 `processDecrypt` SHALL 在任务结束（成功或失败）时调用 `ResetTaskState()`。

#### Scenario: 成功任务后重置
- **WHEN** 加密任务成功完成
- **THEN** 插件的 `ResetTaskState()` 被调用

#### Scenario: 失败任务后重置
- **WHEN** 加密任务失败（failTask）
- **THEN** 插件的 `ResetTaskState()` 仍被调用

### REQ-4: 前端 ExtraField label 走 i18n 翻译（P1）

NewTaskModal.vue 中 ExtraField 的 label 和 help/placeholder SHALL 通过 `t()` 函数翻译。

#### Scenario: alist_encrypt 密码字段显示翻译文字
- **WHEN** 用户选择 alist_encrypt 插件
- **THEN** 密码输入框的 label SHALL 显示 "插件密码"（中文）或 "Plugin Password"（英文），而非原始 key "tasks.pluginPassword"

### REQ-5: 调试日志降级（P3）

以下调试日志 SHALL 从 `slog.Info` 降级为 `slog.Debug`：
- `alistencrypt/plugin.go` 中 `Encrypt` 方法的参数日志
- `registry.go` 中 `EncryptFileWithPlugin` 的参数日志

## MODIFIED Requirements

无

## REMOVED Requirements

无
