# 修复运行时三问题：ffprobe 执行失败 + 插件格式错误 + 文件列表性能

## Why
CI 构建通过、APK 签名正确、前端资源已修复后，实际设备运行暴露三个运行时问题：
1. ffprobe dlopen 成功（引擎状态绿色）但执行加密视频时 exit code 1，且 **stderr 未被捕获导致无法诊断根因**
2. MPV player 插件以 APK 格式打包安装失败，combolite 官方 demo 的插件格式可能不是 APK
3. 文件列表页面在手机大量文件（>5000）下卡在"加载中"，当前全量渲染无虚拟滚动/增量加载

## What Changes

### 问题 1：ffprobe stderr 未捕获 + 运行时诊断
- **`internal/utils/ffmpeg_dlopen.go`**：`CallFFprobeNative()` 当前传 `nil` 作为 `stderr_file` 参数（第253行），导致 ffprobe 失败时 stderr 输出丢失。需同时捕获 stdout 和 stderr。
- 错误消息 `"ffprobe failed (exit 1):"` 后面为空就是因为 stderr 没被捕获
- 可能的根因（按优先级）：Android Scoped Storage 权限、文件路径含中文字符、ffprobe 缺少编解码器

### 问题 2：combolite 插件格式修正
- 当前 CI 将 plugin-mpv-player 打包为 `.apk` 放入 host assets
- 需确认 combolite `aar2apk` 插件的正确输出格式和加载方式
- 如官方 demo 使用不同格式，需调整 CI 打包命令 + host app 加载逻辑

### 问题 3：文件列表性能优化
- 前端 `Files.vue` 全量 v-for 渲染，无虚拟滚动/分页/懒加载
- 后端 API 一次性返回全部文件列表，无分页或流式接口
- 方案：前端虚拟滚动（vue-virtual-scroller 或 @tanstack/vue-virtual）+ 后端可选分页/SSE 流式返回

## Impact
- Affected code:
  - `internal/utils/ffmpeg_dlopen.go` — ffprobe stderr 捕获修复
  - `.github/workflows/android.yml` — 插件打包格式调整
  - `app/encv-mobile/plugin-mpv-player/build.gradle.kts` — 插件构建配置
  - `app/encv-mobile/android/app/build.gradle.kts` — packagePlugins 配置
  - `app/encv-mobile/src/views/Files.vue` — 文件列表 UI 重构
  - `app/encv-mobile/src/composables/useFileList.ts` — 数据获取逻辑改造
  - 后端文件列表 API endpoint — 分页/流式支持

## ADDED Requirements

### Requirement: ffprobe 必须同时捕获 stdout 和 stderr
Go 后端调用 ffprobe 时必须同时重定向 stdout 和 stderr 到临时文件，确保失败时可诊断根因。

#### Scenario: ffprobe 失败时返回完整错误信息
- **WHEN** ffprobe 执行失败（exit code != 0）
- **THEN** 错误消息包含完整的 stderr 输出（而非空字符串）

### Requirement: 插件格式符合 combolite 规范
MPV player 插件的打包格式和加载方式必须与 combolite/aar2apk 官方 demo 一致。

#### Scenario: 插件可正常安装和加载
- **WHEN** 用户在 app 中安装 MPV player 插件
- **THEN** 插件成功加载，视频播放功能可用

### Requirement: 文件列表支持大数据量实时展示
文件列表页面在文件数量 >5000 时不应卡顿或长时间显示"加载中"。

#### Scenario: 大量文件目录快速显示
- **WHEN** 用户进入包含 >5000 个文件的目录
- **THEN** 首屏数据在 <1s 内显示，后续数据滚动时按需加载

## MODIFIED Requirements

### Requirement: ffmpeg_dlopen.go 双向输出捕获
`call_native_run_cached` C 函数已支持 stdout_file 和 stderr_file 两个参数。`CallFFmpegNative` 已正确使用（stdout=nil, stderr=stderrFile），但 `CallFFprobeNative` 反过来了（stdout=stdoutFile, stderr=nil）。需改为双向捕获。

### Requirement: 文件列表 API 性能
当前后端一次性遍历目录返回全量结果。对大目录需支持分页或游标式增量查询。

## REMOVED REQUIREMENTS
（无）
