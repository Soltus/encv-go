# 测试修复与补全 Checklist

## Phase 1: 修复编译失败
- [x] `crypto/aes_v2_test.go`: 所有 `_v2` 函数引用已更新为新名
- [x] `crypto/aes_v2_bench_test.go`: `GenerateKey_v2` → `GenerateKey` (6处)
- [x] `container/manifest/bench_test.go`: Manifest/Fragment/KVI/SerializeToJSON/DeserializeFromJSON/ContainerVersion/FragmentType 全部更新
- [x] `container/block/block_v2_test.go`: 取消注释 + WriteBlock/ReadBlockHeader/ReadBlockData/常量更新
- [x] `physical/bench_test.go`: 所有 _v2 引用更新
- [x] `reader/bench_test.go`: 所有 _v2 引用更新
- [x] `writer/container_writer_v4_test.go`: GenerateKey + IVSize 更新
- [x] `container/detector/bench_test.go`: KVI/Manifest/Fragment 更新
- [x] `go test ./internal/v2/crypto/...` 编译通过
- [x] `go test ./internal/v2/container/block/...` 编译通过
- [x] `go test ./internal/v2/container/manifest/...` 编译通过
- [x] `go test ./internal/v2/container/detector/...` 编译通过
- [x] `go test ./internal/v2/physical/...` 编译通过
- [x] `go test ./internal/v2/reader/...` 编译通过
- [x] `go test ./internal/v2/writer/...` 编译通过

## Phase 2: 测试基础设施 + 新增测试
- [x] `handle_test.go` 存在且编译通过 — **6 个测试**
- [x] TestOpen_V4_ValidContainer: Version()==4, HeaderV4!=nil, FooterV4!=nil, ManifestV4!=nil, Manifest!=nil
- [x] TestOpen_V3_ValidContainer: Version()==3, FooterV2!=nil, Manifest!=nil
- [x] TestOpen_InvalidMagic: 返回 error 含 "not an ENCV container"
- [x] TestOpen_TruncatedData: 返回 error
- [x] TestOpen_V4_BadHeaderCRC32: CRC32 不匹配返回 error
- [x] TestOpen_V4_BadFooterMagic: Footer magic 错误返回 error
- [x] TestAdaptV4ToV2_Mapping: Segment 数量一致, ID 一致, Length 合理

## Phase 3: detector 包测试恢复
- [x] `detector` 包有单元测试（detector_test.go）— **11 个测试**
- [x] TestIsEncvContainerFromBytes_V4Magic: V4 magic → true
- [x] TestIsEncvContainerFromBytes_V3Magic: V3 magic → true
- [x] TestIsEncvContainerFromBytes_V2Magic: V2 magic → true
- [x] TestIsEncvContainerFromBytes_BadMagic: 坏 magic → false
- [x] TestIsEncvContainerFromBytes_ShortData: <6B → false
- [x] TestIsEncvContainerFromBytes_Nil: nil → false
- [x] TestDetectContainerType 覆盖：不存在路径、非容器文件
- [x] TestDetectIndexKind 覆盖：合法容器、不存在、非ENCV文件

## Phase 4: crypto 补全
- [x] TestGenerateSalt_v2_Size16: len == 16
- [x] TestGenerateSalt_v2_Size32: len == 32
- [x] TestGenerateSalt_v2_ZeroSize: len == 0
- [x] TestGenerateSalt_v2_Deterministic: 相同输入 → 相同输出
- [x] TestGenerateIV_v2_CorrectSize: len == size 参数
- [x] TestGenerateIV_v2_ZeroSize: len == 0
- [x] TestBase64Encode_v2_Roundtrip: Encode→Decode == 原始数据
- [x] TestBase64Encode_v2_Empty: 空输入处理
- [x] TestEncryptBytes_v2_DecryptBytes_v2_Roundtrip: 加密→解密 == 原始数据

## Phase 5: 插件系统测试
- [x] TestBuildFullPluginSettings_NilInput 成功返回默认配置
- [x] TestBuildFullPluginSettings_WithUserSettings 合并正确
- [x] TestBuildFullPluginSettings_EmptyUserSettings 等同 nil
- [x] TestFindEncryptingPlugin_ByExtension_TXT 匹配到 text 插件
- [x] TestFindEncryptingPlugin_ByExtension_GO 匹配到 text 插件
- [x] TestFindEncryptingPlugin_NoMatch 返回 error
- [x] TestTextPlugin_Initialize_WithSettings 成功加载自定义 ext
- [x] TestTextPlugin_Initialize_NoSettings 返回 "no settings found" 错误

## Phase 6: 反范式改造
- [x] reader/bench_test.go: `init()` → `TestMain(m)` ✅
- [x] physical/bench_test.go: `init()` → `TestMain(m)` ✅
- [x] detector/bench_test.go: `init()` → `TestMain(m)` ✅
- [x] TaskManager 测试无 "no such file or directory" 警告（t.TempDir()）

## Phase 7: 最终验证
- [x] `go test ./internal/... -count=1` 零 build failure ✅
- [x] `go test ./internal/... -count=1` 零 FAIL ✅
- [x] 新增测试文件数 >= 4（handle_test.go, detector_test.go, registry_test.go + crypto 扩展） ✅
- [x] 总 PASS 测试数 = **97**（远超之前数量） ✅
