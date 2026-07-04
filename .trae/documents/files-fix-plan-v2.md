# Files.vue 三项修复 — 真正根因分析

## Issue 1: Loading 与文件列表同时显示（普通模式）

### 真正的模板结构 Bug

当前模板有 **两组独立的 v-if/v-else 链交叉重叠**：

```
L93:  <div v-if="(loading||isSearching) && !selectedPlugin">   ← 链 A（状态提示）
L98:  <div v-else-if="noPermission">
L108: <div v-else-if="!serverOnline">
L118: <div v-else-if="displayFiles.length === 0>              ← 链 A 结束

L124: <div v-if="selectedPlugin">                              ← 链 B 开始（独立！）
...
L168: </div>                                                    ← 链 B 结束

L170: <ion-list v-else>                                        ← 链 B 的 else（与 L124 配对）
```

**普通模式** (`selectedPlugin=null`, `loading=true`) 时：
- L93 为 true → spinner 显示 ✓
- L124 `v-if="selectedPlugin"` 为 false
- L170 `v-else`（配对 L124）→ **为 true → 文件列表也显示了！**

### 修复方案

将整个内容区重构为单一条件链：先判断是否在加载/错误状态，再根据模式渲染对应列表。

```html
<!-- 加载 / 错误 / 空状态 — 仅非插件模式下显示 -->
<template v-if="(loading || isSearching || noPermission || !serverOnline || displayFiles.length === 0) && !selectedPlugin">
  <div v-if="loading || isSearching" class="loading-container">...</div>
  <div v-else-if="noPermission" class="empty-state">...</div>
  <div v-else-if="!serverOnline" class="empty-state">...</div>
  <div v-else class="empty-state"><!-- empty --></div>
</template>

<!-- 内容区：插件视图 或 普通文件列表 -->
<template v-else>
  <div v-if="selectedPlugin">...</div>     <!-- 插件列表 -->
  <ion-list v-else>...</ion-list>           <!-- 普通列表 -->
</template>
```

---

## Issue 2: 图片预览图透明不显示

### 真正根因：后端拒绝图片请求

`StreamExternalFile` ([mobile_service.go L1027](file:///workspace/internal/service/mobile_service.go#L1027))：

```go
var mediaExtensions = map[string]bool{
    "mp4": true, "mkv": true, "avi": true, "mov",   // 视频
    "mp3": true, "flac": true, "wav": true, "aac",   // 音频
    // ❌ 没有任何图片扩展名！
}
if !mediaExtensions[ext] {
    return &UnsupportedMediaTypeError{...}  // 图片全部被 415 拒绝
}
```

前端流程：
1. IntersectionObserver 触发 → 设置 `thumbnailUrls[path] = streamUrl`
2. `<img :src="streamUrl">` 请求后端
3. 后端返回 **415 Unsupported Media Type**
4. `@error="onThumbError(path)"` → delete URL → 回退到图标
5. 用户看到透明槽位（或 fallback 图标），**永远看不到图片**

### 修复方案

在 [mobile_service.go L856](file:///workspace/internal/service/mobile_service.go#L856) 的 `mediaExtensions` 中添加图片扩展名：

```go
var mediaExtensions = map[string]bool{
    // 视频（已有）
    "mp4": true, "mkv": true, ...
    // 音频（已有）
    "mp3": true, "flac": true, ...
    // 图片（新增）
    "jpg": true, "jpeg": true, "png": true, "gif": true,
    "webp": true, "bmp": true, "svg": true, "heic": true,
    "heif": true, "avif": true,
}
```

`http.ServeFile` 对图片文件工作正常，会自动设置正确的 Content-Type。

---

## Issue 3: 排序仅对普通文件生效

### 根因

| 数据源 | 排序支持 | 路径 |
|--------|---------|------|
| 普通文件 | ✅ `sortedFiles` → `displayFiles` | L172 |
| 插件文件 | ❌ `filteredPluginFiles` 无排序 | L136 |
| 标签筛选 | ✅ 经由 `files.value` → `sortedFiles` | L1035 |

`filteredPluginFiles` computed ([L1043](file:///workspace/app/encv-mobile/src/views/Files.vue#L1043)) 只做了 tab 过滤，没有应用排序：

```typescript
const filteredPluginFiles = computed(() => {
  if (!selectedPlugin.value) return []
  if (pluginTab.value === 'container') {
    return pluginFiles.value.filter(...)  // 无排序！
  }
  return pluginFiles.value.filter(...)    // 无排序！
})
```

### 修复方案

在 `filteredPluginFiles` 的 filter 结果上追加排序逻辑（复用相同的排序规则）：

```typescript
const filteredPluginFiles = computed(() => {
  if (!selectedPlugin.value) return []
  let list: FileItem[]
  if (pluginTab.value === 'container') {
    list = pluginFiles.value.filter(f => ...)
  } else {
    list = pluginFiles.value.filter(f => ...)
  }
  list.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1
    if (!a.isDirectory && b.isDirectory) return 1
    let cmp = 0
    switch (sortBy.value) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'size': cmp = (a.size || 0) - (b.size || 0); break
      case 'time': cmp = (Number(a.modified) || 0) - (Number(b.modified) || 0); break
    }
    return sortDesc.value ? -cmp : cmp
  })
  return list
})
```

---

## 实施步骤

### Step 1: 重构模板条件结构（Issue 1）

修改 Files.vue L93-L170 区域，将两组独立的 v-if 链合并为单一条件链。

### Step 2: 后端添加图片扩展名（Issue 2）

修改 `internal/service/mobile_service.go` 的 `mediaExtensions` map，加入 jpg/jpeg/png/gif/webp/bmp/svg/heic/heif/avif。

### Step 3: 排序应用到插件列表（Issue 3）

修改 `filteredPluginFiles` computed，追加排序逻辑。

### Step 4: 双端构建验证

- 前端: `vue-tsc --noEmit && vite build`
- 后端: `mise exec -- go build ./cmd/encv/`
