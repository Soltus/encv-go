# 后端 Go 测试计划

## 现状

- **已有测试**：`internal/v2/` 下有部分加密核心库的测试和基准测试（`block_v2_test.go`、`aes_v2_test.go`、`seek_regression_test.go` 等）
- **无测试**：`internal/service/`、`internal/server/`、`internal/admin/logic/openlist/`、`internal/webdav/` — 这些是桌面端+移动端共享的服务层
- **测试框架**：Go 标准 `testing` 包 + `github.com/stretchr/testify`（需确认是否已安装）

## 架构特点

项目同时支持**桌面端**和**移动端**两种运行模式：

| 组件 | 桌面端 | 移动端 |
|------|--------|--------|
| HTTP Server | ✅ 浏览器访问 | ✅ Capacitor WebView |
| WebSocket | ✅ 前端实时更新 | ✅ 前端实时更新 |
| MobileService | ❌ 不使用 | ✅ 文件管理/加密任务 |
| TaskManager | ❌ 不使用 | ✅ 加密/解密队列 |
| WebDAV | ✅ 远程挂载 | ✅ 远程挂载 |
| OpenList | ✅ 第三方网盘集成 | ✅ 第三方网盘集成 |
| V2 加密核心 | ✅ 桌面端加密/解密 | ✅ 移动端加密/解密 |

## 测试分层

### Layer 1：纯逻辑单元测试（无 IO 依赖）

这些测试不需要文件系统、网络或数据库，最容易编写和维护。

| 测试目标 | 文件 | 测试内容 |
|----------|------|----------|
| 错误类型 | `service/mobile_service.go` | `ForbiddenError`/`NotFoundError`/`BadRequestError` 等错误类型的 `Error()` 方法和类型断言 |
| WebSocket 消息序列化 | `service/ws_hub.go` | `WSMessage` 的 JSON marshal/unmarshal，`Broadcast` 消息格式正确性 |
| Task 状态机 | `service/task_manager.go` | Task 从 `queued` → `processing` → `completed`/`failed` 的状态转换逻辑 |
| OpenList URL 构建 | `admin/logic/openlist/multi_openlist.go` | `OpenListGetFileURL` 的请求体构建、签名计算逻辑 |
| OpenList Token 管理 | `admin/logic/openlist/token_manager.go` | Token 刷新、过期检测逻辑 |
| 路径安全检查 | `utils/guard.go` | 路径遍历攻击防护（`../../../etc/passwd` 等） |
| MIME 类型判断 | `utils/mime.go` | 文件扩展名 → MIME 类型映射 |
| 配置解析 | `config/config.go` | 配置文件解析、默认值、环境变量覆盖 |

### Layer 2：文件系统依赖的单元测试

使用 `t.TempDir()` 创建临时目录，测试结束后自动清理。

| 测试目标 | 文件 | 测试内容 |
|----------|------|----------|
| ListFiles | `service/mobile_service.go` | 列出目录内容、IsEncrypted 检测、权限错误、路径不存在 |
| 加密文件检测 | `v2/container/detector/` | `DetectContainer()` 对各种文件类型的识别（.encv 容器、普通文件、损坏容器） |
| 加密/解密任务 | `service/task_manager.go` | `Create()` → `processEncrypt()` → `completed` 全流程（使用临时文件） |
| WebDAV 文件系统 | `webdav/fs_v2.go` | `OpenFile`/`Stat`/`ReadDir` 对加密容器的透明解密访问 |
| 解密预览 | `service/decrypt_preview.go` | 文本/图片/PDF 预览内容提取 |

### Layer 3：HTTP API 集成测试

使用 `httptest.NewServer` 启动真实 HTTP 服务器，测试完整的请求/响应流程。

| 测试目标 | 文件 | 测试内容 |
|----------|------|----------|
| Mobile API | `server/mobile_api.go` | `/api/files?path=xxx` 返回正确 JSON、404/403 错误响应 |
| Health Check | `server/mobile_api.go` | `/api/health` 返回 `{"status":"ok"}` |
| WebSocket 连接 | `server/mobile_ws.go` | WS 连接/断开、`task:created`/`task:completed` 事件广播 |
| Server Config API | `server/server_config_api.go` | 配置读写 API |
| 桌面端路由 | `server/server_handle.go` | 加密文件内容服务、静态文件服务 |
| OpenList 代理 | `admin/logic/openlist/proxy_ghttp.go` | 代理请求转发、认证头注入 |

### Layer 4：OpenList 集成测试

Mock OpenList API 服务器，测试与第三方网盘的交互。

| 测试目标 | 测试内容 |
|----------|----------|
| 文件 URL 获取 | Mock `/api/fs/link` 响应，验证 URL 解析和 header 传递 |
| Token 认证 | 验证 HMAC 签名计算和请求头格式 |
| 多站点路由 | 测试 `multi_openlist.go` 的多 OpenList 实例路由逻辑 |
| 错误处理 | OpenList 返回非 200、超时、空响应等异常场景 |

## 实施步骤

### Step 1：安装测试依赖

```bash
cd /workspace
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/mock
go get github.com/stretchr/testify/suite
```

### Step 2：创建测试辅助工具

`internal/testutil/testutil.go`：
- `NewTestServer(t, cfg)` — 创建测试用 HTTP 服务器（含 MobileService）
- `NewTempServingDir(t)` — 创建临时服务目录和测试文件
- `NewMockWSHub()` — Mock WSHub 接口（用于 TaskManager 测试）
- `NewMockOpenListServer(handler)` — Mock OpenList API 服务器

### Step 3：Layer 1 纯逻辑测试（优先实施）

```
internal/service/
  mobile_service_errors_test.go    — 错误类型断言
  ws_hub_test.go                   — Broadcast 消息格式
  task_manager_state_test.go       — Task 状态机

internal/admin/logic/openlist/
  token_manager_test.go            — Token 管理
  multi_openlist_test.go           — URL 构建/签名

internal/utils/
  guard_test.go                    — 路径遍历防护
  mime_test.go                     — MIME 类型
```

### Step 4：Layer 2 文件系统测试

```
internal/service/
  mobile_service_listfiles_test.go — ListFiles + IsEncrypted
  task_manager_process_test.go     — 加密/解密全流程
  decrypt_preview_test.go          — 预览内容提取

internal/webdav/
  fs_v2_test.go                    — WebDAV 文件系统操作
```

### Step 5：Layer 3 HTTP API 测试

```
internal/server/
  mobile_api_test.go               — API 端点测试
  mobile_ws_test.go                — WebSocket 集成测试
  server_handle_test.go            — 桌面端路由测试
```

### Step 6：Layer 4 OpenList 集成测试

```
internal/admin/logic/openlist/
  proxy_test.go                    — 代理请求测试
  integration_test.go              — Mock OpenList 服务器测试
```

### Step 7：CI 集成

在 `.github/workflows/` 中添加 Go 测试步骤：
```yaml
- name: Run Go tests
  run: go test ./internal/... -v -race -coverprofile=coverage.out
```

## 关键设计决策

1. **WSHub 接口化**：当前 `TaskManager` 直接持有 `*WSHub`，测试时需要 Mock。应抽取 `Broadcaster` 接口：
   ```go
   type Broadcaster interface {
       Broadcast(msgType string, data interface{})
   }
   ```
   `TaskManager` 改为依赖 `Broadcaster` 接口，`WSHub` 实现该接口。

2. **MobileService 依赖注入**：当前 `NewMobileService` 内部创建 `WSHub` 和 `TaskManager`，测试时需要注入 Mock。应改为接受接口参数。

3. **OpenList HTTP Client 接口化**：当前直接使用 `http.DefaultClient`，测试时需要 Mock HTTP 响应。应抽取 `HTTPDoer` 接口或使用 `httptest.Server`。

4. **桌面端/移动端兼容**：所有测试应能在两种模式下运行。`MobileService` 相关测试仅在移动端模式下有意义，但 API 层测试（Health、WebSocket）对两种模式都适用。

5. **测试数据隔离**：使用 `t.TempDir()` 创建临时目录，避免测试间相互影响。加密/解密测试使用程序生成的测试文件而非真实用户文件。
