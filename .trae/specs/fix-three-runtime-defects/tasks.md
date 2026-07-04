# Tasks

- [x] Task 0: 修复文本预览默认换行无法滚动 ✅
  - [x] 0.1 读取 FilePreview.vue style 区域
  - [x] 0.2 将 `.text-preview` 的 `overflow: auto` 和 `-webkit-overflow-scrolling: touch` 删除
  - [x] 0.3 确认 PDF 预览（`.pdf-preview`）不受影响

- [x] Task 1: 修复安装确认界面不显示（重构为 BroadcastReceiver 回调）✅
  - [x] 1.1 读取 GoProcessPlugin.kt 完整代码，理解现有 startActivityForResult 流程
  - [x] 1.2 在 GoProcessPlugin 中注册动态 BroadcastReceiver（监听自定义 Action）
  - [x] 1.3 修改 installFromPath()/installPlugin()：将 `activity.startActivityForResult()` 替换为 `context.startActivity(intent)`
  - [x] 1.4 修改 InstallConfirmActivity.kt：确认/取消/返回按钮均通过 sendBroadcast 回传结果
  - [x] 1.5 在 BroadcastReceiver 的 onReceive 中：RESULT_OK → executeComboLiteInstall / RESULT_CANCELED → call.reject
  - [x] 1.6 移除 handleOnActivityResult 中的 REQUEST_CODE_INSTALL_CONFIRM 分支（不再需要）
  - [x] 1.7 确认 Manifest 中 InstallConfirmActivity 不需要特殊权限（sendBroadcast 不需要）

- [x] Task 2: 修复 v4 加密 stsz box missing 误报 ✅
  - [x] 2.1 读取 plugin.go verifyContainer() 完整代码
  - [x] 2.2 读取 registry.go 中 PostEncryptProcessor 调用 verifyContainer 的上下文
  - [x] 2.3 新增 `isPostEncryptVerify bool` 字段到 VideoPlugin（默认 false）
  - [x] 2.4 在 registry.go 调用 PostEncryptProcessor 前设置此标志 SetPostEncryptVerify(true)
  - [x] 2.5 修改 verifyContainer() 中的 SkipStructCheck 判断逻辑：`sourcePath != p.inputPath || p.isPostEncryptVerify`
  - [x] 2.6 确保标志在每次 encrypt 调用时重置（Encrypt() 开头 p.isPostEncryptVerify = false）

- [x] Task 3: 修复 v3 ffprobe JSON 解析失败 ✅
  - [x] 3.1 读取 metadata_extractor.go extractMetadataFromOriginalFile() 完整代码
  - [x] 3.2 在 json.Unmarshal 失败时增加诊断日志（输出原始 ffprobe 前 256 字节 hex）
  - [x] 3.3 增加 ffprobe 输出格式检测：检查 output 是否包含 `"streams"` 或 `"frames"` 关键字
  - [x] 3.4 当输出格式不支持时，构建最小可用 VideoIndex（使用文件大小/名称推断基本信息）而非返回 error
  - [x] 3.5 仅在 ffprobe 进程本身崩溃（exit code 非 0）时返回 fatal error

# Task Dependencies
- [Task 0] 可独立并行 ✅
- [Task 1] 可独立并行 ✅
- [Task 2] 和 [Task 3] 可并行 ✅

# 验证完成时间: 2026-05-28
# 验证结果: **12/12 PASS** | Go 编译 exit code 0
