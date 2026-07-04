# 文件界面优化 + 加密解密任务目标路径 + 路径选择组件复用

## 需求分析

### 需求 1：搜索结果右侧箭头改为"打开所在文件夹"

**当前**：搜索结果中文件条目使用 `detail` 属性显示右侧箭头 `>`，点击整个条目执行 `handleFileClick(file)`（播放/预览文件）。

**目标**：右侧箭头区域点击 → 打开文件所在文件夹（导航到该目录）。条目主体点击行为不变。箭头区域需要有足够的点击区域（不只是图标实际面积）。

**实现**：
- 移除 `detail` 属性（Ionic 默认箭头太小）
- 在 `ion-item` 末尾添加自定义按钮区域，使用 `folderOpen` 图标
- 按钮区域最小点击尺寸 44x44px（符合移动端触控规范）
- 点击后调用 `navigateTo(file.parentDir)` — 需要从 `file.path` 计算父目录

### 需求 2：加密解密任务支持设置目标路径

**当前**：`createTask(type, sourcePath)` 只接受源路径，没有目标路径参数。新建任务弹窗只有源路径输入。

**目标**：
- 新建任务弹窗添加"目标路径"输入框（可选）
- 目标路径支持浏览文件夹选择
- 源路径和目标路径都需要校验（参考文件搜索框的防抖机制）
- API 调用需要支持 `targetPath` 参数

**实现**：
- `Tasks.vue` 新建任务弹窗添加 `newTaskTargetPath` 输入 + 浏览按钮
- `createTask` API 增加 `targetPath` 可选参数
- 路径校验：防抖 300ms + 非空时验证路径格式（以 `/` 开头）
- 目标路径浏览时使用文件夹选择模式（只选目录）

### 需求 3：统一复用文件/文件夹选择组件

**当前**：`FilePickerModal.vue` 只支持文件选择（点击文件 dismiss，点击目录进入）。设置页面的路径字段是纯文本输入框，没有浏览功能。

**目标**：
- `FilePickerModal` 支持两种模式：`file`（选文件）和 `folder`（选文件夹）
- 文件夹模式下：显示"选择此目录"按钮，点击目录进入子目录
- 设置页面所有路径字段（`output_path`、`server.dir`、`webdav.dir`、`log.file`、`plugin_cache_dir`）添加浏览按钮
- 浏览按钮统一调用 `FilePickerModal`，根据字段类型自动选择模式

**路径字段识别**：根据 `schema.json` 和 `fieldIconMap`，以下字段是路径类型：
- `output_path` → 文件夹选择（加密输出目录）
- `server.dir` → 文件夹选择（HTTP 根目录）
- `webdav.dir` → 文件夹选择（WebDAV 根目录）
- `log.file` → 文件选择（日志文件路径）
- `plugin_cache_dir` → 文件夹选择（插件缓存目录）

## 修改方案

### Step 1：FilePickerModal 支持文件夹选择模式

**文件**：`src/components/FilePickerModal.vue`

添加 `mode` prop（`'file' | 'folder'`），通过 `componentProps` 传入：

- `mode='file'`（默认）：现有行为不变，点击文件 dismiss
- `mode='folder'`：
  - 标题改为"选择文件夹"
  - 底部显示"选择此目录"按钮 + 当前路径显示
  - 点击目录进入子目录
  - 点击"选择此目录"按钮 dismiss，返回当前路径
  - 不显示文件（只显示目录）

```vue
// 新增 props
const props = defineProps<{
  mode?: 'file' | 'folder'
  initialPath?: string
}>()

// folder 模式下的底部操作栏
<div v-if="mode === 'folder'" class="folder-picker-bar">
  <text class="current-path">{{ currentPath }}</text>
  <ion-button @click="selectCurrentFolder">
    {{ t('files.selectThisFolder') }}
  </ion-button>
</div>

// folder 模式下过滤文件，只显示目录
const displayFiles = computed(() => {
  if (props.mode === 'folder') {
    return sortedFiles.value.filter(f => f.isDirectory)
  }
  return sortedFiles.value
})
```

### Step 2：搜索结果添加"打开所在文件夹"按钮

**文件**：`src/views/Files.vue`

修改搜索结果条目：
- 移除 `detail` 属性
- 搜索结果时在条目末尾添加文件夹图标按钮
- 按钮区域 44x44px 最小点击尺寸
- 点击后计算父目录并导航

```vue
<ion-item
  v-for="file in displayFiles"
  :key="file.path"
  @click="handleFileClick(file)"
  v-longpress="() => handleLongPress(file)"
>
  <!-- ... 现有内容 ... -->
  <!-- 搜索结果时显示"打开所在文件夹"按钮 -->
  <div v-if="searchQuery && !file.isDirectory" class="open-folder-btn" @click.stop="openContainingFolder(file)">
    <ion-icon :icon="folderOpen" class="open-folder-icon"></ion-icon>
  </div>
</ion-item>
```

```ts
function openContainingFolder(file: FileItem) {
  const parentDir = file.path.substring(0, file.path.lastIndexOf('/')) || '/'
  searchQuery.value = ''
  searchResults.value = null
  navigateTo(parentDir)
}
```

CSS：
```css
.open-folder-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 44px;
  min-height: 44px;
  margin-right: -12px;
  cursor: pointer;
}
.open-folder-icon {
  font-size: 20px;
  color: var(--ion-color-primary);
}
```

### Step 3：加密解密任务支持目标路径

**文件**：`src/views/Tasks.vue`

新建任务弹窗添加目标路径：

```vue
<ion-item>
  <ion-input
    v-model="newTaskPath"
    :label="t('tasks.sourcePath')"
    label-placement="stacked"
    placeholder="/path/to/file"
  ></ion-input>
  <ion-button slot="end" fill="clear" @click="handleBrowseSource">
    <ion-icon :icon="folderOpen" slot="icon-only"></ion-icon>
  </ion-button>
</ion-item>
<ion-item>
  <ion-input
    v-model="newTaskTargetPath"
    :label="t('tasks.targetPath')"
    label-placement="stacked"
    :placeholder="t('tasks.targetPathPlaceholder')"
  ></ion-input>
  <ion-button slot="end" fill="clear" @click="handleBrowseTarget">
    <ion-icon :icon="folderOpen" slot="icon-only"></ion-icon>
  </ion-button>
</ion-item>
```

源路径浏览 → `FilePickerModal` mode='file'
目标路径浏览 → `FilePickerModal` mode='folder'

**文件**：`src/api/encv.ts`

`createTask` 增加 `targetPath` 可选参数：

```ts
export async function createTask(type: TaskType, sourcePath: string, targetPath?: string): Promise<EncvTask> {
  const body: Record<string, unknown> = { type, sourcePath }
  if (targetPath) body.targetPath = targetPath
  const response = await fetch(`${baseUrl}/api/tasks`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  // ...
}
```

### Step 4：设置页面路径字段添加浏览按钮

**文件**：`src/views/Settings.vue` + `src/views/PluginSettings.vue`

在 `schemaParser.ts` 中为 `FieldDef` 添加 `isPath` 标识：

```ts
// schemaParser.ts
function isPathField(key: string): boolean {
  const pathKeys = ['output_path', 'dir', 'file', 'plugin_cache_dir', 'root']
  return pathKeys.includes(key) || key.includes('_path') || key.includes('_dir')
}

// 在 parseProperty 中添加
field.isPath = isPathField(key)
```

```ts
// FieldDef 接口添加
isPath?: boolean
```

在 Settings.vue 和 PluginSettings.vue 的输入框中，当 `field.isPath` 时添加浏览按钮：

```vue
<ion-item v-else>
  <ion-icon :icon="getFieldIcon(child.key, child.type)" slot="start"></ion-icon>
  <ion-input ...></ion-input>
  <ion-button
    v-if="child.isPath"
    slot="end"
    fill="clear"
    @click="handleBrowsePath([section.key, child.key], child)"
  >
    <ion-icon :icon="folderOpen" slot="icon-only"></ion-icon>
  </ion-button>
</ion-item>
```

浏览逻辑：
```ts
async function handleBrowsePath(path: string[], field: FieldDef) {
  const isFolder = field.key !== 'file' && field.key !== 'log_file'
  const currentVal = String(getFieldValue(path) || '/')
  const modal = await modalController.create({
    component: FilePickerModal,
    componentProps: {
      mode: isFolder ? 'folder' : 'file',
      initialPath: currentVal,
    },
  })
  await modal.present()
  const { data, role } = await modal.onDidDismiss()
  if (role === 'select' && data) {
    setFieldValue(path, data.path)
  }
}
```

### Step 5：添加 i18n 翻译

**文件**：`src/composables/useI18n.ts`

新增翻译键：
```ts
'files.selectFolder': '选择文件夹',
'files.selectThisFolder': '选择此目录',
'files.currentPath': '当前路径',
'tasks.targetPath': '目标路径',
'tasks.targetPathPlaceholder': '留空则使用默认输出路径',
```

### Step 6：路径校验

**文件**：`src/views/Tasks.vue`

参考文件搜索的防抖机制，对源路径和目标路径进行校验：

```ts
const sourcePathError = ref('')
const targetPathError = ref('')
let pathValidateTimer: ReturnType<typeof setTimeout> | null = null

function validatePath(path: string, isTarget: boolean) {
  if (pathValidateTimer) clearTimeout(pathValidateTimer)
  pathValidateTimer = setTimeout(() => {
    if (!path) {
      if (!isTarget) sourcePathError.value = t('tasks.pathRequired')
      else targetPathError.value = ''
      return
    }
    if (!path.startsWith('/')) {
      const error = t('tasks.pathMustBeAbsolute')
      if (isTarget) targetPathError.value = error
      else sourcePathError.value = error
      return
    }
    if (isTarget) targetPathError.value = ''
    else sourcePathError.value = ''
  }, 300)
}
```

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `src/components/FilePickerModal.vue` | 添加 mode/initialPath props，支持文件夹选择模式 |
| `src/views/Files.vue` | 搜索结果添加"打开所在文件夹"按钮 |
| `src/views/Tasks.vue` | 新建任务添加目标路径 + 浏览 + 校验 |
| `src/views/Settings.vue` | 路径字段添加浏览按钮 |
| `src/views/PluginSettings.vue` | 路径字段添加浏览按钮 |
| `src/api/encv.ts` | createTask 增加 targetPath 参数 |
| `src/config/schemaParser.ts` | FieldDef 添加 isPath 标识 |
| `src/composables/useI18n.ts` | 添加新翻译键 |
