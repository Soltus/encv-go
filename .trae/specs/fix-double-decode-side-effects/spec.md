# 双重解码全面适配与副作用清理 Spec

## Why

前端 `proxySafeEncode()` 对路径做双重 URL 编码（防 WAF/代理截断 `@`），后端 `SafeURLPathToRelative()` 内置了 `DecodePathParam()` 做双层解码。但此前 `server_handle.go`、`openlist_handlers.go`、`openlist_middleware.go` 中已手动调用 `DecodePathParam()`，再传入 `SafeURLToAbsPath()` 会形成**三重解码**——对含 `%` 字面量的文件名（如 `file%name.txt`）产生乱码。同时 `mobile_api.go` 中 `handleStreamExternalFileGin` 手动 `url.QueryUnescape()` 只做单层解码，与其他 handler 不一致。需要统一解码范式，消除所有过度/不足解码。

## 权威来源与工程范式

### RFC 3986 §2.1 — 百分号编码

URL 中非安全字符必须编码为 `%HH`。代理/WAF 可能截断 `@` 等保留字符，因此前端双重编码（`%2540` 代表 `@`），后端需对应双层解码。

### 解码契约（项目约定）

| 层级 | 职责 | 编解码次数 |
|------|------|-----------|
| 前端 `proxySafeEncode()` | 双重编码 | 2 层 encode |
| HTTP 传输 + Gin/Go `c.Query()` / `r.URL.Query().Get()` | 自动解码 1 层 | -1 层 |
| 后端 `SafeURLPathToRelative()` | 双层解码（`DecodePathParam`） | -2 层 |
| **净效果** | | 2-1-2 = **-1**（还原原始值）|

**核心原则：`SafeURLPathToRelative()` 是唯一的解码入口。所有调用方不得在传入前再做 `DecodePathParam`。**

## What Changes

### 修改：移除手动 `DecodePathParam` 调用（消除三重解码）

| 文件 | 行号 | 当前代码 | 改为 |
|------|------|---------|------|
| `server_handle.go` | L86-88 | `filePath := utils.DecodePathParam(r.URL.Query().Get("path"))` | `filePath := r.URL.Query().Get("path")` |
| `openlist_handlers.go` | L131 | `durl := utils.DecodePathParam(c.Request.URL.Query().Get("file"))` | `durl := c.Request.URL.Query().Get("file")` |
| `openlist_middleware.go` | L43 | `fileURL := utils.DecodePathParam(c.Request.URL.Query().Get("file"))` | `fileURL := c.Request.URL.Query().Get("file")` |

### 修改：统一 `mobile_api.go` 中的解码方式

| 文件 | 行号 | 当前代码 | 改为 |
|------|------|---------|------|
| `mobile_api.go` | L473-477 | `handleStreamExternalFileGin` 手动 `url.QueryUnescape(queryPath)` | 直接用 `c.Query("path")` 传入 `StreamExternalFile`，由 `SafeURLToAbsPath` 统一解码 |

### 修改：`mobile_api.go` 中所有 `c.Query("path")` / `c.Query("sourcePath")` handler 统一走 `SafeURLToAbsPath`

以下 handler 当前直接把 `c.Query("path")` 传给 `MobileService` 方法，依赖 `SafeURLPathToRelative` 内部的 `DecodePathParam` 解码——这是正确的（因为 `SafeURLToAbsPath` → `SafeURLPathToRelative` → `DecodePathParam`）。无需额外修改，但需确认 `MobileService` 方法内部都走 `SafeURLToAbsPath`：

| Handler | 方法 | 是否走 SafeURLToAbsPath | 状态 |
|---------|------|------------------------|------|
| `handleListFilesGin` | `ListFiles(queryPath)` | ✅ 是 | 无需改 |
| `handleDeleteFileGin` | `DeleteFile(queryPath)` | ✅ 是 | 无需改 |
| `handleReadFileContentGin` | `ReadFileContent(queryPath)` | ✅ 是 | 无需改 |
| `handleGetFileInfoGin` | `GetFileInfo(queryPath)` | ✅ 是 | 无需改 |
| `handleSearchFilesGin` | `SearchFiles(queryPath, ...)` | ✅ 是 | 无需改 |
| `handleFileExistsGin` | `FileExists(queryPath)` | ✅ 是 | 无需改 |
| `handleEncryptOutputExistsGin` | `CheckEncryptOutputExists(sourcePath, targetDir)` | ✅ 是 | 无需改 |
| `handleListFilesStreamGin` | 直接 `SafeURLToAbsPath` | ✅ 是 | 无需改 |
| `handleAlistEncryptStreamGin` | 直接 `SafeURLToAbsPath` | ✅ 是 | 无需改 |
| `handlePluginFilesStreamGin` | 直接 `SafeURLToAbsPath` | ✅ 是 | 无需改 |
| `handleStreamExternalFileGin` | 手动 `url.QueryUnescape` | ❌ 不走统一路径 | **需改** |

### 新增：端到端解码测试

- 测试 `proxySafeEncode` → HTTP → Gin `c.Query` → `SafeURLToAbsPath` 完整链路
- 测试含 `%` 字面量的文件名不被过度解码
- 测试含 `@` 的路径被正确还原

### 新增：`DecodePathParam` 幂等性保护

当输入已经是解码后的值（如 `/DCIM/file.txt`），`DecodePathParam` 应该是幂等的（不再改变）。当前实现 `url.QueryUnescape` 对无百分号编码的字符串是幂等的，但连续调用两次可能对含 `%` 字面量的文件名产生错误。在 `SafeURLPathToRelative` 中只需调用一次 `DecodePathParam`，这是正确的。但需要测试覆盖确认。

## Impact

- Affected specs: `unify-path-resolver`（前端路径归一化）
- Affected code:
  - **修改** `internal/server/server_handle.go` — 移除手动 DecodePathParam
  - **修改** `internal/server/openlist_handlers.go` — 移除手动 DecodePathParam
  - **修改** `internal/server/openlist_middleware.go` — 移除手动 DecodePathParam
  - **修改** `internal/server/mobile_api.go` — handleStreamExternalFileGin 统一解码路径
  - **新增测试** `internal/utils/path_test.go` — 端到端解码链路测试
  - **新增测试** `internal/server/mobile_api_test.go` — handler 解码正确性测试

## ADDED Requirements

### Requirement: 统一解码契约

系统 SHALL 遵循唯一解码入口原则：`SafeURLPathToRelative()` 是所有 URL 路径解码的唯一执行点。所有调用方 SHALL NOT 在传入前手动调用 `DecodePathParam` 或 `url.QueryUnescape`。

#### Scenario: 前端双重编码路径正确还原
- **WHEN** 前端调用 `proxySafeEncode("/DCIM/视频@合集")` 生成双重编码值
- **AND** Gin `c.Query("path")` 自动解码一层
- **AND** `SafeURLPathToRelative()` 调用 `DecodePathParam()` 解码两层
- **THEN** 最终路径 SHALL 为 `/DCIM/视频@合集`（原始值）

#### Scenario: 含百分号字面量的文件名不被破坏
- **WHEN** 文件名为 `report%Q1.txt`（`%` 是文件名的一部分而非编码前缀）
- **AND** 前端 `proxySafeEncode` 编码后为 `report%25Q1.txt`（双重编码后 `%2525Q1`）
- **THEN** 经完整解码链路后 SHALL 还原为 `report%Q1.txt`

#### Scenario: 旧代码手动 DecodePathParam 产生三重解码
- **WHEN** `server_handle.go` 中 `DecodePathParam(r.URL.Query().Get("path"))` 再传入 `SafeURLToAbsPath`
- **THEN** 路径被三重解码，含 `%` 字面量的文件名 SHALL 产生乱码
- **AND** 此代码 SHALL 被移除手动 `DecodePathParam` 调用

### Requirement: handleStreamExternalFileGin 统一解码

`handleStreamExternalFileGin` SHALL 与其他 handler 一致，不再手动 `url.QueryUnescape`，而是通过 `MobileService.StreamExternalFile` 内部的 `SafeURLToAbsPath` 统一解码。

#### Scenario: 外部文件流请求路径解码
- **WHEN** 前端请求 `/api/stream/external?path=%252FDCIM%252Fvideo.mp4`
- **THEN** `StreamExternalFile` 内部通过 `SafeURLToAbsPath` 解码后 SHALL 得到 `/DCIM/video.mp4`

## MODIFIED Requirements

### Requirement: DecodePathParam 调用规范

`DecodePathParam` SHALL 仅在 `SafeURLPathToRelative` 内部调用。外部代码禁止直接调用 `DecodePathParam` 处理路径参数，除非该路径不经过 `SafeURLToAbsPath` / `SafeURLPathToRelative` 处理链路。

**例外**：`openlist_handlers.go` 中 `handleDecrypt` 的 `durl` 参数是完整 URL（非文件路径），不经过 `SafeURLPathToRelative`，因此其 `DecodePathParam` 调用保留。但需在代码中添加注释说明原因。

## REMOVED Requirements

无。
