# Capacitor 预览模式允许上传测试文件（浏览器沙箱）

## 背景

### 当前问题

Capacitor 预览模式下，前端运行在浏览器中，无法使用原生文件系统。用户想要测试加密/解密功能时，**没有任何方式将测试文件上传到后端服务器**：

```
当前状态：
┌─────────────┐     HTTP API      ┌──────────────┐
│  浏览器预览   │ ──── 读取 ────→   │  Go 后端服务器  │
│  (前端)      │ ←─── 文件列表 ───  │  (文件系统)    │
│              │                   │               │
│  ❌ 无法上传   │    ❌ 无上传API   │  /data/files/ │
└─────────────┘                   └──────────────┘
```

### 现有代码分析

| 层 | 现状 | 关键位置 |
|---|------|---------|
| **后端路由** | 无 upload 端点 | [server.go:213-217](internal/server/server.go#L213-L217) |
| **后端 Service** | 有 Delete/CreateDir 但无 Upload | [mobile_service.go](internal/service/mobile_service.go) |
| **前端 API** | 有 deleteFile/createDirectory 但无 uploadFile | [encv.ts](app/encv-mobile/src/api/encv.ts) |
| **Files.vue UI** | 无 FAB、无上传按钮 | [Files.vue](app/encv-mobile/src/views/Files.vue) |
| **pickFolder() Web fallback** | 返回 `{ path: '' }`（空） | [web.ts:141-143](app/encv-mobile/src/plugins/web.ts#L141-L143) |

---

## 实施方案

### 架构设计

```
目标状态：
┌─────────────┐                    ┌──────────────┐
│  浏览器预览   │  ① <input type=   │  Go 后端服务器  │
│  (前端)      │     "file"> 选择   │               │
│              │  ② FormData POST   │  POST /api/   │
│  ┌────────┐ │  ──→ /api/files/   │  files/upload │
│  │ FAB 按钮 │ │     upload        │       ↓       │
│  │(上传)   │ │  ③ 刷新文件列表    │  写入文件系统  │
│  └────────┘ │  ←── 200 OK ────── │  /data/files/ │
└─────────────┘                    └──────────────┘
```

---

## Step 1: 后端 — 新增文件上传 API 端点

### 1.1 在 `mobile_service.go` 中添加 `UploadFile` 方法

```go
func (s *MobileService) UploadFile(targetPath string, fileName string, content io.Reader, maxSize int64) (*FileItem, error)
```

**逻辑**：
1. `SafeResolveToAbsPath(s.servingDir, targetPath)` 解析目标目录（路径穿越防护）
2. 检查目标路径是目录且存在
3. 检查磁盘空间（可选）
4. 创建目标文件 `filepath.Join(absDir, fileName)`
5. 从 `io.Reader` 复制内容到文件（限制 `maxSize`）
6. 返回新文件的 `FileItem` 元信息

**安全措施**：
- 路径穿越检查（复用 `SafeResolveToAbsPath`）
- 文件大小上限（默认 500MB，可配置）
- 文件名清理（去除 `../` 前缀和空字节）

### 1.2 在 `server.go` 中注册路由

在现有 `/api/files` 路由组中添加（行 216-217 附近）：

```go
r.POST("/api/files/upload", s.handleUploadFileGin)
```

### 1.3 在 `mobile_api.go` 中添加 handler

```go
func (s *Server) handleUploadFileGin(c *gin.Context) {
    // 1. 解析 query param ?path=/target/dir
    // 2. c.Request.ParseMultipartForm(maxMemory)
    // 3. file, header := c.FormFile("file")
    // 4. 调用 s.mobileSvc.UploadFile(targetPath, file.Filename, file, maxSize)
    // 5. 返回 { name, path, size, ... }
}
```

**请求格式**：`multipart/form-data`
- `path`: 目标目录路径（query string）
- `file`: 上传的文件（form field）

---

## Step 2: 前端 API 层 — 添加 `uploadFile` 函数

### 文件：[encv.ts](app/encv-mobile/src/api/encv.ts)

```typescript
export async function uploadFile(
  targetPath: string,
  file: File,
  onProgress?: (percent: number) => void
): Promise<FileItem> {
  const baseUrl = getApiBaseUrl()
  const formData = new FormData()
  formData.append('file', file)
  formData.append('path', targetPath)

  const response = await fetch(`${baseUrl}/api/files/upload?path=${proxySafeEncode(targetPath)}`, {
    method: 'POST',
    body: formData,
  })

  if (!response.ok) {
    throw new Error(`Upload failed: ${response.status}`)
  }
  return response.json()
}
```

---

## Step 3: Files.vue UI — 添加上传按钮

### 3.1 添加 FAB 上传按钮

在 `<ion-content>` 内部末尾（`</ion-list>` 之后、`</ion-content>` 之前）添加：

```html
<ion-fab vertical="bottom" horizontal="end" slot="fixed">
  <ion-fab-button @click="handleUpload">
    <ion-icon :icon="add" />
  </ion-fab-button>
</ion-fab>

<!-- 隐藏的文件选择 input -->
<input
  ref="fileInputRef"
  type="file"
  multiple
  style="display: none"
  @change="handleFileSelected"
/>
```

### 3.2 添加上传逻辑

```typescript
import { add, cloudUploadOutline } from 'ionicons/icons'
import { IonFab, IonFabButton } from '@ionic/vue'
import { uploadFile } from '@/api/encv'

const fileInputRef = ref<HTMLInputElement>()

function handleUpload() {
  fileInputRef.value?.click()
}

async function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files?.length) return

  const files = Array.from(input.files)
  let successCount = 0
  let failCount = 0

  for (const file of files) {
    try {
      showToast({ message: `正在上传: ${file.name}...`, color: 'primary' })
      await uploadFile(currentPath.value, file)
      successCount++
    } catch (e) {
      console.error('[Files] upload failed:', file.name, e)
      failCount++
    }
  }

  if (successCount > 0) {
    showToast({
      message: `成功上传 ${successCount} 个文件${failCount > 0 ? `，${failCount} 个失败` : ''}`,
      color: failCount > 0 ? 'warning' : 'success',
    })
    await loadFiles() // 刷新列表
  }

  // 重置 input 以便重复选择同一文件
  input.value = ''
}
```

### 3.3 注册新组件 import

在 Files.vue 的 imports 中添加：
- `IonFab`, `IonFabButton` 来自 `@ionic/vue`
- `add` 图标来自 `ionicons/icons`

---

## Step 4: i18n 支持（可选）

在语言文件中添加上传相关的翻译 key（如果项目有 i18n 系统）。

---

## 改动文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/service/mobile_service.go` | **修改** | 添加 `UploadFile()` 方法 |
| `internal/server/server.go` | **修改** | 注册 `POST /api/files/upload` 路由 |
| `internal/server/mobile_api.go` | **修改** | 添加 `handleUploadFileGin` handler |
| `app/encv-mobile/src/api/encv.ts` | **修改** | 添加 `uploadFile()` 函数 |
| `app/encv-mobile/src/views/Files.vue` | **修改** | 添加 FAB + 隐藏 input + 上传逻辑 |

---

## 验证场景

| # | 场景 | 预期结果 |
|---|------|---------|
| 1 | 点击 FAB → 选择本地文件 | 文件上传到当前浏览目录 |
| 2 | 选择多个文件 | 全部依次上传完成 |
| 3 | 上传超大文件 (>500MB) | 返回错误提示 |
| 4 | 路径穿越攻击 (`../../../etc/passwd`) | 被 SafeResolveToAbsPath 拦截返回 400 |
| 5 | 上传完成后自动刷新 | 文件列表显示新上传的文件 |
| 6 | Native 模式下 FAB 行为 | 正常工作（浏览器 `<input type=file>` 在 WebView 中也可用） |
| 7 | 取消文件选择 | 无操作，不报错 |

---

## 注意事项

1. **FAB 与现有 UI 的协调**：FAB 使用 `slot="fixed"` 定位在右下角，与 Ionic 标准一致
2. **大文件上传体验**：当前方案用 `fetch + FormData`，不支持进度条回调（XMLHttpRequest 可支持但更复杂）。如需进度条可在 v2 中改用 XMLHttpRequest
3. **Native 模式兼容性**：Android WebView 同样支持 `<input type="file">`，因此此功能在 native 和 preview 模式均可用
4. **文件名冲突**：同名文件直接覆盖（与大多数文件管理器行为一致），后续可增加"已存在是否覆盖?"确认
