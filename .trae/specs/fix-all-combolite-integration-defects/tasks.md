# Tasks

- [x] Task 1: 重写 GoProcessPlugin.kt 反射代码为直接 API 调用（核心 P0 修复）
  - [x] 1.1 读取 GoProcessPlugin.kt 完整内容，标记所有反射代码块（3 处）
  - [x] 1.2 添加正确的 import：`com.combo.core.runtime.PluginManager`
  - [x] 1.3 重写 `installPlugin()` 方法（L367-422）：移除反射，改用 `PluginManager.installerManager.installPlugin(apkFile, true)` 直接调用，处理 InstallResult 返回值
  - [x] 1.4 重写 `installFromPath()` 方法（L518-570）：同上
  - [x] 1.5 重写 `checkInstalledPlugins()` 方法（L442-461）：移除反射，改用 `PluginManager.getAllInstallPlugins()`
  - [x] 1.6 移除所有不再需要的反射相关 import 和变量（Class, Method, InvocationTargetException 等）

- [x] Task 2: 配置 ProxyManager 支持 Activity 代理启动
  - [x] 2.1 确认 EncvApplication 是否已正确继承 BaseHostApplication（检查 onCreate 链路）
  - [x] 2.2 创建 EncvHostActivity.kt（继承 BaseHostActivity），放在 `com.encvgo.app` 包下
  - [x] 2.3 在 AndroidManifest.xml 中注册 EncvHostActivity（exported=false 或根据需要配置 intent-filter）
  - [x] 2.4 在 EncvApplication.onCreate() 或 BaseHostApplication.onFrameworkSetup() 中添加 `PluginManager.proxyManager.setHostActivity(EncvHostActivity::class.java)`
  - [x] 2.5 确认 PlayerEntry.startMpvPlayer() 的 Intent 启动方式是否与 HostActivity 代理兼容（可能需要调整）

- [x] Task 3: 更新 fix-p0-file-loss-and-reverts checklist（标记已完成的 3 项修复）
  - [x] 3.1 验证 FilePreview.vue iframe 文本预览已恢复
  - [x] 3.2 验证 TempFileReadCloser.Close() 已移除 os.Remove
  - [x] 3.3 验证 parameterCount 已改为 2（代码库无残留，问题已消除）
  - [x] 3.4 更新 fix-p0-file-loss-and-reverts/checklist.md 勾选已完成项

# Task Dependencies
- [Task 2] depends on [Task 1]（安装逻辑正确后，ProxyManager 配置才有意义）
- [Task 3] 可与 [Task 1] 并行执行
