# Tasks

- [x] Task 1: 创建 GoProcessPlugin API 契约测试（T1）
  - [x] 1.1 创建 `internal/service/goprocessplugin_contract_test.go`
  - [x] 1.2 实现 @PluginMethod 方法签名枚举（通过读取 Kotlin 源码或硬编码预期签名表）
  - [x] 1.3 实现 pendingCalls 键值一致性验证
  - [x] 1.4 实现 BroadcastReceiver 注册/注销生命周期验证（mock context.registerReceiver）

- [x] Task 2: 创建 ComboLite API 调用规范静态检查（T2）
  - [x] 2.1 创建 `internal/service/combolite_static_check_test.go`
  - [x] 2.2 实现「零反射」扫描：读取 GoProcessPlugin.kt 源码，搜索 Class.forName/getMethod/invoke 模式
  - [x] 2.3 实现 InstallerManager 访问路径检查：确认 installPlugin 调用经过 installerManager
  - [x] 2.4 实现 EncvApplication onFrameworkSetup 检查：确认 setHostActivity 调用存在

- [x] Task 3: 创建加密 E2E 全链路测试（T3）
  - [x] 3.1 创建 `internal/v2/plugins/video/encryption_roundtrip_e2e_test.go`
  - [x] 3.2 准备小型测试 MP4 固件文件（或在测试中生成最小有效 MP4）
  - [x] 3.3 实现 v3 容器 encrypt→decrypt→MD5 比对测试
  - [x] 3.4 实现 v4 容器 encrypt→decrypt+verify（SkipStructCheck）测试
  - [x] 3.5 实现加密后原始文件存在性验证（P0 防护回归）
  - [x] 3.6 实现 ffprobe 异常格式容错验证（注入 frames 格式输出）

- [x] Task 4: 创建前端↔Go API 对接测试（T4）
  - [x] 4.1 创建 `app/encv-mobile/__tests__/api-contract.test.ts`
  - [x] 4.2 实现 file list API 响应格式验证（mock server + 断言 response shape）
  - [x] 4.3 实现 encrypt API 参数/响应格式验证
  - [x] 4.4 实现 preview URL 生成逻辑验证（getFilePreviewUrl 参数组合）

- [x] Task 5: 创建插件安装全链路前端测试（T6）
  - [x] 5.1 创建 `app/encv-mobile/__tests__/extensions-install-flow.test.ts`
  - [x] 5.2 mock GoProcessPlugin 的 {installPlugin, pickAndInstallPlugin, checkInstalledPlugins}
  - [x] 5.3 实现安装状态机转换测试（idle→picking→confirming→installing→success）
  - [x] 5.4 实现 120s 超时边界测试（BroadcastReceiver 模式应不再触发超时）
  - [x] 5.5 实现 InstallConfirmActivity Intent 数据传递验证（mock bridge.startActivity）

- [x] Task 6: 创建视频播放器启动链路测试（T7）
  - [x] 6.1 创建 `app/encv-mobile/__tests__/player-entry.test.ts`
  - [x] 6.2 mock PlayerEntry 的 startMpvPlayer / startArtPlayer
  - [x] 6.3 实现 MPV 插件已加载时的 Intent 组件验证
  - [x] 6.4 实现 MPV 未加载时的 ArtPlayer fallback 验证
  - [x] 6.5 实现 ProxyManager 路由到 EncvHostActivity 的逻辑验证

- [x] Task 7: 更新 CI 工作流测试矩阵（TC1）
  - [x] 7.1 读取 `.github/workflows/android.yml`
  - [x] 7.2 在现有 "Run Go unit tests" 步骤中扩展测试包路径（service + video plugin）
  - [x] 7.3 添加 ComboLite 静态检查步骤（运行 Task 2 的测试）
  - [x] 7.4 添加加密 E2E 测试步骤（在 test.yml Layer 3 中运行）
  - [x] 7.5 确保 Layer 1 总耗时 <5min（快速反馈）

- [x] Task 8: 创建专用测试 CI 工作流文件 test.yml（TC3）
  - [x] 8.1 创建 `.github/workflows/test.yml` 完整工作流文件
  - [x] 8.2 实现 workflow_dispatch + pull_request + push + schedule 四种触发
  - [x] 8.3 实现 layer1-quick-tests job（frontend-vitest + go-core-test + combolite-static matrix）
  - [x] 8.4 实现 layer2-full-regression job（go-full+coverage + frontend-coverage + api-contract，needs layer1）
  - [x] 8.5 实现 layer3-e2e-integration job（encryption-e2e + config-consistency，needs layer2）
  - [x] 8.6 实现 test-summary job（聚合结果 + 生成报告）
  - [x] 8.7 配置缓存策略（Go/npm 与 android.yml 共享 key 前缀）
  - [x] 8.8 配置超时控制和失败策略

# Task Dependencies
- [Task 1] 可独立并行 ✅
- [Task 2] 可独立并行 ✅
- [Task 3] 可独立并行（需要测试固件或生成逻辑） ✅
- [Task 4] 可独立并行 ✅
- [Task 5] 可独立并行（前端测试） ✅
- [Task 6] 可独立并行（前端测试） ✅
- [Task 7] 依赖 [Task 1-6] 完成（CI 集成新测试） ✅
- [Task 8] 独立创建 ✅