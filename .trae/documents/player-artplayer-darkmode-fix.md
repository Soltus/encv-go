# 播放器修复计划：ArtPlayer 控件崩溃 + 暗黑模式缺失 + 主应用清理

## 问题分析

### 问题 1：ArtPlayer 黑屏无法播放
**错误**：`ArtPlayerError: option.controls.0.html require 'string' or 'Element' type`

**根因**：StandalonePlayer.vue 的 ArtPlayer 配置中 `controls` 数组写法错误：
```typescript
controls: [
  { name: 'play' },
  { name: 'time' },
  ...
]
```
ArtPlayer 5.x 的 `controls` 选项只用于**自定义控件**，每项必须有 `html` 属性。内置控件默认显示，无需声明。主应用 Player.vue 没写 `controls`，一切正常。

**修复**：删除 `controls` 数组，使用默认内置控件。

### 问题 2：播放器未继承暗黑模式
**根因**：`player-main.ts` 没有调用 `useTheme().initTheme()`，`document.body` 永远不会加 `dark` class。同时缺少 5 个 Ionic CSS 导入（structure、typography、padding、flex-utils、display）。

**修复**：添加 `initTheme()` + 补全 CSS + PlayerSettings 加暗黑模式开关。

### 问题 3：主应用 Player.vue 残留
**现状**：
- `Player.vue` 仍注册在主路由 `/tabs/player`
- `Files.vue` 中 3 处 `router.push('/tabs/player')` 作为 web/PWA 回退
- Tabs.vue 已移除 Player tab 按钮，但路由和组件仍在

**修复**：删除 Player.vue，主路由中 `/tabs/player` 改为顶层 `/player` 路由指向 StandalonePlayer.vue（复用独立播放器组件），Files.vue 中 3 处路径同步更新。

---

## 修复步骤

### 步骤 1：修复 StandalonePlayer.vue 的 ArtPlayer 控件配置
**文件**：`src/views/StandalonePlayer.vue`

- 删除 `initArtPlayer()` 中 ArtPlayer 构造参数的 `controls` 数组

### 步骤 2：player-main.ts 添加暗黑模式初始化 + 补全 CSS
**文件**：`src/player-main.ts`

- 导入并调用 `useTheme().initTheme()`
- 补齐缺失的 Ionic CSS 导入（structure、typography、padding、flex-utils、display）

### 步骤 3：PlayerSettings.vue 添加暗黑模式开关
**文件**：`src/views/PlayerSettings.vue`

- 导入 `useTheme`
- 新增"外观"分组，包含暗黑模式 Toggle

### 步骤 4：删除 Player.vue，主路由改用 StandalonePlayer
**文件**：`src/views/Player.vue` → 删除

**文件**：`src/router/index.ts`
- 删除 `/tabs/player` 子路由
- 新增顶层 `/player` 路由，指向 StandalonePlayer.vue

**文件**：`src/views/Files.vue`
- 3 处 `router.push({ path: '/tabs/player', ... })` 改为 `router.push({ path: '/player', ... })`

### 步骤 5：StandalonePlayer.vue 适配 web/PWA 模式
**文件**：`src/views/StandalonePlayer.vue`

当前 StandalonePlayer 在 `initBackend()` 中强制调用 `isStandaloneMode()`，如果不是 standalone 就报错退出。但 web/PWA 模式下也会用到这个组件（从主应用路由跳转），需要适配：
- 如果 `isStandaloneMode()` 返回 false，从 route.query 读取 path/name（与旧 Player.vue 逻辑一致）
- 不依赖 Capacitor 插件获取文件信息，直接用 query 参数
- streamUrl 使用 `getFileStreamUrl()` 而非 `getExternalStreamUrl()`

### 步骤 6：本地构建验证
- 执行 `npm run build` 确认零错误
