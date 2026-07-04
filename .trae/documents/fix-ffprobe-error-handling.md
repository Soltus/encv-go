# ffprobe 失败 + 移动端更好的错误处理

## 问题背景

用户加密视频时遇到 `ffprobe failed (exit 1)` 错误，原始错误信息对用户毫无意义。移动端应该从以下层面改善错误处理体验：

1. **错误分类与友好提示**（✅ 已完成）
2. **前端错误展示优化**（✅ 已完成）
3. **启动时预检**（🔄 进行中 — 后端已实现，路由/Kotlin/前端未完成）
4. **dlopen 缓存优化**（⏳ 待实现）

## 当前已完成

### 步骤 1：后端错误分类与友好化 ✅
- `ffmpeg_dlopen.go`：`NativeError` 类型系统（`NativeErrorDlopen`/`NativeErrorSymbol`/`NativeErrorExit`）
- `video.go`：`classifyNativeError()` 映射为 `[ENGINE_LOAD_FAILED]`/`[ENGINE_SYMBOL_MISSING]`/`[ENGINE_EXIT_ERROR]` 标签
- `metadata_extractor.go`：`ENGINE_LOAD_FAILED`/`ENGINE_SYMBOL_MISSING` → "video engine unavailable"；文件不存在/权限拒绝 → "cannot access file"
- `task_manager.go`：`simplifyErrorMessage()` + `ErrorDetail` 字段（友好错误 + 原始技术详情）

### 步骤 2：前端错误展示优化 ✅
- `Tasks.vue`：可展开的技术详情（`toggleErrorDetail`）
- `encv.ts`：`errorDetail` 字段
- `useI18n.ts`：`tasks.showDetail` i18n key

## 待实现

### 步骤 3：启动时预检

#### 3.1 注册 `/api/ffmpeg-status` 路由
- **文件**：`internal/server/server.go` 第 174 行附近
- **操作**：在 `r.GET("/api/build-info", s.handleBuildInfoGin)` 后添加：
  ```go
  r.GET("/api/ffmpeg-status", s.handleFFmpegStatusGin)
  ```
- handler 已存在于 `mobile_api.go`，只需注册路由

#### 3.2 前端 API 调用函数
- **文件**：`app/encv-mobile/src/api/encv.ts`
- **操作**：添加 `FFmpegStatus` 接口和 `fetchFFmpegStatus()` 函数
  ```typescript
  export interface FFmpegStatus {
    ffmpeg_available: boolean
    ffprobe_available: boolean
    error?: string
  }

  export async function fetchFFmpegStatus(): Promise<FFmpegStatus> {
    const baseUrl = getApiBaseUrl()
    const response = await fetch(`${baseUrl}/api/ffmpeg-status`)
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    return await response.json()
  }
  ```

#### 3.3 设置页面显示引擎状态
- **文件**：`app/encv-mobile/src/views/Settings.vue`
- **操作**：在 "连接" 列表后、"缓存" 列表前，添加 "引擎" 状态区域
  - 调用 `fetchFFmpegStatus()` 获取 ffmpeg/ffprobe 可用性
  - 用 badge 显示 ffmpeg 和 ffprobe 的可用状态（success/danger）
  - 如果不可用，显示错误提示（如 "请重新安装应用"）
  - 仅移动端显示（`v-if="isNative()"`）
- **i18n**：在 `useI18n.ts` 添加：
  - `'settings.engineStatus': '引擎状态'` / `'Engine Status'`
  - `'settings.ffmpegAvail': 'FFmpeg'`
  - `'settings.ffprobeAvail': 'FFprobe'`
  - `'settings.engineError': '引擎不可用，请重新安装应用'` / `'Engine unavailable, please reinstall the app'`

#### 3.4 Kotlin 端（可选增强）
- 不在 Kotlin 端调用 `/api/ffmpeg-status`，因为：
  - Go 进程启动后前端自然会通过 API 检查
  - Kotlin 端无法直接 dlopen 检查（需要 Go cgo 环境）
  - 前端 Settings 页面已经是最自然的展示位置
- **结论**：Kotlin 端不需要额外改动

### 步骤 4：dlopen 缓存优化

#### 4.1 问题分析
当前每次调用 `callFFmpegNative`/`callFFprobeNative` 都会：
1. `dlopen` 加载 .so
2. `dlsym` 查找符号
3. 执行
4. `dlclose` 关闭

这意味着每次加密/解密操作都要重新加载 ~10MB 的 .so 文件，浪费时间和内存。

#### 4.2 方案：缓存 dlopen handle
- **文件**：`internal/utils/ffmpeg_dlopen.go`
- **操作**：修改 C 代码，将 handle 缓存在全局变量中
  - 添加 `g_ffmpeg_handle` 和 `g_ffprobe_handle` 全局指针
  - `call_native_run` 改为接受 `cached_handle` 指针参数
  - 首次调用时 dlopen + dlsym，后续调用复用 handle
  - 不再每次 dlclose
  - 添加 `cleanup_cached_handles()` 函数（进程退出时调用，可选）
- **关键设计**：
  - 保持 `pthread_mutex_t` 互斥锁保护并发访问
  - `RTLD_NOW | RTLD_LOCAL` 保持不变
  - `reset_fn()` 每次调用前仍然执行（清理 ffmpeg/ffprobe 全局状态）
  - handle 缓存是进程生命周期级别的，不需要 dlclose

#### 4.3 具体实现
修改 C 内联代码：
```c
static void *g_ffmpeg_handle = NULL;
static void *g_ffprobe_handle = NULL;

static int call_native_run_cached(
    void **cached_handle,
    const char *lib_path,
    const char *run_sym,
    const char *reset_sym,
    int argc,
    char **argv,
    const char *stdout_file,
    const char *stderr_file
) {
    pthread_mutex_lock(&g_mutex);

    if (!*cached_handle) {
        dlerror();
        void *h = dlopen(lib_path, RTLD_NOW | RTLD_LOCAL);
        if (!h) {
            // ... error handling same as before
            pthread_mutex_unlock(&g_mutex);
            return -1;
        }
        *cached_handle = h;
    }

    // reset + run + stdout/stderr redirect (same as before)
    // ...

    // NO dlclose anymore
    pthread_mutex_unlock(&g_mutex);
    return ret;
}
```

Go 端修改：
```go
// callFFmpegNative 使用缓存的 handle
func callFFmpegNative(args []string) (*nativeResult, error) {
    // ... same arg preparation ...
    ret := C.call_native_run_cached(
        &C.g_ffmpeg_handle,
        cLibPath, cRunSym, cResetSym, argc, &argv[0], nil, cStderrPath,
    )
    // ... same result handling ...
}
```

#### 4.4 CheckFFmpegAvailable 适配
- `checkLibAvailable()` 仍然使用独立的 dlopen/dlclose（仅启动预检用，不缓存）
- 或者改为检查缓存 handle 是否存在（更高效但需区分"从未调用过"和"加载失败"）

### 步骤 5：验证
- `vue-tsc --noEmit && vite build`（前端构建验证）
- `go vet ./...`（Go 编译验证）

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/server/server.go` | 修改 | 注册 `/api/ffmpeg-status` 路由 |
| `app/encv-mobile/src/api/encv.ts` | 修改 | 添加 `FFmpegStatus` 接口和 `fetchFFmpegStatus()` |
| `app/encv-mobile/src/views/Settings.vue` | 修改 | 添加引擎状态显示区域 |
| `app/encv-mobile/src/composables/useI18n.ts` | 修改 | 添加引擎状态相关 i18n key |
| `internal/utils/ffmpeg_dlopen.go` | 修改 | dlopen handle 缓存优化 |
