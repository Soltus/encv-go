# 七项运行时缺陷修复 Spec

## Why

CI 构建通过后，用户实际使用暴露了 7 个运行时缺陷，涵盖前端 API 路径错误、UI 性能、功能缺失、数据显示、后端崩溃残留、配置优先级等多个层面。

## What Changes

- **修复 createDirectory API 路径 404**：前端 URL 缺少 `/api` 前缀
- **优化文本 iframe 预览性能**：替换 iframe 为轻量渲染方案，解决卡顿和无响应问题
- **略缩图索引升级为插件聚合索引**（功能增强）
- **修复插件安装状态卡死**：安装完成后状态不更新 / 反射方法名不匹配
- **修复 v4 容器版本号和容器 ID 乱码**：sanitizeManifestMap 未覆盖顶层字段
- **修复加密崩溃 + 中间产物残留**：添加 panic 恢复 + 临时文件清理机制
- **修复移动端 output_path 配置未生效**：设置页显示错误字段 + 优先级逻辑确认

## Impact

- Affected code:
  - `app/encv-mobile/src/api/encv.ts` — createDirectory 路径修复
  - `app/encv-mobile/src/views/FilePreview.vue` — iframe 预览优化
  - `app/encv-mobile/src/views/ExtensionsPage.vue` — 安装状态管理
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 安装回调可靠性
  - `internal/service/mobile_service.go` — 版本/容器 ID 清洗 + GetFileInfo 容错
  - `internal/v2/plugins/video/content_preprocessor.go` — panic 恢复 + 临时文件清理
  - `internal/config/config.go` — 确认 mobile.output_path 优先级（可能无需改）

## ADDED Requirements

### Requirement 1: createDirectory API 路径正确

#### Scenario: 解密文件夹选择中新建文件夹
- **WHEN** 用户在 FilePickerModal 中点击新建文件夹并确认
- **THEN** 前端发送 POST 到正确路径 `/api/files/mkdir`
- **AND** 后端返回 200 或业务错误（非 404）

### Requirement 2: 文本预览不阻塞主线程

#### Scenario: 预览文本文件
- **WHEN** 用户打开 .txt/.log/.md 等文本文件
- **THEN** 预览内容以轻量方式渲染（非 iframe）
- **AND** 返回按钮、手势操作响应正常无卡顿

### Requirement 3: 插件安装状态正确反馈

#### Scenario: 从本地选择 APK 安装插件
- **WHEN** 用户在 ExtensionsPage 点击「选择 APK」→ 选择文件 → 安装完成
- **THEN** 「安装中」状态变为「已安装」
- **AND** 刷新列表后状态保持一致
- **AND** 安装失败时显示具体错误信息

### Requirement 4: v4 容器版本号和容器 ID 可读

#### Scenario: 查看 v4 加密容器文件信息
- **WHEN** 用户在 FileInfo/FilePreview 页面查看 v4 容器
- **THEN** 版本号显示为纯数字（如 `V4`）
- **AND** 容器 ID 显示为可打印 ASCII 字符串（如 UUID 格式或 `(auto)`）
- **AND** 无乱码、无控制字符

### Requirement 5: 加密失败不崩溃、不留垃圾文件

#### Scenario: MP4 文件加密预处理阶段失败
- **WHEN** remuxMP4ForFastStart 或其他预处理步骤抛出 panic 或返回 error
- **THEN** 后端进程不崩溃（panic 被 recover）
- **AND** 用户可见目录中不存在 `encv-pre-*.mp4` / `encv-pre-*.mkv` 临时文件
- **AND** 任务状态正确标记为失败并返回错误消息

### Requirement 6: 移动端 output_path 配置生效且显示正确

#### Scenario: 移动端查看输出目录配置
- **WHEN** config.user.json 中同时存在顶层 `"output_path": ""` 和 `"mobile": {"output_path": "/storage/emulated/0/encv-output"}`
- **THEN** 设置界面显示移动端的 `/storage/emulated/0/encv-output`
- **AND** 后端实际使用 mobile.output_path 值（优先于顶层空值）

## MODIFIED Requirements

### Requirement: createDirectory API 调用路径

`createDirectory()` 函数的 fetch URL 从 `${getApiBaseUrl()}/files/mkdir` 改为 `${getApiBaseUrl()}/api/files/mkdir`，与其他 API 函数（listFiles、getFileStream 等）保持一致的 `/api` 前缀。

### Requirement: 文本预览渲染方式

FilePreview.vue 中 `previewType === 'text'` 的渲染从 `<iframe :src="textPreviewUrl">` 改为直接 fetch 内容 + `<pre>` 标签渲染，避免 iframe 的线程隔离和事件拦截问题。

### Requirement: GetFileInfo 容器字段清洗范围

`sanitizeManifestMap()` 的清洗逻辑扩展到覆盖 `result.Container` 的顶层字符串字段（version 已是 int 无需处理、container_id 需要清洗），不仅在 manifest 嵌套 map 内递归清洗。

### Requirement: content_preprocessor 错误恢复

`Preprocess()` 及其子函数（remapMP4ForFastStart、transcodeToFastStartMP4 等）添加：
1. `defer` 清理已创建的临时文件（注册清理函数，成功时取消注册）
2. 顶层 panic recover，确保不杀死整个后端进程
