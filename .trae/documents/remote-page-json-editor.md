# 远端页面重构 + JSON 编辑器 — 续接计划

## 当前状态

大部分工作已在上一轮会话中完成：

### ✅ 已完成
1. **后端 API**：`/api/remote/info`、`/api/remote/openlist`（POST 添加）、`/api/remote/openlist/{siteId}`（PUT 更新 / DELETE 删除）
2. **前端 API 层**：`fetchRemoteInfo()`、`addOpenlistSite()`、`updateOpenlistSite()`、`deleteOpenlistSite()`
3. **Remote.vue**：完整的远端页面，WebDAV Tab（本机自动识别 + 手动添加列表）+ Openlist Tab（增删改查 + 复制代理 URL）
4. **路由/Tab 更新**：`webdav` → `remote`，图标 `cloud` → `globe`
5. **i18n 翻译**：远端相关翻译键已添加

### 🔄 未完成
1. **Settings.vue JSON 编辑器 CSS 样式缺失**：模板和脚本逻辑已添加，但 `<style scoped>` 中缺少 `.json-editor-layout`、`.json-annotations`、`.json-textarea`、`.json-error` 等样式
2. **Settings.vue i18n 缺失**：`editRawConfig`、`configAnnotations`、`jsonError`、`saveConfig` 等翻译键未添加到 `useI18n.ts`
3. **PluginSettings.vue JSON 编辑器**：完全未添加
4. **前端类型检查**：未验证

## 实施步骤

### Step 1: 添加 JSON 编辑器相关 i18n 翻译键
在 `useI18n.ts` 中添加：
- `settings.editRawConfig` → 编辑原始配置 / Edit Raw Config
- `settings.configAnnotations` → 配置注释 / Config Annotations
- `settings.jsonError` → JSON 格式错误 / JSON Format Error
- `settings.saveConfig` → 保存配置 / Save Config

### Step 2: Settings.vue 添加 JSON 编辑器 CSS 样式
在 `<style scoped>` 中添加：
- `.json-editor-layout`：flex 布局，左右分栏（注释面板 + 编辑器）
- `.json-annotations`：左侧注释面板，可折叠，显示每个配置路径的 description
- `.json-textarea-wrapper`：右侧编辑区
- `.json-textarea`：等宽字体 textarea，占满空间
- `.json-error`：红色错误提示条
- `.annotation-item`、`.annotation-path`、`.annotation-desc`、`.annotations-title`

### Step 3: PluginSettings.vue 添加 JSON 编辑器
与 Settings.vue 相同的模式：
1. 添加 `showJsonEditor`、`jsonText`、`jsonError`、`configAnnotations` ref
2. 添加 `extractAnnotations`、`openJsonEditor`、`validateJson`、`handleSaveJson` 函数
3. 添加"编辑原始配置"按钮
4. 添加 JSON 编辑器 Modal（模板 + 样式）
5. 导入 `fetchConfig`、`updateConfig`

### Step 4: 运行前端类型检查
```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit
```
修复任何类型错误。

## 设计细节

### JSON 编辑器布局
- 移动端：注释面板在上，textarea 在下（纵向排列）
- 注释面板：可折叠（点击标题切换），每项显示 `path` + `description`
- textarea：等宽字体，暗色背景，实时校验
- 错误提示：红色背景条，显示具体错误信息
- 保存按钮：Modal header 右侧，JSON 无效时禁用

### 设置界面只读策略
用户要求"设置界面考虑只读不可编辑，符合gui友好"。这意味着：
- Settings.vue 和 PluginSettings.vue 中的表单字段保持可编辑（GUI 友好）
- 但对于 Openlist 站点的配置，在远端页面（Remote.vue）中进行增删改查
- 设置界面中 proxy.sites 的 map 展示为只读列表（当前已是这样——只显示条目，无编辑入口）
- JSON 编辑器作为高级功能，允许直接编辑原始配置

### 代码复用
`extractAnnotations` 函数在 Settings.vue 和 PluginSettings.vue 中重复，但考虑到：
1. 两个文件各自独立，不共享 composable
2. 函数逻辑简单（约 15 行）
3. 避免过度抽象

暂时保持各自独立实现。如果后续有更多页面需要 JSON 编辑器，再提取为 composable。
