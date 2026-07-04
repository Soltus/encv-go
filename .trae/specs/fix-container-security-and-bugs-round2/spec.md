# 加密容器 Bug 二轮修复 Spec

## Why

上一轮修复（fix-container-security-and-bugs）完成后，用户测试发现 4 个遗留问题：

1. **新建文件夹功能不工作**：FilePickerModal 的 + 按钮点击后目录列表变空白，无任何实际效果
2. **v4 容器加密误报新错误**：从 `size mismatch` 变为 `stsz box missing`（QuickStructCheck 在重编码文件上过于严格）
3. **v3 容器加密真实失败**：`failed to create temp file for MP4 remuxing: open .../.encv_verify_.../encv-pre-*.mp4: no such file or directory`（临时目录未创建）
4. **v4 容器信息乱码显示**：版本、容器ID、清单数据在前端显示为乱码
5. **Mock 测试覆盖不足**：上述 Bug 均未在测试中暴露，且缺少关键路径的 mock 测试
6. **任务卡片缺少 Warning 显示**：验证降级（SkipSizeCheck/SkipStructCheck）产生的 warning 无法在任务卡片上展示，目前只支持 error 显示

## What Changes

- **修复** FilePickerModal 新建文件夹的渲染逻辑和交互流程（overlay 替代 v-if/v-else 互斥）
- **修复** content_verifier 的 QuickStructCheck 对重编码文件的宽容度（新增 SkipStructCheck）
- **修复** content_preprocessor 中临时文件创建前的目录检查（MkdirAll 防御）
- **修复** v4 容器信息 API 返回数据的编码问题
- **新增** 任务卡片 Warning 状态显示支持（EncvTask 接口 + Tasks.vue + 后端）
- **新增** 完整的 Mock 测试套件（FilePickerModal、加密 E2E、容器信息、任务状态）

## Impact

- Affected specs: fix-container-security-and-bugs（本轮为续修）
- Affected code:
  - `app/encv-mobile/src/components/FilePickerModal.vue` — 新建文件夹 UI 修复
  - `internal/v2/plugins/video/content_verifier.go` — stsz check 宽容 + VerifyWarning 返回
  - `internal/v2/plugins/video/content_preprocessor.go` — MkdirAll 防御
  - `internal/service/mobile_service.go` — 容器信息编码修复
  - `app/encv-mobile/src/api/encv.ts` — EncvTask 新增 warning 字段
  - `app/encv-mobile/src/views/Tasks.vue` — 任务卡片 warning 展示
  - `internal/service/task_manager.go` — 任务完成时携带 warning 信息
  - 新增测试文件

---

## ADDED Requirements

### Requirement: FilePickerModal 新建文件夹功能修复

系统 SHALL 提供可靠的新建文件夹交互。

#### Scenario: 点击 + 按钮显示输入框并保留文件列表背景
- **WHEN** 用户在 folder 模式下点击 + 按钮
- **THEN** 输入框以 overlay 形式显示在文件列表上方（不替换文件列表）
- **AND** 文件列表保持可见（可半透明或正常显示）

#### Scenario: 确认创建后刷新并进入新目录
- **WHEN** 用户输入名称并确认
- **THEN** 调用 createDirectory API → 成功后 navigateTo 到新路径 → 文件列表刷新
- **AND** 输入框自动隐藏

#### Scenario: 取消创建恢复原状态
- **WHEN** 用户点击取消或按返回键
- **THEN** 输入框隐藏，文件列表恢复正常显示

### Requirement: QuickStructCheck 重编码宽容模式

Verify 方法 SHALL 在检测到重编码源时跳过严格的 MP4 结构检查。

#### Scenario: 重编码后的 MP4 缺少 stsz box
- **WHEN** 解密输出是经过 FFmpeg/MediaCodec 重编码的 MP4 文件（可能缺少 stsz/moov 等 box）
- **THEN** QuickStructCheck 不应报 `stsz box missing` 错误
- **AND** 应返回 VerifyWarning（非 error），该 warning 被传递到任务卡片的 warning 字段

#### Scenario: VerifyWarning 数据结构
- **WHEN** 验证器决定降级某个检查项为 warning
- **THEN** 返回的 VerifyWarning 包含：`checkName`（如 "quick_struct_check"）、`message`（如 "stsz box missing, output may be re-encoded"）、`severity`（"warning"）
- **AND** VerifyOptions 新增 `CollectWarnings bool` 字段控制是否收集 warnings

### Requirement: 临时文件创建防御性 MkdirAll

content_preprocessor SHALL 在调用 `os.CreateTemp` 前确保目标目录存在。

#### Scenario: outputDir 不存在时创建临时文件
- **WHEN** `os.CreateTemp(p.outputDir, ...)` 被调用但 p.outputDir 不存在
- **THEN** 不应报 `no such file or directory`
- **AND** 应先执行 `os.MkdirAll(p.outputDir, 0755)` 后重试

### Requirement: v4 容器信息正确编码显示

后端 API 返回的容器元数据 SHALL 使用 UTF-8 编码且前端正确解析。

#### Scenario: 查看 v4 加密文件的容器信息
- **WHEN** 用户在 FileInfo 或 FilePreview 页面查看 v4 容器文件
- **THEN** version 显示为数字（如 4）、container_id 显示为有效 UUID 字符串、manifest 显示为格式化 JSON
- **AND** 不出现乱码、二进制数据或空值

### Requirement: 任务卡片 Warning 显示支持

系统 SHALL 支持在任务卡片上展示验证 warning 信息。

#### Scenario: 加密任务完成但带有验证 warning
- **WHEN** 加密成功完成但 PostEncryptProcessor 的 verifyContainer 产生了 VerifyWarning（如跳过了 size check 或 struct check）
- **THEN** 任务状态为 `completed`（不是 failed）
- **AND** 任务卡片上显示 warning 提示（黄色/橙色 badge 或图标 + warning 文本）
- **AND** 用户可展开查看 warning 详情

#### Scenario: EncvTask 接口扩展
- **WHEN** 后端返回任务数据
- **THEN** EncvTask 接口包含新增字段 `warning?: string` 和 `warningDetail?: string`

#### Scenario: Tasks.vue warning 渲染
- **WHEN** task.warning 存在
- **THEN** 在 task-error 区域上方或旁边显示 warning 样式（不同颜色，如 warning 色的 ion-icon + 文本）
- **AND** warning 可独立于 error 展开/折叠详情

### Requirement: 关键路径 Mock 测试覆盖

系统 SHALL 为以下关键路径提供全面的 mock/integration 测试：

#### FilePickerModal 测试
- 点击 + 显示输入框（overlay 模式，文件列表不消失）
- 输入名称 + 确认 → mock createDirectory API → navigateTo → loadFiles 刷新
- 取消操作 → 输入框隐藏，文件列表恢复
- 空名称提交被拦截
- API 失败（403/400/网络）→ alert 显示错误信息
- 特殊字符名称处理（中文、空格、特殊字符）

#### 加密流程 E2E 测试
- v3 不重编码加密完整流程（Preprocess → Encrypt → PostEncrypt → verifyContainer）
- v4 重编码加密完整流程（含 SkipSizeCheck + SkipStructCheck 验证通过）
- v4 重编码加密产生 VerifyWarning 并正确传递到任务结果
- outputDir 不存在时 Preprocess 自动 MkdirAll 并成功
- ffprobe 输出含 BOM/尾随逗号时的容错解析

#### 容器信息 API 测试
- v3 容器 /api/file/info 返回结构正确的 JSON
- v4 容器 container_id 为有效字符串（UUID 格式）
- v4 容器 manifest 可被 JSON.parse 正确解析（无乱码）
- v4 容器 Segments 数据不含二进制垃圾数据

#### 任务状态测试
- completed + warning 的任务在前端正确渲染 warning 样式
- completed + error 的任务仍显示为 error（回归保护）
- warningDetail 可展开/折叠

---

## MODIFIED Requirements

### Requirement: FilePickerModal.vue template 结构

当前模板使用 `v-if="showNewFolder"` (div) 与 `v-else` (ion-list) 互斥渲染。

**修改为：** new-folder-input 使用绝对定位 overlay，ion-list 始终渲染（不受 v-else 控制）。

### Requirement: VideoContentVerifier.Verify

上一轮已添加 SkipSizeCheck。本轮需：
1. 新增 `SkipStructCheck bool` 到 VerifyOptions
2. QuickStructCheck 接收 opts 参数，SkipStructCheck=true 时返回 []*VerifyWarning 而非 error
3. Verify 方法聚合所有 warnings 并可通过选项控制是否作为 error 返回

### Requirement: VerifyOptions

```go
type VerifyOptions struct {
    SkipSizeCheck    bool
    SkipStructCheck  bool
    CollectWarnings  bool  // 新增：收集而非忽略 warnings
}
```

### Requirement: EncvTask 接口

```typescript
export interface EncvTask {
  // ... 现有字段
  warning?: string        // 新增：警告摘要
  warningDetail?: string  // 新增：警告详情
}
```

### Requirement: TaskStatus 类型

考虑新增 `'completed_with_warnings'` 状态或在现有 `completed` 状态下通过 `warning` 字段区分。

---

## REMOVED REQUIREMENTS

（无）
