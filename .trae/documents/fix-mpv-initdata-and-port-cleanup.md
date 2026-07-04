# 修复 MPV 播放失败 + 清理废弃端口配置

## 问题 1：MPV 播放失败 — initData={} 导致播放地址为空

### 根因分析

**f53917d（可以播放的版本）的工作流程：**
1. `PlayerActivityLynx.buildInitDataJson()` 只传 `filePath`（不传 `streamUrl`）
2. `PlayerApp.tsx` 的 `startPlayback()` 通过 `GoBackendModule.getBackendStatus()` → `startBackend()` → `getStreamUrl()` → `MpvPlayerModule.play()` 四步完成播放
3. `getInitData()` 从 `lynx.__globalProps` 读取数据

**当前版本的错误：**
1. 我们在 `buildInitDataJson()` 中添加了 `streamUrl` 字段（依赖 `EncvGoService.lastKnownPort > 0`）
2. 但 `PlayerApp.tsx` 仍然从 `lynx.__globalProps` 读取 initData
3. **关键问题**：Lynx 框架中 `renderTemplateUrl(url, initData)` 的第二个参数是 `initData`，不是 `globalProps`！`initData` 和 `globalProps` 是两个独立的概念：
   - `initData` → 前端用 `useInitData()` 获取
   - `globalProps` → 前端用 `useGlobalProps()` 或 `lynx.__globalProps` 获取
4. 因此 `lynx.__globalProps` 始终为空对象 `{}`，导致 `initStreamUrl` 和 `initFilePath` 都为空

### 修复方案

**核心原则：ArtPlayer 和 MPV 统一使用 Go 后端流式播放**

ArtPlayer 已经通过 `getFileStreamUrl()` → `baseUrl/stream?path=...` 使用 Go 后端流式播放。MPV 也必须走同样的路径，通过 `GoBackendModule.getStreamUrl()` 获取 Go 后端的流 URL，而不是直接播放文件路径。这样才能支持加密视频的流式预览和 seek。

**核心修复：PlayerApp.tsx 使用 `useInitData()` 替代手动 `getInitData()`，恢复四步播放流程**

1. **`PlayerApp.tsx`**：
   - 导入 `useInitData` from `@lynx-js/react`
   - 用 `useInitData()` 替代手动 `getInitData()` 读取 `lynx.__globalProps`
   - 恢复 f53917d 版本的四步播放流程（`getBackendStatus` → `startBackend` → `getStreamUrl` → `mpv.play`），这是唯一可靠的流程
   - 移除 `initStreamUrl`、`resolvedStreamUrl` 等状态变量
   - 保留错误分类 `classifyError()` 和 `setError()` 统一错误处理
   - 保留 `lynxLog` 日志推送到 DevLogs

2. **`GoBackendModule.getStreamUrl`**：
   - `Uri.encode(path)` → `Uri.encode(path, "/")`，保留路径分隔符 `/` 不被编码

3. **`PlayerActivityLynx.buildInitDataJson()`**：
   - 移除 `streamUrl` 字段（不再需要，PlayerApp 会通过 `GoBackendModule.getStreamUrl` 动态获取）
   - 只保留 `filePath`、`fileName`、`mimeType`、`isExternal`、`mediaType`

4. **`PlayerOverlayManager.buildInitDataJson()`**：
   - 同样移除 `streamUrl`，添加 `filePath` 字段
   - 保持与 PlayerActivityLynx 一致的 initData 结构

### 具体步骤

#### Step 1: 修改 PlayerApp.tsx — 使用 useInitData + 恢复四步播放流程
- 导入 `useInitData` from `@lynx-js/react`
- 用 `const initData = useInitData()` 替代 `getInitData()`
- 恢复 f53917d 的 `startPlayback` 逻辑：接受 `{ filePath, isExternal, mediaType }` 参数
  - Step 1: `getBackendStatus()` 检查后端状态
  - Step 2: 如果需要，`startBackend()` 启动后端
  - Step 3: `getStreamUrl(filePath, isExternal)` 获取流 URL
  - Step 4: `MpvPlayerModule.play(streamUrl)` 播放
- 自动播放 useEffect：`if (filePath) startPlayback({ filePath, isExternal, mediaType })`
- 移除 `initStreamUrl`、`resolvedStreamUrl` 等状态变量
- 保留错误分类 `classifyError()` 和 `setError()` 统一错误处理
- 保留 `lynxLog` 日志推送到 DevLogs

#### Step 2: 修改 GoBackendModule.kt — Uri.encode 保留 /
- `Uri.encode(path)` → `Uri.encode(path, "/")`

#### Step 3: 修改 PlayerActivityLynx.kt — 移除 streamUrl
- `buildInitDataJson()` 移除 `streamUrl` 字段和构建逻辑
- 只保留 `filePath`、`fileName`、`mimeType`、`isExternal`、`mediaType`

#### Step 4: 修改 PlayerOverlayManager.kt — 统一 initData 结构
- `buildInitDataJson()` 移除 `streamUrl`，添加 `filePath` 参数和字段

#### Step 5: 构建验证
- `vue-tsc --noEmit && vite build`
- `go build ./internal/...`

---

## 问题 2：gin 重构后端口统一，旧配置残留

### 根因分析

gin 重构后，所有路由（主服务、Admin、WebDAV、OpenList 代理）都注册在同一个 Gin engine 上，只有一个端口（`server.port`）。但：

1. **Go 类型定义**中仍保留 `AdminServer.Port` 和 `WebdavServer.Port` 字段
2. **config.mobile.json** 中仍有 `admin.port: 18080` 和 `webdav.port: 12340`
3. **schema.json** 中仍定义了这些端口字段
4. **前端 Settings 页面**会根据 schema 自动渲染所有字段，包括 `admin.port` 和 `webdav.port`
5. 用户看到这些端口设置会误以为它们生效，实际上完全无用

### 修复方案

**清理废弃端口字段，避免用户混淆**

1. **Go 类型 `types.go`**：
   - `AdminServer` 移除 `Port` 字段，只保留 `Password`
   - `WebdavServer` 移除 `Port` 字段，保留其他字段

2. **`config.go` 的 `DefaultConfig()`**：
   - 移除 `Webdav.Port` 默认值（2299）

3. **`schema.json`**：
   - `AdminServer` 定义移除 `port` 属性
   - `WebdavServer` 定义移除 `port` 属性
   - 相应的 `required` 数组也移除 `port`

4. **`config.mobile.json`**：
   - 移除 `admin.port` 和 `webdav.port` 字段

5. **`ServerDetail.vue`**：
   - WebDAV URL 不再使用独立端口，改为使用主服务端口 + webdav root 路径

6. **`server_finder.go`**：
   - `FindAdminServer` 函数已无用（admin 路由在主服务上），标记为废弃

### 具体步骤

#### Step 6: 修改 Go 类型 — 移除废弃端口字段
- `internal/v2/types/types.go`: `AdminServer` 移除 `Port`，`WebdavServer` 移除 `Port`
- `internal/config/config.go`: `DefaultConfig()` 移除 `Webdav.Port` 默认值

#### Step 7: 修改 schema.json — 移除废弃端口定义
- `app/encv-mobile/src/config/schema.json`: AdminServer 和 WebdavServer 移除 port 属性和 required

#### Step 8: 修改 config.mobile.json — 移除废弃端口值
- 移除 `admin.port` 和 `webdav.port`

#### Step 9: 修改前端 ServerDetail.vue — 修正 WebDAV URL
- WebDAV URL 使用主服务端口 + webdav root 路径

#### Step 10: 清理 server_finder.go — 标记 FindAdminServer 废弃
- 添加注释说明该函数已不再使用

#### Step 11: 构建验证
- `vue-tsc --noEmit && vite build`
- `go build ./internal/...` 和 `go build ./cmd/...`
