# Plan：将 server/admin/webdav 配置从一级页面移到三级子页面

## 一、现状分析

### 当前配置层级

```
Level 1 (/tabs/settings — Settings.vue)
├── 外观 / 播放器 / 连接（入口） / 缓存
├── 🔴 Schema 驱动的配置字段（直接平铺展示）：
│   ├── password          ← 全局密码
│   ├── recover           ← 恢复模式
│   ├── output_path      ← 输出目录
│   ├── server           ← HTTP Server: port, dir    ⚠️ 应在三级
│   ├── admin            ← Admin Server: password     ⚠️ 应在三级
│   ├── webdav           ← WebDAV Server: root, dir, username, password  ⚠️ 应在三级
│   ├── proxy            ← Openlist 代理
│   └── log              ← 日志级别/文件
├── 插件设置 → goPlugins() (Level 2)
├── DevTools / 关于 / 编辑原始配置

Level 2 (/tabs/settings/server — ServerDetail.vue)
├── 连接：服务器地址(只读) + 状态控制(刷新/停止/重启)
└── 服务地址（只读 URL 展示，不可点击深入）：
    ├── HTTP Server     → baseUrl              ⚠️ 应该可点击→三级
    ├── Admin Server    → baseUrl/admin         ⚠️ 应该可点击→三级
    └── WebDAV Server   → baseUrl + webdav.root  ⚠️ 应该可点击→三级
└── 权限 [native only]
```

### 问题

`server`、`admin`、`webdav` 三个配置段作为**服务端基础设施配置**，却和 `password`、`log` 等全局配置**混在同一个平面**上展示。用户需要进入「后端服务」详情页后，却发现三个服务的配置项散落在主页面上——认知断裂。

### 目标

将 `server` / `admin` / `webdav` 三个配置段的编辑能力从 Settings.vue (L1) 移到 ServerDetail.vue 内部的三个独立子页 (L3)，使配置层级与物理层级对齐：

| 服务 | 配置键 | 字段 | 目标位置 |
|------|--------|------|---------|
| HTTP Server | `server` | port, dir | Level 3: `/tabs/settings/server/http` |
| Admin Server | `admin` | password | Level 3: `/tabs/settings/server/admin` |
| WebDAV Server | `webdav` | root, dir, username, password | Level 3: `/tabs/settings/server/webdav` |

---

## 二、目标结构

```
Level 1 (/tabs/settings — Settings.vue)
├── 外观 / 播放器 / 连接（入口） / 缓存
├── Schema 配置（已移除 server/admin/webdav）：
│   ├── password / recover / output_path
│   ├── proxy（Openlist 代理）
│   └── log（日志）
├── 插件设置 / DevTools / 关于 / 编辑原始配置

Level 2 (/tabs/settings/server — ServerDetail.vue) [修改]
├── 连接：服务器地址 + 状态控制（不变）
├── 服务地址（改为可点击入口）：
│   ├── 🌐 HTTP Server     [port: 2025]  → 点击 → Level 3
│   ├── 🔐 Admin Server    [已配置]       → 点击 → Level 3
│   ├── 📁 WebDAV Server   [/webdav/]     → 点击 → Level 3
└── 权限 [native only]（不变）

Level 3 (新建 × 3)
├── /tabs/settings/server/http    → HttpServerDetail.vue   (server.port, server.dir)
├── /tabs/settings/server/admin   → AdminServerDetail.vue  (admin.password)
└── /tabs/settings/server/webdav  → WebdavServerDetail.vue (webdav.* 全部字段)
```

---

## 三、实施步骤

### Step 1：新建 HttpServerDetail.vue（HTTP Server 三级页面）

**文件**：`src/views/HttpServerDetail.vue`

**功能**：编辑 `server.port` 和 `server.dir`

**Schema 驱动模式（100% 复用 PluginSettings.vue 已验证模式）**：

```typescript
// ✅ 导入与 PluginSettings.vue:230,235 完全一致
import type { FieldDef } from '@/config/schemaParser'
import { parseSchema } from '@/config/schemaParser'
import { useConfig } from '@/composables/useConfig'
import ConfigFieldItem from '@/components/ConfigFieldItem.vue'

const { getFieldValue, setFieldValue, dirty, saveConfig, configLoading } = useConfig()

// ✅ 与 PluginSettings.vue:311 同样的 .find() 模式
const sectionDef = computed(() => parseSchema().find(s => s.key === 'server')!)
const childFields = computed(() => sectionDef.value?.properties ?? [])
```

**模板（复用 ConfigFieldItem，Schema 驱动渲染）**：
```vue
<ion-page>
  <ion-header>
    <ion-toolbar>
      <ion-buttons slot="start">
        <ion-back-button default-href="/tabs/settings/server"></ion-back-button>
      </ion-buttons>
      <ion-title>{{ t('settings.httpServer') }}</ion-title>
    </ion-toolbar>
  </ion-header>
  <ion-content>
    <ion-list>
      <ion-list-header><ion-label>{{ t('settings.httpServerSettings') }}</ion-label></ion-list-header>
      <!-- ✅ 与 Settings.vue L195-L262 相同的 ConfigFieldItem 循环模式 -->
      <template v-for="field in childFields" :key="field.key">
        <ConfigFieldItem :field="field"
          :model-value="getFieldValue(['server', field.key])"
          @update:model-value="setFieldValue(['server', field.key], $event)"
          ... />
      </template>
    </ion-list>
    <ion-button expand="block" @click="handleSave" :disabled="!dirty || configLoading">
      {{ t('settings.saveConfig') }}
    </ion-button>
  </ion-content>
</ion-page>
```

### Step 2：新建 AdminServerDetail.vue（Admin Server 三级页面）

**文件**：`src/views/AdminServerDetail.vue`

**功能**：编辑 `admin.password`

**字段**：`admin.password` — 密码输入框

**与 Step 1 结构完全相同**，仅 `sectionKey` 改为 `'admin'`：
```typescript
const sectionDef = computed(() => parseSchema().find(s => s.key === 'admin')!)
// 模板中: getFieldValue(['admin', field.key]) / setFieldValue(['admin', field.key], $event)
```

### Step 3：新建 WebdavServerDetail.vue（WebDAV Server 三级页面）

**文件**：`src/views/WebdavServerDetail.vue`

**功能**：编辑 `webdav` 全部 4 个字段

**字段**：`webdav.root`, `webdav.dir`, `webdav.username`, `webdav.password`

**与 Step 1 结构完全相同**，仅 `sectionKey` 改为 `'webdav'`：
```typescript
const sectionDef = computed(() => parseSchema().find(s => s.key === 'webdav')!)
// 模板中: getFieldValue(['webdav', field.key]) / setFieldValue(['webdav', field.key], $event)
```

**额外功能**：
- 复用 ServerDetail.vue 的 `testLocalWebDAV()` 逻辑，提供「测试连接」按钮
- 显示 WebDAV 测试结果

### Step 4：注册 3 条新路由

**文件**：`src/router/index.ts`

在现有 `settings/server` 路由之后添加：

```typescript
{ path: 'settings/server/http',   component: () => import('@/views/HttpServerDetail.vue') },
{ path: 'settings/server/admin',  component: () => import('@/views/AdminServerDetail.vue') },
{ path: 'settings/server/webdav', component: () => import('@/views/WebdavServerDetail.vue') },
```

完整顺序：
```
settings              → Settings.vue (L1)
settings/server       → ServerDetail.vue (L2)
settings/server/http   → HttpServerDetail.vue (L3)   ← NEW
settings/server/admin  → AdminServerDetail.vue (L3)  ← NEW
settings/server/webdav → WebdavServerDetail.vue (L3) ← NEW
settings/engine       → EngineDetail.vue (L2)
...
```

### Step 5：修改 ServerDetail.vue — 服务地址变为可点击入口

**文件**：`src/views/ServerDetail.vue`

**改动 L59-L77**（服务地址 `<ion-list>` 区域）：

将当前的只读 URL 展示项改为**可点击的导航入口**：

```vue
<!-- Before: 只读 URL 列表 -->
<ion-item v-for="svc in serviceUrls" :key="svc.label">
  <ion-label><h3>{{ svc.label }}</h3><p class="readonly-url">{{ svc.url }}</p></ion-label>
  <ion-button slot="end" ...>复制/测试</ion-button>
</ion-item>

<!-- After: 可点击入口 + 当前值摘要 -->
<ion-item button @click="goHttpServer" detail>
  <ion-icon :icon="cloudOutline" slot="start"></ion-icon>
  <ion-label>
    <h3>{{ t('settings.httpServer') }}</h3>
    <p>:{{ httpPort }} {{ rootDir }}</p>
  </ion-label>
</ion-item>

<ion-item button @click="goAdminServer" detail>
  <ion-icon :icon="shieldCheckmark" slot="start"></ion-icon>
  <ion-label>
    <h3>{{ t('settings.adminServer') }}</h3>
    <p>{{ adminConfigured ? t('settings.configured') : t('settings.notConfigured') }}</p>
  </ion-label>
</ion-item>

<ion-item button @click="goWebdavServer" detail>
  <ion-icon :icon="globeOutline" slot="start"></ion-icon>
  <ion-label>
    <h3>{{ t('settings.webdavServer') }}</h3>
    <p>{{ webdavRoot }} {{ webdavUsername ? '@' + webdavUsername : '' }}</p>
  </ion-label>
</ion-item>
```

**新增导航函数**：
```typescript
function goHttpServer() { router.push('/tabs/settings/server/http') }
function goAdminServer() { router.push('/tabs/settings/server/admin') }
function goWebdavServer() { router.push('/tabs/settings/server/webdav') }
```

**新增 computed**（从 configData 中提取摘要信息）：
```typescript
const httpPort = computed(() => (configData.value?.server as any)?.port ?? '-')
const rootDir = computed(() => (configData.value?.server as any)?.dir ?? '/')
const adminConfigured = computed(!!(configData.value?.admin as any)?.password)
const webdavRoot = computed(() => (configData.value?.webdav as any)?.root ?? '/')
const webdavUsername = computed(() => (configData.value?.webdav as any)?.username ?? '')
```

### Step 6：修改 Settings.vue — 排除 server/admin/webdav 段（仅模板层过滤）

**文件**：`src/views/Settings.vue`

**核心原则：不修改 Schema 生产链路，仅在消费层过滤。**

#### 6.1 Schema 驱动链路完整性保证

```
schema.json ──→ schemaParser.parseSchema() ──→ useConfig.schemaFields ──→ 消费者
   ✅ 不改           ✅ 不改                    ✅ 不改              ⚠️ 仅此处过滤
```

| 层 | 文件 | 改动？ | 原因 |
|---|------|--------|------|
| Schema 定义 | `config/schema.json` | ❌ 不改 | 数据源不变 |
| Schema 解析 | `config/schemaParser.ts` | ❌ 不改 | `parseSchema()` 返回完整 FieldDef[] |
| Config 状态 | `composables/useConfig.ts` | ❌ 不改 | `schemaFields` computed 保持原样 |
| 插件设置页 | `views/PluginSettings.vue` | ❌ 不受影响 | 用 `.find(s => s.key === 'plugin_settings')` 取值，不过滤 |
| **设置主页** | **`views/Settings.vue`** | **⚠️ 模板 v-if 过滤** | **消费层跳过 3 个 key** |
| L3 子页面 | 新建 × 3 | ✅ 新增 | 各自调用 `parseSchema().find(s => s.key === target)` 获取 |

#### 6.2 具体改动

**方式 A（推荐）：模板层 v-if 包裹**

改动位置：L140-L264 的 `<template v-for="section in schemaFields">` 循环

```vue
<!-- Before: 渲染所有 schema 段 -->
<template v-for="section in schemaFields" :key="section.key">
  <!-- 原有渲染逻辑全部保留在内部 -->
</template>

<!-- After: 消费层过滤，生产层不动 -->
<template v-for="section in schemaFields" :key="section.key">
  <!-- server/admin/webdav 已迁移到 ServerDetail 的三级子页 -->
  <template v-if="!['server', 'admin', 'webdav'].includes(section.key)">
    <!-- 原有渲染逻辑一字不改 -->
    <ion-list v-if="section.key === 'plugin_settings'">...</ion-list>
    <ion-list v-else-if="section.type !== 'object' || !section.properties">...</ion-list>
    <ion-list v-else>...</ion-list>
  </template>
</template>
```

**为什么不用 computed 过滤 schemaFields？**
- `schemaFields` 是 useConfig 导出的共享 computed
- PluginSettings.vue 也导入同一个 `schemaFields`
- 如果在 useConfig 中过滤，PluginSettings 会受影响
- **最安全：每个消费者自行决定渲染哪些字段**

#### 6.3 L3 页面如何获取 Schema 定义

L3 页面不依赖 Settings.vue 传递 props，而是**独立从 Schema 源获取**：

```typescript
// HttpServerDetail.vue / AdminServerDetail.vue / WebdavServerDetail.vue
import { parseSchema } from '@/config/schemaParser'
import { useConfig } from '@/composables/useConfig'
import type { FieldDef } from '@/config/schemaParser'

const { getFieldValue, setFieldValue, dirty, saveConfig } = useConfig()

// 独立解析 schema，获取目标段的 FieldDef（含 properties 子字段）
const sectionKey = 'server'  // | 'admin' | 'webdav'
const sectionDef = parseSchema().find(s => s.key === sectionKey)
const childFields = computed(() => sectionDef?.properties ?? [])
```

这样：
- ✅ 与 Settings.vue 使用**同一份 Schema 数据源**
- ✅ 通过**同一份 useConfig state** 读写值
- ✅ 保存时 `dirty` 标记正确传播
- ✅ 返回 Settings/ServerDetail 时能看到最新值

### Step 7：添加 i18n 键

**文件**：`src/composables/useI18n.ts`

新增键（中英文）：

```typescript
// 中文
'settings.httpServer': 'HTTP 服务器',
'settings.httpServerSettings': '内置 HTTP 服务器设置',
'settings.adminServer': '管理后台',
'settings.webdavServer': 'WebDAV 服务器',
'settings.configured': '已配置',
'settings.notConfigured': '未配置',
'settings.saveConfig': '保存',

// English
'settings.httpServer': 'HTTP Server',
'settings.httpServerSettings': 'Built-in HTTP Server Settings',
'settings.adminServer': 'Admin Panel',
'settings.webdavServer': 'WebDAV Server',
'settings.configured': 'Configured',
'settings.notConfigured': 'Not configured',
'settings.saveConfig': 'Save',
```

### Step 8：验证

1. **vue-tsc** 零错误
2. **vitest** 全部通过（208/208）
3. **vite build** 成功
4. **手动验证路径**：
   - Settings → 后端服务 → HTTP Server → 编辑 port/dir → 保存 → 返回可见值更新
   - Settings → 后端服务 → Admin Server → 编辑 password → 保存
   - Settings → 后端服务 → WebDAV Server → 编辑全部字段 → 测试连接 → 保存
   - Settings 主页确认 `server`/`admin`/`webdav` 段不再出现
   - 其他配置段（proxy/log/password 等）不受影响

---

## 四、影响范围

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `src/views/HttpServerDetail.vue` | **新建** | L3 HTTP Server 配置页 |
| `src/views/AdminServerDetail.vue` | **新建** | L3 Admin Server 配置页 |
| `src/views/WebdavServerDetail.vue` | **新建** | L3 WebDAV Server 配置页 |
| `src/router/index.ts` | **修改** | 新增 3 条路由 |
| `src/views/ServerDetail.vue` | **修改** | 服务地址区改为可点击入口 |
| `src/views/Settings.vue` | **修改** | schema 渲染排除 server/admin/webdav |
| `src/composables/useI18n.ts` | **修改** | 新增 ~7 个 i18n 键 |

**不涉及的文件**：
- ❌ EngineDetail.vue
- ❌ CacheDetail.vue
- ❌ PluginSettings.vue
- ❌ AboutDetail.vue
- ❌ DevToolsDetail.vue
- ❌ config/schema.json（不改 schema，只改渲染过滤）

---

## 五、Schema 驱动兼容性证明

### 5.1 L3 页面 vs PluginSettings.vue 对照表

| 行为 | PluginSettings.vue (已验证) | L3 新页面 (Http/Admin/Webdav) |
|------|---------------------------|------------------------------|
| 获取 Schema 段 | `parseSchema().find(s => s.key === 'plugin_settings')` | `parseSchema().find(s => s.key === 'server'\|'admin'\|'webdav')` ✅ 同模式 |
| 读取字段值 | `getFieldValue(['plugin_settings', field.key])` | `getFieldValue(['server', field.key])` ✅ 同模式 |
| 写入字段值 | `setFieldValue(['plugin_settings', field.key], val)` | `setFieldValue(['server', field.key], val)` ✅ 同模式 |
| 渲染组件 | `<ConfigFieldItem :field="field" ...>` | `<ConfigFieldItem :field="field" ...>` ✅ 同组件 |
| 保存状态 | `dirty` / `saveConfig()` from useConfig | `dirty` / `saveConfig()` from useConfig ✅ 同 state |
| 导入来源 | schemaParser + useConfig + ConfigFieldItem | schemaParser + useConfig + ConfigFieldItem ✅ 完全一致 |

### 5.2 Schema 变更自动传播路径

```
schema.json 改动（如 server 新增字段）
    ↓
schemaParser.parseSchema() 自动解析新字段     ← 不改此文件
    ↓
useConfig.schemaFields 自动包含新字段          ← 不改此文件
    ↓
├── Settings.vue: v-if 过滤 server → 不显示   ← 消费层过滤，不影响其他消费者
├── PluginSettings.vue: .find('plugin_settings') → 不受影响 ← 独立取值
└── HttpServerDetail.vue: .find('server') → 自动获取新字段 ✅ L3 页面零改动即可渲染
```

### 5.3 验证清单

| 验证项 | 方法 | 通过条件 |
|--------|------|---------|
| Schema 解析不变 | grep `parseSchema` — 无改动 | ✅ 原样保留 |
| useConfig 不变 | diff useConfig.ts — 零改动 | ✅ 原样保留 |
| PluginSettings 不受影响 | 访问插件设置页 — 字段正常渲染 | ✅ 独立 find 取值 |
| L3 页面 Schema 驱动 | 修改 schema.json 中 server 段 → L3 页面自动反映 | ✅ 复用 parseSchema |
| ConfigState 共享 | L3 页面修改值 → Settings 页可见变更 | ✅ 同一 reactive 对象 |
| dirty 标记传播 | L3 页面编辑后未保存 → 返回时提示未保存 | ✅ 共享 dirty ref |
