# Tasks

- [x] Task 1: 创建 TaskManager（internal/service/task_manager.go）
  - [x] 定义 MobileTask 结构体（从 mobile_api.go 迁移）
  - [x] 实现 TaskManager 结构体，包含 tasks map 和 sync.RWMutex
  - [x] 实现 Create(taskType, sourcePath) 方法
  - [x] 实现 List() 方法
  - [x] 实现 Cancel(id) 方法
  - [x] 实现 Retry(id) 方法

- [x] Task 2: 创建 WSHub（internal/service/ws_hub.go）
  - [x] 定义 WSMessage、wsClient 结构体（从 mobile_ws.go 迁移）
  - [x] 实现 WSHub 结构体，包含 clients map 和 sync.RWMutex
  - [x] 实现 RegisterClient(conn) 方法，返回 client 并发送 server:status
  - [x] 实现 UnregisterClient(client) 方法
  - [x] 实现 HandlePing(client) 方法
  - [x] 实现 Broadcast(msgType, data) 方法
  - [x] 实现 writePump(client) 方法

- [x] Task 3: 创建 MobileService（internal/service/mobile_service.go）
  - [x] 定义 MobileService 结构体，持有 servingDir、taskManager、wsHub
  - [x] 实现 NewMobileService(servingDir string) 构造函数
  - [x] 实现 ListFiles(queryPath) 方法 — 文件列表逻辑（从 handleListFilesAPI 迁移）
  - [x] 实现 DeleteFile(queryPath) 方法 — 删除文件逻辑（从 handleDeleteFileAPI 迁移）
  - [x] 实现 ReadFileContent(queryPath) 方法 — 读取文件内容逻辑（从 handleReadFileContent 迁移）
  - [x] 实现 TestWebDAV(url, username, password) 方法 — WebDAV 测试逻辑（从 handleTestWebDAVPost 迁移）
  - [x] 实现 GetTaskManager() 方法 — 返回 TaskManager 引用
  - [x] 实现 GetWSHub() 方法 — 返回 WSHub 引用
  - [x] 迁移 isValidUTF8 和 decodeUTF8Rune 辅助函数

- [x] Task 4: 重构 mobile_api.go handler
  - [x] handleHealth — 保持不变（无业务逻辑）
  - [x] handleServerShutdown — 保持不变（无业务逻辑）
  - [x] handleListFilesAPI — 瘦身为解析参数 → 调用 s.mobileService.ListFiles → 写响应
  - [x] handleDeleteFileAPI — 瘦身为解析参数 → 调用 s.mobileService.DeleteFile → 写响应
  - [x] handleReadFileContent — 瘦身为解析参数 → 调用 s.mobileService.ReadFileContent → 写响应
  - [x] handleGetTasks — 瘦身为调用 s.mobileService.GetTaskManager().List() → 写响应
  - [x] handleCreateTask — 瘦身为解析 body → 调用 taskManager.Create() → 写响应
  - [x] handleCancelTask — 瘦身为解析路径 → 调用 taskManager.Cancel() → 写响应
  - [x] handleRetryTask — 瘦身为解析路径 → 调用 taskManager.Retry() → 写响应
  - [x] handleTestWebDAVPost — 瘦身为解析 body → 调用 s.mobileService.TestWebDAV() → 写响应
  - [x] 删除包级全局变量 tasks、tasksMu

- [x] Task 5: 重构 mobile_ws.go handler
  - [x] handleWebSocket — 瘦身为升级连接 → 调用 wsHub 方法 → 循环读取委托给 wsHub
  - [x] BroadcastMessage — 委托给 wsHub.Broadcast()
  - [x] 删除包级全局变量 wsClients、wsClientsMu、wsUpgrader（wsUpgrader 保留在 ws 包或移入 WSHub）

- [x] Task 6: 修改 Server 结构体和构造函数
  - [x] 在 Server 结构体中新增 mobileService 字段
  - [x] 在 NewServer 中创建 MobileService 实例
  - [x] 在 Start() 中将 servingDir 传递给 MobileService（或在 NewServer 时注入）

- [x] Task 7: 构建验证
  - [x] 运行 `go build ./cmd/encv/` 确保编译通过
  - [x] 确认所有 API 行为不变

# Task Dependencies
- [Task 1] 和 [Task 2] 可并行
- [Task 3] 依赖 [Task 1] 和 [Task 2]
- [Task 4] 和 [Task 5] 依赖 [Task 3]
- [Task 6] 依赖 [Task 3]
- [Task 7] 依赖 [Task 4] [Task 5] [Task 6]
