# 降级设计铁律（Graceful Degradation）

> **核心原则：降级必须是显式的、可感知的、有决策依据的。**
> **任何"用户以为有但实际没有"的功能降级，都是严重的设计缺陷。**
> **功能不可用 > 功能看似可用但实际降级了。**

> 创建：2026-07-01（被批评后修正：之前把静默 fallback 只当 CI 问题，格局小了）

---

## 一、什么是「静默降级」（绝对禁止）

### 1.1 定义

**静默降级（Silent Degradation）**：某功能因为环境不具备 / 初始化失败 / 依赖缺失，系统偷偷切换到降级方案，用户从 UI / API 返回 / 日志摘要上完全感知不到。

**典型反面模式**：
```go
// ❌ 错误：turso 初始化失败，偷偷切到 sqlite，调用方完全不知道
func InitDB() {
    if db, err := turso.Open(...); err == nil {
        globalDB = db
        return
    }
    // 静默 fallback
    globalDB, _ = sqlite.Open(...)  // 既不打 error log，也不暴露状态
}
```

```bash
# ❌ 错误：libsql 编译失败，继续出包，用户以为有向量搜索
set +e
build-libsql || true
```

### 1.2 为什么静默降级是严重缺陷

| 危害 | 说明 |
|------|------|
| **功能幻觉** | 用户/开发者以为功能存在，实际不存在，基于错误假设做决策 |
| **问题延迟暴露** | 直到线上出问题才发现"怎么这个功能没生效"，排查成本极高 |
| **技术债务累积** | 失败一直在发生，但没人知道，最后变成"一直就这样"的历史包袱 |
| **信任破坏** | 用户发现"宣称有但实际没有"后，对整个系统的可靠性产生怀疑 |

### 1.3 判断标准：用户能感知到吗？

**灵魂拷问**：如果我是用户，不翻源码/不 dig 日志，能知道这个功能被降级了吗？

- 能 → 可能合法
- 不能 → 绝对禁止

---

## 二、降级的合法形态

### 2.1 三级降级策略

| 级别 | 名称 | 适用场景 | 用户感知 |
|------|------|---------|---------|
| **L1** | **功能禁用** | 核心依赖缺失，功能完全不可用 | ❌ 明确告知"此功能不可用，原因：X" |
| **L2** | **降级体验** | 可选增强缺失，核心功能可用但体验打折 | ⚠️ 明确告知"当前使用降级模式，部分功能不可用" |
| **L3** | **透明降级** | 纯性能优化，功能完全一致，只是快慢不同 | ✅ 用户无需感知（只有性能差异） |

**铁律**：
- **L1/L2 必须用户可感知** — UI 上要有标识、API 返回要有字段、启动日志要高亮
- **L3 才能静默** — 而且必须是真正的"功能完全一致"，不能有任何功能差异

### 2.2 合法降级的 4 个必要条件

**SHALL** 同时满足：

1. **显式声明** — 文档/代码注释中明确说明"此功能在 X 条件下会降级"
2. **状态可查** — 有统一的接口/UI 能查到当前运行在什么级别（完整/降级/禁用）
3. **降级原因可追溯** — 日志/错误信息明确说明"为什么降级了"
4. **有回归路径** — 不是永久降级，有计划修复或提供开启方式

---

## 三、数据库架构定位铁律

> **SQLite 是权威（Source of Truth），libsql/turso 是增强。**
> **数据库文件就是数据库本身，不是传统后端的"数据库服务"。**

### 3.1 架构定位

| 组件 | 定位 | 缺失后果 | 降级级别 |
|------|------|---------|---------|
| **glebarez/sqlite** | ✅ **权威数据库** | 核心功能不可用 | **L1 — 禁用** |
| **libsql（CGO）** | ⚡ **性能增强 + 向量搜索** | 向量搜索不可用，读写性能下降 | **L2 — 降级体验** |
| **turso（purego）** | 🖥️ **桌面端专用** | 桌面端向量搜索不可用 | **L2 — 降级体验** |

### 3.2 为什么 SQLite 是权威

1. **零依赖** — 纯 Go transpile，任何平台都能跑，不需要 NDK/原生库
2. **数据格式兼容** — libsql 是 SQLite 的 fork，文件格式完全兼容
3. **安卓端无适配问题** — glebarez/sqlite 在 Android 上完美运行
4. **数据库文件 = 数据库本身** — 我们不是传统后端，没有独立的数据库服务，db 文件就是全部

### 3.3 libsql/turso 的正确定位

**不是**：替代 SQLite 的"更好的数据库"

**而是**：
- libsql：SQLite 的超集，增加了向量搜索、并发写优化
- turso：桌面端的 libsql 纯 Go 实现（不需要 CGO）

**正确的架构思路**：
- 默认用 glebarez/sqlite（零依赖、全平台）
- 有原生库时启用 libsql（获得向量搜索 + 性能提升）
- 桌面端可以用 turso（purego，不需要 CGO）

### 3.4 数据库初始化的正确模式

```go
// ✅ 正确：状态可查 + 降级原因明确
type DBInfo struct {
    Engine   string // "sqlite" | "libsql" | "turso"
    Features []string // 启用的功能：["base", "vector_search", ...]
    Reason   string // 如果不是完整模式，说明原因
}

func InitDB() DBInfo {
    // 先尝试完整模式
    if config.UseLibsql {
        if lib, err := libsql.Open(path); err == nil {
            globalDB = lib
            return DBInfo{Engine: "libsql", Features: []string{"base", "vector_search"}}
        }
        // L2 降级：明确记录原因
        log.Warnf("libsql 初始化失败，降级到 sqlite: %v", err)
    }
    
    // SQLite 是权威，必须成功，失败就 panic/返回错误（L1 禁用）
    db, err := glebarez.Open(path)
    if err != nil {
        panic("sqlite 初始化失败，核心功能不可用: " + err.Error())
    }
    globalDB = db
    
    reason := ""
    if config.UseLibsql {
        reason = "libsql 初始化失败，降级到 sqlite"
    }
    return DBInfo{Engine: "sqlite", Features: []string{"base"}, Reason: reason}
}
```

---

## 四、各层级降级的实现规范

### 4.1 L1 — 功能禁用（核心依赖缺失）

**必须做到**：
- UI 上明确显示"此功能不可用"，并说明原因
- API 返回明确的错误码（不是 200 + 空结果）
- 启动日志 ERROR 级别高亮
- 不能假装功能还在（比如返回空列表假装"没有数据"）

**反例**：
```go
// ❌ 错误：搜索功能不可用，但返回空列表假装"没搜到"
func Search(query string) []Result {
    if searchSvc == nil {
        return []Result{}  // 静默降级！用户以为是"没搜到"
    }
    return searchSvc.Search(query)
}
```

**正例**：
```go
// ✅ 正确：明确告知功能不可用
func Search(query string) ([]Result, error) {
    if searchSvc == nil {
        return nil, errors.New("搜索功能不可用：向量搜索服务未初始化")
    }
    return searchSvc.Search(query), nil
}
```

### 4.2 L2 — 降级体验（可选增强缺失）

**必须做到**：
- UI 上有明显标识（比如 "基础模式" / "增强功能不可用" 的徽章）
- API 返回中包含 `mode: "degraded"` 或类似字段
- 日志 WARN 级别记录降级原因
- 明确告知用户"缺了什么功能"，不是模糊的"降级了"

### 4.3 L3 — 透明降级（纯性能差异）

**唯一可以静默的降级**，但必须满足：
- 功能完全一致（API 行为、返回结果、边界情况处理都一样）
- 只有性能差异（快慢）
- 没有任何功能上的缺失

---

## 五、本项目已知的降级点清单

| 位置 | 功能 | 当前降级策略 | 级别 | 是否合规 |
|------|------|------------|------|---------|
| `db_init.go` | 数据库引擎 | libsql/turso → glebarez/sqlite | L2 | ✅ 合规（有日志 + API 暴露 engine/fallbackReason） |
| `mobile_api.go` handleDatabaseInfo | 引擎状态查询 | 返回 engine/requestedEngine/fallbackReason | - | ✅ 合规（状态可查） |
| `mobile_api.go` handleVectorSearchTasksGin | 向量搜索 | 不可用时降级为字符串匹配，返回 `vector_search: false` | L2 | ✅ 合规（有明确字段标识） |
| `android.yml` libsql 步骤 | 原生库编译 | 失败继续构建 + step summary + warning | L2 | ✅ 已修复 |

---

## 六、引用其他规则

- [android.md](./android.md) — SQLite / LibSQL 选型铁律
- [ci-workflow.md](./ci-workflow.md) — CI 中的降级规范（子集）
- [development.md](./development.md) — 开发环境规范

> 创建：2026-07-01
> 修正历史：最初错误地把这个原则塞进了 ci-workflow.md，被批评后纠正为通用原则
