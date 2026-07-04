# 改善加密错误反馈 + 复制完整堆栈 + 引擎状态异常排查

## 问题分析

### 问题 1：加密依旧报错，错误信息不够
当前错误链路：
1. `video.go` → `FFProbeOutput()` → ffprobe exit 1，stderr 被 `truncateString(result.stderr, 200)` 截断到 200 字符
2. 错误信息：`"ffprobe failed (exit 1): [截断的 200 字符 stderr]"`
3. `metadata_extractor.go` → 包装为 `"ffprobe failed on original file: ..."`
4. `task_manager.go` → `simplifyErrorMessage()` → 原样返回（不匹配任何已知模式）
5. 前端显示简化的错误 + 可展开的 `errorDetail`

**问题**：
- stderr 被截断到 200 字符，丢失了 ffprobe 的关键诊断信息（如文件路径编码、格式检测失败原因等）
- 用户无法复制完整的原始错误用于调试
- 简化后的错误信息仍然不够友好

### 问题 2：需要复制按钮
前端 Tasks.vue 的可展开区域只有文本展示，没有复制功能。

### 问题 3：引擎状态 FFmpeg 红 / FFprobe 绿
`CheckFFmpegAvailable()` 对两个 .so 分别做独立 dlopen/dlsym/dlclose：
- `libffmpeg.so` → dlopen 失败或 `ffmpeg_run` 符号找不到 → red
- `libffprobe.so` → dlopen 成功 + `ffprobe_run` 找到 → green

这实际上说明 **libffmpeg.so 加载有问题**。可能的原因：
- libffmpeg.so 依赖的某个符号在链接时未解析（RTLD_NOW 会立即检查）
- 但 ffprobe 实际调用时也报错 exit 1（可能是运行时问题而非加载问题）

**注意**：如果 ffmpeg 确实无法加载，那加密时用到的 ffmpeg_run 也会失败。但用户报的是 ffprobe 失败而非 ffmpeg 失败——这说明加密流程中先调 ffprobe 取元数据，这一步就失败了。

## 实现计划

### 步骤 1：后端 — 移除 stderr 截断 + 传递完整错误信息

**文件**：`internal/utils/video.go`

改动：
1. **移除 `truncateString` 截断**：`FFProbeOutput()` 和 `FFmpegRun()` 不再截断 stderr
2. **保留 `classifyNativeError` 返回完整信息**

```go
// Before:
return nil, fmt.Errorf("ffprobe failed (exit %d): %s", result.exitCode, truncateString(result.stderr, 200))

// After:
return nil, fmt.Errorf("ffprobe failed (exit %d): %s", result.exitCode, result.stderr)
```

同理 `FFmpegRun()` 也取消截断。

### 步骤 2：后端 — ErrorDetail 包含完整技术栈

**文件**：`internal/service/task_manager.go`

当前 `failTask()` 已经将原始 `errMsg` 存入 `task.ErrorDetail`（第 642 行）。由于步骤 1 已经确保 errMsg 包含完整 stderr，这里不需要额外修改。

但需要确认：`errMsg` 是否包含足够的信息？是的，因为 `err.Error()` 链路现在包含完整 stderr。

### 步骤 3：后端 — 引擎状态增强

**文件**：`internal/server/mobile_api.go` 的 `handleFFmpegStatusGin`

增加更多信息帮助调试：

```go
type FFmpegStatus struct {
    FfmpegAvailable bool   `json:"ffmpeg_available"`
    FfprobeAvailable bool  `json:"ffprobe_available"`
    Error            string `json:"error,omitempty"`
    FfmpegDetail     string `json:"ffmpeg_detail,omitempty"`  // 新增
    FfprobeDetail    string `json:"ffprobe_detail,omitempty"` // 新增
}
```

更新 `CheckFFmpegAvailable()` 返回每个库的具体失败原因：

```go
func CheckFFmpegAvailable() (ffmpegOk bool, ffprobeOk bool, errMsg string, ffmpegDetail string, ffprobeDetail string) {
```

这样前端可以显示 "FFmpeg: FAIL - cannot locate symbol xxx" 这样的详细信息。

### 步骤 4：前端 — Tasks.vue 添加复制按钮 + 简化报错展示

**文件**：`app/encv-mobile/src/views/Tasks.vue`

改动：
1. 在展开的错误详情区域右侧添加一个小的复制图标按钮（`copyOutline` icon）
2. 点击后复制**完整的原始错误堆栈**到剪贴板
3. 复制成功后短暂提示（toast 或 icon 变为 checkmark）
4. 简化主错误显示：只保留核心描述，技术细节折叠到展开区

UI 设计：
```
❌ encryption failed: plugin failed to process file
   [▶ 展开技术详情]  [📋] ← 复制图标

┌─ 展开后 ──────────────────────────────────────┐
│ metadata extraction failed for ...              ││ ffprobe failed (exit 1):                        │
│ [完整的 stderr 输出，不再截断]                    │
│                                    [📋 复制]    │
└─────────────────────────────────────────────────┘
```

使用 Ionic 的 `IonButton fill="clear" size="small"` 内嵌 copy 图标。

### 步骤 5：前端 — Settings.vue 引擎状态显示详情

**文件**：`app/encv-mobile/src/views/Settings.vue`

当引擎不可用时，badge 旁边显示具体失败原因（来自新增的 detail 字段）：

```html
<ion-badge :color="engineStatus?.ffmpeg_available ? 'success' : 'danger'">
  {{ t('settings.ffmpegAvail') }}
</ion-badge>
<span v-if="!engineStatus?.ffmpeg_available && engineStatus?.ffmpeg_detail" class="engine-detail-text">
  {{ engineStatus.ffmpeg_detail }}
</span>
```

这样用户能看到为什么 FFmpeg 是红色的。

### 步骤 6：i18n keys

**文件**：`app/encv-mobile/src/composables/useI18n.ts`

新增：
```typescript
// 中文
'tasks.copyError': '复制错误信息'
'tasks.copied': '已复制'
'tasks.fullStack': '完整错误堆栈'

// English
'tasks.copyError': 'Copy error'
'tasks.copied': 'Copied'
'tasks.fullStack': 'Full error stack'
```

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/utils/video.go` | 修改 | 移除 stderr 200 字符截断 |
| `internal/utils/ffmpeg_dlopen.go` | 修改 | `CheckFFmpegAvailable` 增加 detail 返回值 |
| `internal/server/mobile_api.go` | 修改 | FFmpegStatus 增加 detail 字段 |
| `app/encv-mobile/src/api/encv.ts` | 修改 | FFmpegStatus 接口增加 detail 字段 |
| `app/encv-mobile/src/views/Tasks.vue` | 修改 | 添加复制按钮 + 简化主错误显示 |
| `app/encv-mobile/src/views/Settings.vue` | 修改 | 引擎状态显示失败详情 |
| `app/encv-mobile/src/composables/useI18n.ts` | 修改 | 新增 copy 相关 i18n key |

## 验证
- `vue-tsc --noEmit && vite build`
- `go vet ./internal/utils/... ./internal/service/... ./internal/server/...`
