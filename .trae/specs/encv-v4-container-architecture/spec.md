# ENCV v4 容器架构 Spec

## Why

当前 v3 容器架构存在以下核心问题：
1. **容器类型识别依赖 Manifest**：必须解析清单才能知道容器交给哪个插件处理，无法在 Header 阶段快速路由。
2. **加密粒度太粗**：整个数据流共享一个 IV/nonce，物理删除中间段后必须重加密后续所有数据。
3. **Manifest 明文可读**：清单以明文 JSON 存储，暴露了文件结构、分片信息等敏感元数据。
4. **视频插件耦合了平台差异**：视频插件直接处理 FFmpeg 调用细节，桌面端和移动端逻辑混杂。
5. **不支持空容器**：无法创建只有清单没有数据的容器，限制了"先建容器后追加数据"的工作流。
6. **不支持非重编码视频加密**：所有视频加密默认 seekable，无法区分原始封装和重编码后的容器。

v4 架构通过引入 Segment 独立加密、Header 自描述、Manifest 混淆、插件容灾区域声明等机制，从根本上解决这些问题。

## What Changes

- **BREAKING**：容器版本号升至 `0x04`，v4 Header 布局与 v3 不兼容
- **BREAKING**：Manifest 结构从 `fragments` 模型迁移到 `segments` + `playlists` 模型
- **BREAKING**：每个 Segment 拥有独立 nonce，counter 从 0 开始
- **BREAKING**：配置项 `chunk_size_mb` 重命名为 `container_chunk_size_mb`
- **BREAKING**：配置项 `light_main_chunk_enabled` 重命名为 `light_container_main_chunk_enabled`
- 新增 Header 字段：`ContainerType`（容器类型标识）和 `IsSeekable`（布尔标志）
- 新增空容器支持（零 Segment）
- 新增插件容灾区域声明接口 `DisasterZones()`
- 新增 Manifest 混淆机制（XOR + 随机填充）
- 新增 Segment 独立加密模型
- 新增 Playlist 概念，支持虚拟编辑
- 新增 Chapter 通用化（不再限于 MKV）
- 新增视频画质预设（StreamPreset）
- FFmpeg 桌面端从源码构建精简版，由插件系统统一封装调用
- 视频插件支持"不重新编码"加密模式（v4 容器）

## Impact

- Affected specs: 容器格式规范、插件接口规范、配置 schema
- Affected code:
  - `internal/v2/types/` — Header、Footer、Manifest、Fragment 类型定义
  - `internal/v2/crypto/` — 加密/解密逻辑（需支持 per-segment nonce）
  - `internal/v2/container/` — 容器读写、检测
  - `internal/v2/plugins/` — 插件接口（新增 DisasterZones、ContainerType）
  - `internal/v2/plugins/video/` — 视频插件全面重构
  - `internal/v2/reader/` — 读取器需支持 Segment 模型
  - `internal/v2/writer/` — 写入器需支持 Segment 模式
  - `internal/v2/physical/` — 物理打包逻辑
  - `internal/v2/service/` — 容器管理服务
  - `internal/v2/provider/` — 文件提供者
  - `internal/utils/` — FFmpeg 集成层
  - `config.schema.json` — 配置 schema 更新
  - `app/encv-mobile/scripts/build-ffmpeg-android.sh` — FFmpeg 构建脚本
  - 前端 — 需适配 seekable 标识、画质预设选择

---

## ADDED Requirements

### Requirement: V4 Header 自描述容器类型

系统 SHALL 在容器 Header 中嵌入 `ContainerType`（uint16）和 `IsSeekable`（bool）字段，使得不解析 Manifest 即可确定容器类型和是否可随机访问。

#### Scenario: 读取器快速路由
- **WHEN** 读取器打开一个 v4 容器文件
- **THEN** 读取 Header 的 `ContainerType` 字段后，即可确定该容器应交给哪个插件处理
- **AND** 读取 `IsSeekable` 字段后，即可决定使用 Seekable 还是 Sequential 读取策略

#### Scenario: 向后兼容
- **WHEN** 读取器打开一个 v2/v3 容器文件
- **THEN** 通过 Version 字段识别旧版本，回退到 Manifest 解析方式确定容器类型

### Requirement: V4 Header 结构

V4 Header SHALL 采用以下布局（固定大小）：

```
Offset  Size  Field               Description
0       4     Magic               "ENVC"
4       2     Version             0x04
6       2     Flags               位标志（沿用 v3 的 FlagIsMainContainer/FlagIsPhysicalChunk）
8       2     ContainerType       容器类型标识（0=unknown, 1=video, 2=audio, 3=image, 4=document, 5=text, ...）
10      1     IsSeekable          0x01=可随机访问, 0x00=顺序访问
11      1     Reserved
12      4     IDType              ID 编码类型（沿用 v3）
16      4     IDLength            ID 有效长度（沿用 v3）
20      8     Reserved            预留扩展
28      2000  SpecialID           特殊 ID 存储（沿用 v3）
2028    4     ManifestOffset      Manifest 在文件中的字节偏移（v4 新增，从 Header 移入）
2032    4     ManifestLength      Manifest 数据长度（v4 新增）
2036    4     HeaderCRC32         头部 CRC32 校验
2040    8     Reserved            填充至 2048 字节
Total: 2048 bytes
```

关键变化：
- `ContainerType` 和 `IsSeekable` 是 v4 新增字段
- `ManifestOffset` 和 `ManifestLength` 从 Footer 移入 Header，实现"只读 Header 即可定位 Manifest"
- Footer 简化，仅保留全局校验信息

### Requirement: V4 Footer 结构

V4 Footer SHALL 简化为：

```
Offset  Size  Field           Description
0       4     Magic           "ENVC"
4       4     GlobalCRC32     全局 CRC32 校验
8       4     Reserved        预留
Total: 12 bytes
```

### Requirement: Segment 独立加密模型

系统 SHALL 支持将数据分割为多个 Segment，每个 Segment 拥有独立的 nonce，AES-CTR counter 从 0 开始。

#### Scenario: Segment 独立解密
- **WHEN** 一个 v4 容器中的 Segment N 被物理删除
- **THEN** 其他所有 Segment 仍可独立解密，无需重加密
- **AND** 解密时每个 Segment 使用各自的 nonce，counter 从 0 开始

#### Scenario: Segment 布局
- **WHEN** 写入一个包含多个 Segment 的 v4 容器
- **THEN** 每个 Segment 在容器中的布局为：`[SegmentHeader][Nonce(16B)][EncryptedData]`
- **AND** SegmentHeader 包含：`SegmentID(uint32) | DataLength(uint64) | NonceSize(uint16) | DataCRC32(uint32)`

### Requirement: Manifest V4 结构

V4 Manifest SHALL 采用以下 JSON 结构：

```json
{
  "version": 4,
  "container_id": "unique-container-id",
  "container_type": "video",
  "is_seekable": true,
  "original_duration": 7250.8,
  "segments": [
    {
      "id": "seg-000001",
      "offset": 123456,
      "size": 536870912,
      "start_time": 0.0,
      "duration": 612.4,
      "nonce": "base64-encoded-nonce",
      "keyframe_info": []
    }
  ],
  "playlists": {
    "default": ["seg-000001", "seg-000002", "seg-000004"],
    "clean": ["seg-000001", "seg-000005"]
  },
  "chapters": [
    {"time": 0, "title": "开场"}
  ],
  "disaster_zones": [
    {"name": "video_header", "offset": 2048, "size": 4096}
  ],
  "kvi": { ... },
  "edl_history": []
}
```

关键设计：
- `segments` 替代 v3 的 `fragments`，每个 segment 有独立 nonce
- `playlists` 支持虚拟编辑（只改 playlist，不动数据）
- `chapters` 通用化，不限于 MKV
- `disaster_zones` 声明容灾区域（如视频头），用于损坏恢复
- `container_type` 和 `is_seekable` 与 Header 冗余存储，提供双重保障

### Requirement: Manifest 混淆

系统 SHALL 对 V4 Manifest 进行混淆处理，破坏可读性。

#### Scenario: 写入时混淆
- **WHEN** 写入 Manifest 到容器
- **THEN** Manifest JSON 先序列化为字节，再使用系统密钥进行 XOR 混淆
- **AND** 混淆后的数据前附加 16 字节随机 salt（用于派生 XOR 密钥流）
- **AND** 存储格式为：`[Salt(16B)][XOR-Obfuscated-Manifest-Bytes]`

#### Scenario: 读取时还原
- **WHEN** 从容器读取 Manifest
- **THEN** 读取前 16 字节 salt，派生 XOR 密钥流，还原原始 JSON
- **AND** 还原后进行 JSON 解析和 CRC32 校验

注意：混淆不是加密，目的是破坏明文可读性，防止工具直接 strings 看到清单内容。安全性仍由数据层的 AES-256-CTR 保证。

### Requirement: 空容器支持

系统 SHALL 支持创建没有数据 Segment 的空容器。

#### Scenario: 创建空容器
- **WHEN** 用户请求创建一个空容器
- **THEN** 容器只包含 Header + Manifest + Footer，segments 列表为空
- **AND** 后续可通过追加操作向容器添加 Segment

### Requirement: 插件容灾区域声明

系统 SHALL 允许插件声明额外的容灾区域。

#### Scenario: 视频插件声明容灾区域
- **WHEN** 视频插件加密一个视频文件
- **THEN** 插件通过 `DisasterZones()` 方法返回容灾区域列表
- **AND** 容灾区域信息写入 Manifest 的 `disaster_zones` 字段
- **AND** 容灾区域在容器中被额外复制存储，用于损坏恢复

#### Scenario: 容灾区域接口
- **WHEN** 插件实现 `DisasterZones()` 方法
- **THEN** 返回 `[]DisasterZone`，每个 DisasterZone 包含 `Name string`、`Offset int64`、`Size int64`

### Requirement: 三种编辑模式

系统 SHALL 支持三种容器编辑模式。

#### Scenario: 虚拟模式（最快）
- **WHEN** 用户选择虚拟编辑模式
- **THEN** 只修改 Manifest 中的 playlist，不修改任何 Segment 数据
- **AND** 播放时根据当前 playlist 动态跳过被排除的 Segment

#### Scenario: 物理删除模式
- **WHEN** 用户选择物理删除模式
- **THEN** 直接在容器内删除对应 Segment，释放空间
- **AND** 更新 Manifest 中的 segments 列表和 playlist
- **AND** 不影响其他 Segment 的解密

#### Scenario: 精细剪辑模式
- **WHEN** 用户选择精细剪辑模式
- **THEN** 对受影响的 Segment 做解密 → I-frame 对齐切割 → 重建 → 重新加密成新 Segment
- **AND** 新 Segment 拥有独立 nonce

### Requirement: 流式播放与 Seek

系统 SHALL 支持基于 Segment 模型的流式播放和 Seek。

#### Scenario: HTTP 流式播放
- **WHEN** HTTP 服务收到播放请求
- **THEN** 根据当前 playlist 动态拼接 segments 的解密数据
- **AND** 每个 Segment 独立解密，使用各自的 nonce

#### Scenario: Seek 定位
- **WHEN** 播放器请求 Seek 到某个时间点
- **THEN** 通过 Manifest 中的 segment 时间信息快速定位到对应 Segment
- **AND** 在 Segment 内部通过 keyframe_info 进一步精确定位

### Requirement: 配置项重命名

系统 SHALL 将以下配置项重命名：

| 旧名称                      | 新名称                              | 原因                                     |
| --------------------------- | ------------------------------------ | ---------------------------------------- |
| `chunk_size_mb`             | `container_chunk_size_mb`            | 避免与加密数据的分片（Segment）混淆       |
| `light_main_chunk_enabled`  | `light_container_main_chunk_enabled` | 语义更精确，表示容器级别的轻量主分片      |

#### Scenario: light_container_main_chunk_enabled 语义
- **WHEN** `container_chunk_size_mb` 有效且 `light_container_main_chunk_enabled` 为 true
- **THEN** 主分片只分离加密数据 Segments，容器尾部的其他数据（Manifest、Recovery 等）保留在 main chunk
- **WHEN** `light_container_main_chunk_enabled` 为 false
- **THEN** 主分片包含完整容器数据（Header + Segments + Manifest + Footer）

### Requirement: 视频插件不重新编码模式

系统 SHALL 支持不重新编码视频直接加密（v4 容器）。

#### Scenario: 不重新编码加密
- **WHEN** 用户选择不重新编码模式加密视频
- **THEN** 视频数据直接按 Segment 切分加密，不经过 FFmpeg 重编码
- **AND** `IsSeekable` 标志取决于原始视频格式（MP4 faststart=seekable, 其他可能 non-seekable）
- **AND** 前端正确处理 non-seekable 容器（不支持拖拽进度条）

### Requirement: 视频画质预设

系统 SHALL 支持前端选择视频画质预设。

#### Scenario: 画质预设定义
- **WHEN** 前端发起加密请求并指定画质预设
- **THEN** 系统根据预设参数配置编码器
- **AND** 预设包括：`balanced`（平衡）、`quality`（高质量）、`high_quality`（极致画质）

#### Scenario: 移动端预设参数
- **WHEN** 在 Android 移动端使用 MediaCodec 编码
- **THEN** 各预设对应以下参数：

| 预设         | quality | bitrateMode | keyFrameInterval | lowLatency | profile | 描述                   |
| ------------ | ------- | ----------- | ---------------- | ---------- | ------- | ---------------------- |
| balanced     | 28      | VBR         | 2s               | true       | main    | 画质较好、体积适中     |
| quality      | 24      | VBR         | 2s               | true       | high    | 画质更好，体积稍大     |
| high_quality | 20      | VBR         | 3s               | false      | high    | 画质最佳，体积控制好   |

#### Scenario: 桌面端预设参数
- **WHEN** 在桌面端使用 FFmpeg + NVENC 编码
- **THEN** 各预设映射到对应的 FFmpeg NVENC 参数（实现细节由视频插件内部处理）

### Requirement: FFmpeg 桌面端源码构建

系统 SHALL 在桌面端也从源码构建精简版 FFmpeg，由插件系统统一封装调用。

#### Scenario: 桌面端 FFmpeg 构建
- **WHEN** 桌面端构建项目
- **THEN** 从源码构建精简版 FFmpeg（仅包含 h264/hevc 编解码 + mkv 封装所需组件）
- **AND** 构建产物为共享库（.so/.dylib/.dll），通过 dlopen 加载

#### Scenario: 插件系统封装平台差异
- **WHEN** 视频插件需要调用 FFmpeg 功能
- **THEN** 通过统一的插件接口调用，不直接关心底层是 dlopen 还是命令行
- **AND** Android 端使用 MediaCodec 编码 h264，FFmpeg 仅负责 mkv 封装
- **AND** 桌面端使用 FFmpeg + NVENC 编码和封装

### Requirement: Chapter 通用化

系统 SHALL 将章节（Chapter）支持从 MKV 专属扩展为通用功能。

#### Scenario: MP4 章节支持
- **WHEN** 加密一个包含章节的 MP4 文件
- **THEN** 章节信息被提取并存储在 Manifest 的 `chapters` 字段
- **AND** 解密时章节信息被正确还原到目标格式

### Requirement: 插件接口扩展

系统 SHALL 扩展插件接口以支持 v4 新特性。

#### Scenario: 新增接口方法
- **WHEN** 插件实现 v4 接口
- **THEN** 需要实现以下新增方法：
  - `ContainerType() uint16` — 返回插件对应的容器类型标识
  - `DefaultIsSeekable(inputPath string) bool` — 返回默认的 seekable 状态
  - `DisasterZones(inputPath string) []DisasterZone` — 返回容灾区域列表
  - `StreamPresets() []StreamPreset` — 返回支持的画质预设（可选）

---

## MODIFIED Requirements

### Requirement: 插件接口（Plugin Interface）

v3 插件接口扩展为 v4：

```go
type Plugin interface {
    // === v3 保留 ===
    Name() string
    GetDefaultSettings() json.RawMessage
    GetSettingsSchemaType() interface{}
    GetContainerExtension() string
    GetSettingFields() []pluginInterfaces.SettingField
    Initialize(ctx context.Context) error
    GetMetadataExtractor() pluginInterfaces.MetadataExtractor
    GetContentPreprocessor() pluginInterfaces.ContentPreprocessor
    GetContentVirifier() pluginInterfaces.ContentVerifier
    GetChunkNamer() namer.ChunkNamer
    SupportedMimePrefixes() []string
    SupportedExtensions() []string
    ShouldProcess(inputPath string) bool
    GroupFiles(inputPaths []string, inputRootDir, outputDir string) ([]string, error)
    PreEncryptProcessor(index types.Index, inputPath, inputRootDir, outputDir string) error
    Encrypt(dataReader io.Reader) (*crypto.EncryptionResult, error)
    PostEncryptProcessor(result *crypto.EncryptionResult) error
    CanDecrypt(containerPath string) bool
    PreDecryptProcessor(containerPath, outputDir string) error
    Decrypt(containerPath, outputDir string) error
    PostDecryptProcessor(containerPath string) error

    // === v4 新增 ===
    ContainerType() uint16
    DefaultIsSeekable(inputPath string) bool
    DisasterZones(inputPath string) []DisasterZone
}
```

### Requirement: 容器版本识别

v4 容器版本识别逻辑更新：
- `Version == 0x04`：v4 容器，使用 `EnvelopeHeaderV4`（2048 字节），从 Header 获取 ContainerType 和 ManifestOffset
- `Version == 0x03`：v3 容器，回退到 v3 解析逻辑
- `Version == 0x02`：v2 容器，回退到 v2 解析逻辑

### Requirement: 视频插件配置

VideoPluginConfig 更新：

```go
type VideoPluginConfig struct {
    Ext                              string `json:"ext"`
    ContainerChunkSizeMB             int    `json:"container_chunk_size_mb"`
    LightContainerMainChunkEnabled   bool   `json:"light_container_main_chunk_enabled"`
    TrackExtensions                  string `json:"track_extensions"`
    KeepMkvForMkvSource             bool   `json:"keep_mkv_for_mkv_source"`
    VerifyAfterPack                  bool   `json:"verify_after_pack"`
    PluginCacheDir                   string `json:"plugin_cache_dir"`
    SkipMergeForSplitMKV             bool   `json:"skip_merge_for_split_mkv"`
    // v4 新增
    AllowNoReencode                  bool   `json:"allow_no_reencode"`
    DefaultStreamPreset              string `json:"default_stream_preset"`
}
```

---

## REMOVED Requirements

### Requirement: v3 Fragment 模型
**Reason**: v4 使用 Segment 模型替代 Fragment 模型，每个 Segment 有独立 nonce，支持物理删除不影响其他段。
**Migration**: v3 容器仍可读取（通过版本号回退），但新创建的容器使用 v4 Segment 模型。读取器需同时支持两种模型。

### Requirement: v3 Footer 中的 ManifestOffset/ManifestLength
**Reason**: v4 将 ManifestOffset 和 ManifestLength 移入 Header，实现"只读 Header 即可定位 Manifest"，减少一次 I/O。
**Migration**: v3 容器仍从 Footer 读取这些字段，v4 容器从 Header 读取。
