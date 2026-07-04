# Tasks

## Phase 1: CipherMode 体系（AES-128-CTR 默认 + AES-256-CTR 可选）

- [x] Task 1: 在 v4 crypto 层引入 CipherMode 枚举
  - [x] SubTask 1.1: 在 `internal/v2/crypto/aes_v2.go` 新增常量 `CipherModeAES128CTR = 0`、`CipherModeAES256CTR = 1`
  - [x] SubTask 1.2: 新增 `KeySize_v4_128 = 16`、`KeySize_v4_256 = 32` 常量
  - [x] SubTask 1.3: 改写 `GenerateKey_v4(password, salt, keyLen)` 支持 16/32 字节输出（PBKDF2-SHA256, 100000 iter）
  - [x] SubTask 1.4: `EncryptStream_v2` / `DecryptStream_v2` 接受变长 key（已支持 16/24/32 字节，验证 16 字节路径走通）
  - [x] SubTask 1.5: 新增单元测试 `TestGenerateKey_VariableLength` 覆盖 16/32 字节

- [x] Task 2: v4 Header 增加 CipherMode 字段
  - [x] SubTask 2.1: 在 `internal/v2/types/header_v4.go` 的 `EnvelopeHeaderV4` 结构体增加 `CipherMode uint16`（复用 `Reserved2` 前 2 字节，offset 2040-2042；不破坏已分配字段偏移）
  - [x] SubTask 2.2: 修改 `WriteHeaderV4` / `ReadHeaderV4` 序列化/反序列化 `CipherMode`
  - [x] SubTask 2.3: 旧 v4 容器（无 CipherMode 字段，Reserved2 零值）按 0 解析（AES-128-CTR），保证向后兼容不报错
  - [x] SubTask 2.4: 与 `plugin-version-selection-and-password-detection` 的 `PasswordHint`（偏移 20-36）共存不冲突（不同偏移）
  - [x] SubTask 2.5: 单元测试 `TestHeaderV4_CipherMode_RoundTrip`

## Phase 2: HMAC-SHA1-80 实现 + Encrypt-then-MAC

- [ ] Task 3: 实现 HMAC-SHA1-80 crypto 原语
  - [x] SubTask 3.1: 在 `internal/v2/crypto/` 创建 `mac.go`，实现 `HMACSHA1_80(key, message []byte) [10]byte`
  - [x] SubTask 3.2: 实现 `VerifyHMACSHA1_80(expected, message []byte, key []byte) bool`（常量时间比较）
  - [x] SubTask 3.3: 引入 `ErrMACMismatch` 错误类型
  - [x] SubTask 3.4: 单元测试 `TestHMACSHA1_80_KnownVector`（用 RFC 2202 / WinZip AES 规范向量）

- [x] Task 4: 重构 Segment 加密为 Encrypt-then-MAC
  - [x] SubTask 4.1: 改写 `internal/v2/crypto/segment_crypto.go` 的 `EncryptSegment`：
    - 接收 `macKey` 参数
    - 加密后追加 `HMAC-SHA1-80(macKey, nonce || ciphertext)`
  - [x] SubTask 4.2: 改写 `DecryptSegment`：
    - 接收 `macKey` 参数
    - **先验 MAC**，验失败立即返回 `ErrMACMismatch`
    - 验通过再解 CTR
  - [x] SubTask 4.3: `EncryptStreamToSegments` 接受 `macKey` 参数
  - [x] SubTask 4.4: 单元测试 `TestEncryptDecryptSegment_WithMAC`
  - [x] SubTask 4.5: 负向测试 `TestDecryptSegment_TamperedCiphertext_ReturnsErrMACMismatch`（翻转 1 bit 必失败）
  - [x] SubTask 4.6: 负向测试 `TestDecryptSegment_WrongMACKey_ReturnsErrMACMismatch`（错误密钥必失败）

- [x] Task 5: mac_key 派生与 Header 存储
  - [x] SubTask 5.1: 在 `internal/v2/crypto/` 创建 `mac_key.go`，实现 `DeriveMACKey(password, macSalt []byte) []byte`（PBKDF2-SHA256, 100000, 32 bytes）
  - [x] SubTask 5.2: ~~在 v4 Header 偏移 36-52 增加 `MacSalt [16]byte]`~~ **实现变更为**：将 MacSalt 改存于 `Manifest_v4.MACSaltBase64`（v4 Header 偏移 36-2028 被 SpecialID 完全占据，物理上无法再插入 16 字节；改存 Manifest 保持向后兼容）。详见 `internal/v2/types/segment_v4.go` 字段注释和 `internal/v2/types/header_v4.go` 包级注释。
  - [x] SubTask 5.3: `WriteV4Container` 自动生成随机 `MacSalt` 并注入 `Manifest.MACSaltBase64`（base64 编码，16 字节）
  - [x] SubTask 5.4: `OpenV4Container` 从 `Manifest.MACSaltBase64` 提取 `MacSalt` 供 `DeriveMACKey` 使用，存入 `V4ContainerInfo.MacSalt` 字段；旧 v4 容器（无 mac_salt 字段）通过 `types.HasMACSalt()` 识别并 fallback
  - [x] SubTask 5.5: 单元测试 `TestHeaderV4_MacSalt_RoundTrip` / `TestHeaderV4_MacSalt_BackwardCompat_AllZeros` / `TestHeaderV4_MacSalt_DistinctFromPasswordHint` / `TestHeaderV4_MacSalt_LengthIs16` / `TestHasMACSalt` / `TestManifestV4_MacSaltBase64_OmitEmpty`

## Phase 3: SegmentHeader 扩展（ModeFlags + 压缩字段）

- [x] Task 6: 扩展 SegmentHeader 结构
  - [x] SubTask 6.1: 在 `internal/v2/types/segment_v4.go` 修改 `SegmentHeader`：
    ```go
    type SegmentHeader struct {
        SegmentID           uint32
        DataLength          uint64
        NonceSize           uint16
        ModeFlags           uint16  // 新增：bit0=Encrypted, bit1=Compression
        MACSize             uint16  // 新增：默认 10
        DataCRC32           uint32
        CompressedBlockSize uint16  // 新增：zstd seekable 块大小
        Reserved            uint16
        SeekTableOffset     uint32  // 新增：zstd 时有效
        SeekTableLength     uint32  // 新增
    }
    ```
  - [x] SubTask 6.2: `SegmentHeaderSize` 从 18 扩展到 34
  - [x] SubTask 6.3: `MarshalBinary` / `UnmarshalBinary` 处理新字段
  - [x] SubTask 6.4: 单元测试 `TestSegmentHeader_Extended_RoundTrip`（及相关 BinarySize/BytesLayout/ModeFlags/DefaultValues/AllFieldsSet/ShortBuffer_Rejected/ExtraBytes_Accepted/BackwardCompat_OldSize18_Rejected/InterfaceCompliance 等共 9 个测试）
  - [x] SubTask 6.5: 定义 `ModeFlag*` 常量（`ModeFlagEncrypted = 1 << 0`，`ModeFlagCompressionZstd = 1 << 1`）及便利组合（`ModeFlagsPlaintext`/`ModeFlagsEncryptedNoCompression`/`ModeFlagsEncryptedZstd`）

## Phase 4: zstd 压缩 + seekable 集成

- [x] Task 7: 引入 zstd-seekable 依赖
  - [x] SubTask 7.1: `go.mod` 添加 zstd-seekable 库（**注**：spec 中 `github.com/saracen/go-zstdseekable` 不存在；实际为 `github.com/SaveTheRbtz/zstd-seekable-format-go/pkg`，即 zstd-seekable-format 官方 Go 绑定，已添加 v0.10.0）
  - [x] SubTask 7.2: `go mod tidy` 验证依赖解析
  - [x] SubTask 7.3: 简单冒烟测试 `TestZstdSeekable_BasicRoundTrip`（`internal/v2/crypto/compression/zstd_smoke_test.go`）

- [x] Task 8: 实现压缩模块
  - [x] SubTask 8.1: 在 `internal/v2/crypto/compression/` 创建 `zstd.go`
  - [x] SubTask 8.2: 实现 `CompressZstdSeekable(src io.Reader) (compressed []byte, seekTable []byte, err error)`
  - [x] SubTask 8.3: 实现 `DecompressZstdSeekable(compressed, seekTable []byte) (plaintext []byte, err error)`
  - [x] SubTask 8.4: 块大小配置项 `zstd_block_size`（默认 64KB）— 实现为 `CompressZstdSeekableWithBlockSize`，常量 `DefaultZstdBlockSize = 64 * 1024`
  - [x] SubTask 8.5: 单元测试 `TestZstdSeekable_LargeFile_CompressDecompress`（以及 BasicRoundTrip / EmptyInput / BinaryRandomData / SeekTable_NonEmpty / SingleFrame / BlockSizeOverride / RandomReadAt）

- [x] Task 9: Segment 集成压缩
  - [x] SubTask 9.1: 在 `internal/v2/crypto/segment_crypto.go` 改写 `EncryptSegment`：
    - 接受 `compressionMode string` 参数
    - `compressionMode == "zstd"` 时先压缩再加密
  - [x] SubTask 9.2: `EncryptSegment` 设置 `ModeFlags.CompressionZstd` 标记
  - [x] SubTask 9.3: `DecryptSegment` 根据 `ModeFlags` 决定是否解压
  - [x] SubTask 9.4: <1KB 数据自动跳过压缩，记 `ModeFlags.Compression = none`
  - [x] SubTask 9.5: 单元测试 `TestEncryptDecryptSegment_ZstdCompressed`
  - [x] SubTask 9.6: 单元测试 `TestEncryptDecryptSegment_MixedModes`（一个 Segment 压缩、一个不压缩）

## Phase 5: detector 边界测试套件（验证现有能力，不改 detector 行为）

> **澄清**：detector 当前已基于魔数 `ENCV` 识别（`IsEncvContainerFromBytes`），不依赖任何文件扩展名（`.sccg*`、`.bin`、空扩展名等均不参与检测）。本任务仅为现有能力补齐测试。

- [ ] Task 10: 在 `internal/v2/container/detector/detector_test.go` 补充边界测试
  - [x] SubTask 10.1: `TestDetect_StrippedSuffix_Plain`（`mydocument` 无扩展名，验证 `IsEncvContainerFromBytes` 仍能识别）
  - [x] SubTask 10.2: `TestDetect_StrippedSuffix_Dotfile`（`.sccgv` 隐藏文件——隐藏文件也算 dotfile）
  - [x] SubTask 10.3: `TestDetect_StrippedSuffix_WrongExtension`（`mydocument.zip` 应识别为非 ENCV）
  - [x] SubTask 10.4: `TestDetect_StrippedSuffix_Boundary_Magic`（恰好 6 字节 "ENCV"+2 字节 version）
  - [x] SubTask 10.5: `TestDetect_StrippedSuffix_Boundary_HeaderMinus1`（2047 字节，差 1 字节完整 Header）
  - [x] SubTask 10.6: `TestDetect_StrippedSuffix_TruncatedAt5Bytes`（5 字节，"ENCV" + 1 字节，< 6 字节最小要求）
  - [x] SubTask 10.7: `TestDetect_StrippedSuffix_NonENCVMagic`（"PK\x03\x04" ZIP 头应返回 `IsEncvContainer=false`）
  - [x] SubTask 10.8: `TestDetect_StrippedSuffix_EmptyFile`（0 字节返回明确错误）
  - [x] SubTask 10.9: `TestDetect_StrippedSuffix_ValidV4_HeaderRead`（完整 v4 容器无后缀可读）
  - [ ] SubTask 10.10: `TestDetect_StrippedSuffix_CipherMode_0` 与 `TestDetect_StrippedSuffix_CipherMode_1`（待 Phase 1 完成后追加）

## Phase 6: writer/reader 集成新能力

- [x] Task 11: container_writer_v4 集成新能力
  - [x] SubTask 11.1: 在 `internal/v2/writer/container_writer_v4.go` 改写写入流程：
    - 接受 `CipherMode` / `CompressionMode` / `EnableHMAC` 参数
    - 按 Phase 1+2+4 写入 Header → Segments（每 Segment 独立 nonce+mac_key+可选压缩）→ Manifest → Footer
  - [x] SubTask 11.2: 写入不加密 Segment（ModeFlags.Encrypted=0）时跳过 mac_key 派生
  - [x] SubTask 11.3: 集成测试 `TestWriterV4_AES128_WithMAC_WithZstd`
  - [x] SubTask 11.4: 集成测试 `TestWriterV4_AES256_WithMAC_NoCompression`
  - [x] SubTask 11.5: 集成测试 `TestWriterV4_MixedSegments_EncryptedAndPlain`

- [x] Task 12: segment_reader 集成 MAC 校验前置
  - [x] SubTask 12.1: 在 `internal/v2/reader/segment_reader.go` 改写解密流程：
    - 接受 `macKey` 参数
    - **强制先验 MAC**，验失败立即 `ErrMACMismatch`
  - [x] SubTask 12.2: 处理 `ModeFlags.Encrypted=0` 的明文 Segment（不验 MAC）
  - [x] SubTask 12.3: 处理 `ModeFlags.Compression=zstd` 的解压路径
  - [x] SubTask 12.4: 集成测试 `TestReaderV4_DetectTamperedMAC_ReturnsErrMACMismatch`
  - [x] SubTask 12.5: 集成测试 `TestReaderV4_DecompressZstd_OnTheFly`

- [x] Task 13: 配置文件 schema 更新
  - [x] SubTask 13.1: 在 `config.schema.json` 增加 `v4_cipher_mode` (integer enum [0,1], default 0)
  - [x] SubTask 13.2: 增加 `v4_compression_mode` (string enum ["none", "zstd"], default "none")
  - [x] SubTask 13.3: 增加 `v4_enable_hmac` (bool, default true)
  - [x] SubTask 13.4: 增加 `v4_zstd_block_size` (integer, default 65536)

- [x] Task 14: 前端 UI 适配
  - [x] SubTask 14.1: 在 `app/encv-mobile/src/components/EncryptBody.vue`（即等价于 spec 中的 `EncryptDialog.vue`，项目实际为 `NewTaskModal` 的子组件）新增 "加密强度" 选择（128/256）—— ion-radio-group 模式，沿用 `ContainerVersionSelector` 风格
  - [x] SubTask 14.2: 新增 "压缩" 选择（无 / zstd）—— ion-radio-group 模式
  - [x] SubTask 14.3: 默认值：128 + 无压缩（`cipherMode: 0`、`compressionMode: 'none'`），在 `NewTaskState.ts`、`useNewTaskModal.ts` 初始 reactive 状态、`NewTaskModal.vue` 的 `fallbackState` 中均设置
  - [x] SubTask 14.4: 选 256 时显示提示 "更慢，强度更高"（`tasks.cipherMode256Help` i18n key）
  - [x] SubTask 14.5: 选 zstd 时显示提示 "纯文本/重复二进制可节省 30-70% 空间"（`tasks.compressionZstdHelp` i18n key）

> **实施说明**：spec 中提到的 `EncryptDialog.vue` 在项目中并不存在。项目的等价组件是 `EncryptBody.vue`（作为 `NewTaskModal.vue` 的子组件，在 `taskType === 'encrypt'` 时渲染）。状态由 `useNewTaskModal.ts` 中的 `state` reactive 对象统一管理，并通过 `modalController.create()` 静态快照模式注入到子组件。提交时 `onSubmit` 把 `cipherMode` 和 `compressionMode` 传给 `api/encv.ts` 的 `createTask`，后端 API body 字段名为 `cipherMode`（number）和 `compressionMode`（'none' | 'zstd'）。

## Task Dependencies

- [Task 3] depends on [Task 1] (HMAC 测试需 CipherMode 体系)
- [Task 4] depends on [Task 3] (Segment 加密需 MAC 原语)
- [Task 6] depends on [Task 4] (SegmentHeader 扩展需 MAC 设计落地)
- [Task 8] depends on [Task 7] (压缩模块需 zstd 依赖)
- [Task 9] depends on [Task 6, 8] (Segment 集成需 Header 扩展 + 压缩模块)
- [Task 11] depends on [Task 1, 4, 6, 9] (writer 集成全部前置)
- [Task 12] depends on [Task 11] (reader 镜像 writer)
- [Task 14] depends on [Task 13] (前端需 schema 落地)

## Parallelization

- Task 1, 3, 7 可并行启动（独立模块）
- Task 10（detector 边界测试）可与 Task 2/5 并行（独立子系统）
- Task 14（前端 UI）可与 Task 11/12 并行（独立代码库）
