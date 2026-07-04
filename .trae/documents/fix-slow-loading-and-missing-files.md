# 修复首屏加载慢 + 权限识别与自动申请

## 问题 1：首屏加载过慢

### 根因

当前 `Files.vue` 的 `loadFiles()` 流程：
1. 先请求 `/health`（checkServerStatus）
2. 如果在线，再请求 `/api/files`
3. 如果离线，显示"服务器离线"页面，用户需手动重试

在原生平台上，后端启动需要数秒。首屏加载时后端可能还没就绪，前端直接显示离线状态，用户体验差。

### 修复方案

**A. 首屏立即显示 UI，后台等待后端就绪后自动加载**

- `loadFiles()` 不再先调 `checkServerStatus()`，而是直接请求 `/api/files`
- 如果 `/api/files` 失败（后端未就绪），自动重试（原生平台最多 15 次，每次 1 秒）
- 重试期间显示"正在连接后端..."的加载状态，而非"服务器离线"
- 只有所有重试都失败后才显示离线状态

**B. 监听 `encv:backend-ready` 事件自动加载**

- `Files.vue` 监听 `encv:backend-ready` / `server:status` 事件
- 后端就绪时自动加载文件列表
- 避免用户在等待期间需要手动操作

### 实施步骤

1. 重构 `Files.vue` 的 `loadFiles()`：
   - 移除 `checkServerStatus()` 的前置调用
   - 直接请求 `listFiles()`，成功则标记在线
   - 失败时自动重试（带间隔）
   - 添加"正在连接后端..."的区分状态

2. 添加 `encv:backend-ready` 事件监听，后端就绪时自动 `loadFiles()`

3. 更新 i18n 文案，添加"正在连接后端..."等新状态文案

## 问题 2：正确识别无权限状态与前端显示 + 需要时自动申请权限

### 根因

1. **后端无法区分"目录为空"和"无权限访问"**：`os.ReadDir()` 失败时返回通用错误，前端无法知道是权限问题
2. **前端没有权限状态感知**：`Files.vue` 不知道用户是否有存储权限，无法给出正确提示
3. **权限申请入口深**：权限管理在"设置→后端服务"页面，用户很难发现

### 修复方案

**A. 后端：`/api/files` 返回权限错误时提供明确信息**

当 `os.ReadDir()` 因权限失败时，返回特定的错误码和消息，让前端能区分：
- 无权限 → 前端提示用户授权
- 目录不存在 → 前端提示路径无效
- 其他错误 → 通用错误

在 `mobile_api.go` 中，`ListFiles` 返回的错误需要区分权限错误。当前 `os.ReadDir()` 返回的 `*os.PathError` 包含 `Err syscall.Errno`，可以检查 `EACCES`。

**B. 后端：添加 `/api/permissions` 端点**

添加一个 API 端点，让前端可以查询当前权限状态（存储权限是否已授予）。后端通过尝试读取 servingDir 来检测权限。

```go
// GET /api/permissions
func (s *Server) handlePermissions(w http.ResponseWriter, r *http.Request) {
    result := map[string]bool{
        "storage": canReadDir(s.mobileSvc.GetServingDir()),
    }
    json.NewEncoder(w).Encode(result)
}
```

**C. 前端：`Files.vue` 识别无权限状态并显示引导**

当 `listFiles` 返回权限错误时，显示"需要文件管理权限"的引导页面，而非"服务器离线"或"空目录"。

**D. 前端：首次启动时自动申请必要权限**

在 `App.vue` 或 `Files.vue` 的 `onMounted` 中，检测是否为首次启动，如果是则自动申请：
1. 通知权限（Android 13+ 前台服务必需）
2. 存储管理权限（`MANAGE_EXTERNAL_STORAGE`，访问 `/storage/emulated/0` 必需）

### 实施步骤

1. **后端 `mobile_service.go`**：
   - `ListFiles` 返回权限特定的错误类型 `PermissionError`
   - `entry.Info()` 失败时降级返回（Size: 0, Modified: ""），不跳过条目
   - 添加 `CheckStoragePermission()` 方法

2. **后端 `mobile_api.go`**：
   - `writeServiceError` 处理 `PermissionError`，返回 HTTP 403 + 特定错误码
   - 添加 `/api/permissions` 端点

3. **后端 `server.go`**：注册 `/api/permissions` 路由

4. **前端 `api/encv.ts`**：
   - 添加 `checkPermissions()` API 调用（后端权限检查）
   - `listFiles` 返回的错误中识别权限错误

5. **前端 `Files.vue`**：
   - 添加 `noPermission` 状态，显示权限引导页面
   - 添加"授权"按钮，调用 `requestStoragePermission()`
   - 监听 `encv:backend-ready` 事件自动加载

6. **前端 `App.vue`**：
   - 首次启动时自动申请通知权限和存储权限

7. **i18n**：添加权限相关文案

## 文件变更清单

| 文件 | 变更 |
|------|------|
| `internal/service/mobile_service.go` | 添加 `PermissionError`；`entry.Info()` 降级返回；添加 `CheckStoragePermission()` |
| `internal/server/mobile_api.go` | 处理 `PermissionError`；添加 `/api/permissions` handler |
| `internal/server/server.go` | 注册 `/api/permissions` 路由 |
| `app/encv-mobile/src/api/encv.ts` | 添加 `checkBackendPermissions()`；`listFiles` 识别权限错误 |
| `app/encv-mobile/src/views/Files.vue` | 重构 `loadFiles`（直接请求+自动重试）；添加权限状态 UI；监听后端就绪事件 |
| `app/encv-mobile/src/App.vue` | 首次启动自动申请通知+存储权限 |
| `app/encv-mobile/src/composables/useI18n.ts` | 添加权限相关 i18n 文案 |
