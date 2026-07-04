# 移动端三问题修复计划

## 架构背景：双端插件初始化不一致

### 桌面端 CLI 流程（正确）
```
encrypt-v2 /path/to/file
  → encv.Init(rootCtx)           // 1次初始化：BuildFullPluginSettings + InitializePlugins
  → encv.EncryptPathV2(rootCtx)  // 直接用已初始化的 plugin，不再 Init
```

### 移动端 Server 流程（有 bug）
```
start
  → encv.Init(rootCtx)                    // 启动时初始化所有插件 ✅
  → NewServer(rootCtx) → NewTaskManager(cfg)

用户点击加密:
  → POST /api/tasks {type:"encrypt", ...}
  → TaskManager.processEncrypt()
    → plugin.FindEncryptingPlugin()       // 拿到已初始化的 TextPlugin 实例
    → plugin.Initialize(cfgCtx)           // ❌ 重新初始化！cfgCtx ≠ rootCtx，缓存失效
    → GetPluginSettingsFor(cfgCtx, "text") // ❌ 找不到 settings（或找到错误的）
```

### 关键证据
- [registry.go:376](file:///workspace/internal/v2/plugins/registry.go#L376): `EncryptFileWithPlugin` 中 `plugin.Initialize(ctx)` **已注释掉**
- [registry.go:410](file:///workspace/internal/v2/plugins/registry.go#L410): `DecryptContainerWithPlugin` 中同样**已注释掉**
- 说明核心加密/解密函数**不需要也不应该**重复初始化插件
- 但 TaskManager 违反了这个约定

---

## 问题 1: 加密报错 "could not get settings for plugin text"

**根因**: `processEncrypt` / `processDecrypt` 冗余调用 `plugin.Initialize(cfgCtx)` 

TextPlugin 有缓存优化 ([text/plugin.go:91-93](file:///workspace/internal/v2/plugins/text/plugin.go#L91-L93)):
```go
if ctx == p.ctx {
    return nil  // 避免重复初始化
}
```
- 启动时 `p.ctx = rootCtx`（来自 `encv.Init`）
- 任务执行时 `cfgCtx = config.NewContext(context.Background(), tm.cfg)` 是**全新 context**
- `cfgCtx != p.ctx` → 缓存未命中 → 尝试重新初始化
- `config.FromContext(cfgCtx)` 返回 `tm.cfg`
- 如果 `tm.cfg.PluginSettings` 因某种原因不完整 → 报错

**修复**: 删除 `processEncrypt` 和 `processDecrypt` 中的冗余 `plugin.Initialize(cfgCtx)` 调用，
与 `EncryptFileWithPlugin` / `DecryptContainerWithPlugin` 保持一致。

**涉及文件**:
- [task_manager.go:414-417](file:///workspace/internal/service/task_manager.go#L414-L417) — 删除 processEncrypt 中的 Initialize
- [task_manager.go:603-606](file:///workspace/internal/service/task_manager.go#L603-L606) — 删除 processDecrypt 中的 Initialize

---

## 问题 2: 插件错误应广播到 DevLogs

**当前状态**: `failTask` 已调用 `slog.Error` + `broadcaster.Broadcast("task:completed")`
- `slog.Error` → `WSLogHandler` → WebSocket → DevLogs 后端 tab（理论上应该工作）
- `broadcaster.Broadcast` 发送的是 task 事件，不是 log 事件

**需要确认**:
1. [server.go:130](file:///workspace/internal/server/server.go#L130) 的 `wsMinLevel` 默认为 `slog.LevelInfo`，Error 级别能通过 ✓
2. 前端 DevLogs [DevLogs.vue:284-297](file:///workspace/app/encv-mobile/src/views/DevLogs.vue#L284-L297) 监听 `ws:message` 事件，格式匹配 ✓

**可能的问题**: 如果错误发生在 WebSocket 连接建立之前（如启动后第一个任务），消息会丢失。或者 slog 的 message 格式与前端期望不匹配。

**修复**: 在 `failTask` 中增加显式的 WSHub broadcast（log 类型），确保格式与 DevLogs 一致：

```go
if tm.broadcaster != nil {
    tm.broadcaster.Broadcast("log", map[string]interface{}{
        "level":   "error",
        "message": friendlyMsg,
    })
}
```

**涉及文件**: [task_manager.go:666-673](file:///workspace/internal/service/task_manager.go#L666-L673)

---

## 问题 3: 未知类型文件卡加载中

**文件**: [FilePreview.vue:203-295](file:///workspace/app/encv-mobile/src/views/FilePreview.vue#L203-L295)

**分析 `loadFile()` 所有路径**:

| 路径 | loading 关闭? | 位置 |
|------|-------------|------|
| 加密文件 + 是 ENCV 容器 | ✅ finally | L267-269 |
| 加密文件 + 非 ENCV 容器 | ✅ finally | L267-269 |
| 加密文件 + fetchFileInfo 异常 | ✅ catch+finally | L256-269 |
| 非加密文件 + determinePreviewType 成功 | ✅ finally | L292-294 |
| 非加密文件 + determinePreviewType 异常 | ✅ catch+finally | L289-294 |

**看起来所有路径都有 finally 关闭 loading**。但有一个隐藏风险：

[determinePreviewType()](file:///workspace/app/encv-mobile/src/views/FilePreview.vue#L185-L201) 内部调用 `fetchTextPreviewExts()`：
```javascript
const textExts = await fetchTextPreviewExts()  // ← API 请求
```

[fetchTextPreviewExts()](file:///workspace/app/encv-mobile/src/api/encv.ts#L352-L364) 没有**超时控制**：
```typescript
export async function fetchTextPreviewExts(): Promise<Set<string>> {
    if (cachedTextExts) return cachedTextExts
    const response = await fetch(`${baseUrl}/api/file/text-preview-exts`)  // ← 无超时!
    ...
}
```

如果 `/api/file/text-preview-exts` 端点挂起或响应慢，整个 `loadFile()` 会卡住，loading 永远不会关闭。

**修复**:
1. 给 `fetchTextPreviewExts()` 加 AbortController 超时（5秒）
2. 超时时 fallback 到空 Set（等同于"没有文本扩展名"→ 文件被判定为 unsupported）

**涉及文件**: [encv.ts:352-364](file:///workspace/app/encv-mobile/src/api/encv.ts#L352-L364)

---

## 实施步骤

### Step 1: Fix 核心加密 bug（删除冗余 Initialize）
1. 编辑 `task_manager.go` processEncrypt：删除 L414-417 的 `plugin.Initialize(cfgCtx)`
2. 编辑 `task_manager.go` processDecrypt：删除 L603-606 的 `plugin.Initialize(cfgCtx)`
3. `go build ./cmd/encv/` 编译验证

### Step 2: Fix DevLogs 广播
1. 编辑 `task_manager.go` failTask：在 broadcaster 处增加 `"log"` 类型消息
2. 编译验证

### Step 3: Fix 未知类型卡加载
1. 编辑 `encv.ts` fetchTextPreviewExts：加 AbortController 5秒超时
2. `vue-tsc --noEmit && vite build` 验证

### Step 4: 端到端验证
1. 启动服务，用移动端触发加密 → 不再报 plugin settings 错误
2. 预览 ENCV 容器 → 仍然正常
3. 打开未知类型文件（如图片）→ 显示 "unsupported" 不再卡加载
4. 触发一个会失败的加密任务 → DevLogs 后端 tab 能看到错误信息
