# ENCV-Mobile 后端 API 实现 Spec

## Why

encv-mobile 前端（Ionic + Vue）已完整实现，包含 Files/Player/Tasks/WebDAV/Settings 五个页面及 WebSocket 实时通讯层。但 Go 后端缺少前端所需的所有 REST API 端点和 WebSocket 端点，导致 App 显示"后端未连接"。需要在 `internal/server` 中实现完整的移动端后端 API。

## What Changes

- 在 `internal/server/server.go` 的路由注册中新增移动端 API 路由
- 新增 `internal/server/mobile_api.go` 文件，实现所有移动端需要的 HTTP API handler：
  - `GET /health` — 健康检查（兼容前端的 `checkServerStatus()`）
  - `GET /api/files?path=xxx` — JSON 格式文件列表（替代当前 HTML 输出）
  - `DELETE /api/files?path=xxx` — 删除文件
  - `GET /stream?path=xxx` — 兼容前端的流式播放接口（现有 `?file=` 的别名）
  - `GET /api/tasks` — 任务列表
  - `POST /api/tasks` — 创建任务
  - `POST /api/tasks/{id}/cancel` — 取消任务
  - `POST /api/tasks/{id}/retry` — 重试任务
  - `POST /api/webdav/test` — 测试 WebDAV 连接
- 新增 `internal/server/mobile_ws.go` 文件，实现 WebSocket 端点：
  - `GET /ws` — WebSocket 连接，支持 ping/pong 心跳和事件推送

## Impact

- Affected specs: 无已有 spec 受影响
- Affected code:
  - `internal/server/server.go` — 新增路由注册
  - `internal/server/mobile_api.go`（新建）— 所有移动端 HTTP API handler
  - `internal/server/mobile_ws.go`（新建）— WebSocket handler

## ADDED Requirements

### Requirement: 健康检查接口

系统 SHALL 提供 `GET /health` 接口，返回 `{"status":"ok"}` JSON 响应，HTTP 200。

#### Scenario: 前端检查服务器状态
- **WHEN** 前端调用 `GET /health`
- **THEN** 返回 `{"status":"ok"}`，HTTP status 200

### Requirement: JSON 文件列表接口

系统 SHALL 提供 `GET /api/files?path=xxx` 接口，返回指定目录的文件列表为 JSON 格式。

#### Scenario: 获取根目录文件列表
- **WHEN** 前端调用 `GET /api/files?path=/`
- **THEN** 返回 `{"files":[{"name":"xxx","path":"/xxx","isDirectory":false,"size":1024,"modified":"2025-01-01T00:00:00Z"}]}`，隐藏以 `.` 开头的文件

#### Scenario: 获取子目录文件列表
- **WHEN** 前端调用 `GET /api/files?path=/subdir`
- **THEN** 返回该子目录的文件 JSON 列表

#### Scenario: 路径遍历攻击防护
- **WHEN** 请求路径包含 `..` 或尝试访问服务目录之外的位置
- **THEN** 返回 HTTP 403 Forbidden

### Requirement: 删除文件接口

系统 SHALL 提供 `DELETE /api/files?path=xxx` 接口，用于删除指定文件。

#### Scenario: 删除成功
- **WHEN** 前端调用 `DELETE /api/files?path=/test.txt` 且文件存在
- **THEN** 文件被删除，返回 HTTP 200

#### Scenario: 文件不存在
- **WHEN** 目标文件不存在
- **THEN** 返回 HTTP 404

### Requirement: 流式播放接口兼容

系统 SHALL 支持 `GET /stream?path=xxx` 查询参数格式，作为现有 `?file=` 格式的别名。

#### Scenario: 通过 path 参数播放
- **WHEN** 前端调用 `GET /stream?path=/video.mp4`
- **THEN** 行为与 `GET /stream?file=/video.mp4` 完全一致

### Requirement: 任务管理接口

系统 SHALL 提供任务 CRUD 接口的内存级实现（后续可对接真实任务系统）。

#### Scenario: 获取任务列表
- **WHEN** 前端调用 `GET /api/tasks`
- **THEN** 返回 `{"tasks":[...]}`，包含 id/type/sourcePath/status/progress/error/createdAt 字段

#### Scenario: 创建任务
- **WHEN** 前端 POST `{"type":"encrypt","sourcePath":"/test.mp4"}` 到 `/api/tasks`
- **THEN** 返回新创建的任务对象，状态为 `queued`

#### Scenario: 取消任务
- **WHEN** 前端调用 `POST /api/tasks/{id}/cancel`
- **THEN** 对应任务状态更新为 `cancelled`

#### Scenario: 重试任务
- **WHEN** 前端调用 `POST /api/tasks/{id}/retry`
- **THEN** 对应任务状态重置为 `queued`

### Requirement: WebDAV 连接测试接口

系统 SHALL 提供 `POST /api/webdav/test` 接口，测试 WebDAV 配置是否可达。

#### Scenario: WebDAV 测试
- **WHEN** 前端 POST WebDAV 配置到 `/api/webdav/test`
- **THEN** 尝试连接并返回成功/失败状态

### Requirement: WebSocket 实时通讯

系统 SHALL 提供 `GET /ws` WebSocket 端点，支持心跳和事件推送。

#### Scenario: WS 连接建立
- **WHEN** 前端连接 `ws://{host}/ws`
- **THEN** 连接建立成功，服务端推送 `server:status` 事件 `{online:true}`

#### Scenario: 心跳机制
- **WHEN** 客户端发送 `{"type":"ping"}`
- **THEN** 服务端回复 `{"type":"pong"}`

#### Scenario: WS 断开
- **WHEN** 客户端断开连接
- **THEN** 服务端清理资源

### Requirement: 本地存储 sqlite 驱动 SHALL 使用 glebarez/sqlite

如 encv-go 端未来需要引入本地 sqlite 持久化（任务队列、本地缓存、用户配置等），SHALL 选用 `github.com/glebarez/sqlite`（pure-Go，基于 `modernc.org/sqlite` 的 GORM Dialector），**禁止**引入 `github.com/mattn/go-sqlite3`。

#### Scenario: 选型一致性
- **WHEN** 任何 `go get` 拉入 sqlite 相关依赖
- **THEN** import path 必须是 `github.com/glebarez/sqlite`；`go.mod` 不应出现 `mattn/go-sqlite3` 任何形式（直接或间接）
- **AND** `gorm.Open(sqlite.Open(...), &gorm.Config{})` API 与 `gorm.io/driver/sqlite` 完全一致，无需业务层改写

#### Scenario: 交叉编译验证
- **WHEN** 在 Linux/macOS/Windows 任一平台为 Android arm64 交叉编译 encv-go
- **THEN** `CGO_ENABLED=0 go build ./...` 应当通过；若失败说明违规引入了 CGO 驱动（如 mattn）

#### Scenario: 性能预算
- **GIVEN** OpenList 元数据场景为读多写少
- **THEN** glebarez 写性能 20-30% 衰减可接受；读性能与 mattn 基本持平

#### Scenario: 与 Hi-Sillot/OpenList fork 选型对齐
- **GIVEN** fork 自身已切到 `github.com/glebarez/sqlite`（见 `.trae/documents/openlist-aar-sqlite-cgo-multi-solution.md` §三 B1）
- **THEN** encv-go 端保持同款驱动，未来 gomobile 工具链复用、跨 ABI 打包、容器化部署均无 CGO 阻塞
