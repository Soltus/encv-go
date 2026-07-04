# 统一 HTTP 框架为 Gin 重构 Spec

## Why

当前项目混用两套 HTTP 框架：GoFrame (ghttp) 用于 admin 服务（登录认证、反向代理、OpenList 代理、文件管理），net/http 标准库用于后端 API（移动端 API、文件服务、WebSocket、WebDAV）。两套框架各自有独立的中间件（CORS、Auth、Logging）、独立的启动逻辑（`StartGfServerWithRetry` vs `StartHttpHandlerWithRetry`）、独立的路由注册方式，导致：
1. 移动端 API（net/http）和 admin API（GoFrame）风格完全不同，维护成本高
2. 两套 CORS、Auth 中间件逻辑重复但实现不同
3. GoFrame 依赖链庞大（引入了 ORM、配置管理、模板引擎等不需要的功能），但实际只用了路由和中间件
4. 新增 API 时需要决定放哪个服务，增加心智负担

## 为什么选 Gin 而非 Fiber

### 决定性因素：net/http 兼容性

本项目有 **5 个关键依赖** 基于 `net/http` 标准接口：

| 依赖 | 使用位置 | Gin 兼容方式 | Fiber 兼容方式 |
|------|---------|-------------|---------------|
| `golang.org/x/net/webdav` | WebDAV 文件服务 | `gin.WrapH(handler)` 一行搞定 | `adaptor.HTTPHandler` + fasthttp↔net/http 转换，有性能损耗和边界问题 |
| `gorilla/websocket` | 日志 WebSocket 推送 | 直接使用，`ws_hub.go` 零改动 | 必须换库，ws_hub 需重写 |
| `net/http/httputil.ReverseProxy` | Admin 反向代理 | 直接使用，响应修改逻辑不变 | 需要 adaptor，响应修改逻辑复杂 |
| `http.ServeFile` | 静态文件服务 | `gin.WrapH` 或 `c.File()` | `c.SendFile()` 路径处理不同 |
| `http.Handler` 接口 | 中间件链 | 原生兼容 | 需要 adaptor 包装 |

### 其他考量

1. **渐进式迁移**：Gin 的 `gin.WrapH()` / `gin.WrapF()` 可以将现有 `net/http` handler 直接包裹使用，无需一次性重写所有 handler。Fiber 基于 fasthttp，所有 handler 签名必须改变，无法渐进迁移。
2. **社区生态**：Gin 是国内 Go 开发者使用最广泛的 Web 框架，文档、教程、问题解决方案丰富。本项目与 OpenList（AList fork）集成，AList 原本就用 Gin。
3. **生产稳定性**：本项目是局域网/移动端场景，Gin 的性能完全足够，Fiber 的极致性能优势在此场景下可忽略。
4. **长期维护**：Gin 的 `net/http` 兼容性意味着未来任何基于 `net/http` 的新库都能直接使用，Fiber 则需要每次做适配。

## What Changes

- **BREAKING**：移除 GoFrame (`gogf/gf/v2`) 依赖，admin 服务改用 Gin 实现
- 后端服务从 `net/http.ServeMux` 迁移到 Gin 路由（渐进式，旧 handler 可通过 `gin.WrapH/F` 临时兼容）
- 合并两个独立服务器（backend + admin）为单个 Gin 应用
- 统一中间件实现（CORS、Auth、Logging）
- 统一启动逻辑（端口递增 + 自检）
- 保留 `gorilla/websocket`（Gin 完全兼容）
- WebDAV 处理器通过 `gin.WrapH()` 直接桥接
- 移除 `internal/admin/` 目录，功能合并到 `internal/server/`
- 移除 `internal/register/server_start.go` 中的 `StartGfServerWithRetry`

## Impact

- Affected specs: refactor-mobile-backend-service
- Affected code:
  - `internal/admin/` — 整个目录重写/合并
  - `internal/server/` — 从 net/http 迁移到 Gin
  - `internal/middleware/` — 统一到 Gin 中间件
  - `internal/register/` — 简化启动逻辑
  - `cmd/encv/servers.go` — 启动逻辑调整
  - `go.mod` — 移除 GoFrame，添加 Gin

## 架构分析：当前双服务器结构

### Backend Server (net/http, 端口由 cfg.Server.Port 决定)
| 路由 | Handler | 用途 |
|------|---------|------|
| `/ping` | handlePing | 自检 |
| `/health` | handleHealth | 健康检查 |
| `/stream` | handleStreamRequest | 加密视频流 |
| `/api/config` | handleConfigAPI | 配置读写 |
| `/api/config/schema` | handleConfigSchemaAPI | 配置 schema |
| `/api/files` | handleMobileFiles | 文件列表 |
| `/api/file` | handleReadFileContent | 文件内容 |
| `/api/files/exists` | handleFileExistsAPI | 文件存在检查 |
| `/api/files/search` | handleSearchFilesAPI | 文件搜索 |
| `/api/tasks` | handleMobileTasks | 任务 CRUD |
| `/api/webdav/test` | handleTestWebDAV | WebDAV 测试 |
| `/api/remote/info` | handleRemoteInfo | 远端信息 |
| `/api/remote/openlist` | handleOpenlistSites | OpenList 站点 |
| `/api/permissions` | handlePermissions | 权限检查 |
| `/api/server/shutdown` | handleServerShutdown | 服务器关闭 |
| `/api/index/stats` | handleIndexStats | 索引统计 |
| `/api/index/rebuild` | handleIndexRebuild | 索引重建 |
| `/api/index/clear` | handleIndexClear | 索引清除 |
| `/api/stream/external` | handleStreamExternalFile | 外部文件流 |
| `/api/logs` | handleAPILogs | 日志 API |
| `/ws` | handleWebSocket | WebSocket |
| `/` | handleRequest | 文件服务（目录列表+静态文件） |
| WebDAV 路由 | goWebdav.Handler | WebDAV 服务 |

### Admin Server (GoFrame ghttp, 端口由 cfg.Admin.Port 决定)
| 路由 | Handler | 用途 |
|------|---------|------|
| `/login` GET/POST | handleLogin | 登录页面 |
| `/logout` ALL | 登出 | 登出 |
| `/admin/*` | GoFrame Bind (hello, file) | Admin API |
| `/admin/file/analyze` POST | file_v1_analyze | 文件分析 |
| `/admin/file/rename` POST | file_v1_rename | 文件重命名 |
| `/openlist/sites` GET | OpenList 站点列表 | OpenList 管理 |
| `/openlist/sites/{siteId}/*` | OpenList 代理 | OpenList 代理 |
| `/p/*` ALL | 反向代理到 Backend | 文件浏览代理 |
| `/p-api/*` ALL | 反向代理到 Backend | API 代理 |

### 中间件对比
| 功能 | Backend (net/http) | Admin (GoFrame) |
|------|-------------------|-----------------|
| CORS | `middleware.CorsMiddleware` | `admin/middleware.CORS` |
| Auth | `middleware.BasicAuth` (WebDAV) | `admin/middleware.AuthMiddleware` (JWT) |
| Logging | `middleware.LoggingMiddleware` | 无 |
| Config注入 | `middleware.WithConfig` | GoFrame 内置配置 |
| Response | 无 | `admin/middleware.Response` |

### GoFrame 依赖范围
- `internal/admin/admin.go` — 主入口
- `internal/admin/middleware/` — CORS、Auth、Response（3 个文件）
- `internal/admin/controller/file/` — 文件分析、重命名（4 个文件）
- `internal/admin/controller/hello/` — Hello API（3 个文件）
- `internal/admin/logic/auth/` — JWT 认证（2 个文件）
- `internal/admin/logic/openlist/` — OpenList 代理（5 个文件）
- `internal/admin/injector/` — HTML 注入（1 个文件）
- `internal/admin/routes/` — 路由常量（1 个文件）

### 移动端 (encv-mobile) 与后端 API 交互全景

移动端通过 **3 种方式** 与后端交互：HTTP API、Capacitor 插件（JNI）、WebSocket。

#### HTTP API 调用（src/api/encv.ts）

所有 HTTP 请求通过 `getApiBaseUrl()` 获取基础 URL（默认 `http://127.0.0.1:2025`），使用 `fetch` 发起请求。

| 前端函数 | HTTP 方法 | 后端路由 | 请求参数 | 响应格式 | 使用页面 |
|---------|----------|---------|---------|---------|---------|
| `listFiles(path)` | GET | `/api/files?path=` | path | `{files: FileItem[]}` | Files.vue |
| `checkBackendPermissions()` | GET | `/api/permissions` | - | `{storage: bool}` | Files.vue |
| `getFileStreamUrl(path)` | - (URL构造) | `/stream?path=` | path | 视频流 | StandalonePlayer.vue |
| `getExternalStreamUrl(path)` | - (URL构造) | `/api/stream/external?path=` | path | 视频流 | PlayerView (Lynx) |
| `checkServerStatus()` | GET | `/health` | - | `{online: bool}` | useServerStatus |
| `deleteFile(path)` | DELETE | `/api/files?path=` | path | - | Files.vue |
| `readFileContent(path)` | GET | `/api/file?path=` | path | `{name, content, size}` | FilePreview.vue |
| `getTasks()` | GET | `/api/tasks` | - | `{tasks: EncvTask[]}` | Tasks.vue |
| `createTask(type, src, tgt, pwd)` | POST | `/api/tasks` | JSON body | `EncvTask` | Files.vue |
| `cancelTask(id)` | POST | `/api/tasks/{id}/cancel` | - | - | Tasks.vue |
| `retryTask(id)` | POST | `/api/tasks/{id}/retry` | - | - | Tasks.vue |
| `testWebDAVConnection(config)` | POST | `/api/webdav/test` | JSON body | - | Remote.vue |
| `fetchConfig()` | GET | `/api/config` | - | `Record<string, any>` | Settings.vue |
| `updateConfig(config)` | PUT | `/api/config` | JSON body | - | Settings.vue |
| `fetchConfigSchema()` | GET | `/api/config/schema` | - | `Record<string, any>` | Settings.vue |
| `searchFiles(path, kw, rec)` | GET | `/api/files/search?` | query params | `{files: FileItem[]}` | Files.vue |
| `getIndexStats()` | GET | `/api/index/stats` | - | `IndexStats` | Settings.vue |
| `rebuildIndex()` | POST | `/api/index/rebuild` | - | - | Settings.vue |
| `clearIndex()` | POST | `/api/index/clear` | - | - | Settings.vue |
| `fetchRemoteInfo()` | GET | `/api/remote/info` | - | `RemoteInfo` | Remote.vue |
| `addOpenlistSite(...)` | POST | `/api/remote/openlist` | JSON body | - | Remote.vue |
| `updateOpenlistSite(...)` | PUT | `/api/remote/openlist/{id}` | JSON body | - | Remote.vue |
| `deleteOpenlistSite(id)` | DELETE | `/api/remote/openlist/{id}` | - | - | Remote.vue |
| `checkFileExists(path)` | GET | `/api/files/exists?path=` | path | `{exists: bool}` | Files.vue |

#### Capacitor 插件调用（src/plugins/GoProcess.ts）

这些调用不经过 HTTP，而是通过 Capacitor JNI 桥接直接调用 Kotlin 原生方法：

| 前端函数 | Kotlin 方法 | 用途 | 使用页面 |
|---------|-----------|------|---------|
| `restartBackend()` | `GoProcessPlugin.restart()` | 重启 Go 后端进程 | Settings.vue |
| `stopBackend()` | `GoProcessPlugin.stop()` | 停止 Go 后端进程 | Settings.vue |
| `getBackendStatus()` | `GoProcessPlugin.getStatus()` | 获取后端运行状态 | useServerStatus |
| `requestNotificationPermission()` | `GoProcessPlugin.requestNotificationPermission()` | 请求通知权限 | Settings.vue |
| `requestStoragePermission()` | `GoProcessPlugin.requestStoragePermission()` | 请求存储权限 | Files.vue |
| `checkPermissions()` | `GoProcessPlugin.checkPermissions()` | 检查权限状态 | Settings.vue |
| `isStandaloneMode()` | `GoProcessPlugin.isStandaloneMode()` | 检查是否独立模式 | useServerStatus |
| `getIntentFileInfo()` | `GoProcessPlugin.getIntentFileInfo()` | 获取外部打开的文件信息 | Tabs.vue |
| `openInPlayer(path, name, mime)` | `GoProcessPlugin.openInPlayer()` | 在 Lynx 播放器中打开文件 | Files.vue |
| `openPlayerHome()` | `GoProcessPlugin.openPlayerHome()` | 打开播放器首页 | HomePage.vue |

#### WebSocket 连接（src/composables/useWebSocket.ts）

- 连接地址：`ws://127.0.0.1:2025/ws`
- 心跳机制：每 30s 发送 `{type: "ping"}`，10s 内未收到 `{type: "pong"}` 则重连
- 断线重连：指数退避，最大 30s
- 消息格式：`{type: string, data: any}`
- 事件分发：通过 `eventBus.emit(msg.type, msg.data)` 分发到各组件
- 使用页面：DevLogs.vue（接收日志推送）、useServerStatus（连接状态监控）

#### Lynx 播放器与后端交互

Lynx 播放器运行在独立的 `PlayerActivityLynx` 中，通过以下方式与后端交互：

1. **视频流**：`/stream?path=` 和 `/api/stream/external?path=` — MPV 直接请求 HTTP 流
2. **NativeModules**：`globalThis.NativeModules.MpvPlayerModule` — 通过 Lynx 桥接调用原生 MPV 播放
3. **GlobalEventEmitter**：`lynx.getJSModule('GlobalEventEmitter')` — 接收 MPV 状态变化和位置更新事件
4. **initData**：`lynx.__globalProps` — 从 Kotlin 端接收文件路径、文件名、MIME 类型等初始数据

#### 移动端对框架迁移的影响

1. **HTTP API 兼容性**：移动端所有 HTTP 调用都是标准 `fetch`，只要后端路由路径和响应格式不变，前端零改动
2. **CORS 重要性**：移动端 WebView 发起的 HTTP 请求需要 CORS 支持，Gin CORS 中间件必须正确配置
3. **WebSocket 兼容性**：Gin 完全兼容 `gorilla/websocket`，WebSocket 连接零改动
4. **单端口简化**：合并为单服务器后，移动端不再需要区分 backend 端口和 admin 端口，`getApiBaseUrl()` 统一指向一个端口
5. **移动端可承载 Admin**：之前 GoFrame 太重（ORM、模板引擎、配置管理等大量不需要的依赖），移动端 `ENCV_MOBILE=1` 时跳过 admin 服务器。Gin 极轻量（零额外依赖），admin 路由只是几个 handler + JWT 中间件，移动端完全可以承载。这意味着移动端用户也可以通过浏览器访问 admin UI、使用 OpenList 代理、登录认证等
- `internal/register/server_start.go` — `StartGfServerWithRetry`

## ADDED Requirements

### Requirement: 统一 Gin 应用

系统 SHALL 将当前的双服务器架构（GoFrame admin + net/http backend）合并为单个 Gin 应用。

#### Scenario: 单端口服务
- **WHEN** 启动 `encv start` 命令
- **THEN** 系统在单个端口上启动 Gin 应用，同时提供文件服务、API、Admin 和代理功能
- **AND** 端口由 `cfg.Server.Port` 决定，`cfg.Admin.Port` 配置项不再使用

#### Scenario: 移动端模式
- **WHEN** 环境变量 `ENCV_MOBILE=1`
- **THEN** 系统注册所有路由（包括 admin 路由），移动端用户可通过同一端口访问 admin UI、OpenList 代理、登录认证等功能
- **AND** 之前 GoFrame 太重导致移动端必须跳过 admin，Gin 极轻量不再有此限制

### Requirement: Gin 路由注册

系统 SHALL 使用 Gin 路由 API 注册所有端点，替代 net/http ServeMux 和 GoFrame RouterGroup。

#### Scenario: API 路由
- **WHEN** 注册 `/api/*` 路由
- **THEN** 使用 `r.Group("/api")` 创建路由组，应用 CORS 和 Config 中间件

#### Scenario: Admin 路由
- **WHEN** 注册 `/admin/*` 路由
- **THEN** 使用 `r.Group("/admin")` 创建路由组，应用 JWT Auth 中间件

#### Scenario: 文件服务路由
- **WHEN** 注册 `/` 兜底路由
- **THEN** 使用 `r.NoRoute()` 处理文件浏览和静态文件服务

### Requirement: 渐进式迁移

系统 SHALL 支持渐进式从 net/http handler 迁移到 Gin handler。

#### Scenario: 旧 handler 兼容
- **WHEN** 暂时不想重写某个 net/http handler
- **THEN** 可以通过 `gin.WrapH()` 或 `gin.WrapF()` 直接包裹使用

#### Scenario: 新 handler 风格
- **WHEN** 编写新 handler
- **THEN** 使用 Gin 风格 `func(c *gin.Context)`，利用 `c.JSON()`、`c.Bind()` 等 API

### Requirement: 统一中间件

系统 SHALL 使用 Gin 中间件机制统一所有 HTTP 中间件。

#### Scenario: CORS 中间件
- **WHEN** 请求到达
- **THEN** 使用 Gin CORS 中间件处理跨域，替代两套自定义 CORS 实现

#### Scenario: JWT Auth 中间件
- **WHEN** 访问 admin 路由
- **THEN** 使用 Gin 中间件检查 JWT token，未认证重定向到 `/login`

#### Scenario: Basic Auth 中间件
- **WHEN** 访问 WebDAV 路由
- **THEN** 使用 `gin.BasicAuth()` 中间件

#### Scenario: Logging 中间件
- **WHEN** 请求处理完成
- **THEN** 使用 Gin Logger 中间件记录请求信息

### Requirement: WebSocket 保留

系统 SHALL 保留 `gorilla/websocket` 依赖，通过 Gin 路由处理 WebSocket 升级。

#### Scenario: 日志 WebSocket
- **WHEN** 客户端连接 `/ws`
- **THEN** 通过 `gin.WrapF()` 包裹现有 WebSocket handler，升级为 WebSocket 连接

### Requirement: WebDAV 桥接

系统 SHALL 通过 `gin.WrapH()` 将 golang.org/x/net/webdav Handler 桥接到 Gin 路由。

#### Scenario: WebDAV 请求
- **WHEN** 请求匹配 WebDAV 路由前缀
- **THEN** 通过 `gin.WrapH()` 将请求转发给 webdav.Handler

### Requirement: HTML 模板渲染

系统 SHALL 使用 Gin 的 `gin.HTML()` + `html/template` 替代手动 template.Execute。

#### Scenario: 目录列表
- **WHEN** 浏览目录
- **THEN** 使用 `c.HTML()` 渲染目录列表页面

#### Scenario: 登录页面
- **WHEN** 访问 `/login`
- **THEN** 使用 `c.HTML()` 渲染登录页面

### Requirement: 反向代理

系统 SHALL 保留 `httputil.ReverseProxy`，通过 Gin 路由处理代理请求。

#### Scenario: 文件代理
- **WHEN** 访问 `/p/*` 路由
- **THEN** 使用 `httputil.ReverseProxy` 转发请求，响应修改逻辑不变

### Requirement: 启动逻辑统一

系统 SHALL 使用 `net/http.Server` + `gin.Engine` 替代两套启动逻辑，保留端口递增和自检功能。

#### Scenario: 端口递增
- **WHEN** 默认端口被占用
- **THEN** 自动尝试下一个端口

#### Scenario: 自检
- **WHEN** 服务器启动后
- **THEN** 通过 `/ping` 端点验证实例 ID

### Requirement: 移动端 Admin 可用

系统 SHALL 在移动端模式下提供完整的 admin 功能，不再因框架重量而跳过。

#### Scenario: 移动端访问 Admin UI
- **WHEN** 移动端用户在浏览器中访问 `http://127.0.0.1:{port}/admin/`
- **THEN** 系统正常渲染 admin 页面，用户可登录、管理文件、使用 OpenList 代理

#### Scenario: 移动端 OpenList 代理
- **WHEN** 移动端用户访问 `/openlist/sites/{siteId}/*` 路由
- **THEN** 系统正常代理 OpenList 请求，与桌面端行为一致

#### Scenario: 移动端登录认证
- **WHEN** 移动端用户访问需要认证的路由
- **THEN** 系统重定向到 `/login` 页面，JWT 认证流程与桌面端一致

## MODIFIED Requirements

### Requirement: 服务器配置
原：`cfg.Server.Port` 用于 backend，`cfg.Admin.Port` 用于 admin
改：统一使用 `cfg.Server.Port`，`cfg.Admin.Port` 配置项移除（移动端和桌面端共用单端口，不再需要区分）

### Requirement: OpenList 代理
原：通过 GoFrame RouterGroup 注册
改：通过 Gin 路由组注册，逻辑不变

## REMOVED Requirements

### Requirement: GoFrame 依赖
**Reason**: 统一到 Gin，GoFrame 不再需要
**Migration**: 移除 `gogf/gf/v2` 依赖，删除 `internal/admin/` 目录

### Requirement: 双服务器架构
**Reason**: 合并为单服务器
**Migration**: 删除 `StartGfServerWithRetry`，简化 `cmd/encv/servers.go`

### Requirement: 移动端跳过 Admin 逻辑
**Reason**: GoFrame 太重导致移动端必须跳过 admin 服务器，Gin 极轻量不再需要此限制
**Migration**: 移除 `ENCV_MOBILE=1` 时跳过 admin 服务器启动的逻辑，移动端统一注册所有路由

### Requirement: cfg.Admin.Port 配置项
**Reason**: 单端口架构不再需要区分 backend 和 admin 端口
**Migration**: 移除 `cfg.Admin.Port` 配置项，统一使用 `cfg.Server.Port`

## 风险评估

### 低风险
- API 路由迁移：Gin 完全兼容 `net/http`，现有 handler 可通过 `gin.WrapH/F` 临时兼容，无需一次性重写
- 中间件统一：Gin 社区有成熟的 CORS、Logger、BasicAuth 中间件
- WebSocket：`gorilla/websocket` 与 Gin 完全兼容，零改动
- WebDAV：`gin.WrapH()` 一行搞定

### 中风险
- HTML 注入（HookAfterServe）：GoFrame 的 `BindHookHandler` 用于在响应后注入工具栏 HTML，Gin 需要用中间件实现类似功能
- OpenList 代理：`proxy_ghttp.go` 大量使用 `ghttp.Request`，需要重写为 `*gin.Context`
- 登录页面：GoFrame 的 `r.Parse(&req)` 需要改为 Gin 的 `c.ShouldBind()`

### 关键优势（相比 Fiber 方案）
- **渐进式迁移**：不需要一次性重写所有 handler，可以先用 `gin.WrapH/F` 兼容，逐步迁移
- **零风险第三方库兼容**：WebDAV、WebSocket、ReverseProxy 无需任何适配
- **社区支持**：Gin 在国内开发者中广泛使用，问题解决方案丰富
