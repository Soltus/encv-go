# 修复导航 — 改为前端控制路由

## 问题本质

PlayerActivity 和 MainActivity 加载的是**同一个 SPA**（同一个 `dist/index.html`）。SPA 加载后 Vue Router 默认导航到 `/tabs/files`（文件列表页）。

在原生层折腾 `webView.loadUrl()` 设置 hash 是错误方向——因为：
1. Capacitor 内部可能覆盖我们设置的 URL
2. Vue Router 初始化后会按自己的规则导航
3. `load()` 中的 `loadUrl` 和 `super.load()` 内部的首次加载存在竞态
4. "Unable to read file at path public/plugins" 说明 WebView 确实加载了 SPA 资源，只是路由没到播放器页

## 正确方案：前端检测模式 → 自动跳转

**不在原生层做任何 URL 导航。让 PlayerActivity 正常加载默认首页，由前端检测到 standalone 模式后自动 redirect 到 `/standalone/player`。**

```
PlayerActivity 启动
  └─ BridgeActivity 正常加载 SPA (默认 /tabs/files)
      └─ App.vue onMounted:
          ├─ 调用 isStandaloneMode()
          ├─ true? → router.push('/standalone/player')   ← 前端控制路由
          └─ false? → 保持默认（MainActivity 行为不变）
```

### 为什么这个方案可靠

| 对比 | 原生层 loadUrl hack | 前端 router.push |
|------|-------------------|------------------|
| 时序 | ❌ 与 Capacitor 内部竞态 | ✅ SPA 完全加载后才执行 |
| 可靠性 | ❌ 取决于 WebViewClient 行为 | ✅ Vue Router 原生能力 |
| 复杂度 | ❌ 反复修改 native 代码 | ✅ 改一处 App.vue |
| 调试 | ❌ 需要重新打包 APK | ✅ 前端热更新即可验证 |

---

## 实施步骤

### Step 1: PlayerActivity.kt — 删除所有导航代码

- [ ] 删除 `override fun load()` 方法（恢复为不 override）
- [ ] 从 `onCreate` 中删除 `navigateToStandalonePlayer()` 调用（已无此方法）
- [ ] 从 `onNewIntent` 中删除 `webView.loadUrl()` 调用
- [ ] PlayerActivity 变成纯粹的 BridgeActivity 子类：注册插件 + 后端交互 + intent 解析，**不做任何 URL 操作**

最终 PlayerActivity.onCreate 就是：
```kotlin
override fun onCreate(savedInstanceState: Bundle?) {
    registerPlugin(GoProcessPlugin::class.java)
    super.onCreate(savedInstanceState)
    registerBackendReceiver()
    resolveFileInfo(intent)
    if (EncvGoService.isRunning && EncvGoService.lastKnownPort > 0) {
        notifyFrontend(...)
    } else {
        startBackendService(...)
    }
}
```

和 MainActivity 结构几乎一样，区别只在 intent 解析逻辑。

### Step 2: App.vue — 启动时检测 standalone 模式并跳转

在 `onMounted` 中增加：

```typescript
import { isNative, isStandaloneMode } from '@/plugins/GoProcess'
import { useRouter } from 'vue-router'

const router = useRouter()

onMounted(async () => {
  hijackConsole()
  initTheme()
  connect()

  // Standalone mode: auto-navigate to player page
  if (isNative()) {
    const { standalone } = await isStandaloneMode()
    if (standalone) {
      console.info('[App] Standalone mode detected, navigating to player')
      router.replace('/standalone/player')
    }
  }

  await requestEssentialPermissions()
})
```

关键点：
- 用 `router.replace` 而非 `router.push`（不留历史记录，返回键直接退出 Activity）
- 在权限请求之前执行（优先级更高）
- `isStandaloneMode()` 是同步插件调用，几乎零开销

### Step 3: 构建验证
- [ ] `go build ./internal/...` 通过
- [ ] `vue-tsc --noEmit` 通过

### Step 4: 本地合并模拟
- [ ] PlayerActivity.kt 无任何 loadUrl / navigate / WebViewClient 代码
