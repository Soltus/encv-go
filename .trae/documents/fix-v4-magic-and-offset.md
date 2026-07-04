# V4 容器识别失败 + Magic Header 修正

## Why

上次修复**不完整**，V4 容器仍然无法被识别。Magic Header "ENVC" 也是错误的（应为 ENCV）。项目未投入生产，无需兼容旧格式。

---

## 🔴 Bug 1 (CRITICAL): Close() V4 ManifestOffset 偏移差 12 字节

**位置**: [single_file_container_writer.go:251](file:///workspace/internal/v2/writer/single_file_container_writer.go#L251)

V4 manifest 直接写入（无 Block header），但 `manifestBlockSize` 统一加了 `block.GetBlockHeader_v2_Size()`(12B) → `ManifestOffset` 比实际早 12B → reader 读到垃圾数据 → Deobfuscate 失败。

**修复**: V4 分支中 `manifestBlockStart = fileInfo.Size() - int64(len(w.manifestBytes))`

---

## 🔴 Bug 2: Magic "ENVC" → "ENCV"

**权威来源（唯一）**: [types/container.go:49-50](file:///workspace/internal/v2/types/container.go#L49-L50)

```go
MagicHeader_v2 = [4]byte{'E', 'N', 'V', 'C'}  // ❌ → {'E','N','C','V'}
MagicFooter_v2 = [4]byte{'E', 'N', 'V', 'C'}  // ❌ → {'E','N','C','V'}
```

全局引用情况：11 个生产文件 + 3 个测试文件，共 ~35 处，**全部通过常量引用**（非硬编码）。改一处定义即可全局生效。

唯一需要额外修复的硬编码：[detector_test.go:106](file:///workspace/internal/v2/container/detector/detector_test.go#L106) `[]byte("ENVC\x00")` → `[]byte("XXXX\x00")` 或其他非 magic 值。

---

## Tasks

### Task 1: 修复 Close() V4 ManifestOffset
- [ ] 1.1 `manifestBlockSize`/`manifestBlockStart` 按 `w.headerVersion` 分流计算
- [ ] 1.2 V4: 不加 GetBlockHeader_v2_Size()；V2/V3: 保持不变

### Task 2: Magic ENVC → ENCV（单一权威源修改）
- [ ] 2.1 types/container.go: MagicHeader_v2 / MagicFooter_v2 值改为 ENCV
- [ ] 2.2 detector_test.go:106 硬编码 "ENVC" 改为非 magic 值

### Task 3: 端到端验证测试
- [ ] 3.1 SingleFileContainerWriterV4 写入 → Open 成功（走插件路径）
- [ ] 3.2 detector.DetectContainer 对插件路径 V4 返回 non-nil
- [ ] 3.3 MobileService ListFiles/GetFileInfo 正确识别 V4
- [ ] 3.4 验证文件头 4 字节为 0x45 0x4E 0x43 0x56 ("ENCV")

### Task 4: 全量迭代验证
- [ ] 4.1 `go test ./internal/... -count=1` 零失败
