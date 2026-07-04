# 修复后端启动后崩溃 + WebSocket 1006 断连

## 问题现象

- logcat 显示后端正常启动
- 前端显示 "Connection closed (code: 1006)"
- 通知栏显示 "后端已退出"

## 根因分析

### 根因（已验证）：gin 路由冲突导致 panic

当前代码：
```go
r.Any(s.webdavPath+"*path", gin.WrapH(protectedWebdavHandler))  // /webdav/*path
r.Any(s.webdavPath, gin.WrapH(protectedWebdavHandler))           // /webdav/
```

`s.webdavPath` = `/webdav/`，所以注册了 `/webdav/*path` 和 `/webdav/`。

**实测验证**：gin 中 `/webdav/` 和 `/webdav/*path` 同时注册会 panic：
```
PANIC: '/' in new path '/webdav/' conflicts with existing wildcard '/*path' in existing prefix '/*path'
```

而 `/webdav`（无尾部斜杠）和 `/webdav/*path` 不冲突（已验证）。

**结论**：`r.Any("/webdav/", ...)` 导致 gin panic，后端崩溃退出。EncvGoService 检测到进程退出，更新通知为"后端已退出"。前端 WebSocket 断开，显示 code 1006。

### 次要问题：`loggingResponseWriter` 丢失 HTTP 接口

`loggingResponseWriter` 只实现了 `WriteHeader`，没有实现 `http.Flusher` 等接口。当 WebDAV handler 尝试 `w.(http.Flusher)` 断言时可能失败。虽然不会导致崩溃（Go net/http 有 panic recovery），但可能导致 WebDAV 功能异常。

### 次要问题：前端 WebSocket 竞态条件

`handleRestart` 中设置 `isOnline = true` 后，如果新 WebSocket 连接失败（比如服务器还没完全就绪），`onclose` 事件通过 `eventBus` 将 `isOnline` 覆盖为 `false`。

## 修复方案

### 修复 1：修复 gin 路由冲突（关键修复）

`internal/server/server.go`：
- 将 `r.Any(s.webdavPath, ...)` 改为 `r.Any(strings.TrimSuffix(s.webdavPath, "/"), ...)`
- 即 `/webdav/` → `/webdav`，与 `/webdav/*path` 不冲突
- 在 `/webdav` handler 中重写 `c.Request.URL.Path` 为 `/webdav/`，然后转发到 WebDAV handler

### 修复 2：`loggingResponseWriter` 实现 `http.Flusher`

`internal/server/server.go`：
- 添加 `Flush()` 方法，委托给底层 ResponseWriter

### 修复 3：前端 WebSocket 竞态条件

`app/encv-mobile/src/composables/useServerStatus.ts`：
- `onServerStatus` 中，如果 `isRestarting.value` 为 true，忽略 `online: false` 事件
- `onConnectionError` 中，如果 `isRestarting.value` 为 true，不更新 `lastError`

## 实施步骤

### 步骤 1：修复 gin 路由冲突

`internal/server/server.go`：
```go
webdavRoot := strings.TrimSuffix(s.webdavPath, "/")
r.Any(webdavRoot, func(c *gin.Context) {
    c.Request.URL.Path = s.webdavPath
    protectedWebdavHandler.ServeHTTP(c.Writer, c.Request)
})
r.Any(s.webdavPath+"*path", gin.WrapH(protectedWebdavHandler))
```

### 步骤 2：修复 `loggingResponseWriter`

`internal/server/server.go`：
```go
func (lrw *loggingResponseWriter) Flush() {
    if f, ok := lrw.ResponseWriter.(http.Flusher); ok {
        f.Flush()
    }
}
```

### 步骤 3：修复前端竞态条件

`app/encv-mobile/src/composables/useServerStatus.ts`：
```typescript
function onServerStatus(data: { online: boolean }) {
  if (isRestarting.value && !data.online) return
  isOnline.value = data.online
  if (data.online) {
    lastError.value = ''
  }
}

function onConnectionError(data: { error: string }) {
  if (isRestarting.value) return
  lastError.value = data.error
}
```

### 步骤 4：验证

1. Go 编译通过
2. 前端构建通过
