# 文件预览 + ArtPlayer 播放器 + 前后端日志增强 Spec

## Why

当前 encv-mobile 存在三个核心缺陷：1）文件预览仅支持纯文本，图片/PDF 等无法预览；2）视频播放器使用原生 `<video>` 标签，缺少手势操作、倍速、画中画等移动端必需功能，且加密视频播放体验差；3）前后端日志严重不足，后端 slog 日志未桥接到前端 DevLogs 页面，排查问题困难。

## What Changes

- 重写 `FilePreview.vue`，根据文件类型分模式预览（图片/PDF/文本/不支持）
- 安装 artplayer 依赖，重写 `Player.vue` 为 ArtPlayer 播放器，支持所有视频格式（含加密视频）
- 调整 `Files.vue` 的 `handleFileClick` 路由逻辑，image/document/other 类型路由到 Preview
- 新建 `internal/middleware/logging.go`，HTTP 请求日志中间件
- 新建 `internal/server/ws_log_handler.go`，slog → WebSocket 桥接，让前端 DevLogs 可查看后端日志
- 修改 `server.go`，注册日志中间件 + 初始化 WSLogHandler
- 给 `mobile_api.go` 和 `mobile_service.go` 各函数添加 Info/Warn/Error 日志
- 给前端 `api/encv.ts` 各 API 函数添加 console.info/warn/error
- 给前端各组件添加关键操作日志
- 修改 `DevLogs.vue`，解析后端推送日志的 level 字段

## Impact

- Affected specs: `implement-mobile-backend-api`（API 行为不变，仅增加日志和 slog→WS 桥接）
- Affected code:
  - `app/encv-mobile/src/views/FilePreview.vue` — 重写为多类型预览
  - `app/encv-mobile/src/views/Player.vue` — 重写为 ArtPlayer
  - `app/encv-mobile/src/views/Files.vue` — handleFileClick 路由调整 + 日志
  - `app/encv-mobile/src/views/DevLogs.vue` — 解析后端日志 level
  - `app/encv-mobile/src/views/App.vue` — 权限申请日志
  - `app/encv-mobile/src/api/encv.ts` — API 调用日志
  - `app/encv-mobile/package.json` — 添加 artplayer 依赖
  - `internal/middleware/logging.go`（新建）— HTTP 请求日志中间件
  - `internal/server/ws_log_handler.go`（新建）— slog→WS 桥接
  - `internal/server/server.go` — 注册中间件 + WSLogHandler
  - `internal/server/mobile_api.go` — handler 日志
  - `internal/service/mobile_service.go` — service 日志

## ADDED Requirements

### Requirement: 多类型文件预览

系统 SHALL 在 `FilePreview.vue` 中根据文件类型提供不同的预览方式。

#### Scenario: 图片预览
- **WHEN** 用户点击图片文件（jpg/jpeg/png/gif/webp/svg）
- **THEN** 使用 `<img :src="streamUrl">` 全屏展示图片，数据源为 `/stream?path=...`

#### Scenario: PDF 预览
- **WHEN** 用户点击 PDF 文件
- **THEN** 使用 `<iframe :src="streamUrl">` 嵌入展示 PDF，数据源为 `/stream?path=...`

#### Scenario: 文本预览
- **WHEN** 用户点击文本文件（txt/log/json/xml/csv 等）
- **THEN** 使用现有 `<pre>` 标签展示文件内容，数据源为 `/api/file?path=...`

#### Scenario: 不支持预览的文件
- **WHEN** 用户点击二进制文件或文件过大（>2MB）
- **THEN** 显示文件元信息（名称、大小、路径）+ "不支持预览"提示

### Requirement: ArtPlayer 视频播放器

系统 SHALL 使用 ArtPlayer.js 替代原生 `<video>` 标签，提供完整的移动端视频播放体验。

#### Scenario: 普通视频播放
- **WHEN** 用户点击视频文件（mp4/mkv/avi/mov/webm 等）
- **THEN** 使用 ArtPlayer 播放，URL 为 `/stream?path=...`，支持手势操作、倍速、画中画

#### Scenario: 加密视频播放
- **WHEN** 用户点击加密视频文件（.encv）
- **THEN** 使用 ArtPlayer 播放，URL 为 `/stream?path=...`（后端已支持解密流式传输），播放体验与普通视频一致

#### Scenario: 音频播放
- **WHEN** 用户点击音频文件（mp3/flac/wav 等）
- **THEN** 保留原生 `<audio>` 标签播放（ArtPlayer 不支持纯音频）

#### Scenario: ArtPlayer 生命周期
- **WHEN** 组件挂载
- **THEN** 创建 ArtPlayer 实例，配置 autoplay/autoSize/playsInline/theme 等
- **WHEN** 组件卸载
- **THEN** 调用 `art.destroy()` 释放资源
- **WHEN** 路由参数变化（切换视频）
- **THEN** 调用 `art.switchUrl(newUrl)` 切换视频源

#### Scenario: 播放错误处理
- **WHEN** 视频加载或播放失败
- **THEN** 显示错误提示 toast

### Requirement: 文件点击路由

系统 SHALL 根据文件类型将用户导航到正确的页面。

#### Scenario: 视频/音频/加密文件
- **WHEN** 用户点击 video/audio/encrypted 类型文件
- **THEN** 导航到 `/tabs/player` 页面

#### Scenario: 其他文件
- **WHEN** 用户点击 image/document/other 类型文件
- **THEN** 导航到 `/tabs/preview` 页面

### Requirement: HTTP 请求日志中间件

系统 SHALL 提供 HTTP 请求日志中间件，记录每个请求的方法、路径、状态码、耗时。

#### Scenario: 请求日志记录
- **WHEN** 任何 HTTP 请求到达服务器
- **THEN** 在请求完成后以 Info 级别记录 `method`、`path`、`status`、`duration`

### Requirement: slog → WebSocket 日志桥接

系统 SHALL 将后端 slog 日志桥接到 WebSocket，使前端 DevLogs 页面可实时查看后端日志。

#### Scenario: 日志推送
- **WHEN** 后端调用 `slog.Info/Warn/Error`
- **THEN** 日志同时输出到 stderr 和 WebSocket，WebSocket 消息格式为 `{"type":"log","level":"info","message":"...","timestamp":"14:30:00"}`

#### Scenario: 日志级别过滤
- **WHEN** 日志级别低于 Info（如 Debug）
- **THEN** 不推送到 WebSocket（避免过多消息），仅输出到 stderr

### Requirement: 后端 API 日志

系统 SHALL 在移动端 API handler 和 service 方法中添加适当的日志。

#### Scenario: handler 日志
- **WHEN** API handler 处理请求
- **THEN** 以 Info 级别记录请求参数和处理结果

#### Scenario: service 日志
- **WHEN** service 方法执行操作
- **THEN** 以 Info 级别记录操作，Warn 级别记录异常情况

### Requirement: 前端 API 调用日志

系统 SHALL 在前端 API 调用中添加 console 日志。

#### Scenario: API 调用成功
- **WHEN** API 调用成功
- **THEN** 以 `console.info` 记录调用和结果摘要

#### Scenario: API 调用失败
- **WHEN** API 调用失败
- **THEN** 以 `console.error` 记录错误信息

### Requirement: 前端组件操作日志

系统 SHALL 在前端关键组件中添加操作日志。

#### Scenario: 文件操作日志
- **WHEN** 用户在 Files.vue 中导航、加载文件、遇到权限问题
- **THEN** 以 `console.info/warn` 记录操作

#### Scenario: 播放器日志
- **WHEN** 播放器加载视频、切换视频、遇到错误
- **THEN** 以 `console.info/error` 记录

#### Scenario: 权限申请日志
- **WHEN** App.vue 申请通知或存储权限
- **THEN** 以 `console.info/warn` 记录申请结果

### Requirement: DevLogs 后端日志级别解析

系统 SHALL 在 DevLogs.vue 中正确解析后端推送日志的 level 字段。

#### Scenario: 日志级别解析
- **WHEN** DevLogs 收到 WebSocket 消息 `{type: "log", level: "warn", message: "..."}`
- **THEN** 日志条目的 level 设置为 "warn"，而非硬编码为 "info"

## MODIFIED Requirements

### Requirement: FilePreview.vue

`FilePreview.vue` SHALL 从仅支持文本预览扩展为支持图片、PDF、文本、不支持预览四种模式，根据 `getFileCategory()` 判断文件类型选择预览方式。

### Requirement: Player.vue

`Player.vue` SHALL 从原生 `<video>/<audio>` 标签替换为 ArtPlayer（视频）+ 原生 `<audio>`（音频），加密视频通过 `/stream?path=...` 端点播放。

## REMOVED Requirements

无移除的需求。
