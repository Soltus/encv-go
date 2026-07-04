# Logcat 诊断结果与白屏修复 Plan

## 日志关键发现（好消息：加载链路完全正常！）

### 完整加载时序（提取自 logcat.log 第一次启动）

```
16:47:00.830  onCreate: super done, bridge=true, webView=true       ✅ 原生就绪
16:47:00.859  navigateToPlayer: https://localhost/player.html         ✅ 导航触发
16:47:00.855  WVC.shouldInterceptRequest: .../player.html            ✅ LocalServer 收到请求
16:47:00.856  Handling local request: .../player.html                ✅ Capacitor 处理中
16:47:00.900  WVC.onPageFinished: url=https://localhost/              ← index.html 完成(被替换)
16:47:00.937  WVC.onPageStarted: url=.../player.html                 ✅ player.html 开始
16:47:00.949  WVC.shouldInterceptRequest: ...player-B_l6eQNh.js      ✅ 主JS chunk
16:47:00.949  WVC.shouldInterceptRequest: ...variables-DfX9m5hy.js   ✅ 变量JS
16:47:00.949  WVC.shouldInterceptRequest: ...variables-BhgLvWne.css  ✅ 变量CSS
16:47:01.008  WVC.onPageFinished: url=.../player.html                 ✅ player.html 加载完成！
16:47:01.008  [PLAYER-INIT] player-main.ts starting                   ✅ JS 执行了！
16:47:01.020  WVC.shouldInterceptRequest: ...p-Cz5nLPGT.js          ✅ 异步chunk继续加载
16:47:01.480  WVC.shouldInterceptRequest: ...p-CneGxKsZ.js          ✅
16:47:01.482  WVC.shouldInterceptRequest: ...p-BgwEQWW6.js          ✅
16:47:03.297  WVC.onPageFinished: url=.../player.html                 ✅ 最终完成
```

### 第二次启动（外部 Intent，新进程）完全相同的成功模式

```
16:47:14.168  WVC.onPageFinished: url=https://localhost/              ← index.html
16:47:14.206  WVC.onPageStarted: url=.../player.html                 ✅
16:47:14.411  WVC.onPageFinished: url=.../player.html                 ✅
16:47:14.412  [PLAYER-INIT] player-main.ts starting                   ✅ JS执行
```

### 排除项

| 检查项 | 结果 | 证据 |
|--------|------|------|
| LocalServer 提供 player.html | ✅ 正常 | `shouldInterceptRequest` 收到请求，无 404 |
| JS/CSS 资源加载 | ✅ 全部正常 | 所有 .js/.css chunk 都被拦截处理，无错误 |
| player-main.ts 执行 | ✅ 正常 | `[PLAYER-INIT]` 已打印 |
| JS 运行时错误 | ✅ 无 | 无 `[PLAYER-ERROR]` 日志 |
| Promise rejection | ✅ 无 | 无 `[PLAYER-PROMISE-REJECT]` 日志 |
| HTTP 错误 | ⚠️ 仅 favicon.ico 404 | 无害，不影响页面 |

## 根因定位：白屏是 Vue 渲染层问题，不是加载问题

**HTML 加载成功 + JS 执行成功 + 无报错 + 白屏 = 组件渲染了但不可见或未渲染预期内容。**

最可能的根因：**StandalonePlayer.vue 初始状态为 `backendLoading=true`，显示的 loading 内容在 PlayerActivity 的 WebView 中无法正确渲染。**

查看 [StandalonePlayer.vue](src/views/StandalonePlayer.vue) 的模板结构：

```vue
<ion-page>
  <ion-header>...</ion-header>           ← 工具栏
  <ion-content>
    <div v-if="backendLoading" class="loading-state">
      <ion-spinner />                     ← Ionic 组件
      <h3>正在启动后端...</h3>           ← 纯 HTML
    </div>
    <div v-else-if="backendError" ...>   ← 错误状态
    <div v-else class="player-container"> ← 播放器
  </ion-content>
</ion-page>
```

如果用户看到的是**纯白色**（连 loading spinner 和文字都看不到），说明 **Ionic 组件在 PlayerActivity 中没有正确初始化/渲染**。

如果用户能看到 **loading spinner 但觉得是"白屏"**（因为期望看到播放器），那是另一个问题。

## 修复方案

### Step 1：添加更多前端诊断日志到 StandalonePlayer.vue

在组件的关键生命周期和状态变化点添加 `console.log`，确认：
- Vue 组件是否挂载
- `backendLoading` 初始值和变化
- 后端通知是否收到
- 路由是否正确匹配 `/player`

### Step 2：检查 Ionic 在 PlayerActivity 中的初始化状态

Capacitor 的 Bridge JS 是通过 `DocumentStartScript` 注入的，使用 `WebViewCompat.addDocumentStartJavaScript` 设置 allowedOrigin。当从 index.html 切换到 player.html 时，注入应该对所有同 origin 页面生效。但需要确认：

1. Capacitor bridge 对象在 player.html 中是否存在
2. `Capacitor.Plugins` 是否可用
3. Ionic 的平台初始化是否完成

### Step 3：给 player.html 添加可见的 fallback 内容

作为最后的保障，如果 Vue/Ionic 完全无法渲染，至少让用户看到一个非白色的页面：

```html
<body style="background:#1a1a2e;color:white;margin:0;padding:20px;font-family:sans-serif;">
  <div id="app">
    <noscript>Please enable JavaScript</noscript>
  </div>
  ...
```

---

## 修改文件清单

| # | 文件 | 操作 |
|---|------|------|
| 1 | `src/views/StandalonePlayer.vue` | 添加 console.log 诊断（onMounted、watch、initBackend 等关键节点） |
| 2 | `src/player-main.ts` | 增强：打印 Capacitor bridge 和 Plugins 可用性 |

**本次修改的目标仍然是诊断** — 确认白屏发生在 Vue 渲染链路的哪个环节，然后精准修复。
