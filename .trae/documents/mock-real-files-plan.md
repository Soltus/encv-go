# Mock 真实文件生成脚本 Plan

## Why

当前 Mock 文件系统（`mock/file-system.ts`）只有**元数据**（name/size/path），没有磁盘上的真实二进制文件。后端 `go run` 扫描 `server_dir` 时找不到文件，导致：
- 文件列表为空
- 预览/下载 404
- ENC-FN 端到端无法验证

需要**一次性生成脚本**，在独立的数据目录中创建真实测试文件。

## 核心约束

1. **不污染项目代码路径** — 输出到 `<project_root>/__mock_data__/` 目录（`.gitignore` 友好）
2. **目录结构清晰分类** — 按类型分层：普通文件 / alist-encrypt 加密 / ENCV 容器 / 边界测试

## 输出目录结构

```
__mock_data__/                          # 项目根下的独立数据目录（不入 git）
├── README.md                            # 说明文件：来源、用途、清理方式
│
├── 01-plain-media/                      # ============ 普通媒体文件（未加密）============
│   ├── video/
│   │   ├── sample.mp4                  # 最小有效 fMP4 (~3KB)
│   │   └── comedy.mkv                  # 最小有效 WebM/MKV (~2KB)
│   ├── audio/
│   │   ├── music.mp3                   # 有效 MP3 + ID3v2 标签 (~15KB, 3s静音)
│   │   └── podcast.flac                # 有效 FLAC 流 (~10KB, 2s静音)
│   ├── image/
│   │   ├── photo.jpg                   # 最小有效 JPEG 1x1 红色 (~300B)
│   │   └── screenshot.png              # 最小有效 PNG 4x4 渐变 (~200B)
│   └── document/
│       ├── report.pdf                  # 最小有效 PDF 含文字 (~1KB)
│       ├── notes.txt                   # 多语言 UTF-8 文本 (EN/CN/JP/emoji)
│       └── data.csv                    # 有效 CSV (header+10行数据)
│
├── 02-alist-encrypt/                   # ============ alist-encrypt 加密文件 =========
│   ├── secret.ae                       # 模拟 AE 文件 (magic + 随机填充)
│   ├── document.ae                     # 同上，不同大小
│   └── hidden-gem.ae                   # Movies 子目录的 AE 文件也放这里（逻辑分组）
│
├── 03-encv-containers/                 # ============ ENCV 容器文件（各插件格式）==========
│   ├── video.sccgv                     # 视频容器模拟 (SCCV4 magic + 填充)
│   ├── image.sccgi                     # 图片容器模拟
│   ├── audio.sccga                     # 音频容器模拟
│   ├── archive.sccgpdf                 # PDF 容器模拟
│   └── track.sccga                     # Music 子目录的音频容器也放这里
│
├── 04-boundary-test/                   # ============ 边界情况专用测试 ================
│   ├── long-filename/
│   │   └── 这是一个非常长的中文文件名用于测试ENC-FN编码功能包含emoji🎉和生僻汉字龘靁齉爨麤.txt
│   ├── empty-name.txt                  # 空文件名测试
│   ├── spaces-only.txt                # 纯空白字符文件
│   ├── special-chars/
│   │   ├── control-chars.bin          # 含 \x00 \n \t 的二进制文件
│   │   ├── unicode-mix.txt            # RTL阿拉伯文 + CJK混合 + emoji
│   │   └── dotfiles/.hidden           # 以 . 开头的隐藏文件
│   └── size-extremes/
│       ├── tiny.txt                    # 1 字节
│       ├── small.txt                   # 100 字节
│       ├── medium.txt                  # 10 KB
│       └── large.bin                   # 接近文件系统限制大小 (200MB 可选生成)
```

## 设计原则

| 原则 | 说明 |
|------|------|
| **数字前缀排序** | `01-` `02-` `03-` `04-` 保证目录按类别有序排列 |
| **同类聚合** | 所有 plain media 在一个根下，所有加密在另一个根下 |
| **逻辑 vs 物理分离** | Movies/Documents/Music 是 UI 逻辑分组；物理文件按**格式类型**分组（因为同一格式的处理逻辑相同） |
| **边界独立** | 04-boundary-test 完全隔离，不影响正常测试流程 |
| **幂等安全** | 多次运行覆盖已有文件，不删除未知文件 |

## What Changes

### 新增文件
- `scripts/generate-mock-files.ts` — 主脚本（纯 Node.js，零依赖）
- `__mock_data__/README.md` — 数据目录说明
- `.gitignore` 追加 `__mock_data__/` 条目（如尚未忽略）

### 修改文件
- `package.json` — 新增 `"generate:mock": "tsx scripts/generate-mock-files.ts"` script
- `mock/handlers.ts` — `/api/files` handler 可选：支持从 `__mock_data__/` 读取真实文件 size/mtime 而非硬编码

---

# Tasks

## Task 1: 创建 `scripts/generate-mock-files.ts`

### 1.1 CLI 参数解析
```bash
# 默认输出到 <project_root>/__mock_data__/
tsx scripts/generate-mock-files.ts

# 自定义输出目录
tsx scripts/generate-mock-files.ts --dir /tmp/encv-mock-data

# 仅生成某类（可选）
tsx scripts/generate-mock-files.ts --type plain      # 仅 01-plain-media
tsx scripts/generate-mock-files.ts --type ae         # 仅 02-alist-encrypt
tsx scripts/generate-mock-files.ts --type container  # 仅 03-encv-containers
tsx scripts/generate-mock-files.ts --type boundary   # 仅 04-boundary-test
tsx scripts/generate-mock-files.ts --type all        # 全部（默认）
```

### 1.2 工具函数层
```typescript
const MOCK_ROOT = path.resolve(process.cwd(), '__mock_data__')

function ensureDir(dir: string): void
function writeBuffer(filePath: string, data: Uint8Array): void
function writeString(filePath: string, content: string, encoding?: BufferEncoding): void
function randomBytes(n: number): Uint8Array
function padToSize(data: Uint8Array, targetSize: number): Uint8Array
function humanSize(bytes: number): string
```

### 1.3 各类型最小有效文件生成器

#### JPEG 生成器 → `photo.jpg`
手工构造最小 JFIF:
- `FF D8` (SOI)
- `FF E0 00 10 4A 46 49 46 00 ...` (APP0 JFID 标记)
- `FF DB 00 43 00 ...` (DQT, 标准量化表)
- `FF C0 00 0B 08 00 01 00 01 01 01 11 00` (SOF0, 1x1, 1 component)
- `FF C4 00 1F 00 ...` (DHT, DC+AC Huffman 表)
- `FF DA 00 08 01 01 00 00 3F 00 ...` (SOS, 1 scan)
- 一个 MCU (8x8 DCT block 全零) + `FF D9` (EOI)

#### PNG 生成器 → `screenshot.png`
- PNG signature (`89 50 4E 47 ...`)
- IHDR chunk (4x4 RGBA, 8bit)
- IDAT chunk (zlib deflate of raw pixel data) — 使用 Node.js `zlib.deflateSync()`
- IEND chunk

#### MP4 生成器 → `sample.mp4`
- ftyp box (`isom/iso2/mp41`)
- moov box:
  - mvhd (timescale=1000, duration=1000)
  - trak → mdia → minf → stbl:
    - stsd (avc1/mp4a)
    - stts (1 sample @ dts=0, duration=1024)
    - stsc (1 entry)
    - stsz (1 sample, size=N)
    - stco (offset=mdat_start)
- mdat box (AAC frame data: ADTS header + silent frame)

#### MKV/WebM 生成器 → `comedy.mkv`
- EBML Header (EBML version=1, read type=webm)
- Segment:
  - Info (TimecodeScale=1000000, Duration=3000000000 ≈ 3s)
  - Tracks (TrackEntry: VP9 video, CodecPrivate=VP9 codec config)
  - Cluster (Timecode=0, SimpleBlock: VP9 keyframe ~1KB)

#### MP3 生成器 → `music.mp3`
- ID3v2.3 tag (TIT2="Mock Music", TPE1="ENCV Test")
- MPEG1 Layer III frames:
  - Header: 0xFFFB (MPEG1 Layer3 128kbps 44100Hz stereo)
  - 每帧 418 bytes payload (silence), 共约 108 帧 × 3秒
  - 总大小 ~45KB

#### FLAC 生成器 → `podcast.flac`
- `fLaC` signature
- STREAMINFO block (min blocksize=4096, max=4096, channels=2, bps=16, rate=44100, total samples=88200)
- PADDING block
- FRAME(s): fixed-size subframes (all zeros = silence), ~2 seconds worth

#### PDF 生成器 → `report.pdf`
```
%PDF-1.4
1 0 obj << /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
  /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj << /Length ... >> stream
BT /F1 12 Tf 100 700 Td (Hello from ENCV Mock!) Tj ET
endstream
endobj
5 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
trailer << /Size 6 /Root 1 0 R >>
%%EOF
```

#### TXT 生成器 → `notes.txt`
多行 UTF-8 内容：
```
=== ENCV Mock Test File ===
English: Hello, World! 🌍
中文: 你好世界！这是测试文件。
日本語: こんにちは、テストファイルです。
한국어: 안녕하세요 테스트 파일입니다.
العربية: مرحبا بالعالم
Emoji: 🎬🎵📷📄🔐💾🔒
Special: \tTab\nNewline\0Null
Line 10: End of test.
```

#### CSV 生成器 → `data.csv`
标准 CSV，含 header + 10 行数据，中英混合字段。

#### alist-encrypt 模拟文件 (.ae)
自定义二进制格式：
```
[4 bytes] Magic: "AENC" (0x41 45 4E 43)
[2 bytes] Version: 0x0001
[2 bytes] Flags: 0x0000
[N bytes] Random padding (to match declared size)
```

#### ENCV v4 容器模拟文件 (.sccgv/.sccgi/.sccga/.sccgpdf)
自定义二进制格式：
```
[4 bytes] Magic: "SCCV" (0x53 43 43 56)
[1 byte]  Version: 0x04
[2 bytes] Flags LE (bit4 = FilenameEncrypted for some files)
[4 bytes] ManifestOffset LE
[4 bytes] ManifestLength LE
[Manifest JSON] {"version":4,"container_id":"mock-xxx",...}
[Padding] to match declared size
```

#### 超长文件名文件
使用 `fs.writeFileSync()` 直接写入，文件名即为测试用超长 Unicode 串。

### 1.4 主函数入口
```typescript
async function main() {
  const args = parseArgs()
  const root = args.dir || MOCK_ROOT
  
  console.log(`📦 ENCV Mock File Generator`)
  console.log(`📂 Output: ${root}`)
  
  if (args.type === 'all' || !args.type || args.type.includes('plain')) {
    await generatePlainMedia(root)
  }
  if (args.type === 'all' || !args.type || args.type.includes('ae')) {
    await generateAEFiles(root)
  }
  // ... 其他类别
  
  console.log('\n✅ Done! Summary:')
  printSummary()
}
```

## Task 2: 更新 package.json 和 .gitignore

```json
{
  "scripts": {
    "generate:mock": "tsx scripts/generate-mock-files.ts"
  }
}
```

`.gitignore` 新增:
```
__mock_data__/
```

## Task 3: 运行验证

```bash
cd app/encv-mobile
npm run generate:mock

# 验证
ls -laR __mock_data__/
file __mock_data__/01-plain-media/video/sample.mp4
file __mock_data__/01-plain-media/image/photo.jpg
file __mock_data__/04-boundary-test/long-filename/*.txt

# 启动后端确认能扫到
# 后端 server_dir 配置指向 __mock_data__ 或建立软链接
```
