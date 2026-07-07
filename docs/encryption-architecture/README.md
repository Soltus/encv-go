# ENCV 加密容器架构文档

> 本文档目录沉淀 ENCV 加密容器的架构设计、演进历史和升级方案。

## 文档列表

| # | 文档 | 说明 | 状态 |
|---|------|------|------|
| 01 | [当前架构现状分析](01-current-architecture-analysis.md) | ECv4 现有架构全面调研：密钥派生、数据加密、完整性校验、容器结构 | ✅ 已完成 |
| 02 | [分层密钥架构升级方案](02-hierarchical-key-architecture.md) | 信封加密（Envelope Encryption）方案：DEK/KEK 分层、换密码零成本、多密码支持 | ✅ 待确认 |
| 03 | [分层密钥架构 API 设计](03-api-design.md) | 数据结构、核心函数、Reader/Writer 改动点、测试设计 | ✅ 待确认 |

## 核心概念速览

### 当前架构（ECv4）
- 单层密钥派生：PBKDF2(password, salt) → 直接得到 AES 密钥
- 数据加密：AES-128-CTR（默认）/ AES-256-CTR（可选）
- 完整性：HMAC-SHA1-80（Encrypt-then-MAC）
- 压缩：seekable zstd（可选）
- **痛点**：换密码 = 重加密所有数据

### 目标架构（ECv5 / v4.1）
- 分层密钥架构（信封加密）：
  - **DEK**（Data Encryption Key）：随机生成，加密实际数据
  - **KEK**（Key Encryption Key）：从密码派生，加密 DEK
- 数据层保持不变（AES-CTR + HMAC）
- **优势**：换密码只需重新包络 DEK，O(1) 完成，数据不动

## 关键决策点（待确认）

1. **版本号**：ECv5 还是 ECv4.1？
2. **KEK 迭代次数**：100k 还是 200k+？
3. **DEK 默认长度**：AES-128 还是 AES-256？
4. **WrappedDEK 存储位置**：Manifest 还是 Header？
5. **AAD（关联数据）**：是否绑定容器元数据？
6. **多密码支持**：第一期做不做？

详细讨论见 [02-hierarchical-key-architecture.md §九](02-hierarchical-key-architecture.md#九待确认问题) 和 [03-api-design.md §七](03-api-design.md#七待确认问题)。
