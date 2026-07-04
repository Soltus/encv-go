# 修复 FFmpeg 符号 + Tasks bug + WebDAV 完整修复

## 问题 1：`ff_graph_css_data` 符号缺失

`--gc-sections` 删除了 libavfilter 间接引用符号。
**文件**：[build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh)
**修复**：ffmpeg + ffprobe 链接命令各加 `-Wl,--undefined=ff_graph_css_data \`

## 问题 2：Tasks 错误详情/复制按钮永远不显示

`simplifyErrorMessage()` 对 ffprobe/ffmpeg 等错误原样返回 → `task.Error === task.ErrorDetail` → 前端 `!==` 条件永远 false。
**文件**：[task_manager.go](file:///workspace/internal/service/task_manager.go)
**修复**：`return errMsg` 前新增 ffprobe/ffmpeg/encryption 匹配规则 + 超长截断

## 问题 3：WebDAV modal 测试连接

源码已确认包含新逻辑（PROPFIND 后端 + 内联结果区前端），需确保 Go 重编后生效。同时补充：列表滑动测试（`testConfig`）也改为内联结果展示，移除 toast 依赖。

## 步骤

### 1. build-ffmpeg-android.sh — 两处链接加 `--undefined=ff_graph_css_data`

### 2. task_manager.go — simplifyErrorMessage 新增：
```go
if strings.Contains(errMsg, "ffprobe failed") {
    return "failed to read video metadata"
}
if strings.Contains(errMsg, "ffmpeg failed") {
    return "video encoding failed"
}
if strings.Contains(errMsg, "encryption failed") || strings.Contains(errMsg, "plugin failed") {
    return "encryption processing failed"
}
if len(errMsg) > 120 {
    return errMsg[:120] + "..."
}
```

### 3. WebDAV.vue — testConfig 改为内联结果
- 添加 `listTestResults: Ref<Record<string, WebDAVTestResult>>`
- `testConfig()` 调用 API → 存入 `listTestResults[config.id]`
- 列表模板每个 config 项下方渲染 `.test-result-area`
- 移除 testConfig 中所有 showToast

### 4. 验证
`go vet` + `vue-tsc --noEmit && vite build`

## 文件变更

| 文件 | 操作 |
|------|------|
| `app/encv-mobile/scripts/build-ffmpeg-android.sh` | 修改 |
| `internal/service/task_manager.go` | 修改 |
| `app/encv-mobile/src/views/WebDAV.vue` | 修改 |
