# 双端 Mock 测试完善计划

## Why

当前项目有 **97 个 PASS 测试**，但存在严重的覆盖不均：

| 覆盖等级 | 包 | 现状 |
|---------|---|------|
| ✅ 良好 | crypto, handle, detector, plugins/registry, task_manager, webdav | 有 unit + 部分集成 |
| ⚠️ 仅 bench | physical, block, fragment, envelope, reader | 只有性能基准，无逻辑单测 |
| ❌ 零覆盖 | MobileService, ReaderService, ContainerManager, ContentHandler, writer 逻辑 | 核心业务路径无测试 |

同时，已有的 Mock 基础设施（`MockBroadcaster`、`ContainerHandle` 接口、`BytesSource`、`FileContentProvider` 接口）未被充分利用来编写真正的 mock 测试。

## 目标

1. 补全核心业务层的单元测试（MobileService、ReaderService、ContainerManager）
2. 为 ContentHandler、physical/block/fragment 包补全逻辑单测
3. 建立 mock 基础设施层（MockFileContentProvider、MockContainerHandle 等）
4. 所有新测试通过 `go test ./internal/... -count=1` 零失败

---

## Phase 1：建立 Mock 基础设施层

### Task 1: 创建 `internal/testutil/` mock 工具包

创建 `/workspace/internal/testutil/mock.go`，提供可复用的 mock 实现：

```
testutil/
  mock.go          — 通用 mock 构造器
  container.go     — MockContainerHandle (实现 ContainerHandle 接口)
  provider.go      — MockFileContentProvider (实现 FileContentProvider 接口)
  http.go          — HTTP 测试辅助 (httptest.ResponseRecorder 封装)
```

**MockContainerHandle** 要点：
- 实现 `containerhandle.ContainerHandle` 全部 15 个方法
- 通过构造函数注入 Version/ContainerType/IsSeekable/Manifest 等字段
- 用于测试 MobileService.GetFileInfo、detector 等依赖 ContainerHandle 的代码

**MockFileContentProvider** 要点：
- 实现 `provider.FileContentProvider` 全部 6 个方法
- 注入 io.Reader/io.Seeker/大小/文件名
- 用于测试 ContentHandler.ServeFile 的 HTTP Range 逻辑

### Task 2: 创建 container fixture 工厂（跨包复用）

在 `internal/testutil/container_fixture.go` 中提供：

```go
// CreateV4Fixture 在 t.TempDir() 中生成一个完整的 V4 容器文件
// 返回 path, password, manifest, originalData
func CreateV4Fixture(t testing.TB, dataSize int64) *ContainerFixture

// CreateV3Fixture 同上 V3 格式
func CreateV3Fixture(t testing.TB, dataSize int64) *ContainerFixture
```

复用 reader/bench_test.go 中 `createContainerFixture` 的逻辑，但提取为共享工具。

---

## Phase 2：核心业务层 Mock 测试

### Task 3: MobileService 单测 (`service/mobile_service_logic_test.go`)

**测试目标**：不依赖真实文件系统的纯逻辑路径

| 测试用例 | 覆盖方法 | Mock 方式 |
|---------|---------|----------|
| TestListFiles_RootPath | ListFiles("/") | t.TempDir() + 写入测试文件 |
| TestListFiles_NonExistent | ListFiles("/nope") | t.TempDir() 但不创建文件 |
| TestListFiles_PathTraversal | ListFiles("../../etc") | 验证返回 ForbiddenError |
| TestGetFileInfo_EmptyPath | GetFileInfo("") | 直接调用，验证 BadRequestError |
| TestGetFileInfo_NonExistent | GetFileInfo("/missing") | t.TempDir() + 不存在路径 |
| TestGetFileInfo_Directory | GetFileInfo("/dir") | t.TempDir() + 创建目录 |
| TestGetFileInfo_NormalFile | GetFileInfo("/file.txt") | t.TempDir() + 创建普通文件 |
| TestGetFileInfo_V4Container | GetFileInfo("/file.sccgv") | 使用 CreateV4Fixture |
| TestDeleteFile_Success | DeleteFile | t.TempDir() + 创建后删除 |
| TestDeleteFile_NotFound | DeleteFile("/nope") | 验证 NotFoundError |
| TestReadFileContent_TextFile | ReadFileContent | t.TempDir() + UTF-8 文本文件 |
| TestReadFileContent_Binary | ReadFileContent | t.TempDir() + 二进制文件 |
| TestCheckStoragePermission | CheckStoragePermission | t.TempDir()（一定有权限） |

**关键改造点**：MobileService 当前构造函数 `NewMobileService(servingDir, cfg)` 内部创建 TaskManager（启动 goroutine）。测试中需要：
- 用 `t.TempDir()` 作为 servingDir
- cfg 可以为 nil 或 defaultTestConfig()
- TaskManager 的 worker goroutine 不影响测试（只读操作）

### Task 4: ContainerManager 单测 (`v2/service/container_manager_test.go`)

| 测试用例 | 验证点 |
|---------|--------|
| TestGetReadablePath_ValidContainer | 有效 ENC 文件 → 返回原路径（不走重建） |
| TestGetReadablePath_InvalidContainer_CacheMiss | 损坏文件 → 触发重建 → 缓存结果 |
| TestGetReadablePath_InvalidContainer_CacheHit | 第二次查询损坏文件 → 返回缓存 |
| TestCleanup | 清理所有缓存的临时文件 |

使用 `CreateV3Fixture` / `CreateV4Fixture` 生成容器，然后截断/损坏 footer 来触发重建路径。

### Task 5: ReaderService 单测 (`v2/service/reader_service_test.go`)

| 测试用例 | 验证点 |
|---------|--------|
| TestGetDecryptReader_ValidContainer | 正确返回 factory + reader + index + size |
| TestGetDecryptReader_Cached | 相同路径第二次调用命中缓存 |
| TestGetDecryptReader_InvalidContainer | 损坏容器返回 error |
| TestGetBulkDecryptor_ValidContainer | 返回可用 BulkDecryptor |
| TestCleanup | 清理所有缓存的 factory |

依赖 ContainerManager 和 fixture 容器。

---

## Phase 3：Handler 与 Provider 层 Mock 测试

### Task 6: ContentHandler 单测 (`handler/content_handler_test.go`)

ContentHandler 是**最佳 mock 候选**——它已经依赖 `provider.FileContentProvider` 接口：

| 测试用例 | 验证点 |
|---------|--------|
| TestServeFile_FullRequest | 无 Range 头 → 200 + 完整内容 |
| TestServeFile_RangeRequest | Range: bytes=0-1023 → 206 + Content-Range |
| TestServeFile_RangeToEnd | Range: bytes=500- → 206 到末尾 |
| TestServeFile_InvalidRange | Range: bytes=999999- → 416 |
| TestServeFile_MalformedRange | Range: invalid → 忽略 Range，返回 200 |
| TestServeFile_NonSeekableWithRange | 非 seeker + 非 zero start → 416 |
| TestServeFile_SeekableRange | 有 io.Seeker → Seek 到正确位置 |

使用 `MockFileContentProvider` + `httptest.ResponseRecorder`，**零磁盘依赖**。

### Task 7: parseRangeHeader 辅助函数测试

| 测试用例 | 输入 | 期望输出 |
|---------|------|---------|
| TestParseRangeHeader_Empty | "" | 0, n-1, 200 |
| TestParseRangeHeader_Valid | "bytes=0-499", size=1000 | 0, 499, 206 |
| TestParseRangeHeader_OpenEnd | "bytes=500-", size=1000 | 0, 999, 206 |
| TestParseRangeHeader_InvalidStart | "bytes=9999-", size=1000 | 0, 999, 416 |
| TestParseRangeHeader_StartGtEnd | "bytes=800-700", size=1000 | 0, 999, 416 |
| TestParseRangeHeader_Malformed | "bytes=abc", size=1000 | 0, 999, 200 |

---

## Phase 4：底层包逻辑补全

### Task 8: block 包逻辑单测 (`container/block/block_logic_test.go`)

当前只有 `bench_test.go`（基准测试），缺少对 `WriteBlock`/`ReadBlockHeader`/`ReadBlockData` 的正确性验证：

| 测试用例 | 验证点 |
|---------|--------|
| TestWriteBlock_And_ReadBlockHeader_Roundtrip | Write 后 ReadBlockHeader 返回一致的数据 |
| TestWriteBlock_KVIType | BlockTypeKVI 的 KVI 数据完整写入/读出 |
| TestWriteBlock_ManifestType | Manifest 数据完整写入/读出 |
| TestReadBlockData_Positioning | SkipTo 正确定位到数据区 |
| TestReadBlockData_WrongType | 对非数据块调用 ReadBlockData 返回错误 |
| TestReadBlockHeader_TruncatedData | 截断数据返回 error |

### Task 9: physical 包逻辑单测 (`physical/physical_logic_test.go`)

| 测试用例 | 验证点 |
|---------|--------|
| TestSinglePhysicalPacker_PackCreatesChunks | Pack 产生正确的分片文件数 |
| TestSinglePhysicalPacker_MainChunkContainsManifest | 主分片包含 KVI + Manifest |
| TestSinglePhysicalUnpacker_UnpackReassembles | Unpack 后文件与原始一致 |
| TestFileChunkerPhysicalPacker_ChunkSize | 分片大小符合预期 |
| TestPackRequest_Validation | 缺少必填字段时的行为 |

### Task 10: fragment 包逻辑单测 (`fragment/fragment_logic_test.go`)

| 测试用例 | 验证点 |
|---------|--------|
| TestCreateLogicalFragmentsFromSize_Atomic | AtomicFile 类型产生单个 Fragment |
| TestCreateLogicalFragmentsFromSize_Seekable | SeekableStream 类型按 fragCount 分割 |
| TestCreateLogicalFragments_Offsets | GlobalStartOffset 连续递增 |
| TestValidateFragmentOffsets | 正确/错误的 offset 检测 |
| TestTotalLogicalSize | 总大小等于各 Fragment.Length 之和 |

---

## Phase 5：迭代验证

### Task 11: 全量运行 + 修复循环

```
Loop:
  go test ./internal/... -count=1 -v 2>&1
  收集 FAIL → 分析根因 → 修复 → goto Loop
until 零 failures
```

---

## 任务依赖关系

```
Task 1 (testutil 基础设施)
  ├── Task 2 (fixture 工厂) — 依赖 testutil 结构
  │     ├── Task 3 (MobileService) — 依赖 fixture
  │     ├── Task 4 (ContainerManager) — 依赖 fixture
  │     └── Task 5 (ReaderService) — 依赖 Task 4
  ├── Task 6 (ContentHandler) — 依赖 MockFileContentProvider
  ├── Task 7 (parseRangeHeader) — 无依赖，可并行
  Task 8 (block logic) — 无额外依赖
  Task 9 (physical logic) — 无额外依赖
  Task 10 (fragment logic) — 无额外依赖
  └── Task 11 (全量验证) — 最后执行
```

**Batch A (并行)**: Task 1 + Task 7 + Task 8 + Task 9 + Task 10
**Batch B (依赖 A)**: Task 2
**Batch C (依赖 B)**: Task 3 + Task 4 + Task 5 + Task 6
**Batch D (最后)**: Task 11

## 预期产出

| 指标 | 当前值 | 目标值 |
|-----|-------|-------|
| 总 PASS 数 | 97 | ~150+ |
| 新增测试文件 | 0 | ~8 个 |
| 有 unit 测试的包 | 8 | ~14 |
| Mock 基础设施 | 1 (MockBroadcaster) | 4+ (MockContainerHandle, MockFileContentProvider, fixture 工厂) |
