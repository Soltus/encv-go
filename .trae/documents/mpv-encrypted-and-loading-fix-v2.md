# 修复：加密视频回滚 ArtPlayer + MPV 加载状态转圈圈（v2）

## 设计原则

**用户核心指示**：预览逻辑是根据 encv 插件实际声明实时分发的，不是再创建维护一套分离的判断规则。

这意味着：
- `getFileCategory()` 中新增的 `encrypted-video`/`encrypted-audio`/`encrypted-image` 类型是**错误的**——它们是前端维护的分离判断规则
- 正确做法：加密文件统一路由到预览页，由预览页查询后端 `/api/file/info` 获取 `container.container_type`（插件声明），再决定播放还是预览
- `FilePreview.vue` 已经正确实现了这个逻辑：`containerType === 'video'|'audio'` → `openPlayer()`

## 问题分析

### 问题 1：加密视频回滚到 ArtPlayer

**根因**：`handleFileClick` 和 `handleLongPress` 中，加密文件用 `getFileCategory()` 前端猜测类型来决定路由，但这个猜测与插件实际声明不一致。

**正确修复**：
1. 回退 `getFileCategory()` 的 `encrypted-video`/`encrypted-audio`/`encrypted-image` 细分类型，加密文件统一返回 `'encrypted'`
2. `handleFileClick`：`category === 'encrypted'` → 路由到预览页（而非直接播放）
3. `handleLongPress`：`category === 'encrypted'` → 显示"预览"按钮（而非"播放"），路由到预览页
4. `FilePreview.vue` 已经正确实现：查询后端 `container.container_type` → `openPlayer()` 或预览

### 问题 2：加载状态一直转圈圈

根因不变：`stateListener` 时序 + Compose 不监听 engine 状态。修复方案不变。

---

## 实施步骤

### Step 1：回退 getFileCategory 的加密细分类型

文件：`src/api/encv.ts`

```typescript
export type FileCategory = 'video' | 'audio' | 'image' | 'document' | 'encrypted' | 'other'

export function getFileCategory(name: string, isEncrypted?: boolean): FileCategory {
  const ext = getFileExtension(name)
  const videoExts = ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v']
  const audioExts = ['mp3', 'flac', 'wav', 'aac', 'ogg', 'wma', 'm4a']
  const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg']
  const docExts = ['pdf', 'doc', 'docx', 'txt', 'xls', 'xlsx', 'ppt', 'pptx']

  if (isEncrypted) return 'encrypted'

  if (videoExts.includes(ext)) return 'video'
  if (audioExts.includes(ext)) return 'audio'
  if (imageExts.includes(ext)) return 'image'
  if (docExts.includes(ext)) return 'document'
  return 'other'
}
```

### Step 2：handleFileClick 加密文件统一路由到预览页

文件：`src/views/Files.vue`

```typescript
async function handleFileClick(file: FileItem) {
  if (file.isDirectory) { /* ... 不变 */ return }
  const category = getFileCategory(file.name, file.isEncrypted)
  if (category === 'video' || category === 'audio') {
    playMedia(file, category)
  } else {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: String(!!file.isEncrypted) },
    })
  }
}
```

加密文件（`category === 'encrypted'`）走 `else` 分支 → 预览页 → 预览页查询后端 `container.container_type` → `openPlayer()`。

### Step 3：handleLongPress 加密文件统一路由到预览页

文件：`src/views/Files.vue`

回退之前的 `startsWith('encrypted')` 修改，恢复为 `category === 'encrypted'`，加密文件显示"预览"按钮：

```typescript
} else if (category === 'encrypted') {
  buttons.push({
    text: t('files.preview'),
    icon: image,
    handler: () => {
      router.push({
        path: '/tabs/preview',
        query: { path: file.path, name: file.name, isEncrypted: 'true' },
      })
    },
  })
```

### Step 4：Files.vue 加密徽章判断修复

文件：`src/views/Files.vue`

第 317 行和第 250 行：加密徽章判断改为 `file.isEncrypted || category === 'encrypted'`（因为 `getFileCategory` 不再返回 `encrypted-*` 子类型）。

### Step 5：FilePreview.vue 已正确（无需修改）

`FilePreview.vue` 已经正确实现了基于插件声明的路由：
- 查询 `/api/file/info` → `container.container_type`
- `video`/`audio` → `openPlayer()`（走原生 MPV）
- `image` → 显示图片
- `document`/`text` → 显示文本

### Step 6：useFileList.ts 同步修复

文件：`src/composables/useFileList.ts`

`getFileIcon` 和 `getFileColor` 中 `case 'encrypted'` 保持不变（已经正确处理 `'encrypted'` 类型）。

### Step 7：MpvPlayerActivity Surface 直接挂载

文件：`plugin-mpv-player/.../MpvPlayerActivity.kt`

删除 `stateListener` 回调方式挂载 Surface，改为 `setContent` 后直接调用 `attachSurfaceView(contentRoot)`。

### Step 8：MpvPlayerScreen 订阅 engine.stateListener

文件：`plugin-mpv-player/.../MpvPlayerScreen.kt`

添加 `DisposableEffect(engine)` 订阅 `engine.stateListener`，映射到 Compose `PlayerState`。删除 `startPlayback` 中重复的 `onStateChange(PlayerState.Loading)` 调用。

### Step 9：MpvAudioPlayerScreen 订阅 engine.stateListener

文件：`plugin-mpv-player/.../MpvAudioPlayerScreen.kt`

同步添加 `DisposableEffect(engine)` 订阅 `engine.stateListener`。

### Step 10：验证构建

```bash
cd /workspace/app/encv-mobile/android && ./gradlew :plugin-mpv-player:compileDebugKotlin
```
