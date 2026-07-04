# 重构 FFmpeg 构建系统：跨平台统一约束 + 工程化架构

## 一、现状诊断

### 1.1 当前架构（两套独立体系，无统一约束）

```
┌────────────────── 桌面端 ──────────────────┐
│  ExecRunner → exec.Command("ffmpeg")       │
│  假设系统安装了完整版 FFmpeg                │
│  ❌ 无功能约束 — 用到什么全靠运气           │
│  ❌ 无构建产物 — 无法验证兼容性             │
└────────────────────────────────────────────┘

┌────────────────── Android 端 ───────────────┐
│  NativeRunner → dlopen("libffmpeg.so")      │
│  build-ffmpeg-android.sh 从源码编译          │
│  ⚠️ 功能列表硬编码在 shell 脚本中            │
│  ⚠️ 与 Go 代码实际使用的功能无关联          │
│  ❌ 桌面端新增用法 → Android 静默失败       │
└────────────────────────────────────────────┘

         ↑ 两边完全独立，无交叉验证
```

### 1.2 具体问题清单

| # | 问题 | 影响 |
|---|------|------|
| P1 | **功能需求无单一来源**：Go 代码用到的编解码器/封装器散落在各 `.go` 文件中，Android 构建脚本的 `--enable-*` 列表是独立维护的副本 | 新增格式支持时容易遗漏 Android 端 |
| P2 | **桌面端零约束**：`exec_runner.go` 直接调用系统 ffmpeg，不验证其是否满足项目需求 | 用户安装了精简版 FFmpeg 时运行时才报错 |
| P3 | **build-info.json 是单向的**：只记录 Android 构建了什么，不记录项目**需要**什么 | 无法做"需要 vs 已有"的完整性校验 |
| P4 | **资源生成管道缺失**（本次报错根因） | `ff_graph_css_data` 未定义 |

### 1.3 Go 代码实际使用的 FFmpeg 功能（审计结果）

从 [content_preprocessor.go](file:///workspace/internal/v2/plugins/video/content_preprocessor.go) 和 [metadata_extractor.go](file:///workspace/internal/v2/plugins/video/metadata_extractor.go) 提取：

| 类别 | 实际使用 | Android 当前配置 |
|------|----------|------------------|
| **视频解码器** | h264, hevc | ✅ h264, hevc |
| **音频解码器** | aac, mp3, opus, vorbis, flac, pcm_s16le/24le/32le, aac_latm | ✅ 全部覆盖 |
| **视频编码器** | libx264 (fallback), h264_nvenc (hw), h264_mediacodec (hw) | ✅ libx264 |
| **音频编码器** | aac | ✅ aac |
| **封装器** | mp4 (mov), matroska (mkv), null | ✅ mp4, matroska, null |
| **解封装器** | mov, matroska, aac, mp3, flac, ogg, wav | ✅ 全部覆盖 |
| **解析器** | h264, hevc, aac/aac_latm, mpegaudio, opus, vorbis | ✅ 全部覆盖 |
| **协议** | file, pipe | ✅ file, pipe |
| **滤镜** | aresample | ✅ aresample |
| **外部库** | libx264 (GPL) | ✅ libx264 |

> 结论：当前 Android 配置**恰好覆盖**了所有已知的 Go 代码用法。但这个"恰好"是脆弱的——没有任何机制保证未来的一致性。

---

## 二、方案设计：三层统一架构

### 2.1 总体架构图

```
                    ┌──────────────────────────────┐
                    │  ffmpeg-feature-manifest.json │ ← ★ 单一真相源
                    │  （项目级 FFmpeg 功能需求声明）  │
                    └──────┬───────────┬────────────┘
                           │           │
              ┌────────────▼──┐  ┌──────▼────────────┐
              │  桌面端验证     │  │  Android 构建     │
              │  (可选 CI 步骤) │  │  (configure 参数) │
              │  ffmpeg -decoders│  │  --enable-decoder=│
              │  vs manifest   │  │  从 manifest 读取 │
              └───────────────┘  └──────┬────────────┘
                                      │
                           ┌──────────▼──────────┐
                           │  build-ffmpeg-       │
                           │  android.sh          │
                           │                      │
                           │  Phase 1: configure   │ ← manifest → flags
                           │  Phase 2: bin2c 资源   │ ← 新增
                           │  Phase 3: fftools 编译  │ ← BUILD_MANIFEST
                           │  Phase 4: 链接+验证    │
                           └──────────┬──────────┘
                                      │
                           ┌──────────▼──────────┐
                           │  build-info.json     │
                           │  (含 manifest 校验和) │
                           │  供 Go 运行时读取     │
                           └─────────────────────┘
```

### 2.2 Layer 0：FFmpeg Feature Manifest（单一真相源）

新建文件 `app/encv-mobile/scripts/ffmpeg-feature-manifest.json`：

```json
{
  "_meta": {
    "version": "1",
    "description": "ENCV 项目 FFmpeg 功能需求清单。桌面端和 Android 构建均以此为准。",
    "last_updated": "2025-05-27"
  },
  "ffmpeg": {
    "version": "8.0",
    "license": "gpl",
    "decoders": ["h264", "hevc", "aac", "mp3", "opus", "vorbis", "flac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "aac_latm"],
    "encoders": ["aac", "pcm_s16le", "pcm_s24le", "pcm_s32le", "libx264"],
    "muxers": ["mp4", "matroska", "flac", "mp3", "adts", "null"],
    "demuxers": ["mov", "matroska", "aac", "mp3", "flac", "ogg", "wav"],
    "parsers": ["h264", "hevc", "aac", "aac_latm", "mpegaudio", "opus", "vorbis"],
    "protocols": ["file", "pipe"],
    "filters": ["aresample"],
    "external_libs": ["libx264"]
  },
  "ffprobe": {
    "shared_with_ffmpeg": ["textformat"],
    "demuxers": ["mov", "matroska", "aac", "mp3", "flac", "ogg", "wav"]
  },
  "ftools_modules": {
    "libffmpeg.so": {
      "core": [
        "fftools/ffmpeg.c", "fftools/ffmpeg_dec.c", "fftools/ffmpeg_demux.c",
        "fftools/ffmpeg_enc.c", "fftools/ffmpeg_filter.c", "fftools/ffmpeg_hw.c",
        "fftools/ffmpeg_mux.c", "fftools/ffmpeg_mux_init.c", "fftools/ffmpeg_opt.c",
        "fftools/ffmpeg_sched.c", "fftools/cmdutils.c", "fftools/opt_common.c",
        "fftools/sync_queue.c", "fftools/thread_queue.c"
      ],
      "graph": ["fftools/graph/graphprint.c"],
      "textformat": [
        "fftools/textformat/avtextformat.c", "fftools/textformat/tf_compact.c",
        "fftools/textformat/tf_default.c", "fftools/textformat/tf_flat.c",
        "fftools/textformat/tf_ini.c", "fftools/textformat/tf_json.c",
        "fftools/textformat/tf_mermaid.c", "fftools/textformat/tf_xml.c",
        "fftools/textformat/tw_avio.c", "fftools/textformat/tw_buffer.c",
        "fftools/textformat/tw_stdout.c"
      ],
      "resources": [
        "fftools/resources/resman.c",
        "fftools/resources/graph.css.c",
        "fftools/resources/graph.html.c"
      ]
    },
    "libffprobe.so": {
      "core": ["fftools/ffprobe.c", "fftools/cmdutils.c", "fftools/opt_common.c"],
      "textformat": "<shared>"
    }
  }
}
```

**关键设计决策**：

| 决策 | 理由 |
|------|------|
| JSON 格式而非 shell 变量 | 可被多种消费者读取：shell 脚本、Go 代码、CI、文档 |
| decoders/encoders 分开列出 | 与 FFmpeg configure 的 `--enable-decoder=` / `--enable-encoder=` 一一对应 |
| ftools_modules 嵌入同一文件 | 源文件清单与功能需求同版本管理，避免漂移 |
| `_meta.version` 字段 | 支持将来扩展 schema（如添加 platform-specific overrides） |

### 2.3 Layer 1：Android 构建脚本消费 Manifest

`build-ffmpeg-android.sh` 的 Phase 1（configure）改为从 manifest 动态生成参数：

```bash
echo "=== Reading FFmpeg feature manifest ==="
MANIFEST="${SCRIPT_DIR}/ffmpeg-feature-manifest.json"

# 用 jq 或 python 从 manifest 提取 configure 参数
# 如果环境没有 jq/python，回退到内联的 helper 函数
if command -v jq &>/dev/null; then
    DECODERS=$(jq -r '.ffmpeg.decoders|join(",")' "$MANIFEST")
    ENCODERS=$(jq -r '.ffmpeg.encoders|join(",")' "$MANIFEST")
    MUXERS=$(jq -r '.ffmpeg.muxers|join(",")' "$MANIFEST")
    DEMUXERS=$(jq -r '.ffmpeg.demuxers|join(",")' "$MANIFEST")
    PARSERS=$(jq -r '.ffmpeg.parsers|join(",")' "$MANIFEST")
    PROTOCOLS=$(jq -r '.ffmpeg.protocols|join(",")' "$MANIFEST")
    FILTERS=$(jq -r '.ffmpeg.filters|join(",")' "$MANIFEST")
else
    # 内联 Python fallback（CI 环境通常有 python3）
    eval "$(python3 -c "
import json, sys
m = json.load(open('$MANIFEST'))
f = m['ffmpeg']
print(f'DECODERS={\",\".join(f[\"decoders\"])}')
print(f'ENCODERS={\",\".join(f[\"encoders\"])}')
print(f'MUXERS={\",\".join(f[\"muxers\"])}')
print(f'DEMUXERS={\",\".join(f[\"demuxers\"])}')
print(f'PARSERS={\",\".join(f[\"parsers\"])}')
print(f'PROTOCOLS={\",\".join(f[\"protocols\"])}')
print(f'FILTERS={\",\".join(f[\"filters\"])}')
")"
fi

echo "  Decoders:  $DECODERS"
echo "  Encoders:  $ENCODERS"
echo "  Muxers:    $MUXERS"
echo "  Demuxers:  $DEMUXERS"
echo "  Parsers:   $PARSERS"
echo "  Protocols: $PROTOCOLS"
echo "  Filters:   $FILTERS"

./configure \
    ...
    --disable-everything \
    --enable-decoder="$DECODERS" \
    --enable-encoder="$ENCODERS" \
    --enable-muxer="$MUXERS" \
    --enable-demuxer="$DEMUXERS" \
    --enable-parser="$PARSERS" \
    --enable-protocol="$PROTOCOLS" \
    --enable-filter="$FILTERS" \
    ...
```

Phase 3 的 BUILD_MANIFEST 同样从 JSON 读取：

```bash
# 从 manifest 读取 fftools 模块定义
# 使用 jq/python 将 JSON 数组转为 shell 兼容格式
eval "$(python3 -c "
import json
m = json.load(open('$MANIFEST'))
for lib_name, modules in m['ftools_modules'].items():
    var_name = lib_name.upper().replace('.', '_') + '_MODULES'
    lines = []
    for mod_name, files in modules.items():
        if files == '<shared>':
            continue
        files_str = ' '.join(files)
        lines.append(f'  \"{mod_name}:{files_str}\"')
    print(f'{var_name}=(')
    for l in lines:
        print(l)
    print(')')
")"
```

### 2.4 Layer 2：桌面端可选验证（CI 守护）

在 CI 或开发脚本中添加一个轻量验证步骤：

```bash
#!/bin/bash
# scripts/validate-ffmpeg-desktop.sh
# 验证桌面端系统 FFmpeg 是否满足项目需求

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MANIFEST="${SCRIPT_DIR}/ffmpeg-feature-manifest.json"

echo "=== Validating system FFmpeg against feature manifest ==="

# 1. 检查 ffmpeg/ffprobe 存在
command -v ffmpeg >/dev/null || { echo "❌ ffmpeg not found"; exit 1; }
command -v ffprobe >/dev/null || { echo "❌ ffprobe not found"; exit 1; }

# 2. 验证所需解码器可用
for decoder in $(python3 -c "import json; print(' '.join(json.load(open('$MANIFEST'))['ffmpeg']['decoders']))"); do
    if ffmpeg -hide_banner -decoders 2>/dev/null | grep -q "^ ${decoder}"; then
        echo "  ✅ decoder: $decoder"
    else
        echo "  ❌ decoder MISSING: $decoder"
        EXIT=1
    fi
done

# 3. 验证所需编码器可用
for encoder in $(python3 -c "import json; print(' '.join(json.load(open('$MANIFEST'))['ffmpeg']['encoders']))"); do
    if ffmpeg -hide_banner -encoders 2>/dev/null | grep -q "^ ${encoder}"; then
        echo "  ✅ encoder: $encoder"
    else
        echo "  ⚠️  encoder missing (may be hw-specific): $encoder"
        # hw encoders (nvenc/mediacodec) 在非对应硬件上不存在是正常的
        # 只标记警告，不阻断
    fi
done

exit ${EXIT:-0}
```

**使用场景**：
- 开发者本地 `make validate-ffmpeg` 快速自检
- CI 中作为 optional job（桌面端 runner 上跑）
- Docker 镜像构建时验证基础镜像是否满足要求

### 2.5 build-info.json 增强

当前的 [build-info.json](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh#L367-L389) 只记录"构建了什么"。增强后同时记录"需要什么"：

```json
{
  "ffmpeg_version": "8.0",
  "...": "...",

  "manifest_checksum": "sha256:abc123...",  // ← 新增：manifest 内容哈希
  "manifest_version": "1",                   // ← 新增：manifest schema 版本

  // 以下保持不变（由 configure 结果填充）
  "enabled_decoders": [...],
  "enabled_encoders": [...],
  "...": "...",

  // ← 新增：一致性校验结果
  "validation": {
    "all_required_decoders_present": true,
    "all_required_encoders_present": true,
    "missing": []
  }
}
```

Go 端的 [build_info.go](file:///workspace/internal/utils/build_info.go) 可以在启动时读取 `validation.missing`，提前告警而非等到运行时报错。

---

## 三、文件变更总览

### 新增文件（2 个）

| 文件 | 职责 | 消费者 |
|------|------|--------|
| `scripts/ffmpeg-feature-manifest.json` | 单一真相源：FFmpeg 功能需求 + fftools 模块清单 | Android 构建脚本、桌面端验证、build-info.json |
| `scripts/validate-ffmpeg-desktop.sh` | 桌面端 FFmpeg 兼容性验证（可选） | 开发者本地、CI |

### 重写文件（1 个）

| 文件 | 变更量 |
|------|--------|
| [`build-ffmpeg-android.sh`](file:///workspace/app/encv-mobile/scripts/build-ffmpeg-android.sh) | ~40% 重写：新增 Phase 2 资源生成；Phase 1/3 改为从 manifest 读取；删除手动枚举循环 |

### 不变文件

| 文件 | 说明 |
|------|------|
| `internal/utils/ffmpeg/exec_runner.go` | 桌面端 runner，无需改动（可选加 validate 入口） |
| `internal/utils/ffmpeg/native_runner.go` | Android runner，无需改动 |
| `internal/utils/build_info.go` | 可选增强：读取 validation 字段 |

---

## 四、实施步骤（按顺序）

### Step 1：创建 `ffmpeg-feature-manifest.json`
基于 1.3 节的审计结果创建初始版本。

### Step 2：重构 `build-ffmpeg-android.sh`
按第二节的四阶段方案执行：
1. Phase 1 configure：从 manifest 读取 `--enable-*` 参数
2. Phase 2 bin2c：编译 bin2c 工具 + 生成资源 .c 文件
3. Phase 3 fftools：从 manifest 读取模块清单 + `compile_modules()` 引擎
4. Phase 4 链接+验证：简化为入口符号检查

### Step 3：创建 `validate-ffmpeg-desktop.sh`（可选）
桌面端验证脚本。

### Step 4：增强 build-info.json 输出
写入 manifest checksum 和 validation 结果。

### Step 5：端到端验证
```bash
# Android 构建
cd app/encv-mobile && bash scripts/build-ffmpeg-android.sh

# 桌面端验证（如有 ffmpeg）
bash scripts/validate-ffmpeg-desktop.sh

# 确认 build-info.json 包含 validation 字段
cat android/app/src/main/jniLibs/arm64-v8a/build-info.json | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('validation',{}))"
```

---

## 五、升级流程（FFmpeg N+1）

```
修改 1 处文件 → 自动传播到全部平台
```

1. 更新 `ffmpeg-feature-manifest.json`：
   - `ffmpeg.version` → `"N+1"`
   - 对比官方 `fftools/Makefile` 的 `OBJS-ffmpeg` → 更新 `ftools_modules`
   - 如有新资源文件 → Phase 2 的 bin2c 循环自动处理
   - 如需新的 decoders/encoders → 更新对应数组

2. 运行构建 → 所有平台自动同步
