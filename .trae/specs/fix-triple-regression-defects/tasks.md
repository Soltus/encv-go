# Tasks

- [x] Task 1: 修复文本预览 iframe 高度
  - [x] text.html height: 100vh → 100%

- [x] Task 2: 修复 InstallConfirmActivity 启动
  - [x] 移除错误 action="com.encvgo.app.INSTALL_RESULT"
  - [x] 添加 FLAG_ACTIVITY_NEW_TASK 检查
  - [x] 添加 try-catch 诊断日志
  - [x] 两处（installPlugin + installFromPath）都修复

- [x] Task 3: 新增 SkipDeepCheck 参数链路
  - [x] interfaces.go VerifyOptions 新增 SkipDeepCheck bool
  - [x] content_verifier.go Verify() 条件跳过 runDeepVideoIntegrityCheck
  - [x] plugin.go verifyContainer() 设置 SkipDeepCheck=true

- [x] Task 4: 饱和调试入口
  - [x] GoProcessPlugin.kt 新增 debugInstallFlow() @PluginMethod
  - [x] web.ts 接口定义 + GoProcess.ts 导出函数
  - [x] ExtensionsPage.vue 调试按钮

- [x] Task 5: 验证
  - [x] Go 编译通过
  - [x] vue-tsc + vite build 通过
  - [x] encryption_roundtrip_e2e_test 通过