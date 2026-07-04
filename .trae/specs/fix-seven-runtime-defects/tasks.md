# Tasks

- [x] Task 1: 修复 createDirectory API 路径 404
  - [x] 1.1 修改 `app/encv-mobile/src/api/encv.ts` 的 `createDirectory()` 函数：URL 从 `/files/mkdir` 改为 `/api/files/mkdir`
  - [x] 1.2 验证：确认与 server.go L217 注册的 `r.POST("/api/files/mkdir", ...)` 路径一致

- [x] Task 2: 优化文本 iframe 预览性能
  - [x] 2.1 修改 FilePreview.vue：移除 text 类型的 iframe 渲染（L65-67）
  - [x] 2.2 改为直接 fetch 文本内容 + `<pre><code>` 渲染，含 loading/error/retry 状态
  - [x] 2.3 添加 500K 字符截断保护防止大文件卡顿
  - [x] 2.4 新增等宽字体 CSS 样式（.text-content）

- [x] Task 3: 修复插件安装状态卡死
  - [x] 3.1 GoProcessPlugin.kt installFromPath(): 系统安装器 fallback 路径新增 pending:true 标记 + 明确日志
  - [x] 3.2 checkInstalledPlugins() fallbackCheckInstalled(): 移除重复目录 + 新增 cacheDir/plugin_install 扫描
  - [x] 3.3 ExtensionsPage.vue: Promise.race 120s 超时保护 + system installer 区分提示

- [x] Task 4: 修复 v4 容器版本号和容器 ID 乱码
  - [x] 4.1 mobile_service.go GetFileInfo(): container_id 值添加 utf8.Valid + isPrintableJSONString 清洗
  - [x] 4.2 version 字段添加 int/float64/string 三类型防御性校验

- [x] Task 5: 修复加密崩溃 + 中间产物残留
  - [x] 5.1 5 处 os.CreateTemp 目录从 outputDir 改为 filepath.Join(outputDir, ".encv_tmp") 隐藏子目录
  - [x] 5.2 Preprocess() 添加命名返回值 + defer panic recover
  - [x] 5.3 EncryptFileWithPlugin() 添加 defer panic recover

- [x] Task 6: 修复移动端 output_path 配置未生效
  - [x] 6.1 config.go: 提取 ApplyMobileOverrides() 公开函数
  - [x] 6.2 server_config_api.go GET handler: 增加移动端合并处理（仅 ENCV_MOBILE=1）
  - [x] 6.3 设置页现在显示合并后的正确 output_path 值

# Task Dependencies
- [Task 5.3] depends on [Task 2 from fix-runtime-triple-defects] (outputDir fix already done) ✅
