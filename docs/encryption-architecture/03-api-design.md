# 分层密钥架构 API 设计

> 设计日期：2026-07-08
> 状态：待确认

## 一、数据结构设计

### 1.1 WrappedDEK 结构体

```go
// internal/v2/types/wrapped_dek.go
package types

// WrappedDEKAlgorithm 标识 Wrapped DEK 使用的包裹算法。
type WrappedDEKAlgorithm string

const (
    WrappedDEKAlgAES256GCM WrappedDEKAlgorithm = "aes-256-gcm"
)

// WrappedDEK 是被 KEK 包络（加密）后的数据加密密钥。
//
// 存储位置：Manifest JSON 中（v4 容器的可变长元数据区域）
//
// 磁盘格式（JSON）：
//   {
//     "algorithm": "aes-256-gcm",
//     "nonce_base64": "...",
//     "ciphertext_base64": "...",
//     "tag_base64": "...",
//     "aad_base64": "..."  // 可选
//   }
type WrappedDEK struct {
    Algorithm        WrappedDEKAlgorithm `json:"algorithm"`
    NonceBase64      string              `json:"nonce_base64"`
    CiphertextBase64 string              `json:"ciphertext_base64"`
    TagBase64        string              `json:"tag_base64"`
    AADBase64        string              `json:"aad_base64,omitempty"`
}

// HasWrappedDEK 判断 Manifest 中是否有有效的 WrappedDEK。
// 用于向后兼容：旧 v4 容器没有此字段，走旧的单密钥派生流程。
func (m *Manifest_v4) HasWrappedDEK() bool {
    return m.WrappedDEK != nil && m.WrappedDEK.Algorithm != ""
}
```

### 1.2 Manifest_v4 扩展

在 `Manifest_v4` 结构体中新增字段：

```go
// internal/v2/types/segment_v4.go

type Manifest_v4 struct {
    // ... 现有字段 ...

    // WrappedDEK 是 v5（或 v4.1）分层密钥架构新增字段。
    // 存储被 KEK 包络后的数据加密密钥（DEK）。
    //
    // 为 nil / 空 时表示使用旧版单层密钥派生（向后兼容）。
    //
    // JSON tag 使用 omitempty：旧 v4 容器的 Manifest 序列化结果不会包含此字段，
    // 保持向前兼容的 JSON 形态。
    WrappedDEK *WrappedDEK `json:"wrapped_dek,omitempty"`
}
```

## 二、核心函数设计

### 2.1 crypto 包新增函数

```go
// internal/v2/crypto/wrapped_dek.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "io"
)

var (
    ErrInvalidWrappedDEK = errors.New("invalid wrapped dek structure")
    ErrDecryptDEKFailed  = errors.New("failed to decrypt DEK: GCM tag mismatch")
)

const (
    // DEKNonceSize 是 AES-GCM 包裹 DEK 时的 Nonce 长度。
    // GCM 标准推荐 12 字节（96 bit）。
    DEKNonceSize = 12

    // DEKTagSize 是 AES-GCM 认证标签长度（固定 16 字节）。
    DEKTagSize = 16
)

// GenerateDEK 生成一个随机的数据加密密钥（DEK）。
//
// 参数：
//   - keyLen: 密钥长度（字节），通常 16 (AES-128) 或 32 (AES-256)
//
// 返回：
//   - []byte: 随机生成的 DEK，长度为 keyLen
func GenerateDEK(keyLen int) ([]byte, error) {
    if keyLen <= 0 {
        keyLen = KeySize_v4_128 // 默认 AES-128
    }
    dek := make([]byte, keyLen)
    if _, err := io.ReadFull(rand.Reader, dek); err != nil {
        return nil, err
    }
    return dek, nil
}

// EncryptDEK 使用 KEK 通过 AES-256-GCM 包裹（加密）DEK。
//
// 参数：
//   - dek: 明文数据加密密钥（16 或 32 字节）
//   - kek: 密钥加密密钥（必须 32 字节，AES-256-GCM）
//   - aad: 可选关联数据（Additional Authenticated Data），可为 nil
//
// 返回：
//   - nonce: 12 字节随机 Nonce
//   - ciphertext: 加密后的 DEK（与明文等长）
//   - tag: 16 字节 GCM 认证标签
func EncryptDEK(dek, kek, aad []byte) (nonce, ciphertext, tag []byte, err error) {
    block, err := aes.NewCipher(kek)
    if err != nil {
        return nil, nil, nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, nil, nil, err
    }

    nonce = make([]byte, DEKNonceSize)
    if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, nil, nil, err
    }

    // Seal 会返回 ciphertext + tag 拼接的结果
    sealed := gcm.Seal(nil, nonce, dek, aad)

    // 拆分为 ciphertext 和 tag
    ciphertext = sealed[:len(sealed)-DEKTagSize]
    tag = sealed[len(sealed)-DEKTagSize:]

    return nonce, ciphertext, tag, nil
}

// DecryptDEK 使用 KEK 通过 AES-256-GCM 解包（解密）DEK。
//
// 参数：
//   - nonce: 12 字节 Nonce
//   - ciphertext: 加密后的 DEK
//   - tag: 16 字节 GCM 认证标签
//   - kek: 密钥加密密钥（32 字节）
//   - aad: 关联数据（与加密时一致，可为 nil）
//
// 返回：
//   - []byte: 解密后的明文 DEK
//   - error: ErrDecryptDEKFailed 如果认证失败
func DecryptDEK(nonce, ciphertext, tag, kek, aad []byte) ([]byte, error) {
    block, err := aes.NewCipher(kek)
    if err != nil {
        return nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    // Open 需要 ciphertext 和 tag 拼接在一起
    sealed := make([]byte, len(ciphertext)+DEKTagSize)
    copy(sealed, ciphertext)
    copy(sealed[len(ciphertext):], tag)

    dek, err := gcm.Open(nil, nonce, sealed, aad)
    if err != nil {
        return nil, ErrDecryptDEKFailed
    }

    return dek, nil
}

// WrapDEKToStruct 是 EncryptDEK 的便利封装，直接返回 WrappedDEK 结构体。
func WrapDEKToStruct(dek, kek, aad []byte) (*types.WrappedDEK, error) {
    nonce, ciphertext, tag, err := EncryptDEK(dek, kek, aad)
    if err != nil {
        return nil, err
    }

    wd := &types.WrappedDEK{
        Algorithm:        types.WrappedDEKAlgAES256GCM,
        NonceBase64:      base64.StdEncoding.EncodeToString(nonce),
        CiphertextBase64: base64.StdEncoding.EncodeToString(ciphertext),
        TagBase64:        base64.StdEncoding.EncodeToString(tag),
    }
    if len(aad) > 0 {
        wd.AADBase64 = base64.StdEncoding.EncodeToString(aad)
    }
    return wd, nil
}

// UnwrapDEKFromStruct 是 DecryptDEK 的便利封装，从 WrappedDEK 结构体解密得到 DEK。
func UnwrapDEKFromStruct(wd *types.WrappedDEK, kek []byte) ([]byte, error) {
    if wd == nil || wd.Algorithm != types.WrappedDEKAlgAES256GCM {
        return nil, ErrInvalidWrappedDEK
    }

    nonce, err := base64.StdEncoding.DecodeString(wd.NonceBase64)
    if err != nil {
        return nil, err
    }
    ciphertext, err := base64.StdEncoding.DecodeString(wd.CiphertextBase64)
    if err != nil {
        return nil, err
    }
    tag, err := base64.StdEncoding.DecodeString(wd.TagBase64)
    if err != nil {
        return nil, err
    }
    var aad []byte
    if wd.AADBase64 != "" {
        aad, err = base64.StdEncoding.DecodeString(wd.AADBase64)
        if err != nil {
            return nil, err
        }
    }

    return DecryptDEK(nonce, ciphertext, tag, kek, aad)
}
```

### 2.2 KEK 派生函数

```go
// internal/v2/crypto/kek_derivation.go
package crypto

import (
    "crypto/sha256"

    "golang.org/x/crypto/pbkdf2"
)

const (
    // KEKKeySize KEK 固定使用 AES-256（32 字节）。
    KEKKeySize = 32

    // KDFIterations KEK 派生的 PBKDF2 迭代次数。
    // 可考虑从 100k 提升到 200k（待确认）。
    KDFIterations = 100000
)

// DeriveKEK 从用户密码和盐派生 KEK（密钥加密密钥）。
//
// 算法：PBKDF2-SHA256
// 输出长度：固定 32 字节（AES-256-GCM 需要）
//
// 注意：KEK 不直接加密数据，只用于包裹/解包 DEK。
func DeriveKEK(password string, salt []byte) []byte {
    return pbkdf2.Key([]byte(password), salt, KDFIterations, KEKKeySize, sha256.New)
}
```

## 三、Reader 路径改动

### 3.1 密钥解析流程变更

**当前流程**：
```
读取 Header → 读取 Manifest → 从 KVI 取 salt →
PBKDF2(password, salt) → 直接得到 Data Key → 解密数据
```

**新流程**：
```
读取 Header → 读取 Manifest → 检查 HasWrappedDEK()
  ├── 有 WrappedDEK（新格式）：
  │     ├── 从 KVI 取 salt → DeriveKEK(password, salt) → KEK
  │     └── UnwrapDEKFromStruct(wrapped_dek, KEK) → DEK（即 Data Key）
  └── 无 WrappedDEK（旧格式，向后兼容）：
        └── 从 KVI 取 salt → PBKDF2(password, salt) → 直接得到 Data Key
```

### 3.2 关键代码改动点

`internal/v2/reader/remote_decrypt_reader_factory.go` 或类似位置：

```go
// resolveDataKey 根据容器版本和是否有 WrappedDEK 解析出最终的数据加密密钥。
func resolveDataKey(password string, manifest *types.Manifest_v4, kvi *types.KVI) ([]byte, []byte, error) {
    salt, err := base64.StdEncoding.DecodeString(kvi.SaltBase64)
    if err != nil {
        return nil, nil, err
    }

    // 新格式：有 WrappedDEK → 分层密钥架构
    if manifest.HasWrappedDEK() {
        // 1. 派生 KEK
        kek := crypto.DeriveKEK(password, salt)

        // 2. 解包得到 DEK（即 Data Key）
        dek, err := crypto.UnwrapDEKFromStruct(manifest.WrappedDEK, kek)
        if err != nil {
            // GCM 认证失败 = 密码错误
            return nil, nil, types.ErrWrongPassword
        }

        // MAC Key 还是独立派生（与当前架构一致）
        macSalt := getMACSalt(manifest)
        macKey := crypto.DeriveMACKey(password, macSalt)

        return dek, macKey, nil
    }

    // 旧格式：无 WrappedDEK → 单层密钥派生（向后兼容）
    keyLen := crypto.KeySize_v4_128 // 默认值，实际从 Header.CipherMode 取
    // ... 旧逻辑 ...
}
```

## 四、Writer 路径改动

### 4.1 新增配置选项

```go
// internal/v2/writer/container_writer_v4.go

type V4WriterConfig struct {
    // ... 现有配置 ...

    // UseHierarchicalKeys 是否使用分层密钥架构。
    // true  → 生成随机 DEK，用 KEK 包络后存 Manifest（推荐，支持换密码）
    // false → 旧版单层密钥派生（向后兼容，不推荐用于新容器）
    UseHierarchicalKeys bool
}
```

### 4.2 写入流程变更

**当前流程**：
```
生成 salt → PBKDF2(password, salt) → Data Key →
加密 Segment → 写 Manifest（不含 WrappedDEK）
```

**新流程**：
```
生成 salt → DeriveKEK(password, salt) → KEK →
生成随机 DEK → WrapDEKToStruct(DEK, KEK) → WrappedDEK →
用 DEK 加密 Segment → 写 Manifest（含 WrappedDEK）
```

## 五、换密码 API 设计

### 5.1 接口定义

```go
// internal/v2/service/password_service.go
package service

// ChangePassword 修改加密容器的主密码。
//
// 分层密钥架构下，此操作只需重新包络 DEK，不需要重加密数据。
// 时间复杂度：O(1)，与文件大小无关。
//
// 参数：
//   - filePath: 容器文件路径
//   - oldPassword: 当前密码
//   - newPassword: 新密码
//
// 返回：
//   - error: nil 表示成功；ErrWrongPassword 表示旧密码错误
func ChangePassword(filePath, oldPassword, newPassword string) error {
    // 1. 打开容器（只读 header + manifest）
    // 2. 验证旧密码（解密 DEK）
    // 3. 用新密码派生新 KEK
    // 4. 重新包络 DEK（生成新 Nonce）
    // 5. 原子写入新 Manifest
}
```

### 5.2 原子写入保证

为防止中途断电导致数据损坏，写入采用「临时文件 + rename」模式：

```
1. 读取旧 Manifest
2. 生成新 WrappedDEK
3. 构造新 Manifest
4. 写入到临时文件 container.tmp
5. fsync 临时文件
6. rename 临时文件 → 原文件（原子操作）
```

## 六、测试设计

### 6.1 单元测试

| 测试用例 | 说明 |
|---------|------|
| TestGenerateDEK_Length | 验证生成的 DEK 长度正确 |
| TestGenerateDEK_Randomness | 两次生成的 DEK 不相同（基本随机性验证） |
| TestEncryptDecryptDEK_RoundTrip | 加密 → 解密，得到原始 DEK |
| TestEncryptDecryptDEK_WrongKey | 用错误的 KEK 解密，返回 ErrDecryptDEKFailed |
| TestEncryptDecryptDEK_WrongTag | 篡改 tag，返回 ErrDecryptDEKFailed |
| TestWrapUnwrapDEKToStruct_RoundTrip | 结构体封装 → 解封，得到原始 DEK |
| TestHasWrappedDEK_Nil | nil WrappedDEK 返回 false |
| TestHasWrappedDEK_Empty | 空 algorithm 返回 false |
| TestHasWrappedDEK_Valid | 有效 WrappedDEK 返回 true |

### 6.2 集成测试

| 测试用例 | 说明 |
|---------|------|
| TestV4Writer_HierarchicalKeys | 用分层架构写入，再读取验证内容一致 |
| TestV4Reader_LegacyFormat | 旧格式（无 WrappedDEK）容器能正常读取 |
| TestChangePassword_RoundTrip | 写 → 换密码 → 用新密码读，内容一致 |
| TestChangePassword_WrongOldPassword | 旧密码错误时返回 ErrWrongPassword |
| TestChangePassword_AfterRead | 换密码前后读取的数据完全一致 |

### 6.3 兼容性测试

| 测试用例 | 说明 |
|---------|------|
| TestLegacyReader_NewFormat | 旧版 Reader（不识别 WrappedDEK）读取新格式容器的行为 |
| TestNewReader_LegacyFormat | 新版 Reader 读取旧格式容器（向后兼容） |

## 七、待确认问题

1. **KEK 迭代次数**：保持 100,000 还是提升到 200,000？
2. **默认密钥长度**：DEK 默认 16 字节（AES-128）还是 32 字节（AES-256）？
3. **AAD 绑定**：是否需要绑定容器 ID / Magic 等作为 AAD？
4. **旧容器升级**：是否需要提供 `UpgradeToHierarchicalKeys()` 工具函数？
5. **多密码支持**：第一期就做 `wrapped_deks` 数组，还是先做单个 `wrapped_dek`？
