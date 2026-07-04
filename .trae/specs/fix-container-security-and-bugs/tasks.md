# Tasks

## Phase 1: 安全测试 — 容器伪造攻击测试用例

- [x] Task 1: 创建容器伪造攻击测试套件
  - [x] SubTask 1.1: 在 `internal/v2/plugins/video/` 下创建 `content_verifier_security_test.go`
  - [x] SubTask 1.2: 实现 `TestVerify_TamperedHeaderMagic` — 篡改 Header Magic 后验证失败
  - [x] SubTask 1.3: 实现 `TestVerify_TamperedDataSegment` — 翻转加密数据字节后解密验证检测到损坏
  - [x] SubTask 1.4: 实现 `TestVerify_TamperedManifest` — 修改 Manifest 内容后验证/解密失败
  - [x] SubTask 1.5: 实现 `TestVerify_AppendedGarbageData` — 追加垃圾数据后大小/CRC 异常被检测
  - [x] SubTask 1.6: 实现 `TestVerify_TamperedCRC32` — 修改数据但不更新 CRC 后校验失败
  - [x] SubTask 1.7: 使用 v3 和 v4 容器 fixture 分别运行上述测试

## Phase 2: Bug 修复 — v4 容器 size mismatch 误报

- [x] Task 2: 修复 v4 容器 PostEncryptProcessor 验证路径
  - [x] SubTask 2.1: 分析 VideoPlugin 中预处理（PreEncryptProcessor）是否产生了中间文件，记录中间文件路径到 plugin state
  - [x] SubTask 2.2: 修改 `verifyContainer()` 方法，优先使用预处理中间文件作为验证基准（`sourcePath`）
  - [x] SubTask 2.3: 修改 `VideoContentVerifier.Verify()` 方法签名，增加 `opts *VerifyOptions` 参数支持非严格模式
  - [x] SubTask 2.4: 在 VerifyOptions 中增加 `SkipSizeCheck bool` 字段，当 SkipSizeCheck=true 时跳过 L1 的精确文件大小比对
  - [x] SubTask 2.5: 当中间文件路径不可用时，以 SkipSizeCheck=true 调用验证器
  - [x] SubTask 2.6: 编写单元测试：模拟重编码场景下验证器不报 size mismatch 错误

## Phase 3: Bug 修复 — v3 容器 ffprobe JSON 解析容错

- [x] Task 3: 增强 ffprobe JSON 输出解析的容错能力
  - [x] SubTask 3.1: 在 `metadata_extractor.go` 中创建 `sanitizeFFProbeOutput(data []byte) []byte` 工具函数
  - [x] SubTask 3.2: 实现 BOM 去除（UTF-8/UTF-16 BOM）
  - [x] SubTask 3.3: 实现尾随逗号去除（非标准 JSON 兼容）
  - [x] SubTask 3.4: 实现 JSON 截断检测（检查括号匹配完整性）
  - [x] SubTask 3.5: 在 `extractMetadataFromOriginalFile` 中对 ffprobe 输出调用 sanitizeFFProbeOutput
  - [x] SubTask 3.6: 改进错误信息：当 json.Unmarshal 失败时，输出原始数据的 hex dump 前 128 字节用于调试
  - [x] SubTask 3.7: 编写单元测试：传入含 BOM、尾随逗号、截断 JSON 数据验证容错行为

## Phase 4: 功能新增 — 文件夹选择支持新建文件夹

### 4.1 后端 API

- [x] Task 4: 新增后端 CreateDirectory API
  - [x] SubTask 4.1: 在 `MobileService` 中添加 `CreateDirectory(parentPath, name string) error` 方法
  - [x] SubTask 4.2: 实现路径安全校验：禁止 `..`、绝对路径、空名称、名称过长（>255 字符）、非法字符（`\0`, `/`, `\`）
  - [x] SubTask 4.3: 调用 `os.Mkdir(fullPath, 0755)` 创建目录
  - [x] SubTask 4.4: 在 HTTP 路由中注册 `POST /api/files/mkdir` 端点
  - [x] SubTask 4.5: 编写单元测试：正常创建、路径遍历攻击拦截、重复创建处理、权限错误处理

### 4.2 前端 UI

- [x] Task 5: 前端 FilePickerModal 新建文件夹功能
  - [x] SubTask 5.1: 在 `encv.ts` API 层添加 `createDirectory(parentPath: string, name: string): Promise<void>` 函数
  - [x] SubTask 5.2: 在 FilePickerModal.vue toolbar 中添加"新建文件夹"按钮（仅 folder 模式显示）
  - [x] SubTask 5.3: 添加新建文件夹输入交互：点击按钮 → 弹出 ion-alert-input 或内联输入框 → 用户输入名称 → 调用 API → 成功后刷新并进入新文件夹
  - [x] SubTask 5.4: 添加 i18n 翻译键：`files.newFolder`, `files.newFolderName`, `files.createFolderSuccess`, `files.createFolderFailed`
  - [x] SubTask 5.5: 处理错误场景：网络错误、权限不足、名称非法、文件夹已存在

# Task Dependencies

- Task 2 depends on Task 1（安全测试应先建立基线，再修复验证逻辑）
- Task 3 is independent of Tasks 1, 2（ffprobe 修复独立）
- Task 4 and Task 5 are independent of Tasks 1, 3（前端功能独立）
- Task 5 depends on Task 4（前端依赖后端 API）

# Parallelizable Work

- Task 1 + Task 3 + Task 4 可并行（互不依赖）
- Task 2 需在 Task 1 之后（先有测试基线再修复）
- Task 5 需在 Task 4 之后（前后端顺序依赖）
