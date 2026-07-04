# 修复计划：容器版本显示 + V4 检测 + 文本预览

## 问题 1：加密解密任务卡片增加容器版本显示

**修改**：

1. **`internal/service/task_manager.go`**：`MobileTask` 添加 `ContainerVersion int` 字段（`json:"containerVersion,omitempty"`）
2. **`internal/service/task_manager.go`**：`processEncrypt` 完成后，检测输出文件的 Header 版本，写入 `task.ContainerVersion`
3. **`app/encv-mobile/src/api/encv.ts`**：`EncvTask` 接口添加 `containerVersion?: number`
4. **`app/encv-mobile/src/views/Tasks.vue`**：任务卡片在 `completed-info` 区域显示容器版本（如 `V4`）

## 问题 2：新加密的 V4 容器无法识别

**根因**：`SingleFileContainerWriter.Close()` V4 分支中，`ManifestOffset` 指向 BlockHeader 的起始位置，但 `ManifestLength` 只是加密数据的长度（不含 BlockHeader 的 16 字节）。`OpenV4Container` 从 `ManifestOffset` 读 `ManifestLength` 字节，实际读到的是 BlockHeader + 部分加密数据，导致 `DeobfuscateManifest` 失败。

**修复**：让 `ManifestOffset` 指向加密数据开始（跳过 BlockHeader），`ManifestLength` 保持为加密数据长度。

1. **`internal/v2/writer/single_file_container_writer.go`**：V4 Close() 中 `ManifestOffset = manifestOffset + blockHeaderSize`
2. **`internal/v2/physical/file_chunker.go`**：V4 finalize 中同样修正

## 问题 3：文本预览复用桌面端逻辑

**桌面端 openlist 预览架构**：
- 文本文件：iframe 加载 `/_preview/text.html?file=xxx`
  - `text.html` 内置交互控件：主题切换（明/暗）、换行切换
  - JS fetch `/decrypt?file=xxx` → `response.text()` → 设置到 `<pre>` 元素
- PDF 文件：iframe 加载 `/_preview/pdf.html?file=xxx`
  - JS fetch `/decrypt?file=xxx` → `response.blob()` → `URL.createObjectURL` → 设置 iframe src

**移动端当前问题**：
- 文本预览用 `<iframe :src="streamUrl">` 直接加载 `/stream`，没有交互控件
- `/stream` 返回 Content-Type 为 `application/octet-stream`（加密文件），浏览器不渲染
- 普通文本（md、json）也卡在加载中

**修复方案**：复用桌面端预览页面

1. **`internal/server/server.go`**：添加 `/preview/*` 路由，提供 `web.PreviewHandler()` 的预览 HTML 页面
2. **`internal/server/server.go`**：添加 `/decrypt` 路由，等同于 `/stream` 但参数名用 `file`（兼容 `text.html` 的 fetch 逻辑）
3. **`internal/v2/handler/content.go`**：`ServeFile` 中，根据原始扩展名（去掉加密后缀）设置正确的 Content-Type
4. **`app/encv-mobile/src/views/FilePreview.vue`**：
   - 文本预览：iframe 加载 `/preview/text.html?file=xxx`（复用桌面端交互控件）
   - PDF 预览：iframe 加载 `/preview/pdf.html?file=xxx`（复用桌面端 PDF 渲染）
   - 加密文本同理：`/decrypt?file=xxx` 自动解密

---

## 实施步骤

### Step 1: 修复 V4 ManifestOffset/ManifestLength 不匹配

**文件**: `internal/v2/writer/single_file_container_writer.go`
- V4 Close() 中，`ManifestOffset` 加上 BlockHeader 大小偏移

**文件**: `internal/v2/physical/file_chunker.go`
- V4 finalize 中同样修正 ManifestOffset

### Step 2: 添加 /preview 和 /decrypt 路由

**文件**: `internal/server/server.go`
- 添加 `/preview/*filepath` 路由，提供 `web.PreviewHandler()` 预览页面
- 添加 `/decrypt` 路由，接受 `file` 参数，等同于 `/stream`（调用 `handleStreamRequest` 逻辑）

**文件**: `internal/v2/handler/content.go`
- 添加辅助函数：从加密容器文件名提取原始扩展名
- `ServeFile` 中用原始扩展名查 Content-Type

### Step 3: FilePreview.vue 复用桌面端预览页面

**文件**: `app/encv-mobile/src/views/FilePreview.vue`
- 文本预览：iframe 加载 `/preview/text.html?file=<path>`
- PDF 预览：iframe 加载 `/preview/pdf.html?file=<path>`
- 图片预览：保持 `<img>` 标签不变
- 加密文件同理（`/decrypt` 自动解密）

### Step 4: 任务卡片增加容器版本

**文件**: `internal/service/task_manager.go`
- `MobileTask` 添加 `ContainerVersion int`
- `processEncrypt` 完成后检测输出文件 Header 版本

**文件**: `app/encv-mobile/src/api/encv.ts`
- `EncvTask` 添加 `containerVersion?: number`

**文件**: `app/encv-mobile/src/views/Tasks.vue`
- completed-info 区域显示容器版本

### Step 5: 验证

- 前端构建 `vue-tsc --noEmit && vite build`
- 后端编译 `go build ./internal/... ./cmd/encv/...`
- 重启后端
- 测试新加密 V4 容器检测
- 测试普通文本（md、json）预览
- 测试加密文本预览
