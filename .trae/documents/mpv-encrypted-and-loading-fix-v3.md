# 修复：加密视频回滚 ArtPlayer + MPV 加载状态转圈圈（v5）

## 设计原则

1. **加密是独立状态，不是文件类型**：`getFileCategory()` 只返回内容类型，不关心加密状态
2. **加密容器的文件名不是原始文件名**：加密后文件名是容器格式（如 `hash.encv`），不含原始扩展名（如 `.mp4`），前端无法从文件名判断内容类型
3. **加密文件必须走预览页**：只有后端 `/api/file/info` → `container.container_type` 才知道加密文件的实际内容类型
4. **预览页是加密文件的权威路由器**：基于插件运行时声明（`container.container_type`），不是前端猜测
5. **传给播放器的路径是容器路径**：后端 `/stream` 端点根据容器路径解密并流式传输，`mimeType` 由 `container.container_type` 决定而非文件名

## 数据流验证

### 加密文件完整链路

```
后端文件列表 → FileItem { path: "/123云盘/hash.encv", isEncrypted: true }
  → handleFileClick: isEncrypted → 预览页
    → FilePreview.vue: /api/file/info → container.container_type = "video"
      → openPlayer(path, fileName, "video/*")
        → GoProcessPlugin: filePath="/123云盘/hash.encv", mimeType="video/*"
          → PlayerEntry: 传递 filePath + mimeType 给 MpvPlayerActivity
            → MpvPlayerActivity: isAudioFile("video/*", "hash.encv") = false ✓
              → resolveStreamUrl("/123云盘/hash.encv") → http://127.0.0.1:2025/stream?path=...
                → 后端 /stream: 解密并流式传输 ✓
```

### 关键验证点

1. **`isAudioFile(mimeType, fileName)`**：优先检查 `mimeType.startsWith("audio/")`，文件名仅作 fallback。加密容器文件名虽不含原始扩展名，但 `mimeType` 由 `container.container_type` 正确设置 ✓
2. **`resolveStreamUrl(filePath)`**：只构造 HTTP 流 URL，不依赖文件名扩展名。后端 `/stream` 端点根据路径找到容器文件并解密 ✓
3. **`MpvPlayerScreen` 播放**：使用 `streamUrl`（HTTP 流），MPV 原生支持 HTTP 流播放，不关心文件名 ✓

### 非加密文件链路（不变）

```
后端文件列表 → FileItem { path: "/123云盘/video.mp4", isEncrypted: false }
  → handleFileClick: getFileCategory("video.mp4") = "video" → playMedia()
    → openPlayer(path, name, "video/*", mode)
      → MPV 或 ArtPlayer
```

---

## 实施步骤

### Step 1：重构 getFileCategory — 移除加密参数

文件：`src/api/encv.ts`

- 移除 `isEncrypted` 参数
- 移除 `'encrypted'`、`'encrypted-video'`、`'encrypted-audio'`、`'encrypted-image'` 类型
- 函数只根据文件名返回内容类型（加密容器文件名不含原始扩展名，返回 `'other'` 是正确的）

```typescript
export type FileCategory = 'video' | 'audio' | 'image' | 'document' | 'other'

export function getFileCategory(name: string): FileCategory {
  const ext = getFileExtension(name)
  const videoExts = ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v']
  const audioExts = ['mp3', 'flac', 'wav', 'aac', 'ogg', 'wma', 'm4a']
  const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg']
  const docExts = ['pdf', 'doc', 'docx', 'txt', 'xls', 'xlsx', 'ppt', 'pptx']

  if (videoExts.includes(ext)) return 'video'
  if (audioExts.includes(ext)) return 'audio'
  if (imageExts.includes(ext)) return 'image'
  if (docExts.includes(ext)) return 'document'
  return 'other'
}
```

### Step 2：handleFileClick — 加密文件统一走预览页

文件：`src/views/Files.vue`

```typescript
async function handleFileClick(file: FileItem) {
  if (file.isDirectory) {
    const newPath = currentPath.value === '/'
      ? '/' + file.name
      : currentPath.value + '/' + file.name
    navigateTo(newPath)
    return
  }

  if (file.isEncrypted) {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'true' },
    })
    return
  }

  const category = getFileCategory(file.name)
  console.info('[Files] Click:', file.name, 'category:', category)
  if (category === 'video' || category === 'audio') {
    playMedia(file, category)
  } else {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'false' },
    })
  }
}
```

关键变化：
- 加密文件**优先判断**，直接路由到预览页，不尝试 `getFileCategory()`
- 非加密文件逻辑不变

### Step 3：handleLongPress — 加密文件统一走预览页

文件：`src/views/Files.vue`

将 `else if (category.startsWith('encrypted'))` 改为 `else if (file.isEncrypted)`，加密文件统一显示"预览"按钮：

```typescript
} else if (file.isEncrypted) {
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
  buttons.push({
    text: t('files.decrypt'),
    icon: lockClosed,
    handler: () => { handleDecryptFile(file) },
  })
  buttons.push({
    text: t('files.delete'),
    icon: trash,
    role: 'destructive',
    handler: () => { handleDeleteFile(file) },
  })
} else {
```

关键变化：
- 加密文件统一显示"预览"按钮（不是"播放"），因为前端无法判断内容类型
- 预览页查询后端 `container.container_type` 后再决定播放还是预览
- 保留"解密"和"删除"按钮

### Step 4：加密徽章判断简化

文件：`src/views/Files.vue` 第 250 行和第 317 行

```html
<ion-badge v-if="file.isEncrypted" color="warning" slot="end">
```

`file.isEncrypted` 是独立 boolean，不需要 `getFileCategory` 辅助判断。

### Step 5：useFileList.ts — 图标和颜色适配

文件：`src/composables/useFileList.ts`

```typescript
export function getFileIcon(file: FileItem) {
  if (file.isDirectory) return folder
  if (file.isEncrypted) return lockClosed
  const category = getFileCategory(file.name)
  switch (category) {
    case 'video': return videocam
    case 'audio': return musicalNotes
    case 'image': return image
    case 'document': return documentIcon
    default: return documentText
  }
}

export function getFileIconColor(file: FileItem): string {
  if (file.isDirectory) return 'primary'
  if (file.isEncrypted) return 'warning'
  const category = getFileCategory(file.name)
  switch (category) {
    case 'video': return 'danger'
    case 'audio': return 'tertiary'
    case 'image': return 'success'
    default: return 'medium'
  }
}
```

### Step 6：FilePickerModal.vue — 同步适配

文件：`src/components/FilePickerModal.vue`

同 Step 5 逻辑：加密文件优先显示锁图标，`getFileCategory` 不传 `isEncrypted`。

### Step 7：FilePreview.vue — 已正确（无需修改）

`FilePreview.vue` 已经基于后端 `container.container_type` 路由：
- `video`/`audio` → `openPlayer(path, fileName, mimeType)` → MPV
  - `path` 是容器路径，后端 `/stream` 端点根据此路径解密并流式传输
  - `mimeType` 由 `container.container_type` 决定（`"video/*"` 或 `"audio/*"`），不依赖文件名
- `image` → 显示图片
- `document`/`text` → 显示文本
- 其他 → 显示容器信息

### Step 8：MpvPlayerActivity Surface 直接挂载

文件：`plugin-mpv-player/.../MpvPlayerActivity.kt`

删除 `stateListener` 回调方式挂载 Surface，改为 `setContent` 后直接调用 `attachSurfaceView(contentRoot)`。

### Step 9：MpvPlayerScreen 订阅 engine.stateListener

文件：`plugin-mpv-player/.../MpvPlayerScreen.kt`

添加 `DisposableEffect(engine)` 订阅 `engine.stateListener`，映射到 Compose `PlayerState`。删除 `startPlayback` 中重复的 `onStateChange(PlayerState.Loading)` 调用。

### Step 10：MpvAudioPlayerScreen 订阅 engine.stateListener

文件：`plugin-mpv-player/.../MpvAudioPlayerScreen.kt`

同步添加 `DisposableEffect(engine)` 订阅 `engine.stateListener`。

### Step 11：验证构建

```bash
cd /workspace/app/encv-mobile/android && ./gradlew :plugin-mpv-player:compileDebugKotlin
```
