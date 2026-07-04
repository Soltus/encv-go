# 加密容器预览修复 + V4 插件迁移计划

## 问题分析

### 1. 为什么 V4 容器开发遗漏这么多？

V4 容器格式虽然定义了完整的 Header（`EnvelopeHeaderV4` 包含 `ContainerType`、`IsSeekable`、`ManifestOffset`、`ManifestLength`），但实际写入路径存在断层：

- **V4 写入器已实现**：`container_writer_v4.go` 的 `WriteV4Container()` 完整支持 V4 格式
- **Plugin 接口已声明 V4 方法**：`ContainerType()`、`DefaultIsSeekable()`、`DisasterZones()` 已在 Plugin 接口中定义
- **但所有 6 个插件仍使用 V3 写入路径**：`HeaderVersion: 3` → `SinglePhysicalPacker` → `SingleFileContainerWriter`（只写 V3 Header）
- **PackParams/PackRequest 缺少 V4 字段**：`ContainerType`、`IsSeekable` 未传递到写入层
- **SinglePhysicalPacker 只支持 V3**：`Pack()` 方法只处理 `HeaderVersion == 3` 的情况

**根本原因**：V4 格式是在 V3 基础上新增的，但只实现了 V4 专用写入器（`WriteV4Container`），没有把 V4 支持集成到通用写入路径（`SinglePhysicalPacker` → `SingleFileContainerWriter`）。插件接口声明了 V4 方法但加密流程没有调用它们。

### 2. logcat.txt.sccgt 为什么无法预览？

`logcat.txt.sccgt` 是文本插件加密的 V3 容器。当前 `FilePreview.vue` 的加密分支逻辑：

```typescript
if (info.is_encv_container && info.container) {
  const containerType = info.container.container_type
  switch (containerType) {
    case 'image': ...
    case 'video': case 'audio': ... router.push('/player')
    case 'document': ...
    default: previewType.value = 'container'  // ← 文本类型落到了这里
  }
}
```

问题：V3 容器的 `container_type` 由 `detector.DetectContainerType()` 返回，文本类型返回 `ContainerTypeText = 5`，在 `mobile_service.go` 中映射为 `"text"`。但前端 switch 只处理了 `image/video/audio/document`，**缺少 `text` 分支**，导致文本容器落入 `default` 显示容器元数据卡片而非文本内容。

### 3. 所有插件适配 V4 工作量大吗？

**工作量不大**，因为基础设施已经就绪：

| 组件 | 状态 | 需要改动 |
|------|------|----------|
| V4 Header 类型 | ✅ 已完成 | 无 |
| V4 写入器 | ✅ 已完成 | 无 |
| Plugin 接口 V4 方法 | ✅ 已声明 | 无 |
| PackParams | ❌ 缺字段 | 添加 `ContainerType`、`IsSeekable` |
| PackRequest | ❌ 缺字段 | 添加 `ContainerType`、`IsSeekable` |
| SinglePhysicalPacker | ❌ 只支持 V3 | 添加 V4 分支 |
| 6 个插件 PostEncryptProcessor | ❌ HeaderVersion=3 | 改为 4 + 传入 ContainerType/IsSeekable |

**视频插件**改动最小——只需改 `HeaderVersion: 3` → `4`，加上 `ContainerType: types.ContainerTypeVideo`、`IsSeekable: true`。视频插件已有 `ContainerType()` 和 `DefaultIsSeekable()` 方法，直接调用即可。

**总改动量**：约 7 个文件，每个文件改动 5-15 行。

---

## 实施步骤

### Step 1: 修复 FilePreview.vue 加密容器预览 + 通用信息卡片

**文件**: `app/encv-mobile/src/views/FilePreview.vue`

1. **添加 `text` 分支**到 switch 语句：`case 'text'` 与 `case 'document'` 合并处理
2. **添加文件通用信息卡片**：在加密容器预览（image/text/pdf）上方显示文件名、大小、修改时间、MIME、分类信息，复用 `FileInfo.vue` 的卡片样式
3. 从 `/api/file/info` 返回的 `info` 对象中提取通用信息（name, size, modified, mime_type, category）

### Step 2: PackParams/PackRequest 添加 V4 字段

**文件 1**: `internal/v2/plugins/interfaces/packer/packer_helper.go`
- `PackParams` 添加 `ContainerType uint16` 和 `IsSeekable bool`

**文件 2**: `internal/v2/physical/types.go`
- `PackRequest` 添加 `ContainerType uint16` 和 `IsSeekable bool`

**文件 3**: `internal/v2/plugins/interfaces/packer/packer_helper.go`
- `StandardPostEncrypt()` 中将 `params.ContainerType` 和 `params.IsSeekable` 传递到 `packReq`

### Step 3: SinglePhysicalPacker 支持 V4 写入

**文件**: `internal/v2/physical/file_single.go`

- 在 `Pack()` 方法中，当 `req.HeaderVersion == 4` 时：
  - 使用 `types.CreateHeaderV4()` 创建 V4 Header
  - 使用 `writer.NewSingleFileContainerWriterV4()` 或直接内联 V4 写入逻辑
  - 注意：V4 Header 大小 2048 字节 vs V3 Header 大小不同，需要适配

**关键决策**：V4 写入路径有两种选择：
- **方案 A**：在 `SingleFileContainerWriter` 中添加 V4 支持（修改现有 Writer）
- **方案 B**：创建新的 `SingleFileContainerWriterV4`，复用 `container_writer_v4.go` 的逻辑

推荐**方案 A**：修改 `SingleFileContainerWriter` 使其支持 V3/V4 双版本，因为 Writer 的核心逻辑（写 Fragment、写 Manifest、写 Footer）在 V3/V4 间高度相似，只是 Header 和 Footer 格式不同。

**文件**: `internal/v2/writer/single_file_container_writer.go`
- `NewSingleFileContainerWriter` 接受 V3 或 V4 Header
- `Close()` 根据 Header 版本写 V3 Footer 或 V4 Footer
- V4 Header 需要在 Close 时回填 `ManifestOffset` 和 `ManifestLength`

### Step 4: 所有插件迁移到 V4

6 个插件的 `PostEncryptProcessor` 方法统一修改：

| 插件 | HeaderVersion | ContainerType | IsSeekable |
|------|--------------|---------------|------------|
| video | 3→4 | `types.ContainerTypeVideo` (1) | `p.DefaultIsSeekable(p.inputPath)` |
| audio | 3→4 | `types.ContainerTypeAudio` (2) | false |
| image | 3→4 | `types.ContainerTypeImage` (3) | false |
| text | 3→4 | `types.ContainerTypeText` (5) | false |
| pdf | 3→4 | `types.ContainerTypeDocument` (4) | false |
| wps | 3→4 | `types.ContainerTypeDocument` (4) | false |

每个插件只需改 3 行：
```go
HeaderVersion:  4,  // 原来是 3
ContainerType:  p.ContainerType(),  // 新增
IsSeekable:     p.DefaultIsSeekable(p.inputPath),  // 新增
```

### Step 5: 验证

1. 重启后端 dev server
2. 测试 V3 容器（如 `logcat.txt.sccgt`）预览 → 应显示文本内容
3. 测试新加密的 V4 容器预览
4. 测试加密容器通用信息卡片显示
5. 运行前端构建验证 `vue-tsc --noEmit && vite build`
