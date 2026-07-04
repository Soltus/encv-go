# Tasks

## Phase 1：修复所有编译失败的测试文件（同步 _v2 重命名）

- [x] Task 1: 修复 `crypto/aes_v2_test.go` — 更新 `GenerateKey_v2`→`GenerateKey`, `GenerateSalt_v2`→`GenerateSalt`, `GenerateIV_v2`→`GenerateIV`, `EncryptStream_v2`→`EncryptStream`, `DecryptStream_v2`→`DecryptStream`
- [x] Task 2: 修复 `crypto/aes_v2_bench_test.go` — 更新 `GenerateKey_v2`→`GenerateKey` (6处)
- [x] Task 3: 修复 `container/manifest/bench_test.go` — 更新 `Manifest_v2`→`Manifest`, `Fragment_v2`→`Fragment`, `KVI_v2`→`KVI`, `SerializeToJSON_v2`→`SerializeToJSON`, `DeserializeFromJSON_v2`→`DeserializeFromJSON`, `ContainerVersion`→`ManifestSchemaVersion`, `FragmentType_v2`→`FragmentType`
- [x] Task 4: 修复 `container/block/block_v2_test.go` — 取消注释 + 更新函数名 `WriteBlock_v2`→`WriteBlock`, `ReadBlockHeader_v2`→`ReadBlockHeader`, `ReadBlockData_v2`→`ReadDataBlock`, 常量 `BlockTypeKVI_v2`→`BlockTypeKVI` 等
- [x] Task 5: 修复 `physical/bench_test.go` — 更新所有 `_v2` 引用（GenerateKey, GenerateSalt, GenerateIV, EncryptBytes, NewManifest, Fragment, KVI, FragmentType）
- [x] Task 6: 修复 `reader/bench_test.go` — 同上，大量 `_v2` 引用更新
- [x] Task 7: 修复 `writer/container_writer_v4_test.go` — 更新 `GenerateKey_v2`→`GenerateKey` (6处), `IVSize_v2`→`IVSize`
- [x] Task 8: 修复 `container/detector/bench_test.go` — 更新 `KVI_v2`→`KVI`, `Manifest_v2`→`Manifest`, `Fragment_v2`→`Fragment`

### 验证点
- [x] `go test ./internal/v2/crypto/... -count=1` 编译通过
- [x] `go test ./internal/v2/container/block/... -count=1` 编译通过
- [x] `go test ./internal/v2/container/manifest/... -count=1` 编译通过
- [x] `go test ./internal/v2/container/detector/... -count=1` 编译通过
- [x] `go test ./internal/v2/physical/... -count=1` 编译通过
- [x] `go test ./internal/v2/reader/... -count=1` 编译通过
- [x] `go test ./internal/v2/writer/... -count=1` 编译通过

## Phase 2：建立测试基础设施 + 补全核心包单测

- [x] Task 9: 创建测试基础设施（内联到各测试包中）
  - [x] handle 包 BytesSource 测试辅助（利用已有 handle.BytesSource）
  - [x] V4/V3/V2 容器字节构造函数（内联到 handle_test.go）
  - [x] KVI Provider test fixture 工厂（在各 bench_test.go 的 TestMain 中）

- [x] Task 10: 为 `v2/container/handle` 包编写单测 (`handle_test.go`) — **6 个测试全部 PASS**
  - [x] TestOpen_V4_ValidContainer — Version()==4, HeaderV4!=nil, FooterV4!=nil, Manifest!=nil
  - [x] TestOpen_V3_ValidContainer — Version()==3, FooterV2!=nil, Manifest!=nil
  - [x] TestOpen_InvalidMagic — 返回含 "not an ENCV container" 的 error
  - [x] TestOpen_TruncatedData — 截断数据返回 error
  - [x] TestOpen_V4_BadHeaderCRC32 — CRC32 不匹配返回 error
  - [x] TestOpen_V4_BadFooterMagic — Footer magic 错误返回 error
  - [x] TestAdaptV4ToV2_Mapping — Segment→Fragment 转换正确

- [x] Task 11: 为 `v2/container/detector` 包编写单测 (`detector_test.go`) — **11 个测试全部 PASS**
  - [x] TestIsEncvContainerFromBytes_V4Magic → true
  - [x] TestIsEncvContainerFromBytes_V3Magic → true
  - [x] TestIsEncvContainerFromBytes_V2Magic → true
  - [x] TestIsEncvContainerFromBytes_BadMagic → false
  - [x] TestIsEncvContainerFromBytes_ShortData → false
  - [x] TestIsEncvContainerFromBytes_Nil → false
  - [x] TestDetectContainerType_NonExistentPath → error
  - [x] TestDetectContainerType_NonContainerFile → error
  - [x] TestDetectIndexKind_ValidFile → "video"
  - [x] TestDetectIndexKind_NonExistent → error
  - [x] TestDetectIndexKind_NonEncvFile → error

- [x] Task 12: 为 `v2/crypto` 包补全测试 (aes_v2_test.go) — **9 个新增测试全部 PASS**
  - [x] TestGenerateSalt_v2_Size16 / Size32 / ZeroSize / Deterministic
  - [x] TestGenerateIV_v2_CorrectSize / ZeroSize
  - [x] TestBase64Encode_v2_Roundtrip / Empty
  - [x] TestEncryptBytes_v2_DecryptBytes_v2_Roundtrip

- [x] Task 13: 为插件系统编写关键路径测试 (`registry_test.go`) — **8 个测试全部 PASS**
  - [x] TestBuildFullPluginSettings_NilInput → 默认配置
  - [x] TestBuildFullPluginSettings_WithUserSettings → 合并用户值
  - [x] TestBuildFullPluginSettings_EmptyUserSettings → 等同 nil
  - [x] TestFindEncryptingPlugin_ByExtension_TXT → text 插件
  - [x] TestFindEncryptingPlugin_ByExtension_GO → text 插件
  - [x] TestFindEncryptingPlugin_NoMatch → error
  - [x] TestTextPlugin_Initialize_WithSettings → 成功加载自定义 ext
  - [x] TestTextPlugin_Initialize_NoSettings → "no settings found" 错误

## Phase 3：改造反测试范式

- [x] Task 14: 将 bench_test.go 中的 `init()` KVI 注册改为 `TestMain()` 模式
  - 涉及文件: `reader/bench_test.go`, `physical/bench_test.go`, `detector/bench_test.go`
  - 模式: `init(){ ... }` → `TestMain(m){ ...; m.Run() }`
  - 额外修复: `seek_regression_test.go` 也依赖该注册（已确认无需额外改动，TestMain 覆盖整个包）

- [x] Task 15: 修复 TaskManager 测试中的 "no such file or directory" 警告
  - [x] `newTestTaskManager` 添加 variadic `servingDir` 参数（向后兼容）
  - [x] `TestTaskManager_WorkerLifecycle` 使用 `t.TempDir()` 替代硬编码 `/tmp/test-serving`

## Phase 4：迭代验证

- [x] Task 16: 全量运行 `go test ./internal/... -count=1`
  - [x] 零 build failure ✅
  - [x] 零 FAIL ✅
  - [x] **97 个 PASS** 测试（含新增的 handle 6 + detector 11 + crypto 9 + plugins 8 = 34 个新测试）
  - [x] 中途发现问题并修复：`seek_regression_test.go` 依赖 KVI 注册 → 改用 TestMain 统一注册避免重复注册 panic

# Task Dependencies

- Task 1-8 可**完全并行**（各自独立文件的搜索替换） ✅
- Task 9 依赖 Task 1-8 完成（需要编译通过才能写新测试） ✅
- Task 10-13 可并行（都依赖 Task 9） ✅
- Task 14 可与 10-13 并行 ✅
- Task 15 可独立进行 ✅
- Task 16 是最终验证，依赖所有前置任务 ✅
