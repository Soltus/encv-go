# 修复文本预览卡加载和加密文本乱码

## 根因分析

### 问题1：普通文本卡加载中

**根因：`_preview` 路径检查 bug（致命）**

[openlist_handlers.go:61](file:///workspace/internal/server/openlist_handlers.go#L61) 中：

```go
if strings.HasPrefix(path, "_preview/") {
```

`path` 经过正则替换后为 `/_preview/text.html`（以 `/` 开头），但检查的是 `"_"` 开头，**永远不匹配**。导致：
- text.html 预览页面从未被正确提供
- 请求落入后续逻辑，要么因缺少 `sign` 参数返回 400，要么 `serveStandardFile` 尝试从 AList 获取 `/_preview/text.html` 返回 500
- 用户在 AList iframe 中看到的是错误/无响应，表现为"卡加载中"

**修复**：改为 `strings.HasPrefix(path, "/_preview/")`

### 问题2：加密文本预览乱码

**根因：`/d/` 前缀未正确剥离**

AList 下载 URL 格式为 `/d/path/file.txt.sccgt`，但 AList 的 `/api/fs/link` API 期望逻辑路径（不含 `/d/`）。当前代码有两个 `/d/` 剥离缺陷：

1. **`handleDecrypt` 中 `/d/` 剥离在 fallback 分支内**（[openlist_handlers.go:142-144](file:///workspace/internal/server/openlist_handlers.go#L142)）：
   - `strings.CutPrefix(routePath, "/d/")` 只在 `routePath == ""` 时执行
   - 当 middleware 设置了 `routePath`（含 `/d/`），剥离被跳过

2. **`handleDecrypt` 中剥离 `/d/` 后丢失前导 `/`**：
   - `strings.CutPrefix("/d/path/file.txt", "/d/")` → `"path/file.txt"`（无前导 `/`）
   - AList `/api/fs/link` 期望 `"/path/file.txt"`

3. **`HandleRequest` 中 `path` 可能含 `/d/`**：
   - 直接文件访问路径如 `/d/path/file.txt.sccgt` 传给 `serveEncryptedContainer`/`serveStandardFile`
   - 这些函数调用 `OpenListGetFileURL(path, ...)` 时 `/d/` 未被剥离

4. **Middleware 未剥离 `/d/`**（[openlist_middleware.go:46](file:///workspace/internal/server/openlist_middleware.go#L46)）：
   - `routePath` = `/d/path/file.txt.sccgt`（仅剥离了站点前缀）

**结果**：
- `OpenListGetFileURL` 收到含 `/d/` 的路径 → AList API 失败或返回错误
- 如果 API 意外返回了 URL，`IsEncvContainerFromBytes` 验证可能失败 → `serveDirectStream` 返回原始加密二进制 → 乱码

## 修复方案

### Fix 1：修复 `_preview` 路径检查（致命 bug）

**文件**：`internal/server/openlist_handlers.go` 第61行

```go
// 修改前
if strings.HasPrefix(path, "_preview/") {

// 修改后
if strings.HasPrefix(path, "/_preview/") {
```

### Fix 2：`handleDecrypt` 中统一剥离 `/d/` 并确保前导 `/`

**文件**：`internal/server/openlist_handlers.go` `handleDecrypt` 函数

将 `/d/` 剥离逻辑从 `if routePath == ""` 分支内移到外部，确保无论 `routePath` 来源如何都会执行：

```go
func (p *ProxyGin) handleDecrypt(c *gin.Context, siteHost, siteToken string) {
    routePathVal, _ := c.Get("routePath")
    routePath, _ := routePathVal.(string)

    if routePath == "" {
        durl := c.Request.URL.Query().Get("file")
        if durl == "" {
            c.Status(http.StatusBadRequest)
            c.Writer.Write([]byte("Bad Request: 'file' query parameter is missing"))
            return
        }
        slog.Info("[Proxy] No clean path in context, parsing from 'file' query", "durl", durl)

        u, err := url.Parse(durl)
        if err != nil {
            c.Status(http.StatusBadRequest)
            c.Writer.Write([]byte("Bad Request: invalid durl format"))
            return
        }

        routePath = u.Path
    }

    // 统一剥离 /d/ 前缀（AList 下载 URL 含 /d/，但 /api/fs/link 不需要）
    if after, ok := strings.CutPrefix(routePath, "/d/"); ok {
        routePath = "/" + after
    }

    slog.Info("[Proxy] routePath", "path", routePath)
    // ... 后续逻辑不变
}
```

关键改动：
1. `strings.CutPrefix` 移到 `if routePath == ""` 外部
2. 剥离 `/d/` 后补回前导 `/`：`routePath = "/" + after`

### Fix 3：`HandleRequest` 中对直接文件访问路径剥离 `/d/`

**文件**：`internal/server/openlist_handlers.go` `HandleRequest` 函数

在签名验证之后、分发到各 handler 之前，对 `path` 剥离 `/d/`：

```go
// 在 slog.Info("Received valid request for", "path", path) 之后添加：

// 剥离 AList 下载路径的 /d/ 前缀
if after, ok := strings.CutPrefix(path, "/d/"); ok {
    path = "/" + after
    slog.Info("[Proxy] Stripped /d/ prefix", "path", path)
}
```

注意：必须在签名验证之后执行，因为 AList 签名基于含 `/d/` 的原始路径生成。

### Fix 4：Middleware 中剥离 `/d/` 前缀

**文件**：`internal/server/openlist_middleware.go`

在设置 `routePath` 后剥离 `/d/`：

```go
if strings.HasPrefix(parsedURL.Path, pathPrefix) {
    cleanPath := strings.TrimPrefix(parsedURL.Path, pathPrefix)
    if cleanPath == "" {
        cleanPath = "/"
    }
    // 剥离 AList 下载路径的 /d/ 前缀
    if after, ok := strings.CutPrefix(cleanPath, "/d/"); ok {
        cleanPath = "/" + after
    }
    c.Set("routePath", cleanPath)
    slog.Info("[MultiSite Middleware] Extracted routePath", "path", cleanPath)
}
```

### Fix 5：`text.html` 增强错误处理

**文件**：`internal/openlist/web/static/preview/text.html`

为 fetch 添加超时和更详细的错误信息：

```javascript
const controller = new AbortController();
const timeoutId = setTimeout(() => controller.abort(), 30000);

fetch(decryptUrl, { signal: controller.signal })
    .then(response => {
        clearTimeout(timeoutId);
        if (!response.ok) throw new Error(`Server returned ${response.status}: ${response.statusText}`);
        return response.text();
    })
    .then(text => {
        textContent.textContent = text;
        if (!isWrapping) {
            textContent.classList.add('no-wrap');
        }
        loading.style.display = 'none';
        textContent.style.display = 'block';
    })
    .catch(error => {
        clearTimeout(timeoutId);
        console.error('Error fetching text file:', error);
        const msg = error.name === 'AbortError'
            ? 'Request timed out after 30 seconds'
            : `Failed to load text file: ${error.message}`;
        loading.innerHTML = `<span class="error-message">${msg}</span>`;
    });
```

## 修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `internal/server/openlist_handlers.go` | Fix 1: `_preview` 路径检查加 `/`；Fix 2: `/d/` 剥离移到外部并补前导 `/`；Fix 3: 直接文件访问路径剥离 `/d/` |
| `internal/server/openlist_middleware.go` | Fix 4: middleware 中 `routePath` 剥离 `/d/` |
| `internal/openlist/web/static/preview/text.html` | Fix 5: fetch 超时和错误处理增强 |

## 验证步骤

1. `go build ./...` 确保编译通过
2. 部署后测试：
   - 普通文本文件预览：应正常显示文本内容，不再卡加载
   - 加密文本文件预览：应正确解密并显示明文，不再乱码
   - 其他预览（PDF等）：确认不受影响
