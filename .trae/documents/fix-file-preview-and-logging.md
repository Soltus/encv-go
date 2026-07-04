# 实现文件预览 + ArtPlayer 播放器 + 增加前后端日志

## 问题 1：预览文件没有实现

### 现状

`FilePreview.vue` 只支持文本预览（`<pre>` 标签 + `/api/file`），缺少图片、PDF 等类型。

### 方案

`FilePreview.vue` 根据文件类型分模式预览：

| 文件类型 | 预览方式 | 数据源 |
|---------|---------|--------|
| image (jpg/png/gif/webp/svg) | `<img :src="streamUrl">` 全屏展示 | `/stream?path=...` |
| pdf | `<iframe :src="streamUrl">` 嵌入展示 | `/stream?path=...` |
| text (txt/log/json/xml/csv 等) | 现有 `<pre>` 文本预览 | `/api/file?path=...` |
| 二进制/过大 | 文件元信息 + "不支持预览"提示 | `/api/file?path=...` |

`Files.vue` 的 `handleFileClick` 路由调整：
- video/audio/encrypted → Player
- image/pdf/document/other → Preview

## 问题 2：基于 ArtPlayer.js 二次开发播放器

### 现状

当前 `Player.vue` 使用原生 `<video>` 和 `<audio>` 标签，功能简陋：
- 没有自定义控制栏
- 没有手势操作（移动端必需）
- 没有倍速播放
- 没有画中画
- 没有字幕支持
- 加密视频播放体验差

### 方案

**安装 artplayer**：`npm install artplayer`

**Player.vue 重写为 ArtPlayer**：
- 视频播放：使用 ArtPlayer 替代原生 `<video>`
- 音频播放：保留原生 `<audio>`（ArtPlayer 不支持纯音频）
- 加密视频：后端 `/stream?path=...` 已支持解密流式传输，ArtPlayer 直接使用该 URL
- ArtPlayer 配置：
  - `container`: ref 绑定 DOM
  - `url`: `getFileStreamUrl(filePath)`
  - `autoplay: true`
  - `autoSize: true`
  - `autoMini: true`（滚动自动迷你化）
  - `mutex: true`（互斥播放）
  - `playsInline: true`
  - `theme: '#ffad00'`
  - `volume: 0.7`
  - 移动端手势自动启用（ArtPlayer 内置）

**生命周期管理**：
- `onMounted` → `new Artplayer(options)`
- `onBeforeUnmount` → `art.destroy()`
- 路由参数变化时 → `art.switchUrl(newUrl)`

### 实施步骤

1. `npm install artplayer`
2. 重写 `Player.vue`：
   - 视频用 ArtPlayer
   - 音频保留原生 `<audio>`
   - 加密视频走 `/stream` 端点（后端已支持解密）
   - 添加加载/错误状态处理

## 问题 3：前后端日志过少

### 现状

**后端**：
- `mobile_api.go`：handler 几乎没有日志
- `mobile_service.go`：只有 Error 级别，缺少 Info/Warn
- 没有 HTTP 请求日志中间件
- slog 日志没有桥接到 WebSocket（前端 DevLogs 看不到后端日志）

**前端**：
- `api/encv.ts`：所有 API 调用没有 console 日志
- 各组件没有操作日志
- `DevLogs.vue`：后端日志标签页只能收到 WebSocket 消息，且所有消息都标记为 info 级别

### 方案

**A. 后端：HTTP 请求日志中间件**

创建 `internal/middleware/logging.go`，记录每个请求的方法、路径、状态码、耗时。

**B. 后端：slog → WebSocket 桥接**

创建自定义 `slog.Handler`，在输出到 stderr 的同时，将日志推送到 WSHub。日志消息格式：
```json
{"type": "log", "level": "info", "message": "...", "timestamp": "14:30:00"}
```

**C. 后端：给各函数添加日志**

- `mobile_api.go`：Info 记录请求参数和结果
- `mobile_service.go`：Info 记录操作，Warn 记录异常
- `server.go`：Info 记录启动和关键状态

**D. 前端：给 API 调用添加日志**

`api/encv.ts` 各函数添加 `console.info/warn/error`。

**E. 前端：给组件添加操作日志**

Files.vue、Player.vue、FilePreview.vue、App.vue 添加关键操作日志。

**F. 前端：DevLogs 解析后端日志 level**

修改 `onWsMessage` 解析 `{type: "log", level: "warn", message: "..."}` 格式。

### 实施步骤

1. 创建 `internal/middleware/logging.go`：HTTP 请求日志中间件
2. 创建 `internal/server/ws_log_handler.go`：slog → WebSocket 桥接
3. 修改 `server.go`：注册日志中间件 + 初始化 WSLogHandler
4. 修改 `mobile_api.go`：各 handler 添加日志
5. 修改 `mobile_service.go`：各方法添加日志
6. 修改 `api/encv.ts`：API 调用添加 console 日志
7. 修改 `Files.vue`：添加操作日志 + handleFileClick 路由调整
8. 重写 `FilePreview.vue`：多类型预览 + 日志
9. 重写 `Player.vue`：ArtPlayer + 日志
10. 修改 `App.vue`：权限申请日志
11. 修改 `DevLogs.vue`：解析后端日志 level

## 文件变更清单

| 文件 | 变更 |
|------|------|
| `internal/middleware/logging.go` | 新建：HTTP 请求日志中间件 |
| `internal/server/ws_log_handler.go` | 新建：slog → WebSocket 桥接 |
| `internal/server/server.go` | 注册日志中间件 + 初始化 WSLogHandler |
| `internal/server/mobile_api.go` | 各 handler 添加日志 |
| `internal/service/mobile_service.go` | 各方法添加日志 |
| `app/encv-mobile/package.json` | 添加 artplayer 依赖 |
| `app/encv-mobile/src/api/encv.ts` | API 调用添加 console 日志 |
| `app/encv-mobile/src/views/Files.vue` | 操作日志 + handleFileClick 路由调整 |
| `app/encv-mobile/src/views/FilePreview.vue` | 多类型预览 + 日志 |
| `app/encv-mobile/src/views/Player.vue` | ArtPlayer 重写 + 日志 |
| `app/encv-mobile/src/views/App.vue` | 权限申请日志 |
| `app/encv-mobile/src/views/DevLogs.vue` | 解析后端日志 level |
