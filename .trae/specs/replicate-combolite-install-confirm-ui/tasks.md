# Tasks

- [x] Task 1: 创建 InstallConfirmActivity.kt（Compose 安装确认界面）
  - [x] 1.1 创建 `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/InstallConfirmActivity.kt`
  - [x] 1.2 实现 APK 元数据提取逻辑（图标/包名/文件名/大小）
  - [x] 1.3 复刻官方 InstallPermissionScreen 布局（Scaffold + TopAppBar + 图标区 + 警告横幅 + InfoRows + 按钮）
  - [x] 1.4 实现「确认安装」→ setResult(OK) + finish、「取消」→ setResult(CANCELED) + finish
  - [x] 1.5 处理 APK 解析失败的降级显示（默认图标 + 仅显示文件名）

- [x] Task 2: 在 AndroidManifest.xml 中注册 InstallConfirmActivity
  - [x] 2.1 添加 activity 声明（exported=false, NoActionBar 主题, configChanges）

- [x] Task 3: 改造 GoProcessPlugin.kt 安装流程（增加确认步骤）
  - [x] 3.1 修改 `installPlugin(call)` 方法：先启动 InstallConfirmActivity，确认后才调 installerManager.installPlugin()
  - [x] 3.2 修改 `installFromPath(call, apkPath, name)` 同上
  - [x] 3.3 处理 onActivityResult：RESULT_OK 继续安装，RESULT_CANCELED reject
  - [x] 3.4 定义 REQUEST_CODE_INSTALL_CONFIRM 常量

# Task Dependencies
- [Task 2] 可与 [Task 1] 并行（Manifest 注册不依赖代码完成）
- [Task 3] 依赖 [Task 1] 和 [Task 2]（需要 Activity 存在才能启动）
