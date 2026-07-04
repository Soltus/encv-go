# 清理失效测试 + 补全双端单元测试与 Mock 基础设施 Spec

## Why

当前项目有 **8 个测试文件编译失败**（全部由 `_v2` 重命名导致测试未同步更新）、**1 个测试文件完全注释掉**、大量核心包**零测试覆盖**（handle、plugins、config、server、mobile_service 等）。同时存在多种阻碍测试的工程范式（init() 副作用、硬编码磁盘依赖、无 mock 基础设施），需要系统性修复。

## 现状诊断

### 编译失败的测试文件（8 个）

| 文件 | 失败原因 | 涉及的旧名引用 |
|------|---------|---------------|
| `container/block/block_v2_test.go` | 整个文件被注释掉 | `WriteBlock_v2`, `ReadBlockHeader_v2`, `ReadBlockData_v2` |
| `container/manifest/bench_test.go` | undefined: `SerializeToJSON_v2`, `DeserializeFromJSON_v2`, `Manifest_v2`, `Fragment_v2`, `KVI_v2`, `ContainerVersion` |
| `crypto/aes_v2_test.go` | undefined: `GenerateKey_v2`, `GenerateSalt_v2`, `GenerateIV_v2`, `EncryptStream_v2`, `DecryptStream_v2` |
| `crypto/aes_v2_bench_test.go` | undefined: `GenerateKey_v2` (6处) |
| `physical/bench_test.go` | undefined: `GenerateKey_v2`, `GenerateSalt_v2`, `GenerateIV_v2`, `EncryptBytes_v2`, `NewManifest_v2`, `Fragment_v2`, `KVI_v2` |
| `reader/bench_test.go` | undefined: `GenerateKey_v2`, `GenerateSalt_v2`, `GenerateIV_v2`, `EncryptStream_v2`, `NewManifest_v2`, `Fragment_v2`, `KVI_v2` |
| `writer/container_writer_v4_test.go` | undefined: `GenerateKey_v2` (6处) |
| `container/detector/bench_test.go` | undefined: `KVI_v2`, `Manifest_v2`, `Fragment_v2` |

### 通过但需关注的测试（3 个）

| 文件 | 状态 | 问题 |
|------|------|------|
| `service/task_manager_state_test.go` | PASS | 日志中有 "rename .tmp : no such file or directory" 警告 — TaskManager 构造函数依赖磁盘路径 |
| `service/mobile_service_errors_test.go` | PASS | 需确认覆盖范围 |
| `service/mock_broadcaster_test.go` | PASS | 已有 mock 模式，可复用 |

### 零测试覆盖的核心包

| 包 | 重要性 | 测试优先级 |
|----|--------|-----------|
| `v2/container/handle` | 🔴 刚创建的核心抽象层，零测试 | P0 |
| `v2/container/detector` | 🔴 容器检测入口 | P0 |
| `v2/plugins/registry` | 🔴 插件初始化（之前 bug 所在） | P0 |
| `v2/plugins/text` | 🔴 TextPlugin Initialize 失败根因 | P0 |
| `v2/crypto` | 🟡 有部分测试，需补全 | P1 |
| `v2/container/block` | 🟡 测试被注释掉 | P1 |
| `v2/reader` | 🟡 只有 bench 测试 | P1 |
| `internal/config` | 🟡 配置管理 | P1 |
| `internal/server` | 🟢 API handler | P2 |
| `internal/service/mobile_service` | 🟢 GetFileInfo 逻辑 | P2 |

### 阻碍测试的工程模式

| 模式 | 位置 | 影响 | 修复方案 |
|------|------|------|---------|
| `init()` 注册 KVI Provider | bench_test.go ×3 | 全局状态污染 | 改为 `TestMain` 或 `setup()` 辅助函数 |
| 直接 `os.CreateTemp` + 磁盘 I/O | 几乎所有集成测试 | 慢、不可并行 | 对纯逻辑用 BytesSource；对 I/O 用 t.TempDir() |
| 无接口抽象（直接用具体类型） | TaskManager, MobileService | 无法注入 Mock | 提取 Broadcaster 接口（已有 mock） |
| 无 mock 框架 | 全局 | 手写成本高 | 引入 `testify/mock` 或手写 test helper |

---

## What Changes

- [ ] **Phase 1：修复所有编译失败** — 同步 `_v2` → 新名的重命名到全部 8 个测试文件
- [ ] **Phase 2：取消注释并修复 block 测试** — 启用 `block_v2_test.go`，更新为新的函数名
- [ ] **Phase 3：建立测试基础设施**
  - 创建 `internal/testutil/` 包：提供通用 mock 和 test helper
  - BytesSource 测试辅助（利用已有的 handle.BytesSource）
  - KVI Provider test fixture 工厂
  - MockBroadcaster（复用现有 mock_broadcaster_test.go 的模式）
- [ ] **Phase 4：补全核心包单元测试**
  - handle 包：V2/V3/V4 解析测试（BytesSource 注入内存数据）
  - detector 包：版本检测、容器类型判断
  - crypto 包：补全 GenerateSalt/IV/Key, Base64Encode 等函数测试
  - plugins/text: Initialize 成功/失败路径
  - plugins/registry: BuildFullPluginSettings, InitializePlugins
- [ ] **Phase 5：改造反测试范式**
  - 将 `init()` 中 KVI 注册改为显式 setup()
  - TaskManager 构造接受可选的 servingDir 参数（或使用 t.TempDir）
- [ ] **Phase 6：迭代运行 `go test ./...` 直到全部通过**

---

## ADDED Requirements

### Requirement: 所有测试必须编译通过

系统 SHALL 确保 `go test ./internal/... -count=1` 零编译错误。

#### Scenario: 运行全量测试
- **WHEN** 执行 `go test ./internal/... -count=1`
- **THEN** 零 build failure，所有测试 PASS 或 SKIP（无 FAIL）

### Requirement: handle 包必须有基础测试

系统 SHALL 为 ContainerHandle 核心逻辑提供单元测试。

#### Scenario: V4 容器内存解析
- **WHEN** 用构造好的 V4 字节序列创建 `BytesSource` 并调用 `handle.Open()`
- **THEN** 返回 Version()==4, HeaderV4()!=nil, FooterV4()!=nil, ManifestV4()!=nil, Manifest()!=nil
- **AND** ContainerType(), IsSeekable() 返回正确值

#### Scenario: V3 容器内存解析
- **WHEN** 同上但注入 V3 字节序列
- **THEN** 返回 Version()==3, FooterV2()!=nil, Manifest()!=nil

#### Scenario: 无效数据返回错误
- **WHEN** 注入非 ENCV magic 字节
- **THEN** Open() 返回包含 "not an ENCV container" 的 error

### Requirement: detector 包测试恢复

系统 SHALL 恢复 detector bench 测试并通过。

### Requirement: crypto 包补全测试

系统 SHALL 为以下函数补充单测：
- `GenerateSalt(size)` / `GenerateIV(size)` / `GenerateKey(password, salt, size)`
- `Base64Encode(data)`
- `EncryptStream` / `DecryptStream` 往返对称性
- `DeobfuscateManifest` / `ObfuscateManifest` 往返对称性

### Requirement: 插件系统关键路径测试

系统 SHALL 覆盖：
- TextPlugin.Initialize 在有/无 PluginSettings 时的行为
- BuildFullPluginSettings 合并默认值和用户配置
- FindEncryptingPlugin / FindDecryptingPlugin 扩展名匹配

---

## MODIFIED Requirements

### Requirement: 测试文件同步更新

所有 `*_test.go` 文件中的类型/函数引用 MUST 与源代码中的最新命名保持一致。

### Requirement: init() 副作用消除

测试包中的 `init()` 函数不应注册全局状态。改为在需要时显式调用。

---

## 执行策略：迭代循环

采用「运行→发现→修复→再运行」的迭代模式：

```
Loop:
  1. go test ./internal/... -count=1 2>&1 | grep -E '(FAIL|PASS|ok|---)'
  2. 收集所有 FAIL 和 build failed
  3. 修复一批
  4. goto 1
until zero failures
```

每轮聚焦一类问题，避免一次性改动过大导致引入新 bug。
