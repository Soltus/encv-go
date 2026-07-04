# P0 原始文件丢失 + iframe 回退 + 插件安装修复 Spec

## Why

上一轮修复引入/遗漏了三个严重问题：

1. **【P0 数据丢失】加密后预处理临时文件消失 + stsz box missing 验证失败**：`TempFileReadCloser.Close()` 在 `defer` 中自动删除底层文件（[temp_file.go:37](internal/v2/reader/temp_file.go#L37)）。虽然当前代码顺序下 verifyContainer 先于 Close 执行，但 `.encv_tmp` 隐藏目录修改前临时文件创建在用户可见目录且 Close 必然删除它——用户看到"文件消失"。stsz box missing 可能是因为 remux 后 MP4 box 结构变化导致 v4 quick check 不适用。

2. **【回退】文本预览 iframe 被错误移除**：用户明确确认 iframe 方案在 openlist 测试 100M 大文本毫无压力。上轮修改将 iframe 替换为 fetch+pre 方案，反而将整个文件加载到内存，性能更差。

3. **【功能缺失】插件安装根本没成功**：[GoProcessPlugin.kt:556](app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt#L556) 反射查找 `installPlugin` 方法时使用 `parameterCount == 1`，但 ComboLite 真实 API 是 `installPlugin(apkFile: File, forceOverwrite: Boolean)` — **2 个参数**。查找永远返回 null → 静默 fallback 到系统 ACTION_INSTALL_PACKAGE → 安装实际未完成或用户不知情。

## What Changes

- **恢复 FilePreview.vue 的 iframe 文本预览**
- **修复 TempFileReadCloser 生命周期**：Close 不自动删除文件
- **修复 ComboLite installPlugin 反射调用参数数量**：1→2

## Impact

- Affected code:
  - `app/encv-mobile/src/views/FilePreview.vue` — 恢复 iframe
  - `internal/v2/reader/temp_file.go` — Close 行为变更
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — parameterCount 1→2
  - `internal/v2/plugins/video/content_preprocessor.go` / `registry.go` — 临时文件清理时机

## ADDED Requirements

### Requirement 1: 文本预览使用 iframe 流式渲染

恢复 `<iframe :src="textPreviewUrl">` 渲染方式。完全回退上轮的 fetch+pre 修改。

### Requirement 2: 临时文件 Close 不自动删除

`TempFileReadCloser.Close()` 仅关闭文件句柄，不执行 os.Remove()。清理由调用方显式控制。

### Requirement 3: ComboLite installPlugin 使用正确的参数数量

反射查找 `installPlugin` 方法时匹配 `parameterCount == 2`（非 1），调用时传入 `(apkFile, true)`（forceOverwrite=true）。

#### Scenario: 通过前端选择 APK 安装插件
- **WHEN** 用户在 ExtensionsPage 选择 APK 文件
- **THEN** GoProcessPlugin 找到 ComboLite `installPlugin(File, Boolean)` 方法
- **AND** 调用 `installMethod.invoke(pm, apkFile, true)`
- **AND** 返回 `method: "combolite"` + `success: true`
- **AND** 前端 ExtensionsPage 显示「已安装」

## MODIFIED Requirements

### Requirement: TempFileReadCloser.Close 行为

| | 之前 | 之后 |
|--|------|------|
| Close() | file.Close() + **os.Remove(path)** | 仅 **file.Close()** |

所有依赖 Close 自动清理的调用点需添加显式 os.Remove。

### Requirement: installFromPath 反射方法匹配

| | 之前 | 之后 |
|--|------|------|
| 参数数过滤 | `parameterCount == 1` | `parameterCount == 2` |
| 调用参数 | `invoke(pm, apkFile)` | `invoke(pm, apkFile, true)` |

### Requirement: FilePreview.vue 文本渲染

完全回退到 iframe 方案：移除 textContent/textLoading/textError/loadTextContent/.text-content 样式。

## REMOVED Requirements

### fetch+pre 文本渲染方案

**原因**：用户确认 iframe 大文件表现良好，替换方案降低性能
**迁移**：完全回退
