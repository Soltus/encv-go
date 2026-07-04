# CI 错误修复计划：三个关联问题

## 问题概览

| # | 问题 | 根因 | 影响 |
|---|------|------|------|
| 1 | 容器预览失败：`(intermediate value).getApiBaseUrl is not a function` | 动态 import 命名导出的模块在 Vite 生产构建中行为异常 | 加密容器预览页完全不可用 |
| 2 | FFmpeg dlopen 失败：`cannot locate symbol "ff_graph_css_data"` | 构建脚本使用 `-Wl,--undefined=ff_graph_css_data` 强制引入了一个不存在的未解析符号，dlopen(RTLD_NOW) 在加载时无法解析 | **libffmpeg.so 和 libffprobe.so 均无法加载，所有视频/音频功能瘫痪（包括 #3）** |
| 3 | ffprobe 加密报错：`ffprobe failed (exit 1)` | 直接由 #2 导致 — libffprobe.so 无法 dlopen，所有 ffprobe 调用失败；引擎详情页显示"可用"是因为 `CheckFFmpegAvailable` 只检查 `ffprobe_run` 符号，不检查 `ff_graph_css_data` | 所有涉及 ffprobe 的操作（元数据提取、加密、格式检测）全部失败 |

---

## 修复步骤

### Step 1：修复 FilePreview.vue 的 getApiBaseUrl 动态导入问题

**文件**: [src/views/FilePreview.vue](src/views/FilePreview.vue)

**问题分析**（第 208-209 行）：
```typescript
const baseUrl = import.meta.env.DEV ? '' : (await import('@/api/encv')).getApiBaseUrl()
```

[encv.ts](src/api/encv.ts) 使用**纯命名导出**（无 default export）。该文件顶部第 123 行已经静态导入了同一模块：
```typescript
import { readFileContent, getFileStreamUrl, getFileCategory, getFileExtension, formatFileSize, fetchTextPreviewExts } from '@/api/encv'
```

Vite 生产构建（Rollup）对动态 import 的命名导出模块处理可能产生 `(intermediate value)` 包装，导致 `.getApiBaseUrl()` 调用失败。

**修复方案**：将 `getApiBaseUrl` 加入已有的静态导入列表，删除动态 import：

1. 第 123 行的 import 语句中加入 `getApiBaseUrl`：
   ```typescript
   import { readFileContent, getFileStreamUrl, getFileCategory, getFileExtension, formatFileSize, fetchTextPreviewExts, getApiBaseUrl } from '@/api/encv'
   ```

2. 第 209 行简化为：
   ```typescript
   const baseUrl = getApiBaseUrl()
   ```
   （DEV 模式下 `getApiBaseUrl()` 本身就返回空字符串，无需三元判断）

### Step 2：修复 FFmpeg 构建脚本的 ff_graph_css_data 符号问题

**文件**: [scripts/build-ffmpeg-android.sh](scripts/build-ffmpeg-android.sh)

**根因分析**：

链接命令使用了 `-Wl,--undefined=ff_graph_css_data`（第 275 行、第 307 行）。此标志的作用是**将指定符号添加到输出 .so 的动态符号表中作为 UNDEFINED（未定义）符号**。

关键点：
- `-Wl,--undefined=SYMBOL` ≠ "确保 SYMBOL 不被裁剪"，而是 "强制 SYMBOL 成为外部依赖"
- 如果 SYMBOL 在任何链接输入（.o / .a）中都不存在，链接器不会报错（共享库允许未解析符号），但会在动态符号表中留下一个 UNDEFINED 条目
- 运行时 `dlopen(..., RTLD_NOW)` 会立即解析所有 UNDEFINED 符号
- 由于 `ff_graph_css_data` 未被实际定义/编译进 .so，动态链接器找不到它 → **dlopen 失败**

`ff_graph_css_data` 是 FFmpeg 8.0 fftools/graph 子系统中的 CSS 数据常量（用于 graph 可视化），它定义在 `fftools/graph/` 目录的 .c 文件中。虽然这些文件被编译进了 libffmpeg.so 的 FFMPEG_FFTOOLS 列表，但该符号可能：
- 被 `-Wl,--gc-sections` 死代码消除裁剪掉（因为没有代码引用它）
- 或其定义本身是 static/内部链接的，不会成为全局符号

**修复方案**（共 6 处修改）：

#### 2a. 移除链接命令中的 `--undefined=ff_graph_css_data`

- **第 274-276 行**（libffmpeg.so 链接）：删除 `-Wl,--undefined=ff_graph_css_data \`
- **第 306-308 行**（libffprobe.so 链接）：删除 `-Wl,--undefined=ff_graph_css_data \`

#### 2b. 更新缓存检查逻辑（第 36-39 行）

当前缓存检查要求 `ff_graph_css_data` 符号存在才视为有效缓存。移除后改为只检查核心运行符号：

```bash
# 修改前
if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
   ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ff_graph_css_data" && \
   ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run" && \
   ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ff_graph_css_data"; then

# 修改后
if ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_run" && \
   ${NM} -D "${OUTPUT_DIR}/libffmpeg.so" | grep -q "ffmpeg_reset" && \
   ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_run" && \
   ${NM} -D "${OUTPUT_DIR}/libffprobe.so" | grep -q "ffprobe_reset"; then
```

#### 2c. 更新构建后验证命令（第 328 行）

```bash
# 修改前
${NM} -D "${OUTPUT_DIR}/${lib}" | grep -E "ffmpeg_run|ffprobe_run|ffmpeg_reset|ffprobe_reset|ff_graph_css_data"

# 修改后
${NM} -D "${OUTPUT_DIR}/${lib}" | grep -E "ffmpeg_run|ffprobe_run|ffmpeg_reset|ffprobe_reset"
```

### Step 3：验证 #2 修复后 #3 是否自动解决

Issue #3（ffprobe exit 1）的直接根因是 Issue #2 — `libffprobe.so` 因 `ff_graph_css_data` 未解析符号无法 dlopen。完成 Step 2 后：

1. `callFFprobeNative()` 能成功 dlopen `libffprobe.so`
2. `ExtractMetadata()` 能正常调用 ffprobe 提取元数据
3. 加密流程不再因 metadata extraction 失败而中断

**额外验证项**：如果修复 #2 后仍有特定文件的 ffprobe exit 1 错误，需要检查：
- 文件路径中的中文字符（`123云盘`）是否正确编码传递给 native 层
- ffprobe 的 stderr 输出是否有更具体的错误信息
- 但这属于后续排查范围，不在本次修复范围内

---

## 执行顺序

1. **Step 1** → 独立修复，无依赖
2. **Step 2a + 2b + 2c** → 独立修复，无依赖（但需重新构建 FFmpeg）
3. **Step 3** → 仅验证，依赖 Step 2 完成

## 验证方式

- Step 1: `vue-tsc --noEmit && vite build` 通过
- Step 2: 重新执行 `build-ffmpeg-android.sh`，确认：
  - 链接无错误
  - `nm -D libffmpeg.so | grep ffmpeg_run` 有输出
  - `nm -D libffmpeg.so | grep ff_graph_css_data` **无输出**（确认已移除）
  - 缓存检查通过
- Step 3: 在设备上测试容器预览页面 + 视频加密流程
