# 修复 HomePage AI 入口缺失 + Settings 插件设置页不渲染

> 用户反馈：「插件设置崩了没有渲染出来。ai agent 入口呢怎么 spec 不是做了吗怎么没看见？」

## 1. 根因（已 100% 定位）

### 1.1 Bug A：HomePage.vue 缺 `AgentEntry` import → AI 浮动按钮不显示

**文件**：[HomePage.vue:62-99](file:///workspace/app/encv-mobile/src/views/HomePage.vue#L62-L99)

第 57 行模板用了 `<AgentEntry />`，但 `<script setup>` 块 import 区**完全没** import `AgentEntry` 组件。

```vue
<template>
  ...
  <!-- 浮动 AI 入口（Phase 7.6） -->
  <AgentEntry />  <!-- ← 第 57 行：使用了 AgentEntry -->
  ...
</template>

<script setup lang="ts">
import { IonPage, IonHeader, IonToolbar, IonTitle, IonContent, IonIcon } from '@ionic/vue'
import { playCircle, folder, lockClosed, globe, layersOutline } from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useRouter } from 'vue-router'
import { onIonViewWillEnter } from '@ionic/vue'
// ❌ 缺：import AgentEntry from '@/components/agent/AgentEntry.vue'

const { t } = useI18n()
const router = useRouter()
// ...
</script>
```

**Vue 编译时行为**：
- `Failed to resolve component: AgentEntry` warning
- `<AgentEntry />` 渲染为 HTML 注释（`<!---->`），**不显示**浮动 AI 按钮
- **不会**导致整个 HomePage 渲染失败（Vue 3 容错好）—— 其他内容正常
- 用户看到的现象：「HomePage 看不到右下角 AI 入口」

### 1.2 Bug B：PluginSettings.vue 渲染失败（v-else-if `configLoaded && pluginSection` 命中后内部 v-for 出错）

**文件**：[PluginSettings.vue:24-127](file:///workspace/app/encv-mobile/src/views/PluginSettings.vue#L24-L127)

PluginSettings.vue 第 24 行：
```vue
<template v-else-if="configLoaded && pluginSection">
  <ion-list>...</ion-list>
</template>
```

`pluginSection` 通过：
```ts
const pluginSection = computed(() => schemaFields.value.find(s => s.key === 'plugin_settings'))
```

**潜在问题**：
1. **schema 解析 `video`/`audio`/`image`/`wps`/`pdf`/`text` 子项时**：`schema.json` 里这些是 `{"$ref": "#/$defs/VideoPluginConfig"}` 形式，[schemaParser.ts:73-76](file:///workspace/app/encv-mobile/src/config/schemaParser.ts#L73-L76) 用 spread 解析 `$ref`：

   ```ts
   let resolved = prop
   if (prop.$ref) {
     resolved = { ...resolveRef(prop.$ref), description: prop.description || resolveRef(prop.$ref).description }
   }
   ```
   
   `resolved = { ...VideoPluginConfig_fields, description: ... }` — **保留** VideoPluginConfig 的 properties/type/required。**应该**能正常生成 FieldDef.properties。

2. **第 35 行 v-if 条件**：`child.type === 'object' && child.properties && !child.isMap`
   
   - `child.type` 应是 `'object'` ✓（VideoPluginConfig.type === 'object'）
   - `child.properties` 应该有 8+ 字段（ext/container_chunk_size_mb/...）
   - `!child.isMap` — plugin_settings 顶层没设 isMap
   - **应该命中**

3. **真正可能问题**：Browser console 报错。比如：
   - `ConfigFieldItem` 组件缺失（PluginSettings.vue 用了 ConfigFieldItem 但没 import）
   - 或者 grandchild 的 `isFieldVisible` 在 mobile 平台过滤掉了所有字段（看 `default_stream_preset.x-platform === 'mobile'`——desktop 平台**不会渲染**，但 mobile 应该渲染）

**需要诊断**：浏览器 DevTools console 报什么错。

### 1.3 假设 1：PluginSettings.vue 缺 `ConfigFieldItem` import

PluginSettings.vue 第 34-121 行用了 `<ConfigFieldItem>`，但 `<script setup>` 块需要确认 import 了它。

### 1.4 假设 2：`configLoaded` 一直 false（async 时序）

`onMounted` 调 `loadConfig()`，async 完成后设 `configLoaded = true`。如果 `loadConfig` 失败或 hang，configLoaded 永远 false，整个 template 不渲染。

## 2. Proposed Changes

### 2.1 修复 A（必做）：HomePage.vue 补 import

**文件**：`/workspace/app/encv-mobile/src/views/HomePage.vue`

**改动**：在 `<script setup>` 顶部 import 区加：
```ts
import AgentEntry from '@/components/agent/AgentEntry.vue'
```

放在 import `useRouter` 后、`onIonViewWillEnter` 前：
```ts
import { useRouter } from 'vue-router'
import { onIonViewWillEnter } from '@ionic/vue'
import AgentEntry from '@/components/agent/AgentEntry.vue'  // ← 新增
```

**为什么不是 `<script setup>` 末尾**：Vue 3 setup 块 import 必须在 `const/function` 声明之前（虽然不是强约束，但规范）。

### 2.2 修复 B（诊断 + 修复）：PluginSettings.vue 缺 ConfigFieldItem import

**文件**：`/workspace/app/encv-mobile/src/views/PluginSettings.vue`

**步骤 1**：grep 看 PluginSettings.vue 的 script setup 块是否 import 了 ConfigFieldItem。

**步骤 2**：如果没 import → 补上（参考 Settings.vue 的 import 模式）。

**步骤 3**：浏览器打开 `/tabs/settings/plugins`，看 DevTools console：
- 如果有 `Failed to resolve component: ConfigFieldItem` → 补 import
- 如果有 `Cannot read properties of undefined (reading 'properties')` → pluginSection 解析 bug，需改 schemaParser
- 如果整个页面空白（loading 状态） → configLoaded 没设 true，需修 async 时序

### 2.3 修复 C（可选，UX 改进）：plugin_settings 也在 Settings 主页面展开

当前 Settings.vue 第 148-162 行只显示一个 goPlugins 跳转按钮，**不展开** 7 个 plugin 子项。但用户说"崩了没渲染出来"可能是想**直接看到**插件配置项。

**两种方案**：
- **A. 维持现状**（点 goPlugins 跳二级页 PluginSettings.vue）—— 修复 2.2 后应该能正常显示
- **B. 主页面展开 7 个 plugin** —— 修改 v-for 让 plugin_settings 也走 v-else 分支（type=object + properties 展开）

**先选 A**（最小改动），B 作为 v2 UX 改进议题。

## 3. 文件改动清单

| 文件 | 改动 | 优先级 |
|------|------|--------|
| [HomePage.vue](file:///workspace/app/encv-mobile/src/views/HomePage.vue) | `<script setup>` 补 `import AgentEntry from '@/components/agent/AgentEntry.vue'` | P0 必做 |
| [PluginSettings.vue](file:///workspace/app/encv-mobile/src/views/PluginSettings.vue) | grep 确认 `ConfigFieldItem` 是否 import，若没则补 | P0 必做 |

## 4. 验证步骤

1. **应用修复 A**：编辑 HomePage.vue 加 import
2. **应用修复 B**：grep 确认 + 补 import
3. **Vite HMR 等待**（5-10s）或重启 vite：`pm2 restart encv-mobile-vite`（start-preview 内部启的 vite 也在监管下）
4. **浏览器检查**：
   - http://localhost:16666/ → HomePage 看到**右下角浮动 AI 按钮**（紫色 sparkles 图标）
   - Settings tab → 看到「AI 助手」按钮 + 「插件配置」按钮
   - 点「AI 助手」→ 跳 /tabs/settings/agent → AgentSettingsDetail.vue 渲染 10 个配置项
   - 点「插件配置」→ 跳 /tabs/settings/plugins → PluginSettings.vue 渲染 7 个 plugin 子项
5. **DevTools console**：无 `Failed to resolve component` 警告
6. **Vue 测试**：`cd /workspace/app/encv-mobile && pnpm test:run` 全部通过

## 5. 风险

| 风险 | 缓解 |
|------|------|
| 补 import 后 Vue 编译时还有别的 component 缺失 | 浏览器 console 看完整 warning 列表 |
| PluginSettings 实际不是 import 问题而是 pluginSection = undefined | 需在浏览器 console 看运行时错误，针对性修复 |
| 修复后 tab 切换异常（参见 capacitor.md §6 渲染异常传播） | 加 try-catch 包裹 v-for 渲染，或把渲染拆细 |

## 6. 不在本次范围

- plugin_settings 主页面展开（v2 UX）
- agent_settings 主页面展开（v2 UX）
- HomePage 视觉重构（不动）

## 7. 预计改动量

- 2 个文件，2-3 行 import
- 预计 15-30 分钟（10 分钟编辑 + 5-10 分钟浏览器验证 + 5 分钟 console 诊断）
