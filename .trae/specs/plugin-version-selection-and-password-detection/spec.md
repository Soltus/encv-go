# 插件系统容器版本选择与密码错误感知 Spec

## Why

当前系统存在两个关键问题：
1. **容器版本选择缺失**：插件系统没有提供容器版本选择的机制，默认版本不明确，v2 容器已过时但未标记为弃用，前端无法引导用户选择合适的版本。
2. **密码错误感知能力不足**：当用户输入错误密码解密时，系统无法区分"密码错误"和"数据损坏"，导致用户体验差（无法给出明确提示）和调试困难。AES-CTR 解密本身不会因密钥错误而报错，只会产生乱码输出。

## What Changes

- 新增容器版本选择机制，支持在配置中定义默认版本（推荐 v4）
- 将 v2 容器标记为已弃用（deprecated），前端显示为不可选择
- 新增密码错误检测机制，通过在 v4 及后续版本容器中嵌入校验特征来实现
- 提供明确的 `ErrWrongPassword` 错误类型，区别于数据损坏错误

## Impact

- Affected specs: 插件接口规范、容器格式规范、配置 schema、前端 UI
- Affected code:
  - `internal/v2/plugins/registry.go` — 插件接口扩展版本选择方法
  - `internal/v2/types/container.go` — 版本常量和元数据定义
  - `internal/v2/crypto/` — 密码错误检测逻辑
  - `internal/v2/container/` — 容器读写层适配
  - `config.schema.json` — 配置 schema 更新
  - 前端 `Files.vue` / `Tasks.vue` — 版本选择 UI 和错误提示

---

## ADDED Requirements

### Requirement: 容器版本选择机制

系统 SHALL 提供容器版本选择机制，允许管理员/用户配置默认容器版本，并在加密时使用指定版本。

#### Scenario: 默认版本配置
- **WHEN** 系统初始化或读取配置时
- **THEN** 从配置中读取 `default_container_version` 字段，若未配置则使用推荐值 `4`
- **AND** 该值用于所有新建容器的版本选择

#### Scenario: 版本有效性验证
- **WHEN** 用户指定容器版本进行加密
- **THEN** 系统验证版本号是否在支持列表 `[2, 3, 4]` 中
- **AND** 若版本为 2，记录弃用警告日志但仍允许操作（向后兼容）

### Requirement: 版本状态定义

系统 SHALL 定义每个容器版本的状态，用于前端展示。

| 版本 | 状态 | 说明 |
|------|------|------|
| v2 | deprecated | 已弃用，仅支持读取，不建议新建 |
| v3 | stable | 稳定版，完全支持 |
| v4 | recommended | 推荐版本，默认选择 |

#### Scenario: 前端版本列表渲染
- **WHEN** 前端渲染容器版本选择列表
- **THEN** 显示 v3 和 v4 为可选状态
- **AND** v2 显示为灰色禁用状态，附带"已弃用"标签
- **AND** v4 标记为"推荐"

### Requirement: 密码错误感知机制（v4+ 容器）

系统 SHALL 在 v4 及后续版本容器中实现密码错误感知能力，使得解密时能区分"密码错误"和"数据损坏"。

#### 设计决策：为什么选择 v4 容器层面实现而非程序架构层面

**方案 A：程序架构层面（全局）**
- 成本：需修改所有插件的解密流程，增加通用校验逻辑
- 效用：对所有版本生效，但 v2/v3 已有容器无此能力
- 缺点：需要额外的存储空间（校验标记），且对旧容器无效；实现复杂度高

**方案 B：v4+ 容器层面（推荐 ✅）**
- 成本：仅在 v4 Header 或 Manifest 中增加少量校验字段
- 效用：仅对新创建的 v4 容器有效，但实现简单、可靠
- 优点：利用 v4 的 Segment 模型天然优势，可在每个 Segment 中嵌入轻量级校验；不影响旧容器兼容性

**结论**：采用方案 B，在 v4 容器中通过以下机制实现：

1. **Header 嵌入 PasswordHint**：在 v4 Header 的 Reserved 区域添加一个 `PasswordHint` 字段（16 字节），该字段是使用正确密码派生的密钥对固定明文 `"ENCV_PASSWORD_CHECK"` 进行 HMAC-SHA256 后取前 16 字节的结果。
2. **Segment 校验增强**：每个 Segment 的 DataCRC32 使用密码相关的密钥计算（而非简单的 CRC32），使得错误密码会导致 CRC 校验失败。

#### Scenario: 密码错误检测（v4 容器）
- **WHEN** 用户使用错误密码解密 v4 容器
- **THEN** 系统首先尝试验证 Header 中的 `PasswordHint`
- **AND** 若 PasswordHint 不匹配，立即返回 `ErrWrongPassword` 错误
- **AND** 错误消息应包含明确的提示："密码可能错误，请检查后重试"
- **AND** 不应返回通用的"解密失败"或"数据损坏"错误

#### Scenario: 数据损坏区分（v4 容器）
- **WHEN** 用户使用正确密码解密但数据已损坏的 v4 容器
- **THEN** PasswordHint 验证通过
- **AND** 系统继续执行 CRC32 校验或其他完整性检查
- **AND** 若校验失败，返回 `ErrDataCorrupted` 错误（区别于 `ErrWrongPassword`）

#### Scenario: 向后兼容（v2/v3 容器）
- **WHEN** 用户解密 v2 或 v3 容器
- **THEN** 系统不执行 PasswordHint 检查（这些版本不支持）
- **AND** 回退到现有的通用错误处理逻辑
- **AND** 错误消息保持原有行为（不做区分）

### Requirement: 密码错误相关错误类型

系统 SHALL 定义明确的错误类型来区分不同解密失败原因。

```go
var (
    ErrWrongPassword   = errors.New("wrong password: password hint mismatch")
    ErrDataCorrupted   = errors.New("data corrupted: integrity check failed")
    ErrDeprecatedVersion = errors.New("container version is deprecated")
)
```

#### Scenario: 错误类型使用
- **WHEN** 插件的 Decrypt 方法检测到密码错误
- **THEN** 返回 `ErrWrongPassword` 包装的错误
- **WHEN** 插件的 Decrypt 方法检测到数据损坏
- **THEN** 返回 `ErrDataCorrupted` 包装的错误
- **WHEN** 用户尝试使用 v2 版本创建新容器
- **THEN** 返回警告但不阻止操作（或根据严格模式决定）

## MODIFIED Requirements

### Requirement: V4 Header 结构

v4 Header SHALL 在 Reserved 区域新增 `PasswordHint` 字段：

```
Offset  Size  Field               Description
0       4     Magic               "ENVC"
4       2     Version             0x04
6       2     Flags               位标志
8       2     ContainerType       容器类型标识
10      1     IsSeekable          0x01=可随机访问, 0x00=顺序访问
11      1     Reserved
12      4     IDType              ID 编码类型
16      4     IDLength            ID 有效长度
20      16    PasswordHint        【新增】密码校验提示（HMAC-SHA256 前 16 字节）
36      1992  SpecialID           特殊 ID 存储（从原 2000 减少至 1992）
2028    4     ManifestOffset      Manifest 在文件中的字节偏移
2032    4     ManifestLength      Manifest 数据长度
2036    4     HeaderCRC32         头部 CRC32 校验
2040    8     Reserved            填充至 2048 字节
Total: 2048 bytes
```

### Requirement: 配置 Schema

配置 schema SHALL 新增以下字段：

```json
{
  "default_container_version": {
    "type": "integer",
    "default": 4,
    "minimum": 2,
    "maximum": 4,
    "description": "默认容器版本（2=已弃用, 3=稳定, 4=推荐）"
  },
  "strict_deprecated_version": {
    "type": "boolean",
    "default": false,
    "description": "是否严格禁止使用已弃用版本创建容器"
  }
}
```

### Requirement: 插件接口

插件接口 SHALL 扩展以下方法：

```go
type Plugin interface {
    // ... 现有接口 ...

    // === 版本选择支持（新增）===
    SupportedContainerVersions() []int          // 返回支持的容器版本列表 [2,3,4]
    DefaultContainerVersion() int               // 返回推荐的默认版本（通常为 4）
    ValidateVersion(version int) error          // 验证版本是否可用
}
```

## REMOVED Requirements

无

---

## 技术实现细节

### PasswordHint 计算算法

```
输入：
  - password: string (用户密码)
  - salt: []byte (KVI 中的 salt)

步骤：
  1. key = PBKDF2(password, salt, 100000 iterations, 32 bytes, SHA-256)
  2. hint = HMAC-SHA256(key, "ENCV_PASSWORD_CHECK")[0:16]

输出：
  - PasswordHint: [16]byte
```

### 碰撞概率分析

PasswordHint 长度为 16 字节（128 位），碰撞概率极低：
- 单次随机碰撞概率：2^-128 ≈ 2.94 × 10^-39
- 即使攻击者尝试 10^12 次，碰撞概率仍可忽略不计

这足以区分"密码错误"和"数据损坏"。

### 前端版本选择 UI 设计

```
┌─────────────────────────────┐
│  选择容器版本                 │
├─────────────────────────────┤
│  ○ V2 (已弃用)              │  ← 灰色，disabled
│  ○ V3                       │
│  ● V4 (推荐)                │  ← 默认选中，绿色标签
└─────────────────────────────┘
```

### 错误消息设计

| 场景 | 错误代码 | 用户可见消息 |
|------|----------|-------------|
| 密码错误 (v4) | WRONG_PASSWORD | "密码可能错误，请检查后重试" |
| 数据损坏 (v4) | DATA_CORRUPTED | "文件数据已损坏，无法完整恢复" |
| 通用解密失败 (v2/v3) | DECRYPT_FAILED | "解密失败，请检查密码和文件完整性" |
| 版本弃用警告 | DEPRECATED_VERSION | "V2 版本已弃用，建议使用 V4" |
