# FFmpeg 平台抽象层重构计划

## 问题分析

### 当前状态（Fix 1 后）

```
internal/utils/
├── video.go              (!android) ← FFmpegRun, FFProbeOutput, DetectVideoFormat...
├── ffmpeg_dlopen.go      (android)  ← callFFmpegNative, NativeError, nativeResult
└── ffmpeg_dlopen_stub.go (!android) ← 同上类型的 stub

internal/v2/plugins/video/
├── plugin.go             (!android) ← 引用 utils.FFmpegRun 等
├── content_preprocessor.go (!android) ← 引用 utils.FFmpegRun 等
├── metadata_extractor.go  (!android) ← 引用 utils.FFProbeOutput 等
├── content_verifier.go    (!android) ← 引用 utils.FFmpegRunWithStderr 等
├── mkvtoolnix.go          (!android) ← 引用 utils.FFmpegCmd 等
├── subtitles.go           (无 tag)   ← 仅用 utils.CopyFile/SortExtensions ✅
├── types.go               (无 tag)   ← 纯数据结构 ✅
├── preset.go              (需确认)
└── plugin_android.go      (android)  ← stub，返回 "not available"
```

**痛点**：
- **8 个文件**需要手动维护互斥 build tag
- 新增任何使用 FFmpeg 的文件都必须记得加 `!android`
- `NativeError`/`nativeResult` 类型在 stub 中重复定义
- `plugin_android.go` 是一个 180 行的空壳 stub，所有方法返回错误
- 运行时 `IsMobile()` 分支判断已无意义（编译时已隔离）

### 目标状态

```
internal/utils/
├── ffmpeg/
│   ├── runner.go            (无 tag) ← 接口定义 + 全局注册
│   ├── exec_runner.go       (!android) ← 桌面端：exec.Command 封装
│   └── native_runner.go     (android)  ← Android：dlopen libffmpeg.so
├── video.go                (删除或简化为薄封装)
├── ffmpeg_dlopen.go        (保留，仅被 native_runner.go 引用)
├── ffmpeg_dlopen_stub.go   (删除，NativeError/nativeResult 移入 native_runner)
└── ...其他不变

internal/v2/plugins/video/
├── plugin.go               (无 tag!) ← 通过 ffmpeg.Runner 接口调用
├── content_preprocessor.go (无 tag!)
├── metadata_extractor.go    (无 tag!)
├── content_verifier.go      (no tag!)
├── mkvtoolnix.go            (无 tag!)
├── subtitles.go             (无 tag) ✅ 已是无平台的
├── types.go                 (无 tag) ✅
├── preset.go                (无 tag?)
└── plugin_android.go        (删除!) ← 不再需要
```

**效果**：
- **仅需 1 对** build tag 文件 (`exec_runner.go` ↔ `native_runner.go`)
- **6 个 plugin 文件**全部恢复为无 tag（普通 Go 文件）
- **删除** `plugin_android.go`（180 行 dead code）
- 新增 FFmpeg 使用者**零成本**——直接调用接口方法

---

## 重构方案：FFmpeg Runner 接口

### Step 1: 定义接口

**新文件**: `internal/utils/ffmpeg/runner.go`

```go
package ffmpeg

import "context"

// Runner 抽象了 FFmpeg/FFProbe 的执行方式
// 桌面端通过 exec.Command 调用二进制，Android 通过 dlopen 调用 .so
type Runner interface {
    // Run 执行 ffmpeg 命令，合并 stdout/stderr 到输出
    Run(ctx context.Context, args []string) error
    
    // RunWithOutput 执行命令并捕获完整输出
    RunWithOutput(args []string) (stdout []byte, stderr string, exitCode int, err error)
    
    // Probe 执行 ffprobe 并返回 stdout
    Probe(args []string) ([]byte, error)
    
    // Available 检查 ffmpeg/ffprobe 是否可用
    Available() (ffmpegOk bool, ffprobeOk bool, errMsg string)
}

// 全局 Runner 实例，由各平台 main/init 注册
var globalRunner Runner

func SetRunner(r Runner) { globalRunner = r }
func GetRunner() Runner { return globalRunner }

// 便捷包装函数（向后兼容现有调用方）
func Run(ctx context.Context, args ...string) error {
    return globalRunner.Run(ctx, args)
}

func RunWithOutput(args ...string) (stdout []byte, stderr string, exitCode int, err error) {
    return globalRunner.RunWithOutput(args)
}

func Probe(args ...string) ([]byte, error) {
    return globalRunner.Probe(args)
}
```

### Step 2: 桌面端实现

**新文件**: `internal/utils/ffmpeg/exec_runner.go`

```go
//go:build !android

package ffmpeg

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
    "path/filepath"
    "sync"
)

type ExecRunner struct {
    binDirOnce sync.Once
    binDir     string
}

func (r *ExecRunner) findBin(name string) string {
    r.binDirOnce.Do(func() {
        // 复用原有 GetBinDir() 逻辑
    })
    if r.binDir != "" {
        path := filepath.Join(r.binDir, name)
        if _, err := os.Stat(path); err == nil {
            return path
        }
    }
    return name
}

func (r *ExecRunner) Run(ctx context.Context, args []string) error {
    cmd := exec.CommandContext(ctx, r.findBin("ffmpeg"), args...)
    return cmd.Run()
}

// ... RunWithOutput, Probe, Available 类似实现
// 从原 video.go 和 ffmpeg_dlopen_stub.go 迁移逻辑
```

### Step 3: Android 实现

**新文件**: `internal/utils/ffmpeg/native_runner.go`

```go
//go:build android

package ffmpeg

import (
    "github.com/Soltus/encv-go/internal/utils" // 引用 dlopen 函数
)

type NativeRunner struct{}

func (r *NativeRunner) Run(_ context.Context, args []string) error {
    result, err := utils.CallFFmpegNative(args) // 复用已有 dlopen 逻辑
    // 错误处理...
}

func (r *NativeRunner) Probe(args []string) ([]byte, error) {
    result, err := utils.CallFFprobeNative(args)
    // ...
}

func (r *NativeRunner) Available() (bool, bool, string) {
    return utils.CheckFFmpegAvailable() // CGO 函数，仅在 android 编译时可用
}
```

### Step 4: 清理 utils/video.go

**选项 A（推荐）**: 删除 `video.go`，将其中的非 FFmpeg 函数迁移到合适位置：
- `GetBinDir()` → 移入 `exec_runner.go`（桌面端专用）
- `IsMobile()` → 删除（不再需要运行时判断）
- `DetectVideoFormat()` → 移入 video plugin 包或 `ffmpeg/detect.go`
- `truncateString()` → 移入 `utils/utils.go` 或 `ffmpeg/` 包

**选项 B（最小改动）**: 保留 `video.go` 作为薄封装，委托给 `ffmpeg.Runner`:
```go
//go:build !android  // 仅桌面端需要此兼容层
package utils

import "github.com/Soltus/encv-go/internal/utils/ffmpeg"

// 向后兼容的便捷函数
func FFmpegRun(args ...string) error {
    return ffmpeg.Run(nil, args...)
}
func FFProbeOutput(args ...string) ([]byte, error) {
    return ffmpeg.Probe(args)
}
// ... 其他类似
```

### Step 5: 更新 video plugin 文件

将所有 `!android` 文件的 build tag **移除**，修改 import：

**修改前**:
```go
//go:build !android
package video
import "github.com/Soltus/encv-go/internal/utils"

utils.FFmpegRun(args...)
utils.FFProbeOutput(args...)
```

**修改后**:
```go
// 无 build tag！
package video
import "github.com/Soltus/encv-go/internal/utils/ffmpeg"

ffmpeg.Run(ctx, args...)
ffmpeg.Probe(args...)
```

涉及文件：
- `content_preprocessor.go`
- `metadata_extractor.go`
- `content_verifier.go`
- `mkvtoolnix.go`
- `plugin.go`

### Step 6: 删除冗余文件

| 文件 | 操作 | 原因 |
|------|------|------|
| `plugin_android.go` | **删除** | 不再需要，所有平台共享同一个 plugin.go |
| `ffmpeg_dlopen_stub.go` | **删除** | NativeError/nativeResult 移入 native_runner.go |
| `video.go` (utils) | **删除或保留为薄封装** | 见 Step 4 |

### Step 7: 初始化入口

确保各平台启动时注册正确的 Runner：

**桌面端** (`cmd/encv/main.go` 或类似):
```go
import "github.com/Soltus/encv-go/internal/utils/ffmpeg"
import "github.com/Soltus/encv-go/internal/utils/ffmpeg/_import_exec" // 空包 import 触发 init()
```

或在 `exec_runner.go` 中添加 `init()` 自动注册：
```go
func init() {
    SetRunner(&ExecRunner{})
}
```

**Android** (`EncvGoService.kt` 启动 Go 时):
```go
// native_runner.go 的 init()
func init() {
    SetRunner(&NativeRunner{})
}
```

---

## 实施步骤

### Phase 1: 创建接口层（不破坏现有代码）
1. [ ] 创建 `internal/utils/ffmpeg/runner.go` — 接口 + 全局变量 + 便捷函数
2. [ ] 创建 `internal/utils/ffmpeg/exec_runner.go` — 桌面端实现（从 video.go + stub 迁移）
3. [ ] 创建 `internal/utils/ffmpeg/native_runner.go` — Android 实现（从 dlopen.go 迁移）
4. [ ] 验证编译：`go build ./internal/utils/...`

### Phase 2: 迁移调用方
5. [ ] 修改 5 个 video plugin 文件：移除 `!android` tag，改用 `ffmpeg.*` API
6. [ ] 删除 `plugin_android.go`
7. [ ] 处理 `utils/video.go`（删除或转为薄封装）
8. [ ] 清理 `ffmpeg_dlopen_stub.go`（删除或精简）
9. [ ] 验证编译：桌面端 + Android 交叉编译

### Phase 3: 清理收尾
10. [ ] 更新 project_rules.md（如有新的规则需要添加）
11. [ ] 运行全量测试
12. [ ] 前端构建验证

---

## 风险评估

| 风险 | 概率 | 缓解措施 |
|------|------|----------|
| CGO 函数在 native_runner 中链接失败 | 低 | 与现有 ffmpeg_dlopen.go 使用相同的 CGO 约束 |
| 接口设计不够抽象，未来需要扩展 | 低 | Runner 接口方法覆盖了当前所有使用场景 |
| 遗漏某个调用方的 import 修改 | 中 | 编译器会立即报错 undefined |
| init() 注册顺序问题 | 低 | Runner 在首次使用前必须注册；main() 之前执行 |

## 不做的事情

- **不重构** video plugin 内部的加密/解密业务逻辑
- **不修改** server 层的 API 接口
- **不改变** 前端任何代码
- **不移动** ffmpeg_dlopen.go 本身的 CGO 逻辑（仅调整引用关系）
