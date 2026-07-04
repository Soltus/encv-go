# ContainerHandle 集中化 Checklist

## Phase 1: 核心抽象层

- [ ] ContainerSource 接口定义正确（io.ReaderAt + io.Seeker + Size() int64 + Name() string）
- [ ] FileSource 实现正确，基于 *os.File
- [ ] BytesSource 实现正确，基于 bytes.Reader，可用于测试
- [ ] RemoteSource 实现正确，基于 HTTP Range
- [ ] ContainerHandle 接口定义完整（Version, HeaderSize, ContainerType, IsSeekable, Manifest, ManifestV4, HeaderV2/V3/V4, FooterV2/V4, Source, Close）
- [ ] Open() 工厂函数正确检测版本号并分发到对应 open 函数
- [ ] openV4() 正确读取 Header(2048B) + Footer(12B) + Manifest(Deobfuscate + Deserialize)
- [ ] openV23() 正确读取 Footer(32B) + Manifest(Block 解密)
- [ ] ContainerType() 对 V4 从 Header 取值，对 V2/V3 从 Manifest.Kind 推断
- [ ] IsSeekable() 对 V4 从 Header.IsSeekable 取值，对 V2/V3 从 Fragment 类型推断
- [ ] adaptV4ToV2Manifest 逻辑已从 manifest_v2.go 迁移到 handle 包

## Phase 2: 消费者迁移

- [ ] detector.DetectContainer 内部使用 ContainerHandle，行为不变
- [ ] detector.DetectContainerType 内部使用 ContainerHandle，行为不变
- [ ] detector.DetectIsSeekable 内部使用 ContainerHandle，行为不变
- [ ] detector.DetectIndexKind 内部使用 ContainerHandle，行为不变
- [ ] detector.DetectV4Header 内部使用 ContainerHandle，行为不变
- [ ] detector.IsEncvContainerFromBytes 保持不变
- [ ] reader.NewEncryptedContainerReaderFromFile 内部使用 ContainerHandle，行为不变
- [ ] remote_container_reader.GetManifest 使用基于 RemoteSource 的 ContainerHandle
- [ ] remote_container_reader 中删除 getManifestV4 / adaptV4ToV2ManifestRemote / binaryRead / headerDataAsReader
- [ ] segment_reader.OpenV4Container 标记为 convenience wrapper 或委托给 ContainerHandle
- [ ] analyzeHeader 改为接收 ContainerHandle，版本分支内聚
- [ ] analyze Footer 分析使用 handle.FooterV2/FooterV4
- [ ] performCrossValidationV4 改为接收 ContainerHandle
- [ ] mobile_service.GetFileInfo 移除双路径回退，单次 ContainerHandle 打开
- [ ] openlist_handlers.handleDecrypt 无回归

## Phase 3: 清理与验证

- [ ] manifest_v2.go 中 readManifestV4 和 adaptV4ToV2Manifest 已删除或标记 deprecated
- [ ] manifest_v2.go 无残留的 V4 相关 import（crypto, base64, ReadFooterV4 等）
- [ ] 全局搜索无散落的 if version == 4 / version == 3 分支（仅在 handle 包内部和 types 包中）
- [ ] BytesSource 单元测试：V2 解析通过
- [ ] BytesSource 单元测试：V3 解析通过
- [ ] BytesSource 单元测试：V4 解析通过（Header/Footer/Manifest/适配全部验证）
- [ ] BytesSource 单元测试：无效数据返回错误
- [ ] go build ./cmd/encv/... 通过
- [ ] vite build 通过
