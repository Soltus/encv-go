# 安卓端 Mock 缺失检查报告

## 一、现有 Android Mock/Stub 清单

| 文件 | Build Tag | Mock 功能 | 状态 |
|------|-----------|----------|------|
| [`ffmpeg_dlopen.go`](internal/utils/ffmpeg_dlopen.go) | `//go:build android` | CGO dlopen 加载 libffmpeg.so/libffprobe.so | ✅ 完整 |
| [`ffmpeg_dlopen_stub.go`](internal/utils/ffmpeg_dlopen_stub.go) | `//go:build !android` | 桌面端返回 "not available" | ✅ 完整 |
| [`build_info.go`](internal/utils/build_info.go) | `//go:build android` | 从 filesDir 读取 build-info.json | ✅ 完整 |
| [`build_info_stub.go`](internal/utils/build_info_stub.go) | `//go:build !android` | 桌面端返回 error | ✅ 完整 |
| [`memory_unix.go`](internal/utils/memory_unix.go) | `//go:build !windows` | unix.Sysinfo 获取内存（Android 属于 unix） | ✅ 可用 |
| [`memory_windows.go`](internal/utils/memory_windows.go) | `//go:build windows` | Windows 内存 API | ✅ 无关 |

### 已有 Mobile 分支覆盖的功能

| 功能 | 文件 | Mobile 处理方式 | 状态 |
|------|------|-----------------|------|
| FFmpeg 执行 | [`video.go:FFmpegRun()`](internal/utils/video.go#L119) | `IsMobile() → callFFmpegNative(dlopen)` | ✅ |
| FFProbe 执行 | [`video.go:FFProbeOutput()`](internal/utils/video.go#L104) | `IsMobile() → callFFprobeNative(dlopen)` | ✅ |
| FFmpeg 带 stderr | [`video.go:FFmpegRunWithStderr()`](internal/utils/video.go#L134) | `IsMobile() → callFFmpegNative` | ✅ |
| FFmpeg 带 context | [`video.go:FFmpegRunWithContext()`](internal/utils/video.go#L152) | `IsMobile() → callFFmpegNative` | ✅ |
| MKV 视频轨道 ID | [`mkvtoolnix.go:getVideoTrackID()`](internal/v2/plugins/video/mkvtoolnix.go#L36) | `IsMobile() → getVideoTrackIDWithFFProbe()` | ✅ |
| MKV 分片检测 | [`mkvtoolnix.go:IsMkvPartOfSplit()`](internal/v2/plugins/video/mkvtoolnix.go#L74) | `IsMobile() → IsMkvPartOfSplitNative(EBML)` | ✅ |
| MKV Cues 检测 | [`mkvtoolnix.go:checkFileForCues()`](internal/v2/plugins/video/mkvtoolnix.go#L215) | `IsMobile() → CheckFileForCuesNative(EBML)` | ✅ |
| MKV 关键帧提取 | [`mkvtoolnix.go:extractKeyFrameOffsetsFromMKV()`](internal/v2/plugins/video/mkvtoolnix.go#L229) | `IsMobile() → ExtractKeyFrameOffsetsFromMKVCuesNative(EBML)` | ✅ |
| MKV 章节提取 | [`mkvtoolnix.go:ExtractChaptersWithMKVExtract()`](internal/v2/plugins/video/mkvtoolnix.go#L160) | `IsMobile() → ExtractChaptersFromMKVNative(EBML)` | ✅ |
| MKV SegmentInfo | [`mkvtoolnix.go:getMkvInfo()`](internal/v2/plugins/video/mkvtoolnix.go#L451) | `IsMobile() → getMkvInfoNative(EBML)` | ✅ |
| MKV 分片合并 | [`mkvtoolnix.go:mergeSplitPartsFromSet()`](internal/v2/plugins/video/mkvtoolnix.go#L609) | `IsMobile() → mergeSplitPartsWithFFmpeg(ffmpeg concat)` | ✅ |
| MKV remux (cues) | [`content_preprocessor.go:remapWithMKVMerge()`](internal/v2/plugins/video/content_preprocessor.go#L355) | `IsMobile() → remapMKVWithFFmpeg()` | ✅ |
| FFmpeg 预处理 | [`content_preprocessor.go:runFFmpegCmd()`](internal/v2/plugins/video/content_preprocessor.go#L70) | `IsMobile() → runFFmpegCmdMobile()` | ✅ |
| MKV cues remux | [`content_preprocessor.go:remuxMKVCues()`](internal/v2/plugins/video/content_preprocessor.go#L355) | `IsMobile() → remapMKVWithFFmpeg()` | ✅ |

---

## 二、发现的缺失项

### 缺失 1（高优先级）：`Preview()` 无 Android 分支 — 直接调用 `exec.Command("mpv")`

**文件**: [`internal/service/decrypt_preview.go:40`](internal/service/decrypt_preview.go#L40)

**问题**:
```go
func Preview(ctx context.Context, inputPath string) error {
    // ...
    mpvPath, err := exec.LookPath("mpv")  // ← Android 上 mpv 是 .so 库，不在 PATH 中
    // ...
    cmd := exec.Command(mpvPath, args...)   // ← os/exec 在 Android 上不可用
    // ...
    cmd.Process.Signal(os.Interrupt)         // ← os.Signal 在 Android 上不可用
}
```

**影响**:
- Android 端调用 `Preview()` 会直接 panic/报错
- 但实际上 **Android 端预览走的是 Kotlin 侧的 `MpvPlayerModule`**（通过 MPVLib JNI 直接加载 libmpv.so），**不经过 Go 的 `Preview()` 函数**
- 所以当前的影响取决于：Go 后端的 `/api/preview` 路由是否在 Android 上被调用
  - 如果 Android Lynx 前端直接调 `MpvPlayerModule.play(url)` 而不调 Go preview API → **无运行时影响**
  - 如果有代码路径可能触发 Go 端 Preview → **会崩溃**

**建议修复方案**:

创建 `decrypt_preview_mobile.go`（`//go:build android`）：
```go
func Preview(ctx context.Context, inputPath string) error {
    return fmt.Errorf("preview not available on mobile; use MpvPlayerModule")
}
```

或者更优：在 `Preview()` 入口加 `IsMobile()` 守卫，返回友好错误。

---

### 缺失 2（低优先级）：`IdentifyWithMKVMerge()` 无 Android 分支 — 死代码

**文件**: [`internal/v2/plugins/video/mkvtoolnix.go:143`](internal/v2/plugins/video/mkvtoolnix.go#L143)

**问题**: 该函数直接调用 `exec.Command("mkvmerge", "-J", filePath)`，没有 `IsMobile()` 守卫。

**影响**: **无运行时影响**。经 Grep 确认该函数在整个项目中**没有任何调用者**，属于死代码。

**建议**: 标记 `// TODO: dead code, no mobile fallback needed` 或删除。

---

### 缺失 3（信息级）：Android Assets 资源文件检查

**目录**: `app/encv-mobile/android/app/src/main/assets/`

| 文件 | 用途 | 状态 |
|------|------|------|
| `config.mobile.json` | 默认配置模板 | ✅ 存在 |
| `capacitor.plugins.json` | Capacitor 插件注册 | ✅ 存在 |
| `player.lynx.bundle` | Lynx Player bundle | ✅ 存在 |
| **`build-info.json`** | **构建元数据（由 Go 后端读取）** | ⚠️ **不存在！** |

**问题**: [`EncvGoService.kt:ensureBuildInfoExists()`](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt#L394) 从 assets 复制 `build-info.json` 到 filesDir。但 assets 目录中**没有这个文件**。

**运行时行为**: `ensureBuildInfoExists()` 会 catch 异常并 warn，但不会崩溃。之后 Go 后端 `GetBuildInfo()` 会因找不到文件而返回错误。

**影响**: 依赖 build-info 的功能（如显示版本号、构建时间）在 Android 上不可用。

**建议**: 在构建脚本中自动生成 `build-info.json` 并打包进 assets。

---

### 缺失 4（信息级）：`content_preprocessor.go` 部分 mkvmerge 调用缺少 Mobile 守卫

**文件**: [`internal/v2/plugins/video/content_preprocessor.go`](internal/v2/plugins/video/content_preprocessor.go)

以下位置的 `exec.Command("mkvmerge", ...)` **已有** `IsMobile()` 守卫（通过外层函数）：

| 行号 | 函数 | 有守卫? |
|------|------|---------|
| L377 | `remapWithMKVMerge()` | ✅ L355 `IsMobile() → remapMKVWithFFmpeg()` |
| L400 | fallback `"video:all"` | ✅ 同上（在同一个 if/else 分支内） |

✅ **全部已覆盖**，无需修改。

---

## 三、总结与优先级排序

| # | 缺失项 | 严重程度 | 运行时影响 | 建议 |
|---|--------|----------|-----------|------|
| 1 | `Preview()` 无 Android stub | **中** | 取决于调用路径；若被调用则崩溃 | 加 `IsMobile()` 守卫或创建 `_mobile.go` stub |
| 2 | `IdentifyWithMKVMerge()` 无 Mobile 分支 | **低** | 无（死代码） | 标记或删除 |
| 3 | `build-info.json` 缺失于 assets | **低** | build info 功能不可用 | 构建脚本补充生成 |
| 4 | content_preprocessor mkvmerge | **无** | 已全覆盖 | 无需操作 |

### 结论

**安卓端 mock 整体覆盖率良好**。核心路径（FFmpeg/FFProbe dlopen、MKV EBML 解析、视频预处理）均有完整的 Android 适配。唯一需要关注的是 **`Preview()` 函数缺少 Android 守卫**，以及 **`build-info.json` 未打包进 APK assets**。
