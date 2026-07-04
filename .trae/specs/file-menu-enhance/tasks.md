# Tasks

- [x] Task 1: 后端 — 新增文件复制和移动 API Handler
  - [x] 在 `admin_handlers.go` 中实现 `handleFileCopyGin`（POST /api/file/copy）：源路径 + 目标路径，校验权限后 `io.Copy` + 文件去重
  - [x] 在 `admin_handlers.go` 中实现 `handleFileMoveGin`（POST /api/file/move）：源路径 + 目标路径，校验后 `os.Rename`（跨设备时 fallback copy+delete）
  - [x] 在 `server.go` 路由组中注册 `POST /file/copy` 和 `POST /file/move`

- [x] Task 2: 后端 — 新增插件元数据 API
  - [x] 在 `mobile_api.go` 或新建 handler 中实现 `handlePluginsGin`（GET /api/plugins）：遍历 `plugins.Plugins` 返回名称/扩展名/MIME前缀/容器扩展名
  - [x] 在 `server.go` 注册路由

- [x] Task 3: 后端 — 标签系统 API
  - [x] 创建 `internal/server/tags.go`：内存中标签存储（map[filePath][]string），基于文件路径关联（首次请求时计算 SHA256 哈希作为内部 ID）
  - [x] 实现 `GET /api/files/tags`：返回所有标签及文件数量统计
  - [x] 实现 `POST /api/files/tags`：为文件添加/移除标签（body: {path, tag, action: "add"|"remove"}）
  - [x] 修改现有 `GET /api/files?path=` handler 支持 `?tag=xxx` 查询参数筛选
  - [x] 在 `server.go` 注册路由

- [x] Task 4: 前端 API 层 — 新增 API 函数
  - [x] 在 `encv.ts` 中新增 `renameFile(oldPath, newName)`、`copyFile(srcPath, destPath)`、`moveFile(srcPath, destPath)`
  - [x] 在 `encv.ts` 中新增 `fetchPlugins()` 返回插件元信息列表
  - [x] 在 `encv.ts` 中新增 `fetchTags()`、`addTag(path, tag)`、`removeTag(path, tag)`、`listFilesByTag(tag)`

- [x] Task 5: 前端 — 安装 @capacitor/share 插件
  - [x] `npm install @capacitor/share`
  - [x] 在 Capacitor 配置中注册 Share 插件

- [x] Task 6: 前端 — Files.vue 长按菜单扩展
  - [x] 扩展 `showFileActions()` ActionSheet，新增：重命名（输入框）、复制（自动命名）、移动（目录选择器）、分享（调用 Share）、标签（输入框）
  - [x] 实现各操作的回调函数并调用 Task 4 的 API
  - [x] 重命名操作成功后刷新当前目录列表

- [x] Task 7: 前端 — Files.vue 侧边抽屉实现
  - [x] 添加左侧抽屉组件（IonDrawer/IonMenu 或自定义滑动抽屉），入口按钮在文件列表顶部工具栏
  - [x] 抽屉内容区动态渲染插件列表（调用 `fetchPlugins()`），每项显示插件名 + 支持格式图标
  - [x] 点击插件项导航到二级页面（复用 Files.vue 组件或新建 PluginFilesView.vue）
  - [x] 二级页使用 `searchFiles()` 按插件支持的扩展名检索，分"未加密"/"已加密容器"两个 Tab 展示
  - [x] 抽屉底部展示标签列表（调用 `fetchTags()`）

- [x] Task 8: Mock 测试
  - [x] 后端：为 `handleFileCopyGin` / `handleFileMoveGin` 编写单元测试（遵循 registry_test.go 的 testify 模式）
  - [x] 后端：为 tags CRUD handlers 编写单元测试
  - [x] 后端：为 plugins 元数据 API 编写单元测试
  - [x] 前端：vite build 验证通过（无编译错误）

# Task Dependencies
- [Task 4] depends on [Task 1], [Task 2], [Task 3]
- [Task 6] depends on [Task 4], [Task 5]
- [Task 7] depends on [Task 4]
- [Task 8] depends on [Task 1], [Task 2], [Task 3], [Task 6], [Task 7]
