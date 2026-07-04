# Tasks

## Phase 1: Bug 修复 — FilePickerModal 新建文件夹

- [x] Task 1: 修复 FilePickerModal 新建文件夹 UI 和交互
  - [x] SubTask 1.1: **根因修复**：将 `v-if="showNewFolder"` (div) 与 `v-else` (ion-list) 的互斥渲染改为 overlay 模式 — ion-list 始终渲染，new-folder-input 用 position: fixed 覆盖其上
  - [x] SubTask 1.2: 确认 createDirectory API 调用路径正确（parent_path snake_case 与后端一致）
  - [x] SubTask 1.3: 确认 navigateTo 成功后触发 loadFiles 刷新（currentPath watch 响应）
  - [x] SubTask 1.4: 错误处理：API 失败时 alertController 显示错误，输入框保持打开
  - [x] SubTask 1.5: 样式：overlay-backdrop (z-index:19, rgba(0,0,0,0.3)) + new-folder-input-overlay (z-index:20, fixed bottom)

## Phase 2: Bug 修复 — v4 stsz box missing + Warning 机制

- [x] Task 2: VerifyWarning 机制 + SkipStructCheck
  - [x] SubTask 2.1: 在 interfaces.go 中定义 `VerifyWarning` 结构体（CheckName/Message/Severity）
  - [x] SubTask 2.2: 在 VerifyOptions 中新增 SkipStructCheck + CollectWarnings 字段
  - [x] SubTask 2.3: QuickStructCheck 签名改为接收 opts，SkipStructCheck=true 时返回 warnings 而非 error
  - [x] SubTask 2.4: Verify 方法签名改为 (error, []*VerifyWarning)，聚合 warnings
  - [x] SubTask 2.5: verifyContainer() 重编码模式传入 SkipStructCheck:true + CollectWarnings:true
  - [x] SubTask 2.6: plugin.go 新增包级变量 lastVerifyWarnings + LastVerifyWarnings() 导出函数
  - [x] SubTask 2.7: 测试：重编码 MP4 通过 SkipStructCheck 并产生预期 warning（3 子测试）

## Phase 3: Bug 修复 — v3 临时目录创建失败

- [x] Task 3: content_preprocessor MkdirAll 防御
  - [x] SubTask 3.1: 创建 ensureOutputDir() 辅助函数（空检查→存在检查→MkdirAll→非目录错误）
  - [x] SubTask 3.2: remapMP4ForFastStart 的 CreateTemp 前调用 ensureOutputDir()
  - [x] SubTask 3.3: transcodeToFastStartMP4 的 CreateTemp 前调用 ensureOutputDir()
  - [x] SubTask 3.4: remapWithMKVMerge 和 remapMKVWithFFmpeg 同样处理
  - [x] SubTask 3.5: 测试：outputDir 不存在 → ensureOutputDir → CreateTemp 成功（5 子测试）

## Phase 4: Bug 修复 — v4 容器信息乱码

- [x] Task 4: 诊断并修复容器信息编码问题
  - [x] SubTask 4.1: 移除 result.Container["segments"] = mf.Segments（避免原始切片暴露）
  - [x] SubTask 4.2: 新增 utf8.Valid(mfBytes) 检查 + sanitizeManifestMap() 递归清洗 C0/C1 控制字符
  - [x] SubTask 4.3: isPrintableJSONString() 检测不可打印字符并替换为 "(non-printable data)"
  - [x] SubTask 4.4: FileInfo.vue 增加 try-catch + 正则检测可打印 ASCII 范围
  - [x] SubTask 4.5: FilePreview.vue 增加 try-catch + 正则双重保护

## Phase 5: 功能新增 — 任务卡片 Warning 显示

- [x] Task 5: 端到端 Warning 显示链路
  - [x] SubTask 5.1: **后端** MobileTask 结构体新增 Warning + WarningDetail 字段（omitempty）
  - [x] SubTask 5.2: **后端** task_manager.go processEncrypt() 完成时读取 video.LastVerifyWarnings() 写入 task
  - [x] SubTask 5.3: **后端** gin.JSON 自动序列化新字段（无需修改 handler）
  - [x] SubTask 5.4: **前端 API** EncvTask 接口新增 warning + warningDetail 字段
  - [x] SubTask 5.5: **前端 UI** Tasks.vue 橙色 warning 条（warningOutline 图标）+ 点击展开 JSON 详情
  - [x] SubTask 5.6: **i18n** tasks.warning / tasks.warningDetail 中英文翻译键
  - [x] SubTask 5.7: onTaskCompleted WebSocket 处理支持 warning 字段

## Phase 6: Mock 测试完善 — FilePickerModal

- [x] Task 6: FilePickerModal 组件测试
  - [x] SubTask 6.1: 创建 FilePickerModal.test.ts（vitest + @vue/test-utils）
  - [x] SubTask 6.2: 点击 + → overlay 输入框显示（ion-list 仍在 DOM）
  - [x] SubTask 6.3: 输入名称 → confirm → createDirectory(parent_path, name) → navigateTo
  - [x] SubTask 6.4: cancel → showNewFolder=false → 输入框消失
  - [x] SubTask 6.5: 空名称 confirm → 不调用 API
  - [x] SubTask 6.6: createDirectory reject(403) → alertController 显示错误
  - [x] SubTask 6.7: file 模式下 + 按钮不可见
  - [x] SubTask 6.8: 点击 backdrop 关闭输入
  - [x] SubTask 6.9: Enter 键提交
  - [x] SubTask 6.10: 根路径 / 下新路径构造正确
  - [x] SubTask 6.11: 嵌套路径下新路径构造正确

## Phase 7: Mock 测试完善 — 加密 E2E

- [x] Task 7: 加密流程 E2E mock 测试
  - [x] SubTask 7.1: 创建 encryption_e2e_test.go
  - [x] SubTask 7.2: TestE2E_V3_NoReencode_CompleteFlow — v3 完整流程通过
  - [x] SubTask 7.3: TestE2E_V4_Reencode_SkipChecks_Passes — SkipSizeCheck+SkipStructCheck+warnings
  - [x] SubTask 7.4: TestE2E_Preprocess_MissingOutputDir_AutoCreated — ensureOutputDir 自动创建
  - [x] SubTask 7.5: TestE2E_FFProbe_BOM_Tolerant — BOM 去除（UTF-8/16 BE/LE, 3 子测试）
  - [x] SubTask 7.6: TestE2E_FFProbe_TrailingComma_Tolerant — 尾随逗号去除（3 子测试）

## Phase 8: Mock 测试完善 — 容器信息 + 任务状态

- [x] Task 8: 容器信息 + 任务状态测试
  - [x] SubTask 8.1: 创建 container_info_test.go
  - [x] SubTask 8.2: TestV4Info_ContainerId_ValidFormat — UUID 格式验证
  - [x] SubTask 8.3: TestV4Info_Manifest_RoundTrip — 序列化往返（3 子测试）
  - [x] SubTask 8.4: TestV4Info_SanitizeManifestMap_SpecialCharacters — C0/C1 清洗（6 子测试）
  - [x] SubTask 8.5: TestV4Info_Segments_NoBinaryGarbage — 二进制 Nonce 无控制字符
  - [x] SubTask 8.6: 创建 task_warning_test.go
  - [x] SubTask 8.7: TestTask_WarningFields_Serialization — JSON 往返
  - [x] SubTask 8.8: TestTask_WarningFields_Optional — omitempty 行为（5 子测试）
  - [x] SubTask 8.9: TestTask_CompletedWithWarning_DisplayLogic — 组合合法性（6 子测试）

# Task Dependencies

- Task 1, 2, 3, 4 可并行（互不依赖的 bug 修复） ✅
- Task 5 依赖 Task 2（Warning 机制需先建立） ✅
- Task 6 可与 Task 1-4 并行（独立组件测试） ✅
- Task 7 依赖 Task 2, 3（需 SkipStructCheck + ensureOutputDir 先就绪） ✅
- Task 8 依赖 Task 4, 5（需容器信息修复 + warning 字段先就绪） ✅

# Parallelizable Work

- Wave 1 (并行): Task 1 + Task 2 + Task 3 + Task 4 + Task 6 ✅
- Wave 2 (依赖 Wave 1): Task 5 (依赖 Task 2) + Task 7 (依赖 Task 2, 3) ✅
- Wave 3 (依赖 Wave 2): Task 8 (依赖 Task 4, 5) ✅
