# 播放器修复计划：ArtPlayer 原生控件 + 全屏退出竖屏 + 主应用插件设置

## 问题分析

### 问题 1：ArtPlayer 显示浏览器原生控件
**现象**：logcat 显示 ArtPlayer 初始化成功，但视频仍显示浏览器原生控件。

**根因**：Android WebView 对 `<video>` 原生控件的渲染不受 `video.controls = false` 完全控制。ArtPlayer 5.4.0 通过 `moreVideoAttr: { controls: false }` 设置，但 WebView 可能忽略。

**修复**：
1. CSS `:deep()` 穿透隐藏 `::-webkit-media-controls` 伪元素
2. JS 双重保障：`removeAttribute('controls')` + `controls = false`

### 问题 2：退出全屏后未恢复竖屏
**根因**：`orientation: 'unlocked'` 对应 `SCREEN_ORIENTATION_UNSPECIFIED`，不强制回竖屏。

**修复**：`handleFullscreenExit()` 中改为 `'portrait'`

### 问题 3：主应用插件设置统一到单独二级界面
**现状**：主应用 Settings.vue 中 `plugin_settings`（含 video/audio/image/wps/pdf/text 六个插件配置）直接平铺在设置主页，与其他全局设置混在一起，页面过长。

**修复**：
- Settings.vue 中 `plugin_settings` section 不再展开渲染，改为一个带箭头的导航项，点击跳转到 `/tabs/settings/plugins`
- 新建 `PluginSettings.vue` 作为插件设置二级页面，复用 `useConfig()` 的 `schemaFields` 中 `plugin_settings` 的渲染逻辑
- 路由注册 `/tabs/settings/plugins`

---

## 修复步骤

### 步骤 1：修复 ArtPlayer 原生控件
**文件**：`src/views/StandalonePlayer.vue`

1. `<style scoped>` 中添加：
   ```css
   :deep(video::-webkit-media-controls) { display: none !important; }
   :deep(video::-webkit-media-controls-enclosure) { display: none !important; }
   :deep(video::-webkit-media-controls-panel) { display: none !important; }
   ```
2. `initArtPlayer()` 中 `new Artplayer(...)` 后添加显式移除 controls 属性

### 步骤 2：修复退出全屏恢复竖屏
**文件**：`src/views/StandalonePlayer.vue`

- `handleFullscreenExit()` 中 `orientation` 从 `'unlocked'` 改为 `'portrait'`

### 步骤 3：创建 PluginSettings.vue
**文件**：`src/views/PluginSettings.vue`（新建）

- 复用 `useConfig()` composable
- 只渲染 `plugin_settings` section（从 schemaFields 中过滤 key === 'plugin_settings'）
- 使用与 Settings.vue 相同的渲染逻辑（object 类型展开子属性）
- 顶部 toolbar 带返回按钮（`ion-back-button`）
- 保存/重置按钮与 Settings.vue 一致

### 步骤 4：修改 Settings.vue
**文件**：`src/views/Settings.vue`

- 在 `schemaFields` 渲染循环中，对 `section.key === 'plugin_settings'` 特殊处理：
  不展开渲染子属性，改为显示一个带箭头的导航项
- 点击导航项跳转 `router.push('/tabs/settings/plugins')`

### 步骤 5：注册路由
**文件**：`src/router/index.ts`

- 在 tabs children 中添加：
  ```typescript
  { path: 'settings/plugins', component: () => import('@/views/PluginSettings.vue') }
  ```

### 步骤 6：本地构建验证
- 执行 `npm run build` 确认零错误
