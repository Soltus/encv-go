# 文件选择器 + 长按菜单 + 播放器空白修复 — 实施计划

## 问题概述

1. **新建加密解密任务只能输入路径** → 需要支持文件选择器浏览
2. **文件界面长按无操作菜单** → 需要长按弹出 action sheet
3. **播放器显示空白** → 即使后端异常，前端也应显示播放器组件而非空白

---

## 步骤 1：修复 Player.vue 空白问题

**根因**：`video-player` div 没有 `min-height`，ArtPlayer 加载失败时容器高度塌缩为 0 → 空白。且没有错误状态 UI，`error` 事件只弹 toast。

**变更**：

### Player.vue

1. 新增 `playerError` ref（`ref(false)`）
2. ArtPlayer `error` 事件回调中设置 `playerError.value = true`
3. 模板中 `video-player` div 条件改为 `v-if="isVideo && !playerError"`
4. 新增错误状态 UI 块（`v-if="isVideo && playerError"`），包含：
   - 错误图标（`alertCircle`）
   - "播放失败" 提示文字
   - "返回文件列表" 按钮（`router-link="/tabs/files"`）
   - "重试" 按钮（重新初始化 ArtPlayer）
5. CSS 修改：
   - `.video-player` 添加 `min-height: 30vh` + `aspect-ratio: 16/9`
   - 移除 `max-height: 40vh`（太小）
   - 新增 `.player-error` 样式

### useI18n.ts

新增 i18n key：
- `player.playError`: '播放失败' / 'Playback Error'
- `player.playErrorDesc`: '无法播放此文件，请检查文件格式或服务器状态。' / 'Unable to play this file. Check the file format or server status.'
- `player.backToFiles`: '返回文件列表' / 'Back to Files'
- `player.retryPlay`: '重试' / 'Retry'

---

## 步骤 2：创建 useFilePicker.ts composable

**目的**：管理文件选择状态，让 Tasks 页面和 Files 页面之间共享选择结果。

**实现**：

```ts
// useFilePicker.ts
import { ref } from 'vue'

const isPickerMode = ref(false)
const selectedPath = ref('')
const selectedName = ref('')
let resolvePicker: ((value: { path: string; name: string } | null) => void) | null = null

export function useFilePicker() {
  function startPicking(): Promise<{ path: string; name: string } | null> {
    return new Promise((resolve) => {
      isPickerMode.value = true
      selectedPath.value = ''
      selectedName.value = ''
      resolvePicker = resolve
    })
  }

  function confirmSelection(path: string, name: string) {
    selectedPath.value = path
    selectedName.value = name
    isPickerMode.value = false
    resolvePicker?.({ path, name })
    resolvePicker = null
  }

  function cancelPicking() {
    isPickerMode.value = false
    selectedPath.value = ''
    selectedName.value = ''
    resolvePicker?.(null)
    resolvePicker = null
  }

  return {
    isPickerMode,
    selectedPath,
    selectedName,
    startPicking,
    confirmSelection,
    cancelPicking,
  }
}
```

**关键设计**：
- 使用模块级 `ref` 保证跨组件共享状态
- `startPicking()` 返回 Promise，Tasks 页面 await 它即可拿到选择结果
- `resolvePicker` 回调机制让选择完成时自动 resolve Promise

---

## 步骤 3：修改 Files.vue — picker 模式 + 长按菜单

### 3a. Picker 模式

1. 引入 `useFilePicker`，解构 `isPickerMode`, `confirmSelection`, `cancelPicking`
2. 模板修改：
   - 顶部 toolbar：picker 模式时标题改为 "选择文件"，右侧添加 "取消" 按钮
   - `ion-item` 的 `@click`：picker 模式下，点击文件调用 `confirmSelection(file.path, file.name)` 后 `router.back()`；点击文件夹仍然导航进入
   - 底部添加 picker 提示条："点击文件以选择"
3. 样式：picker 提示条固定在底部

### 3b. 长按菜单

1. 引入 `actionSheetController` from `@ionic/vue`
2. 在 `ion-item` 上添加 `@longpress.prevent="handleLongPress(file)"`
3. `handleLongPress` 函数：根据文件类型动态生成 action sheet 按钮

**菜单选项**：

| 文件类型 | 选项 |
|---------|------|
| 文件夹 | 打开、加密 |
| 普通文件（视频/音频/图片/文档/其他） | 预览/播放、加密、删除 |
| 加密文件（.encv） | 播放、解密、删除 |

4. 各选项的处理：
   - **打开**：导航进入文件夹
   - **预览/播放**：路由到 Player 或 Preview
   - **加密**：调用 `createTask('encrypt', file.path)`
   - **解密**：调用 `createTask('decrypt', file.path)`
   - **删除**：确认弹窗后调用 `deleteFile(file.path)`，然后刷新列表
5. 删除操作需要确认：使用 `alertController` 弹出确认对话框

### useI18n.ts 新增 key

- `files.selectFile`: '选择文件' / 'Select File'
- `files.cancelSelect`: '取消' / 'Cancel'
- `files.tapToSelect`: '点击文件以选择' / 'Tap a file to select'
- `files.open`: '打开' / 'Open'
- `files.encrypt': '加密' / 'Encrypt'
- `files.decrypt`: '解密' / 'Decrypt'
- `files.delete`: '删除' / 'Delete'
- `files.preview': '预览' / 'Preview'
- `files.play`: '播放' / 'Play'
- `files.deleteConfirm`: '确定删除 "{name}" 吗？' / 'Delete "{name}"?'
- `files.deleteFailed`: '删除失败' / 'Delete failed'
- `files.encrypted': '加密文件' / 'Encrypted file'

---

## 步骤 4：修改 Tasks.vue — 添加浏览按钮

1. 引入 `useFilePicker`，解构 `startPicking`
2. 在 `newTaskModal` 中的路径输入框旁添加 "浏览" 按钮（`ion-button` with `folderOpen` icon）
3. 点击浏览按钮：
   ```ts
   async function handleBrowse() {
     showNewTaskModal.value = false  // 先关闭 modal
     const result = await startPicking()
     if (result) {
       newTaskPath.value = result.path
       showNewTaskModal.value = true  // 重新打开 modal 回填路径
     } else {
       showNewTaskModal.value = true  // 取消也重新打开
     }
   }
   ```
4. 路径输入框改为 `ion-input` + 右侧按钮的布局

### useI18n.ts 新增 key

- `tasks.browse`: '浏览' / 'Browse'

---

## 步骤 5：构建验证

运行 `cd app/encv-mobile && npm run build` 确认无 TypeScript 错误。

---

## 文件变更清单

| 文件 | 操作 | 变更内容 |
|------|------|---------|
| `src/views/Player.vue` | 修改 | 添加 `playerError` 状态 + 错误 UI + 修复容器高度 |
| `src/composables/useFilePicker.ts` | 新建 | 文件选择状态管理 composable |
| `src/views/Files.vue` | 修改 | picker 模式 + 长按 action sheet |
| `src/views/Tasks.vue` | 修改 | 路径输入框添加"浏览"按钮 |
| `src/composables/useI18n.ts` | 修改 | 添加所有新 i18n 文案 |
