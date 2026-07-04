# 修复三重回归缺陷 Spec

## Why

三个严重回归问题，需要一次构建饱和覆盖修复+诊断。

## 修复清单

### 问题 0：文本预览卡顿 + 换行不生效
- **根因**：text.html `#textContent` 使用 `height: 100vh`，在 iframe 内 100vh 不等于 iframe 实际高度
- **修复**：`height: 100vh` → `height: 100%`

### 问题 1：安装确认界面不显示
- **根因**：GoProcessPlugin 启动 InstallConfirmActivity 时：(1) 设置了错误的 `action="com.encvgo.app.INSTALL_RESULT"`（这是 BroadcastReceiver 的 action）；(2) 缺少 `FLAG_ACTIVITY_NEW_TASK`（context 不是 Activity 时必须）
- **修复**：移除错误 action，添加 FLAG_ACTIVITY_NEW_TASK 检查，加 try-catch 诊断
- **饱和调试**：新增 `debugInstallFlow()` @PluginMethod，前端加调试按钮，一次构建即可收集所有运行时信息

### 问题 2：加密验证 deep integrity check 失败
- **根因**：`SkipStructCheck=true` 只跳过 L1，但 L4 `runDeepVideoIntegrityCheck` 不受任何参数控制，无条件执行
- **修复**：VerifyOptions 新增 `SkipDeepCheck bool`，content_verifier.go 条件跳过 L4，plugin.go verifyContainer 设置 SkipDeepCheck=true

## 变更文件

| 文件 | 变更 |
|------|------|
| `internal/openlist/web/static/preview/text.html` | height: 100vh → 100% |
| `app/encv-mobile/android/.../GoProcessPlugin.kt` | 移除错误action + FLAG_ACTIVITY_NEW_TASK + debugInstallFlow方法 |
| `app/encv-mobile/src/plugins/web.ts` | debugInstallFlow 接口定义 |
| `app/encv-mobile/src/plugins/GoProcess.ts` | debugInstallFlow 导出函数 |
| `app/encv-mobile/src/views/ExtensionsPage.vue` | 饱和调试按钮 |
| `internal/v2/plugins/interfaces/interfaces.go` | VerifyOptions 新增 SkipDeepCheck |
| `internal/v2/plugins/video/content_verifier.go` | Verify() 条件跳过 L4 |
| `internal/v2/plugins/video/plugin.go` | verifyContainer 设置 SkipDeepCheck=true |