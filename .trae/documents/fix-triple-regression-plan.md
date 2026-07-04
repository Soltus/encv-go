# 修复三问题：Files页面虚拟滚动崩溃 + ffprobe参数顺序错误 + MPV插件无反馈

## Why
上一轮修改引入三个问题：
1. **Files.vue 虚拟滚动崩溃**：`useVirtualizer` 初始化时 `count` 用了静态值（setup 时=0）、`getScrollElement()` 返回 null、`virtualItems` computed 在 setup 时立即调用 `.getVirtualItems()` 可能抛异常导致整个组件白屏。用户明确指出"全部文件本来没有性能问题，需要优化的是插件类型文件列表"——但方案应是**修复 bug 保留改进**，不是全面回退。
2. **ffprobe 参数顺序错误**：stderr 捕获成功暴露真正根因——`CallFFprobeNative()` 缺少 argv[0] 程序名 `"ffprobe"`，FFmpeg 的 `parse_options()` 把 `-v` 当程序名、`quiet` 当输入文件。报错 `"Argument '...mp4' provided as input filename, but 'quiet' was already specified"` 是典型症状。**同 bug 存在于 CallFFmpegNative**。
3. **MPV 插件安装无反馈**：`Class.forName("com.combo.core.runtime.PluginManager")` 失败时只 Log.w 无 call.reject()。

## What Changes

### 问题 1：修复 Files.vue 虚拟滚动崩溃（不回退，修复）

**根因分析**：
- L515-L520: `useVirtualizer({ count: shouldUseVirtualScroll.value ? displayFiles.value.length : 0 })` — setup 时 displayFiles 为空 → count 永远为 0 的静态值
- L517: `getScrollElement: () => virtualizerRef.value` — template v-if 未渲染时返回 null
- L528: `const virtualItems = computed(() => rowVirtualizer.value.getVirtualItems())` — setup 时立即执行可能抛异常

**修复策略**：

**方案 A（推荐）：延迟初始化 + 响应式 count**
```typescript
// 改为 shallowRef + computed 响应式 count
const virtualizerRef = ref<HTMLElement | null>(null)
const rowVirtualizer = shallowRef<ReturnType<typeof useVirtualizer> | null>(null)

// watch shouldUseVirtualScroll 变化时延迟创建/销毁
watch(shouldUseVirtualScroll, (enabled) => {
  if (enabled && displayFiles.value.length >= VIRTUAL_SCROLL_CONFIG.THRESHOLD) {
    rowVirtualizer.value = useVirtualizer({
      count: computed(() => displayFiles.value.length),
      getScrollElement: () => virtualizerRef.value,
      estimateSize: () => VIRTUAL_SCROLL_CONFIG.ESTIMATE_SIZE,
      overscan: VIRTUAL_SCROLL_CONFIG.OVERSCAN,
    })
  } else {
    rowVirtualizer.value = null
  }
}, { immediate: false })

const virtualItems = computed(() => {
  if (!rowVirtualizer.value || !shouldUseVirtualScroll.value) return []
  try {
    return rowVirtualizer.value.getVirtualItems()
  } catch { return [] }
})
```

**关键改动点**：
1. `count` 从静态值改为 `computed(() => displayFiles.value.length)` — 响应式更新
2. `rowVirtualizer` 改为 `shallowRef` + 延迟初始化（不再在 setup 时直接创建）
3. `virtualItems` computed 加 try-catch 防御性编程
4. `getScrollElement` 返回 null 时 @tanstack/vue-virtual 内部会安全处理（不会 crash）
5. **保留 SSE 流式加载**（`listFilesStream`）— 这是真正的性能改进，首屏秒显

### 问题 2：修复 ffprobe/ffmpeg argv[0] 缺失（范式级修复）

**根因**：FFmpeg C API 的 `*_run(int argc, char **argv)` 函数遵循标准 C `main()` 约定，要求 `argv[0] == 程序名`。

当前代码：
```go
// CallFFprobeNative:
args := []string{"-v", "quiet", "-print_format", "json", path} // 用户传入的参数
argc = C.int(len(args))        // argc = 7
argv[0] = "-v"                 // ffprobe 把 -v 当程序名！
argv[1] = "quiet"              // ffprobe 把 quiet 当输入文件！
```

修正：
```go
fullArgs := make([]string, len(args)+1)
fullArgs[0] = "ffprobe"        // ← 程序名
copy(fullArgs[1:], args)       // ← 用户参数从 argv[1] 开始
argc = C.int(len(fullArgs))     // argc = 8
```

同样修复 `CallFFmpegNative`（prepend `"ffmpeg"`）。

### 问题 3：MPV 插件安装增加用户反馈

- `installPlugin()` 中 `Class.forName("com.combo.core.runtime.PluginManager")` 失败时 → `call.reject("ComboLite PluginManager not available on this device")`
- 所有 catch 分支确保有 resolve 或 reject

## Impact
- Affected code:
  - `app/encv-mobile/src/views/Files.vue` — 修复虚拟滚动初始化（不删除功能）
  - `internal/utils/ffmpeg_dlopen.go` — argv[0] 修复（2个函数）
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 错误反馈

## ADDED Requirements

### Requirement: useVirtualizer 必须延迟初始化
`useVirtualizer` 不应在组件 setup 时以空数据/null ref 创建，应在满足条件后延迟创建，且 count 必须是响应式的 ComputedRef。

#### Scenario: 文件数量达到阈值后启用虚拟滚动
- **WHEN** displayFiles.length 从 <200 增长到 ≥200
- **THEN** 自动创建 virtualizer 实例并开始虚拟渲染
- **AND** 页面不崩溃、无白屏

### Requirement: FFmpeg/ffprobe argv 包含程序名（范式约束）
所有通过 dlopen 调用 FFmpeg `*_run()` 的地方，argv[0] 必须为工具程序名。

#### Scenario: ffProbe 正确解析选项和输入文件
- **WHEN** Go 后端调用 `ffprobe.Probe("-v", "quiet", "-print_format", "json", path)`
- **THEN** 实际传递给 ffprobe_run 的 argv[0]="ffprobe"、argv[1]="-v"、...、最后一个=path
- **AND** 加密视频元数据提取成功

### Requirement: MPV 插件操作必须有前端可见反馈
所有 PluginCall 必须以 resolve 或 reject 结束。

#### Scenario: 设备不支持 ComboLite
- **WHEN** PluginManager 类不存在
- **THEN** 前端收到 reject + 明确错误消息

## MODIFIED Requirements

### Requirement: Files.vue loadFiles 保持流式加载
`loadFiles()` 继续使用 `listFilesStream`（SSE 增量加载），这是正确的性能优化。仅修复虚拟滚动部分的初始化 bug。

## REMOVED Requirements

无
