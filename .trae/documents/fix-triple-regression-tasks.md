# Tasks

- [ ] Task 1: 修复 Files.vue 虚拟滚动初始化崩溃（保留功能，修复 bug）
  - [ ] 将 `rowVirtualizer` 从 `useVirtualizer()` 直接调用改为 `shallowRef<ReturnType<typeof useVirtualizer> | null>(null)`
  - [ ] 添加 `watch(shouldUseVirtualScroll, ...)` 延迟创建 virtualizer：当 shouldUseVirtualScroll 变为 true 时才创建
  - [ ] virtualizer 的 `count` 参数改为 `computed(() => displayFiles.value.length)`（响应式）
  - [ ] `virtualItems` computed 加防御：`if (!rowVirtualizer.value) return []` + try-catch 包裹 `getVirtualItems()`
  - [ ] 添加 `shallowRef` 到 import（从 vue 导入）
  - [ ] **保留** SSE 流式加载（listFilesStream）和 loadFiles 增量追加逻辑
  - [ ] 验证：`cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build`

- [ ] Task 2: 修复 ffprobe/ffmpeg argv[0] 缺失（范式级修复）
  - [ ] 修改 `CallFFmpegNative()`：构建 argv 前 prepend `"ffmpeg"`，argc +1
  - [ ] 修改 `CallFFprobeNative()`：构建 argv 前 prepend `"ffprobe"`，argc +1
  - [ ] 验证：`mise exec -- go build ./internal/utils/`

- [ ] Task 3: MPV 插件安装增加用户反馈
  - [ ] 读取 `GoProcessPlugin.kt` 的 `installPlugin()` 方法完整代码
  - [ ] `Class.forName("com.combo.core.runtime.PluginManager")` ClassNotFoundException → `call.reject("ComboLite PluginManager not available")`
  - [ ] 所有 catch 分支确保有 resolve 或 reject
  - [ ] 验证：grep 确认 installPlugin 中所有 code path 都有 resolve/reject

# Task Dependencies
- 三个任务无依赖，可并行执行
- Task 1 最高优先级（页面崩溃阻塞使用）
- Task 2 高优先级（加密视频不可用）
- Task 3 中优先级
