# Tasks

- [x] Task 1: 长按菜单补全图标
  - [x] 从 ionicons/icons 导入 createOutline, copyOutline, shareOutline, arrowForwardOutline
  - [x] 为"重命名"按钮添加 `icon: createOutline`
  - [x] 为"复制"按钮添加 `icon: copyOutline`
  - [x] 为"移动"按钮添加 `icon: arrowForwardOutline`
  - [x] 为"分享"按钮添加 `icon: shareOutline`
  - [x] 为"标签管理"按钮添加 `icon: pricetagOutline`

- [x] Task 2: 分享功能修复
  - [x] 在 GoProcessPlugin.kt 新增 `getLocalFilePath(call: PluginCall)` 方法
  - [x] 在 GoProcess.ts 新增 `getLocalFilePath(path): Promise<string>` 导出函数
  - [x] 在 web.ts 的 GoProcessPlugin 接口和 GoProcessWeb 类中新增对应 stub
  - [x] 修改 Files.vue 的 `handleShare()`：先调用 getLocalFilePath() 获取本地路径；非空时用 Share.share({ url: 'file://' + localPath })；为空时 showToast 提示

- [x] Task 3: 标签管理 UI 升级 — 编辑器组件
  - [x] 将 showTagDialog（IonAlert 单输入框）替换为 IonModal 内嵌标签编辑器
  - [x] 标签编辑器布局：标题、已有标签 chip 列表（带 × 删除）、底部输入框+添加按钮
  - [x] 新增 editingFileTags / newTagInput ref
  - [x] 打开编辑器时加载已有标签
  - [x] 实现添加标签 handleAddNewTag() + 删除标签 handleRemoveTag()

- [x] Task 4: 文件列表项标签显示
  - [x] 新增 fileTagMap ref 缓存文件→标签映射
  - [x] loadFiles 成功后调用 loadFileTagsForCurrentDir()
  - [x] 文件列表项渲染 IonChip 标签 chips
  - [x] 无标签文件不显示额外区域

- [x] Task 5: 抽屉入口按钮位置修正
  - [x] 将插件分类按钮从 slot="start" 移到 slot="end"

# Task Dependencies
- [Task 2] 无依赖，可与 Task 1 并行 ✅ 并行完成
- [Task 3] 无依赖，可与 Task 1 并行 ✅ 并行完成
- [Task 4] 依赖后端 tags API 已可用 ✅ 完成
- [Task 5] 无依赖，纯 UI 调整 ✅ 完成
