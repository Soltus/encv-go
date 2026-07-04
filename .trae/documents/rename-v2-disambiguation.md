# 排查 `_v2` 后缀歧义 + 重命名计划

## 核心发现：两类 `_v2` 后缀的语义混淆

项目存在**两层嵌套的版本命名**，导致严重歧义：

```
internal/v2/          ← 整个目录 = 架构版本 v2（替代了旧的 v1 架构）
  └── Manifest_v2     ← 类型后缀 = ??? （看起来像"v2 容器格式"，实际是通用数据模型）
  └── EnvelopeHeader_v2 ← 这个才是真正的"v2 容器格式"
```

### 分类表

#### ✅ A 类：`_v2` = 容器格式版本 2（命名正确，无需改动）

这些是**真正的 V2 容器二进制格式**，只存在于 V2 容器中：

| 标识符 | 含义 | 所在文件 |
|--------|------|----------|
| `EnvelopeHeader_v2` struct | V2 Header 二进制结构（16B） | [container.go](internal/v2/types/container.go) |
| `EnvelopeFooter_v2` struct | V2 Footer 二进制结构（32B） | [container.go](internal/v2/types/container.go) |
| `EnvelopeHeaderSize_v2 = 16` | V2 Header 大小常量 | [container.go](internal/v2/types/container.go) |
| `EnvelopeFooterSize_v2 = 32` | V2 Footer 大小常量 | [container.go](internal/v2/types/container.go) |
| `MagicFooter_v2` | V2 Footer Magic（与 V4 共用同一值） | [container.go](internal/v2/types/container.go) |

#### ⚠️ B 类：`_v2` = API / 数据模型版本（**非容器版本**，命名有歧义）

这些是**当前通用的内部数据模型**，被 V2、V3、V4 **所有容器版本**共同使用。`_v2` 的含义是「当前架构下的第二版数据模型」，不是「v2 格式容器专用」。

| 标识符 | 实际含义 | 歧义风险 | 建议重命名为 |
|--------|---------|----------|-------------|
| **`Manifest_v2`** | **通用 Manifest 数据模型**（V4 通过 AdaptV4ToV2 适配到它） | 🔴 **最高**——让人误以为只有 V2 用 | `Manifest` |
| **`Fragment_v2`** | 通用分片描述结构 | 🟡 高 | `Fragment` |
| **`FragmentType_v2`** | 分片类型枚举 | 🟡 高 | `FragmentType` |
| **`KVI_v2`** | 加密信息结构 | 🟡 中 | `KVI` 或 `EncryptionInfo` |
| **`BlockHeader_v2`** | Block 系统的二进制头结构 | 🟡 中 | `BlockHeader` |
| **`BlockType*_v2`** | Block 类型常量（Manifest/KVI/Data/Recovery） | 🟢 低（常量名冲突风险小） | 保持或改 `BlockType*` |
| **`SaltSize_v2` / `KeySize_v2` / `IVSize_v2`** | 加密参数常量 | 🟢 低 | 保持 |
| **`ByteOrder_v2` / `ErrInvalidMagic_v2`** | 系统常量 | 🟢 低 | 保持 |
| **`ContainerVersion = 2`** | Manifest JSON schema 版本号（**不是容器格式版本！**） | 🔴 **最高**——直接误导 | `ManifestSchemaVersion` |
| `SerializeToJSON_v2()` | Manifest 序列化方法 | 🟡 中（在类型上） | 随类型重命名 |
| `NewManifest_v2()` | 工厂函数 | 🟡 中 | 随类型重命名 |
| `DeserializeFromJSON_v2()` | 反序列化函数 | 🟡 中 | 随类型重命名 |
| `ExtractKVI_v2()` / `ExtractManifest_v2()` | 扫描提取函数 | 🟡 中 | 去掉 `_v2` 后缀 |
| `GenerateKey_v2()` | 密钥派生 | 🟢 低 | `GenerateKey` |
| `WriteBlock_v2()` / `ReadBlockHeader_v2()` / `ReadBlockData_v2()` | Block I/O | 🟢 低 | 去掉 `_v2` 后缀 |

---

## 一、之前的重构是否有误删/误改？

### 逐项审查

| 检查项 | 结论 | 说明 |
|--------|------|------|
| 删除 `manifest_v2.go` 中的 `readManifestV4()` | ✅ 正确 | 这是 V4 专用的读取逻辑，已迁移至 `handle.openV4()` |
| 删除 `manifest_v2.go` 中的 `adaptV4ToV2Manifest()` | ✅ 正确 | 已迁移至 `handle/adapter.go` 的 `AdaptV4ToV2()` |
| `ReadManifestFromFile()` 对 V4 返回 error | ⚠️ 需关注 | 见下方分析 |
| 删除 detector 包中的 `detectV4Container()` / `detectV3Container()` | ✅ 正确 | 逻辑已内聚到 ContainerHandle |
| 删除 `indexKindToContainerType()` 从 detector | ✅ 正确 | handle/adapter.go 中已有同名导出函数 |
| `content_verifier.go:488` 调用 `manifest.ReadManifestFromFile()` | ⚠️ **潜在回归点** | 如果传入 V4 文件会返回 error |

### ⚠️ 潜在问题：`content_verifier.go:488`

```go
mf, _, _, _, err := manifest.ReadManifestFromFile(containerPath)
```

现在 `ReadManifestFromFile` 对 V4 返回 error。需要确认：
1. content_verifier 是否可能收到 V4 容器路径？
2. 如果是，这里也需要迁移到使用 ContainerHandle

**处理方案**：将此调用也改为使用 ContainerHandle（纳入本次修复范围）。

---

## 二、重命名计划

### 原则

1. **最小改动原则**：只重命名歧义性最高的标识符（🔴 和 🟡 级别）
2. **不破坏序列化**：JSON tag 不变（`json:"fragments"` 等），只改 Go 类型名
3. **向后兼容别名**：对广泛使用的核心类型，保留旧名作为 type alias 过渡期
4. **分批执行**：先类型→再常量→最后函数

### Phase 1：核心类型去 `_v2`（🔴 高优先级）

#### 1.1 `Manifest_v2` → `Manifest`

**影响面最大**，但收益最高。这是歧义的根源。

- 文件：[types/container.go](internal/v2/types/container.go)
- 操作：
  ```go
  // 旧
  type Manifest_v2 struct { ... }
  // 新
  type Manifest struct { ... }
  
  // 兼容别名（过渡期）
  type Manifest_v2 = Manifest  // deprecated, will remove later
  ```

- 影响范围估算：~80+ 处引用（整个 `internal/v2/` + `pkg/encv/`）

#### 1.2 `Fragment_v2` → `Fragment`

同上模式：
```go
type Fragment struct { ... }    // 新
type Fragment_v2 = Fragment      // 兼容别名
type FragmentType string         // 新（去掉 _v2 后缀）
type FragmentType_v2 = FragmentType  // 兼容别名
```

#### 1.3 `KVI_v2` → `KVI`

```go
type KVI struct { ... }       // 新
type KVI_v2 = KVI             // 兼容别名
```

### Phase 2：中等优先级去 `_v2`（🟡）

#### 2.1 `ContainerVersion = 2` → `ManifestSchemaVersion`

这个常量名最危险——字面意思是"容器版本=2"，但实际是 Manifest JSON schema 的版本号。

```go
// 旧
const ContainerVersion int64 = 2
// 新
const ManifestSchemaVersion int64 = 2
// 兼容
const ContainerVersion = ManifestSchemaVersion  // deprecated alias
```

#### 2.2 函数去 `_v2`

批量操作，模式一致：
- `SerializeToJSON_v2()` → `SerializeToJSON()`
- `DeserializeFromJSON_v2()` → `DeserializeFromJSON()`
- `NewManifest_v2()` → `NewManifest()`
- `ExtractKVI_v2()` → `ExtractKVI()`
- `ExtractManifest_v2()` → `ExtractManifest()`
- `GenerateKey_v2()` → `GenerateKey()`
- `WriteBlock_v2()` → `WriteBlock()`
- `ReadBlockHeader_v2()` → `ReadBlockHeader()`
- `ReadBlockData_v2()` → `ReadBlockData()`

### Phase 3：保持不变的（🟢）

以下 `_v2` 后缀歧义低，保持不变以避免过度重构：

- `EnvelopeHeader_v2` / `EnvelopeFooter_v2` / `EnvelopeHeaderSize_v2` / `EnvelopeFooterSize_v2` — **这是真正的 V2 容器格式**
- `BlockType*_v2` 常量 — 改动收益低且容易与其他包冲突
- `SaltSize_v2` / `KeySize_v2` / `IVSize_v2` — 加密参数常量
- `ByteOrder_v2` / `ErrInvalidMagic_v2` / `MagicHeader_v2` / `MagicFooter_v2` — 系统常量

---

## 三、执行步骤

### Step 0: 修复潜在回归 — content_verifier.go

- [ ] 将 `content_verifier.go:488` 的 `manifest.ReadManifestFromFile()` 改为使用 `containerhandle.Open()`
- [ ] 验证构建通过

### Step 1: Phase 1 — 核心类型重命名（Manifest_v2 → Manifest 等）

- [ ] 在 `types/container.go` 中定义新类型名 + 兼容别名
- [ ] 全局搜索替换所有 `.Manifest_v2` 为 `.Manifest`（排除兼容别名行和注释）
- [ ] 同理替换 `Fragment_v2` → `Fragment`, `FragmentType_v2` → `FragmentType`, `KVI_v2` → `KVI`
- [ ] 更新 handle 包中的引用
- [ ] 更新 detector / reader / analyze / service 层引用
- [ ] `go build ./cmd/encv/...` 验证

### Step 2: Phase 2 — 函数和常量重命名

- [ ] `ContainerVersion` → `ManifestSchemaVersion` + 别名
- [ ] 批量函数去 `_v2` 后缀
- [ ] `go build` 验证

### Step 3: 清理兼容别名（可选，后续迭代）

- [ ] 在确认无外部依赖后删除 `type Manifest_v2 = Manifest` 等别名
- [ ] 这步可以放到后续版本，不在本次执行

### Step 4: 构建 + 测试验证

- [ ] `go build ./cmd/encv/...`
- [ ] `cd app/encv-mobile && npx vite build`
- [ ] `go vet ./...`
