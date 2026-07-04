# Plan: Context 传递验证 + 端口伪占用侦测 + 可测试性改进

## 一、Context 传递审查

### 1.1 当前机制

`ConfigMiddleware` 在 [gin_app.go:37-43](file:///workspace/internal/server/gin_app.go#L37-L43) 中将 `*config.Config` 注入到 `c.Request.Context()`：

```go
func ConfigMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx := config.NewContext(c.Request.Context(), cfg)
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}
```

### 1.2 审查结论：Context 传递正常但未被充分利用

**正常工作的部分：**
- `ConfigMiddleware` 正确注入 config 到 request context
- `NewServer` 通过 `config.FromContext(ctx)` 正确提取 config（[server.go:53](file:///workspace/internal/server/server.go#L53)）
- `handleFileAnalyzeGin` 中 `c.Request.Context()` 正确传递给 `detector.AnalyzeContainerV2`（[admin_handlers.go:211](file:///workspace/internal/server/admin_handlers.go#L211)）
- `handleFSProxyGin` 中 `httptest.NewRecorder()` + `s.servePath(rec, c.Request, path)` 正确传递了带 config 的 context
- `handleStreamRequest`（gin.WrapF）中 `r.Context()` 正确传递给 `provider.NewLocalFileProvider`

**未被充分利用的问题：**
- 所有 Gin 原生 handler 都通过 `s.cfg` 字段访问配置，而非 `config.FromContext(c.Request.Context())`
- 这意味着 `ConfigMiddleware` 注入的 context 值实际上只对 `NewServer` 初始化和少数直接使用 `c.Request.Context()` 的场景有效
- **这不是 bug**，因为 `s.cfg` 是同一个指针，值始终一致。但从架构一致性角度，存在两条配置获取路径

**建议（低优先级，不阻塞）：**
- 保持现状即可。`s.cfg` 直接访问比 context 提取更高效且类型安全
- 如果未来需要请求级别的配置覆盖（如 per-request config override），则应统一使用 context 路径

### 1.3 需要修复的问题

**无**。Context 传递链路完整，无断裂。

---

## 二、端口伪占用侦测审查

### 2.1 当前机制

[server_start.go](file:///workspace/internal/register/server_start.go) 中 `StartGinWithRetry` 实现了：
1. `net.Listen("tcp", addr)` — 尝试绑定端口
2. 如果绑定成功，在 goroutine 中启动 `srv.Serve(listener)`
3. 等待 150ms 后执行 `performPingCheck`
4. `performPingCheck` 发送 GET /ping，验证 `instanceID` 和 `version` 匹配
5. 如果 instanceID 不匹配（端口被其他进程占用），关闭 listener 并尝试下一个端口

### 2.2 审查结论：侦测逻辑正确，但存在关键 bug

**正常工作的部分：**
- 端口递增探测逻辑正确
- `IsAddrInUseErr` 正确识别 EADDRINUSE 错误
- `performPingCheck` 的 instanceID + version 双重验证逻辑正确
- 150ms 等待时间合理（给服务器启动时间）

### 2.3 关键 Bug：`s.server` 从未被赋值

**问题：** `StartGinWithRetry` 在 goroutine 内创建 `srv := &http.Server{Handler: handler}`（[server_start.go:67](file:///workspace/internal/register/server_start.go#L67)），但 `s.server` 字段从未被赋值。

**影响：**
- `Stop()` 方法（[server.go:223-232](file:///workspace/internal/server/server.go#L223-L232)）中 `s.server != nil` 永远为 false，优雅关闭永远不会执行
- `handleServerShutdownGin`（[mobile_api.go:62-73](file:///workspace/internal/server/mobile_api.go#L62-L73)）中 `s.server.Shutdown(ctx)` 永远不会被调用，只能靠 `os.Exit(0)` 强制退出
- 无法实现优雅关闭（graceful shutdown），正在处理的请求会被强制中断

**修复方案：** 修改 `StartGinWithRetry` 使其返回 `*http.Server`，然后在 `Server.Start` 中赋值给 `s.server`。

```go
// register/server_start.go
func StartGinWithRetry(engine *gin.Engine, initialPort int, instanceID, version string) (*http.Server, string, error) {
    // ...
    go func() {
        srv := &http.Server{Handler: handler}
        // 需要通过 channel 或回调将 srv 传出去
    }()
    // ...
}
```

具体实现：在 goroutine 外创建 `*http.Server`，传入 goroutine 使用。

```go
func StartGinWithRetry(engine *gin.Engine, initialPort int, instanceID, version string) (*http.Server, string, error) {
    handler := engine
    maxTries := 100
    for i := 0; i < maxTries; i++ {
        currentPort := initialPort + i
        addr := fmt.Sprintf(":%d", currentPort)

        listener, err := net.Listen("tcp", addr)
        if err != nil {
            logAndContinue(err, currentPort)
            continue
        }

        srv := &http.Server{Handler: handler} // 在 goroutine 外创建
        go func() {
            slog.Info("Server attempting to start", "addr", addr)
            if serveErr := srv.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
                slog.Error("Server encountered an error", "addr", listener.Addr().String(), "error", serveErr)
            }
        }()

        time.Sleep(150 * time.Millisecond)

        if err := performPingCheck(currentPort, instanceID, version); err != nil {
            slog.Warn("Self-check failed, trying next port", "port", currentPort, "error", err)
            listener.Close()
            continue
        }

        actualAddr := listener.Addr().String()
        slog.Info("Server successfully started", "addr", actualAddr)
        return srv, actualAddr, nil
    }

    return nil, "", fmt.Errorf("failed to start server after %d tries", maxTries)
}
```

然后在 `Server.Start` 中：
```go
srv, addr, err := register.StartGinWithRetry(r, s.cfg.Server.Port, s.instanceID, s.version)
if err != nil {
    return "", err
}
s.server = srv
return addr, nil
```

同理修复 `StartHttpHandlerWithRetry`。

---

## 三、其他已发现的 Bug

### 3.1 JWT 中间件双重验证

**位置：** [admin_middleware.go:35-44](file:///workspace/internal/server/admin_middleware.go#L35-L44)

```go
_, err := jwtManager.ValidateToken(token)  // 第一次验证
if err != nil {
    // ... 处理错误
}
claims, _ := jwtManager.ValidateToken(token)  // 第二次验证，错误被忽略
c.Set("auth_claims", claims)
```

**问题：** `ValidateToken` 被调用了两次，第二次的错误被 `_` 忽略。虽然逻辑上第二次不会失败（第一次已验证），但这是不必要的开销和代码坏味道。

**修复：**
```go
claims, err := jwtManager.ValidateToken(token)
if err != nil {
    auth.ClearAuthCookie(c.Writer)
    redirectToLoginGin(c)
    c.Abort()
    return
}
c.Set("auth_claims", claims)
c.Set("is_authenticated", true)
c.Next()
```

### 3.2 `generateSessionID` 使用伪随机

**位置：** [auth.go:135-142](file:///workspace/internal/auth/auth.go#L135-L142)

```go
func generateSessionID() string {
    b := make([]byte, 16)
    for i := range b {
        b[i] = byte(time.Now().UnixNano() >> (i * 8))
    }
    return base64.URLEncoding.EncodeToString(b)
}
```

**问题：** 使用 `time.Now().UnixNano()` 生成 session ID 是可预测的，不符合安全要求。同一纳秒内生成的 ID 完全相同。

**修复：**
```go
func generateSessionID() string {
    b := make([]byte, 16)
    if _, err := crypto_rand.Read(b); err != nil {
        // 极端情况下的 fallback
        panic("failed to generate random session ID: " + err.Error())
    }
    return base64.URLEncoding.EncodeToString(b)
}
```

### 3.3 `GetTokenFromCookie` 调试日志残留

**位置：** [auth.go:174](file:///workspace/internal/auth/auth.go#L174)

```go
log.Printf("cookie not found")
```

**问题：** 每次 cookie 不存在时都打印日志，这是正常的业务场景（未登录用户），会产生大量噪音日志。

**修复：** 删除该行，或改为 `slog.Debug` 级别。

### 3.4 `JWTAuthMiddleware` 中的调试日志残留

**位置：** [admin_middleware.go:21](file:///workspace/internal/server/admin_middleware.go#L21)

```go
log.Printf("[JWTAuthMiddleware] Processing path: %s", c.Request.URL.Path)
```

**问题：** 每个请求都打印，生产环境会产生大量日志。

**修复：** 删除或改为 `slog.Debug`。

### 3.5 `handleLoginGin` 中的调试日志残留

**位置：** [admin_handlers.go:35](file:///workspace/internal/server/admin_handlers.go#L35)

```go
log.Printf("[handleLoginGin] User already logged in, session: %s", claims.SessionID)
```

**问题：** 使用 `log.Printf` 而非 `slog`，且暴露 session ID。

**修复：** 改为 `slog.Debug` 并移除 session ID 输出。

---

## 四、可测试性分析与改进方案

### 4.1 当前状况

- 项目有少量测试文件，集中在底层库（crypto、container、reader），server 层零测试
- `Server` 结构体所有依赖通过 `NewServer` 硬编码创建
- `MobileService` 是具体结构体，非接口，无法 mock
- `Server` 字段全部未导出，外部测试包无法访问
- Handler 直接操作 `os.ReadFile`、`os.WriteFile`、`os.Stat` 等文件系统 API，无法在测试中替换

### 4.2 改进方案

#### 改进 1：为 MobileService 抽取接口

当前 `Server` 直接依赖 `*mobileservice.MobileService` 具体类型。抽取接口后可 mock 测试。

```go
// internal/service/interfaces.go
type MobileServiceInterface interface {
    ListFiles(queryPath string) ([]FileInfo, error)
    DeleteFile(queryPath string) error
    ReadFileContent(queryPath string) (*FileContentResult, error)
    GetTaskManager() *TaskManager
    TestWebDAV(url, username, password string) error
    CheckStoragePermission() bool
    SearchFiles(queryPath, keyword string, recursive bool) ([]FileInfo, error)
    GetIndexStats() IndexStats
    RebuildIndex()
    ClearIndex()
    StreamExternalFile(w http.ResponseWriter, r *http.Request, path string) error
    FileExists(queryPath string) (bool, error)
    SetServingDir(dir string)
    SetEncryptedFileDeps(rs *service.ReaderService, ch *handler.ContentHandler, namers []namer.ChunkNamer)
    GetWSHub() *WSHub
}
```

**注意：** 这是一个较大的重构，需要修改 `Server` 结构体字段类型和所有调用点。建议分步进行。

#### 改进 2：Server 构造函数支持依赖注入

当前 `NewServer` 硬编码创建所有依赖。改为接受可选参数：

```go
type ServerOption func(*Server)

func WithMobileService(svc MobileServiceInterface) ServerOption {
    return func(s *Server) { s.mobileSvc = svc }
}

func NewServer(ctx context.Context, configPath string, opts ...ServerOption) *Server {
    cfg := config.FromContext(ctx)
    // ... 默认创建
    s := &Server{...}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

测试时注入 mock：
```go
mockSvc := &MockMobileService{...}
s := NewServer(ctx, configPath, WithMobileService(mockSvc))
```

#### 改进 3：配置文件操作抽象

`server_config_api.go` 和 `mobile_api.go` 中直接使用 `os.ReadFile`/`os.WriteFile`。可以抽象为接口：

```go
type ConfigStore interface {
    Read() ([]byte, error)
    Write(data []byte) error
    SchemaPath() (string, error)
}

type FileConfigStore struct {
    path string
}
```

测试时使用内存实现：
```go
type MemoryConfigStore struct {
    data []byte
}
```

#### 改进 4：Handler 测试工具函数

创建测试辅助函数，简化 handler 测试：

```go
// internal/server/test_helpers.go (仅测试时使用)
func NewTestServer(t *testing.T, cfg *config.Config) *Server {
    // 创建使用临时目录和 mock 依赖的 Server
}

func NewTestGinContext(t *testing.T, method, path string, body io.Reader) *gin.Context {
    // 创建带 httptest.Recorder 的 Gin Context
}
```

#### 改进 5：全局 mutex 改为实例级别

`configMu` 是包级别全局变量（[server_config_api.go:15](file:///workspace/internal/server/server_config_api.go#L15)），并发测试时会互相干扰。应移到 `Server` 结构体中：

```go
type Server struct {
    // ...
    configMu sync.Mutex
}
```

#### 改进 6：`handleStreamExternalFileGin` 的错误处理

**位置：** [mobile_api.go:223-237](file:///workspace/internal/server/mobile_api.go#L223-L237)

```go
func (s *Server) handleStreamExternalFileGin(c *gin.Context) {
    // ...
    err = s.mobileSvc.StreamExternalFile(c.Writer, c.Request, decodedPath)
    if err != nil {
        writeServiceErrorGin(c, err)  // 如果 stream 已开始写入，这里会失败
        return
    }
}
```

**问题：** 如果 `StreamExternalFile` 已经开始写入响应体（如已发送 Content-Type header 和部分数据），再调用 `writeServiceErrorGin` 写 JSON 会失败或产生混合内容。

**改进：** 在 `StreamExternalFile` 内部处理错误响应，或增加一个 `StreamStarted` 标志避免重复写入。

### 4.3 改进优先级排序

| 优先级 | 改进项 | 原因 |
|--------|--------|------|
| P0 | 修复 `s.server` 未赋值 bug | 影响优雅关闭和 shutdown API |
| P0 | 修复 JWT 双重验证 | 安全相关代码坏味道 |
| P0 | 修复 `generateSessionID` 伪随机 | 安全漏洞 |
| P1 | 清理调试日志残留 | 生产环境日志噪音 |
| P1 | 全局 mutex → 实例级别 | 并发测试前提条件 |
| P2 | MobileService 接口抽取 | 可 mock 测试的前提 |
| P2 | Server 依赖注入 | 可 mock 测试的前提 |
| P2 | Handler 测试工具函数 | 降低测试编写成本 |
| P3 | 配置文件操作抽象 | 配置 API 可测试 |
| P3 | StreamExternalFile 错误处理 | 边界情况改进 |

---

## 五、实施步骤

### Step 1: 修复 `s.server` 未赋值 bug
- 修改 `StartGinWithRetry` 返回 `*http.Server`
- 修改 `StartHttpHandlerWithRetry` 返回 `*http.Server`
- 修改 `Server.Start` 接收并赋值 `s.server`
- 修改 `handleServerShutdownGin` 移除 `os.Exit(0)` 的强制退出（优雅关闭后自然退出）

### Step 2: 修复 JWT 双重验证
- 合并两次 `ValidateToken` 调用为一次
- 清理 `JWTAuthMiddleware` 中的调试日志

### Step 3: 修复 `generateSessionID` 伪随机
- 改用 `crypto/rand.Read`

### Step 4: 清理所有调试日志残留
- `GetTokenFromCookie` 中的 `log.Printf("cookie not found")`
- `JWTAuthMiddleware` 中的 `log.Printf("[JWTAuthMiddleware]...")`
- `handleLoginGin` 中的 `log.Printf("[handleLoginGin]...")`
- `SetRedirectCookie` / `GetRedirectCookie` 中的 `log.Printf`
- `handleFileRenameGin` 中的 `log.Printf`
- 统一使用 `slog.Debug` 或直接删除

### Step 5: 全局 mutex → 实例级别
- 将 `configMu` 从包级别变量移入 `Server` 结构体

### Step 6: 验证编译和基本功能
- `go build ./...`
- 检查所有调用点是否正确更新
