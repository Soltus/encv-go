- [x] GoFrame (`gogf/gf/v2`) 依赖已从 go.mod 中移除
- [x] `internal/admin/` 目录已删除，功能已合并到 `internal/server/`
- [x] 所有路由使用 Gin 路由注册（无 net/http ServeMux 或 GoFrame RouterGroup）
- [x] 单个 Gin 应用同时提供文件服务、API、Admin 和代理功能
- [x] CORS 中间件使用 Gin 中间件实现，无自定义 CORS 代码
- [x] JWT Auth 中间件使用 Gin 中间件签名
- [ ] BasicAuth 中间件使用 Gin 中间件签名（当前仍为 `func(http.Handler) http.Handler`，通过 `gin.WrapH()` 桥接，未改为 `gin.HandlerFunc`）
- [x] Logging 中间件使用 Gin Logger
- [x] WebSocket 保留 gorilla/websocket，通过 Gin 路由处理
- [x] WebDAV 通过 `gin.WrapH()` 桥接到 Gin 路由
- [ ] HTML 模板渲染使用 `c.HTML()`（当前使用 `template.Execute(c.Writer, ...)` + 手动设置 Content-Type，未使用 Gin 的 `c.HTML()` 方法）
- [ ] 反向代理保留 `httputil.ReverseProxy`，通过 Gin 路由注册（当前代码库中无 `httputil.ReverseProxy`，代理功能由自定义 `ProxyGin` 结构体实现）
- [x] 启动逻辑统一为 `StartGinWithRetry`，无 `StartGfServerWithRetry`
- [x] `cmd/encv/servers.go` 使用统一启动逻辑
- [x] `go build ./...` 编译通过

### 移动端兼容性验证
- [ ] 所有 23 个 HTTP API 端点响应格式不变（对照 encv.ts 逐一测试）
- [ ] CORS 配置正确（移动端 WebView 跨域请求正常）
- [ ] WebSocket 连接正常（心跳 ping/pong，日志推送）
- [ ] 视频流端点正常（`/stream` 和 `/api/stream/external`）
- [ ] Capacitor 插件调用不受影响（JNI 桥接不经过 HTTP）
- [ ] `ENCV_MOBILE=1` 模式下 admin 路由正常可用（登录、OpenList 代理等，不再跳过）
- [ ] 单端口模式正常（移动端和桌面端共用 `cfg.Server.Port`，`cfg.Admin.Port` 已移除）
- [ ] 移动端浏览器可访问 admin UI（`http://127.0.0.1:{port}/admin/`）
- [ ] 移动端 OpenList 代理正常工作
- [ ] 移动端 JWT 登录认证流程正常

### 桌面端验证
- [ ] Admin 登录/登出功能正常
- [ ] 反向代理（`/p/*`、`/p-api/*`）功能正常
- [ ] OpenList 代理功能正常
- [ ] WebDAV 功能正常
- [ ] 文件浏览和目录列表页面正常
