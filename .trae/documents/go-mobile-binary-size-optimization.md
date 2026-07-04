# Go 移动端二进制体积优化计划（仅 CLI 相关）

## 现状

- **构建命令**：`GOOS=android GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-s -w" -o encv-go-arm64 ./cmd/encv`
- **当前体积**：约 3.8MB（CI 构建输出，已使用 `-s -w` 剥离调试符号）
- **入口点**：`./cmd/encv` — 一个 cobra CLI 程序，移动端仅使用 `start` 子命令
- **移动端 API**：`pkg/encv` 包（Init、NewServer、FindServer、ParseServerFlags）
- **核心问题**：CLI 入口点将 cobra + pterm 及其传递依赖编译进移动端二进制，但移动端根本不经过 `main()`

**移动端实际调用路径**：`Android JNI → libencv-go.so → NewServer() → Start()`，不经过 `main()`

---

## CLI 相关影响因素分析

### 因素 1：pterm 终端 UI 库（预估贡献 ~800KB-1MB）

**依赖链**：
```
pterm/pterm
  ├── atomicgo.dev/cursor
  ├── atomicgo.dev/keyboard
  ├── atomicgo.dev/schedule
  ├── containerd/console
  ├── gookit/color
  ├── lithammer/fuzzysearch
  ├── mattn/go-runewidth
  ├── mattn/go-isatty
  ├── xo/terminfo
  ├── clipperhouse/uax29/v2（Unicode 分词）
  └── golang.org/x/term
```

**使用位置**：`internal/utils/terminal.go`（定义 `PrintSuccess`/`PrintError`/`PrintInfo`/`PrintSection`/`NewSpinner`/`PrintTable`/`PrintBox`/`PrintKV` 等）

**调用方**：
- `cmd/encv/main.go` — `utils.PrintSuccess`、`utils.PrintInfo`
- `cmd/encv/servers.go` — `utils.PrintSection`、`utils.PrintKV`、`utils.PrintInfo`

**问题**：pterm 是终端美化输出库，移动端无终端，完全不需要。但 `internal/utils/terminal.go` 被 `cmd/encv/` 引用，导致 pterm 及其 11 个传递依赖全部编译进移动端二进制。

**优化**：build tag 拆分
- `internal/utils/terminal.go` → 添加 `//go:build !android`
- 新建 `internal/utils/terminal_mobile.go` → `//go:build android`（空实现，用 `slog` 替代）

---

### 因素 2：cobra CLI 框架（预估贡献 ~300-500KB）

**依赖链**：
```
spf13/cobra
  ├── spf13/pflag
  └── inconshreveable/mousetrap（Windows）
```

**使用位置**：
- `cmd/encv/main.go` — 根命令 + 所有子命令定义
- `cmd/encv/cmd.go`（`//go:build !windows`）
- `cmd/encv/cmd_windos.go`（`//go:build windows`）
- `cmd/encv/servers.go` — `start` 子命令
- `cmd/encv/openas_windows.go`（`//go:build windows`）
- `cmd/encv/register_windows.go`（`//go:build windows`）
- `cmd/encv/encv_protocol_windows.go`（`//go:build windows`）

**问题**：cobra 仅用于 CLI 命令解析，移动端通过 `pkg/encv` API 直接调用，不需要命令行解析。

**优化**：创建移动端专用入口点，不导入 cobra

---

### 因素 3：CLI 入口点耦合（架构级根因）

**问题**：当前移动端和桌面端共用 `cmd/encv/main.go` 入口点。这个入口点：
1. 导入 `cobra`（CLI 框架）
2. 定义所有 CLI 子命令（analyze/manifest/kvi/decrypt/encrypt/play/start）
3. `start` 命令中调用 `utils.PrintSection/PrintKV`（拉入 pterm）
4. 其他命令（analyze/manifest/kvi/decrypt/encrypt/play）在移动端完全无用

**优化**：创建 `cmd/encv-mobile/main.go` 作为移动端专用入口

---

### 因素 4：invopop/jsonschema + wk8/go-ordered-map（仅 cmd/encv-schema 使用）

**问题**：`invopop/jsonschema` 和 `wk8/go-ordered-map/v2` 仅在 `cmd/encv-schema/main.go` 中使用，但被列为 `go.mod` 的直接依赖（无 `// indirect` 标记）。虽然 Go 编译器不会将 `cmd/encv-schema` 的代码编译进 `cmd/encv` 二进制，但 `go.mod` 中的直接依赖声明可能影响间接依赖解析。

**可能的关联**：`invopop/jsonschema` → `goccy/go-yaml`（间接），而 `goccy/go-yaml` 可能是 `mongo-driver/v2`/`quic-go`/`golang-asm` 传递依赖链的入口。需 `go mod why` 确认。

**优化**：将 `invopop/jsonschema` 和 `wk8/go-ordered-map` 从 `go.mod` 直接依赖移至 `// indirect`（或确认它们不影响 `cmd/encv` 构建路径后忽略）

---

### 因素 5：间接传递依赖排查（mongo-driver + golang-asm + quic-go）

**依赖链**（需 `go mod why` 确认）：
```
某直接依赖 → go.mongodb.org/mongo-driver/v2
                ├── twitchyliquid64/golang-asm（汇编优化 BSON 编码）
                └── quic-go/quic-go + qpack（QUIC 传输协议）
```

**问题**：这三个库在项目 Go 源码中**无任何直接 import**，完全是传递依赖。如果它们通过 CLI 相关的代码路径引入，创建移动端入口点后可能自动消除。

**排查步骤**：
1. `go mod why go.mongodb.org/mongo-driver/v2`
2. `go mod why github.com/quic-go/quic-go`
3. `go mod why github.com/twitchyliquid64/golang-asm`

如果引入链经过 `cmd/encv/` 下的代码，创建移动端入口点后自动解决。如果不经过，则需在 `internal/` 中找到引入点并断开。

---

## 实施方案

### Step 1：创建移动端专用入口点

新建 `cmd/encv-mobile/main.go`：

```go
package main

import (
    "context"
    "os"

    "github.com/Soltus/encv-go/internal/config"
    "github.com/Soltus/encv-go/pkg/encv"
)

var Version = "dev"

func main() {
    // 移动端正常不经过 main()，由 JNI 直接调用 pkg/encv 导出函数
    // 此处仅作为 fallback
    ctx := context.Background()
    cfg, err := config.Load("")
    if err != nil {
        os.Exit(1)
    }
    ctx = config.NewContext(ctx, cfg)
    encv.Init(ctx)
    s := encv.NewServer(ctx, "")
    addr, err := s.Start(Version)
    if err != nil {
        os.Exit(1)
    }
    _ = addr
    select {}
}
```

修改 `.github/workflows/android.yml` 构建目标：
```bash
# 之前
go build -ldflags="-s -w -X main.version=..." -o app/encv-mobile/encv-go-arm64 ./cmd/encv
# 之后
go build -ldflags="-s -w -X main.version=..." -o app/encv-mobile/encv-go-arm64 ./cmd/encv-mobile
```

### Step 2：terminal.go build tag 拆分

**文件 1**：`internal/utils/terminal.go`（添加 build tag）
```go
//go:build !android

package utils

// ... 现有 pterm 实现不变 ...
```

**文件 2**：新建 `internal/utils/terminal_mobile.go`
```go
//go:build android

package utils

import (
    "fmt"
    "log/slog"
)

func PrintSuccess(format string, args ...any) {
    slog.Info(fmt.Sprintf(format, args...))
}

func PrintError(format string, args ...any) {
    slog.Error(fmt.Sprintf(format, args...))
}

func PrintInfo(format string, args ...any) {
    slog.Info(fmt.Sprintf(format, args...))
}

func PrintWarning(format string, args ...any) {
    slog.Warn(fmt.Sprintf(format, args...))
}

func PrintHeader(text string) {
    slog.Info(text)
}

func PrintSection(text string) {
    slog.Info(text)
}

type SpinnerStub struct{}

func (s *SpinnerStub) Stop() error { return nil }
func (s *SpinnerStub) Success(text string) *SpinnerStub { return s }
func (s *SpinnerStub) Fail(text string) *SpinnerStub { return s }

func NewSpinner(text string) (*SpinnerStub, error) {
    slog.Info(text)
    return &SpinnerStub{}, nil
}

func PrintTable(header []string, data [][]string) {
    slog.Info("table output not supported on mobile")
}

func PrintBox(title, content string) {
    slog.Info(title, "content", content)
}

func PrintKV(key, value string) {
    slog.Info(key, "value", value)
}

func Green(text string) string { return text }
func Yellow(text string) string { return text }
func Cyan(text string) string { return text }
```

### Step 3：排查间接依赖链

在 CI 或本地运行：
```bash
go mod why go.mongodb.org/mongo-driver/v2
go mod why github.com/quic-go/quic-go
go mod why github.com/twitchyliquid64/golang-asm
```

根据结果判断：
- 如果引入链经过 `cmd/encv/` → 创建移动端入口点后自动消除
- 如果引入链经过 `internal/` → 需要在引入点用接口/build tag 断开

### Step 4：清理 go.mod

将 `invopop/jsonschema` 和 `wk8/go-ordered-map/v2` 从直接依赖移至间接：
```bash
# 确认它们不在主构建路径中后
go mod tidy
```

### Step 5：验证

1. CI 构建对比二进制体积
2. 运行 `go tool nm encv-go-arm64 | grep -c pterm` 确认 pterm 符号已消除
3. 运行 `go tool nm encv-go-arm64 | grep -c cobra` 确认 cobra 符号已消除
4. 功能回归测试

---

## 预期效果

| 项目 | 当前 | 优化后 | 减少 |
|------|------|--------|------|
| pterm + 11 个传递依赖 | ~800KB-1MB | 0 | ~800KB-1MB |
| cobra + pflag + mousetrap | ~300-500KB | 0 | ~300-500KB |
| 可能消除的间接依赖* | ~1-1.5MB | 待确认 | 待确认 |
| **总计** | **~3.8MB** | **~2-2.5MB** | **~35-45%** |

> *mongo-driver/golang-asm/quic-go 是否能消除取决于 `go mod why` 的结果

---

## 不涉及的优化（明确排除）

以下因素虽然也影响体积，但本次**不优化**：
- gin 框架及其传递依赖
- go-exif/v3 EXIF 解析
- go-mp4 MP4 解析
- WebDAV 支持
- cbor/v2 编码
- OpenList 代理 + embed.FS
- UPX 压缩
