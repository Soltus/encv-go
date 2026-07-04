# 实施计划：预览扩展名配置 + FFmpeg 缓存检测修复 + 构建验证

## 概述

基于用户最新需求：
1. **预览类型后缀名参考 OpenList 方案，支持在设置界面配置** — 新需求（Settings 硬编码自定义分区）
2. **FFmpeg `ff_graph_css_data` 符号缺失根因修复** — 更新构建脚本缓存检测
3. **构建验证** — vue-tsc + vite build + go vet

---

## 任务 1：Settings.vue 增加预览扩展名配置 UI

### 背景
- 后端已有 `config.PreviewConfig{TextExtensions []string}` 和 `GetTextPreviewExtensions()`
- 后端已有 `/api/file/text-preview-exts` 接口返回内置+自定义扩展名
- 前端已有 `fetchTextPreviewExts()` / `invalidateTextExtsCache()` API
- `config.schema.json` 中**没有** `preview` 字段 → Settings.vue 的 schema 动态渲染不会显示它
- **方案**：参考外观/播放器分区的做法，在 Settings.vue 中添加**硬编码自定义分区**

### 实施步骤

#### 1.1 Settings.vue 新增"预览设置"硬编码分区
位置：在 `<ion-list>` 的 devtools 分区（`devtools.title`）**之前**，插入新的预览设置列表：

```html
<!-- 预览设置 -->
<ion-list>
  <ion-list-header>
    <ion-label>{{ t('settings.preview') }}</ion-label>
  </ion-list-header>
  <ion-item>
    <ion-icon :icon="textOutline" slot="start"></ion-icon>
    <ion-input
      :value="customTextExts"
      :label="t('settings.customTextExts')"
      label-placement="stacked"
      :placeholder="t('settings.customTextExtsHint')"
      @ionInput="handleCustomTextExtsChange"
    ></ion-input>
  </ion-item>
  <ion-item v-if="builtInTextExtsCount > 0">
    <ion-label class="ion-text-wrap hint-text">
      <p>{{ t('settings.builtInTextExts', { count: String(builtInTextExtsCount) }) }}</p>
    </ion-label>
  </ion-item>
</ion-list>
```

#### 1.2 数据逻辑（script 部分）
```typescript
import { textOutline } from 'ionicons/icons'
import { fetchTextPreviewExts, invalidateTextExtsCache, updateConfig, fetchConfig } from '@/api/encv'

const customTextExts = ref('')
const builtInTextExtsCount = ref(0)

async function loadPreviewConfig() {
  try {
    const cfg = await fetchConfig()
    const preview = cfg.preview as Record<string, unknown> | undefined
    if (preview?.text_extensions && Array.isArray(preview.text_extensions)) {
      customTextExts.value = (preview.text_extensions as string[]).join(',')
    }
  } catch {}
  try {
    const exts = await fetchTextPreviewExts()
    builtInTextExtsCount.value = exts.size
  } catch {}
}

async function handleCustomTextExtsChange(event: CustomEvent) {
  const raw = (event.target as HTMLInputElement).value || ''
  customTextExts.value = raw
  const parsed = raw.split(',')
    .map(s => s.trim().toLowerCase())
    .filter(s => s.length > 0)
  try {
    const cfg = await fetchConfig()
    if (!cfg.preview) cfg.preview = {}
    ;(cfg.preview as Record<string, unknown>).text_extensions = parsed
    await updateConfig(cfg)
    invalidateTextExtsCache()
  } catch (e) {
    console.error('Failed to save preview config:', e)
  }
}
```

在 `onMounted` 中调用 `loadPreviewConfig()`。

#### 1.3 i18n 新增 key（useI18n.ts）
```
'zh-CN':
  settings.preview: '预览设置'
  settings.customTextExts: '自定义文本预览扩展名'
  settings.customTextExtsHint: '追加可预览的文件扩展名，逗号分隔（如 log,ini,cfg,toml）'
  settings.builtInTextExts: '内置支持 {count} 种文本格式'

'en':
  settings.preview: 'Preview'
  settings.customTextExts: 'Custom Text Preview Extensions'
  settings.customTextExtsHint: 'Additional extensions for text preview, comma-separated (e.g. log,ini,cfg,toml)'
  settings.builtInTextExts: '{count} built-in text formats supported'
```

#### 1.4 样式
```css
.hint-text p {
  font-size: 13px;
  color: var(--ion-text-secondary);
  margin: 0;
}
```

#### 1.5 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/src/views/Settings.vue` | 新增预览设置硬编码分区 + script 数据逻辑 |
| `app/encv-mobile/src/composables/useI18n.ts` | 新增 4 个 i18n key |

---

## 任务 2：FFmpeg `ff_graph_css_data` 缓存检测修复

### 根因分析
构建脚本 `build-ffmpeg-android.sh` 第 33-46 行的缓存检测只检查 `ffmpeg_run`/`ffprobe_run`：

```bash
if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
   ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run"; then
    echo "✅ All ffmpeg libraries cached and valid, skipping build"
    exit 0
fi
```

**问题**：
- CI 缓存的 `.so` 可能是旧版构建脚本生成的（没有 `-Wl,--undefined=ff_graph_css_data`）
- 或者链接时 `--gc-sections` 把 `ff_graph_css_data` 优化掉了（如果引用链不完整）
- 缓存命中但缺少符号 → 运行时 dlopen 失败 → `cannot locate symbol "ff_graph_css_data"`

### 实施步骤

#### 2.1 更新缓存校验（第 36-37 行）
```diff
- if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
-    ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run"; then
+ if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
+    ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ff_graph_css_data" && \
+    ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run" && \
+    ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ff_graph_css_data"; then
```

#### 2.2 更新最终符号验证（第 326 行）
```diff
- ${NM} -D "${OUTPUT_DIR}/${lib}" | grep -E "ffmpeg_run|ffprobe_run|ffmpeg_reset|ffprobe_reset"
+ ${NM} -D "${OUTPUT_DIR}/${lib}" | grep -E "ffmpeg_run|ffprobe_run|ffmpeg_reset|ffprobe_reset|ff_graph_css_data"
```

#### 2.3 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/scripts/build-ffmpeg-android.sh` | 2 处改动 |

---

## 任务 3：构建验证

按顺序执行：
1. `cd /workspace && go vet ./internal/...`
2. `cd /workspace/app/encv-mobile && npx vue-tsc --noEmit`
3. `cd /workspace/app/encv-mobile && npx vite build`

---

## 执行顺序

1. **任务 2**（FFmpeg 缓存检测）— 独立改动，2 行 diff
2. **任务 1**（Settings 预览配置 UI）— 主要功能开发
3. **任务 3**（构建验证）— 统一验证
