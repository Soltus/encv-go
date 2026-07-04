# 修复 WebDAV 测试连接严重错误 + 改进结果展示

## 问题分析

### Bug 根因
`internal/service/mobile_service.go` 的 `TestWebDAV()` 方法（第 228-249 行）实现极其简陋：

```go
func (s *MobileService) TestWebDAV(url, username, password string) error {
    client := &http.Client{Timeout: 5 * time.Second}
    httpReq, err := http.NewRequest(http.MethodGet, url, nil)  // ← 用 GET 而非 PROPFIND
    // ...
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {  // ← 任何 2xx 都算成功
        return nil
    }
}
```

**为什么 GitHub URL 会返回成功：**
- 对 `https://github.com/sudoevolve/EUI-NEO` 发送 HTTP GET → 返回 **200 OK**（GitHub 页面正常）
- 代码只检查 2xx 状态码 → 判定"连接成功"
- 完全没有验证是否是 WebDAV 服务器

### 正确的 WebDAV 测试应该做什么
参考已有的本地 WebDAV 测试（`handleTestLocalWebDAVGin`，第 281-349 行）：
1. 使用 **PROPFIND** 方法（WebDAV 核心）
2. 发送 WebDAV XML body（`<d:propfind xmlns:d="DAV:">...`）
3. 设置 `Depth: 1` 头
4. 检查响应码为 **207 MultiStatus**（WebDAV 标准）
5. 检查响应头中的 `DAV` 字段（确认服务器支持 WebDAV）
6. 验证认证是否有效
7. 返回结构化的详细结果

## 实现计划

### 步骤 1：后端 — 重写 TestWebDAV 方法

**文件**：`internal/service/mobile_service.go`

将 `TestWebDAV` 从返回简单 `error` 改为返回结构化结果：

```go
type WebDAVTestResult struct {
    Success bool              `json:"success"`
    Reachable bool            `json:"reachable"`
    IsWebDAV bool             `json:"is_webdav"`
    AuthOK     bool           `json:"auth_ok"`
    DirReadable bool          `json:"dir_readable"`
    StatusCode int            `json:"status_code"`
    DAVHeader  string         `json:"dav_header,omitempty"`
    Error      string         `json:"error,omitempty"`
}

func (s *MobileService) TestWebDAV(urlStr, username, password string) (*WebDAVTestResult, error) {
```

测试逻辑：
1. **URL 可达性测试**：HTTP HEAD 或 GET 检查是否能连上（超时 5s）
2. **WebDAV 协议检测**：发送 PROPFIND + XML body + Depth:0
   - 响应 207 MultiStatus → 是 WebDAV
   - 响应 401/403 → 可能是 WebDAV 但需要认证
   - 响应 200/302/其他 → 不是 WebDAV（普通 HTTP 服务器）
3. **认证测试**：如果配置了用户名密码，带 BasicAuth 再发一次 PROPFIND
4. **目录可读性**：Depth:1 PROPFIND 检查能否列出内容

关键判断：
- `https://github.com/xxx` → HTTP 200 但无 DAV 头、PROPFIND 返回非 207 → `is_webdav=false`, `success=false`
- 真实 WebDAV（如坚果云/Nextcloud）→ PROPFIND 返回 207 + DAV 头 → `is_webdav=true`, `success=true`

### 步骤 2：后端 — 更新 API handler

**文件**：`internal/server/mobile_api.go`

更新 `handleTestWebDAVGin` 以使用新的结构化返回：

```go
func (s *Server) handleTestWebDAVGin(c *gin.Context) {
    var req struct {
        URL      string `json:"url"`
        Username string `json:"username"`
        Password string `json:"password"`
    }
    // ... bind ...

    result, err := s.mobileSvc.TestWebDAV(req.URL, req.Username, req.Password)
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, result)
}
```

### 步骤 3：前端 API 类型更新

**文件**：`app/encv-mobile/src/api/encv.ts`

更新 `testWebDAVConnection` 的返回类型和错误处理：

```typescript
export interface WebDAVTestResult {
  success: boolean
  reachable: boolean
  is_webdav: boolean
  auth_ok: boolean
  dir_readable: boolean
  status_code: number
  dav_header?: string
  error?: string
}

export async function testWebDAVConnection(config: Omit<WebDAVConfig, 'id'>): Promise<WebDAVTestResult> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/webdav/test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
  const data = await response.json()
  if (!data.success) {
    throw new Error(data.error || '连接失败')
  }
  return data as WebDAVTestResult
}
```

### 步骤 4：前端 — 用弹窗/内联区域替代 toast 展示结果

**文件**：`app/encv-mobile/src/views/WebDAV.vue`

**改动要点**：
1. **删除 toast**：不再用 `showToast` 展示测试结果
2. **添加内联结果区域**：在 modal 内的"测试连接"按钮下方添加测试结果展示区
3. **使用 Alert 弹窗**（备选或补充）：对于错误详情较多的场景，用 `IonAlert`

具体 UI 设计：
```
[测试连接按钮]

┌─ 测试结果区域（v-if="testResult"） ─────────────┐
│                                                    │
│  ✅ URL 可达                    [green badge]       │
│  ❌ 非 WebDAV 服务器           [danger badge]       │
│  ⚠️ 服务器响应: HTTP 200                            │
│                                                     │
│  详细信息:                                          │
│  https://github.com/sudoevolve/EUI-NEO 返回了      │
│  HTTP 200，但这不是 WebDAV 服务器。                  │
│  请确认您的 WebDAV 服务器地址正确。                   │
│                                                     │
└────────────────────────────────────────────────────┘
```

数据结构：
```typescript
const testResult = ref<WebDAVTestResult | null>(null)
```

`testConnection()` 和 `testConfig()` 函数改为设置 `testResult` 而不是调用 toast。

### 步骤 5：i18n keys

**文件**：`app/encv-mobile/src/composables/useI18n.ts`

新增 keys：
```typescript
// 中文
'webdav.testResult': '测试结果'
'webdav.reachable': 'URL 可达'
'webdav.notReachable': 'URL 不可达'
'webdav.isWebDAV': 'WebDAV 协议'
'webdav.notWebDAV': '非 WebDAV 服务器'
'webdav.authOK': '认证通过'
'webdav.authFailed': '认证失败'
'webdav.dirReadable': '目录可读'
'webdav.dirNotReadable': '目录不可读'
'webdav.statusCode': '响应状态'
'webdav.davHeader': 'DAV 版本'
'webdav.testDetail': '详细信息'
'webdav.notWebDAVHint': '该地址返回了 HTTP {status}，但未检测到 WebDAV 协议支持。请确认服务器地址和端口正确。'
'webdav.reachableButNotWebDAV': '该地址可以访问，但不是有效的 WebDAV 服务器'

// English
'webdav.testResult': 'Test Results'
'webdav.reachable': 'URL Reachable'
'webdav.notReachable': 'URL Unreachable'
'webdav.isWebDAV': 'WebDAV Protocol'
'webdav.notWebDAV': 'Not a WebDAV Server'
'webdav.authOK': 'Authentication OK'
'webdav.authFailed': 'Authentication Failed'
'webdav.dirReadable': 'Directory Readable'
'webdav.dirNotReadable': 'Directory Not Readable'
'webdav.statusCode': 'Response Status'
'webdav.davHeader': 'DAV Version'
'webdav.testDetail': 'Details'
'webdav.notWebDAVHint': 'The address returned HTTP {status}, but no WebDAV protocol support was detected. Please verify the server address and port.'
'webdav.reachableButNotWebDAV': 'The address is reachable but is not a valid WebDAV server'
```

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/service/mobile_service.go` | 重写 | `TestWebDAV` 返回 `*WebDAVTestResult`，使用 PROPFIND |
| `internal/server/mobile_api.go` | 修改 | handler 返回结构化 JSON |
| `app/encv-mobile/src/api/encv.ts` | 修改 | 新增 `WebDAVTestResult` 接口，更新返回类型 |
| `app/encv-mobile/src/views/WebDAV.vue` | 重写 | toast → 内联结果区域 + 详细信息展示 |
| `app/encv-mobile/src/composables/useI18n.ts` | 修改 | 新增 WebDAV 测试相关 i18n key |

## 验证
- `vue-tsc --noEmit && vite build`
- `go vet ./internal/service/... ./internal/server/...`
