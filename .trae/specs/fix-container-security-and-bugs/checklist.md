# Checklist

## Phase 1: 安全测试

- [x] 容器伪造攻击测试套件已创建（content_verifier_security_test.go）
- [x] Header Magic 篡改测试通过（V3 + V4 + DecryptedOutput 子场景）
- [x] 数据段篡改测试通过（V3 Block + V4 Segment + MultipleRandomFlips）
- [x] Manifest 篡改测试通过（V3 ManifestRegion + DecryptedOutput JSON）
- [x] 文件大小伪造测试通过（V3/V4 AppendedGarbage + IdenticalFiles 场景）
- [x] CRC32 篡改测试通过（V3 Block CRC + V4 SegmentCRC + SingleByteChange）
- [x] v3 和 v4 容器 fixture 均已覆盖

## Phase 2: v4 size mismatch 修复

- [x] verifyContainer() 使用正确的验证基准文件路径（修复 TempFileReadCloser 类型断言 + SkipSizeCheck 模式检测）
- [x] VideoContentVerifier.Verify() 支持 SkipSizeCheck 非严格模式（VerifyOptions 可变参数）
- [x] 重编码模式下不再误报 size mismatch（verifyContainer 自动传入 SkipSizeCheck:true）
- [x] 不重新编码模式原有验证行为不变（可变参数向后兼容，不传 opts 时行为一致）
- [x] 单元测试覆盖重编码场景（verify_options_test.go 6 个用例）

## Phase 3: ffprobe JSON 容错

- [x] sanitizeFFProbeOutput 函数实现 BOM 去除（UTF-8/UTF-16 BE/LE）
- [x] sanitizeFFProbeOutput 函数实现尾随逗号去除（} / ] 前逗号）
- [x] sanitizeFFProbeOutput 函数实现截断检测（括号/方括号平衡检查）
- [x] metadata_extractor.go 调用 sanitizeFFProbeOutput（在 json.Unmarshal 之前）
- [x] json.Unmarshal 失败时输出调试用 hex dump（前 128 字节）
- [x] 单元测试覆盖 BOM/尾随逗号/截断场景（metadata_extractor_sanitize_test.go 4 组测试）

## Phase 4: 新建文件夹功能

### 后端
- [x] MobileService.CreateDirectory 方法已实现（mobile_service.go）
- [x] 路径安全校验（路径遍历、空名称、>255字符、\0、/）
- [x] POST /api/files/mkdir 端点已注册（server.go + mobile_api.go handler）
- [x] 后端单元测试通过（mobile_service_mkdir_test.go 10 个用例）

### 前端
- [x] encv.ts createDirectory API 函数已添加
- [x] FilePickerModal"新建文件夹"按钮已添加（folder 模式，add 图标）
- [x] 新建文件夹交互流程完整（输入 → API 调用 → 成功后 navigateTo → 失败 alertController 提示）
- [x] i18n 翻译键已添加（useI18n.ts: newFolderName, createFolderFailed）
- [x] 错误场景处理完整（网络错误、权限不足、名称非法、文件夹已存在）
