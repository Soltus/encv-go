# 修复 FFmpeg fftools 资源符号命名错误导致 dlopen 失败

## Why

Android 端 `libffmpeg.so` 在运行时 `dlopen` 失败，报错 `cannot locate symbol "ff_graph_css_data"`。经过对 FFmpeg 8.0 源码和构建脚本的深入分析，确认这是一个**构建脚本设计缺陷**——`bin2c` 工具的符号命名参数与 FFmpeg 官方 Makefile 的命名规则不一致，导致生成的 C 数组符号名与 `resman.c` 中 `extern` 声明不匹配。由于 `-shared` 链接允许未定义符号存在，该错误在编译期被掩盖，直到运行时 `dlopen(RTLD_NOW)` 才暴露。

## 根因分析

### 符号命名不匹配（核心 Bug）

FFmpeg 官方 Makefile (`ffbuild/common.mak`) 对资源文件的 bin2c 调用规则：

```makefile
%.css.c: %.css.min $(BIN2CEXE)
	$(BIN2C) $< $@ $(subst .,_,$(basename $(notdir $@)))
```

对于 `graph.css.c`：
- `$(notdir graph.css.c)` = `graph.css.c`
- `$(basename graph.css.c)` = `graph.css`
- `$(subst .,_,graph.css)` = `graph_css`
- 生成符号：`ff_graph_css_data[]` 和 `ff_graph_css_len`

而 `build-ffmpeg-android.sh` Phase 2 的调用：

```bash
"${BUILD_DIR}/bin2c" "${res_file}.min" "${res_file}.c" "$(basename "$res_file" .css)"
```

对于 `graph.css`：
- `$(basename "graph.css" .css)` = `graph`
- 生成符号：`ff_graph_data[]` 和 `ff_graph_len`

`resman.c` 中的 extern 声明期望的是 `ff_graph_css_data`，实际生成的是 `ff_graph_data`，符号不匹配。

### 连锁问题

1. **CSS 和 HTML 生成相同符号名**：`graph.css` 和 `graph.html` 都生成 `ff_graph_data[]`，造成重复定义
2. **`--allow-multiple-definition` 掩盖重复定义错误**：链接器静默选择其中一个定义，丢弃另一个
3. **`-shared` 允许未解析的 extern 符号**：`ff_graph_css_data` 和 `ff_graph_html_data` 未定义但链接不报错
4. **运行时 `dlopen(RTLD_NOW)` 失败**：尝试解析所有符号时发现 `ff_graph_css_data` 不存在

### 潜在的 CONFIG_RESOURCE_COMPRESSION 问题

`resman.c` 根据 `CONFIG_RESOURCE_COMPRESSION` 宏决定是否对资源数据进行 gzip 解压。如果 FFmpeg configure 检测到 zlib 并启用了该选项，但构建脚本生成的是未压缩的 minified CSS 数据，则即使符号名修复后，运行时仍会因解压失败而崩溃。需要确认 `config.h` 中该宏的值，并在必要时调整构建流程（添加 gzip 压缩步骤或禁用该特性）。

## What Changes

- 修复 `build-ffmpeg-android.sh` Phase 2 中 bin2c 的符号命名参数，使其与 FFmpeg 官方 Makefile 的 `$(subst .,_,$(basename $(notdir $@)))` 规则一致
- 确认 `CONFIG_RESOURCE_COMPRESSION` 在 Android 构建中的值，必要时调整资源生成流程
- 在 Phase 4 链接后添加资源符号验证步骤，确保 `ff_graph_css_data` 和 `ff_graph_html_data` 存在于 .so 文件中
- 移除或限制 `--allow-multiple-definition` 的使用范围，避免掩盖类似问题

## Impact

- Affected code: `app/encv-mobile/scripts/build-ffmpeg-android.sh`
- Affected runtime: Android 端 FFmpeg/FFprobe 功能完全不可用
- Affected specs: 无其他 spec 受影响，此为独立修复

## ADDED Requirements

### Requirement: FFmpeg 资源符号命名正确性

构建脚本生成的 fftools 资源 C 文件中的符号名 SHALL 与 FFmpeg 源码中 `resman.c` 的 `extern` 声明完全一致。

#### Scenario: CSS 资源符号匹配
- **WHEN** `build-ffmpeg-android.sh` 对 `graph.css` 执行 bin2c 生成 `graph.css.c`
- **THEN** 生成的 C 文件中 SHALL 定义 `ff_graph_css_data[]` 和 `ff_graph_css_len`（而非 `ff_graph_data[]`）

#### Scenario: HTML 资源符号匹配
- **WHEN** `build-ffmpeg-android.sh` 对 `graph.html` 执行 bin2c 生成 `graph.html.c`
- **THEN** 生成的 C 文件中 SHALL 定义 `ff_graph_html_data[]` 和 `ff_graph_html_len`（而非 `ff_graph_data[]`）

### Requirement: 资源压缩配置一致性

构建脚本生成的资源数据格式 SHALL 与 FFmpeg `config.h` 中 `CONFIG_RESOURCE_COMPRESSION` 宏的值一致。

#### Scenario: CONFIG_RESOURCE_COMPRESSION 未启用
- **WHEN** FFmpeg configure 未启用 `CONFIG_RESOURCE_COMPRESSION`
- **THEN** bin2c SHALL 直接处理 minified CSS / 原始 HTML 文件（当前行为，无需 gzip）

#### Scenario: CONFIG_RESOURCE_COMPRESSION 已启用
- **WHEN** FFmpeg configure 启用了 `CONFIG_RESOURCE_COMPRESSION`
- **THEN** bin2c SHALL 处理 gzip 压缩后的文件（`.css.min.gz` / `.html.gz`）

### Requirement: 构建后符号验证

构建脚本 SHALL 在链接完成后验证关键符号存在于输出的 .so 文件中。

#### Scenario: 资源符号验证
- **WHEN** `libffmpeg.so` 链接完成
- **THEN** 验证步骤 SHALL 检查 `nm -D` 输出中包含 `ff_graph_css_data` 和 `ff_graph_html_data`（作为本地符号），或通过 `nm`（非 `-D`）检查这些符号存在

#### Scenario: 重复符号检测
- **WHEN** 构建过程中出现重复符号定义
- **THEN** 构建 SHALL 报错而非静默通过

## MODIFIED Requirements

### Requirement: build-ffmpeg-android.sh Phase 2 资源生成

原实现使用 `$(basename "$res_file" .css)` / `$(basename "$res_file" .html)` 作为 bin2c 的名称参数，生成 `ff_graph_data` 符号。

修改为使用与 FFmpeg Makefile 一致的命名规则：从输出文件名（去掉 `.c` 后缀）派生名称，将 `.` 替换为 `_`。具体公式：

```bash
# 对于输出文件 graph.css.c：
# 去掉 .c → graph.css
# 替换 . 为 _ → graph_css
# bin2c 生成 ff_graph_css_data[]

name=$(basename "${output_file}" .c | tr '.' '_')
```

### Requirement: 链接选项调整

`--allow-multiple-definition` 链接选项 SHALL 仅在必要时使用（如 FFmpeg 多库重复符号 `ff_log2_tab`），并添加注释说明原因。对于 fftools 自身的符号冲突，SHOULD 通过修复源码而非依赖此选项绕过。
