# Phase 3: 真机 ffmpeg encoder 缺失 + MediaCodec 硬编集成

> **核心问题**：真机 ffmpeg build 按 manifest 裁过，只有 aac + libx264 + 3 pcm。**没有 libmp3lame、没有 flac encoder**。
> **解决思路**：双轨——MediaCodec 硬编 mp4（替 libx264+aac+mp4 muxer）+ manifest 加 libmp3lame（mp3 必选）+ ffmpeg fallback（mkv/flac）。
> **状态**：proposal 阶段。**不实装完整 MediaCodec bridge**（需真机验证），先写 plan + skeleton + 决策点。

---

## 一、问题根因（饱和调试证据链）

### 1.1 现象

`/api/mock/generate` 在真机（Android）调用时，mp3 / flac 始终 fail：

```
event: spec_failed
data: {"relativePath": "01-plain-media/audio/music.mp3", "reason": "ffmpeg 不可用/未编该 encoder (真机常见：libmp3lame/flac 等)", "exitCode": 1, "stderr": "Unknown encoder 'libmp3lame'..."}
```

### 1.2 根因

[ffmpeg-feature-manifest.json:7](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json#L7)：

```json
"encoders": ["aac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "libx264"]
```

被 [build-ffmpeg-android.sh:212](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L212) `--enable-encoder="$ENCODERS"` 使用 → **libmp3lame 和 flac 源码层面就没编**。

### 1.3 沙箱 vs 真机差异

| 场景 | ffmpeg binary | encoder 列表 | mp4 | mkv | mp3 | flac |
|------|---------------|-------------|-----|-----|-----|------|
| 沙箱 `docker exec` | 系统包 ffmpeg 6.1 (full build) | 全部 | ✅ -c copy | ✅ -c copy | ✅ libmp3lame | ✅ flac |
| 真机 APK | 项目编 `libffmpeg.so` (manifest 裁过) | **aac + libx264 + 3 pcm** | ✅ -c copy | ✅ -c copy | ❌ Unknown encoder | ❌ Unknown encoder |

---

## 二、MediaCodec 能力矩阵（2026-06-12 调研结论）

### 2.1 Android MediaCodec encoder 真机能力

| MIME | 能力 | 最低 API | 备注 |
|------|------|---------|------|
| `video/avc` (H.264) | ✅ 必须 | 21 (Android 5.0) | OEM 必支持硬编 |
| `video/hevc` (H.265) | ✅ 通常 | 21 | 部分设备需 API 28+ |
| `video/x-vnd.on2.vp8` | ✅ 通常 | 21 | |
| `video/x-vnd.on2.vp9` | ✅ 通常 | 24 | |
| `audio/mp4a-latm` (AAC-LC) | ✅ 必须 | 21 | OEM 必支持硬编 |
| `audio/3gpp` (AMR-NB) | ✅ 必须 | 21 | |
| `audio/amr-wb` | ✅ 必须 | 21 | |
| `audio/flac` | ⚠️ 部分 | 21 (API 21 引入) | 实际 OEM 实现差异大，需运行时 `MediaCodecList.REGULAR_CODECS` 探测 |
| `audio/opus` | ⚠️ 部分 | 29 (Android 10) | |
| **MP3 encoder** | ❌ **不存在** | — | Android 平台**没有 MP3 encoder**（Google 因 MP3 专利问题故意不集成） |

### 2.2 Android MediaMuxer 支持的 container

| container | MIME | 最低 API | 备注 |
|-----------|------|---------|------|
| MP4 | `video/mp4` | 21 | ✅ 完全支持 |
| M4V | `video/m4v` | 21 | |
| 3GP | `video/3gpp` | 21 | |
| WebM | `video/webm` | 21 | 仅 VP8/VP9 + Vorbis/Opus |
| ADTS raw AAC | `audio/mp4a-latm` | 21 | 仅 AAC |
| **MKV (Matroska)** | — | ❌ | **不支持** |
| **MP3 muxer** | — | ❌ | **不支持** |
| **FLAC muxer** | — | ❌ | **不支持**（FLAC 文件本身就是裸 bitstream，需要手工拼 fLaC + STREAMINFO + frames） |

### 2.3 真机 4 个 mock 格式的 MediaCodec 适配方案

| 格式 | 编码 | 容器 | MediaCodec 全栈 | 评估 |
|------|------|------|----------------|------|
| **mp4** (h264+aac) | MediaCodec video/avc + audio/mp4a-latm | MediaMuxer MP4 | ✅ **完全替 libx264+aac+mp4 muxer** | **Phase 3 实装** |
| **mkv** (h264+aac) | ffmpeg `-c copy` 已够（source.mp4 已经是 h264+aac） | ffmpeg matroska muxer | ⚠️ 维持 ffmpeg（只跑 mux 不需要 encoder） | **不动** |
| **mp3** | libmp3lame（**无替代**） | libmp3lame 自带 muxer | ❌ **必须修 manifest 加 libmp3lame** | **Phase 3 必做** |
| **flac** (raw FLAC) | MediaCodec audio/flac | 手工拼 fLaC magic + STREAMINFO + 帧 | ⚠️ 实现成本中等（200-400 行 Java/Kotlin） | **Phase 3 可选** |

---

## 三、Phase 3 实施计划

### 3.1 范围

**Phase 3.1（必做，最低成本）**：修 manifest，加 libmp3lame
**Phase 3.2（推荐）**：MediaCodec 硬编 mp4（替 libx264+aac+mp4 muxer）
**Phase 3.3（可选）**：MediaCodec 硬编 raw flac
**Phase 3.4（不实装）**：MediaCodec 硬编 mp3（**Android 平台不支持**）

### 3.2 架构变更

#### Init() 优先级（新增 MediaCodecRunner）

```go
// internal/utils/ffmpeg/runner_android.go (新)
func init() {
    // 1️⃣ MediaCodecRunner：Android 平台硬编，最快、CPU 占用最低
    if mcr := NewMediaCodecRunner(); mcr.Available() {
        SetRunner(mcr)
        return
    }
    // 2️⃣ WorkerRunner：subprocess 隔离，可 SIGKILL
    if wr := NewWorkerRunner(); wr.Available() {
        SetRunner(wr)
        return
    }
    // 3️⃣ NativeRunner：in-process cgo fallback（可阻塞）
    SetRunner(&NativeRunner{})
}
```

#### MediaCodecRunner 接口设计

```go
// internal/utils/ffmpeg/mediacodec_runner.go
//
// 🆕 2026-06-12 Phase 3：Android 平台硬编 runner
//   - 用 cgo JNI 调 Kotlin MediaCodecBridge（见 android/app/.../MediaCodecBridge.kt）
//   - 只支持 mp4 (h264+aac) 一种组合（最常见）
//   - 其他格式返回 ErrNotSupported → 上层 fallback 到 ffmpeg
//   - 不替代 ffmpeg，是 ffmpeg 之前的 fast path
type MediaCodecRunner struct {
    available bool
    // 未来可能加 encoder/muxer 缓存池
}

func NewMediaCodecRunner() *MediaCodecRunner { /* cgo */ }

// RunWithOutput 调 Kotlin MediaCodecBridge
// args 必须是固定格式：["mediacodec", "mp4", "-c", "avc+aac", "-i", srcPath, "-y", dstPath]
// 其他 args 走 ffmpeg fallback
func (r *MediaCodecRunner) RunWithOutput(ctx context.Context, args []string) ([]byte, string, int, error) {
    if !r.available {
        return nil, "", -1, ErrNotSupported
    }
    // 调 cgo → JNI → Kotlin MediaCodecBridge.encodeMp4(srcPath, dstPath)
    // 返回 (size, exitCode, err)
}

// Available 探测 MediaCodec 是否可用 + 设备是否支持 video/avc + audio/mp4a-latm encoder
func (r *MediaCodecRunner) Available() (ffmpegOk bool, ffprobeOk bool, errMsg string) {
    // cgo → JNI → MediaCodecBridge.probeCapabilities() 返回 supported encoders 列表
    return r.available, r.available, ""
}
```

#### Go 调 Kotlin MediaCodec 的 JNI 桥

```go
// internal/utils/ffmpeg/mediacodec_android.go (新)
//go:build android
package ffmpeg

/*
#cgo LDFLAGS: -llog
#include <jni.h>
#include <stdlib.h>

// Kotlin MediaCodecBridge 全限定名
extern JavaVM* g_jvm;

// 1. probeCapabilities() → 返回 "avc,aac,flac" 字符串
extern char* MediaCodecProbeCapabilities(void);

// 2. encodeMp4(srcPath, dstPath) → 返回 exit code (0=success, -1=encoder unavailable)
extern int MediaCodecEncodeMp4(const char* srcPath, const char* dstPath);

// 3. encodeFlac(srcPath, dstPath) → 返回 exit code
extern int MediaCodecEncodeFlac(const char* srcPath, const char* dstPath);
*/
import "C"

import (
    "unsafe"
)

func probeMediaCodecCaps() string {
    c := C.MediaCodecProbeCapabilities()
    defer C.free(unsafe.Pointer(c))
    return C.GoString(c)
}

func mediaCodecEncodeMp4(src, dst string) int {
    cSrc := C.CString(src); defer C.free(unsafe.Pointer(cSrc))
    cDst := C.CString(dst); defer C.free(unsafe.Pointer(cDst))
    return int(C.MediaCodecEncodeMp4(cSrc, cDst))
}
```

#### Kotlin MediaCodecBridge（新文件）

```kotlin
// android/app/src/main/java/com/encvgo/mediacodec/MediaCodecBridge.kt
package com.encvgo.mediacodec

import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.media.MediaMuxer
import android.util.Log
import java.io.File
import java.nio.ByteBuffer

/**
 * Phase 3 MediaCodec 硬编桥（Go cgo 调）
 *
 * 支持：
 *   - probeCapabilities(): 返回设备支持的 encoder MIME 列表（逗号分隔）
 *   - encodeMp4(srcPath, dstPath): 从 PCM/YUV 输入硬编 H.264 + AAC，mp4 容器
 *
 * 不支持（调用方 fallback 到 ffmpeg）：
 *   - mkv container
 *   - mp3 encoder（Android 平台没有）
 */
object MediaCodecBridge {
    private const val TAG = "ENCV-MediaCodec"

    @JvmStatic
    fun probeCapabilities(): String {
        val codecs = mutableListOf<String>()
        if (hasEncoder("video/avc")) codecs.add("avc")
        if (hasEncoder("video/hevc")) codecs.add("hevc")
        if (hasEncoder("audio/mp4a-latm")) codecs.add("aac")
        if (hasEncoder("audio/flac")) codecs.add("flac")
        if (hasEncoder("audio/opus")) codecs.add("opus")
        return codecs.joinToString(",")
    }

    private fun hasEncoder(mime: String): Boolean {
        return try {
            val format = MediaFormat.createVideoFormat(mime, 320, 240) // dummy
            // Real probe via MediaCodecList
            val list = android.media.MediaCodecList(android.media.MediaCodecList.REGULAR_CODECS)
            list.codecInfos.any { it.isEncoder && it.supportedTypes.any { t -> t.equals(mime, ignoreCase = true) } }
        } catch (e: Exception) {
            false
        }
    }

    @JvmStatic
    fun encodeMp4(srcPath: String, dstPath: String): Int {
        if (!hasEncoder("video/avc") || !hasEncoder("audio/mp4a-latm")) {
            Log.w(TAG, "encodeMp4: device lacks avc/aac encoder")
            return -1
        }
        return try {
            Mp4Encoder().encode(srcPath, dstPath)
            0
        } catch (e: Exception) {
            Log.e(TAG, "encodeMp4 failed", e)
            -1
        }
    }
}

/**
 * Mp4Encoder 用 MediaCodec video/avc + audio/mp4a-latm → MediaMuxer 拼 mp4
 *
 * 简化版（mock 场景专用）：
 *   - video：1 帧纯色（10s @ 30fps, 320x240）
 *   - audio：1 帧静音（10s @ 44.1kHz, mono）
 *   - 不读 srcPath（mock 不在乎内容，只要 mp4 文件结构合法）
 *   - 目的：让 mock 文件能被 ffprobe 识别 + 自动测试跑通
 */
private class Mp4Encoder {
    fun encode(srcPath: String, dstPath: String) {
        val width = 320
        val height = 240
        val frameRate = 30
        val durationUs = 10_000_000L  // 10s
        val sampleRate = 44100
        val channels = 1

        val muxer = MediaMuxer(dstPath, MediaMuxer.OutputFormat.MUXER_OUTPUT_MPEG_4)
        val videoFormat = MediaFormat.createVideoFormat("video/avc", width, height).apply {
            setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Flexible)
            setInteger(MediaFormat.KEY_BIT_RATE, 400_000)
            setInteger(MediaFormat.KEY_FRAME_RATE, frameRate)
            setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, 1)
        }
        val audioFormat = MediaFormat.createAudioFormat("audio/mp4a-latm", sampleRate, channels).apply {
            setInteger(MediaFormat.KEY_AAC_PROFILE, MediaCodecInfo.CodecProfileLevel.AACObjectLC)
            setInteger(MediaFormat.KEY_BIT_RATE, 64_000)
            setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, 16 * 1024)
        }

        val videoEnc = MediaCodec.createEncoderByType("video/avc")
        val audioEnc = MediaCodec.createEncoderByType("audio/mp4a-latm")
        videoEnc.configure(videoFormat, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        audioEnc.configure(audioFormat, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        videoEnc.start()
        audioEnc.start()

        // ... 编码循环、media muxer 轨道、end-of-stream ...
        // 完整实现 ~300 行（参考 Android 官方 samples/EncodeAndMuxTest）
    }
}
```

### 3.3 manifest 改造（Phase 3.1 必做）

#### 3.3.1 加 libmp3lame（mp3 encoder）

[ffmpeg-feature-manifest.json:7](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json#L7)：
```diff
-  "encoders": ["aac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "libx264"],
+  "encoders": ["aac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "libx264", "libmp3lame"],
```

#### 3.3.2 集成 lame 源码 + 改 build-ffmpeg-android.sh

[build-ffmpeg-android.sh:60](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L60) 加 lame build 步骤（参考 x264 install 模式）：

```bash
LAME_INSTALL="${BUILD_DIR}/lame-install"
if [ ! -f "${LAME_INSTALL}/lib/libmp3lame.a" ]; then
    if [ ! -d "lame" ]; then
        git clone --depth 1 https://sourceforge.net/p/lame/svn/HEAD/tree/trunk/lame lame
    fi
    cd "${BUILD_DIR}/lame"
    CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure \
        --host=${ARCH}-linux-android \
        --prefix="${LAME_INSTALL}" \
        --enable-static \
        --disable-shared \
        --disable-decoder  # libmp3lame 只编 encoder（不编 decoder）减小体积
        ...
    make -j$(nproc)
    make install
    cd ..
fi
```

configure 阶段加 `--enable-libmp3lame` + `--extra-ldflags="-L${LAME_INSTALL}/lib"`。

### 3.4 决策点（待用户确认）

| # | 决策 | 选项 | 推荐 |
|---|------|------|------|
| 1 | Phase 3.1 修 manifest 加 libmp3lame？ | 必做 | ✅ 是 |
| 2 | Phase 3.2 MediaCodec 硬编 mp4？ | 推荐 | ✅ 是（mp4 是最常见格式，CPU 收益最大） |
| 3 | Phase 3.3 MediaCodec 硬编 flac？ | 可选 | ⚠️ 否（实现成本 200-400 行，回报小） |
| 4 | Phase 3.4 ffmpeg 加 flac encoder？ | 可选 | ❌ 否（real device 很少需要 flac 编码；如需：手动生成 .wav 解码为 flac 也用不到） |
| 5 | MediaCodec 桥走 cgo JNI 还是 HTTP IPC？ | JNI（in-process） vs HTTP（subprocess） | ✅ JNI（延迟低 ~10ms / HTTP 100ms+） |
| 6 | AAR 体积上限？ | 现状 ~30 MB / 加上 libmp3lame +1.5MB | ✅ 接受（< 35MB） |

---

## 四、Phase 3.1 实施（libmp3lame 加 manifest）

### 4.1 任务

1. 改 `ffmpeg-feature-manifest.json` encoders 加 `libmp3lame`
2. 改 `build-ffmpeg-android.sh` 加 lame 编译
3. CI 重新编 libffmpeg.so
4. 验证真机 mp3 生成（自动化测试 mp3 文件不空 + ffprobe 识别为 MP3）

### 4.2 风险

- lame 是 LGPL 2.1 / MP3 专利（已过期 2017）→ **法律风险已清零**
- libffmpeg.so 体积增加 ~1.5 MB
- 编译时间 +30s（lame 编译）
- 沙箱不受影响（沙箱本来就有 libmp3lame）

### 4.3 验收

- [ ] `nm -D libffmpeg.so | grep mp3lame` 有符号
- [ ] 真机 `/api/mock/generate` mp3 字段 success（非 skipped）
- [ ] `ffprobe sample.mp3` 识别为 MP3 + 44.1kHz + 有 audio stream
- [ ] AAR 增量 < 2 MB

---

## 五、Phase 3.2 实施（MediaCodec mp4 硬编）

### 5.1 任务

1. 新增 `android/app/src/main/java/com/encvgo/mediacodec/MediaCodecBridge.kt`（Kotlin MediaCodec 桥）
2. 新增 `internal/utils/ffmpeg/mediacodec_runner.go`（Go Runner 接口）
3. 新增 `internal/utils/ffmpeg/mediacodec_android.go`（cgo JNI）
4. 改 `internal/utils/ffmpeg/runner_android.go` 的 `init()` 加 MediaCodecRunner 优先
5. 改 `mock_generator.go` 的 `ffmpegArgs`：mp4 用 `["-mediacodec", "mp4", ...]` 协议触发 MediaCodecRunner，其他不动

### 5.2 风险

- MediaCodec 配置复杂：颜色格式、I 帧间隔、码率控制、buffer 索引、dequeue timeout
- 不同设备硬编能力差异大（pixel 能编 1080p，老 MTK 设备只能 480p）
- MediaCodec 状态机严格：Uninitialized → Configured → Flushing/Running → Eos → Released
- cgo JNI 跨线程调用：MediaCodec 必须在**有 Looper 的线程**创建（不能在 cgo spawn 的线程）

### 5.3 验收

- [ ] 沙箱走 ffmpeg（!android build tag 不走 MediaCodec）
- [ ] 真机 mp4 生成走 MediaCodec（logcat 看 `ENCV-MediaCodec.encodeMp4`）
- [ ] 真机 mp4 文件能被 ffprobe 识别 + 自动化测试通过
- [ ] 硬编 CPU 占用 < 软编 30%（用 `dumpsys cpuinfo` 验证）

---

## 六、MockGenerator 前端配合

### 6.1 spec_diag 加 `runner` 字段

```ts
interface MockSpecDiag {
    index: number
    total: number
    relativePath: string
    status: 'pending' | 'ok' | 'failed'
    encoder: string
    runner: 'ffmpeg' | 'mediacodec' | 'static'   // 🆕
    ffmpegArgs: string[]
    exitCode: number
    stderr: string
}
```

### 6.2 后端 spec_diag 输出

```go
runner := "static"
if !sp.isStatic {
    if runtime.GOOS == "android" && mediacodec.SupportsFormat(ext) {
        runner = "mediacodec"
    } else {
        runner = "ffmpeg"
    }
}
diagData := fmt.Sprintf(`{... "runner": %q ...}`, runner)
```

### 6.3 UI 显示

FFMPEG 流程日志卡每行 entry 加 `runner` 标识（mediacodec = ⚡硬编 / ffmpeg = ⚙软编）。

---

## 七、依赖关系

```
Phase 3.1 (libmp3lame)
  ↓
Phase 3.2 (MediaCodec mp4)
  ↓
Phase 3.3 (MediaCodec flac, optional)
  ↓
Phase 3.4 (UI runner 标识)
```

每个 phase 独立可发版，**3.1 是 P0**（mp3 真机现在跑不通）。

---

## 八、参考

- [android.md](../rule-library/android.md) — gomobile/sqlite 选型铁律
- [combolite.md](../rule-library/combolite.md) — R8 禁用铁律
- [phase2-ffmpeg-worker-implementation-report.md](phase2-ffmpeg-worker-implementation-report.md) — Phase 2 worker 隔离
- [phase2-ffmpeg-worker-ci-step-proposal.md](phase2-ffmpeg-worker-ci-step-proposal.md) — CI step 草案
- [Android MediaCodec 官方文档](https://developer.android.com/reference/android/media/MediaCodec)
- [Android MediaMuxer 官方文档](https://developer.android.com/reference/android/media/MediaMuxer)
- [Android Supported Formats](https://developer.android.com/guide/topics/media/platform/supported-formats)

> 创建：2026-06-12
> 拆分自：本文档 + 用户反馈「重构异步化，encoder 怎么会都没有？MediaCodec 要充分利用」
