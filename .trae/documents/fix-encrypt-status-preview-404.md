# 修复加密状态实时更新 + 加密文件预览识别 + 文件夹404错误处理

## 三个问题的根因分析

---

### 问题 1：加密状态没有实时更新

**根因**：后端 `TaskManager` 在任务状态变化时 **从未通过 WebSocket 广播事件**。

前端 Tasks.vue 监听了三个事件：
```ts
eventBus.on('task:update', onTaskUpdate)      // L345
eventBus.on('task:created', onTaskCreated)     // L346
eventBus.on('task:completed', onTaskCompleted) // L347
```

但后端 `task_manager.go` 中 **零条 `Broadcast` 调用**。`MobileService` 有 `wsHub` 字段，`TaskManager` 没有引用它。

同样，Files.vue 监听 `file:change` 事件（L586），但后端加密/解密完成后也从未广播此事件 → **文件列表不会自动刷新**，加密后的 .encv 文件不会出现，原文件不会消失。

---

### 问题 2：加密文件没有正确识别预览

**根因**：`FileInfo` 结构体（[mobile_service.go:45-51](internal/service/mobile_service.go#L45-L51)）**没有 `isEncrypted` 字段**。

```go
type FileInfo struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    IsDirectory bool   `json:"isDirectory"`
    Size        int64  `json:"size"`
    Modified    string `json:"modified"`
    // ❌ 没有 IsEncrypted 字段
}
```

前端 `getFileCategory()` 仅通过扩展名 `.encv` 判断（[encv.ts:272](app/encv-mobile/src/api/encv.ts#L272)），但加密文件的实际扩展名由插件配置决定（如 `.sccgv`、`.sccga`、`.sccgt` 等），**不是 `.encv`**。

同时 `handleFileClick()` 对 `encrypted` 类型的文件传 `mimeType='application/x-encv'`（[Files.vue:327](app/encv-mobile/src/views/Files.vue#L327)），但 PlayerActivityLynx 只识别 `video/*` 和 `audio/*` → 加密文件无法正确打开播放器。

---

### 问题 3：文件夹删除后点击 404 → 前端误判为服务器离线

**根因**：`loadFiles()` 的重试逻辑（[Files.vue:240-271](app/encv-mobile/src/views/Files.vue#L240-L271)）将 **所有 HTTP 错误**（包括 404）都视为"服务器未就绪"并重试：

```ts
for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
  try {
    files.value = await listFiles(currentPath.value)
    serverOnline.value = true
    return
  } catch (error) {
    if (error instanceof PermissionDeniedError) { ... return }
    // ❌ 404 也走到这里，被当作连接问题重试
    connecting.value = true
    await new Promise(r => setTimeout(r, RETRY_DELAY))
  }
}
serverOnline.value = false  // ← 15次重试后标记服务器离线
```

后端 `listFiles` 对不存在的路径返回 `NotFoundError` → HTTP 404（[mobile_service.go:94-95](internal/service/mobile_service.go#L94-L95)），但前端 `listFiles()` 只处理了 403 PermissionDenied，**404 被当作通用错误**（[encv.ts:57-67](app/encv-mobile/src/api/encv.ts#L57-L67)）。

---

## 修复方案

### Step 1：后端 TaskManager 增加 WebSocket 广播

**文件**：`internal/service/task_manager.go`

1. 给 `TaskManager` 添加 `wsHub *WSHub` 字段
2. `NewTaskManager` 接收 `wsHub` 参数
3. 在以下位置添加 Broadcast 调用：
   - `Create()` → `wsHub.Broadcast("task:created", task)`
   - `processTask()` 开始时 → `wsHub.Broadcast("task:update", {id, status:"running", progress:0})`
   - `processEncrypt()` / `processDecrypt()` 成功后 → `wsHub.Broadcast("task:completed", {id, status:"completed"})`
   - `failTask()` → `wsHub.Broadcast("task:completed", {id, status:"failed", error:errMsg})`
   - `Cancel()` → `wsHub.Broadcast("task:update", {id, status:task.Status})`
   - `Retry()` → `wsHub.Broadcast("task:update", {id, status:"queued"})`
4. 加密/解密完成后额外广播 `wsHub.Broadcast("file:change", {path: task.SourcePath})`

**文件**：`internal/service/mobile_service.go`

`NewMobileService` 中将 `wsHub` 传给 `NewTaskManager`：
```go
taskManager: NewTaskManager(servingDir, cfg, wsHub),
```

### Step 2：FileInfo 增加 IsEncrypted 字段

**文件**：`internal/service/mobile_service.go`

1. `FileInfo` 结构体增加 `IsEncrypted bool` 字段：
```go
type FileInfo struct {
    Name        string `json:"name"`
    Path        string `json:"path"`
    IsDirectory bool   `json:"isDirectory"`
    IsEncrypted bool   `json:"isEncrypted"`
    Size        int64  `json:"size"`
    Modified    string `json:"modified"`
}
```

2. `ListFiles()` 中对非目录文件调用 `detector.DetectContainer()` 检测加密状态：
```go
isEncrypted := false
if !entry.IsDir() {
    _, detectErr := detector.DetectContainer(absPath)
    if detectErr == nil {
        isEncrypted = true
    }
}
```

**注意性能**：`DetectContainer` 需要打开文件读尾部 footer，对大目录可能较慢。优化方案：
- 先通过扩展名快速过滤（只对已知加密扩展名或非标准扩展名检测）
- 或使用 `detector.IsEncvContainerFromBytes()` 只读最后 N 字节

**文件**：`app/encv-mobile/src/api/encv.ts`

1. `FileItem` 接口增加 `isEncrypted?: boolean`
2. `getFileCategory()` 优先使用 `isEncrypted` 字段，回退到扩展名判断：
```ts
export function getFileCategory(name: string, isEncrypted?: boolean): FileCategory {
  if (isEncrypted) return 'encrypted'
  const ext = getFileExtension(name)
  if (ext === 'encv') return 'encrypted'
  // ... 原有逻辑
}
```

3. `handleFileClick()` 中 encrypted 类型使用正确的 mimeType：
```ts
if (category === 'encrypted') {
  // 加密文件可能是视频/音频，用 video/* 让播放器尝试
  const mimeType = 'video/*'
  openInPlayer(file.path, file.name, mimeType)
}
```

### Step 3：前端正确处理 404 错误

**文件**：`app/encv-mobile/src/api/encv.ts`

1. 新增 `NotFoundError` 类：
```ts
export class NotFoundError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'NotFoundError'
  }
}
```

2. `listFiles()` 中处理 404：
```ts
if (response.status === 404) {
  const data = await response.json().catch(() => ({}))
  throw new NotFoundError(data.error || 'Not found')
}
```

**文件**：`app/encv-mobile/src/views/Files.vue`

1. `loadFiles()` 中区分 404 和真正的连接错误：
```ts
} catch (error) {
  if (error instanceof PermissionDeniedError) { ... return }
  if (error instanceof NotFoundError) {
    // 路径不存在 → 导航到父目录或根目录
    serverOnline.value = true
    if (currentPath.value !== '/') {
      showToast({ message: t('files.pathNotFound'), duration: 2000, color: 'warning' })
      goUp()
    }
    return
  }
  // 其他错误才重试
  ...
}
```

2. `handleRefresh()` 同样处理 NotFoundError

### Step 4：构建验证

```bash
cd /workspace && go build ./...
cd /workspace/app/encv-mobile && npm run build 2>&1 | tail -5
```
