# ENCV 加密容器架构文档

> 本文档目录沉淀 ENCV 加密容器的架构设计、演进历史和升级方案。

## 文档列表

| # | 文档 | 说明 | 状态 |
|---|------|------|------|
| 01 | [当前架构现状分析](01-current-architecture-analysis.md) | ECv4 现有架构全面调研：密钥派生、数据加密、完整性校验、容器结构 | ✅ 已完成 |
| 02 | [分层密钥架构升级方案](02-hierarchical-key-architecture.md) | 信封加密（Envelope Encryption）方案：DEK/KEK 分层、换密码零成本、多密码支持 | ⚠️ **已被实现取代（2026-09-28）** —— 见下方「关键决策点的实现答案」 |
| 03 | [分层密钥架构 API 设计](03-api-design.md) | 数据结构、核心函数、Reader/Writer 改动点、测试设计 | ⚠️ **同上** —— 文末 6 个待确认项已由实现给出答案；文档与方法签名冲突时**以代码为准** |

> ⚠️ 02/03 写于方案阶段，行文中「建议 / 待确认 / 计划」的表述未逐个改动。
> 它们是**设计方案的历史记录**，不是现状描述。现状见下。

## 核心概念速览

### 当前架构（ECv4）
- 单层密钥派生：PBKDF2(password, salt) → 直接得到 AES 密钥
- 数据加密：AES-128-CTR（默认）/ AES-256-CTR（可选）
- 完整性：HMAC-SHA1-80（Encrypt-then-MAC）
- 压缩：seekable zstd（可选）
- **痛点**：换密码 = 重加密所有数据

### 目标架构（原称 ECv5 / v4.1）—— **已落地（2026-09-28）**
- 分层密钥架构（信封加密）：
  - **DEK**（Data Encryption Key）：随机生成，加密实际数据
  - **KEK**（Key Encryption Key）：从密码派生，加密 DEK
- 数据层保持不变（AES-CTR + HMAC）
- **优势**：换密码只需重新包络 DEK，O(1) 完成，数据不动
- ⚠️ **换密码这条路目前只是"数据结构上可行"，还没有重包络的实现入口**（未做 UI/CLI 命令）。

## 关键决策点的实现答案（原「待确认」，2026-09-28 起以代码为准）

1. **版本号**：**没有引入 ECv5 / v4.1**，信封仍是既有版本，`WrappedDEK` 以
   `Manifest_v4.WrappedDEK` 扩展字段的方式加入 —— 旧容器因 `omitempty` 不受影响。
2. **KEK 迭代次数**：**10000**（`internal/v2/crypto/wrapped_dek.go` 的 `KDFIterations`）。
   KEK = `SHA256(PBKDF2-SHA256(password, "encv-v4-master-key-context", 10000, 32) ‖ encrypt_salt ‖ "kek")`。
   ⚠️ 两点与直觉不符、别照着 02/03 的方案理解：
   - master key 的 salt 是**固定常量字符串**（不是容器 salt），且进程内有 password→mk 缓存；
   - 迭代次数显著低于数据密钥派生（encvs1 信封用 `crypto.Iterations_v2`）。
3. **DEK 默认长度**：**16 字节 = AES-128-CTR**（`KeySize_v4_128`）。
   `EnvelopeHeaderV4.CipherMode` 因此保持零值；若将来改 AES-256 必须同步改该字段，
   否则旧回退路径会派生 16 字节密钥去解 32 字节密文 —— CTR 无认证标签，
   表现是**不报错、长度正确、内容全乱码**（`internal/v2/types/header_v4.go` 有警示注释）。
4. **WrappedDEK 存储位置**：**Manifest**（`manifest_v4.wrapped_dek`），不在 Header。
5. **AAD（关联数据）**：**绑了，但只绑 `encrypt_salt`** —— 主线
   `crypto.PrepareEncryptionContext` 传 `aad := salt`，并未绑定容器元数据；
   部分测试/演进路径传 `nil`（此时不启用 AAD，仍能解开）。**没有统一约定**。
6. **多密码支持**：**未做**，`Manifest_v4.WrappedDEK` 是单值结构体，没有 recipients 列表。

读取侧的两处关键分支（其余读取路径均由此派生）：
- `internal/v2/reader/segment_reader.go`：有 WrappedDEK → `UnwrapDEK(mfV4.WrappedDEK, kek)`；
  无则回退「password + encrypt_salt 直接派生」，保证迁移前的存量 v4 容器仍能打开。
- `internal/v2/reader/virtual_seekable_reader.go` 的 `deriveKeyAndIV`：同上两分支。
  ⚠️ 2026-09-28 修过一个**半迁移**缺陷：工厂没把 v4 manifest 传给构造函数，
  `manifestV4 == nil` → 静默走回退分支 → 密钥与 DEK 不符 → 乱码。
  回归锁见该期用例，改动这两条分支前务必先跑 `internal/v2/reader` 的测试。

详细讨论见 [02-hierarchical-key-architecture.md §九](02-hierarchical-key-architecture.md#九待确认问题) 和
[03-api-design.md §七](03-api-design.md#七待确认问题)（均为方案阶段的历史记录）。
