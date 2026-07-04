# 插件文件列表：筛选功能 + 切换残留修复

## 一、问题分析

### 问题 1：频繁切换插件类型出现残留

**根因定位**（[Files.vue L1016-1036](src/views/Files.vue#L1016-L1036)）：

```
主文件列表 loadFiles() 有竞态保护 ✅          插件加载 watch(selectedPlugin) 无保护 ❌
─────────────────────────                    ─────────────────────────────
let loadGeneration = 0                       无 generation 计数器
const gen = ++loadGeneration                 直接开始加载
if (gen !== loadGeneration) return           // ← 缺少这行！
files.value.push(file)
```

**时序复现**：
```
T=0ms   用户点击"图片" → gen_A 开始流式加载
T=100ms 首个文件到达 → pluginFiles.push(img_001.jpg)
T=200ms 用户快速切换到"视频" → pluginFiles=[] 清空，gen_B 开始
T=250ms gen_A 的回调仍到达 → pluginFiles.push(img_002.jpg) ← 残留！
T=300ms gen_B 首个文件到达 → pluginFiles.push(video_001.mp4)
结果：pluginFiles = [img_002.jpg, video_001.mp4] ← 混合污染
```

### 问题 2：缺少大小/时间范围筛选

当前 [filteredPluginFiles](src/views/Files.vue#L989-L1013) 仅支持：
- 插件 tab 过滤（source/container）
- 文本搜索（searchQuery）
- 排序（name/size/time）

缺少：
- **大小范围筛选**（如 >1MB 且 <100MB）
- **时间范围筛选**（如最近 7 天内修改）

---

## 二、实施方案

### 步骤 1：修复切换残留 — 添加 pluginLoadGeneration 竞态保护

**文件**：`src/views/Files.vue`

**改动点 A — 新增 generation 变量**（约 L986 后）：
```typescript
let pluginLoadGeneration = 0
```

**改动点 B — 改造 watch(selectedPlugin)**（L1016-1036）：
```typescript
watch(selectedPlugin, async (plugin) => {
  if (plugin) {
    const gen = ++pluginLoadGeneration        // 新增：递增代次
    pluginTab.value = 'source'
    pluginLoaded.value = false
    pluginFiles.value = []
    console.info('[Files] Loading plugin files (stream):', plugin.name)
    try {
      const results = await searchPluginFiles(plugin, (file) => {
        if (gen !== pluginLoadGeneration) return // 新增：过期检查
        pluginFiles.value.push(file)
      })
      if (gen !== pluginLoadGeneration) return   // 新增：最终赋值前再检
      pluginFiles.value = results
    } catch (e) {
      console.error('[Files] Plugin stream load failed:', e)
    }
    if (gen === pluginLoadGeneration) {         // 新增：仅当前代次标记完成
      pluginLoaded.value = true
      setupLazyThumbnails()
    }
  }
})
```

**关键约束**：
- 不改变 SSE 流式特性（onItem 回调仍然逐文件推送）
- 不增加延迟（generation 检查是 O(1) 整数比较）
- 与 `loadFiles()` 的模式完全一致（参考 L456-L483）

---

### 步骤 2：新增筛选状态变量

**文件**：`src/views/Files.vue`（script 区域，约 L988 后）

```typescript
const sizeFilterMin = ref<number | null>(null)    // 最小字节
const sizeFilterMax = ref<number | null>(null)    // 最大字节
const timeFilterFrom = ref<string | null>(null)    // ISO 起始日期
const timeFilterTo = ref<string | null>(null)      // ISO 结束日期
const showPluginFilters = ref(false)               // 筛选面板展开状态
```

**预设快捷选项**（提升 UX）：
```typescript
const SIZE_PRESETS = [
  { label: '< 1MB', max: 1024 * 1024 },
  { label: '1MB - 10MB', min: 1024 * 1024, max: 10 * 1024 * 1024 },
  { label: '10MB - 100MB', min: 10 * 1024 * 1024, max: 100 * 1024 * 1024 },
  { label: '> 100MB', min: 100 * 1024 * 1024 },
]
const TIME_PRESETS = [
  { label: '今天', days: 0 },
  { label: '近 3 天', days: 3 },
  { label: '近 7 天', days: 7 },
  { label: '近 30 天', days: 30 },
]
```

---

### 步骤 3：改造 filteredPluginFiles computed

在现有过滤链中追加两步（在排序之前）：

```typescript
const filteredPluginFiles = computed(() => {
  if (!selectedPlugin.value) return []
  let list: FileItem[]

  // ① 现有：tab 过滤（source/container）
  if (pluginTab.value === 'container') {
    list = pluginFiles.value.filter(f => f.isEncrypted || ...)
  } else {
    list = pluginFiles.value.filter(f => !f.isEncrypted)
  }

  // ② 现有：文本搜索
  const query = searchQuery.value.trim().toLowerCase()
  if (query) {
    list = list.filter(f => f.name.toLowerCase().includes(query))
  }

  // ★ 新增 ③：大小范围筛选（独立生效）
  if (sizeFilterMin.value !== null) {
    list = list.filter(f => (f.size || 0) >= sizeFilterMin.value!)
  }
  if (sizeFilterMax.value !== null) {
    list = list.filter(f => (f.size || 0) <= sizeFilterMax.value!)
  }

  // ★ 新增 ④：时间范围筛选（独立生效）
  if (timeFilterFrom.value !== null) {
    const from = new Date(timeFilterFrom.value).getTime()
    list = list.filter(f => {
      const m = f.modified ? new Date(f.modified).getTime() : 0
      return m >= from
    })
  }
  if (timeFilterTo.value !== null) {
    const to = new Date(timeFilterTo.value).getTime()
    list = list.filter(f => {
      const m = f.modified ? new Date(f.modified).getTime() : 0
      return m <= to
    })
  }

  // ⑤ 现有：排序 + tag 注入（不变）
  list.sort((a, b) => { /* ... */ })
  const tagMap = fileTagMap.value
  return list.map(f => ({ ...f, _tags: tagMap[f.path] || [] }))
})
```

**设计要点**：
- ③ 和 ④ **互相独立**，可以同时生效也可以单独使用
- 使用 `!== null` 判断允许 `0` 作为合法值
- 时间比较用 `.getTime()` 转 timestamp 避免时区问题

---

### 步骤 4：添加筛选 UI

**位置**：插件视图的 `<ion-segment>` 下方（约 L146 前），新增筛选工具栏：

```html
<!-- 筛选触发按钮行 -->
<ion-item button detail @click="showPluginFilters = !showPluginFilters">
  <ion-icon :icon="filterOutline" slot="start"></ion-icon>
  <ion-label>筛选</ion-label>
  <ion-badge v-if="activeFilterCount > 0" slot="end" color="primary">
    {{ activeFilterCount }}
  </ion-badge>
</ion-item>

<!-- 可折叠筛选面板 -->
<ion-list v-if="showPluginFilters" :inset="true">
  <!-- 大小范围 -->
  <ion-item>
    <ion-label position="stacked">大小范围 (Bytes)</ion-label>
    <div style="display:flex;gap:8px;align-items:center;width:100%">
      <ion-input type="number" placeholder="最小"
        :value="sizeFilterMin?.toString()"
        @ionInput="sizeFilterMin = $event.detail.value ? Number($event.detail.value) : null">
      </ion-input>
      <span>~</span>
      <ion-input type="number" placeholder="最大"
        :value="sizeFilterMax?.toString()"
        @ionInput="sizeFilterMax = $event.detail.value ? Number($event.detail.value) : null">
      </ion-input>
      <ion-button fill="clear" size="small" @click="sizeFilterMin=null;sizeFilterMax=null">清除</ion-button>
    </div>
    <!-- 快捷预设 -->
    <div class="filter-chips">
      <ion-chip v-for="p in SIZE_PRESETS" :key="p.label"
        :outline="!isSizePresetActive(p)"
        @click="applySizePreset(p)">
        {{ p.label }}
      </ion-chip>
    </div>
  </ion-item>

  <!-- 时间范围 -->
  <ion-item>
    <ion-label position="stacked">修改时间</ion-label>
    <div style="display:flex;gap:8px;align-items:center;width:100%">
      <ion-input type="date" placeholder="起始"
        :value="timeFilterFrom"
        @ionInput="timeFilterFrom = $event.detail.value || null">
      </ion-input>
      <span>~</span>
      <ion-input type="date" placeholder="结束"
        :value="timeFilterTo"
        @ionInput="timeFilterTo = $event.detail.value || null">
      </ion-input>
      <ion-button fill="clear" size="small" @click="timeFilterFrom=null;timeFilterTo=null">清除</ion-button>
    </div>
    <!-- 快捷预设 -->
    <div class="filter-chips">
      <ion-chip v-for="p in TIME_PRESETS" :key="p.label"
        :outline="!isTimePresetActive(p)"
        @click="applyTimePreset(p)">
        {{ p.label }}
      </ion-chip>
    </div>
  </ion-item>

  <!-- 全部清除 -->
  <ion-item button @click="clearAllPluginFilters">
    <ion-icon :icon="closeCircleOutline" slot="start" color="danger"></ion-icon>
    <ion-label color="danger">清除所有筛选</ion-label>
  </ion-item>
</ion-list>
```

**辅助 computed**：
```typescript
const activeFilterCount = computed(() => {
  let c = 0
  if (sizeFilterMin.value !== null) c++
  if (sizeFilterMax.value !== null) c++
  if (timeFilterFrom.value !== null) c++
  if (timeFilterTo.value !== null) c++
  return c
})

function applySizePreset(preset: typeof SIZE_PRESETS[0]) {
  sizeFilterMin.value = preset.min ?? null
  sizeFilterMax.value = preset.max ?? null
}
function applyTimePreset(preset: typeof TIME_PRESETS[0]) {
  const now = new Date()
  const from = new Date(now)
  from.setDate(from.getDate() - preset.days)
  from.setHours(0, 0, 0, 0)
  timeFilterFrom.value = from.toISOString()
  if (preset.days === 0) {
    timeFilterTo.value = now.toISOString()
  } else {
    timeFilterTo.value = null
  }
}
function clearAllPluginFilters() {
  sizeFilterMin.value = null
  sizeFilterMax.value = null
  timeFilterFrom.value = null
  timeFilterTo.value = null
}
```

**import 补充**：
```typescript
import { filterOutline, closeCircleOutline } from 'ionicons/icons'
```

---

### 步骤 5：CSS 样式补充

**文件**：`src/views/Files.vue` `<style scoped>`

```css
.filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.filter-chips ion-chip {
  font-size: 12px;
  --padding-start: 8px;
  --padding-end: 8px;
  cursor: pointer;
}
```

---

## 三、验证清单

| # | 验证项 | 方法 |
|---|--------|------|
| 1 | 快速切换插件无残留 | 图片→视频→音频快速连续点击，检查列表纯净 |
| 2 | 流式首屏不退化 | 切换后首个文件秒显 |
| 3 | 大小筛选独立生效 | 设 1MB~10MB，确认只有该范围文件显示 |
| 4 | 时间筛选独立生效 | 设"近 7 天"，确认时间外文件隐藏 |
| 5 | 两筛同时生效 | 同时设大小+时间，确认交集正确 |
| 6 | 快捷预设可用 | 点击"近 30 天"，自动填入时间值 |
| 7 | 清除功能正常 | 单项清除 / 全部清除恢复全量 |
| 8 | 构建通过 | `vue-tsc --noEmit && vite build` |

## 四、不做的事

- ❌ 不改后端 API（纯前端 computed 过滤，利用已有 size/modified 字段）
- ❌ 不引入虚拟滚动（已证明在此场景有问题）
- ❌ 不改变现有流式加载架构
