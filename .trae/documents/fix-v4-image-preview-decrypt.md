# 修复 V4 容器图片预览/解密失败 — 收尾计划

## 当前状态总结

### 已完成的代码修改（Task 1-4 ✅）

| Task | 文件 | 状态 | 核心变更 |
|------|------|------|----------|
| Task 1 | [`single_file_container_writer.go:WriteFragment()`](internal/v2/writer/single_file_container_writer.go#L106) | ✅ | V4 分支直接 `file.Write(data)`，无 BlockHeader |
| Task 2 | [`single_file_container_writer.go:WriteKVI()`](internal/v2/writer/single_file_container_writer.go#L89) | ✅ | V4 分支仅 `hasher.Write`，不写独立 Block |
| Task 3 | [`adapter.go:AdaptV4ToV2()`](internal/v2/container/handle/adapter.go#L9) | ✅ | Nonce="" 纯数据模式：encDataSize=seg.Size, physicalOffset=seg.Offset; GlobalStartOffset 用 runningOffset 重构 |
| Task 4 | [`file_container_reader.go`](internal/v2/reader/file_container_reader.go#L121) | ✅ | headerOverhead=0 for V4; 跳过 verifyFragmentAt for V4 |

### 当前阻塞问题

**TestMain 冲突** — reader 包中有两个 `TestMain` 定义：
- [`bench_test.go:19`](internal/v2/reader/bench_test.go#L19) — 注册 "video" KVI provider
- [`reader_v4_e2e_test.go:16`](internal/v2/reader/reader_v4_e2e_test.go#L16) — 注册 "text"/"image"/"video" KVI providers

导致 `go test ./internal/v2/reader/...` 编译失败：`multiple definitions of TestMain`

---

## 剩余任务

### Task 5: 解决 TestMain 冲突 + 修复 E2E 测试加密流程 + 让测试通过

**修改文件:** [`bench_test.go`](internal/v2/reader/bench_test.go#L19)、[`reader_v4_e2e_test.go`](internal/v2/reader/reader_v4_e2e_test.go#L16)

#### 5a. 合并 TestMain 到 bench_test.go

当前 `bench_test.go:19-28` 的 TestMain 只注册 "video"：
```go
func TestMain(m *testing.M) {
    types.RegisterKVIProvider("video", func(rawKVI json.RawMessage) (types.KVIProvider, error) { ... })
    m.Run()
}
```

修改为同时注册 "text"、"image"、"video" 三种 kind（使用闭包捕获 kind 变量）。

#### 5b. 删除 reader_v4_e2e_test.go 中的重复 TestMain 和辅助类型

删除以下内容：
- L16-28: `func TestMain(m *testing.M)` 及函数体
- L30-56: `genericTestKVI` struct + 方法、`genericTestIndex` struct + 方法

#### 5c. ⭐ 关键修复：createV4PluginPathContainer 缺少加密步骤

**已确认的 Bug**：[`createV4PluginPathContainer()`](internal/v2/reader/reader_v4_e2e_test.go#L58) 当前将**原始明文**直接传入 `WriteFragment()`。但实际插件加密路径中，`WriteFragment` 接收的是**已加密的密文**。读者端会用 AES-CTR 解密数据——对明文做 AES 解密必然输出乱码。

**参照正确实现** [`bench_test.go:createContainerFixture()`](internal/v2/reader/bench_test.go#L51) 的做法：
```go
// 正确流程 (bench_test.go L64-65):
var encryptedBuf bytes.Buffer
crypto.EncryptStream_v2(bytes.NewReader(originalData), &encryptedBuf, key, iv)
// 然后 WriteFragment 写入 encryptedBuf.Bytes() (密文)
```

需要在 `createV4PluginPathContainer` 中补充：
1. 生成 key: `key := crypto.GenerateKey(password, salt, types.KeySize_v2)`
2. 加密 plaintext: `crypto.EncryptStream_v2(bytes.NewReader(plaintext), &encryptedBuf, key, iv)`
3. 将 `encryptedBuf.Bytes()`（而非 plaintext）传给 `WriteFragment`

> **注意**: `EncryptStream_v2` 是纯 AES-CTR 输出（无 salt/IV 前缀），输出大小 = 输入大小。直接使用即可。

#### 5d. KVI JSON 格式确认 ✅ 已验证

[`types.KVI`](internal/v2/types/container.go#L137) 结构体使用 JSON tag：
```go
SaltBase64 string `json:"salt_base64"`
IVBase64   string `json:"iv_base64"`
```
测试中 `map[string]string{"salt_base64":..., "iv_base64":...}` 序列化后可正确反序列化为 `types.KVI`。✅ 无需修改。

#### 5e. 运行测试并修复迭代

```bash
go test ./internal/v2/reader/... -count=1 -v -run "TestV4"
```

可能需要调试的问题：
- PhysicalOffset 值是否正确（应从 HeaderSize(2048) 之后开始）
- 解密后数据是否与原始明文完全一致

### Task 6: 更新既有 Writer E2E 测试适配新格式

**文件:** [`single_file_writer_v4_e2e_test.go`](internal/v2/writer/single_file_writer_v4_e2e_test.go)

该文件的 `createV4ViaPluginPath()` 同样存在**缺少加密步骤**的问题（写入的是 plaintext 而非 ciphertext）。但该文件的测试只验证容器结构（Magic/ManifestOffset/Footer/Segment count），不做解密 roundtrip，所以目前不会失败。

**操作步骤：**
1. 运行 writer E2E 测试：
   ```bash
   go test ./internal/v2/writer/... -count=1 -v
   ```
2. 如有断言失败，调整预期值
3. （可选）同样补全加密步骤以保持与真实路径一致

### Task 7: 全量验证

```bash
# 完整测试套件
go test ./internal/... -count=1 2>&1

# 构建验证
go build ./... 2>&1
```

验收标准：零 FAIL、零 PANIC、构建零 error。

---

## 执行顺序

```
Task 5a → Task 5b → Task 5c → Task 5e (运行测试)
    ↓ 通过?
    ↓     ↓ 失败→调试修复→重跑
    ↓ 通过
Task 6 → Task 7
```
