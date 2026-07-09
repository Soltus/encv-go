# ENCV 加密容器架构现状分析（ECv4）

> 调研日期：2026-07-08
> 代码基线：internal/v2/crypto, internal/v2/types, internal/v2/writer, internal/v2/reader

## 一、当前架构总览

ENCV v4 容器（ECv4）当前采用**单密钥派生 + AES-CTR 流式加密 + HMAC 完整性校验**的架构。

```
┌─────────────────────────────────────────────────────────────┐
│                    用户密码 (Password)                       │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼ PBKDF2-SHA256 (100,000 iter)
             ┌─────────────────┴─────────────────┐
             │                                   │
             ▼                                   ▼
    Encrypt Salt (16B)                   MAC Salt (16B)
             │                                   │
             ▼                                   ▼
    Data Key (AES-CTR)                    MAC Key (HMAC)
             │                                   │
             ▼                                   ▼
    ┌─────────────────┐          ┌─────────────────────────┐
    │  Segment 数据层  │          │  完整性校验层 (Segment)   │
    │  AES-128/256-CTR│          │  HMAC-SHA1-80 (10B 截断) │
    └─────────────────┘          └─────────────────────────┘
```

## 二、各层详细说明

### 2.1 密钥派生层（当前：单层 PBKDF2）

**位置**：`internal/v2/crypto/aes_v2.go`

| 项目 | 说明 |
|------|------|
| **算法** | PBKDF2-SHA256 |
| **迭代次数** | 100,000 次 |
| **盐值** | 16 字节随机盐（encrypt_salt + mac_salt 各 1 个） |
| **密钥长度** | AES-128 = 16 字节（默认）/ AES-256 = 32 字节（可选） |
| **派生通道** | 2 条独立通道（encrypt_key + mac_key） |

**关键函数**：
- `GenerateKey_v4(password, salt, keyLen)` — 加密密钥派生
- `DeriveMACKey(password, macSalt)` — MAC 密钥派生（独立通道）

### 2.2 数据加密层（AES-CTR）

**位置**：`internal/v2/crypto/aes_v2.go` + `internal/v2/crypto/segment_crypto.go`

| 项目 | 说明 |
|------|------|
| **算法** | AES-CTR（128 位默认 / 256 位可选） |
| **IV/Nonce** | 每个 Segment 独立 16 字节随机 Nonce |
| **流式支持** | ✅ 原生支持（CTR 模式本质是流密码） |
| **随机访问** | ✅ 支持（直接跳转到对应 block 计算 counter） |
| **并行加密** | ✅ 支持（CTR 模式可并行） |

**Segment 内层磁盘格式**：
```
[Nonce(16B)][Ciphertext(N B)][HMAC-SHA1-80(10B)]
```

### 2.3 完整性校验层（HMAC-SHA1-80）

**位置**：`internal/v2/crypto/segment_crypto.go`

| 项目 | 说明 |
|------|------|
| **算法** | HMAC-SHA1-80（前 80 bit = 10 字节截断） |
| **模式** | Encrypt-then-MAC（先加密，后算 MAC） |
| **MAC 输入** | nonce \|\| ciphertext（nonce 也参与计算防替换） |
| **校验时机** | 解密前校验（失败绝不解密、绝不解压） |
| **侧信道防护** | 使用 `crypto/subtle.ConstantTimeCompare` |

**安全保证**：
1. 防 CTR 模式比特翻转攻击
2. 防 nonce 替换重放攻击
3. 防 zstd 解压炸弹 DoS（MAC 不通过不解压）

### 2.4 容器结构层

**位置**：`internal/v2/types/header_v4.go` + `internal/v2/types/segment_v4.go`

**v4 Envelope Header（2048 字节固定大小）**：

| 偏移 | 大小 | 字段 | 说明 |
|------|------|------|------|
| 0 | 4 | Magic | 'ENCV' |
| 4 | 2 | Version | 0x04 |
| 6 | 2 | Flags | 主容器/物理分片/文件名加密等 |
| 8 | 2 | ContainerType | video/audio/image/document/text |
| 10 | 1 | IsSeekable | 0/1 标识 |
| 12 | 4 | IDType | ID 编码类型 |
| 16 | 4 | IDLength | ID 实际有效长度 |
| 20 | 16 | PasswordHint | 密码提示（快速密码错误感知） |
| 36 | 1992 | SpecialID | 业务元数据/占位 ID |
| 2028 | 4 | ManifestOffset | Manifest 字节偏移 |
| 2032 | 4 | ManifestLength | Manifest 长度 |
| 2036 | 4 | HeaderCRC32 | 头部 CRC32（范围 [0, 2036)） |
| 2040 | 2 | CipherMode | 0=AES-128-CTR / 1=AES-256-CTR |
| 2042 | 6 | Reserved2 | 保留填充 |

**Manifest（JSON，XOR 混淆存储）**：
- 存储在 SpecialID 之后的数据区域
- 包含 Segment 列表、播放列表、章节、KVI 等
- `MACSaltBase64` 字段存 MAC 盐（base64 编码）
- KVI 中存 encrypt_salt + encrypt_iv

### 2.5 压缩层（可选 zstd）

**位置**：`internal/v2/crypto/compression/`

| 项目 | 说明 |
|------|------|
| **算法** | seekable zstd（可寻址 zstd） |
| **块大小** | 64KB |
| **启用阈值** | ≥ 1KB（小于 1KB 不压缩，避免负收益） |
| **pipeline** | plaintext → zstd compress → AES-CTR → HMAC |

## 三、当前架构的优势

1. ✅ **流式性能优秀**：AES-CTR 模式天然支持流式，适合视频/大文件
2. ✅ **随机访问支持**：CTR 模式 + seekable zstd，可 O(1) 跳转到任意位置
3. ✅ **完整性保护完善**：Encrypt-then-MAC + 常量时间比较 + 前置校验
4. ✅ **压缩集成良好**：zstd 与加密 pipeline 无缝集成
5. ✅ **向后兼容设计**：CipherMode 等字段预留，旧容器可正常读取

## 四、当前架构的核心问题

### 4.1 换主密码必须重加密所有数据

**问题本质**：密钥是直接从密码派生的，密码 = 数据加密密钥。

```
用户密码 ──PBKDF2──▶ Data Key ──AES-CTR──▶ 所有数据
```

后果：
- 修改密码 → 需要解密所有数据 → 用新密码重新加密
- 对于 GB 级别的视频文件，耗时可能长达数分钟
- 过程中如果中断，数据可能处于不一致状态

### 4.2 多密码/多用户支持困难

当前架构下，一个容器只能有一个密码。要支持多密码：
- 必须存储多份加密的 DEK（分层架构下很简单）
- 当前架构只能存多份完整加密数据（不可行）

### 4.3 密钥轮换成本高

安全最佳实践要求定期轮换密钥，但当前架构下：
- 密钥轮换 = 重新加密所有数据
- 成本高到几乎不可行

## 五、下一步：分层密钥架构升级

参见 [02-hierarchical-key-architecture.md](02-hierarchical-key-architecture.md) 提出的 v5 升级方案。
