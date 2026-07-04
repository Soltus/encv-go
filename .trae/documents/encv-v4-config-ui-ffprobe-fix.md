# 实施计划：视频插件配置适配 + 移动端设置界面 + ffprobe dlopen 修复

## 任务 1：修复 ffprobe dlopen 失败（最紧急的 Bug）

### 根因分析

错误信息：`dlopen failed: cannot locate symbol "av_log" referenced by libffprobe.so`

**根因**：`libffprobe.so` 链接时使用了 `-lavcodec -lavformat -lavutil ...`（动态链接），运行时需要找到这些 `.so`。虽然 `ffmpeg_dlopen.go` 第 50-54 行已经用 `RTLD_NOW | RTLD_GLOBAL` 预加载了依赖库，但问题出在 **构建脚本** `build-ffmpeg-android.sh` 第 243-248 行：

```bash
$CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffprobe.so" \
    $FFPROBE_OBJS \
    -lavcodec -lavformat -lavutil -lswresample -lswscale -lavfilter -lavdevice \
    -lx264 -lm -llog \
    -Wl,-rpath,\$ORIGIN \
    $LDFLAGS > /dev/null 2>&1
```

`-Wl,-rpath,\$ORIGIN` 设置了 RPATH 为 `$ORIGIN`（即 .so 所在目录），但 Android 的 linker **忽略 RPATH**。Android 只在 `nativeLibraryDir` 中查找库。当 `dlopen("libffprobe.so")` 时，linker 尝试解析 `av_log`，虽然 `libavutil.so` 已经通过 `RTLD_GLOBAL` 加载，但 **Android linker 在 NDK API 24+ 上使用命名空间隔离**，`RTLD_GLOBAL` 加载的符号可能不在 `libffprobe.so` 的解析命名空间中。

### 修复方案

**方案 A（推荐）**：修改构建脚本，将 `libffprobe.so` 改为静态链接所有 FFmpeg 依赖。这样 `libffprobe.so` 自身包含所有需要的符号，不再依赖外部 `libavutil.so` 等。

修改 `build-ffmpeg-android.sh` 第 242-248 行：
```bash
# 改为静态链接 FFmpeg 库
$CC $CFLAGS -shared -o "${FTOOLS_BUILD}/libffprobe.so" \
    $FFPROBE_OBJS \
    -Wl,--whole-archive \
    ${FFMPEG_INSTALL}/lib/libavformat.a \
    ${FFMPEG_INSTALL}/lib/libavcodec.a \
    ${FFMPEG_INSTALL}/lib/libavutil.a \
    ${FFMPEG_INSTALL}/lib/libswresample.a \
    ${FFMPEG_INSTALL}/lib/libswscale.a \
    ${FFMPEG_INSTALL}/lib/libavfilter.a \
    ${FFMPEG_INSTALL}/lib/libavdevice.a \
    -Wl,--no-whole-archive \
    ${X264_INSTALL}/lib/libx264.a \
    -lm -llog \
    $LDFLAGS > /dev/null 2>&1
```

同样修改 `libffmpeg.so` 的链接方式。

**方案 B（备选）**：修改 `ffmpeg_dlopen.go`，在 dlopen 主库时也使用 `RTLD_GLOBAL`，并确保依赖库加载顺序正确。

### 实施步骤

1. 修改 `app/encv-mobile/scripts/build-ffmpeg-android.sh`：
   - 将 `libffprobe.so` 和 `libffmpeg.so` 从动态链接改为静态链接 FFmpeg 库
   - 使用 `--whole-archive` 确保所有符号都被包含
   - 移除不再需要的 `-lavcodec` 等动态链接参数
   - 不再需要将 `libavcodec.so` 等复制到输出目录（因为已静态链接）

2. 修改 `internal/utils/ffmpeg_dlopen.go`：
   - 移除 `g_dep_libs` 数组（不再需要预加载依赖）
   - 简化 `call_native_run` 函数，直接 dlopen 主库即可

---

## 任务 2：视频插件配置适配 v4 架构

### 现状

- Go 后端 `VideoPluginConfig` 已经在之前的 v4 实现中更新了字段名（`container_chunk_size_mb`、`light_container_main_chunk_enabled`、`allow_no_reencode`、`default_stream_preset`）
- 但前端 `app/encv-mobile/src/config/schema.json` 仍然使用旧字段名（`chunk_size_mb`、`light_main_chunk_enabled`）
- 前端 `fieldIconMap` 仍然映射旧字段名
- 前端 `schemaParser.ts` 的 `isPathField` 函数需要更新

### 实施步骤

1. **更新前端 schema.json**：将 `app/encv-mobile/src/config/schema.json` 中的 `VideoPluginConfig` 与后端 `config.schema.json` 同步
   - `chunk_size_mb` → `container_chunk_size_mb`
   - `light_main_chunk_enabled` → `light_container_main_chunk_enabled`
   - 新增 `allow_no_reencode`（boolean）
   - 新增 `default_stream_preset`（string，enum: balanced/quality/high_quality）

2. **更新 PluginSettings.vue**：更新 `fieldIconMap`，添加新字段的图标映射
   - 移除 `chunk_size_mb`、`light_main_chunk_enabled` 的映射
   - 新增 `container_chunk_size_mb`、`light_container_main_chunk_enabled`、`allow_no_reencode`、`default_stream_preset` 的映射

3. **更新 Settings.vue**：同上，更新 `fieldIconMap`

4. **为 `default_stream_preset` 添加 select 控件支持**：在 `PluginSettings.vue` 和 `Settings.vue` 中，对 `default_stream_preset` 字段使用 `ion-select` 而非 `ion-input`，提供三个选项

5. **更新 `schemaParser.ts`**：
   - 在 `FieldDef` 中增加 `isSelect` 和 `selectOptions` 字段
   - 在 `parseProperty` 中检测 enum 类型字段，设置 `isSelect: true` 和 `selectOptions`
   - 在 `isPathField` 中添加 `container_cache_dir`（如果需要）

---

## 任务 3：移动端设置界面可视化 — 区分配置来源与平台

### 现状

当前 `PluginSettings.vue` 将所有 `plugin_settings` 下的字段平铺展示，没有区分：
- 桌面端和移动端都有效的配置（如 `container_chunk_size_mb`）
- 仅桌面端有效的配置（如某些 NVENC 相关参数）
- 仅移动端有效的配置（如 `default_stream_preset`，因为移动端用 MediaCodec）
- **移动端应用自身独有的设置**（如视频打开方式、播放器选择），这些不是后端 JSON 配置的一部分

### 设计方案

#### 3.1 配置来源分类

将设置分为三大类，用**视觉徽章（Badge）**区分：

| 来源 | 徽章颜色 | 徽章文字 | 说明 |
|------|----------|----------|------|
| 后端 JSON 配置 | `primary`（蓝色） | "服务端" | 来自 Go 后端 config.json，桌面端和移动端共享 |
| 移动端独有配置 | `tertiary`（紫色） | "移动端" | 仅移动端应用本地存储的设置 |
| v4 新增 | `success`（绿色） | "v4" | v4 架构新增的配置项 |

#### 3.2 字段平台过滤

在 `schema.json` 的 `VideoPluginConfig` 中，为每个字段添加 `x-platform` 扩展属性：
- `"x-platform": "both"` — 桌面端和移动端都显示（默认）
- `"x-platform": "desktop"` — 仅桌面端显示
- `"x-platform": "mobile"` — 仅移动端显示

在 `schemaParser.ts` 中解析 `x-platform`，在 `FieldDef` 中增加 `platform` 字段。

在前端根据当前平台过滤字段显示。

#### 3.3 移动端独有设置区域

在 `PluginSettings.vue` 中新增一个独立的 ion-list 区域，专门展示移动端应用独有的设置（存储在 localStorage）：

- **视频打开方式**（已有，在 Settings.vue 中，但应移至 PluginSettings 中与视频配置一起）
- **画质预设选择器**（v4 新增，使用 ion-select + 卡片式选项展示）

这些设置使用**紫色徽章**标记，与蓝色徽章的后端配置形成视觉区分。

#### 3.4 视觉设计

每个配置项的展示格式：

```
┌─────────────────────────────────────────────┐
│ 🎬 container_chunk_size_mb        [服务端]  │
│    容器分片大小（MB）                        │
│    [0                                    ]   │
├─────────────────────────────────────────────┤
│ 🎬 default_stream_preset     [移动端] [v4]  │
│    画质预设                                  │
│    ┌──────────┐ ┌──────────┐ ┌──────────┐   │
│    │ 平衡(推荐)│ │  高质量  │ │ 极致画质 │   │
│    │  28 VBR  │ │  24 VBR  │ │  20 VBR  │   │
│    └──────────┘ └──────────┘ └──────────┘   │
├─────────────────────────────────────────────┤
│ 🎬 视频打开方式                   [移动端]  │
│    ┌─────────────────────────────────────┐  │
│    │ Artplayer (内置)        ▼           │  │
│    └─────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

徽章样式：
- **服务端**：蓝色小圆角徽章，白字
- **移动端**：紫色小圆角徽章，白字
- **v4**：绿色小圆角徽章，白字

### 字段平台分类

| 字段 | 平台 | v4 新增 | 原因 |
|------|------|---------|------|
| `ext` | both | 否 | 通用 |
| `container_chunk_size_mb` | both | 否(重命名) | 通用 |
| `light_container_main_chunk_enabled` | both | 否(重命名) | 通用 |
| `track_extensions` | both | 否 | 通用 |
| `keep_mkv_for_mkv_source` | both | 否 | 通用 |
| `verify_after_pack` | both | 否 | 通用 |
| `plugin_cache_dir` | both | 否 | 通用 |
| `skip_merge_for_split_mkv` | both | 否 | 通用 |
| `allow_no_reencode` | both | 是 | v4 新功能 |
| `default_stream_preset` | mobile | 是 | 移动端用 MediaCodec |

### 实施步骤

1. **更新 `schema.json`（前端和后端）**：
   - 为 `VideoPluginConfig` 的每个属性添加 `x-platform` 扩展属性
   - 为 v4 新增字段添加 `x-v4: true` 扩展属性
   - 为 `default_stream_preset` 添加 `enum` 和 `x-enum-labels` 扩展

2. **更新 `schemaParser.ts`**：
   - `FieldDef` 增加 `platform?: 'both' | 'desktop' | 'mobile'` 字段
   - `FieldDef` 增加 `isV4?: boolean` 字段
   - `FieldDef` 增加 `isSelect?: boolean` 和 `selectOptions?: { value: string; label: string; description: string }[]` 字段
   - `parseProperty` 解析 `x-platform`、`x-v4`、`enum`、`x-enum-labels` 属性

3. **更新 `PluginSettings.vue`**：
   - 导入平台检测工具（`isNative()` from `@/plugins/GoProcess`）
   - 过滤 `pluginSection.properties`，根据 `platform` 字段和当前平台决定是否显示
   - 为每个字段添加徽章组件（服务端/移动端/v4）
   - 新增移动端独有设置区域（视频打开方式等，从 Settings.vue 迁移）
   - 为 `default_stream_preset` 实现卡片式选择器（而非简单 select）

4. **更新 `Settings.vue`**：
   - 同上，对 plugin_settings 的子字段进行平台过滤
   - 将视频打开方式等移动端独有设置移至 PluginSettings.vue

5. **添加徽章样式**：
   - 在 `PluginSettings.vue` 的 `<style>` 中添加徽章 CSS
   - 蓝色 `.badge-server`、紫色 `.badge-mobile`、绿色 `.badge-v4`

6. **为 `default_stream_preset` 添加卡片式选择器**：
   - 在移动端，使用 ion-card 组展示三个预设选项
   - 每个卡片显示：预设名称、参数概要（quality/bitrateMode/keyFrameInterval）、描述
   - 选中状态用边框高亮
   - 在 `schema.json` 中为 `default_stream_preset` 添加 `enum` 和 `x-enum-labels` 扩展

---

## 实施顺序

1. **任务 1**（ffprobe dlopen 修复）— 最紧急，阻塞用户使用
2. **任务 2**（视频插件配置适配 v4）— 前后端 schema 同步
3. **任务 3**（移动端设置界面可视化）— UX 改进

## 依赖关系

- 任务 2 和任务 3 都涉及 `schema.json` 更新，可以合并处理
- 任务 1 独立，不依赖其他任务
