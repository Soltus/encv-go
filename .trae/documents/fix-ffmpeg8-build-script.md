# FFmpeg 8.0 构建修复 + 移动端第三方库展示

## 背景

两个任务：
1. FFmpeg 8.0 构建脚本的 fftools 源文件列表不完整，需要修复
2. 移动端设置界面需要展示模块使用的第三方库信息，包含 FFmpeg 编译的详细技术细节

---

## 第一部分：FFmpeg 8.0 构建脚本修复

### 问题分析

根据 FFmpeg 8.0 官方 `fftools/Makefile`，当前脚本的 FFMPEG_FFTOOLS 列表缺少以下文件：

**缺失的核心文件：**
- `ffmpeg_sched.c` — 调度器（7.x 已有，脚本遗漏）

**FFmpeg 8.0 新增模块（ffmpeg 和 ffprobe 都依赖）：**
- `textformat/` 目录下的所有 `.c` 文件 — 当前只为 ffprobe 收集了，ffmpeg 也需要
- `graph/graphprint.c` — 执行图打印功能
- `resources/resman.c` — 资源管理器
- `resources/graph.html.c`、`resources/graph.css.c` — 嵌入式资源（构建时生成）

**CFLAGS 缺少 include 路径：**
- `-I${FFMPEG_SRC}/fftools/graph`
- `-I${FFMPEG_SRC}/fftools/resources`

### 实施步骤

#### Step 1：更新 FFMPEG_FFTOOLS 列表

补充 `ffmpeg_sched.c`，添加 textformat/graph/resources 的通配符收集。

**关于 graph 和 resources 模块**：`graph.html.c` 和 `graph.css.c` 是构建时从 HTML/CSS 生成的嵌入资源，源码包中不存在预生成的 `.c` 文件。策略：通配符收集时自动跳过不存在的文件；如果 `graphprint.c` 编译失败（缺少依赖），则跳过。我们不需要执行图打印功能。

#### Step 2：补充 CFLAGS include 路径

添加 `-I${FFMPEG_SRC}/fftools/graph` 和 `-I${FFMPEG_SRC}/fftools/resources`。

#### Step 3：改进编译错误处理

当前脚本在 fftools 编译失败时使用 `continue` 跳过，会导致链接时缺少目标文件。改为：
- 核心文件（ffmpeg.c, ffmpeg_sched.c, cmdutils.c 等）编译失败 → 终止构建
- 可选模块（graph/, resources/）编译失败 → 警告并跳过

#### Step 4：在构建脚本末尾生成构建信息 JSON

构建成功后，生成一个 `build-info.json` 文件，包含 FFmpeg 版本、编译配置、启用的编解码器等技术细节。此文件将被打包进 Android APK，供前端读取展示。

```bash
cat > "${OUTPUT_DIR}/build-info.json" << EOF
{
  "ffmpeg_version": "${FFMPEG_VERSION}",
  "x264_version": "${X264_VERSION}",
  "ndk_version": "${NDK_VERSION}",
  "api_level": ${API_LEVEL},
  "abi": "${ABI}",
  "build_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "configure_flags": [...],
  "enabled_decoders": [...],
  "enabled_encoders": [...],
  "enabled_muxers": [...],
  "enabled_demuxers": [...],
  "enabled_parsers": [...],
  "enabled_protocols": [...],
  "enabled_filters": [...],
  "static_libs": [...],
  "linking": "static-into-so",
  "cflags": "..."
}
EOF
```

---

## 第二部分：移动端第三方库展示

### 设计思路

在设置界面的"关于"页面（`AboutDetail.vue`）中新增"第三方库"区域，展示每个模块使用的第三方库及其编译技术细节。

**展示内容：**

1. **FFmpeg 8.0 "Huffman"**
   - 版本号、代号
   - 编译配置（启用的编解码器、容器格式、协议等）
   - 链接方式（静态链接进 libffmpeg.so/libffprobe.so）
   - NDK 版本、API Level、目标架构
   - CFLAGS 关键参数
   - 构建日期

2. **x264**
   - 版本
   - 编译选项（--enable-pic, --enable-static）
   - 许可证（GPL）

3. **Go 标准库 / 运行时**
   - Go 版本

4. **其他前端依赖**（可选）
   - Ionic、Vue、Capacitor 等版本

### 数据流设计

```
构建时 → build-info.json → Android assets → Go 后端 API → 前端展示
```

**方案选择**：构建信息有两种传递方式：

方案 A：构建信息写入 `build-info.json`，打包进 APK 的 assets，Go 后端通过 Android Asset API 读取并暴露为 `/api/build-info` 端点
- 优点：数据准确，与实际编译结果一致
- 缺点：需要修改 Go 后端和 Android 端

方案 B：构建信息硬编码在 Go 后端的编译变量中（通过 `-ldflags` 注入），暴露为 `/api/build-info` 端点
- 优点：简单，不需要读取文件
- 缺点：需要 CI 在构建时注入变量

方案 C：前端直接读取静态 JSON 文件（放在 `public/` 目录），构建时由脚本生成
- 优点：最简单，不需要后端参与
- 缺点：移动端 APK 中静态文件路径可能不同

**选择方案 A**：构建信息写入 `build-info.json`，放在 `jniLibs/${ABI}/` 目录下（与 .so 文件一起），Go 后端通过 `ENCV_LIB_DIR` 环境变量读取该文件，暴露为 `/api/build-info` 端点。

### 实施步骤

#### Step 5：Go 后端 — 新增 `/api/build-info` 端点

1. 在 `internal/utils/` 下新增 `build_info.go`（Android）和 `build_info_stub.go`（桌面端）

`build_info.go`：
```go
//go:build android

package utils

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
)

var (
    buildInfoOnce sync.Once
    buildInfoData map[string]interface{}
    buildInfoErr  error
)

func GetBuildInfo() (map[string]interface{}, error) {
    buildInfoOnce.Do(func() {
        libDir := os.Getenv("ENCV_LIB_DIR")
        if libDir == "" {
            buildInfoErr = fmt.Errorf("ENCV_LIB_DIR not set")
            return
        }
        path := filepath.Join(libDir, "build-info.json")
        data, err := os.ReadFile(path)
        if err != nil {
            buildInfoErr = fmt.Errorf("failed to read build-info.json: %w", err)
            return
        }
        var result map[string]interface{}
        if err := json.Unmarshal(data, &result); err != nil {
            buildInfoErr = fmt.Errorf("failed to parse build-info.json: %w", err)
            return
        }
        buildInfoData = result
    })
    return buildInfoData, buildInfoErr
}
```

`build_info_stub.go`：
```go
//go:build !android

package utils

func GetBuildInfo() (map[string]interface{}, error) {
    return nil, fmt.Errorf("build info not available on this platform")
}
```

2. 在 `internal/server/mobile_api.go` 中新增 handler：
```go
func (s *Server) handleBuildInfoGin(c *gin.Context) {
    info, err := utils.GetBuildInfo()
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
        return
    }
    info["app_version"] = s.version
    c.JSON(http.StatusOK, info)
}
```

3. 在 `internal/server/server.go` 中注册路由：
```go
r.GET("/api/build-info", s.handleBuildInfoGin)
```

#### Step 6：前端 API — 新增 `fetchBuildInfo` 函数

在 `app/encv-mobile/src/api/encv.ts` 中新增：
```typescript
export interface BuildInfo {
  ffmpeg_version: string
  x264_version: string
  ndk_version: string
  api_level: number
  abi: string
  build_date: string
  configure_flags: string[]
  enabled_decoders: string[]
  enabled_encoders: string[]
  enabled_muxers: string[]
  enabled_demuxers: string[]
  enabled_parsers: string[]
  enabled_protocols: string[]
  enabled_filters: string[]
  static_libs: string[]
  linking: string
  cflags: string
  app_version?: string
}

export async function fetchBuildInfo(): Promise<BuildInfo> {
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/build-info`)
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  return await response.json()
}
```

#### Step 7：前端 UI — 改造 AboutDetail.vue

在"关于"页面新增"第三方库"区域，展示 FFmpeg 编译详情。

**UI 设计：**
- 使用 `ion-accordion` 折叠面板展示每个第三方库
- FFmpeg 条目展开后显示详细编译信息（编解码器列表、容器格式、链接方式等）
- 使用标签/徽章展示关键信息（版本号、许可证、架构等）
- 技术细节使用等宽字体、分组展示

**布局：**
```
关于
├── ENCV-go v1.0.0
├── 引擎: ENCV-go Daemon
├── GitHub
│
第三方库
├── ▸ FFmpeg 8.0 "Huffman"     [GPL] [arm64-v8a]
│   ├── 编译配置
│   │   ├── NDK: 26.1.10909125
│   │   ├── API Level: 24
│   │   ├── 链接: 静态链接进 libffmpeg.so/libffprobe.so
│   │   ├── CFLAGS: -std=c11 -fPIC -DANDROID ...
│   │   └── 构建日期: 2026-05-25
│   ├── 解码器: h264, hevc, aac, mp3, opus, ...
│   ├── 编码器: aac, pcm_s16le, libx264
│   ├── 封装器: mp4, matroska, flac, ...
│   ├── 解封装器: mov, matroska, aac, ...
│   └── 协议: file, pipe
│
├── ▸ x264 (stable)            [GPL]
│   └── 编译选项: --enable-static --enable-pic --disable-cli
│
└── ▸ Go Runtime               [BSD]
    └── 版本: go1.22.x
```

#### Step 8：前端 i18n — 添加翻译键

在 i18n 文件中添加第三方库相关的翻译键。

---

## 修改文件清单

### 构建脚本
1. `app/encv-mobile/scripts/build-ffmpeg-android.sh` — 修复 fftools 源文件列表、CFLAGS、编译错误处理、生成 build-info.json

### Go 后端
2. `internal/utils/build_info.go` — 新增，Android 端读取 build-info.json
3. `internal/utils/build_info_stub.go` — 新增，桌面端 stub
4. `internal/server/mobile_api.go` — 新增 handleBuildInfoGin handler
5. `internal/server/server.go` — 注册 /api/build-info 路由

### 前端
6. `app/encv-mobile/src/api/encv.ts` — 新增 fetchBuildInfo 函数和 BuildInfo 类型
7. `app/encv-mobile/src/views/AboutDetail.vue` — 改造，新增第三方库展示区域
8. `app/encv-mobile/src/i18n/` — 添加翻译键（zh-CN 和 en）

---

## 验证清单

### 构建脚本
1. `./configure` 成功完成
2. `make` 成功编译所有库
3. fftools 编译成功（无缺失符号、无头文件错误）
4. 链接成功（无重复符号错误、无未定义引用）
5. `nm -D libffprobe.so | grep av_log` — 确认 FFmpeg 符号已静态链接
6. `nm -D libffprobe.so | grep ffprobe_run` — 确认入口函数存在
7. `nm -D libffmpeg.so | grep ffmpeg_run` — 确认入口函数存在
8. `readelf -d libffprobe.so | grep NEEDED` — 无 libavutil.so 等动态依赖
9. `build-info.json` 生成且内容正确

### 后端 API
10. `go build ./internal/...` 编译通过
11. `/api/build-info` 返回正确的 JSON

### 前端
12. `vue-tsc --noEmit && vite build` 通过
13. 关于页面正确展示第三方库信息
14. FFmpeg 详情展开后显示编译技术细节
