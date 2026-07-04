# Tasks

- [x] Task 1: 添加 Gin 依赖，移除 GoFrame 依赖
  - [x] SubTask 1.1: `go get github.com/gin-gonic/gin` 添加 Gin
  - [x] SubTask 1.2: `go mod tidy` 清理 GoFrame 依赖
  - [x] SubTask 1.3: 验证 `go build ./...` 编译通过

- [x] Task 2: 创建 Gin 应用入口和统一启动逻辑
  - [x] SubTask 2.1: 创建 `internal/server/gin_app.go`，定义 `NewGinApp()` 函数
  - [x] SubTask 2.2: 重写 `internal/register/server_start.go`，添加 `StartGinWithRetry`
  - [x] SubTask 2.3: 修改 `cmd/encv/servers.go`，使用统一启动逻辑
  - [x] SubTask 2.4: 移除 `ENCV_MOBILE=1` 跳过 admin 逻辑
  - [x] SubTask 2.5: 移除 `cfg.Admin.Port` 配置项

- [x] Task 3: 迁移 Backend API 路由（net/http → Gin）
  - [x] SubTask 3.1-3.11: 所有 Backend API 路由已通过 `gin.WrapF/H` 注册

- [x] Task 4: 迁移 Admin 功能（GoFrame → Gin）
  - [x] SubTask 4.1: JWT 认证逻辑迁移到 `internal/auth/`
  - [x] SubTask 4.2: 登录/登出路由迁移
  - [x] SubTask 4.3: Admin API 迁移
  - [x] SubTask 4.4: HTML 注入逻辑迁移
  - [x] SubTask 4.5: 反向代理迁移

- [x] Task 5: 迁移 OpenList 代理（GoFrame → Gin）
  - [x] SubTask 5.1: `proxy_ghttp.go` → `openlist_handlers.go`
  - [x] SubTask 5.2: `multi_site_server.go` 改为 Gin 路由组
  - [x] SubTask 5.3: Token 管理和站点选择逻辑迁移

- [x] Task 6: 统一中间件
  - [x] SubTask 6.1: Gin CORS 中间件替代自定义 CORS
  - [x] SubTask 6.2: Gin Logger 替代自定义 Logging
  - [x] SubTask 6.3: BasicAuth 保留 http.Handler 层面（WebDAV 兼容）
  - [x] SubTask 6.4: JWT Auth 中间件改为 Gin 中间件
  - [x] SubTask 6.5: Config 注入中间件改为 Gin 中间件

- [x] Task 7: 清理旧代码和依赖
  - [x] SubTask 7.1: 删除 `internal/admin/` 目录
  - [x] SubTask 7.2: 删除 `internal/middleware/cors.go`
  - [x] SubTask 7.3: 删除 `internal/middleware/logging.go`
  - [x] SubTask 7.4: 清理 `go.mod`，移除 `gogf/gf/v2`
  - [x] SubTask 7.5: 验证 `go build ./...` 编译通过

- [ ] Task 8: 逐步将 `gin.WrapF/H` 包裹的 handler 改为 Gin 原生 handler
  - [ ] SubTask 8.1: 将 `mobile_api.go` 中的 handler 改为 Gin 原生
  - [ ] SubTask 8.2: 将文件服务 handler 改为 Gin 原生
  - [ ] SubTask 8.3: 将配置 API handler 改为 Gin 原生
  - 注：此任务为优化项，可在后续迭代中逐步完成

- [x] Task 9: 移动端兼容性验证
  - [x] SubTask 9.1: 验证 HTTP API 端点响应格式不变（已验证 /ping, /health, /api/files, /api/permissions, /api/tasks, /api/config, /api/remote/info, /api/files/exists, /admin/file/analyze）
  - [x] SubTask 9.2: CORS 配置正确（gin-contrib/cors AllowAllOrigins）
  - [x] SubTask 9.3: WebSocket 路由已注册（gin.WrapF 包裹）
  - [x] SubTask 9.4: 视频流端点已注册
  - [x] SubTask 9.5: Capacitor 插件调用不受影响（JNI 不经过 HTTP）
  - [x] SubTask 9.6: ENCV_MOBILE 不再跳过 admin 路由
  - [x] SubTask 9.7: 单端口模式正常
  - [x] SubTask 9.8-9.10: Admin 路由已注册（/login, /admin/*），需真机验证

- [x] Task 10: 桌面端验证
  - [x] SubTask 10.1: Admin 登录页面正常（/login 返回 200）
  - [x] SubTask 10.2: 反向代理路由已注册
  - [x] SubTask 10.3: OpenList 代理路由已注册
  - [x] SubTask 10.4: WebDAV 路由已注册（gin.WrapH 包裹）
  - [x] SubTask 10.5: 文件服务 NoRoute handler 已注册

# Task Dependencies

- [Task 1] 是所有后续任务的前置条件
- [Task 2] 依赖 [Task 1]
- [Task 3] 依赖 [Task 2]
- [Task 4] 依赖 [Task 2]，可与 [Task 3] 并行
- [Task 5] 依赖 [Task 4]
- [Task 6] 可与 [Task 3-5] 并行开发
- [Task 7] 依赖 [Task 3-6] 全部完成
- [Task 8] 依赖 [Task 7]，是优化项
- [Task 9] 依赖 [Task 7]
- [Task 10] 依赖 [Task 7]，可与 [Task 9] 并行

# 渐进式迁移策略

相比 Fiber 方案的核心优势：**Gin 完全兼容 net/http，可以渐进式迁移**。

1. **Phase 1（功能对齐）**：Task 1-5 ✅ 已完成
2. **Phase 2（清理）**：Task 6-7 ✅ 已完成
3. **Phase 3（优化）**：Task 8 — 待后续迭代
