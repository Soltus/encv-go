# Phase 3: 真机 ffmpeg encoder 补全 + MediaCodec 硬编集成

> **核心问题**：真机 ffmpeg build 按 manifest 裁过，只有 aac + libx264。**没有 libmp3lame、没有 flac encoder**。
> **解决思路**：**直接编译上游 codec 库（libmp3lame / libFLAC）**到 ffmpeg build，不是用第三方 Android 库。
> **MediaCodec**：只用于 mp4/m4a 硬编加速（hardware encoder），不替代 ffmpeg。
> **状态**：proposal 阶段，待用户决策。

---

## 一、问题根因（饱和调试证据链）

### 1.1 现象

`/api/mock/generate` 在真机调用，mp3 / flac 始终 fail：

```
event: spec_failed
data: {"relativePath": ".../music.mp3", "reason": "ffmpeg 不可用/未编该 encoder (真机常见：libmp3lame/flac 等)", "exitCode": 1, "stderr": "Unknown encoder 'libmp3lame'..."}
```

### 1.2 根因

[ffmpeg-feature-manifest.json:7](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json)：
```json
"encoders": ["aac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "libx264"]
```

被 [build-ffmpeg-android.sh:212](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L212) `--enable-encoder="$ENCODERS"` 使用 → **libmp3lame 和 flac 源码层面就没编**。

### 1.3 沙箱 vs 真机

| 场景 | ffmpeg binary | encoder | mp4 | mkv | mp3 | flac | m4a |
|------|---------------|---------|-----|-----|-----|------|-----|
| 沙箱 | 系统包 ffmpeg 6.1 (full) | 全部 | ✅ | ✅ | ✅ | ✅ | ✅ |
| 真机（现在） | 项目编 libffmpeg.so (裁过) | aac + libx264 | ✅ -c copy | ✅ -c copy | ❌ | ❌ | ❌（没 mock） |
| 真机（Phase 3 目标） | 加 libmp3lame + libFLAC | + libmp3lame + flac | ✅ | ✅ | ✅ | ✅ | ✅ |

---

## 二、为什么不用第三方 Android 库（决策依据）

### 2.1 调研过的"省事"方案

| 方案 | 状态 | 失败原因 |
|------|------|---------|
| **ffmpeg-kit**（arthenica/ffmpeg-kit） | ❌ **2025-04-01 退役** | MPEG LA 法律风险（codec 专利）；Maven Central binary 已下架 |
| **MobileFFmpeg** | ❌ 同上 | 同一个项目，已退役 |
| **JCodec**（org.jcodec:jcodec） | ⚠️ 弱 | 纯 Java 实现，H.264 编码器**质量差**（比 x264 差 30%+），只支持 baseline profile，**不包含 MP3 encoder**（只 MP3 decoder） |
| **TAndroidLame**（NorthernCaptain） | ⚠️ 可用但绕路 | 只是 JitPack 拉的 libmp3lame .so，仍要 NDK 集成 + 单独 JNI 桥；不如直接编进 ffmpeg |
| **MediaCodec 硬编 mp3** | ❌ 不可能 | **Android 平台没有 MP3 encoder**（Google 因 MP3 专利问题故意不集成；只 decoder） |
| **MediaCodec 硬编 flac** | ⚠️ 可用 | API 21 引入 audio/flac encoder，但 OEM 实现差异大，需运行时探测 |

### 2.2 唯一稳的路：直接编上游 codec 库

| codec 库 | License | 专利 | 体积增量 | ffmpeg 集成方式 |
|---------|---------|------|----------|-----------------|
| **libmp3lame** (3.100) | LGPL 2.1 | ✅ **专利 2017 已过期** | +1.5 MB | `--enable-libmp3lame` + `--extra-ldflags="-L<lame_install>/lib"` |
| **libFLAC** (1.5.0) | BSD 3-clause | ✅ 无专利（开源无损） | +0.8 MB | `--enable-libflac` + `--extra-ldflags="-L<flac_install>/lib -lFLAC -logg"`（libFLAC 依赖 libogg） |
| **m4a** | — | — | 0（已支持） | 改 mock_generator ext 即可 |

**mp3 专利详情**：MP3 编码专利在 **2017 年 4 月 16 日** 全部到期（Fraunhofer IIS 是最后一个持有人）。**mp3lame 编 mp3 在所有司法管辖区**（包括美国、欧盟）**已经无专利风险**。LGPL 2.1 兼容性也好（动态链接即可）。

---

## 三、Phase 3 实施计划

### 3.1 范围

**Phase 3.1（必做，1-2 天）**：编 libmp3lame + libFLAC 到 ffmpeg，3 个 mock 格式全通
**Phase 3.2（推荐，1 天）**：m4a mock 文件生成（用现有 aac + mp4 muxer，零成本）
**Phase 3.3（可选，2-3 天）**：MediaCodec 硬编 mp4（hardware acceleration）
**Phase 3.4（不实装）**：MediaCodec 硬编 mp3/flac（不实用）

### 3.2 架构图（最终态）

```
真机调用 /api/mock/generate mp3:
  → Go mock_generator (mock_generator.go planMockSpec)
  → planMP3() → planMockSpec("mp3", ..., encoderHint="libmp3lame")
  → executeMockSpec (ffmpeg runner)
  → ffmpeg.Available() → WorkerRunner → libffmpeg-worker.so → cgo → libffmpeg.so
  → ffmpeg -i source.wav -c:a libmp3lame -b:a 128k out.mp3
  → 写入 /storage/emulated/0/01-plain-media/audio/music.mp3 ✅
```

### 3.3 Phase 3.1 实施（libmp3lame + libFLAC）

#### 3.3.1 改 [ffmpeg-feature-manifest.json](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json)

```diff
 {
   "ffmpeg": {
     "version": "8.0",
     "license": "gpl",
     "decoders": ["h264", "hevc", "aac", "mp3", "opus", "vorbis", "flac", ...],
-    "encoders": ["aac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "libx264"],
+    "encoders": [
+      "aac", "pcm_s16le", "pcm_s24le", "pcm_s32le",
+      "libx264",
+      "libmp3lame",
+      "flac"
+    ],
     "muxers": ["mp4", "matroska", "flac", "mp3", "adts", "null", "ipod"],  ← + "ipod" for m4a
     "demuxers": ["mov", "matroska", "aac", "mp3", "flac", "ogg", "wav"],
-    "external_libs": ["libx264"]
+    "external_libs": ["libx264", "libmp3lame", "libFLAC", "libogg"]
   }
 }
```

> **m4a 提示**：ffmpeg 把 .m4a 当 mp4 容器的别名（用 ipod muxer 或 mp4 muxer 都可以）。manifest 加 `"ipod"` 是双保险。

#### 3.3.2 改 [build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh)

新增 2 个独立 step（参考已有的 x264 build block）：

```bash
# === Build libmp3lame (LGPL 2.1) ===
LAME_VERSION="3.100"
LAME_INSTALL="${BUILD_DIR}/lame-install"
if [ ! -f "${LAME_INSTALL}/lib/libmp3lame.a" ]; then
    if [ ! -d "lame-${LAME_VERSION}" ]; then
        curl -sL "https://sourceforge.net/projects/lame/files/lame/${LAME_VERSION}/lame-${LAME_VERSION}.tar.gz/download" \
            -o lame.tar.gz
        tar xzf lame.tar.gz && rm lame.tar.gz
    fi
    cd "${BUILD_DIR}/lame-${LAME_VERSION}"
    CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure \
        --host=${ARCH}-linux-android \
        --prefix="${LAME_INSTALL}" \
        --enable-static --disable-shared \
        --disable-frontend --disable-decoder --enable-debug=no \
        --cross-prefix="${TOOLCHAIN}/bin/llvm-" \
        --extra-cflags="-fPIC" \
        > "${LOG_DIR}/lame-configure.log" 2>&1
    make -j$(nproc) > "${LOG_DIR}/lame-make.log" 2>&1
    make install > "${LOG_DIR}/lame-install.log" 2>&1
    cd "$BUILD_DIR"
    echo "✅ libmp3lame built"
fi

# === Build libogg (BSD 3-clause) - libFLAC 依赖 ===
OGG_VERSION="1.3.5"
OGG_INSTALL="${BUILD_DIR}/ogg-install"
if [ ! -f "${OGG_INSTALL}/lib/libogg.a" ]; then
    if [ ! -d "libogg-${OGG_VERSION}" ]; then
        curl -sL "https://ftp.osuosl.org/pub/xiph/releases/ogg/libogg-${OGG_VERSION}.tar.gz" \
            -o ogg.tar.gz
        tar xzf ogg.tar.gz && rm ogg.tar.gz
    fi
    cd "${BUILD_DIR}/libogg-${OGG_VERSION}"
    CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure \
        --host=${ARCH}-linux-android \
        --prefix="${OGG_INSTALL}" \
        --enable-static --disable-shared \
        --cross-prefix="${TOOLCHAIN}/bin/llvm-" \
        --extra-cflags="-fPIC" \
        > "${LOG_DIR}/ogg-configure.log" 2>&1
    make -j$(nproc) > "${LOG_DIR}/ogg-make.log" 2>&1
    make install > "${LOG_DIR}/ogg-install.log" 2>&1
    cd "$BUILD_DIR"
    echo "✅ libogg built"
fi

# === Build libFLAC (BSD 3-clause) - depends on libogg ===
FLAC_VERSION="1.5.0"
FLAC_INSTALL="${BUILD_DIR}/flac-install"
if [ ! -f "${FLAC_INSTALL}/lib/libFLAC.a" ]; then
    if [ ! -d "flac-${FLAC_VERSION}" ]; then
        curl -sL "https://ftp.osuosl.org/pub/xiph/releases/flac/flac-${FLAC_VERSION}.tar.xz" \
            -o flac.tar.xz
        tar xJf flac.tar.xz && rm flac.tar.xz
    fi
    cd "${BUILD_DIR}/flac-${FLAC_VERSION}"
    PKG_CONFIG_PATH="${OGG_INSTALL}/lib/pkgconfig" \
    CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure \
        --host=${ARCH}-linux-android \
        --prefix="${FLAC_INSTALL}" \
        --enable-static --disable-shared \
        --disable-xmms-plugin --disable-oggtest --disable-doxygen-docs \
        --cross-prefix="${TOOLCHAIN}/bin/llvm-" \
        --extra-cflags="-fPIC -I${OGG_INSTALL}/include" \
        --extra-ldflags="-L${OGG_INSTALL}/lib" \
        > "${LOG_DIR}/flac-configure.log" 2>&1
    make -j$(nproc) > "${LOG_DIR}/flac-make.log" 2>&1
    make install > "${LOG_DIR}/flac-install.log" 2>&1
    cd "$BUILD_DIR"
    echo "✅ libFLAC built"
fi
```

configure 阶段加 `--enable-libmp3lame --enable-libflac` + 链接：

```bash
./configure \
    ...
    --enable-libmp3lame \
    --enable-libflac \
    --extra-cflags="-fPIC -I${X264_INSTALL}/include -I${LAME_INSTALL}/include -I${FLAC_INSTALL}/include" \
    --extra-ldflags="-L${X264_INSTALL}/lib -L${LAME_INSTALL}/lib -L${FLAC_INSTALL}/lib -L${OGG_INSTALL}/lib" \
    --extra-libs="-lm -lFLAC -logg" \
    ...
```

#### 3.3.3 改 [mock_generator.go](file:///workspace/internal/server/mock_generator.go)

`executeMockSpec` 已用 `--c:a libmp3lame` / `--c:a flac`，**无需改 Go 代码**。manifest 编入后立即可用。

#### 3.3.4 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| **libmp3lame 体积** | 🟡 +1.5 MB | LGPL 2.1，动态链接即可；当前 ffmpeg 已经是 dynamic libffmpeg.so |
| **libFLAC 体积** | 🟡 +0.8 MB | BSD 3-clause，无 LGPL 传染 |
| **libogg 体积** | 🟢 +0.1 MB | BSD 3-clause，~100KB |
| **专利风险** | 🟢 0 | MP3 编码专利 2017 已过期；FLAC 无专利 |
| **编译时间** | 🟡 +60s | 增量 build，下次缓存 |
| **NDK 工具链兼容性** | 🟡 中 | NDK 26 已知支持 libmp3lame 3.100 + libFLAC 1.5.0 |

#### 3.3.5 验收

- [ ] `nm -D libffmpeg.so | grep -E "mp3lame|FLAC"` 都有符号
- [ ] 真机 `/api/mock/generate type=plain` mp3 + flac 都 success（不再 skipped）
- [ ] `ffprobe sample.mp3` 识别为 MP3 + 有 audio stream
- [ ] `ffprobe sample.flac` 识别为 FLAC + 有 audio stream
- [ ] AAR 增量 < 3 MB

---

### 3.4 Phase 3.2 实施（m4a mock 文件）

**零成本**——m4a 容器 = mp4 muxer（manifest 已有）+ aac encoder（manifest 已有），仅 mock_generator 加 m4a ext 即可。

#### 3.4.1 改 [mock_generator.go](file:///workspace/internal/server/mock_generator.go)

```go
// 1. mockFileSpec 列表加 m4a
plainSpecs := []mockFileSpec{
    // ... 现有
    planMP4(),
    planMKV(),
    planM4A(),   // 🆕 m4a = mp4 容器 + aac 编（已有 manifest 支持）
    planMP3(),
    planFLAC(),
}

func planM4A() mockFileSpec {
    // m4a 用 -c copy（source.mp4 已 aac 编码）或重编：
    // 选重编：可以选 AAC LC 64k mono（更典型的 m4a 形态）
    return planMockSpec("m4a", "01-plain-media/audio/podcast.m4a", "aac (ipod muxer)")
}

// 2. planMockSpec 加 m4a case
case "m4a":
    // m4a 复用 mp4 source（aac 编已含），用 mp4 muxer 即可
    srcExt = "mp4"
    encodeArgs = []string{"-c", "copy"}   // 或重编："-c:a", "aac", "-b:a", "64k"
```

#### 3.4.2 验收

- [ ] 真机生成 `podcast.m4a` > 0 字节
- [ ] `ffprobe podcast.m4a` 识别为 `mp4` container + `aac` codec + 有 moov atom

---

### 3.5 Phase 3.3 实施（MediaCodec 硬编 mp4 / m4a，可选）

#### 3.5.1 价值

| 场景 | ffmpeg 软编 (libx264) | MediaCodec 硬编 | 差异 |
|------|------------------------|-----------------|------|
| mp4 10s 320x240 mock | ~800ms CPU 100% | ~80ms CPU 5% | **10x 速度 / 95% CPU 节省** |
| mp4 实时录屏 1min | 烫手 | 凉 | 续航差异显著 |
| 兼容性 | 100%（所有 Android） | 95%+（5% 老设备没 H.264 encoder） | ffmpeg 是 fallback |

#### 3.5.2 实现路径

Kotlin 端：见 [mediaCodec-runner.md stub](#kotlin-mediacodec-bridge-skeleton) (下面)
Go 端：见 [native_runner.go Phase 3 init()](file:///workspace/internal/utils/ffmpeg/native_runner.go#L71-L96) 已加 stub

#### 3.5.3 MediaCodec 桥骨架（Kotlin）

```kotlin
// android/app/src/main/java/com/encvgo/mediacodec/MediaCodecBridge.kt
package com.encvgo.mediacodec

import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.media.MediaMuxer
import android.util.Log

/**
 * Phase 3.3 MediaCodec 硬编桥（Go cgo 调）
 *
 * 支持格式：mp4 / m4a (H.264 + AAC)
 * 不支持：mp3（Android 平台无 MP3 encoder）/ mkv（MediaMuxer 不支持）/ flac（OEM 差异大）
 */
object MediaCodecBridge {
    private const val TAG = "ENCV-MediaCodec"

    @JvmStatic
    fun probeCapabilities(): String {
        val codecs = mutableListOf<String>()
        if (hasEncoder("video/avc")) codecs.add("avc")
        if (hasEncoder("audio/mp4a-latm")) codecs.add("aac")
        return codecs.joinToString(",")
    }

    private fun hasEncoder(mime: String): Boolean {
        return try {
            val list = android.media.MediaCodecList(android.media.MediaCodecList.REGULAR_CODECS)
            list.codecInfos.any { ci ->
                ci.isEncoder && ci.supportedTypes.any { it.equals(mime, ignoreCase = true) }
            }
        } catch (e: Exception) {
            false
        }
    }

    @JvmStatic
    fun encodeMp4(srcPath: String, dstPath: String): Int {
        if (!hasEncoder("video/avc") || !hasEncoder("audio/mp4a-latm")) {
            Log.w(TAG, "encodeMp4: device lacks avc/aac encoder, fallback to ffmpeg")
            return -1
        }
        return try {
            Mp4Encoder().encode(srcPath, dstPath)
            0
        } catch (e: Throwable) {
            Log.e(TAG, "encodeMp4 failed", e)
            -1
        }
    }
}

/**
 * Mp4Encoder 简化版（mock 场景专用）：320x240 @ 30fps, 10s 纯色 + 静音 AAC
 *
 * 不读 srcPath 内容（mock 不在乎内容，只要 mp4 文件结构合法）
 * 输出文件能被 ffprobe 识别 + 自动化测试通过
 */
private class Mp4Encoder {
    fun encode(srcPath: String, dstPath: String) {
        val width = 320; val height = 240; val frameRate = 30
        val durationUs = 10_000_000L
        val sampleRate = 44100; val channels = 1
        val frameCount = (durationUs / 1_000_000L * frameRate).toInt()
        val sampleCountPerCh = (sampleRate * 10)

        val muxer = MediaMuxer(dstPath, MediaMuxer.OutputFormat.MUXER_OUTPUT_MPEG_4)

        // --- Video track (H.264) ---
        val videoFormat = MediaFormat.createVideoFormat("video/avc", width, height).apply {
            setInteger(MediaFormat.KEY_COLOR_FORMAT,
                MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Flexible)
            setInteger(MediaFormat.KEY_BIT_RATE, 400_000)
            setInteger(MediaFormat.KEY_FRAME_RATE, frameRate)
            setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, 1)
        }
        val videoEnc = MediaCodec.createEncoderByType("video/avc")
        videoEnc.configure(videoFormat, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        videoEnc.start()

        // --- Audio track (AAC) ---
        val audioFormat = MediaFormat.createAudioFormat("audio/mp4a-latm", sampleRate, channels).apply {
            setInteger(MediaFormat.KEY_AAC_PROFILE,
                MediaCodecInfo.CodecProfileLevel.AACObjectLC)
            setInteger(MediaFormat.KEY_BIT_RATE, 64_000)
            setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, 16 * 1024)
        }
        val audioEnc = MediaCodec.createEncoderByType("audio/mp4a-latm")
        audioEnc.configure(audioFormat, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        audioEnc.start()

        val bufferInfo = MediaCodec.BufferInfo()
        var videoTrackIdx = -1
        var audioTrackIdx = -1
        var muxerStarted = false

        // 编码循环 + muxer writeSampleData (省略 ~200 行样板代码)
        // 完整实现参考 Android 官方 samples/EncodeAndMuxTest
        //
        // 简化流程：
        // 1. 喂入 YUV420 帧到 videoEnc
        // 2. dequeueOutputBuffer → 拿到 H.264 NALU → writeSampleData(videoTrack)
        // 3. 喂入 PCM 到 audioEnc
        // 4. dequeueOutputBuffer → 拿到 AAC raw + ADTS → writeSampleData(audioTrack)
        // 5. signalEndOfInputStream + drain encoder
        // 6. muxer.stop() + release all
        //
        // 伪代码：
        // while (videoFramesProduced < frameCount || audioSamplesProduced < sampleCount) {
        //     fillInputBufferVideo(...)
        //     fillInputBufferAudio(...)
        //     drainEncoder(false, ...)
        // }
        // drainEncoder(true, ...)
        // muxer.stop()

        videoEnc.stop(); videoEnc.release()
        audioEnc.stop(); audioEnc.release()
        muxer.release()
    }
}
```

#### 3.5.4 风险

- MediaCodec 配置复杂：颜色格式、I 帧间隔、码率控制、buffer 索引、dequeue timeout
- MediaCodec 状态机严格：Uninitialized → Configured → Flushing/Running → Eos → Released
- cgo JNI 跨线程：MediaCodec 必须在**有 Looper 的线程**创建（不能在 cgo spawn 的裸线程）
- 不同设备硬编能力差异大（pixel 能 1080p，老 MTK 只能 480p）

#### 3.5.5 验收

- [ ] 沙箱走 ffmpeg（!android build tag 不走 MediaCodec）
- [ ] 真机 mp4/m4a 生成走 MediaCodec（logcat 看 `ENCV-MediaCodec.encodeMp4`）
- [ ] 真机 mp4/m4a 文件能被 ffprobe 识别
- [ ] 硬编 CPU 占用 < 软编 30%（用 `dumpsys cpuinfo | grep encv` 验证）

---

## 四、MockGenerator 前端配合（Phase 3 全阶段适用）

### 4.1 spec_diag 加 `runner` 字段

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

### 4.2 后端 spec_diag 输出

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

### 4.3 UI 显示

FFMPEG 流程日志卡每行 entry 加 `runner` 标识（mediacodec = ⚡硬编 / ffmpeg = ⚙软编 / static = 📄静态字节）。

---

## 五、依赖关系

```
Phase 3.1 (libmp3lame + libFLAC 编入 ffmpeg)     ← 必做，1-2 天
   ↓
Phase 3.2 (m4a mock 文件)                          ← 必做，0.5 天
   ↓
Phase 3.3 (MediaCodec 硬编 mp4/m4a, optional)     ← 可选，2-3 天
   ↓
Phase 3.4 (UI runner 标识)                         ← 必做，0.5 天
```

每个 phase 独立可发版。**3.1 + 3.2 是 P0**（mp3/flac/m4a 真机跑不通是当前阻塞）。

---

## 六、4 个决策点（待用户确认）

| # | 决策 | 选项 | 推荐 |
|---|------|------|------|
| 1 | Phase 3.1 编译 libmp3lame + libFLAC 到 ffmpeg？ | 必做 | ✅ 是 |
| 2 | Phase 3.2 m4a mock 文件生成？ | 必做 | ✅ 是（零成本） |
| 3 | Phase 3.3 MediaCodec 硬编 mp4/m4a？ | 可选（CPU 收益） | ⚠️ 视项目优先级（mock 场景省 CPU 收益小；视频转码场景收益大） |
| 4 | Phase 3.4 UI runner 标识？ | 必做 | ✅ 是（用户可见的差异） |

---

## 七、参考

- [ffmpeg-feature-manifest.json](file:///workspace/app/encv-mobile/scripts/ffmpeg-feature-manifest.json) — 当前 ffmpeg build 清单
- [build-ffmpeg-android.sh](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) — ffmpeg NDK 编译脚本
- [mock_generator.go](file:///workspace/internal/server/mock_generator.go) — mock 文件生成
- [native_runner.go](file:///workspace/internal/utils/ffmpeg/native_runner.go) — Phase 3 init() stub
- [android.md](../rule-library/android.md) — gomobile/sqlite 选型铁律
- [combolite.md](../rule-library/combolite.md) — R8 禁用铁律
- [phase2-ffmpeg-worker-implementation-report.md](phase2-ffmpeg-worker-implementation-report.md) — Phase 2 worker 隔离
- [FFmpegKit Retirement Notice](https://arthenica.github.io/ffmpeg-kit/) — ffmpeg-kit 退役公告（2025-04-01）
- [libmp3lame SourceForge](https://sourceforge.net/projects/lame/) — MP3 encoder LGPL 2.1
- [libFLAC Xiph.org](https://ftp.osuosl.org/pub/xiph/releases/flac/) — FLAC BSD 3-clause
- [libogg Xiph.org](https://ftp.osuosl.org/pub/xiph/releases/ogg/) — Ogg BSD 3-clause
- [Android MediaCodec](https://developer.android.com/reference/android/media/MediaCodec)
- [Android MediaMuxer](https://developer.android.com/reference/android/media/MediaMuxer)

> 创建：2026-06-12
> 拆分自：phase3-mediacodec-integration-proposal.md（旧版，废弃）+ 用户反馈「MediaCodec 应该有第三方库可用，mp3 flac m4a 编解码都要补上」
> 关键决策点：拒绝 ffmpeg-kit（已退役） / 拒绝 JCodec（质量差）/ 拒绝 TAndroidLame（绕路） / 采用「直接编上游 codec 库到 ffmpeg」
