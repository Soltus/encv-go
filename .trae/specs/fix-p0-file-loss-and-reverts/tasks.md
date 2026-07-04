# Tasks

- [x] Task 1: 恢复 FilePreview.vue iframe 文本预览
  - [x] 1.1 读取当前 FilePreview.vue 完整内容
  - [x] 1.2 将 `<pre><code>` 渲染回退为 `<iframe :src="textPreviewUrl" class="preview-iframe">`
  - [x] 1.3 恢复 `textPreviewUrl` ref，移除 textContent/textLoading/textError
  - [x] 1.4 恢复 loadFile() text 分支：`textPreviewUrl.value = getFilePreviewUrl('text.html', path)`
  - [x] 1.5 移除 loadTextContent() 函数和 .text-content/.text-preview CSS

- [x] Task 2: 修复 TempFileReadCloser.Close 自动删除 + 排查 stsz box missing
  - [x] 2.1 修改 `internal/v2/reader/temp_file.go` Close()：移除 `os.Remove(t.path)`，仅保留 `t.file.Close()`
  - [ ] 2.2 在 registry.go EncryptFileWithPlugin 中，在 PostEncryptProcessor 返回**之后**添加显式 `os.Remove(encryptedSourcePath)` 清理预处理临时文件
  - [ ] 2.3 排查 "stsz box missing"：检查 content_verifier.go 的 QuickStructCheck 是否对 remux 后的 MP4 文件误报。确认加密后验证路径是否使用了 SkipStructCheck 选项。

- [x] Task 3: 修复插件安装根本不成功（parameterCount 1→2）
  - [x] 3.1 修改 GoProcessPlugin.kt L556：`parameterCount == 1` → `parameterCount == 2`
  - [ ] 3.2 修改 L558 调用参数：`installMethod.invoke(pm, apkFile)` → `installMethod.invoke(pm, apkFile, true)`
  - [ ] 3.3 确认 ComboLite InstallerManager.installPlugin(File, Boolean) 返回 InstallResult（非 void），处理返回值
  - [ ] 3.4 如果反射仍失败（方法名/签名变化），添加详细日志输出所有可用方法名和签名供调试

# Task Dependencies
- 无跨任务依赖
