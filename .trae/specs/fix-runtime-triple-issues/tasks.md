# Tasks

- [x] Task 1: 修复 ffprobe stderr 未捕获问题
  - [x] 修改 `internal/utils/ffmpeg_dlopen.go` 的 `CallFFprobeNative()` 函数：添加 stderr 临时文件捕获（参照 `CallFFmpegNative` 的 stderr 捕获模式）
  - [x] 将 `call_native_run_cached` 调用从 `(cStderrPath, nil)` 改为 `(cStdoutPath, cStderrPath)` — 同时捕获 stdout 和 stderr
  - [x] 将 `NativeResult` 结构体填充时加入 stderr 数据：`Stderr: string(stderrData)`
  - [x] 验证：grep 确认 `CallFFprobeNative` 中 stderr_file 参数不再为 nil

- [x] Task 2: 调查并修正 combolite 插件格式
  - [x] 搜索 combolite/aar2apk 官方文档或 demo，确认插件正确格式（.aar vs .apk vs 其他）→ 结论：APK 格式正确，问题在 enabled.set(false)
  - [x] 检查当前 `packagePlugins { enabled.set(false) }` 配置的影响 — 确认禁用了 host app 的插件加载能力
  - [x] 检查 CI workflow 中 `convert_plugin-mpv-player_release` Gradle task 的输出格式 → APK 正确
  - [x] **修复**：将 `enabled.set(false)` 改为 `enabled.set(true)`
  - [x] 验证：确认插件格式与 combolite 官方 demo 一致，host app 插件系统已启用

- [x] Task 3: 文件列表性能优化（前端虚拟滚动 + 增量加载）
  - [x] 安装虚拟滚动依赖 `@tanstack/vue-virtual`
  - [x] 重构 `Files.vue`：将全量 v-for 替换为虚拟滚动列表组件（条件渲染，<200 文件回退普通模式）
  - [x] 重构 `useFileList.ts`：添加 VIRTUAL_SCROLL_CONFIG 配置常量
  - [x] 构建验证：vue-tsc --noEmit ✅ + vite build ✅

# Task Dependencies
- [Task 1] 无依赖，可立即执行 ✅
- [Task 2] 无依赖，可并行执行 ✅
- [Task 3] 无依赖，可并行执行 ✅
