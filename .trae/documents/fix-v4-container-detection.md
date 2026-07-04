# V4 容器识别失败排查与修复计划

## Why

**移动端长按菜单中，V4 加密容器被当作普通文件处理（显示"加密"而非"解密/预览"），而 V3 容器正常。**

用户报告：v3 容器可以正常识别和预览，v4 不行。

---

## 根因分析（Root Cause）

### 调用链路

```
移动端 Files.vue 长按文件
  → getFileCategory(name, file.isEncrypted)  // L397
    → if (isEncrypted) return 'encrypted'     // L398 ← 关键！依赖后端 isEncrypted 字段
      → category === 'encrypted'
        → 显示: 预览 / 解密 / 删除           // L488-513 ✅ 正确路径
        → 不显示: 加密                        // V4 容器走到了 else 分支（L514）❌
```

### 后端检测链路

`ListFiles` (mobile_service.go:L134-138) 和 `GetFileInfo` (L292-294) 都使用同一个检测：

```go
if _, detectErr := detector.DetectContainer(entryAbsPath); detectErr == nil {
    isEncrypted = true  // ← V4 在这里返回了 error，所以 isEncrypted=false
}
```

`DetectContainer` → `containerhandle.Open(src)` → `openV4()` → **在 manifest 解码步骤失败**

### 🔴 BUG 根因：V4 Writer 与 Reader 的 Manifest 编码不匹配

**写入端** ([single_file_container_writer.go:L137-L174](file:///workspace/internal/v2/writer/single_file_container_writer.go#L137-L174)):

```go
func (w *SingleFileContainerWriter) WriteManifest(manifestObj *types.Manifest) error {
    manifestBytes, _ := manifestObj.SerializeToJSON()
    // ❌ 使用了 V3 的 AES-CBC 加密！
    encryptedManifestBytes, _ := manifest.EncryptManifest(manifestBytes)  // → EncryptSystemPayload (AES)
    block.WriteBlock(w.file, types.BlockTypeManifest_v2, encryptedManifestBytes)
}
```

**读取端** ([handle.go:L115-L128](file:///internal/v2/container/handle/handle.go#L115-L128)):

```go
func (h *containerHandle) openV4() error {
    obfuscatedManifest := make([]byte, hdr.ManifestLength)
    h.source.ReadAt(obfuscatedManifest, int64(hdr.ManifestOffset))
    // ✅ 期望 V4 的 XOR 混淆！
    plainManifest, _ := crypto.DeobfuscateManifest(obfuscatedManifest)  // → XOR deobfuscation
    v4Manifest, _ := types.DeserializeManifest_v4(plainManifest)
}
```

| | 写入 (SingleFileContainerWriterV4) | 读取 (openV4) |
|---|---|---|
| **编码方式** | `EncryptSystemPayload` = AES-CBC + 随机IV + 系统密钥 | `DeobfuscateManifest` = XOR + 盐前缀 |
| **数据格式** | BlockHeader(12B) + AES密文 | 原始 XOR 混淆数据 |
| **结果** | ❌ **完全不兼容** | |

**对比**: 直接的 `WriteV4Container` ([container_writer_v4.go:L120](file:///workspace/internal/v2/writer/container_writer_v4.go#L120)) 使用的是正确的 `crypto.ObfuscateManifest()`，所以通过那个路径写的 V4 容器可以正常读取。但插件加密流程走的是 `SinglePhysicalPacker` → `SingleFileContainerWriterV4` → `WriteManifest()` 这个错误路径。

---

## What Changes

### Fix 1: **[CRITICAL]** SingleFileContainerWriterV4.WriteManifest 使用正确的 V4 编码

**文件**: `/workspace/internal/v2/writer/single_file_container_writer.go`

**修改**: `WriteManifest()` 方法中，当 `headerVersion == 4` 时使用 `ObfuscateManifest` + `SerializeToJSON_v4` 替代 `EncryptManifest` + `SerializeToJSON`：

```go
func (w *SingleFileContainerWriter) WriteManifest(manifestObj *types.Manifest) error {
    if w.headerVersion == 4 {
        return w.writeManifestV4(manifestObj)
    }
    return w.writeManifestV23(manifestObj)
}

func (w *SingleFileContainerWriter) writeManifestV4(manifestObj *types.Manifest) error {
    mf := AdaptV2ToV4Manifest(manifestObj, w.v4Header)
    manifestJSON, err := mf.SerializeToJSON_v4()
    // ... ObfuscateManifest → 直接写入文件（不包裹 Block header）
}

func (w *SingleFileContainerWriter) writeManifestV23(manifestObj *types.Manifest) error {
    // 现有逻辑：SerializeToJSON → EncryptManifest → block.WriteBlock(BlockTypeManifest_v2)
}
```

**注意**: V4 格式的 manifest 是直接写入文件的（无 Block header 包裹），所以 `ManifestOffset` 应指向混淆数据的起始位置（不需要跳过 Block header）。需要同步调整 `Close()` 中 `ManifestOffset` 的计算：

```go
// Close() 中 V4 分支：
if w.headerVersion == 4 {
    // V4: manifest 直接写在当前位置，无 Block header
    w.v4Header.ManifestOffset = uint64(manifestBlockStart)  // 不是 + GetBlockHeader_v2_Size()
    w.v4Header.ManifestLength = w.manifestLength
}
```

### Fix 2: openV4 增加 fallback 兼容性（防御性编程）

**文件**: `/workspace/internal/v2/container/handle/handle.go`

即使 Fix 1 修复了写入端，已有的错误 V4 容器仍存在于用户设备上。增加 fallback 尝试：

```go
func (h *containerHandle) openV4() error {
    // ... read header, footer ...
    
    obfuscatedManifest := make([]byte, hdr.ManifestLength)
    h.source.ReadAt(obfuscatedManifest, int64(hdr.ManifestOffset))
    
    // Primary: V4 XOR deobfuscation
    plainManifest, err := crypto.DeobfuscateManifest(obfuscatedManifest)
    if err != nil {
        // Fallback: try V2/V3 block format (for legacy containers written by buggy writer)
        plainManifest, err = h.tryReadManifestAsV2Block(obfuscatedManifest)
        if err != nil {
            return fmt.Errorf("failed to decode v4 manifest: %w", err)
        }
    }
    
    v4Manifest, err := types.DeserializeManifest_v4(plainManifest)
    // ...
}
```

### Fix 3: 补全端到端测试防止回归

新增测试覆盖完整链路：**加密 → 检测 → 识别**

---

## Impact

- **影响范围**: 所有通过插件加密流程产生的 V4 容器（即移动端加密的所有文件）
- **已有 V4 容器**: 需要重新加密才能被正确识别（或通过 Fix 2 的 fallback 兼容）
- **不影响**: V3 容器、通过 `WriteV4Container` 直接写的 V4 容器

---

## Tasks

### Task 1: 修复 SingleFileContainerWriterV4.WriteManifest

- [ ] 1.1 将 `WriteManifest` 按 headerVersion 分流为 `writeManifestV4` / `writeManifestV23`
- [ ] 1.2 `writeManifestV4`: SerializeToJSON_v4 → ObfuscateManifest → 直接写文件（无 Block header）
- [ ] 1.3 修正 `Close()` 中 V4 的 `ManifestOffset` 计算（去掉 `+ GetBlockHeader_v2_Size()`）
- [ ] 1.4 验证现有 V3 测试不受影响

### Task 2: openV4 增加 fallback 兼容旧格式

- [ ] 2.1 实现 `tryReadManifestAsV2Block` 方法（读 BlockHeader → DecryptSystemPayload → DeserializeFromJSON → AdaptV4ToV2→转回 V4）
- [ ] 2.2 Deobfuscate 失败时自动降级到 V2 block 解码
- [ ] 2.3 日志记录使用了 fallback 路径（方便追踪旧格式容器）

### Task 3: 补全 V4 端到端集成测试

- [ ] 3.1 TestV4_Roundtrip_Encrypt_Detect_Recognize — 用 TextPlugin 加密一个文件 → Verify ListFiles 返回 isEncrypted=true → Verify GetFileInfo 返回 IsEncvContainer=true
- [ ] 3.2 TestV4_GetFileInfo_ContainerFields — 验证 Container 字段包含 version/container_type/is_seekable/segment_count
- [ ] 3.3 TestV4_LongPressMenu_Category — 验证 getFileCategory(name, true) === 'encrypted'
- [ ] 3.4 TestV4_Fallback_LegacyFormat — 模拟旧的错误格式 V4 容器，验证 fallback 路径可正常打开

### Task 4: 全量迭代验证

- [ ] 4.1 `go test ./internal/... -count=1` 零失败
- [ ] 4.2 `go build ./...` 编译通过
- [ ] 4.3 手动验证场景：创建 V4 容器 → 文件列表显示加密标记 → 长按显示解密选项

## Task Dependencies

- Task 1 (修复 Writer) → Task 3 (E2E 测试依赖正确写入)
- Task 2 (Reader Fallback) → Task 4 (全量验证)
- Task 3 可与 Task 2 并行（但 Task 3.4 依赖 Task 2）
- Task 4 最后执行
