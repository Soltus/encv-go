# 修复计划：加密容器预览 + V3 容器读取 + 通用信息卡片

## V4 容器迁移分析

### 为什么所有插件还在用 V3？

V4 header 结构（`EnvelopeHeaderV4`）相比 V3 新增了以下字段：
- `ContainerType uint16` — 容器类型（video=1, audio=2, image=3, document=4）
- `IsSeekable uint8` — 是否可寻址
- `ManifestOffset uint64` — Manifest 绝对偏移
- `ManifestLength uint64` — Manifest 长度

V3 header 没有这些字段，V3 的 Manifest 位置是通过扫描块头（block scan）找到的，而不是通过 header 中的偏移量直接定位。

**迁移到 V4 的工作量评估**：

| 插件 | 修改点 | 工作量 |
|------|--------|--------|
| video | `HeaderVersion: 3` → `4`，需设置 `ContainerType=1`, `IsSeekable=1` | 小 |
| audio | `HeaderVersion: 3` → `4`，需设置 `ContainerType=2`, `IsSeekable=0` | 小 |
| image | `HeaderVersion: 3` → `4`，需设置 `ContainerType=3`, `IsSeekable=0` | 小 |
| text | `HeaderVersion: 3` → `4`，需设置 `ContainerType=4`, `IsSeekable=0` | 小 |
| pdf | `HeaderVersion: 3` → `4`，需设置 `ContainerType=4`, `IsSeekable=0` | 小 |
| wps | `HeaderVersion: 3` → `4`，需设置 `ContainerType=4`, `IsSeekable=0` | 小 |

**关键**：`StandardPostEncrypt` → `PhysicalPacker.Pack` → `SingleFileContainerWriter` / `V4ContainerWriter` 已经根据 `HeaderVersion` 选择写入 V3 或 V4 header。所以**只需将 `HeaderVersion: 3` 改为 `HeaderVersion: 4`，并设置对应的 `ContainerType` 和 `IsSeekable`**。

但 `PackParams` / `PackRequest` 目前没有 `ContainerType` 和 `IsSeekable` 字段，需要添加。

### 推荐方案：双管齐下

1. **短期**：修改 `GetFileInfo` 支持 V3 容器读取（向后兼容已有容器）
2. **长期**：将所有插件迁移到 V4 header（新加密的容器使用 V4）

---

## 问题 1：加密容器预览缺少通用信息卡片

### 修复方案

在 `FilePreview.vue` 的加密容器分支中，添加文件基本信息显示（名称、大小、修改时间、MIME 类型等）。API 响应中已有这些字段。

---

## 问题 2：V3 容器无法被 GetFileInfo 读取

### 修复方案

修改 `mobile_service.go` 的 `GetFileInfo`：
1. 先尝试 `OpenV4Container`
2. V4 失败时，降级使用 `detector.DetectContainerType` + `detector.DetectIsSeekable`
3. 返回基本的 container_type、is_seekable 等字段

---

## 问题 3：所有插件迁移到 V4

### 修复方案

1. 在 `PackParams` / `PackRequest` 中添加 `ContainerType` 和 `IsSeekable` 字段
2. 在 `V4ContainerWriter` / `SingleFileContainerWriter` 中使用这些字段设置 V4 header
3. 将所有插件的 `HeaderVersion: 3` 改为 `HeaderVersion: 4`，并设置对应的 `ContainerType` 和 `IsSeekable`

---

## 执行步骤

### Step 1：修改 mobile_service.go 支持 V3 容器读取

### Step 2：修改 FilePreview.vue 显示通用信息卡片

### Step 3：将所有插件迁移到 V4 header

1. `PackParams` 添加 `ContainerType uint16` 和 `IsSeekable bool`
2. `PackRequest` 添加对应字段
3. `V4ContainerWriter` 使用这些字段
4. `SingleFileContainerWriter` 使用这些字段
5. 所有插件设置 `HeaderVersion: 4` + 对应的 `ContainerType` + `IsSeekable`

### Step 4：验证

1. 重启后端
2. 测试 V3 容器（logcat.txt.sccgt）的预览
3. 测试新加密的 V4 容器
4. 测试各种类型加密容器的预览

---

## 文件修改清单

| 文件 | 修改内容 |
|------|----------|
| `internal/service/mobile_service.go` | `GetFileInfo` V3 容器降级读取 |
| `internal/v2/plugins/interfaces/packer/packer_helper.go` | `PackParams` 添加 `ContainerType`、`IsSeekable` |
| `internal/v2/writer/single_file_container_writer.go` | 使用 V4 header，设置 ContainerType、IsSeekable |
| `internal/v2/writer/container_writer_v4.go` | 使用 PackParams 中的 ContainerType、IsSeekable |
| `internal/v2/plugins/video/plugin.go` | `HeaderVersion: 4`, `ContainerType: 1`, `IsSeekable: true` |
| `internal/v2/plugins/audio/plugin.go` | `HeaderVersion: 4`, `ContainerType: 2`, `IsSeekable: false` |
| `internal/v2/plugins/image/plugin.go` | `HeaderVersion: 4`, `ContainerType: 3`, `IsSeekable: false` |
| `internal/v2/plugins/text/plugin.go` | `HeaderVersion: 4`, `ContainerType: 4`, `IsSeekable: false` |
| `internal/v2/plugins/pdf/plugin.go` | `HeaderVersion: 4`, `ContainerType: 4`, `IsSeekable: false` |
| `internal/v2/plugins/wps/plugin.go` | `HeaderVersion: 4`, `ContainerType: 4`, `IsSeekable: false` |
| `src/views/FilePreview.vue` | 加密容器预览添加文件通用信息卡片 |
