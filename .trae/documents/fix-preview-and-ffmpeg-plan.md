# 三个问题修复计划（完整版）

## 问题概览

| # | 问题 | 根因 |
|---|------|------|
| 1 | 预览加密容器显示元数据而非内容 | `FilePreview.vue` 对所有加密容器显示元数据卡片，应根据 `container_type` 调用对应预览方式 |
| 2 | 容器信息/清单未正确解析 | 需验证 API 调用和数据传递 |
| 3 | ffmpeg/ffprobe 失败无变化 | 旧缓存 .so 仍含 `ff_graph_css_data` 符号，需强制重建 |

---

## 问题 1：加密容器预览应根据类型调用对应插件

### 容器类型系统

后端 `GetFileInfo` 返回 `container_type`：
- `"video"` → 视频容器
- `"audio"` → 音频容器
- `"image"` → 图片容器
- `"document"` → 文档容器

### 流式传输端点

`/stream?path=xxx` 端点会：
1. 检测文件是否是 ENCV 容器
2. 如果是容器，解密并流式传输内容
3. 如果不是容器，直接提供原始文件

### 当前代码问题

1. **FilePreview.vue**：`determinePreviewType()` 对 `category === 'encrypted'` 返回 `'container'`，显示元数据卡片
2. **Files.vue**：`getFileCategory()` 对加密文件返回 `'encrypted'`，无法区分 video/audio/image/document
3. **Files.vue**：长按菜单判断 `isMedia = category === 'video' || category === 'audio'`，加密文件不满足，显示"预览"而非"播放"
4. **Files.vue**：`playMedia()` 有 `isVideo = category === 'video' || category === 'encrypted'`，这是错误的

### 修复方案

#### 方案：加密文件统一跳转预览页面，由预览页面根据 `container_type` 决定

**Files.vue 修改**：
1. `handleFileClick()` 对加密文件跳转预览页面（已有）
2. 长按菜单对加密文件显示"预览"而非"播放"（已有）
3. 删除 `playMedia()` 中错误的 `|| category === 'encrypted'` 判断

**FilePreview.vue 修改**：
在 `loadFile()` 中，对加密文件先调用 `/api/file/info` 获取 `container_type`，然后根据类型设置 `previewType`：
- `image` → `<img src="/stream?path=xxx">` 显示图片
- `video`/`audio` → 跳转到 `/player` 播放器
- `document` → PDF 用 iframe，文本用文本预览

---

## 修复步骤

### Step 1：修复 Files.vue

1. 删除 `playMedia()` 中错误的 `|| category === 'encrypted'` 判断
2. 长按菜单对加密文件显示"预览"（已有，无需修改）

```typescript
// 修改前
function playMedia(file: FileItem, category: string) {
  const isVideo = category === 'video' || category === 'encrypted'  // 错误
  ...
}

// 修改后
function playMedia(file: FileItem, category: string) {
  const isVideo = category === 'video'
  ...
}
```

### Step 2：修复 FilePreview.vue

修改 `loadFile()` 函数，对加密文件根据 `container_type` 设置预览类型：

```typescript
async function loadFile() {
  const path = (route.query.path as string) || ''
  const name = (route.query.name as string) || ''
  if (!path) {
    error.value = t('filePreview.noPath')
    loading.value = false
    return
  }
  filePath.value = path
  fileName.value = name || path.split('/').pop() || path
  loading.value = true
  error.value = ''
  copied.value = false
  showManifest.value = false
  containerInfo.value = null

  const isEncrypted = route.query.isEncrypted === 'true'

  if (isEncrypted) {
    // 加密文件需要先获取 container_type 来决定预览方式
    try {
      const baseUrl = getApiBaseUrl()
      const resp = await fetch(`${baseUrl}/api/file/info?path=${encodeURIComponent(path)}`)
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      const info = await resp.json()

      fileSize.value = info.size || 0

      if (info.is_encv_container && info.container) {
        const containerType = info.container.container_type
        containerInfo.value = info.container
        manifestJson.value = JSON.stringify(info.container.manifest || info.container, null, 2)

        // 根据 container_type 设置预览类型
        switch (containerType) {
          case 'image':
            previewType.value = 'image'
            streamUrl.value = getFileStreamUrl(path)
            break
          case 'video':
          case 'audio':
            // 跳转到播放器
            router.push({ path: '/player', query: { path, name: fileName.value } })
            loading.value = false
            return
          case 'document':
            // 判断是 PDF 还是文本
            const ext = getFileExtension(fileName.value)
            if (ext === 'pdf') {
              previewType.value = 'pdf'
              streamUrl.value = getFileStreamUrl(path)
            } else {
              previewType.value = 'text'
              const data: FileContentResponse = await readFileContent(path)
              content.value = data.content
              fileSize.value = data.size
              encoding.value = data.encoding
            }
            break
          default:
            previewType.value = 'container' // 显示元数据
        }
      } else {
        previewType.value = 'unsupported'
      }
    } catch (e: any) {
      console.error('Failed to load encrypted file:', e)
      error.value = e?.message || String(e)
    } finally {
      loading.value = false
    }
    return
  }

  // 非加密文件的现有逻辑
  previewType.value = await determinePreviewType(fileName.value, isEncrypted)

  try {
    if (previewType.value === 'image' || previewType.value === 'pdf') {
      console.info('Loading stream preview:', fileName.value)
      streamUrl.value = getFileStreamUrl(path)
    } else if (previewType.value === 'text') {
      console.info('Loading text preview:', fileName.value)
      const data: FileContentResponse = await readFileContent(path)
      content.value = data.content
      fileSize.value = data.size
      encoding.value = data.encoding
    } else {
      console.info('Unsupported file type:', fileName.value)
      fileSize.value = 0
    }
  } catch (e: any) {
    console.error('Failed to load file:', e)
    error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}
```

### Step 3：修复 FFmpeg 构建脚本

在缓存检查前添加 `ff_graph_css_data` 符号检测，强制重建：

```bash
echo "=== Checking for cached ffmpeg output ==="
if [ -f "${OUTPUT_DIR}/libffmpeg.so" ] && [ -f "${OUTPUT_DIR}/libffprobe.so" ]; then
    # 检查是否包含已弃用的 ff_graph_css_data 符号
    if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" 2>/dev/null | grep -q "ff_graph_css_data"; then
        echo "⚠️  Cached libraries contain deprecated ff_graph_css_data symbol, forcing rebuild..."
        rm -f "${OUTPUT_DIR}/libffmpeg.so" "${OUTPUT_DIR}/libffprobe.so"
    elif ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
         ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_reset" && \
         ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run" && \
         ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_reset"; then
        echo "✅ All ffmpeg libraries cached and valid, skipping build"
        echo "Output: $OUTPUT_DIR"
        ls -lh "$OUTPUT_DIR"
        exit 0
    else
        echo "⚠️  Cached libraries missing expected symbols, rebuilding..."
        rm -f "${OUTPUT_DIR}/libffmpeg.so" "${OUTPUT_DIR}/libffprobe.so"
    fi
fi
```

---

## 文件修改清单

| 文件 | 修改内容 |
|------|----------|
| `src/views/Files.vue` | 删除 `playMedia()` 中错误的 `\|\| category === 'encrypted'` |
| `src/views/FilePreview.vue` | 根据 `container_type` 调用对应预览方式 |
| `scripts/build-ffmpeg-android.sh` | 添加 `ff_graph_css_data` 符号检测强制重建 |
