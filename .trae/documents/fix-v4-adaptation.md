# V4 容器适配全面排查与修复计划

## 背景

项目已实现 V4 容器的**写入端**（所有插件已设置 `HeaderVersion: 4`，`WriteV4Container`、物理打包器均支持 V4），但**读取端**存在大量代码路径仍使用 V2/V3 格式假设，导致 V4 容器在解密、预览、分析等场景下失败。

## V3 vs V4 关键差异

| 维度 | V2/V3 | V4 |
|------|-------|-----|
| Header 大小 | 2048B (`EnvelopeHeaderSize_v3`) | 2048B (`EnvelopeHeaderSize_v4`) |
| Footer 大小 | **32B** (`EnvelopeFooterSize_v2`) | **12B** (`EnvelopeFooterSize_v4`) |
| Footer 字段 | Magic(4) + ManifestOffset(8) + ManifestLength(8) + ManifestCRC32(4) + GlobalCRC32(4) + Reserved(4) | Magic(4) + GlobalCRC32(4) + Reserved(4) |
| Manifest 存储 | Block 结构（BlockHeader_v2 + 加密数据）+ Footer 指向偏移量 | 扁平 JSON（Obfuscate 后直接写入）+ Header 存 ManifestOffset/Length |
| 数据组织 | Fragment_v2 + BlockHeader_v2 前缀 | Segment_v4 + SegmentHeader(18B) 前缀 |
| 读取入口 | `NewEncryptedContainerReaderFromFile` → Manifest_v2 | `OpenV4Container` → Manifest_v4 |

---

## 问题清单（按严重程度排序）

### 🔴 P0 — 必须修复（V4 容器在这些路径完全无法工作）

#### 问题 1：`ReadManifestFromFile` 对 V4 使用错误的 Footer 格式

- **文件**: [manifest_v2.go:L182-L220](internal/v2/container/manifest/manifest_v2.go#L182-L220)
- **现象**: V4 容器解密时 `readManifestWithFallback` 调用此函数 → `ReadEnvelopeFooter_v2` seek 到文件末尾 `-32` 字节 → V4 footer 只有 12B → 读到错误数据 → footer magic 不匹配或 manifest offset 错误 → **解密完全失败**
- **根因**: 函数签名返回 `*types.EnvelopeFooter_v2`，内部无条件调用 `envelope.ReadEnvelopeFooter_v2(file)`，不区分版本
- **修复方案**: 在函数开头检测 `headerVersion == 4` 时走 V4 路径：
  1. Seek 到 `-EnvelopeFooterSize_v4` (12B)
  2. 用 `types.ReadFooterV4` 读取
  3. 从 V4 Header 的 `ManifestOffset`/`ManifestLength` 读取 manifest（而非 Footer）
  4. 用 `crypto.DeobfuscateManifest` 解码（而非 `readAndDecryptManifest` 的 Block 方式）
  5. 用 `types.DeserializeManifest_v4` 反序列化 → 需要适配返回类型或新增 V4 专用函数

#### 问题 2：远程容器读取器 `GetManifest()` 硬编码 `-32` Footer

- **文件**: [remote_container_reader.go:L298-L363](internal/v2/reader/remote_container_reader.go#L298-L363)
- **现象**: OpenList 代理预览 V4 远程容器时失败（footer range 错误 + 解析错误）
- **根因**:
  - L305: `GetRemoteStreamWithRange(..., -32, -1)` — V4 footer 只有 12B
  - L319: `ParseEnvelopeFooterFromBytes(footerData)` — 用 V2 格式解析 V4 数据
  - 整个 GetManifest 流程基于 V2/V3 的 Footer→ManifestOffset→Block 读取链路
- **修复方案**: 在 `initHeaderSize()` 已检测到版本的基础上，`GetManifest()` 根据 `headerSize` 判断版本：
  - 如果 `headerSize == EnvelopeHeaderSize_v4`（或缓存 version==4）：走 V4 路径
    1. 读最后 12B footer → `ReadFooterV4`
    2. 读前 2048B header → `ReadHeaderV4` 得到 ManifestOffset/ManifestLength
    3. Range 请求获取 manifest 数据 → `DeobfuscateManifest` → `DeserializeManifest_v4`
  - 注意：远程 reader 当前返回 `*types.Manifest_v2`，V4 需要 `*types.Manifest_v4` — 这是接口兼容性问题

#### 问题 3：`ensureChunkScanned` 缺少 V4 Header 处理

- **文件**: [file_container_reader.go:L411-L416](internal/v2/reader/file_container_reader.go#L411-L416)
- **代码**:
  ```go
  headerSize := int64(0)
  if r.headerVersion == 3 {
      headerSize = types.EnvelopeHeaderSize_v3
  }
  ```
- **现象**: V4 分片容器的 chunk 文件扫描时 `headerSize=0` → 不跳过 header → 所有 block 偏移量差 2048 字节 → **CRC 校验全部失败** → `corruption detected`
- **修复**: 添加 `else if r.headerVersion == 4 { headerSize = types.EnvelopeHeaderSize_v4 }`

#### 问题 4：`findAndOpenFragmentRecovery` 缺少 V4 Header 处理

- **文件**: [file_container_reader.go:L514-L517](internal/v2/reader/file_container_reader.go#L514-L517)
- **代码**:
  ```go
  if r.headerVersion == 3 {
      chunkHeaderOffset = types.EnvelopeHeaderSize_v3
  }
  ```
- **现象**: V4 容器的 recovery 模式同样因 header 未跳过而失败
- **修复**: 同问题 3，添加 V4 分支

### 🟠 P1 — 应当修复（功能缺失或不正确）

#### 问题 5：`analyzeHeader` 缺少 V4 case

- **文件**: [analyze.go:L181-L236](internal/v2/container/detector/analyze.go#L181-L236)
- **代码**:
  ```go
  switch version {
  case 3:  // ... V3 handling
  case 2:  // ... V2 handling
  }
  return 0, fmt.Errorf("unknown header version")
  ```
- **现象**: `encv analyze` 命令对 V4 容器报 "unknown header version"
- **修复**: 添加 `case 4:` 分支，调用 `types.ReadHeaderV4` 并输出 V4 特有字段（ContainerType, IsSeekable, ManifestOffset, ManifestLength）

#### 问题 6：`AnalyzeContainer` Footer 分析只支持 V2/V3

- **文件**: [analyze.go:L66-L78](internal/v2/container/detector/analyze.go#L66-L78)
- **现象**: V4 容器分析报告中 Footer 段显示 FAILED
- **修复**: 根据检测到的版本选择 `ReadEnvelopeFooter_v2` 或 `ReadFooterV4`

#### 问题 7：`performCrossValidation` 只接受 V2 Footer 类型

- **文件**: [analyze.go:L404](internal/v2/container/detector/analyze.go#L404)
- **现象**: V4 容器交叉校验不可用（Footer 字段不同：V4 无 ManifestOffset/ManifestLength/ManifestCRC32，只有 GlobalCRC32）
- **修复**: 为 V4 实现专用的交叉校验逻辑（验证 Header CRC32 + Footer GlobalCRC32 + Manifest 完整性）

### 🟢 P2 — 改进项（不影响核心功能）

#### 问题 8：移动端 `GetFileInfo` V4 回退路径信息不足

- **文件**: [mobile_service.go:L339-L349](internal/service/mobile_service.go#L339-L349)
- **现状**: `OpenV4Container` 失败后回退到 V3 路径，但错误信息不明确
- **改进**: 记录更详细的日志说明为何 V4 打开失败

---

## 架构性注意事项

### 核心矛盾：Manifest_v2 vs Manifest_v4

当前代码库存在两套并行的容器数据模型：

| 组件 | V3 路径 | V4 路径 |
|------|---------|---------|
| Manifest 类型 | `Manifest_v2` (Fragments[] + Index) | `Manifest_v4` (Segments[] + Playlists) |
| Reader 入口 | `NewEncryptedContainerReaderFromFile` | `OpenV4Container` |
| DecryptReaderFactory | 本地工厂（基于 Manifest_v2） | 无（segment_reader 是独立的） |
| 远程 Reader | `remoteEncryptedContainerReader`（基于 Manifest_v2） | 无 |
| 插件 Decrypt | 通过 factory → Sequential/SeekableDecryptReader | 通过 `OpenV4Container` → SegmentSeekableReader |

**关键问题**: 问题 1 和 2 的修复需要决定如何统一这两套模型。有两个方向：

**方向 A（推荐）：让 `ReadManifestFromFile` / `remote GetManifest` 支持 V4 并返回统一适配层**
- 新增内部适配函数，将 `Manifest_v4` 转换为 `Manifest_v2` 兼容结构（或定义接口）
- 最小改动现有调用链
- 风险：V4 的 Segment 模型和 V3 的 Fragment 模型语义差异较大，转换可能丢失信息

**方向 B：在调用链早期分支，V4 走专用路径**
- `NewEncryptedContainerReaderFromFile` 内部检测版本，V4 委托给 `OpenV4Container` 逻辑
- `RemoteDecryptReaderFactory` 同理
- 更干净但改动面更大

---

## 修复执行顺序

### Step 1: 修复 P0-3 和 P0-4（`ensureChunkScanned` + `findAndOpenFragmentRecovery`）
- 文件：`file_container_reader.go`
- 改动小，风险低，立即解决 V4 分片容器的解密崩溃

### Step 2: 修复 P0-1（`ReadManifestFromFile` V4 适配）
- 文件：`manifest/manifest_v2.go`
- 核心难点：需处理 Manifest_v4 vs Manifest_v2 类型差异
- 可能需要在 `readManifestWithFallback` 中增加 V4 分支，直接使用 V4 读取逻辑

### Step 3: 修复 P0-2（远程 Reader V4 适配）
- 文件：`remote_container_reader.go`
- 依赖 Step 2 的类型决策

### Step 4: 修复 P1-5/6/7（analyze 工具 V4 支持）
- 文件：`detector/analyze.go`
- 独立工具，不影响核心流程

### Step 5: 端到端验证
- 创建 V4 测试容器
- 验证本地解密、远程预览（OpenList）、分片容器、analyze 工具
