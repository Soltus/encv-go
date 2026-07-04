---
name: "ffmpeg"
description: "FFmpeg音视频处理命令参考指南。Invoke when user needs to convert media formats, process video/audio, apply filters, extract streams, or work with any FFmpeg-related tasks."
---

# FFmpeg 音视频处理指南

FFmpeg 是一个强大的开源多媒体框架，能够解码、编码、转码、复用、解复用、流式传输、过滤和播放几乎所有格式的音视频。

## 基础命令结构

```bash
ffmpeg [全局选项] [输入选项] -i 输入文件 [输出选项] 输出文件
```

## 常用全局选项

| 选项 | 说明 | 示例 |
|------|------|------|
| `-y` | 覆盖输出文件（不询问） | `ffmpeg -y -i input.mp4 output.avi` |
| `-n` | 不覆盖输出文件 | `ffmpeg -n -i input.mp4 output.avi` |
| `-stats` | 显示编码进度统计 | `ffmpeg -stats -i input.mp4 output.mp4` |
| `-hide_banner` | 隐藏版本和编译信息 | `ffmpeg -hide_banner -i input.mp4 output.mp4` |
| `-loglevel` | 设置日志级别 (quiet/panic/fatal/error/warning/info/verbose/debug) | `ffmpeg -loglevel error -i input.mp4 output.mp4` |

## 输入/输出选项

### 输入选项 (-i 之前)

| 选项 | 说明 | 示例 |
|------|------|------|
| `-ss` | 开始时间偏移 | `ffmpeg -ss 00:01:30 -i input.mp4 output.mp4` |
| `-t` | 持续时间 | `ffmpeg -i input.mp4 -t 60 output.mp4` |
| `-f` | 强制输入格式 | `ffmpeg -f lavfi -i testsrc=duration=10:size=320x240:rate=30 output.mp4` |

### 输出选项 (-i 之后)

| 选项 | 说明 | 示例 |
|------|------|------|
| `-c:a` / `-acodec` | 音频编码器 | `ffmpeg -i input.mp4 -c:a aac output.mp4` |
| `-c:v` / `-vcodec` | 视频编码器 | `ffmpeg -i input.mp4 -c:v libx264 output.mp4` |
| `-c:s` / `-scodec` | 字幕编码器 | `ffmpeg -i input.mkv -c:s mov_text output.mp4` |
| `-b:a` | 音频比特率 | `ffmpeg -i input.mp4 -b:a 128k output.mp4` |
| `-b:v` | 视频比特率 | `ffmpeg -i input.mp4 -b:v 2000k output.mp4` |
| `-r` | 帧率 | `ffmpeg -i input.mp4 -r 30 output.mp4` |
| `-s` | 分辨率 (宽x高) | `ffmpeg -i input.mp4 -s 1920x1080 output.mp4` |
| `-ar` | 音频采样率 | `ffmpeg -i input.mp4 -ar 44100 output.mp4` |
| `-ac` | 音频通道数 | `ffmpeg -i input.mp4 -ac 2 output.mp4` |
| `-pix_fmt` | 像素格式 | `ffmpeg -i input.mp4 -pix_fmt yuv420p output.mp4` |
| `-preset` | 编码预设 (ultrafast/superfast/veryfast/faster/fast/medium/slow/slower/veryslow) | `ffmpeg -i input.mp4 -c:v libx264 -preset slow output.mp4` |
| `-crf` | 恒定质量因子 (0-51, 越小质量越高) | `ffmpeg -i input.mp4 -c:v libx264 -crf 23 output.mp4` |

## 常用编码器

### 视频编码器

| 编码器 | 用途 | 示例 |
|--------|------|------|
| `libx264` | H.264/AVC (最常用) | `-c:v libx264` |
| `libx265` | H.265/HEVC (更高压缩率) | `-c:v libx265` |
| `libvpx-vp9` | VP9 (WebM) | `-c:v libvpx-vp9` |
| `libaom-av1` | AV1 (下一代编码) | `-c:v libaom-av1` |
| `mpeg4` | MPEG-4 Part 2 | `-c:v mpeg4` |
| `copy` | 复制流（不重新编码） | `-c:v copy` |

### 音频编码器

| 编码器 | 用途 | 示例 |
|--------|------|------|
| `aac` | AAC (推荐) | `-c:a aac` |
| `libmp3lame` | MP3 | `-c:a libmp3lame` |
| `libopus` | Opus (高质量低延迟) | `-c:a libopus` |
| `libvorbis` | Vorbis (OGG) | `-c:a libvorbis` |
| `flac` | FLAC (无损) | `-c:a flac` |
| `copy` | 复制流（不重新编码） | `-c:a copy` |

## 常用操作示例

### 格式转换

```bash
# MP4 转 AVI
ffmpeg -i input.mp4 output.avi

# 视频转音频
ffmpeg -i input.mp4 -vn -c:a mp3 output.mp3

# 提取音频（保持原格式）
ffmpeg -i input.mp4 -vn -c:a copy output.aac
```

### 视频裁剪与剪辑

```bash
# 从第30秒开始，截取60秒
ffmpeg -ss 00:00:30 -t 60 -i input.mp4 -c copy output.mp4

# 截取前10秒
ffmpeg -i input.mp4 -t 10 -c copy output.mp4

# 截取最后10秒（需要先获取时长）
ffmpeg -sseof -10 -i input.mp4 -c copy output.mp4
```

### 视频缩放与分辨率调整

```bash
# 缩放到 1280x720
ffmpeg -i input.mp4 -vf "scale=1280:720" output.mp4

# 保持宽高比，宽度设为1280，高度自动
ffmpeg -i input.mp4 -vf "scale=1280:-1" output.mp4

# 保持宽高比，高度设为720，宽度自动
ffmpeg -i input.mp4 -vf "scale=-1:720" output.mp4

# 强制精确分辨率（可能变形）
ffmpeg -i input.mp4 -vf "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2" output.mp4
```

### 视频压缩与优化

```bash
# H.264 压缩（推荐）
ffmpeg -i input.mp4 -c:v libx264 -crf 23 -preset medium -c:a aac -b:a 128k output.mp4

# H.265 压缩（更高压缩率，更慢）
ffmpeg -i input.mp4 -c:v libx265 -crf 28 -preset medium -c:a aac -b:a 128k output.mp4

# 限制文件大小（例如 10MB）
ffmpeg -i input.mp4 -fs 10M output.mp4

# 2-pass 编码（更好的质量）
ffmpeg -i input.mp4 -c:v libx264 -b:v 1000k -pass 1 -f null /dev/null
ffmpeg -i input.mp4 -c:v libx264 -b:v 1000k -pass 2 -c:a aac -b:a 128k output.mp4
```

### 视频滤镜处理

```bash
# 添加水印
ffmpeg -i input.mp4 -i watermark.png -filter_complex "overlay=10:10" output.mp4

# 添加文字水印
ffmpeg -i input.mp4 -vf "drawtext=text='Hello World':fontcolor=white:fontsize=24:x=10:y=10" output.mp4

# 视频旋转（90度顺时针）
ffmpeg -i input.mp4 -vf "transpose=1" output.mp4

# 水平翻转
ffmpeg -i input.mp4 -vf "hflip" output.mp4

# 垂直翻转
ffmpeg -i input.mp4 -vf "vflip" output.mp4

# 调整亮度/对比度/饱和度
ffmpeg -i input.mp4 -vf "eq=brightness=0.1:contrast=1.2:saturation=1.1" output.mp4

# 去噪
ffmpeg -i input.mp4 -vf "hqdn3d" output.mp4

# 锐化
ffmpeg -i input.mp4 -vf "unsharp" output.mp4
```

### 音频处理

```bash
# 调整音量（2倍）
ffmpeg -i input.mp4 -af "volume=2.0" output.mp4

# 淡入淡出
ffmpeg -i input.mp4 -af "afade=t=in:ss=0:d=3,afade=t=out:st=57:d=3" output.mp4

# 音频降噪
ffmpeg -i input.mp4 -af "arnndn=m=sh.rnnn" output.mp4

# 改变播放速度（音频+视频）
ffmpeg -i input.mp4 -filter_complex "[0:v]setpts=0.5*PTS[v];[0:a]atempo=2.0[a]" -map "[v]" -map "[a]" output.mp4

# 仅改变视频速度（保持音频）
ffmpeg -i input.mp4 -filter:v "setpts=0.5*PTS" output.mp4
```

### 合并与分割

```bash
# 合并视频（使用 concat 协议）
ffmpeg -i "concat:file1.mp4|file2.mp4|file3.mp4" -c copy output.mp4

# 合并视频（使用 concat 过滤器，适用于不同编码）
ffmpeg -i file1.mp4 -i file2.mp4 -filter_complex "[0:v:0][0:a:0][1:v:0][1:a:0]concat=n=2:v=1:a=1[outv][outa]" -map "[outv]" -map "[outa]" output.mp4

# 使用文件列表合并
# 创建 filelist.txt:
# file 'video1.mp4'
# file 'video2.mp4'
ffmpeg -f concat -safe 0 -i filelist.txt -c copy output.mp4
```

### 截图与缩略图

```bash
# 截取单帧（第5秒）
ffmpeg -ss 00:00:05 -i input.mp4 -vframes 1 output.jpg

# 生成缩略图网格（4x4）
ffmpeg -i input.mp4 -vf "select='not(mod(n\,100))',scale=320:180,tile=4x4" -frames:v 1 output.jpg

# 生成视频预览（每10秒一帧）
ffmpeg -i input.mp4 -vf "fps=1/10,scale=480:-1" preview_%03d.jpg
```

### 流媒体与网络

```bash
# 从 RTMP 流拉取
ffmpeg -i rtmp://server/live/stream -c copy output.mp4

# 推送到 RTMP 服务器
ffmpeg -re -i input.mp4 -c copy -f flv rtmp://server/live/stream

# 生成 HLS
ffmpeg -i input.mp4 -codec: copy -start_number 0 -hls_time 10 -hls_list_size 0 -f hls index.m3u8

# 生成 DASH
ffmpeg -i input.mp4 -codec: copy -f dash manifest.mpd
```

### 字幕处理

```bash
# 烧录字幕到视频
ffmpeg -i input.mp4 -vf "subtitles=subtitle.srt" output.mp4

# 提取字幕
ffmpeg -i input.mkv -map 0:s:0 subtitle.srt

# 添加字幕流（不烧录）
ffmpeg -i input.mp4 -i subtitle.srt -c copy -c:s mov_text output.mp4
```

## 滤镜语法详解

### 简单滤镜 (-vf / -af)

```bash
# 单个滤镜
ffmpeg -i input.mp4 -vf "scale=1280:720" output.mp4

# 多个滤镜（逗号分隔）
ffmpeg -i input.mp4 -vf "scale=1280:720,fps=30,format=yuv420p" output.mp4
```

### 复杂滤镜 (-filter_complex)

用于多输入/多输出场景：

```bash
# 画中画
ffmpeg -i main.mp4 -i pip.mp4 -filter_complex "[0:v]scale=1280:720[main];[1:v]scale=320:180[pip];[main][pip]overlay=10:10" output.mp4

# 视频拼接（横向）
ffmpeg -i left.mp4 -i right.mp4 -filter_complex "[0:v][1:v]hstack" output.mp4

# 视频拼接（纵向）
ffmpeg -i top.mp4 -i bottom.mp4 -filter_complex "[0:v][1:v]vstack" output.mp4

# 多宫格（2x2）
ffmpeg -i tl.mp4 -i tr.mp4 -i bl.mp4 -i br.mp4 -filter_complex "[0:v][1:v]hstack[top];[2:v][3:v]hstack[bottom];[top][bottom]vstack" output.mp4
```

## 常用滤镜参考

### 视频滤镜

| 滤镜 | 功能 | 示例 |
|------|------|------|
| `scale` | 缩放 | `scale=1280:720` |
| `crop` | 裁剪 | `crop=640:480:100:100` (宽:高:x:y) |
| `fps` | 改变帧率 | `fps=30` |
| `transpose` | 旋转/翻转 | `transpose=1` (90度顺时针) |
| `hflip` | 水平翻转 | `hflip` |
| `vflip` | 垂直翻转 | `vflip` |
| `rotate` | 任意角度旋转 | `rotate=PI/6` |
| `eq` | 调整亮度/对比度/饱和度 | `eq=brightness=0.1:contrast=1.2` |
| `hue` | 调整色相 | `hue=h=30` |
| `blur` | 模糊 | `boxblur=2:1` |
| `sharpen` | 锐化 | `unsharp=5:5:1.0` |
| `noise` | 添加噪点 | `noise=alls=20:allf=t+u` |
| `deshake` | 防抖 | `deshake` |
| `stabilize` | 视频稳定 | `vidstabdetect` / `vidstabtransform` |
| `fade` | 淡入淡出 | `fade=t=in:st=0:d=3` |
| `delogo` | 去除水印 | `delogo=x=10:y=10:w=100:h=50` |
| `drawtext` | 添加文字 | `drawtext=text='Hello':x=10:y=10` |
| `overlay` | 叠加 | `overlay=x=10:y=10` |
| `colorkey` | 色度键控 | `colorkey=color=green:similarity=0.3` |
| `chromakey` | 高级色度键控 | `chromakey=color=0x00FF00:similarity=0.3` |
| `split` | 分流 | `split[main][pip]` |
| `format` | 格式转换 | `format=yuv420p` |
| `pad` | 填充 | `pad=1920:1080:(ow-iw)/2:(oh-ih)/2` |
| `setsar` | 设置采样宽高比 | `setsar=1:1` |
| `setdar` | 设置显示宽高比 | `setdar=16:9` |

### 音频滤镜

| 滤镜 | 功能 | 示例 |
|------|------|------|
| `volume` | 音量调整 | `volume=2.0` 或 `volume=6dB` |
| `afade` | 音频淡入淡出 | `afade=t=in:ss=0:d=3` |
| `atempo` | 速度调整（0.5-2.0） | `atempo=1.5` |
| `asetrate` | 采样率调整 | `asetrate=44100*1.5` |
| `aresample` | 重采样 | `aresample=48000` |
| `acompressor` | 压缩器 | `acompressor=threshold=-20dB:ratio=4` |
| `anormalize` | 音频归一化 | `anormalize` |
| `arnndn` | RNN降噪 | `arnndn=m=sh.rnnn` |
| `highpass` | 高通滤波 | `highpass=f=200` |
| `lowpass` | 低通滤波 | `lowpass=f=3000` |
| `equalizer` | 均衡器 | `equalizer=f=1000:width_type=h:width=200:g=-10` |
| `aecho` | 回声 | `aecho=0.8:0.9:1000|1800:0.3|0.25` |
| `amix` | 音频混合 | `amix=inputs=2` |
| `acrossfade` | 交叉淡入淡出 | `acrossfade=d=3` |

## 硬件加速

### NVIDIA NVENC (需要NVIDIA GPU)

```bash
# H.264 硬件编码
ffmpeg -i input.mp4 -c:v h264_nvenc -preset p4 -cq 23 output.mp4

# H.265 硬件编码
ffmpeg -i input.mp4 -c:v hevc_nvenc -preset p4 -cq 28 output.mp4
```

### Intel Quick Sync (需要Intel核显)

```bash
ffmpeg -i input.mp4 -c:v h264_qsv -preset medium -global_quality 23 output.mp4
```

### AMD VCE/VCN

```bash
ffmpeg -i input.mp4 -c:v h264_amf -quality balanced -qp_p 23 output.mp4
```

### Apple VideoToolbox (macOS/iOS)

```bash
ffmpeg -i input.mp4 -c:v h264_videotoolbox -b:v 5000k output.mp4
```

### VA-API (Linux)

```bash
ffmpeg -vaapi_device /dev/dri/renderD128 -i input.mp4 -vf 'format=nv12,hwupload' -c:v h264_vaapi output.mp4
```

## 实用技巧

### 获取媒体信息

```bash
# 基本信息
ffmpeg -i input.mp4

# 使用 ffprobe 获取详细信息
ffprobe -v quiet -print_format json -show_format -show_streams input.mp4

# 获取时长
ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 input.mp4

# 获取分辨率
ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=s=x:p=0 input.mp4
```

### 批量处理

```bash
# 批量转换（Bash）
for f in *.mp4; do ffmpeg -i "$f" -c:v libx264 -crf 23 "output/${f%.mp4}.mkv"; done

# 批量转换（PowerShell）
Get-ChildItem *.mp4 | ForEach-Object { ffmpeg -i $_.FullName -c:v libx264 -crf 23 ("output/" + $_.BaseName + ".mkv") }
```

### 处理损坏的文件

```bash
# 尝试修复损坏的视频
ffmpeg -err_detect ignore_err -i corrupted.mp4 -c copy fixed.mp4

# 忽略解码错误继续处理
ffmpeg -fflags +discardcorrupt -i input.mp4 -c copy output.mp4
```

### 创建测试视频

```bash
# 测试视频源
ffmpeg -f lavfi -i testsrc=duration=10:size=640x480:rate=30 -pix_fmt yuv420p test.mp4

# 彩条测试
ffmpeg -f lavfi -i smptebars=duration=10:size=640x480:rate=30 test.mp4

# 纯音频测试
ffmpeg -f lavfi -i sine=frequency=1000:duration=5 -c:a aac test.aac
```

## 最佳实践

1. **优先使用 copy**：如果只是封装格式转换，使用 `-c copy` 避免重新编码
2. **合理选择编码器**：H.264 兼容性最好，H.265/HEVC 压缩率更高，AV1 是下一代标准
3. **使用 CRF 控制质量**：比固定比特率更灵活，推荐值 18-28
4. **选择合适的 preset**：越慢质量越好但编码时间越长，默认 medium 是平衡选择
5. **注意像素格式**：确保使用广泛支持的格式如 yuv420p
6. **利用硬件加速**：有支持的 GPU 时优先使用硬件编码器
7. **2-pass 编码**：追求最佳质量时使用，特别是固定比特率场景

## 官方文档链接

- [FFmpeg 主文档](https://ffmpeg.org/ffmpeg.html)
- [FFmpeg 滤镜文档](https://ffmpeg.org/ffmpeg-filters.html)
- [FFmpeg 编解码器文档](https://ffmpeg.org/ffmpeg-codecs.html)
- [FFmpeg 格式文档](https://ffmpeg.org/ffmpeg-formats.html)
- [FFmpeg 协议文档](https://ffmpeg.org/ffmpeg-protocols.html)
- [FFmpeg 设备文档](https://ffmpeg.org/ffmpeg-devices.html)
- [FFmpeg 重采样文档](https://ffmpeg.org/ffmpeg-resampler.html)
- [FFmpeg 缩放文档](https://ffmpeg.org/ffmpeg-scaler.html)
