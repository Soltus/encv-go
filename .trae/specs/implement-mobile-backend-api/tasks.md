# Tasks

- [x] Task 1: 新增移动端 HTTP API handlers（mobile_api.go）
  - [x] 实现 `handleHealth` — `GET /health` 健康检查
  - [x] 实现 `handleListFilesAPI` — `GET /api/files?path=` JSON 文件列表
  - [x] 实现 `handleDeleteFile` — `DELETE /api/files?path=` 删除文件
  - [x] 修改 `handleStreamRequest` — 兼容 `?path=` 查询参数
  - [x] 实现 `handleGetTasks` / `handleCreateTask` / `handleCancelTask` / `handleRetryTask` — 任务管理 API
  - [x] 实现 `handleTestWebDAV` — WebDAV 连接测试

- [x] Task 2: 新增 WebSocket handler（mobile_ws.go）
  - [x] 实现 WebSocket 升级处理器，支持 `/ws` 端点
  - [x] 实现 ping/pong 心跳处理
  - [x] 实现连接建立时推送 server:status 事件
  - [x] 实现连接清理和资源释放

- [x] Task 3: 注册新路由到 Server 启动流程
  - [x] 在 `server.go` 的 `Start()` 方法中注册所有新增路由
  - [x] 确保 CORS 中间件覆盖新路由

- [x] Task 4: 构建验证
  - [x] 运行 `go build ./cmd/encv/` 确保编译通过
