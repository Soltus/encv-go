# Files.vue 三项修复计划

## Issue 1: 加载状态与文件列表同时显示（上半加载+下半列表）

### 根因分析

模板条件分支结构问题：

```
L87:  <div v-if="loading || isSearching">          ← 全局 loading spinner
L92:  <div v-else-if="noPermission">
L102: <div v-else-if="!serverOnline">
L112: <div v-else-if="displayFiles.length === 0">

L118: <div v-if="selectedPlugin">                   ← ⚠️ 独立 v-if，不在上面的 else-if 链中！
L164: <ion-list v-else>                              ← 仅与 L118 配对
```

当进入插件模式时，`openPluginView()` 设置 `loading=true` + `selectedPlugin=plugin`，导致 **L87 的 spinner 和 L118 的插件列表同时渲染**。

### 修复方案

将全局 loading 的显示条件增加 `&& !selectedPlugin`：

```html
<!-- L87 修改前 -->
<div v-if="loading || isSearching" class="loading-container">

<!-- L87 修改后 -->
<div v-if="(loading || isSearching) && !selectedPlugin" class="loading-container">
```

插件模式下的加载状态由其自身内容处理（`filteredPluginFiles` 为空时显示"无匹配文件"，watch 异步填充后自动更新）。

---

## Issue 2: 图片预览图透明不显示

### 根因分析

两层问题：

**问题 A — 模板 v-if/v-else 死锁**
```html
<img v-if="isImageFile(file) && thumbnailUrls[file.path]" @error="隐藏img" />
<ion-icon v-else ... />
```
当图片加载失败时，`@error` 将 img 设为 `display:none`，但 **v-if 条件仍为 true**，v-else 分支的图标永远不会渲染。结果：48px 槽位完全空白（透明）。

**问题 B — stream URL 可能无法作为 img src 直接使用**
`getExternalStreamUrl()` 返回的 `/api/stream/external?path=...` 是通用流端点，可能：
- 缺少 CORS 头导致 `<img>` 跨域失败
- 返回 Content-Disposition 导致浏览器不直接渲染
- 需要认证头而 `<img>` 无法携带

### 修复方案

1. **模板结构改造**：移除 `@error` 隐藏逻辑，改用 `onload/onerror` 回调来管理 `thumbnailUrls` 状态——加载失败时 delete 该路径的 URL，让 v-if 自动回退到图标：

```typescript
function onThumbLoad(path: string) { /* 已成功，无需操作 */ }
function onThumbError(path: string) { delete thumbnailUrls.value[path] }
```

```html
<img
  v-if="isImageFile(file) && thumbnailUrls[file.path]"
  :src="thumbnailUrls[file.path]"
  class="file-thumb"
  loading="lazy"
  @load="onThumbLoad(file.path)"
  @error="onThumbError(file.path)"
/>
<ion-icon v-else ... />
```

2. **验证 stream 端点对 img 的兼容性**：检查后端 `handleStreamExternalFileGin` 是否对图片请求返回正确的 `Content-Type` + `Access-Control-Allow-Origin`。如不兼容，考虑在 URL 中添加 `format=thumb` 参数或改用 dedicated thumbnail API。

---

## Issue 3: 增加三种排序方式（名字/大小/时间 + 倒序）

### 设计

| 排序字段 | 默认顺序 | 倒序 |
|---------|---------|------|
| name | A→Z | Z→A |
| size | 小→大 | 大→小 |
| time | 旧→新 | 新→旧 |

目录始终排在文件前面（保持现有行为）。

### 修改清单

#### 3a. 新增状态变量（~L397 附近）

```typescript
const sortBy = ref<'name' | 'size' | 'time'>('name')
const sortDesc = ref(false)
```

#### 3b. 改造 sortedFiles computed（~L428）

```typescript
const sortedFiles = computed(() => {
  const list = [...files.value]
  list.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1
    if (!a.isDirectory && b.isDirectory) return 1
    let cmp = 0
    switch (sortBy.value) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'size': cmp = (a.size || 0) - (b.size || 0); break
      case 'time': cmp = (a.modified || 0) - (b.modified || 0); break
    }
    return sortDesc.value ? -cmp : cmp
  })
  return list
})
```

#### 3c. 工具栏添加排序按钮（搜索栏下方或面包屑行末尾）

在搜索栏 toolbar（L28-L44）中 slot="end" 添加排序切换按钮：

```html
<ion-button fill="clear" size="small" @click="cycleSort" slot="end" v-if="!searchQuery">
  <ion-icon :icon="currentSortIcon" slot="icon-only" />
</ion-button>
```

点击循环切换：name↑ → name↓ → size↑ → size↓ → time↑ → time↓ → name↑...

或更直观地用两个按钮：一个选排序字段，一个切换升降序。

**推荐 UI 方案**：单个按钮循环切换，图标随当前状态变化（使用 `swapVertical` / `arrowDown` / `arrowUp` 等 ionicons）。

#### 3d. 新增 import

需要导入排序相关图标：`swapVertical`, `arrowDownOutline`, `arrowUpOutline` 等（从 `ionicons/icons`）。

---

## 实施顺序

1. **Issue 1** — 修复 loading 重叠（1 行改动）
2. **Issue 2** — 修复图片预览（模板 + TS 回调函数 + 可选后端检查）
3. **Issue 3** — 排序功能（状态 + computed + UI 按钮 + import）
4. **构建验证** — `vue-tsc --noEmit && vite build`
