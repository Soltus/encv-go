# Tasks

## Phase 1: 核心类型与数据模型

- [x] Task 1: 定义 V4 Header 和 Footer 类型
  - [x] SubTask 1.1: 在 `internal/v2/types/` 中创建 `header_v4.go`，定义 `EnvelopeHeaderV4` 结构体（2048 字节，含 ContainerType、IsSeekable、ManifestOffset、ManifestLength）
  - [x] SubTask 1.2: 定义 `EnvelopeFooterV4` 结构体（12 字节，仅 Magic + GlobalCRC32 + Reserved）
  - [x] SubTask 1.3: 实现 `WriteHeaderV4`、`ReadHeaderV4`、`WriteFooterV4`、`ReadFooterV4` 函数
  - [x] SubTask 1.4: 更新 `DetectHeaderVersion` 支持 0x04 版本识别
  - [x] SubTask 1.5: 更新 `GetHeaderSize` 返回 v4 的 2048 字节
  - [x] SubTask 1.6: 定义 ContainerType 常量（0=unknown, 1=video, 2=audio, 3=image, 4=document, 5=text）

- [x] Task 2: 定义 V4 Segment 和 Manifest 类型
  - [x] SubTask 2.1: 在 `internal/v2/types/` 中创建 `segment_v4.go`，定义 `SegmentHeader`（SegmentID、DataLength、NonceSize、DataCRC32）
  - [x] SubTask 2.2: 定义 `Segment_v4` 结构体（id、offset、size、start_time、duration、nonce、keyframe_info）
  - [x] SubTask 2.3: 定义 `Manifest_v4` 结构体（version=4、container_id、container_type、is_seekable、segments、playlists、chapters、disaster_zones、kvi、edl_history）
  - [x] SubTask 2.4: 定义 `DisasterZone` 结构体（name、offset、size）
  - [x] SubTask 2.5: 定义 `PlaylistEntry` 和 `EDLEntry` 类型
  - [x] SubTask 2.6: 实现 `Manifest_v4` 的序列化/反序列化方法

- [x] Task 3: 实现 Manifest 混淆机制
  - [x] SubTask 3.1: 在 `internal/v2/crypto/` 中创建 `obfuscate.go`，实现 `ObfuscateManifest(data []byte) ([]byte, error)` — 生成 16B salt + XOR 混淆
  - [x] SubTask 3.2: 实现 `DeobfuscateManifest(data []byte) ([]byte, error)` — 提取 salt + XOR 还原
  - [x] SubTask 3.3: 编写单元测试验证混淆/还原的正确性

## Phase 2: 加密层改造

- [x] Task 4: Segment 独立加密支持
  - [x] SubTask 4.1: 在 `internal/v2/crypto/` 中创建 `segment_crypto.go`，实现 `EncryptSegment(data []byte, key []byte) (*SegmentEncryptionResult, error)` — 生成独立 nonce + AES-CTR 加密
  - [x] SubTask 4.2: 实现 `DecryptSegment(encryptedData []byte, nonce []byte, key []byte) ([]byte, error)` — 使用指定 nonce 解密
  - [x] SubTask 4.3: 实现 `EncryptStreamToSegments(src io.Reader, key []byte, segmentSize int64) ([]*SegmentEncryptionResult, error)` — 流式分片加密
  - [x] SubTask 4.4: 编写单元测试验证 Segment 独立加密/解密的正确性

## Phase 3: 插件接口扩展

- [x] Task 5: 扩展插件接口
  - [x] SubTask 5.1: 在 `Plugin` 接口中添加 `ContainerType() uint16` 方法
  - [x] SubTask 5.2: 添加 `DefaultIsSeekable(inputPath string) bool` 方法
  - [x] SubTask 5.3: 添加 `DisasterZones(inputPath string) []DisasterZone` 方法
  - [x] SubTask 5.4: 为所有现有插件（audio、image、wps、pdf、text）实现默认的 v4 接口方法
  - [x] SubTask 5.5: 更新 `FindDecryptingPlugin` 逻辑，优先使用 Header 的 ContainerType 路由，回退到 Manifest 解析

- [x] Task 6: 视频插件 v4 改造
  - [x] SubTask 6.1: 实现 `ContainerType()` 返回 `ContainerTypeVideo`
  - [x] SubTask 6.2: 实现 `DefaultIsSeekable()` — 根据输入文件格式判断（MP4 faststart=true, 其他=false）
  - [x] SubTask 6.3: 实现 `DisasterZones()` — 返回视频头区域
  - [x] SubTask 6.4: 重构 `Encrypt` 方法，使用 Segment 独立加密替代全局 IV 加密
  - [x] SubTask 6.5: 重构 `PostEncryptProcessor`，生成 v4 Manifest（segments + playlists）
  - [x] SubTask 6.6: 重构 `Decrypt` 方法，支持 v4 Segment 模型解密
  - [x] SubTask 6.7: 重构 `CanDecrypt` 方法，优先使用 Header ContainerType 判断
  - [x] SubTask 6.8: 更新 `VideoPluginConfig`，重命名 `chunk_size_mb` → `container_chunk_size_mb`，`light_main_chunk_enabled` → `light_container_main_chunk_enabled`
  - [x] SubTask 6.9: 新增 `AllowNoReencode` 和 `DefaultStreamPreset` 配置项
  - [x] SubTask 6.10: 实现"不重新编码"加密模式 — 直接按 Segment 切分加密原始视频数据

## Phase 4: 容器读写层改造

- [x] Task 7: V4 容器写入器
  - [x] SubTask 7.1: 在 `internal/v2/writer/` 中创建 `container_writer_v4.go`，实现 v4 容器写入逻辑
  - [x] SubTask 7.2: 写入流程：Header → Segments（各自独立 nonce）→ Manifest（混淆后）→ Footer
  - [x] SubTask 7.3: 支持空容器写入（零 Segment）
  - [x] SubTask 7.4: 支持容灾区域复制写入

- [x] Task 8: V4 容器读取器
  - [x] SubTask 8.1: 在 `internal/v2/reader/` 中创建 `segment_reader.go`，实现基于 Segment 模型的读取器
  - [x] SubTask 8.2: 实现 `SegmentSeekableReader` — 支持 io.ReaderAt 和 io.Seeker，按 Segment 独立解密
  - [x] SubTask 8.3: 实现 `SegmentSequentialReader` — 顺序拼接解密，用于 non-seekable 容器
  - [x] SubTask 8.4: 实现基于 Playlist 的虚拟编辑读取 — 根据 playlist 过滤 Segment
  - [x] SubTask 8.5: 实现 Seek 逻辑 — 通过 Manifest 的 segment 时间信息定位

- [x] Task 9: 容器检测与版本路由
  - [x] SubTask 9.1: 更新 `internal/v2/container/detector/` 支持 v4 版本检测
  - [x] SubTask 9.2: 实现版本路由：根据 Header Version 选择 v2/v3/v4 读取路径
  - [x] SubTask 9.3: v4 优先从 Header 获取 ContainerType，回退到 Manifest 解析

## Phase 5: 视频插件高级功能

- [x] Task 10: 画质预设系统
  - [x] SubTask 10.1: 在 `internal/v2/plugins/video/` 中创建 `preset.go`，定义 `StreamPreset` 类型
  - [x] SubTask 10.2: 实现移动端预设参数映射（balanced/quality/high_quality → MediaCodec 参数）
  - [x] SubTask 10.3: 实现桌面端预设参数映射（balanced/quality/high_quality → FFmpeg NVENC 参数）
  - [x] SubTask 10.4: 在加密流程中集成预设选择逻辑

- [x] Task 11: Chapter 通用化
  - [x] SubTask 11.1: 将 `MKVChapterInfo` 重命名为 `ChapterInfo`，移至 `internal/v2/types/`
  - [x] SubTask 11.2: 更新视频插件的元数据提取器，支持从 MP4 提取章节（使用 ffprobe）
  - [x] SubTask 11.3: 更新解密后处理器，支持将章节写回 MP4 格式

- [ ] Task 12: FFmpeg 桌面端源码构建
  - [ ] SubTask 12.1: 创建桌面端 FFmpeg 构建脚本（Linux/macOS/Windows），仅启用 h264/hevc 编解码 + mkv 封装
  - [ ] SubTask 12.2: 更新 `internal/utils/` 中的 FFmpeg 集成层，桌面端也使用 dlopen 方式调用
  - [ ] SubTask 12.3: 统一 Android 和桌面端的 FFmpeg 调用接口，由插件系统封装平台差异

## Phase 6: 配置与 Schema 更新

- [x] Task 13: 更新配置 Schema
  - [x] SubTask 13.1: 更新 `config.schema.json`，重命名 `chunk_size_mb` → `container_chunk_size_mb`
  - [x] SubTask 13.2: 重命名 `light_main_chunk_enabled` → `light_container_main_chunk_enabled`
  - [x] SubTask 13.3: 新增 `allow_no_reencode` 和 `default_stream_preset` 字段
  - [x] SubTask 13.4: 更新 `VideoPluginConfig` Go 结构体和默认值
  - [x] SubTask 13.5: 更新 `GetSettingFields()` 返回新的字段定义

## Phase 7: 集成测试与验证

- [x] Task 14: 端到端集成测试
  - [x] SubTask 14.1: 测试 v4 容器创建（单 Segment、多 Segment、空容器）
  - [x] SubTask 14.2: 测试 v4 容器读取和解密（seekable 和 non-seekable）
  - [x] SubTask 14.3: 测试 Segment 独立解密（删除中间 Segment 后其余仍可解密）
  - [x] SubTask 14.4: 测试 Manifest 混淆/还原
  - [x] SubTask 14.5: 测试虚拟编辑模式（修改 playlist）
  - [x] SubTask 14.6: 测试物理删除模式
  - [x] SubTask 14.7: 测试 v2/v3 容器的向后兼容读取
  - [x] SubTask 14.8: 测试容灾区域声明和恢复
  - [x] SubTask 14.9: 测试不重新编码加密模式
  - [x] SubTask 14.10: 测试画质预设对加密参数的影响

# Task Dependencies

- Task 2 depends on Task 1（Segment/Manifest 类型依赖 Header 类型定义）
- Task 3 depends on Task 2（Manifest 混淆依赖 Manifest 类型定义）
- Task 4 depends on Task 2（Segment 加密依赖 Segment 类型定义）
- Task 5 depends on Task 1（插件接口扩展依赖 ContainerType 常量定义）
- Task 6 depends on Task 4, Task 5（视频插件改造依赖 Segment 加密和插件接口）
- Task 7 depends on Task 3, Task 4（容器写入器依赖 Manifest 混淆和 Segment 加密）
- Task 8 depends on Task 7（容器读取器依赖容器写入器产出物）
- Task 9 depends on Task 1, Task 8（版本路由依赖 Header 检测和读取器）
- Task 10 depends on Task 6（画质预设依赖视频插件基础改造）
- Task 11 depends on Task 6（Chapter 通用化依赖视频插件基础改造）
- Task 12 depends on Task 5（FFmpeg 桌面端构建依赖插件接口统一）
- Task 13 depends on Task 6（配置更新依赖视频插件配置定义）
- Task 14 depends on all previous tasks（集成测试依赖所有功能完成）

# Parallelizable Work

- Task 1 + Task 2 可并行（类型定义互不依赖）
- Task 3 + Task 4 可并行（混淆和加密独立）
- Task 10 + Task 11 + Task 12 可并行（视频插件高级功能互不依赖）
