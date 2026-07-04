# Go 测试架构重构：全面防御 + 态势感知

> **触发**：merge 前最后一次提交异常中止；Go 测试无数次把沙箱跑崩。
> **根因（用户提示）**：「把沙箱搞崩一般是没有异常退出的，退出就不会崩」——测试**未干净退出**是沙箱崩溃的根源。
> **范围**：只重构 Go 后端测试（`/workspace/internal/...`）。
> **核心目标**：让 Go 测试**无论何种异常都必退出 + 退出时自动取证**，且运行过程**完全可观测**。

---

## Why（背景与根因）

### 1. 沙箱崩溃的真正根因

| 现象 | 根因 | 反证 |
|------|------|------|
| `go test ./...` 沙箱 OOM | goroutine 泄漏 + temp 文件累积 + 部分测试 panic 后 `t.Fatal` 仍持有 fd | 干净退出的测试不会崩溃 |
| 提交到一半 abort | pre-commit 跑了 `go test`，hang 在某个 case 永远不退，git 被外部 SIGKILL | 强超时 + 强制退出可避免 |
| 重新跑也崩 | 上次崩溃留下了 `/tmp/encv-test-*.bin` 大文件 + 占用的端口 | 需要"启动前清理 + 退出后清理" |

### 2. 当前测试架构的薄弱点（基于 [dual-platform-mock-test-plan.md](file:///workspace/.trae/documents/dual-platform-mock-test-plan.md) + [mock-data-architecture.md](file:///workspace/.trae/rule-library/mock-data-architecture.md) 现状）

| 薄弱点 | 现状 | 风险 |
|--------|------|------|
| 无强超时 | `go test` 默认 10min 总超时，单 case 无超时 | 某个 case 死循环 → 整个测试挂死 |
| 无 panic 拦截 | panic 直接打到 test runner，stack 残缺 | 看不到 panic 现场 |
| 无资源快照 | 跑前后不知道内存涨了多少 | 排查「为什么沙箱 OOM」全靠猜 |
| 无退出码保证 | 即便 panic，os.Exit 也不一定触发 | 沙箱看进程在 → 等 5min → 强杀 |
| 无清理钩子 | t.TempDir() 之外写 `/tmp` 的测试没人管 | 跨次测试累积 |
| Mock 污染 | 17 个 mock handler，部分 mock 走真实 fs 路径 | 跑测试 = 改真实数据 |

### 3. 防御 vs 感知 分层

```
┌────────────────────────────────────────────────────────┐
│  Layer 5  报告层     →  JSON/HTML 统一报告 + 趋势      │
├────────────────────────────────────────────────────────┤
│  Layer 4  感知层     →  结构化日志 + 资源快照 + 落盘    │
├────────────────────────────────────────────────────────┤
│  Layer 3  退出层     →  强超时 + panic 拦截 + 必退出    │  ← 用户提示的根因
├────────────────────────────────────────────────────────┤
│  Layer 2  资源层     →  CPU/内存/FD/Goroutine 硬限     │
├────────────────────────────────────────────────────────┤
│  Layer 1  隔离层     →  t.TempDir + clock + env stub   │
└────────────────────────────────────────────────────────┘
```

---

## Current State（现状盘点）

### 文件结构
```
/workspace/
├── internal/                  # ~13 个包，~216 个测试文件
│   ├── crypto/                ✅ 有测试
│   ├── handle/                ✅
│   ├── detector/              ✅
│   ├── plugins/registry/      ✅
│   ├── task_manager/          ✅
│   ├── webdav/                ✅
│   ├── physical/              ⚠️ 仅 bench
│   ├── block/                 ⚠️ 仅 bench
│   ├── fragment/              ⚠️ 仅 bench
│   ├── envelope/              ⚠️ 仅 bench
│   ├── reader/                ⚠️ 仅 bench
│   ├── service/               ❌ 零覆盖（MobileService）
│   ├── v2/service/            ❌ 零覆盖（ContainerManager）
│   └── ...
├── scripts/                   # 当前散落的脚本，无统一入口
└── .trae/documents/
    ├── dual-platform-mock-test-plan.md   # 已有补全计划
    ├── fix-5-test-issues-plan.md         # 5 个测试 bug 修复计划
    └── go-backend-test-plan.md           # Go 后端测试计划
```

### 已有的良性资产（不要重写）
- `mock-data-architecture.md` 已规定 mock 边界
- `dual-platform-mock-test-plan.md` 已规划 testutil 基础设施
- `saturation-debugging.md` 已沉淀饱和调试套路
- 多数测试用 `t.TempDir()`，基础隔离已具备

---

## Proposed Changes（变更清单）

### 阶段 0：诊断快照（30 分钟，立竿见影）

> **目的**：在写防御代码前，先量化"现在到底崩成什么样"。

#### 0.1 创建 `/workspace/scripts/test-diagnose.sh`

```bash
#!/usr/bin/env bash
# 跑一次 go test 全集，收集：耗时、RSS 峰值、goroutine 峰值、退出码、stderr 摘要
set -uo pipefail
LOG_DIR=".test-runs/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$LOG_DIR"

# 1) 启动资源监视器（每 2s 采样一次）
(
  while true; do
    ps -o pid=,rss=,pcpu=,etime=,comm= -p $(pgrep -f "go test" | head -1) 2>/dev/null \
      >> "$LOG_DIR/resource.csv"
    sleep 2
  done
) &
SAMPLER_PID=$!

# 2) 跑测试，强制 5min 退出
timeout --kill-after=30s 300s go test ./internal/... -count=1 -v -timeout=120s \
  > "$LOG_DIR/stdout.log" 2> "$LOG_DIR/stderr.log"
EXIT=$?

# 3) 杀 sampler
kill $SAMPLER_PID 2>/dev/null

# 4) 归档
echo "exit_code=$EXIT" > "$LOG_DIR/summary.txt"
ls -la .test-runs/ | tail -5
```

#### 0.2 第一次跑（建立 baseline）

执行 `bash scripts/test-diagnose.sh`，得到：
- 总耗时
- RSS 峰值（验证"沙箱 OOM"是否真的发生）
- 退出码（是否 0）
- 是否有 case 没收到 `-timeout` 信号

> **不修复任何代码**。只为了下一阶段有数据支撑。

---

### 阶段 1：退出层（**核心，用户提示的根因**）

> **目标**：让 `go test` **必退出**——任何 case 死循环/panic 都不会让进程挂死。

#### 1.1 强超时链（`scripts/test-go.sh` 替代直接 `go test`）

```bash
#!/usr/bin/env bash
# scripts/test-go.sh — Go 测试唯一入口
# 强保证：无论内部发生什么，10 分钟后此脚本必返回。
set -uo pipefail

HARD_TIMEOUT=${HARD_TIMEOUT:-600}     # 总硬超时 10min
PACKAGE_TIMEOUT=${PACKAGE_TIMEOUT:-120}  # 单包超时 2min
MEM_LIMIT_MB=${MEM_LIMIT_MB:-2048}    # 内存超限 2GB

LOG_DIR=".test-runs/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$LOG_DIR"

echo "=== Go test started at $(date -Iseconds) ===" | tee "$LOG_DIR/summary.txt"
echo "hard_timeout=${HARD_TIMEOUT}s package_timeout=${PACKAGE_TIMEOUT}s" | tee -a "$LOG_DIR/summary.txt"

# 1) 总硬超时（最外层兜底）
timeout --kill-after=30s "${HARD_TIMEOUT}s" go test ./internal/... \
  -count=1 \
  -timeout="${PACKAGE_TIMEOUT}s" \
  -v \
  2> "$LOG_DIR/stderr.log" > "$LOG_DIR/stdout.log"
EXIT=$?

echo "exit_code=$EXIT" | tee -a "$LOG_DIR/summary.txt"
echo "=== Go test finished at $(date -Iseconds) ===" | tee -a "$LOG_DIR/summary.txt"

# 2) 兜底：如果总 timeout 触发（exit=124），再 SIGKILL 所有残留
if [ $EXIT -eq 124 ] || [ $EXIT -eq 137 ]; then
  echo "HARD TIMEOUT HIT, killing residual processes" | tee -a "$LOG_DIR/summary.txt"
  pkill -9 -f "go test" 2>/dev/null
  pkill -9 -f "compile" 2>/dev/null
fi

exit $EXIT
```

**关键点**：
- `timeout --kill-after=30s`：先 SIGTERM，30s 后 SIGKILL
- `-timeout=120s`：go test 自身单包超时
- `HARD_TIMEOUT=600s`：外层兜底
- 残留进程清理：避免下次跑被影响

#### 1.2 测试内 panic 拦截（`testutil/safeguard.go`）

```go
// Package testutil 提供测试"必退出"基础设施。
package testutil

import (
    "context"
    "os"
    "runtime"
    "runtime/pprof"
    "runtime/trace"
    "testing"
    "time"
)

// SafeGo 把 func 包装在 panic recover + watchdog 中。
// 任何 panic 都会被捕获，堆栈写入 .test-runs/crash-<time>.stack。
func SafeGo(t *testing.T, name string, fn func()) {
    t.Helper()
    done := make(chan struct{})
    go func() {
        defer close(done)
        defer func() {
            if r := recover(); r != nil {
                dumpStack(t, name, r)
                t.Errorf("[SafeGo] panic in %s: %v", name, r)
            }
        }()
        fn()
    }()
    // watchdog: 单独 case 超时保护
    select {
    case <-done:
    case <-time.After(2 * time.Minute):
        dumpStack(t, name, "watchdog-timeout")
        t.Fatalf("[SafeGo] %s exceeded 2min", name)
    }
}

func dumpStack(t *testing.T, name string, reason interface{}) {
    crashDir := ".test-runs/crashes"
    _ = os.MkdirAll(crashDir, 0o755)
    fname := fmt.Sprintf("%s/%s-%d.stack", crashDir, name, time.Now().Unix())
    f, err := os.Create(fname)
    if err != nil { return }
    defer f.Close()
    fmt.Fprintf(f, "Reason: %v\n\n", reason)
    _ = pprof.Lookup("goroutine").WriteTo(f, 2)
    _ = pprof.WriteHeapProfile(f)  // 顺便拿 heap
    t.Logf("[CRASH DUMP] %s", fname)
}
```

#### 1.3 残留进程清理（`testutil/cleanup.go`）

```go
// CleanupOnExit 注册退出时清理 t.TempDir() 之外的临时文件。
// 用法：defer testutil.CleanupOnExit(t, []string{"/tmp/encv-test-*.bin"})
func CleanupOnExit(t *testing.T, patterns []string) {
    t.Helper()
    t.Cleanup(func() {
        for _, p := range patterns {
            matches, _ := filepath.Glob(p)
            for _, m := range matches {
                _ = os.RemoveAll(m)
            }
        }
    })
}

// KillOnExit 杀掉测试期间 spawn 的子进程。
// 用法：defer testutil.KillOnExit(t, cmd.Process.Pid)
func KillOnExit(t *testing.T, pids ...int) {
    t.Helper()
    t.Cleanup(func() {
        for _, pid := range pids {
            _ = syscall.Kill(pid, syscall.SIGKILL)
        }
    })
}
```

---

### 阶段 2：资源层（硬限）

> **目标**：让单个测试**不会**耗光沙箱。

#### 2.1 资源采样器（`testutil/probe.go`）

```go
// Probe 监控测试期间的 RSS / goroutine / FD，超过阈值即 t.Fatal。
type Probe struct {
    t          *testing.T
    maxRSSMB   uint64
    maxG       int
    startRSS   uint64
    startG     int
    startFds   int
    interval   time.Duration
}

func StartProbe(t *testing.T, opts ...ProbeOpt) *Probe {
    p := &Probe{t: t, maxRSSMB: 2048, maxG: 5000, interval: 500 * time.Millisecond}
    for _, o := range opts { o(p) }
    p.startRSS = readRSS()
    p.startG = runtime.NumGoroutine()
    p.startFds = countFds()
    go p.loop()
    t.Cleanup(p.report)
    return p
}

func (p *Probe) loop() {
    ticker := time.NewTicker(p.interval)
    defer ticker.Stop()
    for range ticker.C {
        rss := readRSS()
        g := runtime.NumGoroutine()
        if rss > p.maxRSSMB*1024*1024 {
            dumpStack(p.t, "rss-limit", fmt.Sprintf("rss=%dMB > %dMB", rss/1024/1024, p.maxRSSMB))
            p.t.Fatalf("RSS limit exceeded: %dMB", rss/1024/1024)
        }
        if g > p.maxG {
            p.t.Errorf("goroutine leak: %d > %d", g, p.maxG)
        }
    }
}

func (p *Probe) report() {
    endRSS := readRSS()
    endG := runtime.NumGoroutine()
    endFds := countFds()
    delta := struct {
        RSS_MB  int `json:"rss_delta_mb"`
        Goroutine int `json:"goroutine_delta"`
        FDs int `json:"fd_delta"`
    }{
        RSS_MB: int((endRSS - p.startRSS) / 1024 / 1024),
        Goroutine: endG - p.startG,
        FDs: endFds - p.startFds,
    }
    // 写入测试报告
    writeReport(p.t.Name(), delta)
}
```

**典型调用**：
```go
func TestMobileService(t *testing.T) {
    probe := testutil.StartProbe(t,
        testutil.WithMaxRSS(512),  // 512MB 上限
        testutil.WithMaxGoroutine(200),
    )
    defer probe.Snapshot()
    // ... 测试逻辑
}
```

#### 2.2 启动前清理（`scripts/test-go.sh` 阶段 0）

```bash
# 跑测试前：清理上次的残留（避免累积）
echo "[pre-flight] cleaning previous artifacts..."
rm -rf .test-runs/crashes/* 2>/dev/null

# 检测端口占用（WebDAV mock 可能占 2025）
if lsof -i :2025 -t >/dev/null 2>&1; then
  echo "[pre-flight] WARNING: port 2025 in use, killing"
  kill -9 $(lsof -i :2025 -t) 2>/dev/null
fi

# 检测 /tmp 大文件
find /tmp -maxdepth 1 -name "encv-test-*" -size +100M -exec rm -rf {} \;
```

---

### 阶段 3：感知层（结构化日志 + 落盘）

> **目标**：失败现场必须能**离线分析**。

#### 3.1 测试报告格式（`testutil/report.go`）

```go
type TestReport struct {
    Name        string            `json:"name"`
    Package     string            `json:"package"`
    Status      string            `json:"status"`  // pass/fail/skip/timeout/panic
    DurationMS  int64             `json:"duration_ms"`
    StartTime   string            `json:"start_time"`
    EndTime     string            `json:"end_time"`
    RSS_MB_Peak int               `json:"rss_mb_peak"`
    Goroutine_Peak int            `json:"goroutine_peak"`
    FDs_Peak    int               `json:"fd_peak"`
    ErrorMsg    string            `json:"error_msg,omitempty"`
    StackFile   string            `json:"stack_file,omitempty"`
    Logs        []string          `json:"logs,omitempty"`
    SubTests    []TestReport      `json:"sub_tests,omitempty"`
}

// 每个 test 函数 / t.Run 子测试自动产出一份 JSON
// 合并到 .test-runs/<timestamp>/report.json
```

#### 3.2 失败落盘（`testutil/forensics.go`）

```go
// OnFailureHook 注册 t.Cleanup 中的取证钩子。
// 失败时自动保存：stack、heap、env、last 100 行 t.Log。
func OnFailureHook(t *testing.T) {
    crashDir := filepath.Join(".test-runs", "crashes", t.Name())
    t.Cleanup(func() {
        if !t.Failed() { return }
        _ = os.MkdirAll(crashDir, 0o755)
        // 1) stack
        writeFile(filepath.Join(crashDir, "goroutine.stack"), runtimeStack())
        // 2) heap profile
        writeHeapProfile(filepath.Join(crashDir, "heap.pprof"))
        // 3) env
        writeFile(filepath.Join(crashDir, "env.txt"), dumpEnv())
        // 4) test logs（t.Log buffer）
        writeFile(filepath.Join(crashDir, "test.log"), captureLog(t))
        t.Logf("[FORENSICS] dumped to %s", crashDir)
    })
}
```

#### 3.3 跨包统一入口（`scripts/test-all-go.sh`）

```bash
#!/usr/bin/env bash
# scripts/test-all-go.sh — Go 测试唯一入口
# 行为：
#   1. pre-flight 清理
#   2. 跑测试（强超时 + panic 拦截）
#   3. 合并所有 .test-runs/*/report.json → report-all.json
#   4. 失败时 dump 摘要
#   5. 永远 exit 0（让 CI 知道发生了什么）除非真的全过

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

bash scripts/test-go.sh
TEST_EXIT=$?

# 合并报告
python3 scripts/merge-test-reports.py .test-runs/ > .test-runs/report-all.json 2>/dev/null

# 打印摘要
if [ -f .test-runs/report-all.json ]; then
  python3 -c "
import json, sys
data = json.load(open('.test-runs/report-all.json'))
print(f'Total: {data[\"total\"]}  Pass: {data[\"passed\"]}  Fail: {data[\"failed\"]}  Skip: {data[\"skipped\"]}')
print(f'Peak RSS: {data.get(\"peak_rss_mb\", 0)}MB  Max Goroutine: {data.get(\"peak_goroutine\", 0)}')
for f in data.get('failures', [])[:10]:
    print(f'  ✗ {f[\"name\"]}: {f[\"error_msg\"]}')
"
fi

# 不让测试失败阻塞本地开发（CI 走另一条路）
# 但若资源超限，仍然非零退出
if [ $TEST_EXIT -eq 124 ] || [ $TEST_EXIT -eq 137 ]; then
  echo "[FATAL] test process was killed (timeout/OOM)"
  exit 137
fi
exit $TEST_EXIT
```

---

### 阶段 4：隔离层（防 mock 污染）

> **目标**：测试**只**动 t.TempDir()，不污染 `/tmp`、不写真实 fs。

#### 4.1 检测违规（`testutil/sandbox.go`）

```go
// CheckTempLeak 扫描 /tmp 是否有测试期间产生的未清理大文件。
// 跑完所有测试后调用一次。
func CheckTempLeak(t *testing.T) {
    t.Helper()
    var largeFiles []string
    filepath.Walk("/tmp", func(path string, info os.FileInfo, err error) error {
        if err != nil { return nil }
        if info.Size() > 100*1024*1024 && strings.HasPrefix(info.Name(), "encv-") {
            largeFiles = append(largeFiles, fmt.Sprintf("%s (%dMB)", path, info.Size()/1024/1024))
        }
        return nil
    })
    if len(largeFiles) > 0 {
        t.Logf("[TEMP-LEAK] %d files: %v", len(largeFiles), largeFiles)
    }
}
```

#### 4.2 整改 mock 中走真实 fs 的部分

参考 [mock-data-architecture.md](file:///workspace/.trae/rule-library/mock-data-architecture.md) 铁律：
- 任何 mock 走 `os.WriteFile("/tmp/...")` 的 → 改为 `t.TempDir()`
- 任何 mock 调 `os/exec` 真实命令的 → 改为 in-process stub
- 任何 mock 启 `httptest.NewServer` 后没 defer Close 的 → 强制加 t.Cleanup

---

## 实施顺序（3 个 sprint）

### Sprint 1（防御最小可用，1-2 天）
- [ ] **0.1** 写 `scripts/test-diagnose.sh` 跑一次 baseline
- [ ] **1.1** 写 `scripts/test-go.sh`（强超时链）
- [ ] **1.2** 写 `testutil/safeguard.go`（SafeGo + dumpStack）
- [ ] **1.3** 写 `testutil/cleanup.go`（KillOnExit + CleanupOnExit）
- [ ] **2.2** 在 `test-go.sh` 加 pre-flight 清理
- [ ] **3.1** 写 `testutil/report.go`（TestReport struct + JSON 写入）
- [ ] 把 `test-go.sh` 接入 `make test` / `package.json` 的 test 脚本

**验收**：`bash scripts/test-go.sh` 必在 10min 内返回；不残留进程；崩了有 `.test-runs/crashes/` 落盘。

### Sprint 2（资源监控 + 失败落盘，2-3 天）
- [ ] **2.1** 写 `testutil/probe.go`（StartProbe + report）
- [ ] **3.2** 写 `testutil/forensics.go`（OnFailureHook）
- [ ] **3.3** 写 `scripts/test-all-go.sh` + `scripts/merge-test-reports.py`
- [ ] **4.1** 写 `testutil/sandbox.go`（CheckTempLeak）
- [ ] 在 5 个高频测试包里接入 `StartProbe` + `OnFailureHook`

**验收**：跑一次 `test-all-go.sh`，输出 JSON 报告 + 失败时 `.test-runs/crashes/<name>/` 完整。

### Sprint 3（隔离 + 整改 mock，2-3 天）
- [ ] **4.2** 扫描 17 个 mock handler，标出走真实 fs 的，强制整改
- [ ] 给所有走 `os.WriteFile("/tmp/")` 的测试加 `CleanupOnExit`
- [ ] 给所有 `cmd := exec.Command(...)` 的测试加 `KillOnExit`
- [ ] 在 `dev-start-guard.ts` 同样精神下写 Go 版 dev-start-guard

**验收**：连续跑 5 次 `test-all-go.sh`，第 5 次与第 1 次的资源快照差异 < 5%。

---

## 关键文件清单

### 新增
- `/workspace/scripts/test-diagnose.sh` — 诊断快照
- `/workspace/scripts/test-go.sh` — Go 测试唯一入口（强超时）
- `/workspace/scripts/test-all-go.sh` — 跨包统一入口
- `/workspace/scripts/merge-test-reports.py` — 报告合并
- `/workspace/internal/testutil/safeguard.go` — SafeGo + panic 拦截
- `/workspace/internal/testutil/cleanup.go` — 退出清理
- `/workspace/internal/testutil/probe.go` — 资源监控
- `/workspace/internal/testutil/forensics.go` — 失败落盘
- `/workspace/internal/testutil/sandbox.go` — 临时文件泄漏检测
- `/workspace/internal/testutil/report.go` — JSON 报告

### 修改
- `/workspace/Makefile` — 添加 `test-go` / `test-diagnose` 目标
- `/workspace/.gitignore` — 添加 `.test-runs/`
- `/workspace/internal/service/mobile_service_logic_test.go`（新建，作为示范）— 接入 `StartProbe` + `OnFailureHook`

### 不动
- `/workspace/internal/crypto/` 等已有测试（逻辑不变，只在新测试里用新工具）
- 任何生产代码

---

## 验证步骤

### 1. 必退出验证
```bash
# 故意写一个死循环测试
cat > /tmp/hang_test.go <<'EOF'
func TestHang(t *testing.T) { select{} }
EOF

# 跑 1.1 写的 test-go.sh
time bash scripts/test-go.sh
# 期望：< 2min30s 返回（120s package timeout + 30s kill-after）
# 期望：exit code = 124
# 期望：.test-runs/<ts>/stderr.log 含 "panic: test timed out"
```

### 2. 资源硬限验证
```bash
# 故意写一个 OOM 测试
cat > /tmp/oom_test.go <<'EOF'
func TestOOM(t *testing.T) {
    a := make([][]byte, 0)
    for i := 0; i < 1_000_000; i++ { a = append(a, make([]byte, 10*1024*1024)) }
}
EOF

# 用 512MB 上限跑
TEST_GO_MAX_RSS=512 bash scripts/test-all-go.sh
# 期望：触发 RSS limit，t.Fatal("RSS limit exceeded")
# 期望：.test-runs/crashes/rss-limit-*.stack 有堆栈
```

### 3. 残留清理验证
```bash
# 跑完测试
bash scripts/test-all-go.sh
# 检查
ls /tmp/encv-test-* 2>/dev/null | wc -l
# 期望：0
lsof -i :2025 -t | wc -l
# 期望：0
```

### 4. 报告完整性
```bash
# 跑完所有测试
bash scripts/test-all-go.sh
# 检查
cat .test-runs/report-all.json | python3 -m json.tool | head -50
# 期望：total/pass/fail/skip/peak_rss/peak_goroutine 全有
```

### 5. 连续跑稳定性
```bash
# 连续跑 5 次
for i in 1 2 3 4 5; do bash scripts/test-all-go.sh; done
# 期望：每次都干净退出，第 5 次 peak_rss ≈ 第 1 次
```

---

## 假设与决策

### 假设
1. Go 版本 ≥ 1.22（已确认，CI 跑 Go 1.23）
2. `internal/testutil/` 当前不存在或可安全新增
3. 沙箱不限制 `os/exec` 的子进程数（只限制 CPU/内存）
4. 用户接受新增约 600 行测试基础设施代码

### 决策
- **不用** `go test -race` 作主路径（开销大，按需手动开）
- **不**重构已有通过测试的逻辑
- **不**引入第三方依赖（testify/cmp 之类）— 用 stdlib 够用
- **不**做测试并行化（保持顺序跑，避免 goroutine 互相干扰）
- 报告用 JSON 而非 protobuf/TAP（人类可读 + 工具链简单）

### 边界
- 不动 Vue 前端测试（用户明确范围 = Go only）
- 不动 Android/JUnit 测试
- 不动生产代码

---

## 后续（可选，本次不做）

- 把 `.test-runs/report-all.json` 接入 GitHub Actions 的 artifact
- 写一个简单的 HTML dashboard 看趋势
- 接入 `gocover` 做覆盖率趋势
- 引入 fuzz test（`go test -fuzz`）— 待 mock 整改完成后

---

## 引用

- [dual-platform-mock-test-plan.md](file:///workspace/.trae/documents/dual-platform-mock-test-plan.md) — 已有补全计划
- [fix-5-test-issues-plan.md](file:///workspace/.trae/documents/fix-5-test-issues-plan.md) — 5 个测试 bug
- [mock-data-architecture.md](file:///workspace/.trae/rule-library/mock-data-architecture.md) — mock 边界铁律
- [saturation-debugging.md](file:///workspace/.trae/rule-library/saturation-debugging.md) — 饱和调试套路
- [development.md](file:///workspace/.trae/rule-library/development.md) — 后台运行 + 端口铁律
- [test.md](file:///workspace/.trae/rules/test.md) — 测试铁律（mock > 10 个 = 违规）
