# 白屏根因确认与修复 Plan

## 日志分析结果（完整链路）

### Native 层：✅ 全部正常
```
onCreate: super done, bridge=true, webView=true       ✅
navigateToPlayer: https://localhost/player.html         ✅
handleBackend: already running port=2025                ✅
WVC.onPageStarted: url=.../player.html                 ✅
WVC.onPageFinished: url=.../player.html                ✅ (多次)
```

### JS 层：Vue 挂载成功，但组件未渲染！
```
[PLAYER-INIT] player-main.ts starting                  ✅ JS执行了
[PLAYER-INIT] Capacitor: exists                        ✅ Bridge注入正常
[PLAYER-INIT] Capacitor.isNativePlatform(): true        ✅ 原生平台
[PLAYER-INIT] Capacitor.Plugins keys: SystemBars,GoProcess,...  ✅ 插件可用
[PLAYER-INIT] Router ready, mounting app to #app       ✅ Router就绪
[PLAYER-INIT] #app element: true innerHTML length: 0   ✅ DOM挂载点存在
[PLAYER-INIT] App mounted successfully                  ✅ Vue挂载完成
```

### 🔴 关键缺失：StandalonePlayer.vue 的日志完全没有出现！

```
❌ [StandalonePlayer] <script setup> evaluating...     ← 没有！
❌ [StandalonePlayer] onMounted fired                   ← 没有！
❌ [StandalonePlayer] initBackend() called              ← 没有！
```

## 根因定位

**Vue App 成功挂载到 `#app`，但 StandalonePlayer 组件从未被加载/执行。**

这意味着路由没有正确导航到 `/player` 路由，或者 `<ion-router-outlet>` 没有渲染路由组件。

最可能的原因：**Ionic 的 `<ion-router-outlet>` 在 PlayerActivity 这个"第二实例"中无法正确工作。**

原因分析：
1. MainActivity 先启动，初始化了完整的 Ionic 环境（CSS 变量、全局事件监听、overlay 系统）
2. PlayerActivity 在同一进程中创建新的 WebView + 新的 Ionic App 实例
3. 两个 Ionic App 共享同一个 JS 运行时环境的一些全局状态
4. `<ion-router-outlet>` 依赖 Ionic 的内部路由系统（ion-nav、view-controller 等），这些可能在第二实例中冲突或未正确初始化

## 修复方案

### 方案：绕过 Ionic Router，直接渲染 StandalonePlayer

不再依赖 `<ion-router-outlet>` + Vue Router 的间接渲染。改为在 `PlayerApp.vue` 中直接使用 `<Suspense>` 包裹异步组件：

```vue
<template>
  <ion-app>
    <Suspense>
      <template #default>
        <StandalonePlayer />
      </template>
      <template #fallback>
        <div class="loading-fallback">Loading...</div>
      </template>
    </Suspense>
  </ion-app>
</template>

<script setup lang="ts">
import { IonApp } from '@ionic/vue'
import StandalonePlayer from '@/views/StandalonePlayer.vue'
</script>
```

**变更要点：**
1. 移除 `<ion-router-outlet>`
2. 直接 import 并渲染 `StandalonePlayer` 组件
3. 用 `<Suspense>` 处理可能的异步依赖
4. 保留 `<ion-app>` 以维持 Ionic 组件（ion-header、ion-content、ion-spinner 等）的样式上下文
5. `player-main.ts` 中仍需 use(playerRouter) 因为 IonicVue plugin 可能需要它

### 备选：如果上述方案仍有问题

完全移除 Ionic 依赖，用纯 HTML/CSS/Vue 渲染播放器界面：
```vue
<template>
  <div class="player-app">
    <!-- 纯 HTML header + content -->
    <StandalonePlayer />
  </div>
</template>
```

---

## 修改文件清单

| # | 文件 | 操作 |
|---|------|------|
| 1 | `src/PlayerApp.vue` | 移除 `<ion-router-outlet>`，改为直接 `<Suspense><StandalonePlayer /></Suspense>` |

仅此一个文件改动。

---

## 验证预期

修复后 logcat 应出现：
```
[StandalonePlayer] <script setup> evaluating...    ← 新增！
[StandalonePlayer] onMounted fired                 ← 新增！
[StandalonePlayer] initBackend() called            ← 新增！
[StandalonePlayer] isStandaloneMode: true          ← 新增！
...
```
