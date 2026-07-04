# 加密容器安全加固与 Bug 修复 Spec

## Why

当前系统存在三个需要解决的问题：

1. **安全性缺失**：缺少加密容器的伪造攻击测试，无法验证容器验证机制能否有效检测被篡改的容器文件（如修改 Header、篡改数据、伪造 CRC 等）。
2. **加密流程 Bug**：
   - **v4 容器误报**：使用 v4 容器加密视频后，预览和解密均正常，但加密后验证阶段报错 `container verification failed: size mismatch`。原因可能是重编码模式下解密输出与原始文件大小不一致时，验证器错误地对比了原始明文而非预处理后的中间文件。
   - **v3 容器 ffprobe 解析失败**：v3 容器加密时报错 `failed to unmarshal ffprobe data: invalid character '[' after array element`，表明 ffprobe 输出的 JSON 格式与预期结构不兼容（可能由某些特殊视频文件的流结构导致）。
3. **功能缺失**：解密时选择目标文件夹的 FilePickerModal 不支持新建文件夹操作，用户无法在选择路径时直接创建新目录。

## What Changes

- 新增容器伪造攻击测试用例，覆盖 Header 篡改、数据篡改、CRC 伪造、大小伪造等场景
- **修复** v4 容器验证逻辑：重编码模式下应对比预处理后文件（而非原始输入文件）与解密输出
- **修复** v3 容器 ffprobe JSON 解析：增强容错能力，处理非标准 ffprobe 输出格式
- **新增** 后端 `CreateDirectory` API 端点
- **新增** 前端 FilePickerModal 新建文件夹 UI 和交互逻辑

## Impact

- Affected specs:encv-v4-container-architecture（验证修复）
- Affected code:
  - `internal/v2/plugins/video/content_verifier.go` — 验证逻辑修复 + 安全测试
  - `internal/v2/plugins/video/metadata_extractor.go` — ffprobe JSON 解析容错
  - `internal/v2/plugins/video/plugin.go` — PostEncryptProcessor 验证路径修复
  - `internal/service/mobile_service.go` — 新增 CreateDirectory 方法
  - `app/encv-mobile/src/api/encv.ts` — 新增 createDirectory API 调用
  - `app/encv-mobile/src/components/FilePickerModal.vue` — 新建文件夹 UI

---

## ADDED Requirements

### Requirement: 容器伪造攻击测试

系统 SHALL 提供一套完整的容器伪造攻击测试用例，用于验证容器完整性检测机制的有效性。

#### Scenario: Header Magic 被篡改
- **WHEN** 攻击者修改容器文件的 Header Magic 字段（如将 "ENVC" 改为 "FAKE"）
- **THEN** 容器检测器 SHALL 拒绝打开该文件并返回格式错误

#### Scenario: 数据段被篡改
- **WHEN** 攻击者修改加密容器中的某个数据段内容（翻转随机字节）
- **THEN** 解密后验证器 SHALL 检测到数据完整性失败（hash mismatch 或结构损坏）

#### Scenario: Manifest 被篡改
- **WHEN** 攻击者修改容器中的 Manifest 内容（如修改 fragment offset 或 length）
- **THEN** 解密过程 SHALL 失败或验证器 SHALL 报告完整性错误

#### Scenario: 文件大小被伪造
- **WHEN** 攻击者在容器外追加垃圾数据使文件变大，或截断文件尾部使文件变小
- **THEN** 验证器 SHALL 通过大小校验或 CRC 校验检测到异常

#### Scenario: CRC32 被篡改
- **WHEN** 攻击者修改数据但不更新对应的 CRC32 校验值
- **THEN** 块级 CRC 校验 SHALL 失败

### Requirement: v4 容器验证路径修正

系统 SHALL 在 v4 容器的 PostEncryptProcessor 验证阶段，正确选择对比基准文件。

#### Scenario: 重编码模式下的验证
- **WHEN** 视频经过 FFmpeg/MediaCodec 重编码后再加密
- **THEN** 验证器 SHALL 对比重编码后的中间文件与解密输出，而非原始输入文件
- **AND** 如果无法获取中间文件路径， SHALL 跳过大小精确匹配检查，仅做结构完整性验证

#### Scenario: 不重新编码模式下的验证
- **WHEN** 视频使用"不重新编码"模式直接加密
- **THEN** 验证器行为保持不变（对比原始文件与解密输出）

### Requirement: ffprobe JSON 解析容错

系统 SHALL 增强 ffprobe 输出的 JSON 解析容错能力，处理非标准格式的 ffprobe 输出。

#### Scenario: 非标准 streams 数组格式
- **WHEN** ffprobe 返回的 JSON 中 streams 字段包含非预期的数组嵌套或特殊字符
- **THEN** 解析器 SHALL 尝试修复常见格式问题（如去除 BOM、处理特殊 Unicode、容忍尾随逗号）
- **AND** 如果修复失败， SHALL 返回明确的错误信息并建议降级方案

#### Scenario: ffprobe 输出截断
- **WHEN** ffprobe 进程异常终止导致 JSON 输出不完整
- **THEN** 解析器 SHALL 检测到 JSON 截断并返回 `ffprobe output truncated` 错误，而非 `invalid character` 底层错误

### Requirement: 新建文件夹功能

系统 SHALL 在文件夹选择界面提供新建文件夹的能力。

#### Scenario: 用户在解密目标路径选择时新建文件夹
- **WHEN** 用户在 FilePickerModal（folder 模式）中点击"新建文件夹"按钮
- **THEN** 系统弹出输入框让用户输入文件夹名称
- **AND** 用户确认后在当前路径下创建该文件夹
- **AND** 创建成功后自动进入新文件夹
- **AND** 文件夹名称需经过安全校验（禁止路径遍历字符、空名称、过长名称）

#### Scenario: 后端 CreateDirectory API
- **WHEN** 前端调用 POST `/api/files/mkdir` 并传递 `{ path: "/parent/dir", name: "newfolder" }`
- **THEN** 后端在校验路径安全性和名称合法性后调用 `os.Mkdir` 创建目录
- **AND** 返回创建结果（成功或带原因的错误）

---

## MODIFIED Requirements

### Requirement: VideoContentVerifier.Verify

`Verify` 方法的行为 SHALL 根据是否为重编码模式进行调整：

- **原始行为**：始终对比 `originalPath` 与 `decryptedPath` 的文件大小和内容
- **修改后行为**：
  - 接受可选参数 `isReencoded bool` 或通过上下文判断
  - 当 `isReencoded=true` 时，跳过精确文件大小比对（L1 的 size check），仅执行结构检查和采样哈希检查
  - 当 `isReencoded=false` 时，保持原有完整验证流程

### Requirement: VideoPlugin.PostEncryptProcessor

PostEncryptProcessor 的 verifyContainer 步骤 SHALL 向验证器传递正确的对比基准：

- **原始行为**：使用 `p.inputPath`（原始输入文件）作为验证基准
- **修改后行为**：如果加密过程中经过了重编码预处理，使用预处理输出文件路径作为验证基准；如果预处理路径不可用，以非严格模式运行验证器

---

## REMOVED Requirements

（无）
