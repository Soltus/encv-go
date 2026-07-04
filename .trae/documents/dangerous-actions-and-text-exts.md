# Plan：危险操作上移 + 文本扩展名下沉到插件系统

## 一、现状分析

### 需求 1：危险操作位置

**当前**：`AboutDetail.vue:233-245`
```
Level 1 (Settings.vue) → Level 2 (AboutDetail.vue) → 底部「危险操作」区域
  ├── 清除缓存 (handleClearCache)
  └── 重置设置 (handleResetSettings)
```

**问题**：危险操作藏在"关于"页面底部，用户难以发现，且"关于"页面的语义与"重置/清除"不匹配。

**目标**：移到 `Settings.vue` 一级页面底部（DevTools 和关于之间）。

### 需求 2：自定义文本预览扩展名位置

**当前**：`Settings.vue:268-286` 作为独立的 **preview 段**
```
Level 1 (Settings.vue)
├── ...
├── 🔴 Preview 段（独立区域）：
│   └── 自定义文本预览扩展名 (customTextExts)
│       → 存储：preview.text_extensions
│       → 无格式校验
│       → 无冲突检测
├── 插件设置 → goPlugins() (Level 2)
```

**数据流链路**：
```
Settings.vue (编辑 preview.text_extensions)
    ↓ POST /api/config
后端存储 config.preview.text_extensions
    ↓ GET /api/file/text-preview-exts
api/encv.ts fetchTextPreviewExts()
    ↓ Set<string>
FilePreview.vue isTextPreviewable(name)
```

**问题**：
1. 文本预览扩展名是**文本插件的功能属性**，却作为全局 preview 配置存在
2. 无格式校验（用户可输入任意字符）
3. 无冲突检测（可能与插件容器扩展名冲突，如 `.sccgt`）
4. 与插件系统的 text 插件段 (`plugin_settings.text`) 物理分离

**目标**：将 customTextExts 从一级页面独立设置改为**文本插件设置内的子字段**。

---

## 二、目标结构

### 2.1 危险操作（需求 1）

```
Level 1 (/tabs/settings — Settings.vue) [修改]
├── 外观 / 播放器 / 连接（入口） / 缓存
├── Schema 配置（排除 server/admin/webdav）
├── 插件设置
├── DevTools
├── ⚠️ 危险操作 ← NEW HERE（从 AboutDetail 移入）
│   ├── 🗑️ 清除缓存
│   └── 🔄 重置所有设置
├── 关于
└── 编辑原始配置

Level 2 (/tabs/settings/about — AboutDetail.vue) [修改]
├── 版本信息（不变）
├── 开源许可（不变）
└── ~~危险操作~~ ← REMOVED
```

### 2.2 文本扩展名（需求 2）

```
Level 1 (/tabs/settings — Settings.vue) [修改]
├── ...
├── ~~Preview 段~~ ← 移除整个区域
├── 插件设置 → goPlugins()
...

Level 2 (/tabs/settings/plugins — PluginSettings.vue) [修改]
├── 选择插件（video/image/audio/text/wps/pdf）
├── 当选中 text 插件时：
│   ├── ext (容器扩展名 .sccgt) — 已有
│   └── 📝 custom_text_extensions ← NEW（从 Settings.preview 移入）
│       ├── 格式校验（每个扩展名必须为合法标识符）
│       └── 冲突检测（不能与任何插件的 containerExtension 重叠）
└── 后缀冲突警告（已有 alist_encrypt 的逻辑）
```

**数据流变更**：

```
Before:
  Settings.vue → preview.text_extensions → API → FilePreview

After:
  PluginSettings.vue → plugin_settings.text.custom_text_extensions → API → FilePreview
                    （schema 新字段）           （后端兼容读取新路径）
```

---

## 三、实施步骤

### Step 1：危险操作从 AboutDetail 移到 Settings（需求 1）

#### 1.1 修改 Settings.vue

**模板**：在「DevTools」和「关于」之间插入危险操作区域

```vue
<!-- 在 goDevTools 条目之后、goAbout 条目之前插入 -->
<ion-list :inset="true">
  <ion-list-header color="danger">
    <ion-label>{{ t('settings.dangerousActions') }}</ion-label>
  </ion-list-header>

  <ion-item button @click="handleClearCache" detail>
    <ion-icon :icon="trashOutline" slot="start" color="danger"></ion-icon>
    <ion-label>
      <h3>{{ t('about.clearCache') }}</h3>
      <p>{{ t('about.clearCacheDesc') }}</p>
    </ion-label>
  </ion-item>

  <ion-item button @click="handleResetSettings" detail>
    <ion-icon :icon="refreshOutline" slot="start" color="danger"></ion-icon>
    <ion-label>
      <h3>{{ t('about.resetSettings') }}</h3>
      <p>{{ t('about.resetSettingsDesc') }}</p>
    </ion-label>
  </ion-item>
</ion-list>
```

**Script**：从 AboutDetail.vue 复制以下函数到 Settings.vue：
- `handleClearCache()` — 清除缓存逻辑
- `handleResetSettings()` — 重置设置逻辑（含确认对话框）

**Import**：新增 `trashOutline, refreshOutline` 图标 + `clearCache` API 导入

#### 1.2 修改 AboutDetail.vue

**删除** L233-245 整个 `<ion-list>`（危险操作区域）及其关联的 script 函数。

### Step 2：Schema 新增 custom_text_extensions 字段（需求 2）

**文件**：`src/config/schema.json`

在 `plugin_settings.text` 对象内新增字段：

```json
{
  "key": "text",
  "type": "object",
  "properties": {
    "ext": {
      "key": "ext",
      "type": "string",
      "default": ".sccgt",
      "description": "Text container extension"
    },
    "custom_text_extensions": {          // ← NEW
      "key": "custom_text_extensions",   // ← NEW
      "type": "string",                  // ← NEW
      "default": "",                     // ← NEW
      "description": "Comma-separated additional extensions for text preview (e.g. log,ini,cfg,toml,yaml,json)"  // ← NEW
    }                                    // ← NEW
  }
}
```

**为什么用 string 而非 array？**
- 与现有 Settings.vue 的 `customTextExts` 实现一致（逗号分隔字符串）
- Schema parser 已支持 string 类型渲染为 ion-input
- 用户输入体验更好（不需要动态增删行）

### Step 3：PluginSettings.vue 增加 text 插件的 custom_text_extensions 渲染+校验（需求 2）

**文件**：`src/views/PluginSettings.vue`

#### 3.1 模板改动

在 text 插件的 properties 循环中，当 `child.key === 'custom_text_extensions'` 时使用特殊渲染：

```vue
<!-- 在 v-for child 循环内部，existing template 之后 -->
<template v-if="pluginSection.key === 'plugin_settings' && currentPluginKey === 'text' && child.key === 'custom_text_extensions'">
  <ion-item>
    <ion-icon :icon="textOutline" slot="start"></ion-icon>
    <ion-input
      :value="String(getValue(['plugin_settings', 'text', 'custom_text_extensions']) ?? '')"
      type="text"
      :label="t('settings.customTextExts')"
      label-placement="stacked"
      :placeholder="t('settings.customTextExtsHint')"
      @ionInput="handleCustomTextExtsInput($event)"
      :error-text="textExtsError || undefined"
    ></ion-input>
  </ion-item>

  <!-- 格式校验错误提示 -->
  <div v-if="textExtsError" class="ext-validation-error">
    <ion-icon :icon="warningOutline"></ion-icon>
    <span>{{ textExtsError }}</span>
  </div>

  <!-- 冲突警告 -->
  <div v-if="textExtsConflicts.length > 0" class="ext-conflict-warning">
    <ion-icon :icon="warningOutline"></ion-icon>
    <span>{{ t('settings.textExtsConflictWarning', { extensions: textExtsConflicts.join(', ') }) }}</span>
  </div>
</template>
```

#### 3.2 Script 改动

新增变量和函数：

```typescript
import { usePluginExtensions } from '@/composables/usePluginExtensions'

const { getPluginContainerExtensions } = usePluginExtensions()  // 复用已有 composable
const textExtsError = ref('')
const textExtsConflicts = ref<string[]>([])

const currentPluginKey = computed(() => {
  // 从路由 query 或内部状态获取当前选中的插件 key
})

function handleCustomTextExtsInput(event: CustomEvent) {
  const raw = (event.target as HTMLInputElement).value
  const parsed = parseAndValidateTextExts(raw)

  if (parsed.errors.length > 0) {
    textExtsError.value = parsed.errors[0]
    return
  }

  textExtsError.value = ''
  setFieldValue(['plugin_settings', 'text', 'custom_text_extensions'], raw)

  // 冲突检测
  checkTextExtsConflicts(parsed.validExts)
}

interface TextExtsParseResult {
  validExts: string[]
  errors: string[]
}

function parseAndValidateTextExts(raw: string): TextExtsParseResult {
  const errors: string[] = []
  const validExts: string[] = []

  if (!raw.trim()) return { validExts: [], errors: [] }

  const parts = raw.split(',').map(s => s.trim().toLowerCase()).filter(Boolean)

  for (const ext of parts) {
    // 格式校验：必须是字母数字组成，可选前导 .
    if (!/^[a-z][a-z0-9_\-]*$/.test(ext.replace(/^\./, ''))) {
      errors.push(t('settings.invalidExtFormat', { ext }))
      continue
    }

    // 不能以 . 开头后跟空（如 "." 或 ".."）
    if (/^\.+$/.test(ext)) {
      errors.push(t('settings.invalidExtDots', { ext }))
      continue
    }

    validExts.push(ext.startsWith('.') ? ext : '.' + ext)
  }

  return { validExts, errors }
}

function checkTextExtsConflicts(extensions: string[]) {
  if (extensions.length === 0) {
    textExtsConflicts.value = []
    return
  }

  // 获取所有插件的 containerExtension
  const containerExts = getPluginContainerExtensions()

  const conflicts = extensions.filter(ext =>
    containerExts.some(ce => ce.toLowerCase() === ext.toLowerCase())
  )

  textExtsConflicts.value = conflicts
}
```

### Step 4：修改 Settings.vue — 移除 Preview 段（需求 2）

**删除** L268-286 整个 preview `<ion-list>` 区域（含 customTextExts 输入框）。

**删除** script 中相关的：
- `customTextExts` ref (L408)
- loadConfig 中 `preview?.text_extensions` 读取 (L470-471)
- `handleCustomTextExtsChange` 函数 (L480-493)

### Step 5：后端兼容性处理（需求 2）

**关键约束**：前端 schema 字段从 `preview.text_extensions` 迁移到 `plugin_settings.text.custom_text_extensions`，但后端 API `/api/file/text-preview-exts` 可能仍读旧路径。

**解决方案（双写策略）**：

在 PluginSettings.vue 的 saveConfig 或专门的保存逻辑中，同时写入两个路径：

```typescript
// PluginSettings.vue — 保存时双写
async function handleSaveConfig() {
  const textExtsRaw = String(getValue(['plugin_settings', 'text', 'custom_text_extensions']) ?? '')
  if (textExtsRaw.trim()) {
    // 写入新路径（schema 标准位置）
    // setFieldValue 已在上面完成

    // 同时写入旧路径（向后兼容后端 API）
    const parsed = textExtsRaw.split(',').map(s => s.trim()).filter(Boolean)
    setFieldValue(['preview', 'text_extensions'], parsed)
  }

  await saveConfig()
}
```

⚠️ **注意**：如果后端已更新为读取 `plugin_settings.text.custom_text_extensions`，则不需要双写。此步骤需根据实际后端实现决定是否保留。

### Step 6：i18n 键更新

**文件**：`src/composables/useI18n.ts`

新增键：

```typescript
// 中文
'settings.dangerousActions': '⚠️ 危险操作',
'settings.textExtsConflictWarning': '以下扩展名与插件容器扩展名冲突: {extensions}',
'settings.invalidExtFormat': '扩展名 "{ext}" 格式无效，仅允许字母、数字、连字符',
'settings.invalidExtDots': '扩展名 "{ext}" 无效',

// English
'settings.dangerousActions': 'Dangerous Actions',
'settings.textExtsConflictWarning': 'Extensions conflict with plugin containers: {extensions}',
'settings.invalidExtFormat': 'Invalid extension format "{ext}", only alphanumeric and hyphens allowed',
'settings.invalidExtDots': 'Invalid extension "{ext}"',
```

### Step 7：验证

1. **vue-tsc** 零错误
2. **vitest** 全部通过（208/208）
3. **vite build** 成功
4. **手动验证**：
   - Settings 页面底部出现「危险操作」区域（清除缓存 + 重置设置）
   - About 页面不再有危险操作
   - Settings 页面不再有 Preview/customTextExts 独立区域
   - 插件设置 → 选择 text → 出现 custom_text_extensions 输入框
   - 输入非法字符 → 显示校验错误
   - 输入 `.sccgt` → 显示冲突警告
   - 保存后 FilePreview 能正确识别新的文本扩展名

---

## 四、影响范围

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `src/views/Settings.vue` | **修改** | +危险操作区域; -Preview/customTextExts 区域 |
| `src/views/AboutDetail.vue` | **修改** | -危险操作区域 |
| `src/config/schema.json` | **修改** | plugin_settings.text 新增 custom_text_extensions |
| `src/views/PluginSettings.vue` | **修改** | text 插件特殊渲染 + 格式校验 + 冲突检测 |
| `src/composables/useI18n.ts` | **修改** | ~6 个新 i18n 键 |
| `src/composables/usePluginExtensions.ts` | **可能修改** | 新增 getPluginContainerExtensions() 如不存在 |

**不涉及的文件**：
- ❌ HttpServerDetail/AdminServerDetail/WebdavServerDetail（上一轮新建的 L3 页面）
- ❌ api/encv.ts（fetchTextPreviewExts 不改，仍从后端 API 读取）
- ❌ FilePreview.vue（不改，仍通过 fetchTextPreviewExts 获取列表）
