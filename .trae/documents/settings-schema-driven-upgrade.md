# 设置页面 Schema 驱动深度升级 Plan

## 问题诊断

### 当前状态：Schema 数据丰富，但 UI 渲染几乎没用

**schemaParser.ts 已解析但 ConfigFieldItem.vue 完全忽略的字段：**

| FieldDef 字段 | 来源 | 当前用途 | 应有用途 |
|---|---|---|---|
| `isSelect` | `enum` 存在时自动设为 true | ❌ 未使用 | 渲染 `<ion-select>` 而非 `<ion-input>` |
| `selectOptions` | `x-enum-labels` + `x-enum-descriptions` | ❌ 未使用 | 渲染带描述的选项卡片/下拉 |
| `description` | JSON Schema `description` | 仅作 placeholder | 应作为 helper text 显示 |
| `required` | JSON Schema `required` 数组 | 仅在 label 加 `*` | 需要更醒目的必填标记 |
| `enum` | JSON Schema `enum` | ❌ 未使用 | 驱动 select 选项 |
| `default` | JSON Schema `default` | 显示"默认: xxx"文本 | 需要重置到默认值按钮 |

### 硬编码 vs Schema 驱动的对比

**Settings.vue 第 261-276 行**：`log.level` 被硬编码为 `<ion-select>` + 4 个硬编码选项：
```vue
<!-- ❌ 硬编码：如果后端新增日志级别，前端不会自动显示 -->
<ion-item v-else-if="section.key === 'log' && child.key === 'level'">
  <ion-select :value="String(getValue(['log', 'level']) ?? 'info')" ...>
    <ion-select-option value="debug">DEBUG</ion-select-option>
    <ion-select-option value="info">INFO</ion-select-option>
    <ion-select-option value="warn">WARN</ion-select-option>
    <ion-select-option value="error">ERROR</ion-select-option>
  </ion-select>
</ion-item>
```

**PluginSettings.vue 第 71-88 行**：`isSelect` + `selectOptions` 渲染为预设卡片——这是正确的 schema 驱动方式，但代码没有复用到 ConfigFieldItem。

### 核心矛盾

`ConfigFieldItem` 被设计为通用组件，但它只处理了 2 种情况（boolean toggle / text input），而 `FieldDef` 接口定义了 6+ 种渲染分支。Settings.vue 和 PluginSettings.vue 不得不在各自模板中硬编码处理 `isSelect`、`log.level` 等特殊情况。

### 关键缺失：服务端配置 vs 本地偏好的视觉区分

当前设置页面中，**服务端 JSON 配置**（保存到 config.json，可同步到 PC/WebDAV）和**本地客户端偏好**（仅存 localStorage，如暗黑模式、语言、主题色）混在一起，用户无法区分。

**两类设置的本质差异**：

| 属性 | 服务端 JSON 配置（Schema 驱动） | 本地客户端偏好（localStorage） |
|------|------|------|
| 存储位置 | 服务端 `config.json` | 浏览器 `localStorage` |
| 同步性 | ✅ PC/移动端/WebDAV 同步 | ❌ 仅当前设备 |
| 数据来源 | JSON Schema 定义 | 前端硬编码 |
| 渲染方式 | ConfigFieldItem（schema 驱动） | 独立 ion-item/ion-toggle |
| 典型字段 | password, output_path, log.level, plugin_settings | 暗黑模式, 语言, 主题色, 视频播放器 |

**用户需求**：一眼看出哪些设置是"同步的"（保存到服务端 JSON），哪些是"仅本地的"。

---

## 设计参考：优秀 Schema 驱动配置 UI 案例

### 1. VS Code Settings（最经典的 schema-driven settings UI）

**核心设计模式：**
- **JSON Schema → UI 控件自动映射**：`type: string` + `enum` → dropdown；`type: boolean` → toggle；`type: integer` → number input；`type: string` → text input
- **Description 作为 helper text**：每个设置下方显示描述文字，支持 Markdown
- **Modified indicator**：左侧蓝色竖线标记已修改的值
- **Reset to default**：hover 时出现齿轮图标，点击重置为默认值
- **Enum descriptions**：下拉选项显示描述文字（如 `balanced: 画质较好、体积适中`）
- **分组标题**：`--- Section Title ---` 语法从 description 中提取分组名
- **User vs Workspace 区分**：VS Code 用标签页区分"用户设置"和"工作区设置"，视觉上用不同图标标记

### 2. macOS System Preferences

- **分组卡片**：每个设置区域是一个圆角卡片
- **Helper text**：灰色小字在控件下方
- **Select 控件**：原生下拉菜单，选项简洁
- **iCloud 同步标记**：系统偏好中 iCloud 同步的设置有云图标标记

### 3. Ant Design ProForm

- **Schema → Form**：JSON Schema 驱动表单渲染
- **`x-decorator` / `x-component`**：自定义扩展字段控制渲染方式
- **联动校验**：基于 schema 的 required/type 自动校验

### 4. Grafana Settings

- **Server Config vs User Prefs 分区**：用不同的 section header + 图标区分
- **云同步标记**：服务端配置有 `<cloud-outline>` 图标

### 本项目已有的优秀实践

**PluginSettings.vue 的 preset-card 模式**（第 71-88 行）已经是 schema 驱动的正确实现：
```vue
<ion-item v-else-if="grandchild.isSelect && grandchild.selectOptions">
  <div class="preset-cards" slot="end">
    <div v-for="opt in grandchild.selectOptions" class="preset-card"
      :class="{ 'preset-card-active': getValue(...) === opt.value }"
      @click="setValue(..., opt.value)">
      <div class="preset-card-title">{{ opt.label }}</div>
      <div class="preset-card-desc">{{ opt.description }}</div>
    </div>
  </div>
</ion-item>
```

**问题**：这段代码在 PluginSettings.vue 中，ConfigFieldItem 完全没有。

---

## 实施步骤

### Step 1: ConfigFieldItem 全面 Schema 驱动化

**目标**：让 ConfigFieldItem 成为真正的 schema-driven 渲染组件，覆盖 FieldDef 的所有字段。

**改动文件**：`/workspace/app/encv-mobile/src/components/ConfigFieldItem.vue`

**新增渲染分支**：

```
field.type === 'boolean'
  → ion-toggle + ion-label + description helper text

field.isSelect && field.selectOptions?.length > 2
  → preset-card 模式（从 PluginSettings.vue 迁移）
  → 每个选项显示 label + description
  → 当前选中项高亮

field.isSelect && field.selectOptions?.length <= 2
  → ion-select + ion-select-option
  → 选项显示 label

field.type === 'integer'
  → ion-input type="number" + description helper text

field.type === 'string' (default)
  → ion-input + description helper text
  + isPassword → 密码切换按钮
  + isPath → 文件夹浏览按钮
```

**新增 UI 元素**：

1. **Description helper text**：所有字段类型都显示 `field.description` 作为 `<ion-note slot="helper">`
2. **Reset to default 按钮**：当 `isCustomized` 为 true 时，在字段右侧显示一个小重置按钮
3. **Required 标记**：在 label 后显示红色 `*`（已有，保持）
4. **默认值显示**：保持现有的"默认: xxx"文本，但移到 description 下方更自然的位置
5. **Modified indicator**：已修改的字段左侧显示主题色竖线
6. **云同步标记**：每个 schema 驱动字段右侧显示云图标，表示"此配置保存到服务端，可跨设备同步"

**模板结构**：

```vue
<template>
  <!-- Boolean: ion-item with toggle + label + description -->
  <ion-item v-if="field.type === 'boolean'" class="config-field" :class="{ 'field-modified': isCustomized }">
    <ion-icon :icon="icon" slot="start"></ion-icon>
    <ion-label>
      {{ label }}
      <span v-if="field.required" class="required-mark">*</span>
    </ion-label>
    <ion-toggle slot="end" :checked="!!modelValue" @ionChange="..." />
    <ion-note slot="helper" class="field-description">
      {{ field.description }}
      <span v-if="hasDefault" class="default-hint">（{{ t('settings.default') }}: {{ formatDefault(defaultVal) }}）</span>
    </ion-note>
    <ion-badge v-if="isTaskOverridable" slot="end" color="light" class="task-override-badge">...</ion-badge>
    <ion-button v-if="isCustomized" slot="end" fill="clear" size="small" @click="resetToDefault">
      <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
    </ion-button>
  </ion-item>

  <!-- Select with preset cards (3+ options) -->
  <ion-item v-else-if="field.isSelect && field.selectOptions && field.selectOptions.length > 2"
    class="config-field" :class="{ 'field-modified': isCustomized }">
    <ion-icon :icon="icon" slot="start"></ion-icon>
    <ion-label>
      <h3>{{ label }}<span v-if="field.required" class="required-mark">*</span></h3>
      <p v-if="field.description" class="field-description-text">{{ field.description }}</p>
    </ion-label>
    <div class="preset-cards" slot="end">
      <div v-for="opt in field.selectOptions" :key="opt.value" class="preset-card"
        :class="{ 'preset-card-active': String(modelValue) === opt.value }"
        @click="$emit('update:modelValue', opt.value)">
        <div class="preset-card-title">{{ opt.label }}</div>
        <div v-if="opt.description" class="preset-card-desc">{{ opt.description }}</div>
      </div>
    </div>
    <ion-note slot="helper" v-if="hasDefault" class="default-hint">
      {{ t('settings.default') }}: {{ defaultOptionLabel }}
    </ion-note>
    <ion-button v-if="isCustomized" slot="end" fill="clear" size="small" @click="resetToDefault">
      <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
    </ion-button>
  </ion-item>

  <!-- Select with dropdown (1-2 options) -->
  <ion-item v-else-if="field.isSelect" class="config-field" :class="{ 'field-modified': isCustomized }">
    <ion-icon :icon="icon" slot="start"></ion-icon>
    <ion-select :value="String(modelValue ?? '')" :label="label" label-placement="stacked"
      interface="action-sheet" mode="ios" @ionChange="...">
      <ion-select-option v-for="opt in field.selectOptions" :key="opt.value" :value="opt.value">
        {{ opt.label }}
      </ion-select-option>
    </ion-select>
    <ion-note slot="helper" class="field-description">
      {{ field.description }}
      <span v-if="hasDefault">（{{ t('settings.default') }}: {{ defaultOptionLabel }}）</span>
    </ion-note>
    <ion-button v-if="isCustomized" slot="end" fill="clear" size="small" @click="resetToDefault">
      <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
    </ion-button>
  </ion-item>

  <!-- Text/Integer input -->
  <ion-item v-else class="config-field" :class="{ 'field-modified': isCustomized }">
    <ion-icon :icon="icon" slot="start"></ion-icon>
    <ion-input ... />
    <ion-note slot="helper" class="field-description">
      {{ field.description }}
      <span v-if="hasDefault">（{{ t('settings.default') }}: {{ formatDefault(defaultVal) }}）</span>
    </ion-note>
    <!-- password toggle / path browse / reset button -->
  </ion-item>
</template>
```

**新增 computed**：

```typescript
const hasDefault = computed(() => props.field.default !== undefined)
const defaultOptionLabel = computed(() => {
  if (!props.field.selectOptions || !props.field.default) return formatDefault(defaultVal.value)
  const opt = props.field.selectOptions.find(o => o.value === String(props.field.default))
  return opt ? opt.label : formatDefault(defaultVal.value)
})
```

**新增 emit**：

```typescript
defineEmits<{
  'update:modelValue': [value: unknown]
  input: [event: CustomEvent]
  browse: []
  reset: []  // 新增：重置到默认值
}>()
```

### Step 2: 服务端配置 vs 本地偏好的视觉区分系统

**目标**：用户一眼区分"保存到服务端 JSON 可跨设备同步"的配置和"仅存本地 localStorage"的偏好。

**设计方案：双区域 + 云同步标记**

Settings.vue 的设置列表分为两个视觉区域：

#### 区域 A：本地偏好（Appearance 区域）

保持当前的 `ion-list` 样式，但添加一个区域标题 badge 标识"仅本设备"：

```vue
<ion-list>
  <ion-list-header>
    <ion-label>{{ t('settings.appearance') }}</ion-label>
    <ion-badge slot="end" color="medium" class="scope-badge">
      <ion-icon :icon="phonePortraitOutline" size="small"></ion-icon>
      {{ t('settings.localOnly') }}
    </ion-badge>
  </ion-list-header>
  <!-- 暗黑模式、语言、主题色、播放器、屏幕方向 -->
</ion-list>
```

#### 区域 B：服务端配置（Schema 驱动区域）

每个 schema 驱动的 section 添加云同步标记：

```vue
<ion-list v-for="section in schemaFields" :key="section.key">
  <ion-list-header>
    <ion-label>{{ sectionTitle }}</ion-label>
    <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
      <ion-icon :icon="cloudOutline" size="small"></ion-icon>
      {{ t('settings.synced') }}
    </ion-badge>
  </ion-list-header>
  <!-- ConfigFieldItem 渲染的 schema 驱动字段 -->
</ion-list>
```

**视觉差异总结**：

| 元素 | 本地偏好 | 服务端配置 |
|------|---------|-----------|
| 区域标题 badge | 📱 仅本设备（灰色） | ☁️ 可同步（主题蓝色） |
| 字段左侧图标 | 无特殊标记 | 无特殊标记 |
| 字段 helper text | 无"默认值"提示 | 有"默认: xxx"提示 |
| 字段右侧 | 无云图标 | ☁️ 小云图标 |
| 修改标记 | 无 | 左侧主题色竖线 |
| 重置按钮 | 无 | 已修改时显示 ↺ |

**scope-badge 样式**：

```css
.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.scope-synced {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
}
```

**ConfigFieldItem 新增云同步图标**：

在每个 schema 驱动字段的 ion-item 右侧（或 helper text 末尾），添加一个小云图标：

```vue
<ion-icon :icon="cloudOutline" slot="end" class="sync-indicator" />
```

```css
.sync-indicator {
  font-size: 14px;
  color: var(--ion-color-primary);
  opacity: 0.5;
  margin-left: 4px;
}
```

### Step 3: 消除 Settings.vue 中的硬编码 select

**目标**：删除 Settings.vue 中 `log.level` 的硬编码 `<ion-select>`，改由 ConfigFieldItem 的 schema 驱动渲染处理。

**改动文件**：`/workspace/app/encv-mobile/src/views/Settings.vue`

**删除代码**（第 261-276 行）：
```vue
<!-- ❌ 删除这段硬编码 -->
<ion-item v-else-if="section.key === 'log' && child.key === 'level'">
  <ion-select ...>
    <ion-select-option value="debug">DEBUG</ion-select-option>
    <ion-select-option value="info">INFO</ion-select-option>
    <ion-select-option value="warn">WARN</ion-select-option>
    <ion-select-option value="error">ERROR</ion-select-option>
  </ion-select>
</ion-item>
```

**前提**：后端 schema 中 `log.level` 需要添加 `enum` 和 `x-enum-labels`（见 Step 5）。

### Step 4: 消除 Settings.vue 中的硬编码 boolean 渲染

**目标**：Settings.vue 第 254-260 行的 boolean 硬编码 toggle 也应走 ConfigFieldItem。

**当前代码**：
```vue
<ion-item v-if="child.type === 'boolean'">
  <ion-toggle :checked="!!getValue([section.key, child.key])" @ionChange="...">{{ tField(child.key) }}</ion-toggle>
</ion-item>
```

**改为**：
```vue
<ConfigFieldItem
  :field="child"
  :model-value="getValue([section.key, child.key])"
  :label="fieldLabel(child.key, child.required)"
  :icon="getFieldIcon(child.key, child.type)"
  @update:model-value="setValue([section.key, child.key], $event)"
  @input="handleInput([section.key, child.key], child, $event)"
  @browse="handleBrowsePath([section.key, child.key], child)"
  @reset="resetFieldToDefault([section.key, child.key], child)"
/>
```

### Step 5: 后端 Schema 补充 enum 定义

**目标**：让 `log.level` 和 `alist_encrypt.algorithm` 的 enum 在 JSON Schema 中完整定义，使前端能自动渲染 select。

**改动文件**：`/workspace/app/encv-mobile/src/config/schema.json`

在 `LogConfig.level` 中添加：
```json
{
  "type": "string",
  "enum": ["debug", "info", "warn", "error"],
  "x-enum-labels": {
    "debug": "DEBUG",
    "info": "INFO",
    "warn": "WARN",
    "error": "ERROR"
  },
  "x-enum-descriptions": {
    "debug": "详细的插件匹配、文件处理等调试信息",
    "info": "关键操作信息和服务状态",
    "warn": "潜在问题但不影响运行的警告",
    "error": "仅输出错误信息"
  },
  "default": "info",
  "description": "日志级别..."
}
```

在 `alist_encrypt.algorithm` 中添加：
```json
{
  "type": "string",
  "enum": ["AES-128-CTR"],
  "x-enum-labels": {
    "AES-128-CTR": "AES-128-CTR"
  },
  "x-enum-descriptions": {
    "AES-128-CTR": "当前仅支持此算法"
  },
  "default": "AES-128-CTR"
}
```

### Step 6: PluginSettings.vue 复用 ConfigFieldItem

**目标**：将 PluginSettings.vue 中的 isSelect/boolean/input 渲染逻辑替换为 ConfigFieldItem。

**改动文件**：`/workspace/app/encv-mobile/src/views/PluginSettings.vue`

**替换逻辑**：
- 第 64-102 行（grandchild 渲染）→ 统一使用 ConfigFieldItem
- 第 130-170 行（child 渲染）→ 统一使用 ConfigFieldItem
- 保留 `custom_text_extensions` 的特殊校验逻辑（通过 ConfigFieldItem 的 `@input` 事件处理）
- 保留 `suffix` 的冲突检查逻辑

**注意**：PluginSettings.vue 的 `shouldShowBadge` 和 preset-card 样式需要迁移到 ConfigFieldItem 中。

### Step 7: 新增 resetFieldToDefault 函数

**目标**：支持每个字段"重置到默认值"操作。

**改动文件**：`/workspace/app/encv-mobile/src/composables/useConfig.ts`

```typescript
function resetFieldToDefault(path: string[], field: FieldDef) {
  const defaultVal = getDefaultValue(field)
  setFieldValue(path, defaultVal)
}
```

### Step 8: 样式升级

**改动文件**：`/workspace/app/encv-mobile/src/components/ConfigFieldItem.vue`

**新增样式**：

```css
/* 已修改标记：左侧主题色竖线 */
.config-field.field-modified {
  --border-width: 0 0 0 3px;
  --border-style: solid;
  --border-color: var(--ion-color-primary);
  --padding-start: 13px;  /* 补偿 3px border */
}

/* Description helper text */
.field-description {
  font-size: 12px;
  color: var(--ion-color-medium);
  white-space: normal;
}

/* Required mark */
.required-mark {
  color: var(--ion-color-danger);
  margin-left: 2px;
}

/* Preset cards（从 PluginSettings.vue 迁移） */
.preset-cards { ... }
.preset-card { ... }
.preset-card-active { ... }
.preset-card-title { ... }
.preset-card-desc { ... }

/* Reset button */
.reset-btn {
  --padding-start: 6px;
  --padding-end: 6px;
  min-width: 32px;
  min-height: 32px;
}

/* 云同步标记 */
.sync-indicator {
  font-size: 14px;
  color: var(--ion-color-primary);
  opacity: 0.5;
  margin-left: 4px;
}

/* 区域 scope badge */
.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.scope-synced {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
}
```

### Step 9: Badge 系统统一

**目标**：将 PluginSettings.vue 的 `shouldShowBadge` + `config-badge` 样式迁移到 ConfigFieldItem。

**ConfigFieldItem 新增 badge 渲染**：

```vue
<span v-if="field.isV4" class="config-badge badge-v4">v4</span>
<span v-else-if="field.platform === 'mobile'" class="config-badge badge-mobile">移动端</span>
```

---

## 改动文件清单

| 文件 | 改动类型 | 描述 |
|------|---------|------|
| `src/components/ConfigFieldItem.vue` | **重大重写** | 全面 schema 驱动化：新增 select/preset-card/description/reset-to-default/modified-indicator/badge/云同步标记 |
| `src/views/Settings.vue` | **精简 + 双区域** | 删除硬编码 `log.level` select + 硬编码 boolean toggle；新增"仅本设备"/"可同步"区域标记 |
| `src/views/PluginSettings.vue` | **精简** | 删除重复的 isSelect/boolean/input 渲染逻辑，统一走 ConfigFieldItem |
| `src/config/schema.json` | **补充** | `log.level` 添加 enum + x-enum-labels + x-enum-descriptions；`alist_encrypt.algorithm` 添加 x-enum-labels |
| `src/composables/useConfig.ts` | **小改** | 新增 `resetFieldToDefault` 函数 |

---

## Schema 驱动映射总表（改造后）

| JSON Schema 特征 | FieldDef 字段 | UI 渲染 |
|---|---|---|
| `type: "boolean"` | `type: 'boolean'` | `<ion-toggle>` + label + description + ☁️ |
| `type: "string"` + `enum` + `x-enum-labels` (3+ 项) | `isSelect: true`, `selectOptions` | preset-card 卡片组 + ☁️ |
| `type: "string"` + `enum` (1-2 项) | `isSelect: true`, `selectOptions` | `<ion-select>` 下拉 + ☁️ |
| `type: "string"` + 含 password | `isPassword: true` | `<ion-input type="password">` + 眼睛切换 + ☁️ |
| `type: "string"` + 含 path/dir | `isPath: true` | `<ion-input>` + 文件夹浏览 + ☁️ |
| `type: "string"` (普通) | — | `<ion-input>` + ☁️ |
| `type: "integer"` | `type: 'integer'` | `<ion-input type="number">` + ☁️ |
| `description` | `description` | `<ion-note slot="helper">` helper text |
| `default` | `default` | 默认值提示 + 重置按钮 |
| `required: true` | `required: true` | label 红色 `*` |
| `x-platform: "mobile"` | `platform: 'mobile'` | 移动端 badge |
| `x-v4: true` | `isV4: true` | v4 badge |
| `x-enum-labels` | `selectOptions[].label` | 选项显示名 |
| `x-enum-descriptions` | `selectOptions[].description` | 选项描述文字 |

---

## 服务端配置 vs 本地偏好视觉区分总表

| 视觉元素 | 本地偏好（localStorage） | 服务端配置（Schema 驱动） |
|---------|---------|-----------|
| 区域标题 badge | 📱 仅本设备（灰色 medium） | ☁️ 可同步（主题蓝色 primary） |
| 字段 helper text | 无"默认值"提示 | 有"默认: xxx"提示 |
| 字段右侧图标 | 无云图标 | ☁️ 小云图标（半透明主题蓝） |
| 修改标记 | 无 | 左侧主题色竖线 |
| 重置按钮 | 无 | 已修改时显示 ↺ |
| 典型字段 | 暗黑模式、语言、主题色、播放器、屏幕方向 | password、output_path、log.level、plugin_settings |

---

## 验证标准

1. **Settings.vue 中无硬编码 select**：`log.level` 由 schema 驱动渲染
2. **Settings.vue 中无硬编码 boolean toggle**：所有 boolean 走 ConfigFieldItem
3. **PluginSettings.vue 中无重复渲染逻辑**：isSelect/boolean/input 全部走 ConfigFieldItem
4. **每个字段都显示 description**：helper text 可见
5. **isSelect 字段渲染为 select/preset-card**：而非 text input
6. **已修改字段有视觉标记**：左侧竖线 + 重置按钮
7. **服务端/本地视觉区分**：区域标题 badge + 字段云图标
8. **编译无报错**：`npm run build` 通过
