# Checklist

## Phase 1: FilePickerModal 新建文件夹修复

- [x] 模板重构：new-folder-input 使用 overlay 定位（position: fixed），ion-list 不受 v-else 控制
- [x] 点击 + 按钮正确显示输入框（文件列表保持可见，overlay-backdrop 半透明遮罩）
- [x] createDirectory API 调用路径和参数正确（parent_path + name, snake_case）
- [x] 创建成功后 navigateTo 触发 loadFiles 刷新（currentPath watch 响应）
- [x] 取消操作正确恢复 UI 状态（showNewFolder=false, newFolderName=''）
- [x] 错误场景有明确用户反馈（alertController.create 弹出错误信息）
- [x] overlay 样式与 folder-picker-bar 风格一致（z-index 层叠: bar=10, backdrop=19, input=20）

## Phase 2: v4 stsz box missing + Warning 机制

- [x] VerifyWarning 结构体已定义（CheckName/Message/Severity，json tags）
- [x] VerifyOptions 新增 SkipStructCheck + CollectWarnings 字段
- [x] QuickStructCheck 支持 SkipStructCheck 跳过模式，返回 warnings 而非 error
- [x] Verify 方法签名改为 (error, []*VerifyWarning)，聚合各阶段 warnings
- [x] verifyContainer() 对重编码源传入 SkipStructCheck:true + CollectWarnings:true
- [x] plugin.go 包级变量 lastVerifyWarnings 存储并导出 LastVerifyWarnings()
- [x] 重编码 MP4 不再报 stsz box missing error（降级为 warning）
- [x] 非重编码模式下结构检查仍正常执行（回归保护）
- [x] 所有现有 Verify 调用点已适配双返回值签名（security_test 20 处 + options_test）

## Phase 3: v3 临时目录创建修复

- [x] ensureOutputDir() 辅助函数已创建（空→存在→MkdirAll→非目录错误）
- [x] remapMP4ForFastStart 中 CreateTemp 前调用 ensureOutputDir()
- [x] transcodeToFastStartMP4 中 CreateTemp 前调用 ensureOutputDir()
- [x] remapWithMKVMerge/remapMKVWithFFmpeg 中同样处理
- [x] outputDir 不存在时能自动创建并成功创建临时文件
- [x] 测试覆盖 outputDir 不存在场景（5 子测试全部 PASS）

## Phase 4: v4 容器信息乱码修复

- [x] container_id 显示为有效字符串（UUID 格式，非乱码/非空）
- [x] manifest JSON 可正确解析和格式化显示（无乱码）
- [x] version 数字正确显示
- [x] Segments 数据中不含二进制垃圾（sanitizeManifestMap 清洗 C0/C1 控制字符）
- [x] 移除 result.Container["segments"] 直接暴露（避免原始切片数据泄露）
- [x] utf8.Valid(mfBytes) 后置检查通过
- [x] 前端 FileInfo.vue try-catch + 正则可打印字符检测容错
- [x] 前端 FilePreview.vue try-catch + 正则双重保护容错

## Phase 5: 任务卡片 Warning 显示

### 后端
- [x] Task model (MobileTask) 结构体新增 Warning + WarningDetail 字段（omitempty json tag）
- [x] task_manager.go processEncrypt() 完成时读取 video.LastVerifyWarnings() 写入 task
- [x] GET /api/tasks 返回 JSON 包含 warning 字段（gin.H 自动序列化）

### 前端
- [x] EncvTask 接口新增 warning + warningDetail 可选字段
- [x] Tasks.vue completed 状态下显示 warning 区域（橙色图标+文本，区别于红色 error）
- [x] warning 可独立展开/折叠详情（复用 expandedDetail 模式）
- [x] i18n 翻译键已添加（tasks.warning / tasks.warningDetail 中英文）
- [x] onTaskCompleted WebSocket handler 支持 warning 字段解析

### 测试
- [x] 带 warning 的 task JSON 前端渲染验证通过
- [x] completed + error 回归测试仍显示 error（不被 warning 覆盖）

## Phase 6: Mock 测试 — FilePickerModal

- [x] 点击 + → 输入框 overlay 显示（文件列表仍在 DOM）✓
- [x] 输入名称 + 确认 → createDirectory API 调用参数正确 ✓
- [x] 创建成功 → navigateTo → 文件列表刷新 ✓
- [x] 取消 → 输入框隐藏 ✓
- [x] 空名称提交被拦截 ✓
- [x] API 失败(403) → alert 显示错误 ✓
- [x] file 模式下 + 按钮不可见 ✓
- [x] 点击 backdrop 关闭输入 ✓
- [x] Enter 键提交 ✓
- [x] 根路径 / 下新路径构造正确 ✓
- [x] 嵌套路径下新路径构造正确 ✓

## Phase 7: Mock 测试 — 加密 E2E

- [x] v3 不重编码加密完整流程通过 ✓
- [x] v4 重编码 + SkipSizeCheck + SkipStructCheck 验证通过 ✓
- [x] v4 重编码产生 VerifyWarning 并传递到任务结果 ✓
- [x] outputDir 不存在时 ensureOutputDir 自动创建 ✓
- [x] ffprobe BOM 容错解析通过（UTF-8/16 BE/LE）✓
- [x] ffprobe 尾随逗号容错解析通过 ✓

## Phase 8: Mock 测试 — 容器信息 + 任务状态

- [x] v3 容器 info 返回结构正确的 JSON ✓
- [x] v4 container_id 为有效 UUID 格式字符串 ✓
- [x] v4 manifest JSON.parse 往返不丢数据 ✓
- [x] v4 Segments 无二进制垃圾字符 ✓
- [x] sanitizeManifestMap C0/C1 控制字符清洗验证通过 ✓
- [x] Task warning 字段 JSON 序列化/反序列化正确 ✓
- [x] Task omitempty 行为验证通过 ✓
- [x] Task completed + warning 组合行为正确 ✓
