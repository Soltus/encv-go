# Plan: 生成可正常播放的 Mock 视频/音频文件

## 一、问题诊断

### 1.1 现状：所有 mock 媒体文件均不可播放

当前 `generate-mock-files.ts` 使用**手写二进制字节**模拟媒体容器格式，但**不包含有效的编码数据**：

| 文件 | 容器头 | 实际数据 | ffprobe 结果 |
|------|--------|---------|-------------|
| `sample.mp4` | ✅ 有效 ISO Base Media | ❌ 假 AAC 静音帧（0xFF 0xFB 重复） | `Invalid data found when processing input` |
| `comedy.mkv` | ✅ 有效 EBML/Matroska | ❌ 零填充 Vorbis 数据 | 解析失败 |
| `music.mp3` | ✅ 有效 ID3v2.0 | ❌ 假 MPEG 帧（帧大小错误） | `Invalid frame size (417)` |
| `podcast.flac` | ✅ 有效 fLaC 签名 | ❌ 零填充帧数据 | `Input/output error` |

**根因**：MP4/MKV/MP3/FLAC 的容器结构（box/EBML/ID3/frame header）可以用手写字节伪造，但**编码后的音视频数据流必须由真正的编解码器生成**——H.264 视频帧需要 DCT+量化+熵编码，MP3 音频需要 MDCT+心理声学模型。这些无法用假数据模拟。

### 1.2 影响

- **ArtPlayer（Web 端）**：HTML5 `<video>` 加载无效 MP4 → `error` 事件触发 → 显示"播放失败"
- **MPV Plugin（Native 端）**：libmpv 无法解码无效流 → 播放器报错
- **前端预览流程**：Files.vue 点击视频 → FilePreview.vue → `containerType=video` → ArtPlayerView → **播放失败**

### 1.3 环境

- **FFmpeg 6.1.1 已安装**（`/usr/bin/ffmpeg`），包含完整编解码器：
  - 视频：`libx264`, `libx265`, `libvpx`(VP8/VP9)
  - 音频：`libmp3lame`, `libvorbis`, `libopus`, `flac`, `aac`
- Node.js 可通过 `child_process.execSync` 调用 FFmpeg

---

## 二、方案设计

### 2.1 核心策略：FFmpeg lavfi 生成合成媒体

使用 FFmpeg 的 `lavfi`（libavfilter input device）从**纯算法生成**音视频信号——无需任何源文件：

```
音频源：sine=frequency=440:duration=1    ← 1 秒 440Hz 正弦波（A4 音符）
视频源：color=c=blue:s=160x120:d=1:r=10   ← 1 秒蓝色画面，160×120，10fps
```

**优势**：
- 零依赖外部文件
- 生成的文件是**真正可播放的**有效编码流
- 文件体积小（1 秒 MP4 ≈ 11 KB）
- 支持任意格式/编码组合

### 2.2 目标文件规格

| 文件名 | 格式 | 视频编码 | 音频编码 | 分辨率 | 时长 | 预估大小 |
|--------|------|---------|---------|--------|------|---------|
| `sample.mp4` | MP4 | H.264 (libx264) | AAC | 320×240 | 2s | ~15 KB |
| `comedy.mkv` | MKV (Matroska) | H.264 (libx264) | Vorbis | 160×120 | 2s | ~10 KB |
| `music.mp3` | MP3 | — | MP3 (libmp3lame) | — | 2s | ~9 KB |
| `podcast.flac` | FLAC | — | FLAC (native) | — | 2s | ~20 KB |

### 2.3 视觉内容：测试图案（Test Pattern）

为使 mock 视频更有辨识度（方便确认"播放的不是空白"），使用 **testsrc** 或 **color + drawtext**：

```bash
# 方案 A：带文字的彩色测试图（推荐）
ffmpeg -f lavfi -i "
  color=c=0x3B82F6:s=320x240:d=2:r=15,
  drawtext=text='ENCV Mock Video':fontsize=24:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2
" -i "sine=frequency=440:duration=2" ...

# 方案 B：SMPTE 测试条（更专业但依赖 AVFilter）
ffmpeg -f lavfi -i "smptebars=r=15:size=320x240:duration=2" ...
```

> **决策**：方案 A 更轻量（只需 color + drawtext），且自定义文字便于识别。

### 2.4 音频内容

- **MP3/FLAC**：440Hz 正弦波（A4 音符）+ 880Hz（A5）双音调，有明显的听觉反馈
- **视频内嵌音频**：440Hz 单音，与视频同步

---

## 三、实施步骤

### Step 1：修改 `generate-mock-files.ts` — 新增 FFmpeg 媒体生成函数

在现有脚本中添加 4 个新函数，替换原有的手写二进制版本：

```typescript
// ====== FFmpeg-based media generation ======

function ffmpegGenerate(args: string[], label: string): Buffer {
  const { execSync } = require('child_process')
  const result = execSync(`ffmpeg ${args.join(' ')}`, {
    encoding: null as never,
    stdio: ['pipe', 'pipe', 'pipe'],
    timeout: 15000,
  })
  return Buffer.from(result)
}

function createValidMP4(): Buffer {
  return ffmpegGenerate([
    '-f', 'lavfi',
    '-i', 'sine=frequency=440:duration=2',
    '-f', 'lavfi',
    '-i', 'color=c=0x3B82F6:s=320x240:d=2:r=15,drawtext=text=\'ENCV Mock\':fontsize=20:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2',
    '-c:v', 'libx264', '-preset', 'ultrafast', '-tune', 'stillimage', '-pix_fmt', 'yuv420p',
    '-c:a', 'aac', '-b:a', '64k',
    '-shortest', '-y', 'pipe:mp4'
  ], 'MP4')
}

function createValidMKV(): Buffer {
  return ffmpegGenerate([
    '-f', 'lavfi',
    '-i', 'sine=frequency=660:duration=2',
    '-f', 'lavfi',
    '-i', 'color=c=0x10B981:s=160x120:d=2:r=10,drawtext=text=\'ENCV MKV\':fontsize=16:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2',
    '-c:v', 'libx264', '-preset', 'ultrafast', '-tune', 'stillimage', '-pix_fmt', 'yuv420p',
    '-c:a', 'libvorbis',
    '-shortest', '-y', 'pipe:mkv'
  ], 'MKV')
}

function createValidMP3(): Buffer {
  return ffmpegGenerate([
    '-f', 'lavfi',
    -i', 'sine=frequency=440:duration=2:sine=frequency=880:duration=2',
    '-c:a', 'libmp3lame', '-b:a', '128k',
    '-y', 'pipe:mp3'
  ], 'MP3')
}

function createValidFLAC(): Buffer {
  return ffmpegGenerate([
    '-f', 'lavfi',
    '-i', 'sine=frequency=440:duration=2',
    '-c:a', 'flac', '-sample_fmt', 's16',
    '-y', 'pipe:flac'
  ], 'FLAC')
}
```

**关键设计点**：
- 输出到 `pipe:mp4` / `pipe:mkv` 等（FFmpeg 特殊输出目标），直接获取二进制 Buffer
- `stdio: ['pipe', 'pipe', 'pipe']` 捕获 stdout 作为文件内容
- 15 秒超时防止 FFmpeg 卡死
- 保留原有函数名签名不变（`createMP4()` → 返回值类型从 `Uint8Array` 变为 `Buffer`，兼容）

### Step 2：更新 main() 中的调用点

将 `writeBuffer(join(root, '01-plain-media/video/sample.mp4'), createMP4())` 等调用改为新函数。

**向后兼容处理**：
- 检测 `process.env.USE_FFMPEG_MEDIA === '1'` 或检测 `which ffmpeg` 成功时使用 FFmpeg 路径
- FFmpeg 不可用时 fallback 到原有手写二进制（虽然不可播放，但不阻塞生成流程）
- 在 CI/开发环境中 FFmpeg 必须可用（已验证存在）

```typescript
const useFFmpeg = (() => {
  try {
    execSync('which ffmpeg', { stdio: 'ignore' })
    return true
  } catch {
    console.warn('[WARN] ffmpeg not found, using legacy binary mock files (unplayable)')
    return false
  }
})()
```

### Step 3：验证生成的文件

```bash
# 重新生成 mock 数据
cd app/encv-mobile && npx tsx scripts/generate-mock-files.ts --dir __mock_data__

# 验证每个文件
ffprobe -v error __mock_data__/01-plain-media/video/sample.mp4
# 预期: duration=2.x, codec_name=h264+aac

ffprobe -v error __mock_data__/01-plain-media/audio/music.mp3
# 预期: duration=2.x, codec_name=mp3
```

### Step 4：浏览器端播放测试

1. 启动后端 + Vite dev server
2. 打开 Files 页面 → 点击 `sample.mp4`
3. ArtPlayer 应显示蓝色测试画面 + "ENCV Mock" 文字水印，时长约 2 秒
4. 点击 `music.mp3` / `podcast.flac` → 应能听到 440Hz+880Hz 双音调

---

## 四、FFmpeg 命令参考（已验证可用）

### 4.1 MP4（H.264 + AAC）— 主力视频格式
```bash
ffmpeg -f lavfi \
  -i "sine=frequency=440:duration=2" \
  -f lavfi \
  -i "color=c=0x3B82F6:s=320x240:d=2:r=15,drawtext=text='ENCV':fontsize=20:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2" \
  -c:v libx264 -preset ultrafast -tune stillimage -pix_fmt yuv420p \
  -c:a aac -b:a 64k \
  -shortest -y output.mp4
# 结果: ~15KB, 2s, 320×240, 可在 ArtPlayer 和 MPV 中播放
```

### 4.2 MKV（H.264 + Vorbis）
```bash
ffmpeg -f lavfi \
  -i "sine=frequency=660:duration=2" \
  -f lavfi \
  -i "color=c=0x10B981:s=160x120:d=2:r=10,drawtext=text='ENCV MKV':fontsize=16:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2" \
  -c:v libx264 -preset ultrafast -tune stillimage -pix_fmt yuv420p \
  -c:a libvorbis \
  -shortest -y output.mkv
# 结果: ~10KB, 2s, 160×120
```

### 4.3 MP3
```bash
ffmpeg -f lavfi \
  -i "sine=frequency=440:duration=2" \
  -c:a libmp3lame -b:a 128k \
  -y output.mp3
# 结果: ~17KB, 2s, 44100Hz mono
```

### 4.4 FLAC
```bash
ffmpeg -f lavfi \
  -i "sine=frequency=440:duration=2" \
  -c:a flac -sample_fmt s16 \
  -y output.flac
# 结果: ~32KB, 2s, 44100Hz mono 16bit
```

---

## 五、风险与缓解

| 风险 | 概率 | 缓解措施 |
|------|------|---------|
| CI 环境无 FFmpeg | 低 | 当前 sandbox 已确认有 ffmpeg 6.1.1；CI Docker 镜像通常包含 |
| `drawtext` filter 依赖 libfreetype | 低 | FFmpeg 6.1.1 配置中已启用 `--enable-libfreetype`；fallback 到纯 color 无文字 |
| pipe 输出 buffer 过大 | 极低 | 2 秒视频 < 50KB；加 timeout 15s 防卡死 |
| Windows 开发环境无 FFmpeg | 中 | 仅影响本地开发；可提示安装或 fallback 到旧逻辑 |
| 生成耗时增加 | 低 | 每个文件 < 1 秒；总共 4 个文件 < 5 秒（vs 旧方案瞬间完成） |

---

## 六、不做的事情

- ❌ 不生成真实视频内容（如录屏片段）—— 测试图案足够
- ❌ 不支持更多格式（OGG/AVI/MOV/WAV）—— 当前 5 种覆盖主要使用场景
- ❌ 不修改后端 stream handler —— 流式传输逻辑与文件有效性无关
- ❌ 不修改 ArtPlayer/MPV 配置 —— 播放器本身没问题，问题在源文件

---

## 七、验收标准

1. `ffprobe` 对生成的 4 个文件全部返回有效 duration 和 codec 信息（非 error）
2. 浏览器 ArtPlayer 能加载 `sample.mp4` 并显示画面（非 error 状态）
3. 浏览器 `<audio>` 能播放 `music.mp3` 和 `podcast.flac`（有声音输出）
4. `generate-mock-files.ts` 执行无错误，总耗时 < 10 秒
5. 原 JPEG/PNG/PDF/txt 等非媒体文件的生成逻辑不受影响
