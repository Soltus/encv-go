# V4 容器架构 Checklist

## 核心类型与数据模型

- [x] V4 Header 结构体定义正确，包含 ContainerType、IsSeekable、ManifestOffset、ManifestLength 字段
- [x] V4 Footer 结构体定义正确，仅包含 Magic + GlobalCRC32 + Reserved
- [x] V4 Header 读写函数正确实现，CRC32 校验通过
- [x] V4 Footer 读写函数正确实现
- [x] DetectHeaderVersion 正确识别 0x04 版本
- [x] GetHeaderSize 对 v4 返回 2048
- [x] ContainerType 常量定义完整（video=1, audio=2, image=3, document=4, text=5）
- [x] SegmentHeader 结构体定义正确（SegmentID、DataLength、NonceSize、DataCRC32）
- [x] Segment_v4 结构体定义正确（id、offset、size、start_time、duration、nonce、keyframe_info）
- [x] Manifest_v4 结构体定义正确（version=4、container_id、container_type、is_seekable、segments、playlists、chapters、disaster_zones、kvi、edl_history）
- [x] DisasterZone 结构体定义正确（name、offset、size）
- [x] Manifest_v4 序列化/反序列化方法正确实现

## Manifest 混淆

- [x] ObfuscateManifest 正确生成 16B salt + XOR 混淆数据
- [x] DeobfuscateManifest 正确提取 salt + XOR 还原原始 JSON
- [x] 混淆后的数据无法通过 strings 命令直接读取清单内容
- [x] 混淆/还原单元测试通过

## Segment 独立加密

- [x] EncryptSegment 为每个 Segment 生成独立 nonce
- [x] DecryptSegment 使用指定 nonce 正确解密
- [x] 删除任意 Segment 后，其余 Segment 仍可独立解密
- [x] AES-CTR counter 从 0 开始（每个 Segment 独立计数）
- [x] EncryptStreamToSegments 正确按大小分片加密
- [x] Segment 加密/解密单元测试通过

## 插件接口扩展

- [x] Plugin 接口新增 ContainerType() uint16 方法
- [x] Plugin 接口新增 DefaultIsSeekable(inputPath string) bool 方法
- [x] Plugin 接口新增 DisasterZones(inputPath string) []DisasterZone 方法
- [x] 所有现有插件（audio、image、wps、pdf、text）实现了默认的 v4 接口方法
- [x] FindDecryptingPlugin 优先使用 Header ContainerType 路由

## 视频插件 v4 改造

- [x] VideoPlugin.ContainerType() 返回 ContainerTypeVideo
- [x] VideoPlugin.DefaultIsSeekable() 根据文件格式正确判断
- [x] VideoPlugin.DisasterZones() 返回视频头区域
- [x] VideoPlugin.Encrypt 使用 Segment 独立加密
- [x] VideoPlugin.PostEncryptProcessor 生成 v4 Manifest（segments + playlists）
- [x] VideoPlugin.Decrypt 支持 v4 Segment 模型解密
- [x] VideoPlugin.CanDecrypt 优先使用 Header ContainerType 判断
- [x] VideoPluginConfig 中 chunk_size_mb 已重命名为 container_chunk_size_mb
- [x] VideoPluginConfig 中 light_main_chunk_enabled 已重命名为 light_container_main_chunk_enabled
- [x] VideoPluginConfig 新增 AllowNoReencode 配置项
- [x] VideoPluginConfig 新增 DefaultStreamPreset 配置项
- [x] "不重新编码"加密模式正确实现，IsSeekable 根据原始格式设置

## 容器读写层

- [x] V4 容器写入器正确实现（Header → Segments → Manifest → Footer）
- [x] 空容器写入正确（零 Segment）
- [x] 容灾区域复制写入正确
- [x] SegmentSeekableReader 正确实现 io.ReaderAt 和 io.Seeker
- [x] SegmentSequentialReader 正确实现顺序拼接解密
- [x] 基于 Playlist 的虚拟编辑读取正确
- [x] Seek 逻辑通过 Manifest segment 时间信息正确定位
- [x] 版本路由正确：v4 从 Header 获取 ContainerType，v2/v3 回退到 Manifest 解析

## 视频插件高级功能

- [x] StreamPreset 类型定义正确（balanced/quality/high_quality）
- [x] 移动端预设参数映射正确（MediaCodec 参数）
- [x] 桌面端预设参数映射正确（FFmpeg NVENC 参数）
- [x] MKVChapterInfo 已重命名为 ChapterInfo 并移至 types 包
- [x] 支持从 MP4 提取章节
- [x] 支持将章节写回 MP4 格式

## FFmpeg 桌面端构建

- [ ] 桌面端 FFmpeg 构建脚本正确（仅 h264/hevc + mkv 封装）
- [ ] 桌面端使用 dlopen 方式调用 FFmpeg
- [ ] Android 和桌面端 FFmpeg 调用接口统一

## 配置与 Schema

- [x] config.schema.json 中 chunk_size_mb 已重命名为 container_chunk_size_mb
- [x] config.schema.json 中 light_main_chunk_enabled 已重命名为 light_container_main_chunk_enabled
- [x] config.schema.json 新增 allow_no_reencode 字段
- [x] config.schema.json 新增 default_stream_preset 字段
- [x] VideoPluginConfig Go 结构体与 schema 一致
- [x] GetSettingFields() 返回更新后的字段定义

## 向后兼容

- [x] v2 容器仍可正确读取和解密
- [x] v3 容器仍可正确读取和解密
- [x] 旧版配置文件（使用 chunk_size_mb）能被兼容处理或给出明确迁移提示

## 集成测试

- [x] v4 单 Segment 容器创建和读取测试通过
- [x] v4 多 Segment 容器创建和读取测试通过
- [x] v4 空容器创建测试通过
- [x] Segment 独立解密测试通过（删除中间 Segment）
- [x] Manifest 混淆/还原测试通过
- [x] 虚拟编辑模式测试通过
- [x] 物理删除模式测试通过
- [x] 容灾区域声明和恢复测试通过
- [x] 不重新编码加密模式测试通过
- [x] 画质预设对加密参数的影响测试通过
