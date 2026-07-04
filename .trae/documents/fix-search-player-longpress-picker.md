# 文件搜索 + 播放器优化 + 长按菜单修复 + 文件选择器修复

## 问题概述

1. **文件搜索**：需要搜索框，支持子文件夹递归搜索（可选）和模糊匹配
2. **竖屏视频错位**：播放器 `aspect-ratio: 16/9` 强制横屏比例，竖屏视频显示错位，需要完善横竖屏和全屏
3. **长按菜单无效**：`@longpress.prevent` 在 Ionic web component 上不工作
4. **文件选择器无效**：`startPicking()` 只设置了状态，没有导航到 Files 页面

---

## 问题 4：文件选择器 — 用 Modal 替代 Tab 导航

### 根因

`handleBrowse()` 调用 `startPicking()` 后只设置了 `isPickerMode = true`，但用户仍在 Tasks 标签页，根本看不到 Files 页面。而导航到另一个 tab 不是正常的开发思路。

### 方案：新建 FilePickerModal.vue 组件

**核心思路**：用 Ionic Modal 在 Tasks 页面上层弹出文件浏览器，用户选择文件后 Modal 关闭，路径回填。不需要 `useFilePicker` composable，不需要跨 tab 导航。

#### 新建 `src/components/FilePickerModal.vue`

独立的文件浏览 Modal 组件，包含：
- 文件列表浏览（复用 Files.vue 的核心逻辑：`listFiles`、导航、面包屑）
- 顶部标题"选择文件" + 取消按钮
- 点击文件 → `dismiss({ path, name })`
- 点击文件夹 → 进入子目录
- 底部提示"点击文件以选择"

```vue
<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('files.selectFile') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="cancel">{{ t('files.cancelSelect') }}</ion-button>
        </ion-button>
      </ion-toolbar>
      <!-- 面包屑 -->
    </ion-header>
    <ion-content>
      <!-- 文件列表，点击文件调用 modalController.dismiss -->
    </ion-content>
  </ion-page>
</template>
```

#### Tasks.vue 变更

1. 删除 `useFilePicker` 引入
2. `handleBrowse()` 改为打开 FilePickerModal：

```ts
import { modalController } from '@ionic/vue'
import FilePickerModal from '@/components/FilePickerModal.vue'

async function handleBrowse() {
  const modal = await modalController.create({
    component: FilePickerModal,
  })
  await modal.present()
  const { data, role } = await modal.onDidDismiss()
  if (role === 'select' && data) {
    newTaskPath.value = data.path
  }
}
```

#### Files.vue 变更

1. 删除 `useFilePicker` 引用和所有 picker 相关逻辑（`isPickerMode`, `confirmSelection`, `cancelPicking`, `handleCancelPicker`, picker 提示条等）
2. Files.vue 回归纯文件浏览功能

#### 删除 `src/composables/useFilePicker.ts`

不再需要跨组件状态管理，Modal 自带 dismiss 回调机制。

---

## 问题 3：长按菜单无效

### 根因

`@longpress` 在 `ion-item` 上不触发。`ion-item` 是 Ionic 的 Shadow DOM web component，原生 `longpress` 事件在 Shadow DOM 边界被阻断。移动端 Capacitor WebView 也不一定支持 `longpress` 事件。

### 方案：创建自定义 `v-longpress` 指令

基于 `touchstart`/`touchend` + `setTimeout` 实现 500ms 长按检测。

#### 新建 `src/directives/longpress.ts`

```ts
import type { Directive } from 'vue'

export const vLongpress: Directive<HTMLElement, () => void> = {
  mounted(el, binding) {
    if (typeof binding.value !== 'function') return
    let pressTimer: ReturnType<typeof setTimeout> | null = null

    const start = (e: Event) => {
      e.preventDefault()
      if (pressTimer !== null) return
      pressTimer = setTimeout(() => {
        binding.value()
        pressTimer = null
      }, 500)
    }

    const cancel = () => {
      if (pressTimer !== null) {
        clearTimeout(pressTimer)
        pressTimer = null
      }
    }

    el.addEventListener('touchstart', start, { passive: false })
    el.addEventListener('touchend', cancel)
    el.addEventListener('touchmove', cancel)
    el.addEventListener('touchcancel', cancel)
    el.addEventListener('mousedown', start)
    el.addEventListener('mouseup', cancel)
    el.addEventListener('mouseleave', cancel)
    el.addEventListener('contextmenu', (e) => e.preventDefault())

    ;(el as any)._longpress_cleanup = () => {
      el.removeEventListener('touchstart', start)
      el.removeEventListener('touchend', cancel)
      el.removeEventListener('touchmove', cancel)
      el.removeEventListener('touchcancel', cancel)
      el.removeEventListener('mousedown', start)
      el.removeEventListener('mouseup', cancel)
      el.removeEventListener('mouseleave', cancel)
    }
  },
  unmounted(el) {
    ;(el as any)._longpress_cleanup?.()
  },
}
```

#### Files.vue 变更

1. 引入 `vLongpress` 指令
2. 将 `@longpress.prevent="handleLongPress(file)"` 替换为 `v-longpress="() => handleLongPress(file)"`

---

## 问题 2：竖屏视频错位 + 横竖屏切换 + 全屏

### 根因

1. `.video-player` 设置了 `aspect-ratio: 16/9`，强制横屏比例，竖屏视频被压缩或留黑边
2. ArtPlayer 配置了 `autoSize: true` 但容器 CSS 覆盖了自动尺寸
3. 没有全屏播放支持
4. 非全屏时空白区域没有利用

### 方案

#### Player.vue 变更

1. **移除 `aspect-ratio: 16/9`**：让 ArtPlayer 的 `autoSize` 根据视频实际比例自动调整
2. **容器改为自适应**：`.video-player` 使用 `width: 100%`，不强制比例
3. **ArtPlayer 配置增强**：
   - 添加 `fullscreen: true` 启用全屏按钮
   - 添加 `miniProgressBar: true`
   - 监听 `video:loadedmetadata` 事件，autoSize 自动调整
4. **视频信息区域**：非全屏时在播放器下方显示文件名、路径等信息
5. **ArtPlayer fullscreen 事件**：监听 `fullscreen` 和 `fullscreenExit` 事件

#### 模板变更

```html
<div v-else class="player-container">
  <div v-if="isVideo && !playerError" ref="artContainer" class="video-player"></div>

  <div v-if="isVideo && playerError" class="player-error">
    <!-- 错误 UI 不变 -->
  </div>

  <div v-if="isVideo && !playerError && !isFullscreen" class="video-info">
    <h3>{{ fileName }}</h3>
    <p v-if="filePath" class="video-path">{{ filePath }}</p>
  </div>

  <div v-if="isAudio" class="audio-player-wrapper">
    <!-- 音频 UI 不变 -->
  </div>
</div>
```

#### CSS 变更

```css
.video-player {
  width: 100%;
  background: #000;
}

.video-info {
  padding: 16px;
}

.video-path {
  font-size: 12px;
  color: var(--encv-text-secondary);
  word-break: break-all;
  margin-top: 4px;
}
```

#### ArtPlayer 初始化变更

```ts
art = new Artplayer({
  container: artContainer.value,
  url: streamUrl.value,
  autoplay: true,
  autoSize: true,
  autoMini: true,
  mutex: true,
  playsInline: true,
  theme: '#ffad00',
  volume: 0.7,
  fullscreen: true,
  miniProgressBar: true,
})

art.on('fullscreen', () => { isFullscreen.value = true })
art.on('fullscreenExit', () => { isFullscreen.value = false })
```

新增 `isFullscreen` ref。

---

## 问题 1：文件搜索

### 后端：新增搜索 API

#### `internal/service/mobile_service.go` — 新增 `SearchFiles` 方法

- 接收 `queryPath`、`keyword`、`recursive` 参数
- `recursive = false`：仅当前目录，`os.ReadDir` + `strings.Contains` 过滤
- `recursive = true`：`filepath.WalkDir` 递归遍历，跳过隐藏文件和无权限目录
- 模糊匹配：`strings.Contains(strings.ToLower(name), keyword)` 大小写不敏感
- 返回 `[]FileInfo`，包含完整路径

#### `internal/server/mobile_api.go` — 新增 `handleSearchFilesAPI`

- 解析 query 参数：`path`、`keyword`、`recursive`
- 调用 `mobileSvc.SearchFiles()`
- 返回 `{ files: []FileInfo }`

#### `internal/server/server.go` — 注册路由

添加 `/api/files/search` GET 路由。

### 前端

#### `src/api/encv.ts` — 新增 `searchFiles` 函数

```ts
export async function searchFiles(path: string, keyword: string, recursive = false): Promise<FileItem[]> {
  const baseUrl = getApiBaseUrl()
  const params = new URLSearchParams({ path, keyword, recursive: String(recursive) })
  const response = await fetch(`${baseUrl}/api/files/search?${params}`)
  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
  const data = await response.json()
  return data.files || []
}
```

#### Files.vue — 搜索 UI

1. 在 header 下方添加 `ion-searchbar`：
   ```html
   <ion-toolbar>
     <ion-searchbar
       v-model="searchQuery"
       :placeholder="t('files.searchPlaceholder')"
       @ionInput="handleSearchInput"
       @ionClear="handleSearchClear"
     ></ion-searchbar>
   </ion-toolbar>
   ```

2. 搜索状态：
   - `searchQuery` ref：搜索关键词
   - `searchRecursive` ref：是否递归（默认 false）
   - `searchResults` ref：搜索结果（null 表示未搜索，显示原始列表）
   - `isSearching` ref：搜索中

3. 搜索逻辑：
   - 输入防抖 300ms（`setTimeout` + `clearTimeout`）
   - 空关键词 → `searchResults = null`，恢复原始文件列表
   - 有关键词 → 调用 `searchFiles(currentPath, keyword, recursive)`
   - 搜索结果替代 `sortedFiles` 显示

4. 递归搜索开关：搜索框旁的 toggle

5. 搜索结果列表：
   - 复用现有 `ion-list` 模板
   - 搜索模式下显示完整路径（而非仅文件名）
   - 搜索模式下不显示面包屑

6. 搜索缓存：
   - `Map<string, { timestamp: number, results: FileItem[] }>`
   - 缓存 key：`${path}:${keyword}:${recursive}`
   - 有效期 30 秒
   - `file:change` 事件时清除缓存

---

## 问题 5：设置新增缓存设置组 + 二级详情页

### 需求

在设置页新增"缓存与索引"设置组，点击进入二级页面显示全局索引详情，支持非阻塞更新和清空。

### 后端：缓存/索引管理 API

#### `internal/service/mobile_service.go` — 新增缓存管理方法

```go
type IndexStats struct {
    TotalFiles    int   `json:"totalFiles"`
    TotalDirs     int   `json:"totalDirs"`
    TotalSize     int64 `json:"totalSize"`
    IndexedAt     string `json:"indexedAt"`
    IsIndexing    bool  `json:"isIndexing"`
    LastBuildMs   int64 `json:"lastBuildMs"`
}

func (s *MobileService) GetIndexStats() *IndexStats { ... }
func (s *MobileService) RebuildIndex() error { ... }  // 非阻塞，后台 goroutine
func (s *MobileService) ClearIndex() error { ... }
```

**索引实现**：`filepath.WalkDir(s.servingDir)` 递归遍历，构建文件名→路径的搜索索引。索引数据存储在内存中（`sync.Map` 或 `map` + `sync.RWMutex`）。

- `RebuildIndex()`：启动 goroutine 后台构建，`IsIndexing` 标记进行中状态
- `ClearIndex()`：清空内存索引
- `GetIndexStats()`：返回索引统计信息
- `SearchFiles()` 优化：有索引时直接查索引，无索引时 fallback 到 `filepath.WalkDir`

#### `internal/server/mobile_api.go` — 新增缓存管理 handler

- `GET /api/index/stats` → 返回 `IndexStats`
- `POST /api/index/rebuild` → 触发非阻塞重建
- `POST /api/index/clear` → 清空索引

#### `internal/server/server.go` — 注册路由

```go
mux.HandleFunc("/api/index/stats", s.handleIndexStats)
mux.HandleFunc("/api/index/rebuild", s.handleIndexRebuild)
mux.HandleFunc("/api/index/clear", s.handleIndexClear)
```

### 前端

#### `src/api/encv.ts` — 新增索引 API

```ts
export interface IndexStats {
  totalFiles: number
  totalDirs: number
  totalSize: number
  indexedAt: string
  isIndexing: boolean
  lastBuildMs: number
}

export async function getIndexStats(): Promise<IndexStats> { ... }
export async function rebuildIndex(): Promise<void> { ... }
export async function clearIndex(): Promise<void> { ... }
```

#### `src/views/CacheDetail.vue` — 新建二级设置页

参照 `ServerDetail.vue` 的模式，使用 `ion-back-button` 返回。

**内容**：
1. **索引状态**：
   - 总文件数 / 总目录数
   - 索引数据大小
   - 最后索引时间
   - 索引状态（空闲 / 索引中）
   - 上次构建耗时

2. **操作按钮**：
   - "更新索引"按钮：调用 `rebuildIndex()`，非阻塞，按钮显示 spinner + "索引中..."
   - "清空索引"按钮：确认弹窗后调用 `clearIndex()`

3. **搜索缓存**（前端缓存）：
   - 缓存条目数
   - "清空搜索缓存"按钮

4. **轮询更新**：索引中时每 2 秒轮询 `getIndexStats()` 更新状态

#### `src/views/Settings.vue` — 新增缓存设置组

在"连接"设置组之后添加：

```html
<ion-list>
  <ion-list-header>
    <ion-label>{{ t('settings.cache') }}</ion-label>
  </ion-list-header>
  <ion-item button @click="goCache">
    <ion-icon :icon="databaseIcon" slot="start"></ion-icon>
    <ion-label>
      <h3>{{ t('settings.cacheAndIndex') }}</h3>
      <p>{{ indexStats?.isIndexing ? t('settings.indexing') : t('settings.indexReady') }}</p>
    </ion-label>
    <ion-icon :icon="chevronForward" slot="end"></ion-icon>
  </ion-item>
</ion-list>
```

#### `src/router/index.ts` — 新增路由

```ts
{
  path: 'settings/cache',
  component: () => import('@/views/CacheDetail.vue'),
},
```

---

## 文件变更清单

| 文件 | 操作 | 变更内容 |
|------|------|---------|
| `src/components/FilePickerModal.vue` | 新建 | Modal 文件选择器组件 |
| `src/directives/longpress.ts` | 新建 | 自定义 v-longpress 指令 |
| `src/composables/useFilePicker.ts` | 删除 | 不再需要 |
| `src/views/Files.vue` | 修改 | 搜索框 + v-longpress + 移除 picker 逻辑 |
| `src/views/Tasks.vue` | 修改 | 用 Modal 替代 useFilePicker |
| `src/views/Player.vue` | 修改 | 移除 aspect-ratio + 全屏 + 视频信息 |
| `src/views/CacheDetail.vue` | 新建 | 缓存与索引二级设置页 |
| `src/views/Settings.vue` | 修改 | 新增缓存设置组入口 |
| `src/router/index.ts` | 修改 | 新增 settings/cache 路由 |
| `src/api/encv.ts` | 修改 | 新增 searchFiles + 索引管理 API |
| `src/composables/useI18n.ts` | 修改 | 添加搜索、播放器、缓存相关 i18n |
| `internal/service/mobile_service.go` | 修改 | 新增 SearchFiles + 索引管理方法 |
| `internal/server/mobile_api.go` | 修改 | 新增搜索 + 索引管理 handler |
| `internal/server/server.go` | 修改 | 注册搜索 + 索引管理路由 |

## 实施顺序

1. 修复 Bug：文件选择器（FilePickerModal）+ 长按菜单（v-longpress）
2. 修复播放器：竖屏视频 + 全屏 + 视频信息
3. 后端：搜索 API + 索引管理 API
4. 前端搜索 UI
5. 缓存设置二级页（CacheDetail.vue + Settings.vue 入口 + 路由）
6. i18n 文案
7. 构建验证
