# 三项修复计划

## 一、文件长按菜单增加"查看信息"

### 当前状态
[Files.vue:458-547](file:///workspace/app/encv-mobile/src/views/Files.vue#L458-L547) 长按菜单缺少"查看信息"入口。

### 方案

#### 1. 后端新增 `/api/file/info` 接口

**mobile_api.go** 新增 handler，**mobile_service.go** 新增方法：

普通文件返回：
```json
{
  "name": "test.txt", "path": "/files/test.txt", "size": 1234,
  "modified": "2025-01-01T00:00:00Z", "mime_type": "text/plain",
  "category": "document", "is_directory": false, "is_encrypted": false,
  "is_encv_container": false
}
```

ENCV 容器额外返回（调用 `reader.OpenV4Container` 只读 header + manifest，不读内容）：
```json
{
  ...普通字段, "is_encv_container": true,
  "container": {
    "version": 4, "container_id": "uuid", "container_type": "video",
    "is_seekable": true, "original_duration": 120.5, "segment_count": 3,
    "segments": [...], "manifest_size": 1024
  }
}
```

- 复用 `detector` 检测容器 + `reader.OpenV4Container` 获取元数据
- 密码从全局配置获取
- 前端 API 新增 `fetchFileInfo(path)` → `FileInfoResponse`

#### 2. 前端新建 FileInfo.vue 页面

路由：`/tabs/file-info?path=xxx&name=xxx`

页面结构：
```
┌─────────────────────────────┐
│ ←  文件信息                  │
├─────────────────────────────┤
│ 📄 基本信息                  │
│   名称 / 路径 / 大小 / 时间   │
│   MIME 类型 / 分类           │
├─────────────────────────────┤
│ 🔒 ENCV 容器（仅 .encv）     │
│   版本 / ID / 类型 / 可寻址  │
│   时长 / 分段数              │
├─────────────────────────────┤
│ 📋 清单 JSON（可折叠）        │
└─────────────────────────────┘
```

#### 3. Files.vue 长按菜单 + 路由注册 + i18n

所有分类的 buttons 数组前插入"信息"按钮；router/index.ts 注册路由；useI18n.ts 新增 key。

---

## 二、ff_graph_css_data 符号缺失根因

### 分析

构建脚本 [build-ffmpeg-android.sh:273,305](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L273) **已有** `-Wl,--undefined=ff_graph_css_data`。

**`-Wl,--undefined=SYMBOL` 的语义是："链接后 SYMBOL 仍未定义则报错"，它不强制链接器从库中提取符号。**

两种可能：

**可能 A（最大概率）：CI 缓存了旧 .so**
- 构建脚本第 34-46 行有缓存检测：如果 `libffmpeg.so` 存在且含 `ffmpeg_run` 就跳过
- **修改了链接参数但缓存命中→旧 .so 没有符号保护**
- 用户设备上的 .so 是加 `--undefined` 之前构建的

**可能 B：符号确实不在 libavfilter.a 中**
- `ff_graph_css_data` 可能被 FFmpeg 8.0 的 configure 条件编译排除了
- 需要 grep 确认符号定义位置和它所在 .c 是否被编入

### 执行步骤

1. 在 FFMPEG_SRC 目录搜索 `ff_graph_css_data` 定义位置
2. 确认该 .c 是否被编入 libavfilter.a（检查 ffmpeg_install/lib/libavfilter.a 的符号表）
3. **更新缓存检测**：把 [build-ffmpeg-android.sh:36-37](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L36-L37) 的缓存校验从只查 `ffmpeg_run` 改为同时查 `ff_graph_css_data`：
   ```bash
   if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
      ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ff_graph_css_data"; then
   ```
   这样一旦链接参数变更，缓存自动失效触发重建

---

## 三、文件预览机制修复（核心改动）

### 根因

[FilePreview.vue:98-106](file:///workspace/app/encv-mobile/src/views/FilePreview.vue#L98-L106) 的 `determinePreviewType()` 有缺陷：

```typescript
// 当前逻辑（有 bug）
if (category === 'image') return 'image'
if (ext === 'pdf') return 'pdf'
if (category === 'other') return 'text'    // ❌ 只有 other → text
return 'unsupported'                       // ❌ document(.txt) → unsupported!
```

`.txt` 被 `getFileCategory()` 归为 `'document'`（因为 `docExts = ['pdf','doc','docx','txt',...]`），而 `determinePreviewType` 不处理 `'document'` → 返回 `'unsupported'`。

### 方案：复用后端 ContentTypes 映射 + 设置可配置

#### 第一步：后端暴露可预览扩展名列表

新增接口 `GET /api/file/text-preview-exts`，返回当前配置的文本类型扩展名：

```json
{
  "extensions": ["txt","md","csv","log","ini","toml","yaml","yml","json",
    "xml","html","htm","css","js","ts","py","java","c","cpp","h","hpp",
    "go","rs","sh","bat","sql","conf","env","gitignore","vue","jsx",
    "tsx","php","properties","vtt","srt","ass","lrc","strm"],
  "custom_extensions": []  // 用户自定义追加的
}
```

数据源直接来自已有的 [config.ContentTypes](file:///workspace/internal/config/config.go#L211-L244)（其中 MIME 含 `text/` 的条目）。

同时支持用户在配置中自定义追加扩展名：
- 配置字段：`preview.text_extensions: string[]`（可选，默认空 = 仅用内置列表）
- 如果用户配置了此字段，与内置列表合并去重

#### 第二步：前端从 API 动态获取预览类型判断

**enc.ts** 新增：
```typescript
export interface TextPreviewExts {
  extensions: string[]
  custom_extensions: string[]
}

let cachedTextExts: Set<string> | null = null

export async function fetchTextPreviewExts(): Promise<Set<string>> {
  if (cachedTextExts) return cachedTextExts
  const data = await fetchJson<TextPreviewExts>(`${baseUrl}/api/file/text-preview-exts`)
  const all = new Set([...data.extensions, ...data.custom_extensions])
  cachedTextExts = all
  return all
}

export function isTextPreviewable(name: string): boolean {
  if (!cachedTextExts) return false
  const ext = getFileExtension(name)
  return cachedTextExts.has(ext)
}
```

**FilePreview.vue** 重写 `determinePreviewType`：
```typescript
type PreviewType = 'image' | 'pdf' | 'text' | 'container' | 'unsupported'

async function determinePreviewType(name: string, isEncrypted?: boolean): Promise<PreviewType> {
  const category = getFileCategory(name, isEncrypted)
  const ext = getFileExtension(name)

  if (category === 'image') return 'image'
  if (ext === 'pdf') return 'pdf'
  if (category === 'encrypted' || ext === 'encv') return 'container'

  const textExts = await fetchTextPreviewExts()
  if (textExts.has(ext)) return 'text'
  return 'unsupported'
}
```

#### 第三步：Settings.vue 增加预览扩展名配置

在设置页面的合适位置（文件相关设置区），增加：
```
📝 文本预览扩展名
  txt, md, csv, log, ini ...（显示当前列表）
  [+ 添加自定义扩展名] 按钮
  已添加: py, go, rs        （用户自定义的）
```

- 使用 `updateConfig({ preview: { text_extensions: [...] } })` 保存
- 保存后刷新 `cachedTextExts` 缓存

#### 第四步：FilePreview.vue 增加 container 类型

新增 `previewType === 'container'` 渲染分支：
- 调用 `fetchFileInfo(path)` 获取容器元数据
- 显示容器基本信息卡片（版本、ID、类型、分段数等）
- 可折叠区域显示完整 manifest JSON

#### 第五步：后端 ReadFileContent 加固

[mobile_service.go:178+](file:///workspace/internal/service/mobile_service.go#L178)：
- `os.ReadFile` 前，先检查是否为 ENCV 容器（扩展名 `.encv` 或 magic bytes）
- 如果是容器，返回错误 `"is_encv_container: use /api/file/info for metadata"` 而非二进制乱码

---

## 执行顺序

1. **问题 3（预览机制）**——前端改动集中，影响面大
2. **问题 1（文件信息页）**——依赖后端新接口
3. **问题 2（FFmpeg 符号）**——需要确认是缓存还是编译缺失
