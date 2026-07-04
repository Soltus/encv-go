# 容器识别与处理集中化 Spec

## Why

当前容器识别与处理逻辑分散在至少 **6 个包、8 个文件**中，每个消费者都独立执行「打开文件 → 检测版本 → 按版本分支读取 Header/Footer/Manifest」的相同流程。这导致：

1. **大量重复代码**：V4 的 Header+Footer+Manifest 读取模式在 `detector.go`（3处）、`manifest_v2.go`（1处）、`remote_container_reader.go`（1处）、`segment_reader.go`（1处）、`analyze.go`（1处）中各自独立实现，总计 **7 处重复**
2. **版本分支散落各处**：每个涉及容器的函数都有 `if version == 4 { ... } else { ... }` 分支，新增版本时需修改 7+ 个文件
3. **无法单元测试**：所有入口依赖真实磁盘文件（`os.Open`），无法用内存数据测试容器解析逻辑
4. **V4 回退路径脆弱**：移动端 `GetFileInfo` 先试 `OpenV4Container` 再回退 `detector.DetectContainerType`，两套路径维护成本高且行为不一致

## What Changes

- 新增统一抽象层 `ContainerHandle` 接口 + 实现，收敛所有容器打开/检测/元数据访问到单一入口
- 将现有分散的版本分支逻辑内聚到 `ContainerHandle` 内部，外部调用者不再感知版本差异
- 定义 `ContainerSource` 接口支持文件和远程（HTTP Range）两种数据源，便于测试和扩展
- 清理现有消费者代码，迁移到使用 `ContainerHandle`
- **不改变**现有公共 API 的行为语义（向后兼容）

### 核心设计：`ContainerHandle`

```go
// ContainerHandle 是一个已打开容器的统一视图。
// 它在构造时一次性完成版本检测、Header/Footer/Manifest 解析，
// 之后通过类型安全的方法提供对所有元数据的访问。
// 调用者无需关心底层是 v2/v3 还是 v4 容器。
type ContainerHandle interface {
    // === 版本信息 ===
    Version() int                    // 2, 3, 或 4
    HeaderSize() int64              // 对应版本的 Header 大小

    // === 通用属性（跨版本统一）===
    ContainerType() uint16          // 从 V4 Header 或 V2/V3 Manifest 推断
    IsSeekable() bool               // 从 V4 Header 或 V2/V3 Fragment 类型推断
    ContainerID() string            // V4 有，V2/V3 返回 ""
    OriginalDuration() float64      // V4 有，V2/V3 返回 0

    // === Manifest 数据（统一为 Manifest_v2 兼容格式）===
    Manifest() *types.Manifest_v2   // V4 内部自动适配
    ManifestV4() *types.Manifest_v4 // 仅 V4 有效，其他版本返回 nil

    // === 原始结构体（按版本返回）===
    HeaderV2() *types.EnvelopeHeader_v2  // nil 如果不是 v2
    HeaderV3() *types.EnvelopeHeaderV3   // nil 如果不是 v3
    HeaderV4() *types.EnvelopeHeaderV4   // nil 如果不是 v4
    FooterV2() *types.EnvelopeFooter_v2  // nil 如果是 v4
    FooterV4() *types.EnvelopeFooterV4   // nil 如果不是 v4

    // === 底层访问 ===
    Source() ContainerSource           // 数据源（用于 Reader 等下游组件）
    Close() error
}
```

### 核心设计：`ContainerSource`

```go
// ContainerSource 抽象了容器数据的来源，
// 支持本地文件和远程 HTTP Range 两种方式，也便于测试时注入内存数据。
type ContainerSource interface {
    io.ReaderAt
    io.Seeker
    Size() int64
    Name() string
}

// FileSource — 本地文件实现
// RemoteSource — HTTP Range 实现（用于 OpenList 场景）
// BytesSource — 内存字节切片实现（用于单元测试）
```

## Impact

- Affected specs: 无（纯重构）
- Affected code:
  - `internal/v2/container/detector/detector.go` — 消费者迁移到 ContainerHandle，保留公开函数作为薄包装
  - `internal/v2/container/manifest/manifest_v2.go` — `readManifestV4` / `adaptV4ToV2Manifest` 迁移入 ContainerHandle
  - `internal/v2/reader/file_container_reader.go` — 构造函数改为接受 ContainerHandle
  - `internal/v2/reader/remote_container_reader.go` — GetManifest 改为委托给 ContainerHandle
  - `internal/v2/reader/segment_reader.go` — `OpenV4Container` 可基于 ContainerHandle 简化或标记 deprecated
  - `internal/v2/container/detector/analyze.go` — analyzeHeader 等函数改用 ContainerHandle
  - `internal/server/openlist_handlers.go` — handleDecrypt 中检测逻辑简化
  - `internal/service/mobile_service.go` — GetFileInfo 简化

---

## ADDED Requirements

### Requirement: ContainerHandle 统一容器句柄

系统 SHALL 提供 `ContainerHandle` 接口及其实现，作为所有容器操作的唯一入口。

#### Scenario: 打开本地容器文件
- **WHEN** 调用 `OpenContainer(filePath)` 打开一个本地 ENCV 文件
- **THEN** 函数自动检测版本号，读取对应格式的 Header + Footer + Manifest
- **AND** 返回的 `ContainerHandle` 通过统一方法提供所有元数据访问
- **AND** 调用者无需编写任何 `if version == 4` 分支代码

#### Scenario: 打开远程容器（HTTP Range）
- **WHEN** 调用 `OpenContainerRemote(url, headers)` 打开一个远程 ENCV 文件
- **THEN** 使用 HTTP Range 请求完成与本地文件相同的检测和解析流程
- **AND** 返回相同的 `ContainerHandle` 接口

#### Scenario: 单元测试中注入内存数据
- **WHEN** 调用 `OpenContainerFromBytes(data)` 用字节数据构造 ContainerHandle
- **THEN** 所有解析逻辑正常工作，无需真实文件系统
- **AND** 可用于测试 V2/V3/V4 各版本的解析正确性

### Requirement: ContainerSource 数据源抽象

系统 SHALL 定义 `ContainerSource` 接口，支持三种实现。

#### Scenario: 三种实现的互操作性
- **WHEN** 任何接受 `ContainerSource` 的函数被调用
- **THEN** 无论传入 FileSource / RemoteSource / BytesSource，行为一致
- **AND** 下游组件（Reader、Analyzer 等）只依赖 `ContainerSource` 接口

### Requirement: 版本分支内聚

系统的版本判断与格式特定读取逻辑 SHALL 只存在于 `ContainerHandle` 实现内部。

#### Scenario: 外部代码无版本感知
- **WHEN** 业务代码需要获取容器的 ContainerType、IsSeekable、Manifest 等信息
- **THEN** 直接调用 `handle.ContainerType()` / `handle.IsSeekable()` / `handle.Manifest()`
- **AND** 不需要导入 types 包中的 V2/V3/V4 特定类型
- **AND** 不需要编写 `if version == 4` 条件分支

### Requirement: 现有公开 API 向后兼容

系统 SHALL 保留现有的公开函数签名，内部改为委托给 ContainerHandle。

#### Scenario: detector 包的公开函数
- **WHEN** 调用 `detector.DetectContainer(path)` / `detector.DetectContainerType(path)` 等
- **THEN** 行为不变，内部实现改为创建 ContainerHandle 并提取所需字段

#### Scenario: reader 包的工厂函数
- **WHEN** 调用 `reader.NewEncryptedContainerReaderFromFile(path)` 等
- **THEN** 行为不变，内部改为基于 ContainerHandle 构建

---

## MODIFIED Requirements

### Requirement: detector 包重构

`internal/v2/container/detector/detector.go` 中的以下函数 SHALL 改为薄包装：

| 函数 | 当前实现 | 重构后 |
|------|---------|--------|
| `DetectContainer()` | 自行打开文件 + 版本分支 + 读 Footer/Header | 创建 ContainerHandle，从中提取字段 |
| `DetectContainerType()` | 自行打开文件 + 版本分支 + 读 Header 或 Manifest | 创建 ContainerHandle，调用 `handle.ContainerType()` |
| `DetectIsSeekable()` | 同上模式 | 创建 ContainerHandle，调用 `handle.IsSeekable()` |
| `DetectIndexKind()` | 自行打开文件 + 完整 V4 Manifest 读取或 V2/V3 Footer+Manifest | 创建 ContainerHandle，从 `handle.Manifest().Kind` 提取 |
| `DetectV4Header()` | 自行打开文件 + 版本检测 + ReadHeaderV4 | 创建 ContainerHandle，返回 `handle.HeaderV4()` |
| `IsEncvContainerFromBytes()` | 字节级快速检测（保持不变，这是轻量路径） | 保持不变 |

### Requirement: manifest 包重构

`internal/v2/container/manifest/manifest_v2.go` 中的 V4 读取逻辑 SHALL 迁移：

- `readManifestV4()` 函数体迁移至 ContainerHandle 实现内部
- `adaptV4ToV2Manifest()` 迁移至 ContainerHandle 或独立的 adapter 包
- `ReadManifestFromFile()` 保留但改为委托给 ContainerHandle

### Requirement: reader 包重构

`internal/v2/reader/` 中的容器打开逻辑 SHALL 统一：

- `NewEncryptedContainerReaderFromFile()` 内部改用 ContainerHandle 获取 Manifest 和版本信息
- `remote_container_reader.go` 的 `getManifestV4()` 删除重复实现，改为构建 ContainerHandle(RemoteSource)
- `segment_reader.go` 的 `OpenV4Container()` 标记为 convenience wrapper，内部委托给 ContainerHandle

### Requirement: analyze 工具重构

`internal/v2/container/detector/analyze.go` 中的版本分支 SHALL 收敛：

- `analyzeHeader()` 改为接收 ContainerHandle，根据 `handle.Version()` 单次分发
- Footer 分析改为从 `handle.FooterV2()` 或 `handle.FooterV4()` 获取
- `performCrossValidationV4()` 改为接收 ContainerHandle

### Requirement: 服务层简化

- `mobile_service.go` 的 `GetFileInfo()` 移除 try-openV4-then-fallback 模式，直接用 ContainerHandle 一次打开
- `openlist_handlers.go` 的 `handleDecrypt()` 中的检测逻辑可考虑复用 ContainerHandle

---

## REMOVED Requirements

无。本 spec 为纯重构，不删除任何功能。
