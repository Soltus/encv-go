# alist-encrypt 插件 + ENCV v4 容器文件名加密 Spec

## Why

当前 ENCV 系统需要两个独立的文件名加密能力：
1. **alist-encrypt 插件**：与上游 [alist-encrypt-go](https://github.com/qingwo1991-debug/alist-encrypt-go) 保持一致的 mixBase64 + CRC6 文件名加密
2. **ENCV v4 容器**：基于 Manifest 清单的**深度定制的文件名混淆编码方案**——核心原则是**文件名为展示层抽象**，物理文件名可被任意破坏，原始文件名始终可从容器清单恢复；支持运行时修改原始文件名并实时反映到显示层；需妥善处理超长/超短/特殊字符等边界情况

> **关键设计决策**：v4 容器的文件名混淆**不能直接套用通用加密算法**（AES-GCM 等），而必须是一套**深度定制的确定性编码方案**，具备以下独特属性：
> - **人类可读性**：输出是结构化的编码串而非纯乱码
> - **规律性**：确定性算法，同输入+同密钥→相同输出；编码结构可分析
> - **长短双方案**：短模式（紧凑）和长模式（结构化丰富）两种输出策略
> - **字符集多选合集**：用户从多种字符池中自由组合，输出字符集为所选集合的并集
> - **去混淆全局开关**：独立于字符集选择，可选移除 `0Oo1lI` 等易混淆字符

## What Changes

### A. alist-encrypt 插件文件名加密（与 alist-encrypt-go 对齐）

- **Go 后端** (`internal/alistencrypt/`)：确保 EncryptedName 的 mixBase64 编码 + CRC6 校验 + AES-CTR 加密逻辑与参考实现二进制兼容
- **前端** (`features/alist-encrypt/useAlistEncrypt.ts`)：decodeAlistFilename 函数同步更新

### B. ENCV v4 容器文件名混淆 — 深度定制的 ENC-FN 编码方案

核心设计原则：**文件名是展示层元数据，不是存储标识**。

- **Manifest_v4 扩展**：`original_name` 字段存储 UTF-8 原始文件名（明文或 ENC-FN 编码后）
- **Header Flags**：`FlagFilenameEncrypted` (0x0010) 标志位表示 original_name 已编码
- **ENC-FN 编码器** (`internal/v2/filename/encfn.go`)：全新实现的深度定制文件名混淆方案
- **文件名解析优先级**：`Manifest.original_name（解码后） > 物理文件名`
- **原始文件名修改 API**：`PATCH /api/v1/file/rename`
- **边界处理**：超长/空/Unicode 全量支持

## Impact

- Affected code:
  - `internal/alistencrypt/filename.go` — alist-encrypt 插件 EncryptedName 加解密
  - `internal/v2/filename/encfn.go` — **新增** ENC-FN 深度定制编码器
  - `internal/v2/filename/charset.go` — **新增** 字符集定义（多选合集 + 去混淆开关）
  - `internal/v2/types/segment_v4.go` — Manifest_v4 新增 original_name / filename_alg 字段
  - `internal/v2/types/header_v4.go` — FlagFilenameEncrypted 标志位
  - `app/encv-mobile/src/features/alist-encrypt/useAlistEncrypt.ts` — 前端解码函数
  - `app/encv-mobile/src/views/Files.vue` — 文件列表显示逻辑

## ADDED Requirements

### Requirement: alist-encrypt 文件名加密对齐

系统 SHALL 提供 alist-encrypt 插件的文件名加解密能力，且与 alist-encrypt-go 参考实现保持二进制兼容。

#### Scenario: 加密文件名生成
- **WHEN** 后端执行 EncryptName(plainName, password)
- **THEN** 输出格式：`mixBase64(AES-CTR(plaintext, key)) + "_" + hex(CRC6(ciphertext))`

#### Scenario: 解密文件名还原
- **WHEN** 执行 DecryptName(encName, password)
- **THEN** 先校验 CRC6，再 AES-CTR 解密，返回 UTF-8 明文
- **AND** CRC6 失败时返回明确错误而非乱码

---

### Requirement: ENC-FN 深度定制文件名编码方案

系统 SHALL 为 v4 容器提供一套**深度定制的确定性文件名编码方案**（代号 ENC-FN），具备人类可读性、规律性、长短双模式、**多选字符集合集**和**独立去混淆开关**。此方案**不直接复用任何现成的通用加密或编码库**，而是从零设计的专用算法。

#### ENC-FN 算法概要

```
编码流程 (Encode):
  plaintext(UTF-8 bytes) → KDF(password) → S-box生成 → 字节置换 → Feistel轮变换 → 目标字符集映射 → 输出编码串

解码流程 (Decode):
  编码串 → 字符集逆映射 → Feistel逆轮变换 → 字节逆置换 → UTF-8 → 明文
```

#### ENC-FN 核心组件

| 组件 | 说明 | 设计要点 |
|------|------|---------|
| **KDF** | 密钥派生函数 | 基于 HKDF-SHA256 从密码派生主密钥，再派生 S-box 种子和轮密钥 |
| **S-box** | 256 字节置换表 | 由 KDF 输出的种子确定性生成，每次编码/解码重建相同的 S-box |
| **Feistel 网络** | 多轮混淆变换 | 4-12 轮（可配置），每轮使用不同轮密钥，保证雪崩效应 |
| **字符集引擎** | 多选合集 → 并集映射 | 用户从多个字符池中选择，最终字符表为并集，再经去混淆过滤 |

#### ENC-FN 长短双方案

| 模式 | 标识 | 输出特征 | 适用场景 |
|------|------|---------|---------|
| **短模式 (compact)** | `C` | 无前缀，纯编码体，最大压缩率 | 需要最小长度 |
| **长模式 (structured)** | `S` | 带 `[S]` 前缀 + 长度标记 + 编码体 + 校验后缀 | 需要自描述和完整性校验 |

**短模式输出示例**：
```
原始: "report.pdf"
ENC-FN(compact, [alnum]):           "xK7mPq2RnWv"
ENC-FN(compact, [alnum, hanzi_rare]): "xK7mPq2龘Wv"
ENC-FN(compact, [alnum, symbols]):   "xK7mPq2®Wv"
```

**长模式输出示例**：
```
原始: "2024年度财务报表_Q3_final_version.pdf"
ENC-FN(structured, [alnum, hanzi_common]):
  "[S]44:xK7mPq2RnWvLsT9uYzAbCdEfGhIjKlMnOpQrStUv财年报Q3:p3"
```

其中 `[S]` = 结构化前缀，`44` = 原始字节长度，`:p3` = 截断校验

#### ENC-FN 字符集池（多选）

每个字符池是一个预定义的 Unicode 字符子集。用户可**同时选择多个**，最终输出字符表为所有选中字符池的**并集**。

`alnum` 是**必选基础池**（始终包含），其余为可选扩展池。

| 字符集 ID | 必选 | 示例字符 | 大小 | 特征 |
|-----------|------|----------|------|------|
| `alnum` | ✅ 是 | `a-z A-Z 0-9` | 62 | 全字母数字，信息密度基础 |
| `symbols_basic` | 可选 | `_-.~` | 4 | URL 安全基本符号 |
| `symbols_extended` | 可选 | `!@#$%^&*()+=[]{}|;:,.<>?/` | 26 | 扩展符号集 |
| `hanzi_rare` | 可选 | `龘靁齉爨麤毊...` | ~1000+ | 生僻汉字（Unicode CJK 扩展区精选，天然规避敏感词审核） |
| `emoji` | 可选 | `🎉🔐📁💾🔒...` | ~100 | 表情符号（精选适合文件名的非控制类 emoji） |

> **设计决策**：
> - **alnum 必选**：保证输出至少包含字母数字，避免纯符号/纯汉字等极端组合导致文件系统兼容性问题
> - **无 hanzi_common**：常用汉字需复杂的敏感字过滤来避免内容审核误判；生僻汉字（hanzi_rare）天然不在任何常见敏感词库中，零过滤成本
> - **无 alnum 子集**：lowercase/uppercase/digits/hex_* 均为 alnum 的真子集，用户可通过去混淆开关控制是否保留数字或大小写，无需独立字符池

#### 去混淆全局开关

去混淆是**独立于字符集选择的布尔选项**。启用后，从最终并集字符表中移除以下易混淆字符：

| 移除字符 | 替代说明 |
|----------|---------|
| `0` (零) | 与 `O`(大写o) 和 `D` 易混 |
| `O` (大写o) | 与 `0`(零) 和 `Q` 易混 |
| `o` (小写o) | 与 `0`(零) 和 `c` 易混 |
| `1` (一) | 与 `l`(小写L) 和 `I`(大写i) 易混 |
| `l` (小写L) | 与 `1`(一) 和 `I`(大写i) 易混 |
| `I` (大写i) | 与 `1`(一) 和 `l`(小写L) 易混 |

> **设计理由**：去混淆不是某个特定字符集的内置属性，因为用户可能组合任意字符集（如 `hanzi_rare + alnum + symbols`），每种组合都需要同样的去混淆保护。

#### ENC-FN 配置接口

```go
type FNConfig struct {
    Mode        FNMode      // FNCompact | FNStructured
    Charsets    []FNCharset // 多选字符集扩展池（alnum 始终隐含包含）
    Deconfuse   bool        // 是否移除易混淆字符 0Oo1lI (默认 true)
    Rounds      int         // Feistel 轮数 (默认 6, 范围 4-12)
    Truncate    bool        // 是否在长模式下截断并附加校验 (默认 true)
}

type FNMode string
const (
    FNCompact    FNMode = "compact"
    FNStructured FNMode = "structured"
)

type FNCharset string
const (
    FNAlnum        FNCharset = "alnum"            // 必选（隐含），a-zA-Z0-9
    FNSymbolsBasic FNCharset = "symbols_basic"    // _-.~
    FNSymbolsExt   FNCharset = "symbols_extended" // !@#$%^&*()+=[]{}|;:,.<>?/
    FNHanziRare    FNCharset = "hanzi_rare"       // 生僻汉字 ~1000+
    FNEmoji        FNCharset = "emoji"            // 文件名安全 emoji ~100
)

// FilenameAlgorithm 序列化格式:
// "enc-fn:{mode}:{charset1,charset2,...}:deconfuse={true|false}"
// alnum 不在序列化中显式出现（始终隐含）
// 例: "enc-fn:compact:hanzi_rare,emoji:deconfuse=true"
// 例: "enc-fn:structured:symbols_extended:deconfuse=false"
// 例: "enc-fn:compact::deconfuse=true"  （仅 alnum，无扩展池）
```

#### Scenario: 多选字符集编码
- **WHEN** 用户选择扩展字符集为 `[hanzi_rare, emoji]`，去混淆开启
- **THEN** 最终字符表 = alnum(62-6去混淆) ∪ hanzi_rare(~1000) ∪ emoji(~100) ≈ 1156 个字符
- **AND** 输出编码串中同时包含拉丁字母/数字、生僻汉字和表情符号
- **AND** 信息密度高（log2(1156) ≈ 10.18 bits/字符）

#### Scenario: 纯 alnum 编码（无扩展池）
- **WHEN** 用户不选择任何扩展字符集（Charsets 为空）
- **THEN** 最终字符表 = alnum(62) - 去混淆(6) = 56 个字符（若 Deconfuse=true）
- **AND** 输出为纯字母数字编码串，最简配置

#### Scenario: 去混淆开关行为
- **WHEN** Deconfuse=true 且字符集包含 alnum
- **THEN** 从最终字符表中移除 `0 O o 1 l I` 共 6 个字符
- **WHEN** Deconfuse=false
- **THEN** 保留所有原始字符不做任何移除
- **WHEN** 字符集不含 alnum/digits（如纯 hanzi_common）
- **THEN** Deconfuse 开关无效果（无易混淆字符可移除）

#### Scenario: ENC-FN 短模式编码
- **WHEN** 使用 compact 模式，字符集 `[alnum, hanzi_rare]`，Deconfuse=true
- **THEN** 输出一个紧凑的混合编码串（无前缀无后缀）
- **AND** 同一输入+同一密码→相同输出（确定性）

#### Scenario: ENC-FN 长模式编码
- **WHEN** 使用 structured 模式编码含 emoji 的文件名
- **THEN** 输出以 `[S]` 开头，包含长度标记、编码体、校验后缀
- **AND** 解码时可验证完整性

#### Scenario: ENC-FN 解码失败处理
- **WHEN** 编码串被篡改或使用错误配置（字符集不匹配/密码错误）
- **THEN** Decode 返回明确错误（ErrFNInvalidFormat / ErrFNCorrupt / ErrFNChecksumMismatch / ErrFNCharsetMismatch）

---

### Requirement: ENCV v4 容器文件名作为展示层抽象

系统 SHALL 将 v4 容器的文件名视为**可恢复的展示层元数据**，而非不可变的存储标识。

#### Scenario: 物理文件名被破坏时从 Manifest 恢复
- **WHEN** v4 容器的物理文件名被重命名为任意字符串
- **AND** Manifest.original_name 存在有效值
- **THEN** 系统（API 返回 / 前端列表）始终显示 Manifest 中的原始文件名（ENC-FN 解码后）

#### Scenario: 启用文件名编码时的显示行为
- **WHEN** 创建 v4 容器时设置了 FlagFilenameEncrypted 且指定了密码和 FNConfig
- **THEN** Manifest.original_name 存储的是 ENC-FN 编码后的文件名
- **AND** 前端/API 层通过 ENC-FN.Decode 还原明文文件名显示
- **AND** 若解码失败（无密码/密码错误），则显示物理文件名或占位符

#### Scenario: 修改原始文件名
- **WHEN** 用户通过 API 或 UI 修改某个 v4 容器的原始文件名
- **THEN** Manifest.original_name 被 ENC-FN.Encode(newName) 更新
- **AND** 文件列表立即反映修改后的文件名
- **AND** 物理文件名不受影响

#### Scenario: 超长文件名处理
- **WHEN** 原始文件名超过 255 字节
- **THEN** Manifest.original_name 完整存储 ENC-FN 编码结果
- **AND** 物理文件名自动缩短为安全长度
- **AND** 展示层始终显示完整的原始文件名

#### Scenario: 空/Unicode 文件名处理
- **WHEN** 原始文件名为空或含 emoji/CJK/生僻汉字/控制字符
- **THEN** UTF-8 字节全量传入 ENC-FN 编码，不做过滤
- **AND** 展示层正确渲染

## MODIFIED Requirements

### Requirement: Manifest_v4 数据结构

```go
type Manifest_v4 struct {
    // ... 已有字段 ...
    OriginalName      string `json:"original_name,omitempty"`       // 原始文件名（明文或 ENC-FN 编码）
    FilenameAlgorithm string `json:"filename_alg,omitempty"`        // "none" | "enc-fn:{mode}:{cs1,cs2,...}:dc={bool}"
}
```

### Requirement: Header Flags 位域

```go
const FlagFilenameEncrypted uint16 = 0x0010 // Manifest.original_name 是 ENC-FN 编码存储的
```

### Requirement: 文件名解析优先级

```
1. 若 v4 容器 Manifest.original_name 非空：
   a. FlagFilenameEncrypted 未设置 → 直接作为显示名
   b. FlagFilenameEncrypted 已设置 → ENC-FN.Decode(original_name, password, fnConfig) → 显示（失败 fallback 到步骤 2）
2. 否则 → 物理文件名
```

> **注意**：Decode 时需要的 fnConfig（字符集列表、去混淆开关等）从容器创建时的配置持久化而来，可通过 API 获取或在 Manifest.FilenameAlgorithm 中解析重建。

### Requirement: 前端 decodeAlistFilename

alist-encrypt 插件的 decodeAlistFilename SHALL 支持 EncryptedName 格式（mixBase64 + CRC6 + AES-CTR），与 v4 容器的 ENC-FN 方案独立。

## REMOVED Requirements

无。
