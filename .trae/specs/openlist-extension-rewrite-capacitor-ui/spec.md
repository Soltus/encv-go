# OpenList 扩展重写 Spec（ComboLite 扩展 + Capacitor 嵌入式 UI + 资源共享）

## Why

经过多轮澄清，最终需求是：

1. ✅ **保持 ComboLite 扩展形态**——plugin-openlist 仍是插件 APK（被主 app 通过 ComboLite 加载），**不是**独立应用
2. ✅ **插件 UI 用 Capacitor 实现**——但 Capacitor runtime 是**嵌入式**的（不是独立 Capacitor 应用）
3. ✅ **与主应用共享资源**——包括 UI 组件、状态、配置等

之前几版 spec 的错误方向：
- ❌ V1: 把 Compose UI 改为 Capacitor 多例 WebView
- ❌ V2: 让 plugin-openlist 变成独立 Capacitor Android 应用
- ❌ V3: 在主 app Ionic Vue 里重建 OpenList UI

正确方向：
- ✅ plugin-openlist 仍是 ComboLite 插件（被主 app 加载到 ProcessPool）
- ✅ 插件 `Content()` 内部嵌入 Capacitor runtime，加载**共享的 web 资源**
- ✅ web 资源由主 app 与插件**共享**（避免重复打包、共享 npm 包）

---

## Part A: ComboLite + Capacitor 嵌入式架构

### A.1 架构总览

```
┌─ 主 ENCV App (com.encvgo.app) ──────────────────────────────────┐
│                                                                 │
│  Capacitor 运行时 (已存在)                                       │
│  └─ 主 app web 资源 (src/)                                      │
│      ├─ 公共 Vue 组件库 (@/components/...)                       │
│      ├─ 公共 composable (@/composables/...)                      │
│      ├─ 公共主题/样式 (@/theme/...)                              │
│      └─ 业务页面 (Files/Tasks/Settings/etc.)                     │
│                                                                 │
│  ComboLite 框架 (combolite-host AAR)                            │
│  └─ PluginLifecycleEngine                                       │
│      └─ 加载 plugin-openlist.apk → 实例化 OpenListPluginEntry   │
│          └─ 渲染 Content() → 嵌入式 Capacitor WebView            │
│              └─ 加载共享 web 资源                                │
│                  └─ OpenList 业务页面 (复用主 app 组件)          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
        ↓ 跨进程 (主 app 进程 ⇄ plugin-openlist 进程)
┌─ plugin-openlist APK (ComboLite 插件) ──────────────────────────┐
│                                                                 │
│  OpenListPluginEntry                                            │
│  ├─ onLoad / onUnload: 初始化 OpenListBridge (gomobile bind)   │
│  └─ Content():                                                   │
│      └─ AndroidView(WebView) + 嵌入式 Capacitor runtime          │
│          └─ 加载主 app 共享的 web 资源（特定路由）               │
│              └─ OpenList 页面（与主 app 同一技术栈）            │
│                                                                 │
│  OpenListBridge (gomobile bind, 现有)                            │
│  OpenListService (Android 前台服务, 现有)                        │
│  OpenListConfig (配置持久化, 现有)                               │
│  OpenListStatusProvider (ContentProvider IPC, 现有)              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### A.2 关键创新点：嵌入式 Capacitor + 资源共享

| 挑战 | 解决方案 |
|------|---------|
| **ComboLite 插件如何用 Capacitor 写 UI** | 在 `Content()` 内嵌入 `AndroidView(WebView)` + 桥接到主 app Capacitor runtime |
| **如何与主 app 共享 web 资源** | 通过 Capacitor 的 `server.url` 指向主 app dev server 或共享 assets 路径 |
| **如何与主 app 共享 Vue 组件** | 插件 web 资源使用 monorepo 结构（pnpm workspace），从 `@/components/...` 导入 |
| **如何避免重复打包** | 插件 web 资源只包含 OpenList 业务页面 + 共享组件的引用，主 app 包含全部组件 |
| **如何与主 app 通信** | 通过 Capacitor 桥接（主 app Capacitor 插件）调用主 app 已有的方法 |

### A.3 资源架构（pnpm workspace monorepo）

```
encv-mobile/                                  # 项目根
├── package.json                              # workspace root
├── pnpm-workspace.yaml                       # workspace 配置
├── app/                                      # 主 ENCV app
│   ├── src/                                  # 主 app 业务代码
│   ├── android/                              # 主 app Android
│   └── package.json
├── plugin-openlist/
│   ├── web/                                  # 插件 web 资源 (新)
│   │   ├── package.json                      # 引用 workspace 包
│   │   ├── src/
│   │   │   ├── views/
│   │   │   │   ├── OpenListHome.vue          # OpenList 主控页
│   │   │   │   ├── OpenListWebView.vue       # iframe 加载 OpenList SPA
│   │   │   │   └── OpenListSettings.vue
│   │   │   ├── components/                   # 复用主 app 组件
│   │   │   │   ├── OpenListStatusCard.vue    # 来自 @encvgo/components
│   │   │   │   └── OpenListLogList.vue
│   │   │   └── plugins/
│   │   │       └── OpenListService.ts        # 调主 app Capacitor 插件
│   │   ├── capacitor.config.ts
│   │   └── vite.config.ts
│   └── android/                              # 插件 Android（保持现状）
│       └── src/main/java/.../OpenListPluginEntry.kt
└── packages/                                  # 共享包 (新)
    └── components/                            # 共享 Vue 组件
        ├── package.json
        ├── src/
        │   ├── OpenListStatusCard.vue
        │   ├── OpenListLogList.vue
        │   └── index.ts
        └── tsconfig.json
```

### A.4 嵌入式 WebView 与主 app Capacitor 桥接

plugin-openlist 的 `Content()` 内部 WebView 通过**主 app 已有的 Capacitor 桥接**与底层 OpenList 通信：

```kotlin
// OpenListPluginEntry.kt (修复后)
@Composable
override fun Content() {
    // 嵌入式 WebView：直接通过 file:// 加载主 app assets 中的 openlist 子目录
    // 或通过 dev server 的 /openlist/ 子路径
    AndroidView(
        factory = { ctx ->
            WebView(ctx).apply {
                webViewClient = WebViewClient()
                // 关键：通过主 app Capacitor 桥接（共享 WebView 进程）
                // 这样插件 web 资源可以直接用 @capacitor/core 的 registerPlugin
                settings.javaScriptEnabled = true
                settings.domStorageEnabled = true
                loadUrl("https://localhost/openlist/")  // 主 app dev server 子路径
            }
        },
        update = { webView -> /* bounds update */ }
    )
}
```

### A.5 状态共享

| 状态 | 共享机制 |
|------|---------|
| **OpenList 运行时状态** | ContentProvider IPC（已有 OpenListStatusProvider + OpenListStatusBridge） |
| **Vue 组件** | pnpm workspace 共享（编译时复用） |
| **主题/样式** | 全局 CSS 变量（与主 app 一致） |
| **本地化（i18n）** | 共享 i18n 包，复用主 app 翻译 |
| **后端调用** | 全部走主 app Capacitor 插件（OpenListServicePlugin + 已有的 GoProcess 桥接） |

---

## Part B: ComboLite 合规修复（与 UI 无关）

参考 MpvPluginEntry 的模式（`pluginModule = emptyList()`，运行时内嵌在 Content 中），但本次要嵌入 WebView 而非 Compose：

### B.1 修复项

```kotlin
// OpenListPluginEntry.kt (合规修复版)
class OpenListPluginEntry : IPluginEntryClass {
    override val pluginModule = emptyList<Module>()  // 与 MpvPluginEntry 一致

    override fun onLoad(context: PluginContext) {
        // 初始化 OpenListBridge (gomobile bind)
        OpenListBridge.init(context)
    }

    override fun onUnload() {
        // shutdown Bridge + Service
        OpenListBridge.shutdown()
        OpenListService.stop()
    }

    @Composable
    override fun Content() {
        // 嵌入式 WebView + Capacitor 桥接（详见 Part C）
        OpenListEmbedWebView(...)
    }
}
```

### B.2 删除的 Compose UI 代码

- `StatusCard` (90 行) → 替换为 web 组件 `@encvgo/components/OpenListStatusCard.vue`
- `ControlCard` (60 行) → 替换为 web 组件 `OpenListScreen.vue` 中的 FAB
- `ConfigCard` (65 行) → 替换为 web 页面 `OpenListConfigEditor.vue`
- `InfoGrid` (35 行) → 替换为 web 组件
- `formatFileSize` (10 行) → 替换为 web 工具函数

### B.3 build.gradle.kts 变更

- **删除** `id("org.jetbrains.kotlin.plugin.compose")`
- **删除** `buildFeatures { compose = true }`
- **删除** compose BOM + ui/runtime/material3/icons-extended/lifecycle-runtime-compose
- **保留** combolite-core, openlist-classes.jar, core-ktx, localbroadcastmanager, koin-core
- **新增** （如未存在）`androidx.webkit:webkit` (WebView 增强)

---

## Part C: 嵌入式 WebView + Capacitor 桥接实现

### C.1 嵌入式 WebView 容器

```kotlin
// plugin-openlist/src/main/java/.../OpenListEmbedWebView.kt
@Composable
fun OpenListEmbedWebView(
    containerId: String = "openlist-container",
    initialPath: String = "/openlist"
) {
    val ctx = LocalContext.current
    val webViewRef = remember { mutableStateOf<WebView?>(null) }
    
    AndroidView(
        factory = { c ->
            WebView(c).apply {
                id = containerId.hashCode()
                webViewClient = OpenListWebViewClient()
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    allowFileAccess = true
                    allowContentAccess = true
                }
                // 关键：通过主 app Capacitor 桥接
                addJavascriptInterface(
                    OpenListPluginJSInterface(ctx),
                    "OpenListNative"
                )
                loadUrl("https://localhost$initialPath")
                webViewRef.value = this
            }
        },
        update = { /* bounds update */ }
    )
}
```

### C.2 JS-Native 桥接（共享主 app Capacitor runtime）

```kotlin
// plugin-openlist/src/main/java/.../OpenListPluginJSInterface.kt
class OpenListPluginJSInterface(private val context: Context) {
    @JavascriptInterface
    fun startOpenList(): String {
        // 调 OpenListBridge.start()
        val port = OpenListBridge.start()
        return port.toString()
    }
    
    @JavascriptInterface
    fun stopOpenList(): Boolean {
        return OpenListBridge.stop()
    }
    
    @JavascriptInterface
    fun getRuntimeStatus(): String {
        // 返回 JSON 字符串
        val status = OpenListBridge.snapshot()
        return JSONObject().apply {
            put("running", status.running)
            put("port", status.port)
            put("pid", status.pid)
        }.toString()
    }
    
    @JavascriptInterface
    fun setAdminPassword(password: String): Boolean {
        return OpenListBridge.setAdminPwd(password)
    }
    
    @JavascriptInterface
    fun readConfig(): String {
        return File(OpenListConfig.dataDir, "config.json").readText()
    }
    
    @JavascriptInterface
    fun writeConfig(content: String): Boolean {
        val configFile = File(OpenListConfig.dataDir, "config.json")
        val backup = File(OpenListConfig.dataDir, "config.json.bak")
        if (configFile.exists()) configFile.copyTo(backup, overwrite = true)
        return try {
            configFile.writeText(content)
            true
        } catch (e: Exception) {
            false
        }
    }
}
```

### C.3 web 端调用方式

```typescript
// plugin-openlist/web/src/plugins/openlist-native.ts
declare global {
    interface Window {
        OpenListNative: {
            startOpenList(): string
            stopOpenList(): boolean
            getRuntimeStatus(): string
            setAdminPassword(password: string): boolean
            readConfig(): string
            writeConfig(content: string): boolean
        }
    }
}

export const OpenListNative = {
    start: () => window.OpenListNative?.startOpenList() ?? '0',
    stop: () => window.OpenListNative?.stopOpenList() ?? false,
    getStatus: () => {
        const json = window.OpenListNative?.getRuntimeStatus() ?? '{}'
        return JSON.parse(json)
    },
    setPassword: (pwd: string) => window.OpenListNative?.setAdminPassword(pwd) ?? false,
    readConfig: () => window.OpenListNative?.readConfig() ?? '{}',
    writeConfig: (content: string) => window.OpenListNative?.writeConfig(content) ?? false,
}
```

---

## Part D: 共享 web 资源（pnpm workspace）

### D.1 Monorepo 结构

```yaml
# pnpm-workspace.yaml
packages:
  - 'app'
  - 'plugin-openlist/web'
  - 'packages/*'
```

### D.2 共享组件包

```json
// packages/components/package.json
{
    "name": "@encvgo/components",
    "version": "0.1.0",
    "main": "src/index.ts",
    "exports": {
        "./OpenListStatusCard": "./src/OpenListStatusCard.vue",
        "./OpenListLogList": "./src/OpenListLogList.vue"
    },
    "peerDependencies": {
        "vue": "^3.4",
        "@ionic/vue": "^8.0",
        "ionicons": "^7.0"
    }
}
```

```typescript
// packages/components/src/index.ts
export { default as OpenListStatusCard } from './OpenListStatusCard.vue'
export { default as OpenListLogList } from './OpenListLogList.vue'
```

### D.3 插件 web 项目

```json
// plugin-openlist/web/package.json
{
    "name": "@encvgo/plugin-openlist-web",
    "dependencies": {
        "@encvgo/components": "workspace:*",
        "vue": "^3.4",
        "@ionic/vue": "^8.0",
        "ionicons": "^7.0",
        "vue-router": "^4.0"
    }
}
```

### D.4 插件 web 页面

```vue
<!-- plugin-openlist/web/src/views/OpenListHome.vue -->
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>OpenList - v{{ version }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="showPwdEdit">
            <ion-icon :icon="keyOutline" />
          </ion-button>
          <ion-button @click="openConfigEditor">
            <ion-icon :icon="codeSlashOutline" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <!-- 复用 @encvgo/components 中的状态卡 -->
      <OpenListStatusCard :runtime="runtime" />
      <OpenListLogList :logs="logs" />
    </ion-content>
    <ion-fab vertical="bottom" horizontal="end" slot="fixed">
      <ion-fab-button @click="toggleService" :color="runtime.running ? 'danger' : 'primary'">
        <ion-icon :icon="runtime.running ? powerOutline : playOutline" />
      </ion-fab-button>
    </ion-fab>
  </ion-page>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { OpenListStatusCard, OpenListLogList } from '@encvgo/components'
import { keyOutline, codeSlashOutline, powerOutline, playOutline } from 'ionicons/icons'
import { OpenListNative } from '@/plugins/openlist-native'

const runtime = ref({ running: false, port: 0, pid: 0 })
const version = ref('0.0.0')
const logs = ref<any[]>([])

onMounted(() => {
    refreshStatus()
    setInterval(refreshStatus, 3000)
})

async function refreshStatus() {
    runtime.value = OpenListNative.getStatus()
}

async function toggleService() {
    if (runtime.value.running) {
        OpenListNative.stopOpenList()
    } else {
        const port = OpenListNative.startOpenList()
        runtime.value.port = parseInt(port)
    }
    setTimeout(refreshStatus, 1000)
}
</script>
```

### D.5 主 app 集成（最小改动）

主 app 的 `src/router/index.ts` 添加 OpenList 路由（如果在主 app 中也需要访问）：

```typescript
// app/src/router/index.ts
import OpenListHome from '@encvgo/plugin-openlist-web/views/OpenListHome.vue'

export const routes = [
    // ... 现有路由 ...
    { path: '/openlist', component: OpenListHome }
]
```

主 app 通过路由 + 插件 Content() 双重入口都能访问 OpenList 页面。

---

## What Changes

### 变更一：ComboLite 合规修复（Phase 0）

- **修改**: `plugin-openlist/OpenListPluginEntry.kt`
  - `pluginModule = emptyList()`
  - `onLoad()` 初始化 OpenListBridge
  - `onUnload()` shutdown OpenListBridge + OpenListService
  - `Content()` 用 `AndroidView(WebView)` 替代 Compose UI
- **删除**: `StatusCard` / `ControlCard` / `ConfigCard` / `InfoGrid` / `formatFileSize`
- **修改**: `plugin-openlist/build.gradle.kts`（移除 compose 依赖）

### 变更二：嵌入式 WebView + JS-Native 桥接（Phase 1）

- **新增**: `plugin-openlist/src/main/java/.../OpenListEmbedWebView.kt`
- **新增**: `plugin-openlist/src/main/java/.../OpenListPluginJSInterface.kt`
- **新增**: `plugin-openlist/src/main/java/.../OpenListWebViewClient.kt`

### 变更三：Monorepo 改造（Phase 2）

- **新增**: `pnpm-workspace.yaml`（项目根）
- **修改**: 项目根 `package.json`（增加 workspaces）
- **新增**: `packages/components/`（共享 Vue 组件包）
- **新增**: `plugin-openlist/web/`（插件 web 项目）
- **修改**: `app/src/router/index.ts`（添加 OpenList 路由）

### 变更四：插件 web 项目页面（Phase 2）

- **新增**: `plugin-openlist/web/src/views/OpenListHome.vue`
- **新增**: `plugin-openlist/web/src/views/OpenListConfigEditor.vue`
- **新增**: `plugin-openlist/web/src/plugins/openlist-native.ts`
- **新增**: `plugin-openlist/web/src/main.ts` / `App.vue` / `router/index.ts`
- **新增**: `plugin-openlist/web/vite.config.ts` / `package.json` / `tsconfig.json`

### 变更五：主 app 集成（最小改动，Phase 3）

- **修改**: `src/router/index.ts`（添加 OpenList 路由）
- **新增**: `src/plugins/OpenListPluginLaunch.ts`（可选用：主 app 主动调插件 Content 渲染）

### 不变文件

- `plugin-openlist/OpenListBridge.kt`
- `plugin-openlist/OpenListService.kt`
- `plugin-openlist/OpenListConfig.kt`
- `plugin-openlist/OpenListStatusProvider.kt`
- `android/combolite-host/OpenListStatusBridge.kt`
- `src/plugins/GoProcess.ts` / `web.ts`（保留 OpenList 桥接方法）

---

## Impact

### 修改文件
- `plugin-openlist/OpenListPluginEntry.kt`
- `plugin-openlist/build.gradle.kts`
- 项目根 `package.json` / 新增 `pnpm-workspace.yaml`
- `app/src/router/index.ts`

### 新增文件
- `plugin-openlist/src/main/java/.../OpenListEmbedWebView.kt`
- `plugin-openlist/src/main/java/.../OpenListPluginJSInterface.kt`
- `plugin-openlist/src/main/java/.../OpenListWebViewClient.kt`
- `plugin-openlist/web/`（完整 Vite + Vue + Ionic Vue 项目）
- `packages/components/`（共享 Vue 组件包）

### 不变文件
- `plugin-openlist/OpenListBridge.kt`（gomobile bind 入口不变）
- `plugin-openlist/OpenListService.kt`（Android Service 不变）
- `plugin-openlist/OpenListConfig.kt`（config 持久化不变）
- `plugin-openlist/OpenListStatusProvider.kt`（ContentProvider IPC 不变）
- 主 app 现有组件（LocalOpenListStatusCard、ExtensionsPage 等）保持兼容

---

## ADDED Requirements

### Requirement: 保持 ComboLite 插件形态

`plugin-openlist` SHALL 保持 ComboLite 扩展 APK 形态（被主 app 通过 PluginLifecycleEngine 加载到独立进程），**不是**独立 Capacitor Android 应用。`OpenListPluginEntry` 必须实现 `IPluginEntryClass` 接口。

#### Scenario: 主 app 加载 OpenList 插件
- **WHEN** 主 app 通过 ComboLite 框架加载 plugin-openlist.apk
- **THEN** 框架实例化 OpenListPluginEntry → 调 onLoad() → 在 Content() 上下文中渲染嵌入式 WebView

#### Scenario: 卸载插件
- **WHEN** 用户禁用 OpenList 扩展
- **THEN** ComboLite 调 onUnload() → shutdown OpenListBridge + OpenListService → 释放嵌入式 WebView

### Requirement: 嵌入式 Capacitor WebView

`OpenListPluginEntry.Content()` SHALL 渲染一个嵌入式 Android WebView（通过 `AndroidView` 包裹），WebView 加载主 app Capacitor 桥接的 web 资源（共享 dev server 或 assets 子目录）。

#### Scenario: 嵌入式 WebView 启动
- **WHEN** 框架调用 `Content()` 渲染
- **THEN** 创建 WebView → 注册 OpenListPluginJSInterface → 加载 `https://localhost/openlist/`

#### Scenario: JS 调 Native 桥接
- **WHEN** web 端 `OpenListNative.startOpenList()` 被调用
- **THEN** 触发 JSInterface → `OpenListBridge.start()` → 返回端口号给 web 端

### Requirement: 共享 web 资源（pnpm workspace）

主 app 和 plugin-openlist SHALL 通过 pnpm workspace monorepo 共享 web 资源（Vue 组件、工具函数、主题样式）。

#### Scenario: 复用共享组件
- **WHEN** plugin-openlist web 项目 import `@encvgo/components`
- **THEN** 解析到 `packages/components/src/index.ts` 导出的共享组件

#### Scenario: 修改共享组件自动同步
- **WHEN** 修改 `packages/components/src/OpenListStatusCard.vue`
- **THEN** 主 app 和 plugin-openlist web 项目都自动使用最新版本

### Requirement: 嵌入式 WebView 与主 app 共享 Capacitor 桥接

plugin-openlist 内部 WebView SHALL 通过 `addJavascriptInterface` 注册 `OpenListPluginJSInterface`，复用主 app Capacitor runtime（避免每个插件都启动独立 Capacitor）。

#### Scenario: 通过 JSInterface 调底层
- **WHEN** web 端调 `OpenListNative.getRuntimeStatus()`
- **THEN** JSInterface 调 `OpenListBridge.snapshot()` → 返回 JSON 字符串

## MODIFIED Requirements

### Requirement: OpenListPluginEntry 合规

`OpenListPluginEntry.pluginModule = emptyList()`、`onLoad()` 初始化 OpenListBridge、`onUnload()` shutdown。`Content()` 用嵌入式 WebView 替代 Compose。

### Requirement: 删除 plugin-openlist 内 Compose UI

plugin-openlist 内的 `StatusCard` / `ControlCard` / `ConfigCard` / `InfoGrid` / `formatFileSize` SHALL 全部删除，由共享 web 组件替代。

## REMOVED Requirements

### Removed: plugin-openlist 独立 Capacitor 应用

**Reason**: 用户明确"应该还是 ComboLite 插件（不是独立应用）"。
**Migration**: plugin-openlist 保持 ComboLite 插件形态，UI 通过嵌入式 WebView 在 `Content()` 内部渲染。

### Removed: 主 app 集成 OpenListEmbedPlugin / StartOpenListAppPlugin

**Reason**: 不需要新增主 app Capacitor 插件来启动 OpenList 独立 Activity——OpenList UI 已嵌入主 app 进程内的 WebView。
**Migration**: 通过路由 `/openlist` 访问 OpenList 页面，UI 渲染在 `Content()` 上下文中。

---

## 架构总览（最终）

```
┌─ 主 ENCV App Process (com.encvgo.app) ──────────────────────────┐
│                                                                  │
│  Capacitor 运行时（已有）                                         │
│  └─ 主 app web 资源                                              │
│      ├─ / 业务页面 (Files/Tasks/Settings)                         │
│      └─ /openlist/ → 插件 OpenList 页面（共享路由）                │
│                                                                  │
│  ComboLite 框架（combolite-host AAR）                             │
│  └─ PluginLifecycleEngine                                        │
│      └─ 加载 plugin-openlist.apk → OpenListPluginEntry           │
│          └─ Content() → AndroidView(WebView)                     │
│              └─ OpenListPluginJSInterface (addJavascriptInterface)│
│                  ├─ startOpenList() → OpenListBridge.start()    │
│                  ├─ stopOpenList() → OpenListBridge.stop()      │
│                  └─ getRuntimeStatus() → OpenListBridge.snapshot│
│                                                                  │
│  pnpm workspace 共享：                                            │
│  └─ packages/components/ → @encvgo/components                    │
│      ├─ OpenListStatusCard.vue（主 app 和插件都引用）             │
│      └─ OpenListLogList.vue（主 app 和插件都引用）                │
│                                                                  │
├──────────────────────────────────────────────────────────────────┤
│  跨进程 IPC：ContentProvider（com.encvgo.plugin.openlist.provider）│
│  ├─ /status → 主 app LocalOpenListStatusCard 读取                │
│  └─ /control → 写操作（保留兼容）                                │
└──────────────────────────────────────────────────────────────────┘
        ↓ ComboLite 跨进程加载
┌─ plugin-openlist APK Process (com.encvgo.plugin.openlist) ──────┐
│                                                                  │
│  OpenListPluginEntry                                             │
│  ├─ pluginModule = emptyList()                                   │
│  ├─ onLoad → OpenListBridge.init()                               │
│  ├─ onUnload → OpenListBridge.shutdown()                         │
│  └─ Content() → OpenListEmbedWebView                             │
│      └─ WebView + JSInterface                                    │
│                                                                  │
│  OpenListBridge (gomobile bind)                                   │
│  OpenListService (Android 前台服务)                              │
│  OpenListConfig (config.json 持久化)                              │
│  OpenListStatusProvider (ContentProvider IPC)                    │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```
