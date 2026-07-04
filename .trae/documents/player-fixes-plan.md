# PlayerActivity 待修复问题 Plan（完整版）

## 问题清单（按优先级排序）

### 🔴 P0：播放器显示为浏览器原生控件而非 ArtPlayer

**现象**：即使第三方打开能播放，界面也是浏览器原生的 `<video>` 控件（原生进度条、播放按钮），不是 ArtPlayer 的橙色主题自定义控件。

**可能根因**：
1. **ArtPlayer CSS 未加载** — 从 logcat 的 `shouldInterceptRequest` 日志看，只看到 JS/CSS chunk 被加载，但没有看到 ArtPlayer 自带的样式文件。Vite 多入口构建时 artplayer 的 CSS 可能没有被正确打包到 player 入口的依赖中。
2. **Android WebView 全屏视频拦截** — 当 `<video>` 元素在某些条件下播放时，Android WebView 会自动启动系统原生全屏播放器（`WebChromeClient.onShowCustomView`），绕过 ArtPlayer 的自定义 UI。
3. **ArtPlayer 初始化后立即报错** — 由于 404 导致 `art.on('error')` 触发后，ArtPlayer 可能降级为显示原始 video 元素。

**排查方向**：
- 检查 Vite 构建产物中 player 相关的 asset 是否包含 artplayer 样式
- 确认 ArtPlayer 实例的 DOM 结构是否正确生成（检查 `artContainer` 内部是否有 `artplayer-container` 等 class）
- 在 PlayerActivity.kt 中覆写 `onShowCustomView` / `onHideCustomView` 阻止 WebView 原生全屏拦截

**修复方案**：
```kotlin
// PlayerActivity.kt — 阻止 WebView 原生全屏视频拦截
override fun onCreate(savedInstanceState: Bundle?) {
    // ... existing code ...
    super.onCreate(savedInstanceState)
    
    // 阻止 WebView 拦截 <video> 全屏为原生播放器
    bridge?.webView?.webChromeClient = object : WebChromeClient() {
        // 不实现 onShowCustomView → 视频始终在页面内播放（playsInline）
    }
}
```

同时在前端确保：
- ArtPlayer option 中 `playsInline: true` 已设置 ✅（已有）
- 添加 `customType: 'normal'` 确保 ArtPlayer 使用自己的渲染模式
- 检查并确认 artplayer CSS 正确导入

**文件**：
- `src/views/StandalonePlayer.vue` — ArtPlayer 配置优化
- `android-overlay/.../PlayerActivity.kt` — 阻止原生全屏拦截

---

### P0：应用内打开视频无法播放（路径 404）

**现象**：从 ENC 应用内 Files 页面点击视频 → PlayerActivity 打开 → `GET /api/stream/external?path=/123云盘/...` → **404** → ArtPlayer 循环报错

**根因**：Files.vue 发送的路径 `/123云盘/xxx.mp4` 是相对于 serve root (`/storage/emulated/0`) 的路径。Android 文件系统中该路径不存在，真实路径是 `/storage/emulated/0/123云盘/xxx.mp4`。

**对比**：第三方应用打开时 content URI 解析出的完整路径正常工作。

**修复**：在 `StandalonePlayer.vue` 中添加路径补全函数：

```typescript
function resolveNativePath(raw: string): string {
  if (!raw.startsWith('/')) return raw
  if (raw.startsWith('/storage/') || raw.startsWith('/sdcard/') || raw.startsWith('/data/')) return raw
  return `/storage/emulated/0${raw}`
}

// streamUrl computed 中使用
const streamUrl = computed(() => {
  if (!filePath.value) return ''
  const resolvedPath = resolveNativePath(filePath.value)
  if (isExternalFile.value) return getExternalStreamUrl(resolvedPath)
  return getFileStreamUrl(resolvedPath)
})
```

**文件**：`src/views/StandalonePlayer.vue`

---

### P1：设置按钮无响应

**现象**：右上角设置图标点击无反应

**根因**：`goSettings()` 使用 `router.push('/player/settings')`，但 `<ion-router-outlet>` 已移除。

**修复**：

**Step 1**: `StandalonePlayer.vue` — emit 事件替代路由跳转
```typescript
const emit = defineEmits(['open-settings'])
function goSettings() { emit('open-settings') }
```

**Step 2**: `PlayerApp.vue` — 条件渲染 player/settings
```vue
<template>
  <ion-app>
    <Suspense>
      <template #default>
        <StandalonePlayer v-if="view === 'player'" @open-settings="view = 'settings'" />
        <PlayerSettings v-else @close="view = 'player'" />
      </template>
    </Suspense>
  </ion-app>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { IonApp } from '@ionic/vue'
import StandalonePlayer from '@/views/StandalonePlayer.vue'
import PlayerSettings from '@/views/PlayerSettings.vue'

const view = ref<'player' | 'settings'>('player')
</script>
```

**Step 3**: `PlayerSettings.vue` — 返回按钮 emit close
```typescript
const emit = defineEmits(['close'])
function goBack() { emit('close') }
```

**文件**：`src/PlayerApp.vue`, `src/views/StandalonePlayer.vue`, `src/views/PlayerSettings.vue`

---

### P2：全屏旋转屏幕

**需求**：进入全屏时根据视频宽高比智能旋转（横屏/竖屏），退出恢复。

**修复**：监听 ArtPlayer fullscreen 事件 + 屏幕方向 API

```typescript
// StandalonePlayer.vue initArtPlayer() 中添加
art.on('fullscreen', (state: boolean) => {
  isFullscreen.value = state
  if (state) {
    const video = art?.video
    if (video?.videoWidth && video?.videoHeight) {
      const ratio = video.videoWidth / video.videoHeight
      const orientation = ratio > 1.3 ? 'landscape' : ratio < 0.77 ? 'portrait' : 'landscape'
      ScreenOrientation.lock({ orientation }).catch(() => {})
    }
  } else {
    ScreenOrientation.unlock().catch(() => {})
  }
})
```

**依赖**：`@capacitor/screen-orientation`
**备选**：通过 GoProcessPlugin 调用原生 `setRequestedOrientation()`，无需新依赖。

**文件**：`src/views/StandalonePlayer.vue`, `package.json`

---

## 修改文件清单

| # | 文件 | 修改内容 |
|---|------|---------|
| 1 | `src/views/StandalonePlayer.vue` | ArtPlayer 显示修复 + 路径补全 + 设置 emit + 全屏旋转 |
| 2 | `src/PlayerApp.vue` | 条件渲染 player/settings |
| 3 | `src/views/PlayerSettings.vue` | 返回按钮 emit close |
| 4 | `android-overlay/.../PlayerActivity.kt` | 阻止 WebView 原生全屏视频拦截 |
| 5 | `package.json` | 新增 @capacitor/screen-orientation（可选） |

---

## 执行顺序

1. **ArtPlayer 原生控件问题** — 最影响用户体验，必须先修
2. **路径 404** — 应用内打开功能核心阻塞
3. **设置按钮** — 用户体验
4. **全屏旋转** — 增强功能
