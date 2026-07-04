# 修复后端启动失败问题

## 问题分析

通过本地模拟移动端环境启动后端，发现以下问题：

### 问题 1：WebDAV 索引器 panic（致命）

`internal/webdav/fs_v2.go:195` 行的 `filepath.Walk` 回调中，没有先检查 `err` 参数就直接访问 `info.IsDir()`：

```go
_ = filepath.Walk(fs.dir, func(path string, info os.FileInfo, err error) error {
    if info.IsDir() {  // ← 当 err != nil 时 info 为 nil，panic!
        return watcher.Add(path)
    }
    return nil
})
```

**触发条件**：当 WebDAV 目录不存在时（如移动端首次启动，`/storage/emulated/0/encv-output` 不存在），`filepath.Walk` 传入 `err != nil, info == nil`，导致 nil pointer dereference panic，进程直接崩溃。

**影响**：即使移动端配置中 `webdav.dir` 为空（不启用 WebDAV），默认配置中 `output_path: "/storage/emulated/0/encv-output"` 也可能触发此问题，因为 WebDAV dir 可能被默认设置。

### 问题 2：WebDAV 目录不存在时 NewENCVFS 直接 os.Exit（致命）

`fs_v2.go:124-126` 行，当 WebDAV 目录无法解析为绝对路径时直接 `os.Exit(1)`，而不是返回错误让调用者处理：

```go
dir, err = filepath.Abs(dir)
if err != nil {
    slog.Error("Failed to resolve absolute path for WebDAV directory", "dir", dir, "error", err)
    os.Exit(1)  // ← 直接退出进程！
}
```

**影响**：移动端首次启动时，WebDAV 目录可能还不存在，这会导致整个后端进程直接退出。

### 问题 3：testWebDAV API 返回格式与前端期望不一致

前端 `testWebDAVConnection` 期望 HTTP 200 + body，但当前实现中非 BadRequestError 类型的错误返回 500 状态码。前端代码：

```typescript
if (!response.ok) {
    let detail = `HTTP ${response.status}`
    try { const body = await response.text(); if (body) detail += `: ${body}` } catch {}
    throw new Error(detail)
}
```

当前后端 `handleTestWebDAV` 对 BadRequestError 返回 `{success: false, error: ...}` + 200，但对其他错误（网络超时等）返回 500。前端会抛出异常而不是显示友好的错误信息。

### 问题 4：MobileService 在 NewServer 时创建两次

`server.go` 中：
- `NewServer()` 中创建 `mobileSvc := mobileservice.NewMobileService("")`
- `Start()` 中又创建 `s.mobileSvc = mobileservice.NewMobileService(s.servingDir)`

第一次创建的实例被丢弃，浪费资源。应该只在 `Start()` 中创建一次，或使用 `SetServingDir`。

## 修复计划

### Step 1：修复 WebDAV fs_v2.go 的 filepath.Walk panic

在 `runIndexer` 方法的 `filepath.Walk` 回调中，先检查 `err` 参数：

```go
_ = filepath.Walk(fs.dir, func(path string, info os.FileInfo, err error) error {
    if err != nil {
        return nil  // 跳过无法访问的路径
    }
    if info.IsDir() {
        return watcher.Add(path)
    }
    return nil
})
```

### Step 2：修复 WebDAV NewENCVFS 的 os.Exit 问题

将 `os.Exit(1)` 改为返回 error，让调用者（`server.go`）决定如何处理。如果 WebDAV 目录不存在，跳过 WebDAV 初始化并记录警告日志。

具体改动：
1. `NewENCVFS` 返回 `(goWebdav.FileSystem, error)` 而非 `goWebdav.FileSystem`
2. 目录不存在时返回 nil 和友好错误
3. `server.go` 中检查 error，如果 WebDAV 初始化失败则跳过 WebDAV 注册并记录警告

### Step 3：修复 MobileService 重复创建

移除 `NewServer()` 中的 `mobileSvc` 创建，改为在 `Start()` 中使用 `SetServingDir`：

```go
// NewServer 中
mobileSvc := mobileservice.NewMobileService("")

// Start 中
s.mobileSvc.SetServingDir(s.servingDir)
```

### Step 4：修复 testWebDAV API 返回格式

将所有 WebDAV 测试错误统一返回 HTTP 200 + `{success: false, error: ...}` 格式，与前端期望一致：

```go
err = s.mobileSvc.TestWebDAV(req.URL, req.Username, req.Password)
if err != nil {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
    return
}
```

### Step 5：验证

- `go build ./cmd/encv/` 编译通过
- 模拟移动端模式启动不 panic
- 模拟 WebDAV 目录不存在时不崩溃
