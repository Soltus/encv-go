# Spec: 统一性能指标体系 — 任务详情 + 自动化测试报告 + bench-report 融合

> 创建：2026-06-22
> 状态：待确认

---

## 一、Why

### 1.1 现状痛点

**加解密任务详情性能指标不足**：
- 仅显示 `progress` / `phase` / `speed`（字符串 "12.5 MB/s"）/ `eta`（字符串 "30s"）
- `speed` / `eta` 是格式化字符串，无法做聚合统计或趋势分析
- v2 加解密主流程（`EncryptFileWithPlugin` / `DecryptContainerWithPlugin`）**完全无计时 instrumentation**
- 无源文件大小、输出文件大小、加密比率、各 phase 耗时（KDF/encrypt/pack/verify）
- 无峰值吞吐、P50/P99 采样
- Speed/Eta 不持久化，重启即丢

**自动化插件测试报告缺性能维度**：
- 仅有 `durationMs` + `pass rate %`，无吞吐量/评级/历史对比
- `UnifiedRunRecord.results[].duration` 是字符串 "150ms"，非结构化数值
- 无法回答"这次比上次快多少"、"这个 plugin 在这台设备上表现如何"等问题

**bench-report 与任务系统完全解耦**：
- Windows-only CLI 工具（`//go:build windows`，依赖 kernel32.dll `GlobalMemoryStatusEx`）
- 跑 `go test -bench` 微基准，采集 NsPerOp/MBPerSec/BytesPerOp/AllocsPerOp
- 输出独立 HTML（Chart.js 雷达图 + 评分仪表盘 + 历史对比）
- 9 个 category + 硬件校准（AES-CTR 3000 MB/s 基准线 → CPUScore）+ 评级系统（excellent/good/warn）
- **与任务系统无数据流连接**，结果不进入前端，前端自动化测试不收集微基准指标

### 1.2 用户确认的设计方向

| 决策点 | 选择 |
|--------|------|
| bench-report 定位 | 保留并融入任务系统（CLI 跑微基准 + production 采集 + 前端统一展示） |
| 指标采集范围 | 轻量级核心指标（time.Now + atomic，<1% 开销，不采 CPU/内存） |
| 评级与校准 | 引入评级 + 硬件校准 + 历史对比 |
| 跨平台 | 重写为跨平台（废弃 kernel32.dll，改用 GOMEMLIMIT + runtime.MemStats 轻量采样） |
| 指标存储 | 独立 `performance_metrics` 表（task_id FK + JSON payload） |
| 校准时机 | 后端启动时一次，CPUScore 持久化到 SQLite config 表 |
| 报告整合 | 扩展现有 zip 结构（向后兼容 v1，加 performance 可选字段） |

---

## 二、What Changes

### 2.1 新包 `pkg/tasksystem/performance/`（跨平台性能采集 + 评级 + 校准）

#### 2.1.1 `metrics.go` — 性能指标数据结构

```go
// PerformanceMetrics 单次任务的完整性能指标。
type PerformanceMetrics struct {
    TaskID         string         `json:"taskId"`
    TaskType       string         `json:"taskType"`       // encrypt/decrypt/move/copy/...
    PluginName     string         `json:"pluginName,omitempty"`
    ContainerVer   int            `json:"containerVersion,omitempty"`
    CipherMode     int            `json:"cipherMode,omitempty"`
    CompressionMode string        `json:"compressionMode,omitempty"`

    // 文件大小
    SourceSize     int64          `json:"sourceSize"`     // 源文件字节数
    OutputSize     int64          `json:"outputSize,omitempty"` // 输出文件字节数
    SizeRatio      float64        `json:"sizeRatio,omitempty"`  // output/source

    // 吞吐量（MB/s，结构化数值）
    AvgThroughput  float64        `json:"avgThroughput"`  // 平均吞吐 = sourceSize / totalDuration
    PeakThroughput float64        `json:"peakThroughput"` // 峰值吞吐（采样窗口最大值）
    P50Throughput  float64        `json:"p50Throughput"`  // 中位数
    P99Throughput  float64        `json:"p99Throughput"`  // P99

    // Phase 耗时（毫秒）
    PhaseTimings   []PhaseTiming  `json:"phaseTimings"`
    TotalDurationMs int64         `json:"totalDurationMs"`

    // 评级（由 grading.go 计算）
    Grade          Grade          `json:"grade"`          // excellent/good/warn
    GradeScore     float64        `json:"gradeScore"`     // 0-100
    GradeReason    string         `json:"gradeReason,omitempty"`

    // 硬件校准上下文（快照，便于跨设备分析）
    CPUScore       float64        `json:"cpuScore"`       // 校准时的 CPUScore
    CPULabel       string         `json:"cpuLabel"`       // fast/medium/slow

    CreatedAt      time.Time      `json:"createdAt"`
}

type PhaseTiming struct {
    Phase       string `json:"phase"`       // analyzing/initializing/preprocessing/encrypting/decrypting/packing/verifying
    DurationMs  int64  `json:"durationMs"`
    BytesProcessed int64 `json:"bytesProcessed,omitempty"` // 该 phase 处理的字节数
    ThroughputMBps float64 `json:"throughputMBps,omitempty"` // 该 phase 的吞吐量
}

type Grade string
const (
    GradeExcellent Grade = "excellent"
    GradeGood      Grade = "good"
    GradeWarn      Grade = "warn"
)
```

#### 2.1.2 `collector.go` — 轻量级采集器

```go
// Collector 性能采集器。线程安全，基于 time.Now + atomic，开销 <1%。
//
// 设计原则：
//   - 不使用 runtime.ReadMemStats（会 STW）
//   - 不使用 pprof（生产环境不适用）
//   - 不使用 syscall（跨平台兼容性差）
//   - 仅用 time.Now() + sync/atomic + sync.Mutex
type Collector struct {
    taskID       string
    taskType     string
    startTime    time.Time
    phaseStack   []phaseFrame
    throughputSamples []throughputSample // 环形缓冲，容量 64
    mu           sync.Mutex
}

type phaseFrame struct {
    Phase     string
    StartedAt time.Time
    Bytes     int64 // atomic
}

type throughputSample struct {
    Timestamp time.Time
    Bytes     int64
}

// 主要 API：
func NewCollector(taskID, taskType string) *Collector
func (c *Collector) StartPhase(phase string)
func (c *Collector) EndPhase(phase string, bytesProcessed int64)
func (c *Collector) RecordBytes(n int64) // io.Writer wrapper 调用
func (c *Collector) WrapWriter(w io.Writer) io.Writer // 透明包装，零侵入
func (c *Collector) WrapReader(r io.Reader) io.Reader
func (c *Collector) Finalize(sourceSize, outputSize int64, cpuScore float64, cpuLabel string) PerformanceMetrics
```

**采集策略**：
- `StartPhase` / `EndPhase` 用 `time.Now()` 记录 phase 起止
- `WrapReader` / `WrapWriter` 返回透明包装的 `io.Reader` / `io.Writer`，每次 Read/Write 累加字节数 + 采样吞吐量
- 采样窗口：每 500ms 或每 1MB 取一个采样点（取先到者），环形缓冲容量 64
- `Finalize` 计算 avg/peak/p50/p99 + 评级 + 持久化

#### 2.1.3 `calibration.go` — 硬件校准

```go
// CalibrationResult 硬件校准结果。
type CalibrationResult struct {
    CPUScore      float64   `json:"cpuScore"`      // aesThroughput / 3000.0
    AESThroughput float64   `json:"aesThroughput"` // AES-CTR 单线程 MB/s
    CPULabel      string    `json:"cpuLabel"`      // fast(>=1.5) / medium(>=0.5) / slow(<0.5)
    CalibratedAt  time.Time `json:"calibratedAt"`
    GoVersion     string    `json:"goVersion"`
    OS            string    `json:"os"`
    Arch          string    `json:"arch"`
    NumCPU        int       `json:"numCpu"`
}

// RunCalibration 运行 AES-CTR 1MB 加密测试，计算 CPUScore。
// 耗时约 50-200ms（取决于 CPU），启动时调用一次。
func RunCalibration() CalibrationResult

// 阈值（与 bench-report 一致）：
//   fast:   CPUScore >= 1.5 (AES-CTR >= 4500 MB/s)
//   medium: CPUScore >= 0.5 (AES-CTR >= 1500 MB/s)
//   slow:   CPUScore <  0.5
```

**跨平台实现**：
- AES-CTR 测试用 `internal/v2/crypto.EncryptStream_v2`（纯 Go，全平台）
- 测试数据：1MB 随机数据，`crypto/rand.Read`
- 计时：`time.Now()` + `time.Since()`
- 不依赖任何系统调用

#### 2.1.4 `grading.go` — 评级系统

```go
// GradeThresholds 评级阈值（按 taskType + pluginName 区分）。
type GradeThresholds struct {
    ExcelThroughput float64 // MB/s，>= 此值 → excellent
    GoodThroughput  float64 // MB/s，>= 此值 → good
    ExcelDurationMs int64   // ms，<= 此值 → excellent
    GoodDurationMs  int64   // ms，<= 此值 → good
}

// GetThresholds 根据 taskType + pluginName + CPUScore 返回动态阈值。
// CPUScore 越高，吞吐量及格线越高（与 bench-report applyCalibration 一致）。
func GetThresholds(taskType, pluginName string, cpuScore float64) GradeThresholds

// CalculateGrade 计算评级。
func CalculateGrade(metrics PerformanceMetrics, thresholds GradeThresholds) (Grade, float64, string)
// 返回：grade, score(0-100), reason
//
// 评分公式（与 bench-report 一致）：
//   吞吐量分 = (实际 MB/s / ExcelMB) * 100，上限 100
//   延迟分 = (ExcelMs / 实际 ms) * 100，上限 100
//   综合分 = (吞吐量分 + 延迟分) / 2
//   excellent: 综合分 >= 80
//   good:      综合分 >= 50
//   warn:      综合分 < 50
```

**默认阈值表**（按 taskType）：

| taskType | ExcelMB/s | GoodMB/s | 备注 |
|----------|-----------|----------|------|
| encrypt  | 200       | 80       | AES-CTR 加密 |
| decrypt  | 300       | 100      | AES-CTR 解密（通常更快） |
| move     | 500       | 200      | 文件移动（IO 密集） |
| copy     | 300       | 100      | 文件复制 |
| rename   | -         | -        | 元数据操作，不按吞吐评级 |
| delete   | -         | -        | 同上 |

**CPUScore 校准**（与 bench-report `applyCalibration` 一致）：
- `fast` (CPUScore >= 1.5)：ExcelMB × 1.5, GoodMB × 1.5
- `medium` (CPUScore >= 0.5)：ExcelMB × 1.0, GoodMB × 1.0
- `slow` (CPUScore < 0.5)：ExcelMB × 0.6, GoodMB × 0.6

#### 2.1.5 `history.go` — 历史对比

```go
// HistoryComparison 历史对比结果。
type HistoryComparison struct {
    Current    PerformanceMetrics `json:"current"`
    Previous   *PerformanceMetrics `json:"previous,omitempty"`
    ThroughputPctChange float64 `json:"throughputPctChange,omitempty"` // 正数=更快，负数=更慢
    DurationPctChange  float64 `json:"durationPctChange,omitempty"`   // 正数=更慢，负数=更快
    GradeChanged       bool     `json:"gradeChanged"`
}

// CompareWithHistory 与同 plugin + taskType 的上一次运行对比。
func CompareWithHistory(current PerformanceMetrics, previous *PerformanceMetrics) HistoryComparison
```

### 2.2 SQLite 存储扩展

#### 2.2.1 `performance_metrics` 表

```sql
CREATE TABLE IF NOT EXISTS performance_metrics (
    task_id TEXT PRIMARY KEY,
    task_type TEXT NOT NULL,
    plugin_name TEXT,
    container_version INTEGER,
    cipher_mode INTEGER,
    compression_mode TEXT,
    source_size INTEGER,
    output_size INTEGER,
    size_ratio REAL,
    avg_throughput REAL,
    peak_throughput REAL,
    p50_throughput REAL,
    p99_throughput REAL,
    total_duration_ms INTEGER,
    phase_timings_json TEXT,       -- JSON 编码的 []PhaseTiming
    grade TEXT,
    grade_score REAL,
    grade_reason TEXT,
    cpu_score REAL,
    cpu_label TEXT,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
CREATE INDEX IF NOT EXISTS idx_perf_plugin_type ON performance_metrics(plugin_name, task_type);
CREATE INDEX IF NOT EXISTS idx_perf_created_at ON performance_metrics(created_at);
CREATE INDEX IF NOT EXISTS idx_perf_grade ON performance_metrics(grade);
```

#### 2.2.2 `calibration` 表（config 类）

```sql
CREATE TABLE IF NOT EXISTS calibration (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- 单行表
    cpu_score REAL NOT NULL,
    aes_throughput REAL NOT NULL,
    cpu_label TEXT NOT NULL,
    calibrated_at DATETIME NOT NULL,
    go_version TEXT,
    os TEXT,
    arch TEXT,
    num_cpu INTEGER
);
```

#### 2.2.3 Store 接口扩展

`pkg/tasksystem/store/sqlite/sqlite.go` 新增方法：
- `SaveMetrics(metrics performance.PerformanceMetrics) error`
- `GetMetrics(taskID string) (performance.PerformanceMetrics, error)`
- `ListMetricsByPlugin(pluginName string, taskType string, limit int) ([]performance.PerformanceMetrics, error)`
- `GetLatestMetrics(pluginName string, taskType string) (*performance.PerformanceMetrics, error)` — 用于历史对比
- `SaveCalibration(cal performance.CalibrationResult) error`
- `GetCalibration() (*performance.CalibrationResult, error)`

### 2.3 v2 加解密流程 instrumentation

#### 2.3.1 `internal/v2/plugins/registry.go` 改造

`EncryptFileWithPlugin` 和 `DecryptContainerWithPlugin` 加 Collector 集成：

```go
func EncryptFileWithPlugin(ctx context.Context, ..., collector *performance.Collector) error {
    if collector != nil {
        collector.StartPhase("analyzing")
    }
    // ... 分析阶段 ...
    if collector != nil {
        collector.EndPhase("analyzing", 0)
        collector.StartPhase("encrypting")
    }
    // ... 加密主流程 ...
    // io.Copy(collector.WrapWriter(writer), collector.WrapReader(reader))
    if collector != nil {
        collector.EndPhase("encrypting", bytesProcessed)
        collector.StartPhase("packing")
    }
    // ... 打包阶段 ...
    if collector != nil {
        collector.EndPhase("packing", 0)
        collector.StartPhase("verifying")
    }
    // ... 验证阶段 ...
    if collector != nil {
        collector.EndPhase("verifying", 0)
    }
}
```

**零侵入原则**：
- `collector` 参数允许 nil（向后兼容）
- `WrapReader` / `WrapWriter` 返回透明包装，不修改原有 io 流
- 计时点仅用 `time.Now()`，开销可忽略

#### 2.3.2 `internal/service/task_manager.go` 集成

```go
func (tm *TaskManager) processEncrypt(ctx context.Context, task *MobileTask) {
    collector := performance.NewCollector(task.ID, "encrypt")
    collector.StartPhase("initializing")
    // ... 初始化 ...
    collector.EndPhase("initializing", 0)

    err := v2plugins.EncryptFileWithPlugin(ctx, ..., collector)
    if err != nil { ... }

    // 采集文件大小
    sourceSize, _ := os.Stat(task.SourcePath)
    outputSize, _ := os.Stat(task.OutputPath)

    // 获取校准
    cal, _ := tm.store.GetCalibration()

    // Finalize + 评级 + 持久化
    metrics := collector.Finalize(sourceSize.Size(), outputSize.Size(), cal.CPUScore, cal.CPULabel)
    thresholds := performance.GetThresholds("encrypt", task.PluginName, cal.CPUScore)
    grade, score, reason := performance.CalculateGrade(metrics, thresholds)
    metrics.Grade = grade
    metrics.GradeScore = score
    metrics.GradeReason = reason
    tm.store.SaveMetrics(metrics)

    // WS 推送性能摘要
    tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
        "id": task.ID,
        "status": "completed",
        "outputPath": task.OutputPath,
        "performanceSummary": map[string]interface{}{
            "avgThroughput": metrics.AvgThroughput,
            "grade": string(metrics.Grade),
            "gradeScore": metrics.GradeScore,
            "totalDurationMs": metrics.TotalDurationMs,
            "sourceSize": metrics.SourceSize,
            "outputSize": metrics.OutputSize,
        },
    })
}
```

#### 2.3.3 WS `task:progress` 扩展

```go
tm.broadcaster.Broadcast("task:progress", map[string]interface{}{
    "id":            id,
    "progress":      progress,
    "phase":         phase,
    "speed":         speed,         // 保留（向后兼容）
    "eta":           eta,           // 保留（向后兼容）
    "avgThroughput": avgThroughput, // 🆕 结构化数值 MB/s
    "currentPhaseDurationMs": currentPhaseDurationMs, // 🆕 当前 phase 已耗时
})
```

### 2.4 后端 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/tasks/:id/performance` | 返回 PerformanceMetrics |
| GET | `/api/performance/calibration` | 返回当前 CalibrationResult |
| POST | `/api/performance/calibration` | 手动重跑校准（dev-only） |
| GET | `/api/performance/history?plugin=xxx&type=encrypt&limit=10` | 返回历史趋势 |

### 2.5 `cmd/bench-report/` 跨平台重写

#### 2.5.1 移除 Windows 约束

- 删除 `//go:build windows`
- 删除 `memoryGuard` 中的 `kernel32.dll` `GlobalMemoryStatusEx` 调用
- 改用 `runtime.MemStats`（轻量采样，每 5s 一次）+ `GOMEMLIMIT`（基于 `runtime.NumCPU` 和可用内存估算）

#### 2.5.2 新增 `--store` flag

```bash
# 原有行为：生成 HTML + history.json
./bench-report --skip-integration

# 新增：把结果写入 SQLite performance_metrics 表
./bench-report --skip-integration --store /path/to/.encv-tasks.db
```

`--store` flag 触发：
- 把每个 `benchResult` 转换为 `PerformanceMetrics`（task_id 用 `bench-{name}-{timestamp}`）
- 调用 `sqlite.SaveMetrics` 入库
- 前端可通过 `/api/performance/history` 查询微基准历史

#### 2.5.3 保留原有功能

- `go test -bench` 微基准采集（9 个 category）
- HTML 报告生成（Chart.js 雷达图 + 评分仪表盘）
- 历史对比（`bench-history.json`）
- 硬件校准（改用 `performance.RunCalibration()`）

### 2.6 前端适配

#### 2.6.1 `api/encv.ts` 扩展

```typescript
// EncvTask 加字段
interface EncvTask {
  // ... 原有 ...
  performanceSummary?: PerformanceSummary
}

interface PerformanceSummary {
  avgThroughput: number
  grade: 'excellent' | 'good' | 'warn'
  gradeScore: number
  totalDurationMs: number
  sourceSize: number
  outputSize: number
}

interface PerformanceMetrics {
  taskId: string
  taskType: string
  pluginName?: string
  sourceSize: number
  outputSize: number
  sizeRatio: number
  avgThroughput: number
  peakThroughput: number
  p50Throughput: number
  p99Throughput: number
  phaseTimings: PhaseTiming[]
  totalDurationMs: number
  grade: Grade
  gradeScore: number
  gradeReason?: string
  cpuScore: number
  cpuLabel: string
}

interface PhaseTiming {
  phase: string
  durationMs: number
  bytesProcessed?: number
  throughputMBps?: number
}

interface CalibrationResult {
  cpuScore: number
  aesThroughput: number
  cpuLabel: string
  calibratedAt: string
  goVersion: string
  os: string
  arch: string
  numCpu: number
}

// 新增 API
export async function getTaskPerformance(taskId: string): Promise<PerformanceMetrics>
export async function getCalibration(): Promise<CalibrationResult>
export async function recalibrateCalibration(): Promise<CalibrationResult>
export async function getPerformanceHistory(plugin: string, type: string, limit?: number): Promise<PerformanceMetrics[]>
```

#### 2.6.2 `TaskDetailModal.vue` 加性能区块

新子组件 `TaskPerformanceSection.vue`：

```
┌─────────────────────────────────────────┐
│ 性能指标                  [excellent 92] │
├─────────────────────────────────────────┤
│ 源文件大小      128.5 MB                 │
│ 输出大小        129.1 MB                 │
│ 加密比率        1.005                    │
│ 平均吞吐        245.3 MB/s               │
│ 峰值吞吐        312.7 MB/s               │
│ P50 / P99       240 / 298 MB/s           │
│ 总耗时          524 ms                   │
├─────────────────────────────────────────┤
│ Phase 耗时                               │
│ ▓▓▓▓▓▓▓▓ encrypting   312ms  245 MB/s   │
│ ▓▓ packing             89ms             │
│ ▓ verifying            42ms             │
│ ▓ initializing         31ms             │
│ ▓ analyzing            50ms             │
├─────────────────────────────────────────┤
│ 硬件校准        CPUScore 1.8 (fast)      │
│                 校准时间 2026-06-22 10:00│
└─────────────────────────────────────────┘
```

- 仅在 `performanceSummary` 存在时显示
- 评级 badge 颜色：excellent=success / good=primary / warn=warning
- Phase 耗时用进度条可视化（按 durationMs 比例）
- 点击"查看完整指标"展开 `PerformanceMetrics` 详情

#### 2.6.3 `GroupDetail.vue` 加 Performance tab

新 tab：Performance（4 tab：Pipeline / Tasks / Diagnostics / Performance）

```
┌─────────────────────────────────────────┐
│ Performance Tab                          │
├─────────────────────────────────────────┤
│ 硬件校准        CPUScore 1.8 (fast)      │
├─────────────────────────────────────────┤
│ Plugin 聚合表格                          │
│ ┌──────────┬──────┬──────┬──────┬─────┐ │
│ │ Plugin   │ 用例数│ 平均吞吐│ 评级 │ 趋势│ │
│ │ mp4      │  12  │ 245 MB/s│ 92  │ ↗ 5%│ │
│ │ mkv      │  8   │ 198 MB/s│ 78  │ ↘ 2%│ │
│ │ ...      │      │         │     │     │ │
│ └──────────┴──────┴──────┴──────┴─────┘ │
├─────────────────────────────────────────┤
│ 历史趋势（折线图）                       │
│ [吞吐量 vs 时间]                         │
│ [评级分布饼图]                           │
└─────────────────────────────────────────┘
```

- Plugin 聚合：按 pluginName 分组，计算平均吞吐 + 评级分布
- 趋势列：与上一次同 plugin 运行对比的 pctChange
- 折线图：用 Chart.js（与 bench-report 一致）或简单 SVG

#### 2.6.4 `buildReportZip.ts` 扩展

**report.json 加 performance 字段**（向后兼容 v1）：

```json
{
  "schema": "encv-report/v1",
  "run": { ... },
  "summary": { ... },
  "plugins": [{ ... }],
  "failures": [{ ... }],
  "cases": [{
    ...,
    "performance": {  // 🆕 可选字段
      "avgThroughput": 245.3,
      "grade": "excellent",
      "gradeScore": 92,
      "totalDurationMs": 524,
      "sourceSize": 134748160,
      "outputSize": 135266304,
      "phaseTimings": [...]
    }
  }],
  "calibration": {  // 🆕 可选字段
    "cpuScore": 1.8,
    "cpuLabel": "fast",
    "calibratedAt": "2026-06-22T10:00:00Z"
  }
}
```

**summary.md 加性能聚合表格**：

```markdown
## 性能聚合

| Plugin | 用例数 | 平均吞吐 | 评级分布 | 趋势 |
|--------|--------|----------|----------|------|
| mp4    | 12     | 245 MB/s | 10 excellent / 2 good | ↗ 5% |
| mkv    | 8      | 198 MB/s | 6 excellent / 2 good  | ↘ 2% |

硬件校准：CPUScore 1.8 (fast)，校准时间 2026-06-22 10:00
```

**新增 `performance.md` 顶层文件**：

```markdown
# 性能报告

## 硬件校准
- CPUScore: 1.8 (fast)
- AES-CTR 吞吐: 5400 MB/s
- 校准时间: 2026-06-22 10:00:00

## Plugin 性能详情

### mp4
| Case | 平均吞吐 | 峰值 | P99 | 评级 | 总耗时 |
|------|----------|------|-----|------|--------|
| ...  | 245 MB/s | 312  | 298 | excellent | 524ms |

### Phase 耗时分布
（按 plugin 聚合的 phase 耗时箱线图描述）
```

### 2.7 i18n 扩展

`i18n/tasks.ts` 加性能相关翻译键（中英文各约 30 个）：
- `tasks.performance.title` / `tasks.performance.sourceSize` / `tasks.performance.outputSize` / ...
- `tasks.performance.grade.excellent` / `tasks.performance.grade.good` / `tasks.performance.grade.warn`
- `tasks.performance.calibration.title` / `tasks.performance.calibration.cpuScore` / ...
- `tasks.performance.history.title` / `tasks.performance.history.trend` / ...

---

## 三、Impact

### 3.1 后端
- **新增** `pkg/tasksystem/performance/` 包（5 个文件：metrics/collector/calibration/grading/history）
- **新增** SQLite 2 张表（performance_metrics / calibration）+ Store 方法
- **修改** `internal/v2/plugins/registry.go`（EncryptFileWithPlugin / DecryptContainerWithPlugin 加 collector 参数）
- **修改** `internal/service/task_manager.go`（processEncrypt/processDecrypt 集成 collector + WS 推送扩展）
- **修改** `cmd/bench-report/main.go`（跨平台重写 + --store flag）
- **新增** 4 条 API 路由

### 3.2 前端
- **修改** `api/encv.ts`（EncvTask 加 performanceSummary + 新增 4 个 API + 类型定义）
- **新增** `components/TaskPerformanceSection.vue`（任务详情性能区块）
- **修改** `TaskDetailModal.vue`（加性能区块）
- **修改** `GroupDetail.vue`（加 Performance tab）
- **新增** `components/group-detail/PerformanceTab.vue`
- **修改** `buildReportZip.ts`（report.json 加 performance + summary.md 加聚合表 + 新增 performance.md）
- **修改** `i18n/tasks.ts`（加性能翻译键）

### 3.3 性能影响
- 采集开销 <1%（time.Now + atomic，不采 CPU/内存）
- 启动时校准开销 ~50-200ms（一次性）
- SQLite 写入开销 <1ms（单行 INSERT）
- WS 推送扩展字段开销可忽略

---

## 四、ADDED Requirements

1. **性能采集必须跨平台**：Linux/Android/Windows/macOS 全支持，不依赖任何系统调用
2. **采集开销 <1%**：仅用 time.Now + atomic + sync.Mutex，不使用 runtime.ReadMemStats（STW）/ pprof
3. **硬件校准启动时一次**：CPUScore 持久化到 SQLite calibration 表，所有任务共享
4. **评级系统基于动态阈值**：阈值按 taskType + CPUScore 动态调整（与 bench-report applyCalibration 一致）
5. **历史对比 pctChange**：与同 plugin + taskType 的上一次运行对比
6. **performance_metrics 独立表**：task_id FK + JSON payload，TaskData 保持精简
7. **WS task:progress 向后兼容**：新增 avgThroughput / currentPhaseDurationMs 字段，旧字段保留
8. **WS task:completed 向后兼容**：新增 performanceSummary 字段，旧字段保留
9. **buildReportZip 向后兼容**：report.json schema 保持 v1，performance 为可选字段
10. **bench-report 跨平台**：移除 //go:build windows，移除 kernel32.dll 依赖
11. **bench-report --store flag**：可选把微基准结果写入 SQLite performance_metrics 表

## 五、MODIFIED Requirements

1. `EncryptFileWithPlugin` / `DecryptContainerWithPlugin` 签名加 `collector *performance.Collector` 参数（允许 nil）
2. WS `task:progress` payload 扩展（加 avgThroughput / currentPhaseDurationMs）
3. WS `task:completed` payload 扩展（加 performanceSummary）
4. `buildReportZip` report.json 加 performance + calibration 可选字段
5. `buildReportZip` summary.md 加性能聚合表格
6. `buildReportZip` 新增 performance.md 顶层文件

## 六、REMOVED Requirements

1. bench-report `//go:build windows` 约束
2. bench-report `kernel32.dll` `GlobalMemoryStatusEx` 依赖
3. bench-report `memoryGuard` 中的 Windows API 调用

---

## 七、验收标准

### 7.1 功能验收
- [ ] 加密任务完成后，TaskDetailModal 显示性能区块（源大小/输出大小/平均吞吐/峰值/P50/P99/各 phase 耗时/评级/CPUScore）
- [ ] 解密任务同上
- [ ] GroupDetail Performance tab 显示 plugin 聚合表格 + 历史趋势
- [ ] 导出 zip 含 performance.md + report.json 含 performance 字段 + summary.md 含性能聚合表
- [ ] bench-report 在 Linux/macOS 可编译运行
- [ ] bench-report --store flag 把结果写入 SQLite
- [ ] 硬件校准启动时自动运行，CPUScore 持久化
- [ ] 历史对比显示 pctChange 趋势

### 7.2 性能验收
- [ ] 采集开销 <1%（对比启用前后加密 100MB 文件耗时差异）
- [ ] 启动时校准开销 <300ms
- [ ] SQLite 写入不阻塞任务完成（异步或 <1ms）

### 7.3 兼容性验收
- [ ] 旧版 WS 客户端（不识别新字段）仍能正常工作
- [ ] 旧版 buildReportZip 解析（无 performance 字段）不报错
- [ ] bench-report 无 --store flag 时行为与原来一致

---

## 八、相关规则

- [development.md](../../.trae/rules/development.md) — 开发环境规范
- [automation-workflow.md](../../.trae/rules/automation-workflow.md) — 自动化测试 4 件套 WS 事件
- [capacitor.md](../../.trae/rules/capacitor.md) — Capacitor 架构规范
- [android.md](../../.trae/rules/android.md) — gomobile + sqlite 选型（modernc.org/sqlite pure-Go）
