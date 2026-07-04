# WebDAV 认证 / V3-V4 旧值清理 / ECv4 128GB 容量边界测试

> **生成时间**：2026-06-11
> **状态**：Plan 阶段，等待批准
> **scope**：3 个独立但相关的任务，全部在 encv-mobile devtools 域内
> **不投入生产约束**：用户在前几轮已声明，当前 dev branch 没上生产 → 不需要 v1/v2/v3 向后兼容代码，可放心清理。

---

## Phase 1 探索关键发现

### A. WebDAV 认证现状

- 18 个测试用例全部走 `fetch()` 直接调 `${baseUrl}/webdav/...`（[useWebDavAutomationTests.ts:566-699](file:///workspace/app/encv-mobile/src/composables/useWebDavAutomationTests.ts#L566-L699)），**没有一处传 Authorization header**。
- 后端 [middleware/basic_auth.go:13-49](file:///workspace/internal/middleware/basic_auth.go#L13-L49) 是 `BasicAuthDynamic`：从 `cfg.Webdav.Username/Password` 读，配置了才校验。
- 后端 [mobile_api.go:449-461](file:///workspace/internal/server/mobile_api.go#L449-L461) `handleTestWebDAVGin` 接受用户传 username/password 测**远端** webdav；本地 webdav（`/webdav/`，我们的目标）当前没有暴露 `webdavUsername/webdavPassword` 字段。
- 浏览器原生行为：fetch 收到 401 且没带 Authorization → 触发 native 弹窗（用户严令禁止）。

### B. V3/V4 旧值全量扫描结果（**我检查了**）

| 位置 | 现状 | 替换目标 |
|------|------|---------|
| [useAutomationTests.ts:261](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L261) | `version <= 3` 数字比较 | `isDeprecatedVersion(version)` |
| [useAutomationTests.ts:263](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L263) | `const isV4 = version === 4` | `const isV4 = isRecommendedVersion(version)` |
| [useAutomationTests.ts:288](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L288) | `version <= 3 ? 'might-fail' : 'success'` | `isDeprecatedVersion(version) ? 'might-fail' : 'success'` |
| [useAutomationTests.ts:276](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L276) | `\`v${version}\`` 拼 test id | `\`ECv${version}\`` 拼 id |
| [AutomationTestsDetail.vue:506,530,604](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue#L506) | `\`v${version}\`` 拼 safeId / name | `\`ECv${version}\`` 拼 safeId |
| [Tasks.vue:342](file:///workspace/app/encv-mobile/src/views/Tasks.vue#L342) | `V{{ item.task.containerVersion }}` 数字 | `{{ formatContainerVersion(item.task.containerVersion) }}` |
| [TaskBasicInfo.vue:98](file:///workspace/app/encv-mobile/src/components/TaskBasicInfo.vue#L98) | `V{{ task.containerVersion }}` 数字 | `{{ formatContainerVersion(task.containerVersion) }}` |
| [EncryptBody.vue:62,98](file:///workspace/app/encv-mobile/src/components/EncryptBody.vue#L62) | `version === 4` 模板条件 | `isRecommendedVersion(version)` |
| [useAutomationTests.test.ts:53,71,73,91,93,97,100,104,108-115,118-124,150](file:///workspace/app/encv-mobile/src/composables/__tests__/useAutomationTests.test.ts#L53) | 9 处数字字面量 + `'mp4-encrypt-v4-c1-zstd'` id 字面量 | helper 替换 + 期望值改成 `ECV4` |
| [ContainerVersionSelector.vue:60-61](file:///workspace/app/encv-mobile/src/components/ContainerVersionSelector.vue#L60) | `version: 3, label: 'ECv3'` / `version: 4, label: 'ECv4'` | 改用 `CONTAINER_VERSIONS` 常量数组（来自新 constants 模块） |
| [useNewTaskModal.ts:163-164](file:///workspace/app/encv-mobile/src/composables/useNewTaskModal.ts#L163) | `v4 独有 / v2/v3` 注释 | 注释引用 `ECV4` / `isDeprecatedVersion` |
| [useErrorAnalyzer.ts:167-213](file:///workspace/app/encv-mobile/src/composables/useErrorAnalyzer.ts#L167) | 错误文案里硬编码 "v2/v3" | 文案改 "ECv3"、分支条件改 helper |
| [NewTaskState.ts:17-19](file:///workspace/app/encv-mobile/src/components/NewTaskState.ts#L17) | `v4 CipherMode` / `v4 CompressionMode` 注释 | 注释引用 `ECV4` |
| [useErrorAnalyzer.test.ts](file:///workspace/app/encv-mobile/src/composables/__tests__/useErrorAnalyzer.test.ts) | （推测）类似数字字面量 | 同上 |
| [mockDataGenerator.ts:483](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts#L483) | `ENCV v4 Container` 注释 | 注释引用 `ECV4` |
| [internal/v2/types/container.go:30-37](file:///workspace/internal/v2/types/container.go#L30) | `ContainerECv3/ContainerECv4` 已有 | 加 helper `IsRecommendedVersion(v int) bool` |

**Plugin name 字面量含 "v4"（不动）**：`useAutomationTests.test.ts:53` `makePlugin('video-v4', [4], 4)`、`WorkflowDashboard.vue:412` `plugin: ['video-v4', 'audio-v4']`、`matrixExpander.ts` 示例数据 — 这些 "v4" 是 **plugin 字符串 id**（不是容器版本），不替换。`i18n/agent.ts:252` `agent.v2Chip.searchPrompt` 的 "v2" 是 agent 架构版本（不是容器），不替换。

### C. ECv4 128GB 容量边界 — 关键工程事实

| 维度 | 现状 | 推论 |
|------|------|------|
| **V4 Header** | [header_v4.go:13-16](file:///workspace/internal/v2/types/header_v4.go#L13) `EnvelopeHeaderSize_v4=2048`，`ManifestOffset uint32` | 单 main file 上限 = 4GB（uint32） |
| **物理分片** | [plugin.go:73-240](file:///workspace/internal/v2/plugins/video/plugin.go#L73) `ContainerChunkSizeMB` 配置（默认 0=不分片，最小 30MB） | 启用分片后单 main file 不限 4GB，碎片在 `.part` 文件 |
| **物理写盘** | [file_chunker.go:181-252](file:///workspace/internal/v2/physical/file_chunker.go#L181) `os.Create()` 真写盘，无 sparse / Truncate | 改 sparse 需新增 helper |
| **V4 Footer** | [header_v4.go:87-91](file:///workspace/internal/v2/types/header_v4.go#L87) `GlobalCRC32 uint32` | CRC 是 32-bit，不限制数据量 |
| **Manifest** | [container.go:134-148](file:///workspace/internal/v2/types/container.go#L134) `Fragment.Length/Offset uint64` | 单 manifest 可描述 2^64 字节数据 |
| **物理分片读** | [file_container_reader.go:444-451](file:///workspace/internal/v2/reader/file_container_reader.go#L444) 真实读 `actualFileSize` 校验 | 读侧会检测"sparse file 实际占用 < 声明大小"，要测试这个失败路径 |

**结论**：
- **128GB 单文件上限是合理的**（开启物理分片），但需要明确 spec：单 main file ≤ 4GB（uint32 限制），单 virtual file 128GB（需物理分片）
- **100×128GB = 12.8TB virtual** 完全可描述（manifest 字节数 < 100KB），但物理写盘爆
- **正确测试策略 = sparse file + manifest-driven**：写一个 1KB 物理字节但声明 128GB 虚拟大小的 main file，外加 100 个 sparse `.part` 文件（每个 `os.Truncate(128GB)` 占用 ≈ 0 字节），manifest 描述 100 fragments
- **真机约束**：Android storage quota / iOS APFS free space 可能不足 128GB 物理；fallback 是参数化 `fragmentCount × fragmentSize`，让用户输入 ≤ 设备可用空间

---

## Phase 3 — 实施方案

### 任务 1：WebDAV 自动化测试加 Basic Auth（无弹窗）

**目标**：所有 18 个 webdav 测试用例共用一组 username/password，**禁止** native 弹窗。

**实现**：
1. 在 `useWebDavAutomationTests.ts` 顶部加常量：
   ```ts
   // 从后端 /api/server/info 或 /api/config 派生（fallback 写死）
   const WEBDAV_CREDENTIALS = {
     username: 'encv',           // 默认值，可被配置覆盖
     password: 'encv-webdav',    // 同上
   }
   ```
2. 加辅助函数 `buildAuthHeaders(): Record<string,string> | undefined`：
   - 如果 username/password 都非空 → 返回 `{ Authorization: 'Basic ' + btoa(\`${u}:${p}\`) }`
   - 否则返回 `undefined`（后端未启用 auth，透传即可）
3. 修改 `expectList / expectGetFile / expectHeadFile / expectOptions / expectPropfind / expectMkcol / expectPut / expectMove / expectCopy / expectDelete / expectStatus` 11 个 helper，**所有 fetch 调用统一注入** `...(authHeaders ? { headers: { ...authHeaders, ...existingHeaders } } : {})`
4. WebDavAutomationTestsDetail.vue 加一个"账号配置"折叠面板（开发工具用，明文显示）：
   - 2 个 ion-input：username / password
   - 1 个开关："使用内置默认账号（如果后端未启用 auth，跳过）"
   - 改了就写 localStorage `encv_webdav_creds_v1`，优先用
5. **新增**后端 endpoint `/api/webdav/local-info`：返回 `{ enabled, authRequired, defaultUsername, defaultPassword }`（dev only）。前端 `onMounted` 拉一次，存在 baseUrl state 里。
6. **关键防御**：username/password 为空时 fetch 不带 Authorization 头 → 后端 BasicAuthDynamic 看到空就跳过校验（[basic_auth.go:17-20](file:///workspace/internal/middleware/basic_auth.go#L17)）→ 不会触发 401 → 不会弹窗。

**修改文件**：
- [useWebDavAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useWebDavAutomationTests.ts) — 加 credentials + auth headers 注入
- [WebDavAutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/WebDavAutomationTestsDetail.vue) — 加配置面板
- [mobile_api.go](file:///workspace/internal/server/mobile_api.go) — 加 `handleWebDavLocalInfoGin`（仿 [handleTestLocalWebDAVGin:563-632](file:///workspace/internal/server/mobile_api.go#L563)）
- [server.go](file:///workspace/internal/server/server.go) — 注册 `r.GET("/api/webdav/local-info", s.handleWebDavLocalInfoGin)`
- [api/encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts) — `fetchWebDavLocalInfo()` 函数
- [i18n/tasks.ts](file:///workspace/app/encv-mobile/src/i18n/tasks.ts) — 加 `devtools.webdavAuth*` 中英文 key

**验证**：
- 启动后端，curl `/api/webdav/local-info` 期望返回 200 + JSON
- vite 编译产物含 `Basic ${btoa(...)}` 字符串
- 强制刷新浏览器 → 18 个用例不触发 native 弹窗
- 后端 cfg 设了 `webdav.username/password` → 用例能跑通；未设 → 用例继续能跑（不 401）

---

### 任务 2：V3/V4 旧值 → 容器版本 enum + helper（前端+后端）

**目标**：所有"硬编码 V3/V4 数字"统一来自 `constants/containerVersion.ts`，新增"已弃用"判断 helper。

**新文件** `app/encv-mobile/src/constants/containerVersion.ts`：
```ts
/**
 * ENCV 容器版本常量（v3 / v4 + 派生 helper）
 *
 * 命名沿用 internal/v2/types/container.go 的 ContainerECv3/ContainerECv4
 * 关键事实：
 * - ECV2 已在 SupportedVersions 移除，仅 detector 识别存量文件
 * - ECV3 = deprecated（仍可创建/读取）
 * - ECV4 = recommended（默认）
 */
export const ECV2 = 2 as const
export const ECV3 = 3 as const
export const ECV4 = 4 as const

export type ContainerVersion = 2 | 3 | 4

export interface ContainerVersionInfo {
  version: ContainerVersion
  status: 'deprecated' | 'recommended'
  label: string
}

/** 当前支持的版本（v3 deprecated + v4 recommended） */
export const CONTAINER_VERSIONS: readonly ContainerVersionInfo[] = [
  { version: ECV3, status: 'deprecated', label: 'ECv3' },
  { version: ECV4, status: 'recommended', label: 'ECv4' },
] as const

export const DEFAULT_CONTAINER_VERSION: ContainerVersion = ECV4

export function isDeprecatedVersion(v: number): boolean {
  return v === ECV2 || v === ECV3
}

export function isRecommendedVersion(v: number): boolean {
  return v === ECV4
}

export function formatContainerVersion(v: number | undefined | null): string {
  if (v === undefined || v === null) return ''
  return `ECv${v}`  // 'ECv3' / 'ECv4'
}

export function parseContainerVersion(label: string): ContainerVersion | null {
  const m = /^ECv([2-4])$/.exec(label)
  if (!m) return null
  return Number(m[1]) as ContainerVersion
}
```

**后端镜像** `internal/v2/types/container.go` 末尾加：
```go
// IsRecommendedVersion v4 才是 recommended，其他都按 deprecated 处理
func IsRecommendedVersion(v int) bool { return v == ContainerECv4 }

// IsDeprecatedVersion v2/v3 都算 deprecated（v2 已从 SupportedVersions 移除，
// 但语义上 v2 也算 deprecated 便于外部判断）
func IsDeprecatedVersion(v int) bool { return v == ContainerECv3 || v == ContainerECv2 }
```

**替换映射**（按文件）：
| 文件 | 替换内容 |
|------|---------|
| [ContainerVersionSelector.vue:59-62](file:///workspace/app/encv-mobile/src/components/ContainerVersionSelector.vue#L59) | `defaultVersions` 删，改 `const versions = computed(() => props.versions ?? CONTAINER_VERSIONS)` |
| [useAutomationTests.ts:11-12,243,259,261,263,264,276,288](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L11) | 全部数值/字符串替换为 helper / 字符串模板前缀改 `ECv` |
| [AutomationTestsDetail.vue:423,506,530,604](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue#L423) | 同上 + 注释 |
| [Tasks.vue:342](file:///workspace/app/encv-mobile/src/views/Tasks.vue#L342) | `V{{...}}` → `{{ formatContainerVersion(...) }}` |
| [TaskBasicInfo.vue:98](file:///workspace/app/encv-mobile/src/components/TaskBasicInfo.vue#L98) | 同上 |
| [EncryptBody.vue:62,98](file:///workspace/app/encv-mobile/src/components/EncryptBody.vue#L62) | 模板条件改 `isRecommendedVersion` |
| [useErrorAnalyzer.ts:167-213](file:///workspace/app/encv-mobile/src/composables/useErrorAnalyzer.ts#L167) | 分支条件改 helper + 文案统一 "ECv3 已弃用" |
| [NewTaskState.ts:17-19](file:///workspace/app/encv-mobile/src/components/NewTaskState.ts#L17) | 注释 |
| [useNewTaskModal.ts:163-164](file:///workspace/app/encv-mobile/src/composables/useNewTaskModal.ts#L163) | 注释 |
| [mockDataGenerator.ts:483](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts#L483) | 注释 |
| [useAutomationTests.test.ts](file:///workspace/app/encv-mobile/src/composables/__tests__/useAutomationTests.test.ts) | 期望值改用 helper，'mp4-encrypt-v4-c1-zstd' → 'mp4-encrypt-ECv4-c1-zstd' |

**i18n 联动**：`tasks.containerVersion` 字段在 [i18n/tasks.ts:123](file:///workspace/app/encv-mobile/src/i18n/tasks.ts#L123) 保持 "容器版本" / "Container Version"，前端读 `formatContainerVersion` 拼出 "ECv4"。

**验证**：
- 单元测试 `useAutomationTests.test.ts` 改完后 `pnpm test` 全部通过
- 视觉验证：任务卡片显示从 "V4" 变 "ECv4"
- API 验证：传 `containerVersion: 4` 给后端 → 后端能识别为 ECV4
- grep 整个 src 目录 `version\s*[<>=!]+\s*[2-4]` → 0 命中
- grep 整个 src 目录 `\`v\$\{version\}\`` → 0 命中
- grep 整个 src 目录 `V\{\{\s*.*[Cc]ontainer[Vv]ersion` → 0 命中

---

### 任务 3：ECv4 128GB / 100×128GB sparse 虚拟容器容量边界测试

**目标**：在不动 12.8TB 物理盘的前提下，验证 ECv4 metadata 系统能正确描述 12.8TB 虚拟碎片 + sparse 物理分配不会导致容器崩溃。

**核心策略（用户严令的"避免容器崩溃"）**：
- **不写 12.8TB 真数据**。只写 V4 Header（2048B）+ EncryptedManifest（< 100KB），外加 100 个 sparse `.part` 文件（每个 `os.Truncate(128GB)` → OS-level sparse，≈ 0 物理占用）
- **不读 12.8TB 真数据**。读侧只测试"声明 128GB fragment 但磁盘无数据 → 优雅返回 error"
- **不缓存 12.8TB 在内存**。Manifest 解析时总在流式读 fragment header，不预读数据
- **真机差异**：Android `mmap` / `fseek` 128GB 文件可能因 ulimit -l 失败；iOS APFS 没问题但要申请 full storage 权限。前端 UI 暴露参数让用户根据设备能力调

**3a. 后端 — 新建 sparse 容器写入器**

新文件 `internal/v2/testutil/sparse_container.go`（testutil 而非 v2 内部，避免污染生产代码）：
```go
package testutil

import (
    "fmt"
    "os"
    "github.com/Soltus/encv-go/internal/v2/types"
)

type SparseContainerConfig struct {
    OutputDir       string
    BaseName        string
    FragmentCount   int   // 默认 100
    FragmentSize    int64 // 默认 128GB = 128 * 1024^3
    PhysicalChunkMB int   // 默认 0（单 main file）
    CipherMode      uint16 // 0=AES-128, 1=AES-256
    PasswordHint    [16]byte
}

// WriteSparseVirtualContainer 写出"声明大尺寸但物理 sparse"的 ECv4 容器
//
// 关键设计：
// 1. 只写 1 个真实 main file（Header + Manifest），不写真实 fragment data
// 2. 100 个 fragment 在 manifest 里描述，每个 128GB virtual
// 3. main file 大小被 f.Truncate(HeaderSize + ManifestSize) → sparse 占用 ≈ 0
// 4. **不创建 .part 文件**（如果 PhysicalChunkMB==0）
// 5. 返回 manifest / main file size / 实际物理占用（用于断言）
func WriteSparseVirtualContainer(cfg SparseContainerConfig) (SparseResult, error)

// SparseResult 报告"声称 vs 实际"用于断言
type SparseResult struct {
    VirtualTotal int64    // 100 * 128GB = 12.8TB
    PhysicalMain int64    // os.Stat 实际 main file size
    PhysicalUsed int64    // du/blocks 实际占用（main + .part）
    ManifestSize int64
    FragmentCount int
    // 是否真正 sparse（>10x virtual/physical ratio 算成功）
    IsSparse bool
}
```

**3b. 后端 — 新建 reader 边界探测**

同文件加：
```go
// ReadSparseContainerEdgeProbe 模拟"读 128GB fragment 之一"
// 关键设计：
// 1. 只 open 1 个 fragment（不开全部 100 个）
// 2. 读 4KB header 验证 magic
// 3. seek 到 fragment 物理 offset (128GB 处) 探测 sparse 区域
// 4. 读 1KB（不读全部 128GB）
// 5. 测耗时 + 内存峰值
func ReadSparseContainerEdgeProbe(mainPath string, fragmentIdx int) (EdgeProbeResult, error)
```

**3c. 后端 — HTTP endpoint**

[mobile_api.go](file:///workspace/internal/server/mobile_api.go) 加：
- `POST /api/dev/sparse-container` 接受 `{ fragmentCount, fragmentSize, physicalChunkMB }` → 写 sparse 容器 → 返回 `SparseResult`
- `GET  /api/dev/sparse-container/probe?mainPath=...&fragmentIdx=N` → 读 1 个 fragment 边界
- `DELETE /api/dev/sparse-container?baseName=...` → 清理测试产物

[server.go](file:///workspace/internal/server/server.go) 注册 3 个路由（dev 模式才挂载）。

**3d. 前端 — 加到 DevToolsDetail.vue**

`webdav 自动化测试` 下面加第 2 个 entry："ECv4 容量边界测试"。

新文件 `app/encv-mobile/src/views/SparseContainerTestDetail.vue`：
- 3 个 ion-input：fragmentCount (默认 100) / fragmentSize GB (默认 128) / physicalChunkMB (默认 0)
- 1 个按钮："写入 sparse 容器"
- 1 个按钮："探测 1 个 fragment 边界"
- 1 个按钮："清理产物"
- 实时显示：声称 total / 物理 main file size / sparse 压缩比 / 读 fragment 耗时 / 内存峰值
- 真机约束检测：调 `navigator.storage.estimate()` 拿可用配额，不够 128GB 时弹警告 + 建议降级

**3e. 关键防崩溃设计**（用户严令）

| 风险点 | 防御 |
|--------|------|
| Manifest 序列化 OOM | 100 fragments 描述总 < 50KB，JSON.Marshal 后 < 100KB，正常 |
| 读 fragment 一次性 mmap 128GB | **禁止 mmap**。用 `os.Open` + `f.Seek` + `f.Read(buf[:4096])` 1 次 4KB |
| OS sparse 创建失败（Android quota） | 探测 `f.Truncate` error → 返 4xx 给前端 → 前端弹降级建议 |
| main file `f.Truncate(128GB)` 实际写盘 | Linux 默认 sparse 不会写盘（ftruncate 不预分配 blocks）；`posix_fallocate` 会强制预分配 → 选 `f.Truncate` 不用 `posix_fallocate` |
| 真机：用户选 100×128GB 但设备只 64GB free | 前端 `navigator.storage.estimate()` 拦截 + 提示降级 |
| 100 个 .part 文件创建（如果 PhysicalChunkMB>0） | 默认 PhysicalChunkMB=0（不创建 .part），如用户填了则提示"将创建 100 个 sparse .part 文件，每个 128GB" |
| 多个测试运行遗留 | 每次运行前 `DELETE /api/dev/sparse-container?baseName=...` 清理 |
| 并发写同一 baseName | 加 file lock（`flock(LOCK_EX)` on OutputDir/.lock） |

**修改/新增文件**：
- 新建 [internal/v2/testutil/sparse_container.go](file:///workspace/internal/v2/testutil/sparse_container.go) — sparse 写/读 helper
- 新建 [internal/v2/testutil/sparse_container_test.go](file:///workspace/internal/v2/testutil/sparse_container_test.go) — 单元测试（写 5×5GB 跑通后扩到 100×128GB）
- [mobile_api.go](file:///workspace/internal/server/mobile_api.go) — 加 3 个 handler
- [server.go](file:///workspace/internal/server/server.go) — 注册路由
- 新建 [app/encv-mobile/src/views/SparseContainerTestDetail.vue](file:///workspace/app/encv-mobile/src/views/SparseContainerTestDetail.vue) — UI
- [DevToolsDetail.vue](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue) — 加 entry
- [router/index.ts](file:///workspace/app/encv-mobile/src/router/index.ts) — 加路由
- 新建 [app/encv-mobile/src/api/sparseContainer.ts](file:///workspace/app/encv-mobile/src/api/sparseContainer.ts) — API client
- [i18n/tasks.ts](file:///workspace/app/encv-mobile/src/i18n/tasks.ts) — 加中英文 key

**验证**：
- 单元测试：先跑 `5×5GB`（disk 不爆）→ 通过；再跑 `100×1GB`（10GB 总量）→ 通过；最后跑 `100×128GB` sparse（< 1MB 物理）→ 通过
- 端到端：devtools 入口 → 写入 100×128GB → 实际物理占用 < 1MB（`ls -la` + `du -h`）
- 读 fragment：< 200ms 完成（只读 1KB）
- 内存峰值：< 50MB（用 `runtime.ReadMemStats` 打点）
- 真机：浏览器打开 → 选 100×128GB → navigator.storage.estimate 触发降级建议
- 清理：跑完后 `ls` 验证 output dir 干净

---

## 执行顺序与依赖

```
任务 2 (V3/V4 清理)      ──┐
                          ├── 任务 1 (WebDAV auth) 依赖任务 2 提供的 formatContainerVersion
任务 1 (WebDAV auth)     ──┘
                          │
                          └─→ 任务 3 (128GB sparse) 独立，可并行

建议执行顺序：任务 2 → 任务 1 → 任务 3
```

## 总览：修改/新建文件清单

### 修改
- [app/encv-mobile/src/composables/useWebDavAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useWebDavAutomationTests.ts)
- [app/encv-mobile/src/views/WebDavAutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/WebDavAutomationTestsDetail.vue)
- [app/encv-mobile/src/views/DevToolsDetail.vue](file:///workspace/app/encv-mobile/src/views/DevToolsDetail.vue)
- [app/encv-mobile/src/router/index.ts](file:///workspace/app/encv-mobile/src/router/index.ts)
- [app/encv-mobile/src/components/ContainerVersionSelector.vue](file:///workspace/app/encv-mobile/src/components/ContainerVersionSelector.vue)
- [app/encv-mobile/src/components/TaskBasicInfo.vue](file:///workspace/app/encv-mobile/src/components/TaskBasicInfo.vue)
- [app/encv-mobile/src/components/EncryptBody.vue](file:///workspace/app/encv-mobile/src/components/EncryptBody.vue)
- [app/encv-mobile/src/composables/useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts)
- [app/encv-mobile/src/composables/useNewTaskModal.ts](file:///workspace/app/encv-mobile/src/composables/useNewTaskModal.ts)
- [app/encv-mobile/src/composables/useErrorAnalyzer.ts](file:///workspace/app/encv-mobile/src/composables/useErrorAnalyzer.ts)
- [app/encv-mobile/src/composables/__tests__/useAutomationTests.test.ts](file:///workspace/app/encv-mobile/src/composables/__tests__/useAutomationTests.test.ts)
- [app/encv-mobile/src/views/AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue)
- [app/encv-mobile/src/views/Tasks.vue](file:///workspace/app/encv-mobile/src/views/Tasks.vue)
- [app/encv-mobile/src/lib/mockDataGenerator.ts](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts)
- [app/encv-mobile/src/components/NewTaskState.ts](file:///workspace/app/encv-mobile/src/components/NewTaskState.ts)
- [app/encv-mobile/src/i18n/tasks.ts](file:///workspace/app/encv-mobile/src/i18n/tasks.ts)
- [app/encv-mobile/src/api/encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts)
- [internal/server/mobile_api.go](file:///workspace/internal/server/mobile_api.go)
- [internal/server/server.go](file:///workspace/internal/server/server.go)
- [internal/v2/types/container.go](file:///workspace/internal/v2/types/container.go)

### 新建
- [app/encv-mobile/src/constants/containerVersion.ts](file:///workspace/app/encv-mobile/src/constants/containerVersion.ts)
- [app/encv-mobile/src/views/SparseContainerTestDetail.vue](file:///workspace/app/encv-mobile/src/views/SparseContainerTestDetail.vue)
- [app/encv-mobile/src/api/sparseContainer.ts](file:///workspace/app/encv-mobile/src/api/sparseContainer.ts)
- [internal/v2/testutil/sparse_container.go](file:///workspace/internal/v2/testutil/sparse_container.go)
- [internal/v2/testutil/sparse_container_test.go](file:///workspace/internal/v2/testutil/sparse_container_test.go)

---

## Assumptions & Decisions

1. **128GB 单文件上限合理**：开启物理分片（`ContainerChunkSizeMB` ≥ 30）后，main file 受 uint32 限制为 4GB，但分片在 `.part` 文件无此限制 → 单 virtual file 128GB 可行。
2. **Sparse file 在所有目标平台可用**：Linux ext4、macOS APFS、Windows NTFS、Android ext4/f2fs、iOS APFS 都支持 `f.Truncate` 实现的 sparse file。如有平台不支持，会 fallback 到真分配（带警告）。
3. **真机测试不强制**：UI 提供参数让用户调，**不**在 encv-mobile 启动时强制跑这个测试（避免移动端用户每次启动都跑 12.8TB 探测）。
4. **后端 helper 不影响生产**：`internal/v2/testutil/` 目录是开发辅助，不参与生产构建路径（`go build` 主包不引用 testutil）。**新增 endpoint 都挂 `/api/dev/` 前缀**。
5. **现有 webdav 默认无 auth**：如果后端 `webdav.username/password` 为空，BasicAuthDynamic 透传 → 前端 18 个用例不带 Authorization 也能跑通（向后兼容）。
6. **不引入新的 webdav 端点依赖**：任务 1 加的 `/api/webdav/local-info` 是只读（GET），不引入循环依赖。
7. **V3/V4 字符串模板替换影响 safeId/test id 格式**：从 `v${version}` 改成 `ECv${version}` 会让 `useAutomationTests.test.ts:150` 的期望值 `'mp4-encrypt-v4-c1-zstd'` 变 `'mp4-encrypt-ECv4-c1-zstd'`，测试需要同步改。
