# 远端页面重构方案：WebDAV + Openlist 站点（v2）

## 现状分析

### 当前架构
- **Tab 栏**：文件 / 任务 / WebDAV / 设置 / DevLogs
- **WebDAV 页面**：纯前端 localStorage 存储的 WebDAV 配置列表
- **后端 WebDAV 服务器**：`config.webdav`（port/root/dir/username/password）
- **后端 Openlist 代理**：`config.proxy.sites` map（siteId → {host, description}）
- **后端 API**：`/api/config` GET/PUT 完整配置，`/api/config/schema` GET 配置 schema
- **配置 schema**：`config.schema.json` 包含每个字段的 `description` 注释

### 核心需求
1. WebDAV 页面改为"远端"页面，顶部 Tab 切换 WebDAV / Openlist 站点
2. encv 自身提供的 WebDAV 应当自动识别并添加到 WebDAV 列表
3. Openlist Tab 支持增删改查（用户友好）
4. 设置界面只读不可编辑（配置项通过专用页面管理）
5. 设置界面增加编辑原始配置 JSON 文件的功能，提供 JSON 校验与配置注释显示

---

## 实现步骤

### Step 1：后端 — 新增 `/api/remote/info` 端点

**文件**：`internal/server/server.go` + `internal/server/mobile_api.go`

新增 API 端点，返回远端服务信息：

```json
GET /api/remote/info
{
  "webdav": {
    "enabled": true,
    "url": "http://127.0.0.1:8080/webdav/",
    "username": "admin",
    "root": "/webdav/"
  },
  "openlistSites": {
    "myalist": {
      "host": "http://192.168.1.100:5244",
      "description": "我的Alist",
      "proxyUrl": "http://127.0.0.1:2025/openlist/sites/myalist/"
    }
  }
}
```

关键逻辑：
- `webdav.enabled`：`config.Webdav.Port > 0`
- `webdav.url`：`http://127.0.0.1:{config.Webdav.Port}{config.Webdav.Root}`
- `openlistSites`：遍历 `config.Proxy.Sites`，为每个站点生成代理 URL
- 代理 URL 格式：`http://127.0.0.1:{config.Server.Port}/openlist/sites/{siteId}/`

### Step 2：后端 — 新增 Openlist 站点 CRUD API

**文件**：`internal/server/mobile_api.go`

```
POST   /api/remote/openlist       — 添加站点 {siteId, host, description}
PUT    /api/remote/openlist/:id   — 更新站点 {host, description}
DELETE /api/remote/openlist/:id   — 删除站点
```

逻辑：
- 读取当前配置 → 修改 `proxy.sites` → 写回配置文件（复用 `handlePutConfig` 的写入逻辑）
- siteId 作为 map key，必须合法（字母数字下划线）
- 添加/删除后通知 Openlist 代理服务刷新

### Step 3：前端 — API 层扩展

**文件**：`src/api/encv.ts`

1. 新增类型：
```ts
export interface RemoteWebDAVInfo {
  enabled: boolean
  url: string
  username: string
  root: string
}

export interface OpenlistSiteInfo {
  host: string
  description: string
  proxyUrl: string
}

export interface RemoteInfo {
  webdav: RemoteWebDAVInfo
  openlistSites: Record<string, OpenlistSiteInfo>
}
```

2. 新增 API 函数：
- `fetchRemoteInfo(): Promise<RemoteInfo>`
- `addOpenlistSite(siteId: string, host: string, description: string): Promise<void>`
- `updateOpenlistSite(siteId: string, host: string, description: string): Promise<void>`
- `deleteOpenlistSite(siteId: string): Promise<void>`

3. 扩展 `WebDAVConfig` 添加 `isBuiltIn?: boolean`

### Step 4：前端 — 重构 WebDAV.vue 为 Remote.vue

**文件**：`src/views/WebDAV.vue` → 重命名为 `src/views/Remote.vue`

页面结构：
```
┌─────────────────────────────────────┐
│ 远端                                 │ ← ion-title
├─────────────────────────────────────┤
│ [WebDAV]  [Openlist]                │ ← ion-segment 顶部 Tab
├─────────────────────────────────────┤
│                                     │
│  WebDAV Tab:                        │
│  ┌─────────────────────────────────┐│
│  │ 🏠 本机 WebDAV    ● 已启用      ││ ← 自动识别，不可删除
│  │ http://127.0.0.1:8080/webdav/   ││
│  └─────────────────────────────────┘│
│  ┌─────────────────────────────────┐│
│  │ ☁️ 我的NAS       ○ 已保存       ││ ← 用户手动添加
│  │ https://nas.local/webdav        ││
│  └─────────────────────────────────┘│
│  [+ 添加 WebDAV]                    │ ← FAB 按钮
│                                     │
│  Openlist Tab:                      │
│  ┌─────────────────────────────────┐│
│  │ 📂 myalist      我的Alist       ││ ← 可增删改查
│  │ http://192.168.1.100:5244       ││
│  │ 代理: /openlist/sites/myalist/  ││
│  └─────────────────────────────────┘│
│  [+ 添加站点]                       │ ← FAB 按钮
│                                     │
└─────────────────────────────────────┘
```

关键逻辑：
1. **顶部 Segment**：`ion-segment` 切换 WebDAV / Openlist
2. **WebDAV Tab**：
   - 调用 `fetchRemoteInfo()` 获取本机 WebDAV 信息
   - 如果 `webdav.enabled`，自动在列表顶部添加"本机 WebDAV"项（标记 `isBuiltIn=true`）
   - 本机 WebDAV 项：显示 URL + 用户名，不可删除/编辑，可测试连接
   - 用户手动添加的 WebDAV：保持现有逻辑（增删改查 + 测试）
3. **Openlist Tab**：
   - 从 `fetchRemoteInfo()` 获取站点列表
   - 每个站点显示：站点 ID + 描述 + 原始 Host + 代理 URL
   - 支持增删改查：添加/编辑用 Modal（siteId + host + description），删除用滑动按钮
   - 点击站点可复制代理 URL

### Step 5：前端 — 设置界面改为只读 + JSON 编辑器

**文件**：`src/views/Settings.vue`

1. **当前设置项改为只读展示**：
   - 所有 `ion-input` 改为 `ion-label` 只读展示
   - 底部添加"编辑原始配置"按钮，打开 JSON 编辑器 Modal

2. **JSON 编辑器 Modal**：
   ```
   ┌─────────────────────────────────────┐
   │ 编辑配置                    [保存]  │ ← ion-toolbar
   ├─────────────────────────────────────┤
   │ ┌─ 配置注释 ─────────────────────┐  │ ← 左侧/上方注释面板
   │ │ password: 用于加密和解密视频... │  │
   │ │ server.port: HTTP服务器端口... │  │
   │ └─────────────────────────────────┘  │
   │ ┌─ JSON 编辑器 ─────────────────┐  │ ← 右侧/下方编辑器
   │ │ {                              │  │
   │ │   "password": "xxx",           │  │
   │ │   "server": {                  │  │
   │ │     "port": 2025               │  │
   │ │   },                           │  │
   │ │   ...                          │  │
   │ │ }                              │  │
   │ └─────────────────────────────────┘  │
   │ ⚠️ JSON 格式错误：第 5 行缺少逗号   │ ← 校验错误提示
   └─────────────────────────────────────┘
   ```

   关键实现：
   - 使用 `<textarea>` 作为 JSON 编辑器（简单可靠）
   - 从 `/api/config` 获取当前配置 JSON
   - 从 `/api/config/schema` 获取 schema，解析 `description` 字段生成注释面板
   - 实时 JSON 校验：`JSON.parse()` 失败时显示错误行号
   - Schema 校验：与 schema 对比，检查必填字段和类型
   - 保存时调用 `updateConfig()` PUT 回后端
   - 注释面板：解析 schema 的 `$defs` → 每个字段的 `description`，按层级展示

3. **PluginSettings.vue 同样处理**：
   - 插件配置项改为只读展示
   - 添加"编辑插件配置"按钮，打开 JSON 编辑器（只编辑 `plugin_settings` 部分）

### Step 6：前端 — 路由和 Tab 更新

**文件**：`src/router/index.ts`

- 路由路径从 `webdav` 改为 `remote`

**文件**：`src/views/Tabs.vue`

- Tab 从 `webdav` 改为 `remote`
- 图标从 `cloud` 改为 `globe`
- 标签从 `WebDAV` 改为 `远端` / `Remote`

### Step 7：前端 — i18n 翻译

**文件**：`src/composables/useI18n.ts`

新增翻译键：
```
tabs.remote: 远端 / Remote
remote.title: 远端 / Remote
remote.webdav: WebDAV
remote.openlist: Openlist
remote.builtInWebdav: 本机 WebDAV / Built-in WebDAV
remote.enabled: 已启用 / Enabled
remote.disabled: 未启用 / Disabled
remote.noOpenlistSites: 暂无 Openlist 站点 / No Openlist sites
remote.noOpenlistSitesDesc: 点击右下角按钮添加 Openlist 代理站点 / Tap the button to add an Openlist proxy site
remote.proxyUrl: 代理地址 / Proxy URL
remote.host: 原始地址 / Host
remote.siteId: 站点 ID / Site ID
remote.siteIdPlaceholder: myalist / myalist
remote.hostPlaceholder: http://192.168.1.100:5244 / http://192.168.1.100:5244
remote.description: 描述 / Description
remote.descriptionPlaceholder: 我的Alist / My Alist
remote.addSite: 添加站点 / Add Site
remote.editSite: 编辑站点 / Edit Site
remote.copied: 已复制 / Copied
settings.editRawConfig: 编辑原始配置 / Edit Raw Config
settings.configAnnotations: 配置注释 / Config Annotations
settings.jsonError: JSON 格式错误 / JSON Format Error
settings.saveConfig: 保存配置 / Save Config
settings.configSaved: 配置已保存 / Config Saved
settings.configSaveFailed: 保存失败 / Save Failed
```

---

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `internal/server/server.go` | 注册 `/api/remote/info` + Openlist CRUD 路由 |
| `internal/server/mobile_api.go` | 新增 `handleRemoteInfo` + Openlist CRUD 处理函数 |
| `src/api/encv.ts` | 新增 RemoteInfo 类型 + fetchRemoteInfo + Openlist CRUD API |
| `src/views/WebDAV.vue` → `src/views/Remote.vue` | 重构为远端页面（Segment Tab + 本机 WebDAV + Openlist CRUD） |
| `src/views/Settings.vue` | 设置项改为只读 + JSON 编辑器 Modal |
| `src/views/PluginSettings.vue` | 插件配置改为只读 + JSON 编辑器 Modal |
| `src/router/index.ts` | 路由从 webdav 改为 remote |
| `src/views/Tabs.vue` | Tab 从 webdav 改为 remote，图标改 globe |
| `src/composables/useI18n.ts` | 新增远端 + JSON 编辑器翻译 |

## 优先级

1. **Step 1-4**（远端页面核心）：后端 API + 前端重构 — 用户可见的核心功能
2. **Step 5**（设置界面改造）：只读 + JSON 编辑器 — 配置管理体验提升
3. **Step 6-7**（路由和翻译）：收尾工作

## 风险与边界

- **Openlist CRUD 写入配置文件**：需要加锁防止并发写入冲突，复用 `configMu` 互斥锁
- **JSON 编辑器保存**：用户可能输入无效 JSON 导致配置损坏，需要严格的校验 + 保存前备份
- **配置热更新**：保存配置后，后端服务需要重新加载配置（当前可能需要重启）
- **siteId 合法性**：只允许字母数字下划线，防止路径注入
