# 修复 Android 上 ffmpeg/ffprobe/mkvmerge 不可用的问题

## 问题根因

Android 上没有 ffmpeg/ffprobe/mkvmerge 可执行文件。mpv 捆绑的 ffmpeg .so 是解码专用的。

## 方案

### 1. ffmpeg/ffprobe：编译为共享库 + JNI 调用

**标准 Android 做法**（参考 FFmpegCommand、FFmpegX-Android、RxFFmpeg）：

1. 交叉编译 ffmpeg 为共享库（`libavcodec.so`, `libavformat.so` 等）+ 静态链接 libx264
2. 修改 `fftools/ffmpeg.c` 的 `main()` → `run()`，编译为 JNI wrapper（`libffmpeg_jni.so`）
3. 同理修改 `fftools/ffprobe.c` → `libffprobe_jni.so`
4. Kotlin 层通过 `System.loadLibrary()` 调用

**精简编解码器**（`--disable-everything` + 按需启用）：
- 解码器：h264, hevc, mpeg4, mpeg2video, aac, mp3, flac, opus, vorbis, ac3, eac3, dts, pcm_s16le, pcm_s24le, ass, srt, subrip, dvbsub, dvdsub
- 编码器：libx264, aac, h264_mediacodec
- 封装器：mp4, matroska, mov, avi, flv, webm, adts, ogg, wav, mp3
- 解封装器：mp4, matroska, mov, avi, flv, webm, adts, ogg, wav, mp3, h264, hevc, aac, mpegts
- 过滤器：anull, null, scale, aresample, copy, overlay, amix
- 协议：file, pipe
- 解析器：h264, hevc, aac, opus, mpeg4video, mpegaudio, ac3, dts
- BSF：h264_mp4toannexb, aac_adtstoasc, extract_extradata, hevc_mp4toannexb

**预估体积**：所有 .so 合计 ~15-25MB（精简编解码器 + `--enable-small`）

**Go 后端调用方式**：

Go 后端通过 HTTP API 调用 Kotlin 层的 ffmpeg 功能：

```
Go 后端 → HTTP POST → Kotlin 层 → JNI → libffmpeg_jni.so → ffmpeg run()
```

具体实现：
1. Kotlin 层新增 `FFmpegNative` 类，封装 JNI 调用
2. Kotlin 层在 Go 后端的 HTTP 服务器上注册新端点（通过 Go 后端的 proxy 机制）
3. 或者 Kotlin 层自己起一个轻量 HTTP 服务器（NanoHTTPD），Go 后端调用

**更简洁的方案**：Go 后端通过 `exec.Command` 调用一个 Kotlin 命令行工具，该工具内部通过 JNI 调用 ffmpeg。但这需要额外的进程。

**最简洁方案**：Kotlin 层直接在 Go 后端的 HTTP 服务器上注册 ffmpeg 执行端点。Go 后端发 HTTP 请求到自己的子路径，Kotlin 层拦截并处理。

**实际最简方案**：在 EncvGoService 中添加一个本地 HTTP 服务器（NanoHTTPD），监听 `configPort + 1`，提供 ffmpeg/ffprobe 执行端点。Go 后端通过 `http://127.0.0.1:{port+1}/api/native/ffmpeg` 调用。

### 2. mkvmerge/mkvinfo/mkvextract：用 Go 原生库替代

用 `ebml-go` + `matroska` Go 原生库替代所有 mkvtoolnix CLI 调用，零外部依赖。

| 场景 | 当前 CLI | Go 替代方案 |
|------|---------|------------|
| 识别轨道 | `mkvmerge -J` | ffprobe `-show_streams` |
| 检查 Cues | `mkvinfo -v` | ebml-go 解析 EBML |
| 提取关键帧 | `mkvextract cues` | ebml-go 解析 Cues |
| 提取章节 | `mkvextract chapters` | matroska 库读取 Chapters |
| MKV remux+Cues | `mkvmerge --cues` | ebml-go 重写带 Cues 的 MKV |
| 合并分片 | `mkvmerge +` | ebml-go 合并 Cluster |
| 验证完整性 | `mkvinfo -P` | ffprobe 检查 duration |
| 检查分片链接 | `mkvinfo -p` | ebml-go 解析 SegmentInfo |

降级策略：Go 原生优先，CLI 回退（桌面端）。

### 3. 实施步骤

#### Step 1: 创建精简版 ffmpeg 编译脚本
创建 `scripts/build-ffmpeg-android.sh`：
- 下载 ffmpeg 源码 + libx264 源码
- 交叉编译 libx264 静态库
- 交叉编译 ffmpeg 共享库（精简编解码器）
- 编译 JNI wrapper（修改 fftools/ffmpeg.c 和 fftools/ffprobe.c）
- 产物：`libavcodec.so`, `libavformat.so`, ..., `libffmpeg_jni.so`, `libffprobe_jni.so`

#### Step 2: 添加 JNI wrapper 代码
创建 `android/app/src/main/cpp/` 目录：
- `ffmpeg_wrapper.c`：`Java_com_encvgo_app_FFmpegNative_runFFmpeg()`
- `ffprobe_wrapper.c`：`Java_com_encvgo_app_FFmpegNative_runFFprobe()`
- `CMakeLists.txt`：编译 JNI wrapper，链接 ffmpeg 共享库

#### Step 3: 添加 Kotlin FFmpegNative 类
创建 `FFmpegNative.kt`：
```kotlin
object FFmpegNative {
    init {
        System.loadLibrary("avcodec")
        System.loadLibrary("avformat")
        // ... 其他 ffmpeg 库
        System.loadLibrary("ffmpeg_jni")
        System.loadLibrary("ffprobe_jni")
    }
    external fun runFFmpeg(args: Array<String>): Int
    external fun runFFprobe(args: Array<String>): Int
}
```

#### Step 4: 添加 Kotlin HTTP 服务器
在 EncvGoService 中添加 NanoHTTPD 服务器：
- `POST /api/native/ffmpeg` → 调用 `FFmpegNative.runFFmpeg(args)`
- `POST /api/native/ffprobe` → 调用 `FFmpegNative.runFFprobe(args)`
- 监听 `configPort + 1`

#### Step 5: 修改 Go 后端调用方式
在 `utils/video.go` 中：
- `FFmpegCmd()` 在 Android 上改为 HTTP 调用 Kotlin 层
- `FFProbeCmd()` 同理
- 通过 `ENCV_MOBILE` 环境变量判断是否在 Android 上

#### Step 6: 添加 Go MKV 处理库
创建 `internal/v2/plugins/video/mkv_native.go`

#### Step 7: 添加降级逻辑
在 `mkvtoolnix.go` 中，Go 原生优先，CLI 回退

#### Step 8: 更新 CI 构建流程
替换 `setup-ffmpeg-cli.sh` 为 `build-ffmpeg-android.sh`

#### Step 9: 验证
- `go build ./internal/...`
- `vue-tsc --noEmit`

## 修改文件清单

1. `/workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh` — 新建，精简版 ffmpeg 编译脚本
2. `/workspace/app/encv-mobile/scripts/setup-ffmpeg-cli.sh` — 删除
3. `/workspace/app/encv-mobile/android/app/src/main/cpp/` — 新建，JNI wrapper
4. `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/FFmpegNative.kt` — 新建
5. `/workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt` — 添加 NanoHTTPD + ffmpeg 端点
6. `/workspace/app/encv-mobile/android/app/build.gradle` — 添加 CMake + NanoHTTPD 依赖
7. `/workspace/internal/utils/video.go` — Android 上改为 HTTP 调用
8. `/workspace/internal/v2/plugins/video/mkv_native.go` — 新建，Go 原生 MKV 处理
9. `/workspace/internal/v2/plugins/video/mkvtoolnix.go` — 添加降级逻辑
10. `/workspace/internal/v2/plugins/video/content_preprocessor.go` — remapWithMKVMerge 降级
11. `/workspace/.github/workflows/android.yml` — 更新构建步骤
12. `/workspace/go.mod` / `go.sum` — 添加 ebml-go、matroska 依赖
