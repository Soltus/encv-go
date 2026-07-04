# 修复 ENVC 残留 + V4 预览失败 + 测试漏洞

## 问题诊断

### 问题1: 文档中 ENVC 残留（8处）
Go 代码已全部清理完毕，仅剩 `.md` 文档文件中有 8 处 "ENVC"：
- `/workspace/README_LLM.md:93` — 1处
- `/workspace/ENCV容器2.1架构.md:26,65` — 2处
- `/workspace/ENCV容器3.0架构.md:37,77,98,156,212` — 5处

### 问题2: V4 容器预览失败（根因确认）

**预览链路有两条，一条已修复、一条仍断裂：**

**链路A（已可用）— TextPlugin.Decrypt 路径：**
```
TextPlugin.Decrypt → containerManager.GetReadablePath → reader.NewDecryptReaderFactory
  → NewEncryptedContainerReaderFromFile → containerhandle.Open(src) → openV4() ✅
```
此路径经过 `containerhandle.Open()` → `openV4()`，已支持 V4 XOR 去混淆。

**链路B（断裂）— WebDAV 索引路径：**
```
WebDAV statFile → getIndexFromContainerPathWithCache → getIndexFromContainerPath()
  → manifest.ExtractManifest(fullPath)  ❌ 仅支持 V2/V3 Block 扫描！
```

**根因：[manifest_v2.go:ExtractManifest()](internal/v2/container/manifest/manifest_v2.go#L108-L149)**
- 该函数扫描文件寻找 `BlockTypeManifest_v2` 类型的 Block
- V4 容器的 manifest 是 **XOR 混淆后存储在 Header.ManifestOffset 指定位置**，没有 Block 头
- 因此对 V4 容器永远扫描不到 → 返回 error → WebDAV 无法获取 Index → 预览失败

**相关函数同样受影响：**
- [manifest_v2.go:ExtractKVI()](internal/v2/container/manifest/manifest_v2.go#L50-L105) — 同样只扫 V2/V3 Block
- [manifest_v2.go:ReadManifestFromFile()](internal/v2/container/manifest/manifest_v2.go#L182-L223) — 已明确拒绝 V4（L200-202 返回错误）
- [manifest_v2.go:ScanManifestFromFile()](internal/v2/container/manifest/manifest_v2.go#L227-L272) — 同样只扫 V2/V3 Block

**validateContainerHeader 不受影响：** [fs_v2.go:580-593](internal/webdav/fs_v2.go#L580-L593)
- 使用 `DetectHeaderInfoFromReaderAt`，该函数正确识别 V4（version=4, headerSize=2048）
- 判断条件 `version != 0 && headerSize > 0` 对 V4 也成立 ✅

### 问题3: 测试覆盖空白
- `manifest` 包无任何测试
- WebDAV 无 V4 容器测试
- `ExtractManifest` 无 V4 分支测试

---

## 修复计划

### Task 1: 清理文档中的 ENVC 残留（8处）

**文件清单与修改：**

| 文件 | 行号 | 修改 |
|------|------|------|
| `README_LLM.md` | L93 | `"ENVC"` → `"ENCV"` |
| `ENCV容器2.1架构.md` | L26 | `"ENVC"` → `"ENCV"` |
| `ENCV容器2.1架构.md` | L65 | `"ENVC"` → `"ENCV"` |
| `ENCV容器3.0架构.md` | L37 | `"ENVC"` → `"ENCV"` |
| `ENCV容器3.0架构.md` | L77 | `"ENVC"` → `"ENCV"` |
| `ENCV容器3.0架构.md` | L98 | `"ENVC"` → `"ENCV"` |
| `ENCV容器3.0架构.md` | L156 | `"ENVC"` → `"ENCV"` |
| `ENCV容器3.0架构.md` | L212 | `"ENVC"` → `"ENCV"` |

### Task 2: 修复 ExtractManifest 支持 V4 容器

**修改文件:** [`internal/v2/container/manifest/manifest_v2.go`](internal/v2/container/manifest/manifest_v2.go)

**修改 `ExtractManifest()` 函数（L108-149）：**

在现有 V2/V3 Block 扫描逻辑之前，增加版本检测和 V4 分支：

```go
func ExtractManifest(containerPath string) ([]byte, error) {
    file, err := os.Open(containerPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open file '%s': %w", containerPath, err)
    }
    defer file.Close()

    // 1. 探测 Header 版本
    version, headerSize, err := types.DetectHeaderInfoFromReaderAt(file)
    if err != nil {
        return nil, fmt.Errorf("failed to detect header size: %w", err)
    }

    // 2. V4 分支：从 Header.ManifestOffset 读取 XOR 混淆的 Manifest
    if version == 4 {
        return extractManifestV4(file, headerSize)
    }

    // 3. V2/V3 分支：原有 Block 扫描逻辑（不变）
    if _, err := file.Seek(headerSize, io.SeekStart); err != nil {
        return nil, fmt.Errorf("failed to seek to data stream start: %w", err)
    }
    // ... 原 for 循环扫描 BlockTypeManifest_v2 ...
}
```

**新增内部函数 `extractManifestV4()`：**

```go
func extractManifestV4(file *os.File, headerSize int64) ([]byte, error) {
    // 1. 回到文件开头读取完整 V4 Header
    if _, err := file.Seek(0, io.SeekStart); err != nil {
        return nil, fmt.Errorf("failed to seek to start: %w", err)
    }
    hdr, err := types.ReadHeaderV4(file)
    if err != nil {
        return nil, fmt.Errorf("failed to read v4 header: %w", err)
    }

    // 2. 从 ManifestOffset 读取混淆数据
    if hdr.ManifestOffset == 0 || hdr.ManifestLength == 0 {
        return nil, fmt.Errorf("v4 header has invalid manifest offset/length")
    }
    obfuscated := make([]byte, hdr.ManifestLength)
    if _, err := file.ReadAt(obfuscated, int64(hdr.ManifestOffset)); err != nil {
        return nil, fmt.Errorf("failed to read v4 manifest data: %w", err)
    }

    // 3. XOR 去混淆
    plainData, err := crypto.DeobfuscateManifest(obfuscated)
    if err != nil {
        return nil, fmt.Errorf("failed to deobfuscate v4 manifest: %w", err)
    }

    return plainData, nil
}
```

**同步修改 `ExtractKVI()` 函数（L50-105）：**
- 同样在 Block 扫描前增加版本检测
- V4 容器的 KVI 存储在 Manifest JSON 中，先 ExtractManifest 再解析 KVI
- 或者：V4 的 KVI 同样通过 extractManifestV4 → DeserializeFromJSON → 提取 KVI 字段

**方案选择：** `ExtractKVI` 对于 V4 走 `extractManifestV4` + JSON 解析 KVI 字段的路径。

### Task 3: 修复 ReadManifestFromFile / ScanManifestFromFile 支持 V4

**修改文件:** [`internal/v2/container/manifest/manifest_v2.go`](internal/v2/container/manifest/manifest_v2.go)

**`ReadManifestFromFile()`（L182-223）：**
- 当前 L200-202 对 V4 直接返回 error
- 改为：V4 时调用 `extractManifestV4()` + `DeserializeFromJSON()` 返回 V2 兼容的 Manifest

**`ScanManifestFromFile()`（L227-272）：**
- 当前只扫 V2/V3 Block
- 增加 V4 分支：版本检测 → `extractManifestV4()` → `DeserializeFromJSON()` → 返回

### Task 4: 补全 manifest 包单元测试

**新建文件:** `internal/v2/container/manifest/manifest_test.go`

测试用例：

| 测试名 | 描述 |
|--------|------|
| `TestExtractManifest_V2` | 用 testutil.CreateV3Fixture 创建 V2 容器，验证 ExtractManifest 返回合法 JSON |
| `TestExtractManifest_V3` | 用 testutil.CreateV3Fixture 创建 V3 容器，验证同上 |
| `TestExtractManifest_V4` | 用 testutil.CreateV4Fixture 创建 V4 容器，验证 ExtractManifest 返回合法 JSON（核心回归测试） |
| `TestExtractManifest_V4_Deobfuscation` | 验证返回的数据是正确的去混淆后 JSON（对比原始 Manifest） |
| `TestExtractManifest_NotContainer` | 对非容器文件调用，验证返回明确错误 |
| `TestExtractKVI_V4` | V4 容器提取 KVI 数据 |
| `TestReadManifestFromFile_V4` | V4 容器通过 ReadManifestFromFile 读取 |
| `TestScanManifestFromFile_V4` | V4 容器通过 ScanManifestFromFile 读取 |

使用 `testutil.CreateV3Fixture` / `CreateV4Fixture` 创建临时容器文件进行测试。

### Task 5: 补全 WebDAV V4 预览路径集成测试

**新建/修改文件:** `internal/webdav/fs_v2_test.go`（如不存在则创建）

测试用例：

| 测试名 | 描述 |
|--------|------|
| `TestValidateContainerHeader_V4` | V4 容器通过 validateContainerHeader 验证 |
| `TestGetIndexFromContainerPath_V4` | V4 容器通过 getIndexFromContainerPath 获取 Index（核心回归测试） |
| `TestStatFile_V4_Container` | V4 容器通过 statFile 返回虚拟 FileInfo |
| `TestFullV4PreviewChain` | 端到端：创建 V4 容器 → WebDAV Stat → OpenFile → Read → 验证解密内容 |

### Task 6: 全量测试验证

```bash
go test ./internal/... -count=1 -v 2>&1 | tail -50
```

确保：
- 所有既有测试仍然 PASS
- 新增测试全部 PASS
- 零 FAIL/PANIC/SKIP（SKIP 仅限需要特殊环境的测试）

---

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| ExtractManifest 增加 V4 分支影响现有 V2/V3 调用者 | 低 | V4 分支通过 version==4 守卫，V2/V3 走原逻辑不变 |
| DeobfuscateManifest 依赖 crypto 包循环导入 | 无 | manifest 已依赖 crypto（EncryptManifest/DecryptManifest） |
| 测试 fixture 创建的 V4 容器格式需与生产一致 | 中 | 复用 testutil.CreateV4Fixture，它使用 SingleFileContainerWriterV4 生产代码 |
