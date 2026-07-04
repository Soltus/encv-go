# ENCV-Mobile 后端 Service 层重构 Spec

## Why

当前 `internal/server/mobile_api.go` 和 `mobile_ws.go` 中的所有业务逻辑直接嵌入在 HTTP handler 函数中，存在以下问题：
1. **业务逻辑与 HTTP 传输层耦合**：文件操作、任务管理、WebDAV 测试等逻辑直接写在 handler 中，无法独立测试或复用
2. **全局状态管理**：任务存储使用包级变量 `tasks` 和 `tasksMu`，WebSocket 客户端使用包级变量 `wsClients` 和 `wsClientsMu`，无法支持多实例或依赖注入
3. **违反单一职责**：Server 结构体同时承担路由分发和业务逻辑两重职责

需要将业务逻辑从 handler 中抽离到独立的 service 层，使 handler 只负责 HTTP 请求/响应的解析和格式化。

## What Changes

- 新增 `internal/service/mobile_service.go`，实现 `MobileService` 结构体，封装所有移动端业务逻辑
- 新增 `internal/service/task_manager.go`，实现 `TaskManager` 结构体，封装任务 CRUD 和状态管理
- 新增 `internal/service/ws_hub.go`，实现 `WSHub` 结构体，封装 WebSocket 连接管理和广播
- 修改 `internal/server/mobile_api.go`，将 handler 瘦身为仅做 HTTP 解析/响应，委托给 `MobileService`
- 修改 `internal/server/mobile_ws.go`，将 WebSocket 逻辑委托给 `WSHub`
- 修改 `internal/server/server.go`，在 `NewServer` 中创建并注入 `MobileService`

## Impact

- Affected specs: `implement-mobile-backend-api`（API 行为不变，仅内部重构）
- Affected code:
  - `internal/server/mobile_api.go` — handler 瘦身
  - `internal/server/mobile_ws.go` — handler 瘦身
  - `internal/server/server.go` — 注入 MobileService
  - `internal/service/mobile_service.go`（新建）— 移动端业务逻辑
  - `internal/service/task_manager.go`（新建）— 任务管理
  - `internal/service/ws_hub.go`（新建）— WebSocket 管理

## ADDED Requirements

### Requirement: MobileService 业务逻辑层

系统 SHALL 提供 `MobileService` 结构体，封装所有移动端 API 的业务逻辑，handler 仅负责 HTTP 请求解析和响应格式化。

#### Scenario: 文件列表业务逻辑
- **WHEN** handler 调用 `mobileService.ListFiles(queryPath)`
- **THEN** 返回文件信息列表和 nil error（或相应错误）

#### Scenario: 文件删除业务逻辑
- **WHEN** handler 调用 `mobileService.DeleteFile(queryPath)`
- **THEN** 执行删除并返回 nil（或相应错误）

#### Scenario: 文件内容读取业务逻辑
- **WHEN** handler 调用 `mobileService.ReadFileContent(queryPath)`
- **THEN** 返回文件内容结构体和 nil error（或相应错误）

#### Scenario: WebDAV 连接测试业务逻辑
- **WHEN** handler 调用 `mobileService.TestWebDAV(url, username, password)`
- **THEN** 返回测试结果和 nil error（或相应错误）

### Requirement: TaskManager 任务管理

系统 SHALL 提供 `TaskManager` 结构体，封装所有任务的 CRUD 操作，消除包级全局变量。

#### Scenario: 创建任务
- **WHEN** 调用 `taskManager.Create(taskType, sourcePath)`
- **THEN** 创建并返回新任务，状态为 `queued`

#### Scenario: 获取任务列表
- **WHEN** 调用 `taskManager.List()`
- **THEN** 返回所有任务列表

#### Scenario: 取消任务
- **WHEN** 调用 `taskManager.Cancel(id)`
- **THEN** 对应任务状态更新为 `cancelled`

#### Scenario: 重试任务
- **WHEN** 调用 `taskManager.Retry(id)`
- **THEN** 对应任务状态重置为 `queued`

### Requirement: WSHub WebSocket 管理

系统 SHALL 提供 `WSHub` 结构体，封装 WebSocket 连接管理和消息广播，消除包级全局变量。

#### Scenario: 客户端注册
- **WHEN** 新 WebSocket 连接建立
- **THEN** 客户端被注册到 hub，接收 `server:status` 事件

#### Scenario: 心跳处理
- **WHEN** 客户端发送 `{"type":"ping"}`
- **THEN** hub 回复 `{"type":"pong"}`

#### Scenario: 消息广播
- **WHEN** 调用 `wsHub.Broadcast(msgType, data)`
- **THEN** 所有已连接客户端收到消息

#### Scenario: 客户端断开
- **WHEN** 客户端断开连接
- **THEN** hub 清理该客户端资源

### Requirement: Handler 瘦身

所有移动端 HTTP handler SHALL 仅负责：
1. 解析 HTTP 请求参数
2. 调用 `MobileService` 方法
3. 将结果格式化为 HTTP 响应

handler SHALL NOT 包含任何业务逻辑（文件系统操作、状态管理、网络请求等）。

### Requirement: 依赖注入

`MobileService` SHALL 通过 `Server.NewServer()` 构造函数注入到 `Server` 中，`Server` 结构体持有 `MobileService` 引用而非直接实现业务逻辑。

## MODIFIED Requirements

### Requirement: Server 结构体

`Server` 结构体 SHALL 新增 `mobileService *service.MobileService` 字段，移除对包级全局变量（tasks、wsClients）的依赖。

## REMOVED Requirements

### Requirement: 包级全局变量
**Reason**: 任务和 WebSocket 客户端状态迁移到 `TaskManager` 和 `WSHub` 实例中
**Migration**: `tasks`/`tasksMu` → `TaskManager` 内部字段；`wsClients`/`wsClientsMu` → `WSHub` 内部字段
