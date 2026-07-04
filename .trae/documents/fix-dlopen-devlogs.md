# 修复 dlopen 加载失败 + 加密错误同步到 DevLogs

## 问题 1：dlopen 加载 libffprobe.so 失败

### 根因

`dlopen("libffprobe.so", RTLD_NOW)` 失败，因为 `libffprobe.so` 依赖 `libavcodec.so`、`libavformat.so` 等共享库，但 Android 的动态链接器在 `dlopen` 时不会自动搜索应用的 `nativeLibraryDir`。

虽然我们在链接 fftools 时设置了 `-Wl,-rpath,\$ORIGIN`，但 **Android 的 linker64 不支持 RPATH/RUNPATH**（Android 7.0+ 部分支持，但不保证对所有场景生效）。

当 `dlopen` 尝试加载 `libffprobe.so` 时，它找不到 `libavcodec.so` 等依赖库，导致加载失败。

### 修复方案

在 `call_native_run()` C 函数中，先加载所有依赖库（使用 `RTLD_NOW | RTLD_GLOBAL`），使它们的符号对后续加载的库可见：

```c
static const char *deps[] = {
    "libavutil.so",
    "libswresample.so",
    "libswscale.so",
    "libavcodec.so",
    "libavformat.so",
    "libavfilter.so",
    "libavdevice.so",
    NULL
};

// 先加载依赖库
for (int i = 0; deps[i]; i++) {
    char path[512];
    // 从 lib_path 所在目录构建依赖库路径
    snprintf(path, sizeof(path), "%s", lib_path);
    // 替换文件名部分为 deps[i]
    char *last_slash = strrchr(path, '/');
    if (last_slash) {
        snprintf(last_slash + 1, sizeof(path) - (last_slash + 1 - path), "%s", deps[i]);
    }
    void *dep = dlopen(path, RTLD_NOW | RTLD_GLOBAL);
    // 不检查返回值，某些库可能不存在
}

// 再加载目标库
void *handle = dlopen(lib_path, RTLD_NOW);
```

同时改进错误信息：当 dlopen 失败时，返回 `dlerror()` 的内容，方便诊断。

## 问题 2：加密解密任务报错没有同步到 DevLogs

### 根因

DevLogs 后端日志通过 `WSLogHandler`（slog handler）桥接到 WebSocket。只有通过 `slog` 输出的日志才会被转发到前端。

但视频插件大量使用 `log.Printf()`（标准库 log），而不是 `slog`。标准库 `log` 的输出不会经过 `WSLogHandler`，因此不会出现在 DevLogs 的后端标签页中。

`metadata_extractor.go` 使用了 `slog`，但 `content_preprocessor.go`、`content_verifier.go`、`mkvtoolnix.go`、`plugin.go` 都使用 `log.Printf()`。

### 修复方案

将视频插件中所有 `log.Printf()` 替换为 `slog.Info()`/`slog.Warn()`/`slog.Error()`，确保日志通过 `WSLogHandler` 转发到前端 DevLogs。

具体文件：
1. `plugin.go` — `log.Printf` → `slog.Info`/`slog.Error`
2. `content_preprocessor.go` — `log.Printf` → `slog.Info`/`slog.Warn`/`slog.Error`
3. `content_verifier.go` — `log.Printf`/`log.Println` → `slog.Info`/`slog.Error`
4. `mkvtoolnix.go` — `log.Printf` → `slog.Info`/`slog.Warn`/`slog.Error`
5. 移除所有文件中的 `"log"` import

## 实施步骤

### Step 1: 修复 ffmpeg_dlopen.go
- 在 `call_native_run()` 中先加载依赖库（RTLD_GLOBAL）
- dlopen 失败时返回 dlerror() 内容
- Go 层解析 dlerror 信息

### Step 2: 修改 ffmpeg_dlopen_stub.go
- 添加 dlerror 相关的错误类型（保持接口一致）

### Step 3: 替换视频插件 log.Printf → slog
- plugin.go
- content_preprocessor.go
- content_verifier.go
- mkvtoolnix.go

### Step 4: 验证编译
- go build
- go vet
