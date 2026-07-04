# Tasks

- [x] Task 1: 移除 server_handle.go 中的手动 DecodePathParam 调用
  - [x] 1.1 将 L86 `filePath := utils.DecodePathParam(r.URL.Query().Get("path"))` 改为 `filePath := r.URL.Query().Get("path")`
  - [x] 1.2 将 L88 `filePath = utils.DecodePathParam(r.URL.Query().Get("file"))` 改为 `filePath = r.URL.Query().Get("file")`
  - [x] 1.3 确认 utils import 仍被 SafeURLToAbsPath 使用，保留

- [x] Task 2: 移除 openlist_middleware.go 中的手动 DecodePathParam 调用
  - [x] 2.1 将 L43 `fileURL := utils.DecodePathParam(c.Request.URL.Query().Get("file"))` 改为 `fileURL := c.Request.URL.Query().Get("file")`
  - [x] 2.2 确认 `fileURL` 后续经过 `url.Parse()` 处理（不经过 SafeURLPathToRelative），验证解码正确性

- [x] Task 3: 移除 openlist_handlers.go 中的手动 DecodePathParam 调用
  - [x] 3.1 将 L131 `durl := utils.DecodePathParam(c.Request.URL.Query().Get("file"))` 改为 `durl := c.Request.URL.Query().Get("file")`
  - [x] 3.2 确认 `durl` 后续经过 `url.Parse()` 处理（不经过 SafeURLPathToRelative），验证解码正确性
  - [x] 3.3 添加注释说明：此处 `durl` 是完整 URL 不经过 SafeURLPathToRelative，由 `url.Parse` 处理

- [x] Task 4: 统一 handleStreamExternalFileGin + StreamExternalFile 解码路径
  - [x] 4.1 移除 L473-477 的手动 `url.QueryUnescape(queryPath)` 逻辑
  - [x] 4.2 改为直接 `queryPath := c.Query("path")` 传入 `StreamExternalFile`
  - [x] 4.3 StreamExternalFile 统一走 SafeURLToAbsPath 解码（不再区分绝对/相对路径）

- [x] Task 5: 增强端到端解码链路测试
  - [x] 5.1 在 `path_test.go` 新增 `TestSafeURLToAbsPath_ProxySafeEncodeRoundTrip` — 10 个子场景覆盖根路径、@、#、中文、emoji、%字面量、空格、深层路径
  - [x] 5.2 测试含 `%` 字面量文件名（如 `report%Q1.txt`）不被过度解码
  - [x] 5.3 测试含 `@` 的路径正确还原
  - [x] 5.4 测试含中文/emoji 的路径正确还原
  - [x] 5.5 新增 `TestDecodePathParam_Idempotent` — 6 个子场景确认幂等性
  - [x] 5.6 新增 `TestNoTripleDecode_AfterRemovingManualCalls` — 确认三重解码已消除

- [x] Task 6: 编译验证 + 回归测试
  - [x] 6.1 `go build ./cmd/encv/...` 编译通过
  - [x] 6.2 `go test ./internal/utils/ ./internal/server/ ./internal/config/ -count=1` 通过（唯一 FAIL 是预已存在的 TestHandlePluginsGin_ReturnsAllPlugins）
  - [x] 6.3 grep `DecodePathParam` 确认仅出现在 SafeURLPathToRelative 内部和测试中

# Task Dependencies
- [Task 5] depends on [Task 1, 2, 3, 4]
- [Task 6] depends on [Task 1, 2, 3, 4, 5]
- [Task 1, 2, 3, 4] 可并行执行
