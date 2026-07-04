# ENCV v4 容器能力升级 Spec

## Why

在 v4 容器基础架构（`encv-v4-container-architecture`）之上，存在三个独立但相互关联的能力缺口。

### 项目实际结构（事实核查结果）

| 维度 | 实际状态 | 来源 |
|------|---------|------|
| 容器魔数 | `ENCV`（4 字节，bytes `0x45 0x4E 0x43 0x56`） | `internal/v2/types/container.go:66-67` `MagicHeader_v2`/`MagicFooter_v2` |
| 版本检测 | 读前 6 字节 → `DetectHeaderVersion` → v2/v3/v4 | `internal/v2/container/handle/handle.go:55-67` |
| **检测与扩展名关系** | **完全无关**，仅基于魔数 | `IsEncvContainerFromBytes` |
| 插件输出扩展名 | video=`.sccgv`、audio=`.sccga`、image=`.sccgi`、pdf=`.sccgpdf`、text=`.sccgt`、wps=`.sccgwps`、alistencrypt=`.bin` | `internal/v2/plugins/*/plugin.go: GetContainerExtension()` |
| v2 legacy 保留扩展名 | `.sccgv` 等 v2 历史扩展名被 `errors.go:9` 列为保留字，**禁止用户/插件使用**（避免与 v2 legacy 冲突） | `internal/v2/plugins/alistencrypt/errors.go:9` |

**澄清**：本项目 detector 100% 基于魔数 `ENCV` 识别容器，与文件扩展名（包括 `.sccgv`、`.sccga`、`.bin`、`.zip`、空扩展名等）完全解耦。"去掉后缀名边界测试"实际指"无扩展名/任意扩展名时，detector 必须返回相同结果"。

### 三项能力缺口

1. **加密算法单一**：v4 当前默认强制 AES-256-CTR（32 字节密钥），对绝大多数单机文件加密场景来说强度过剩且 2 倍于 128 位的吞吐开销。WinZip/RAR/7-Zip 等行业惯例都将 AES-128-CTR 列为默认，AES-256 仅作为可选加强档。
2. **缺少 zstd 压缩支持**：v4 容器内 Segment 数据是密文，密文通常不可压缩。当原始文件高度可压缩时（如日志/纯文本文档/重复二进制），**加密前预压缩**能节省 30-70% 空间。`zstd-seekable-format-go` 提供 seekable zstd frame 索引，正好契合 v4 Segment 模型。
3. **缺少 HMAC 完整性保护**：v4 现有 `DataCRC32` 是**未加密 CRC**，攻击者篡改密文后重算 CRC 可绕过校验。WinZip 早在 2009 年就在 AES 加密中引入 HMAC-SHA1-80（Encrypt-then-MAC 顺序），专门防御 CTR 模式比特翻转攻击。v4 必须补齐这一安全缺口。

## What Changes

- **BREAKING（可选，向后兼容）**：v4 Header 增加 `CipherMode` 字段（0=AES-128-CTR, 1=AES-256-CTR），旧 v4 容器按 0 解析
- v4 默认 `CipherMode = 0`（AES-128-CTR），通过配置项 `v4_cipher_mode` 可切到 1（AES-256-CTR）
- v4 KDF 支持变长密钥派生（PBKDF2-SHA256 输出 16 或 32 字节）
- v4 容器引入 `CompressionMode`（none / zstd），用户级可配，默认 none
- 集成 `github.com/saracen/go-zstdseekable`（zstd-seekable-format-go 的 Go 绑定）
- Segment 内部布局从 `[Header][Nonce][EncryptedData]` 升级为 `[Header][Nonce][EncryptedData][HMAC-SHA1-80(10B)]`
- 新增 `Encrypt-then-MAC` 计算顺序：先加密，再对"Nonce+密文"计算 HMAC-SHA1-80，截断到 10 字节
- 解密时强制 MAC 校验：**先验 MAC → 验失败立即 `ErrMACMismatch` → 验通过才解 CTR**
- v4 Segment Header 增加 `ModeFlags` 位字段，支持每 Segment 独立声明：是否压缩 / 是否加密 / 压缩算法（仿 WinZip "mixed" 模式）
- 配置文件 schema 增加 `v4_cipher_mode` 和 `v4_compression_mode` 两个配置项
- 新增魔数 detector 边界测试套件（不修改 detector 行为，仅补齐测试）

## Impact

- Affected specs:
  - `encv-v4-container-architecture`（v4 基础架构）
  - `plugin-version-selection-and-password-detection`（密码错误感知，复用 HMAC 体系）
- Affected code:
  - `internal/v2/crypto/aes_v2.go` — 增加变长 KDF
  - `internal/v2/crypto/segment_crypto.go` — 改用 `KeySize` 参数化；集成 MAC
  - `internal/v2/crypto/mac.go`（新增）— HMAC-SHA1-80 实现
  - `internal/v2/crypto/compression/zstd.go`（新增）— zstd 压缩/解压 + seekable 帧
  - `internal/v2/types/segment_v4.go` — `ModeFlags` 字段
  - `internal/v2/types/header_v4.go` — `CipherMode` 字段、`MacSalt` 字段
  - `internal/v2/container/detector/detector_test.go`（补充）— 魔数识别边界测试
  - `internal/v2/writer/container_writer_v4.go` — 写入 MAC + 可选压缩
  - `internal/v2/reader/segment_reader.go` — MAC 校验前置
  - `internal/v2/physical/file_chunker.go` — 物理切分时处理 zstd frame
  - `config.schema.json` — 两个新配置项
  - 前端 `EncryptDialog.vue` — 新增 cipher mode + compression mode 选择 UI
  - `go.mod` — 新增 `github.com/saracen/go-zstdseekable`

---

## ADDED Requirements

### Requirement: v4 CipherMode 配置化

系统 SHALL 在 v4 容器中支持 AES-128-CTR（默认）和 AES-256-CTR（可选）两种加密算法，通过 Header 的 `CipherMode` 字段标识。

#### Scenario: 默认加密算法变更
- **WHEN** 创建新 v4 容器
- **THEN** 默认使用 AES-128-CTR（`CipherMode=0`），密钥长度 16 字节
- **AND** 配置项 `v4_cipher_mode` 可显式设为 1 切到 AES-256-CTR

#### Scenario: 老 v4 容器解析
- **WHEN** 读取器打开一个 CipherMode 字段不存在的旧 v4 容器
- **THEN** 按 `CipherMode=0`（AES-128-CTR）解析
- **AND** 不报错，向后兼容

#### Scenario: 密钥长度变化
- **WHEN** CipherMode=0
- **THEN** PBKDF2 派生 16 字节密钥（`KeySize_v4_128 = 16`）
- **WHEN** CipherMode=1
- **THEN** PBKDF2 派生 32 字节密钥（`KeySize_v4_256 = 32`）
- **AND** 派生参数（salt、迭代次数）保持不变

#### Scenario: 算法迁移兼容
- **WHEN** 用户已加密的容器使用旧默认（AES-256）
- **THEN** 读取器根据 Header 的 `CipherMode` 字段自动选择密钥长度
- **AND** 用户无需手动选择算法

### Requirement: detector 魔数识别边界测试套件

`internal/v2/container/detector/` 包 SHALL 提供覆盖**魔数识别**（不依赖任何文件扩展名）边界场景的完整测试用例。**注意：detector 当前已基于魔数 `ENCV` 识别，文件扩展名（包括 `.sccgv`、`.sccga`、`.bin`、`.zip`、空扩展名等）从不参与检测**。本任务仅为现有能力补齐测试，不修改 detector 行为。

#### Scenario: 任意扩展名均能识别
- **WHEN** 一个 v4 容器文件被命名为 `mydocument`（无扩展名）/ `mydocument.bin` / `mydocument.sccgv` / `mydocument.zip` / `mydocument.exe`
- **THEN** `IsEncvContainerFromBytes` 仅根据文件内容判断，对所有上述命名都返回一致的 `IsEncvContainer` 结果
- **AND** 表明 detector 与文件扩展名完全解耦

#### Scenario: 魔数误判防护
- **WHEN** 一个非 ENCV 文件（如普通 ZIP 头 `PK\x03\x04`、PNG 头 `\x89PNG`、MP4 头 `ftyp`）被传入 detector
- **THEN** detector 返回 `IsEncvContainer = false`
- **AND** 不抛异常，返回明确的 "not an ENCV container" 错误

#### Scenario: 截断 Header 识别
- **WHEN** 传入的字节长度 < Header 完整大小（2048 字节）但 ≥ 魔数长度（6 字节，`DetectHeaderVersion` 最小要求）
- **THEN** detector 返回 "header truncated" 错误
- **AND** 不尝试后续字段解析

#### Scenario: 边界值测试
- **WHEN** 文件长度 = 6 字节（恰好能 DetectHeaderVersion）
- **THEN** detector 返回 v4 容器标识但 `ContainerType`/`IsSeekable` 等 Header 派生字段为 `unknown`/零值
- **WHEN** 文件长度 = 5 字节（差 1 字节 `DetectHeaderVersion` 最小要求）
- **THEN** detector 返回 `IsEncvContainer = false`（不抛异常）
- **WHEN** 文件长度 = 0 字节（空文件）
- **THEN** detector 返回明确错误 "empty input"

### Requirement: zstd 压缩支持（用户可配）

系统 SHALL 在 v4 容器中支持 zstd 压缩模式，与 AES-CTR 加密组合使用，压缩率与加密强度可独立配置。

#### Scenario: 压缩模式配置
- **WHEN** 配置文件 `v4_compression_mode` 设为 `zstd`
- **THEN** 创建新 v4 容器时，每个 Segment 在加密**前**先 zstd 压缩明文
- **AND** 写入布局：`[Header][Nonce][EncryptedData][HMAC-SHA1-80]`，其中 `EncryptedData` 是 zstd 压缩后的密文
- **WHEN** `v4_compression_mode` 设为 `none`（默认）
- **THEN** 跳过压缩步骤，密文即明文的 AES-CTR 输出

#### Scenario: 压缩 + 加密顺序固定
- **WHEN** 同时启用 zstd 和 AES-CTR
- **THEN** 数据流顺序为：`plaintext → zstd_compress → aes_ctr_encrypt → hmac_sha1_80`
- **AND** 解密时反向：`hmac_verify → aes_ctr_decrypt → zstd_decompress`
- **AND** HMAC 校验失败的 Segment 绝不解压（防 zstd 解压炸弹攻击）

#### Scenario: Seekable zstd frame 索引
- **WHEN** Segment 数据使用 zstd 压缩
- **THEN** 使用 `github.com/saracen/go-zstdseekable` 的 seekable 格式
- **AND** Segment 内部可细粒度 seek 到任意压缩块
- **AND** Seek 索引作为 Segment 的一部分存储在 SegmentHeader 之后

#### Scenario: 单 Segment 不压缩边界
- **WHEN** 原始 Segment 数据 < 1KB（压缩无收益）
- **THEN** 自动跳过压缩，记 `ModeFlags.compression = none`
- **AND** 不强制压缩所有 Segment（仿 WinZip "mixed" 模式）

#### Scenario: 压缩失败降级
- **WHEN** zstd 压缩过程返回错误（如压缩器 OOM）
- **THEN** 记录警告日志，降级为 `compression = none` 写入
- **AND** 不阻断整体加密流程

### Requirement: 加密容器的 "Mixed" 模式（仿 WinZip）

系统 SHALL 允许 v4 容器内不同 Segment 独立选择是否加密、是否压缩，使用 SegmentHeader 的 `ModeFlags` 位字段标识。WinZip AES 规范明确说明："not all files in a Zip file need to be encrypted, nor is it required that all encrypted files use the same encryption method or password"。

#### Scenario: Segment 级 ModeFlags
- **WHEN** 写入 v4 Segment
- **THEN** SegmentHeader 新增 `ModeFlags uint16` 字段，按位定义：
  - bit 0: `Encrypted`（1=加密, 0=明文）
  - bit 1: `Compression`（00=none, 01=zstd）
  - bit 2-15: 预留
- **AND** 默认所有 Segment `Encrypted=1, Compression=00`

#### Scenario: 混合内容容器
- **WHEN** 容器包含 3 个 Segment：A（明文 zip）+ B（加密不压缩 txt）+ C（加密 + zstd 压缩 log）
- **THEN** 三个 Segment 各自标记正确的 `ModeFlags`
- **AND** 解密时按各自 `ModeFlags` 决定处理路径
- **AND** 读取器根据 `ModeFlags.Encrypted` 决定是否需要 MAC 校验

#### Scenario: 不强制统一密码
- **WHEN** 同一容器内不同 Segment 来自不同密码源
- **THEN** Manifest 增加 `SegmentKeyRefs` 字段，引用外部 key store
- **AND** 默认情况下，所有 Segment 共享同一密码（兼容性）
- **AND** SegmentKeyRefs 为空数组时，回退到 Container 级单一密码

### Requirement: HMAC-SHA1-80 完整性保护（Encrypt-then-MAC）

系统 SHALL 在 v4 Segment 中嵌入 HMAC-SHA1-80 截断值，用于检测密文被篡改/比特翻转。WinZip 规范明确指出："HMAC-SHA1-80 is used because it is a mature and widely respected authentication algorithm"，并规定 MAC 在 "压缩/加密后" 计算（Encrypt-then-MAC 顺序），在 "解密/解压前" 校验。

#### Scenario: MAC 计算顺序（Encrypt-then-MAC）
- **WHEN** 加密 v4 Segment
- **THEN** 计算顺序固定为：
  1. 压缩（如启用）plaintext → compressed
  2. AES-CTR 加密 compressed → ciphertext
  3. 计算 `MAC = HMAC-SHA1(mac_key, nonce || ciphertext)[:10]`
  4. 写入 `[SegmentHeader][Nonce(16B)][Ciphertext][MAC(10B)]`
- **AND** `mac_key` 由主密钥 PBKDF2 派生（独立 salt）

#### Scenario: MAC 校验前置（解密前）
- **WHEN** 解密 v4 Segment
- **THEN** 严格顺序（WinZip 规范："MAC is checked before decryption/decompression"）：
  1. 读取 `SegmentHeader` 取 `ModeFlags` / `DataLength`
  2. 读取 `Nonce(16B)` 和 `Ciphertext`
  3. 计算 `expected_mac = HMAC-SHA1(mac_key, nonce || ciphertext)[:10]`
  4. **与存储的 MAC 常量时间比较**（防侧信道）
  5. 验失败 → 返回 `ErrMACMismatch`（不进行任何 CTR / 解压）
  6. 验通过 → 执行 AES-CTR 解密
- **AND** CRC32 校验可作为可选第二道防线，但不替代 MAC

#### Scenario: 错误密码立即识别
- **WHEN** 用户用错误密码解密 v4 容器
- **THEN** 第一个 Segment 的 MAC 校验立即失败
- **AND** 返回 `ErrWrongPassword` 错误（与 `plugin-version-selection-and-password-detection` spec 中的 PasswordHint 机制协同）
- **AND** 不会产生 "解密出乱码但通过 CRC" 的混淆场景

#### Scenario: 比特翻转攻击防御
- **WHEN** 攻击者在不知密码的情况下翻转密文中的 1 比特
- **THEN** MAC 校验以 1 - 2^(-80) 概率失败
- **AND** 失败时不解密，不返回任何明文线索

#### Scenario: MAC 长度与存储
- **WHEN** 写入 v4 Segment
- **THEN** MAC 长度固定为 10 字节（80 bit），符合 WinZip AES 规范
- **AND** MAC 不计入 `DataLength`（DataLength 只包含密文长度）
- **AND** 读取器按 `SegmentHeader.DataLength + 10` 定位 MAC

#### Scenario: MAC key 派生
- **WHEN** 派生 v4 容器的 MAC key
- **THEN** `mac_key = PBKDF2-SHA256(password, mac_salt, 100000, 32 bytes)`
- **AND** `mac_salt` 是与加密 salt 不同的独立 16 字节随机值
- **AND** `mac_salt` 存储在 v4 Header 偏移 36-52（复用 Reserved 区域）
- **AND** 读取器根据 `mac_salt` 重新派生 mac_key

### Requirement: v4 Header CipherMode + MacSalt 字段

v4 Header SHALL 在固定偏移位置嵌入 `CipherMode` 和 `MacSalt` 字段，标识加密算法和 MAC 密钥派生 salt。

#### Scenario: Header 布局更新
- **WHEN** 写入 v4 Header
- **THEN** 在偏移 4 字节（Version 字段之后）保留 2 字节的 `CipherMode` 字段
- **AND** `CipherMode = 0` 表示 AES-128-CTR，`CipherMode = 1` 表示 AES-256-CTR
- **AND** 在偏移 36-52 字节存储 `MacSalt [16]byte`（复用 Reserved 区域）
- **AND** 与 `plugin-version-selection-and-password-detection` 的 `PasswordHint`（偏移 20-36）共存不冲突
- **AND** 旧 v4 容器的 CipherMode 默认按 0 解析，MacSalt 缺失时回退到与 encrypt salt 共享

### Requirement: SegmentHeader 扩展字段

`SegmentHeader` SHALL 扩展为支持 MAC、ModeFlags、压缩索引：

```
Offset  Size  Field               Description
0       4     SegmentID           Segment 唯一标识
4       8     DataLength          密文长度（不含 MAC）
12      2     NonceSize           16
14      2     ModeFlags           位字段（bit0=Encrypted, bit1=Compression）
16      2     MACSize             10（HMAC-SHA1-80 截断长度）
18      4     DataCRC32           密文 CRC32（可选）
22      2     CompressedBlockSize seekable zstd 块大小（0=无压缩）
24      2     Reserved
26      4     SeekTableOffset     seek table 相对 Segment 起始的偏移（zstd 时有效）
30      4     SeekTableLength     seek table 长度
Total: 34 bytes (SegmentHeaderSize_v2 = 34)
```

---

## MODIFIED Requirements

### Requirement: Segment 独立加密模型

v4 Segment 独立加密模型扩展为：**独立 nonce + 独立 mac_key + 可选压缩 + 尾部 MAC**。

#### Scenario: 扩展后的 Segment 布局
- **WHEN** 写入加密 Segment
- **THEN** 布局为：`[SegmentHeader(34B)][Nonce(16B)][Ciphertext(DataLength B)][HMAC-SHA1-80(10B)]`
- **AND** 整 Segment 总大小 = `34 + 16 + DataLength + 10`
- **WHEN** 写入不加密 Segment（ModeFlags.Encrypted=0）
- **THEN** 跳过 Nonce/Ciphertext/MAC，整 Segment 仅包含 `SegmentHeader(34B) + Plaintext(DataLength B)`

### Requirement: KDF 密钥派生

`GenerateKey_v4` SHALL 支持变长输出。

#### Scenario: 16 字节派生
- **WHEN** `keyLen = 16`
- **THEN** `key = PBKDF2-SHA256(password, salt, 100000, 16 bytes)`
- **AND** 对应 `CipherMode = 0`（AES-128-CTR）

#### Scenario: 32 字节派生
- **WHEN** `keyLen = 32`
- **THEN** `key = PBKDF2-SHA256(password, salt, 100000, 32 bytes)`
- **AND** 对应 `CipherMode = 1`（AES-256-CTR）

### Requirement: 配置 schema 扩展

`config.schema.json` SHALL 增加：
- `v4_cipher_mode`: integer enum [0, 1]，默认 0
- `v4_compression_mode`: string enum ["none", "zstd"]，默认 "none"
- `v4_enable_hmac`: bool，默认 true（关闭则跳过 MAC 写入，向后兼容旧 v4 容器）

---

## REMOVED Requirements

### Requirement: v3 默认 AES-256-CTR 强制
**Reason**: v4 改用 AES-128-CTR 默认 + AES-256 可选，与 WinZip/RAR/7-Zip 行业惯例对齐
**Migration**: v3 仍按 AES-256-CTR 处理（旧版本兼容），所有 v4 新容器按新 spec 创建

### Requirement: DataCRC32 单独作为完整性保证
**Reason**: 未加密 CRC 可被攻击者重算绕过，对 CTR 模式的比特翻转攻击无效
**Migration**: HMAC-SHA1-80 成为强制完整性保护，CRC32 降级为可选二次校验

---

## Tasks

按 Phase 1 → 6 顺序执行，详见 `tasks.md`。

## Verification

按 `checklist.md` 逐项验证。
