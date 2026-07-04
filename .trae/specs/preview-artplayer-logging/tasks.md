# Tasks

- [x] Task 1: 安装 artplayer 依赖
  - [x] SubTask 1.1: 在 `app/encv-mobile/` 目录执行 `npm install artplayer`
  - [x] SubTask 1.2: 确认 `package.json` 中出现 artplayer 依赖

- [x] Task 2: 重写 Player.vue 为 ArtPlayer 播放器
  - [x] SubTask 2.1: 视频播放使用 ArtPlayer，配置 autoplay/autoSize/playsInline/theme/mutex
  - [x] SubTask 2.2: 音频播放保留原生 `<audio>` 标签
  - [x] SubTask 2.3: 加密视频通过 `/stream?path=...` 端点播放
  - [x] SubTask 2.4: 生命周期管理：onMounted 创建实例，onBeforeUnmount 销毁，路由变化 switchUrl
  - [x] SubTask 2.5: 播放错误处理，显示 toast 提示
  - [x] SubTask 2.6: 添加 console.info/error 操作日志

- [x] Task 3: 重写 FilePreview.vue 为多类型预览
  - [x] SubTask 3.1: 图片类型使用 `<img :src="streamUrl">` 全屏展示
  - [x] SubTask 3.2: PDF 类型使用 `<iframe :src="streamUrl">` 嵌入展示
  - [x] SubTask 3.3: 文本类型保持现有 `<pre>` 预览逻辑
  - [x] SubTask 3.4: 二进制/过大文件显示元信息 + "不支持预览"提示
  - [x] SubTask 3.5: 添加 console.info/error 操作日志

- [x] Task 4: 调整 Files.vue 的 handleFileClick 路由
  - [x] SubTask 4.1: video/audio/encrypted → `/tabs/player`
  - [x] SubTask 4.2: image/document/other → `/tabs/preview`
  - [x] SubTask 4.3: 添加 console.info 操作日志

- [x] Task 5: 创建 HTTP 请求日志中间件
  - [x] SubTask 5.1: 新建 `internal/middleware/logging.go`
  - [x] SubTask 5.2: 实现 `LoggingMiddleware`，记录 method/path/status/duration
  - [x] SubTask 5.3: 实现 `responseWriter` 包装器捕获状态码

- [x] Task 6: 创建 slog → WebSocket 日志桥接
  - [x] SubTask 6.1: 新建 `internal/server/ws_log_handler.go`
  - [x] SubTask 6.2: 实现 `WSLogHandler`，包装默认 slog handler + WSHub 推送
  - [x] SubTask 6.3: 日志消息格式 `{"type":"log","level":"info","message":"...","timestamp":"..."}`
  - [x] SubTask 6.4: 仅推送 Info 及以上级别日志到 WebSocket

- [x] Task 7: 修改 server.go 注册日志中间件和 WSLogHandler
  - [x] SubTask 7.1: 在路由注册前添加 LoggingMiddleware
  - [x] SubTask 7.2: 在 Start() 中初始化 WSLogHandler 并设置为 slog 默认 handler

- [x] Task 8: 后端 API 和 Service 添加日志
  - [x] SubTask 8.1: `mobile_api.go` 各 handler 添加 slog.Info 日志（请求参数和结果）
  - [x] SubTask 8.2: `mobile_service.go` 各方法添加 slog.Info/Warn 日志

- [x] Task 9: 前端 API 调用添加日志
  - [x] SubTask 9.1: `api/encv.ts` 各函数添加 console.info/warn/error

- [x] Task 10: 前端组件添加操作日志
  - [x] SubTask 10.1: `App.vue` 权限申请添加 console.info/warn 日志
  - [x] SubTask 10.2: `DevLogs.vue` 解析后端日志的 level 字段

- [x] Task 11: 验证
  - [x] SubTask 11.1: Go 编译通过 `go build ./cmd/encv/`
  - [x] SubTask 11.2: 前端构建通过 `npx vite build`

# Task Dependencies

- [Task 2] depends on [Task 1] (需要 artplayer 依赖)
- [Task 7] depends on [Task 5, Task 6] (注册中间件和 WSLogHandler)
- [Task 8] depends on [Task 7] (WSLogHandler 初始化后日志才能推送到 WS)
- [Task 11] depends on [Task 1-10]
