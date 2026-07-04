# 范式优化：alist_encrypt 预览流走统一 ContentHandler

> **核心问题**：alist_encrypt 的 `ServeStream` 自己实现 Range/206/Content-Length/Content-Range，**重复造轮子**且易出 bug。
> **范式真理之源**：v4 容器走 `LocalFileProvider` → `ContentHandler.ServeFile`，Range/206/Content-Length 都已正确实现。
> **目标**：让 alist_encrypt 也走这条统一路径，撤销之前在 streamer.go 加的局部 fix，避免走偏。

---

## 一、当前状态盘点

### 1.1 4 个用户反馈的修复进度

| # | 问题 | 状态 | 说明 |
|---|------|------|------|
| 1 | alist-decrypt 不该有前置 promptPassword | ✅ 已修 | `actions.ts:42-53` 直接 `openNewTask` |
| 2a | 解密输出文件名多 `.bin` 后缀 | ✅ 已修 | `decryptor.go` `tryDecodeFilename` |
| 2b | 重命名 404 | ✅ 已修 | `server.go:223` `/api/file/rename` |
| 2c | 文件名还原成 `CAD放样.mp4` | ✅ 已完成 | 用 `rename` API 还原 |
| 3 | 任务详情缺产物展示 | ✅ 已修 | `TaskDetailModal.vue` + `task_manager.go` OutputPath + i18n key |
| 4 | alist_encrypt 预览失败 | ⚠️ 修了局部，全局范式未统一 | 详见 §1.2 |

### 1.2 问题 4 现状：已 2 个修复，1 个范式不统一

**已修 ①**：`streamer.go:144-145` 补了 `Content-Length` 在 206 时的部分大小（curl 验证通过）
**已修 ②**：`actions.ts` + `Files.vue` + `ArtPlayerView.vue` 用 `alistPath+password` 替代整体 `streamUrl` 传 query

**❌ 范式不统一**：
- v4 容器走 `LocalFileProvider` → `ContentHandler.ServeFile`（统一抽象）
- alist_encrypt 走 `plugin.ServeStream` → **自己实现 HTTP 协议**（重复造轮子）
- **后果**：任何 HTTP 协议 bug（如 Content-Length）都要在两处修一次
- **风险**：未来加 v5/v6 容器或改 HTTP 协议（如增加 `If-Range`），alist_encrypt 永远滞后

### 1.3 用户的明确反馈（plan 被拒原因）

> "流式预览需要统筹整个项目范式优化，避免过于差异。预览服务提供委托到插件系统后，alist_encrypt 插件不应该过度关心如何处理流式预览细节。插件系统应当优先考虑 video 插件（v4 容器）预览稳定，alist_encrypt 不一定是视频也需要注意，不要修迷路了"

**翻译成行动要求**：
1. **不要在 alist_encrypt 内部塞 HTTP 协议特例**（我的 streamer.go 局部 fix 走偏了）
2. **统一接口是真理之源**（v4 走 `ContentHandler.ServeFile` → alist_encrypt 也走）
3. **v4 容器（video 插件）预览是主战场**，任何范式变更必须先保证 v4 不退化
4. **alist_encrypt 是异类**（不一定是视频），统一接口要支持任意 content-type
5. **不要修迷路了** —— 不要在某个具体插件里塞特例

---

## 二、统一范式

### 2.1 真理之源：`provider.FileContentProvider` + `ContentHandler.ServeFile`

**接口契约**（`internal/v2/provider/provider.go:15-34`）：
```go
type FileContentProvider interface {
    GetReader() io.ReadCloser       // 读流
    GetSeeker() (io.Seeker, bool)   // 随机访问
    GetSeekerTo() (SeekerTo, bool)  // 快速定位（远程）
    GetSize() int64                 // 明文总大小
    GetName() string                // 明文文件名
    Close() error
}
```

**HTTP 协议层**（`internal/v2/handler/content.go:32-109`）—— `ContentHandler.ServeFile`：
- 自动解析 `Range` header
- 自动 Seek 到 `start`
- 自动设置 `Content-Type`（按 `filepath.Ext(name)`）
- 自动设置 `Content-Disposition: inline; filename="<name>"`
- 自动设置 `Accept-Ranges: bytes`
- 自动设置 `Content-Range: bytes start-end/total`（206 时）
- 自动设置 `Content-Length: end-start+1`（**部分大小**）
- 自动写 `200 / 206 / 416` 状态码
- 自动 `io.LimitReader(reader, contentLength)` 防止越界

### 2.2 v4 容器走法（已稳定，不动）

```
server.go:466 detect ENCV container
  → server_handle.go:113 serveEncryptedFile(w, r, path)
  → readerService.GetDecryptReader(...) → factory + decryptReader
  → provider.NewLocalFileProvider(ctx, factory, decryptReader) → FileContentProvider
  → contentHandler.ServeFile(w, r, prov)   ← 统一 HTTP 协议
```

### 2.3 alist_encrypt 走法（**本次目标**）

**当前（走偏）**：
```
mobile_api.go:1076 handleAlistEncryptStreamGin
  → plugin.ServeStream(c.Writer, c.Request, path, password)   ← 自己实现 Range/206/Content-Length
```

**目标（统一）**：
```
mobile_api.go:1076 handleAlistEncryptStreamGin
  → alistencrypt.NewAlistEncryptFileProvider(...) → FileContentProvider
  → contentHandler.ServeFile(c.Writer, c.Request, prov)   ← 与 v4 走同一路径
```

---

## 三、实施步骤

### Step 1: 新增 `AlistEncryptFileProvider`（不破坏现有代码）

**文件**：`internal/v2/plugins/alistencrypt/provider.go`（新建）

```go
package alistencrypt

import (
    "io"

    "github.com/Soltus/encv-go/internal/v2/provider"
)

// AlistEncryptFileProvider 实现 provider.FileContentProvider 接口
// 包装 seekableDecryptReader，让 alist_encrypt 走统一的 ContentHandler.ServeFile
type AlistEncryptFileProvider struct {
    reader *seekableDecryptReader  // 已实现 Read + Seek + Close
    size   int64                   // 明文大小（来自 Stream() 返回）
    name   string                  // 解码后的明文文件名（来自 ConvertShowName）
}

func NewAlistEncryptFileProvider(reader *seekableDecryptReader, size int64, name string) *AlistEncryptFileProvider {
    return &AlistEncryptFileProvider{reader: reader, size: size, name: name}
}

func (p *AlistEncryptFileProvider) GetReader() io.ReadCloser {
    return p.reader
}

func (p *AlistEncryptFileProvider) GetSeeker() (io.Seeker, bool) {
    // seekableDecryptReader 内嵌 DecryptReader，DecryptReader 实现了 Seek
    return p.reader.DecryptReader, true
}

func (p *AlistEncryptFileProvider) GetSeekerTo() (provider.SeekerTo, bool) {
    return nil, false
}

func (p *AlistEncryptFileProvider) GetSize() int64 {
    return p.size
}

func (p *AlistEncryptFileProvider) GetName() string {
    return p.name
}

func (p *AlistEncryptFileProvider) Close() error {
    return p.reader.Close()
}
```

**已有基础**：
- `seekableDecryptReader`（`streamer.go:15-25`）—— 包装了 `DecryptReader`，`DecryptReader` 已实现 `Read` + `Seek`
- `Stream()`（`streamer.go:38-88`）—— 返回 `(*seekableDecryptReader, plainSize, contentType, showName, error)`
- `ShowName` 来自 `ConvertShowName` —— alist-encrypt-go 兼容解码，**已经是明文**（如 `CAD放样.mp4`）

### Step 2: 改写 `handleAlistEncryptStreamGin` 走统一范式

**文件**：`internal/server/mobile_api.go:1076-1097`

```go
func (s *Server) handleAlistEncryptStreamGin(c *gin.Context) {
    queryPath := utils.DecodeGinQueryParam(c.Query("path"))
    password := c.Query("password")
    if queryPath == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "'path' query parameter is required"})
        return
    }
    absPath, err := utils.SafeResolveToAbsPath(s.servingDir, queryPath)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
        return
    }
    slog.Info("API: alist-encrypt stream", "path", absPath)

    // 走统一范式：构造 FileContentProvider，调 ContentHandler.ServeFile
    var plugin alistencrypt.AlistEncryptPlugin
    rc, size, _, showName, err := plugin.Stream(absPath, password)
    if err != nil {
        slog.Error("API: alist-encrypt stream open failed", "error", err)
        writeServiceErrorGin(c, err)
        return
    }
    // rc 是 *seekableDecryptReader，需要做 type assertion
    sr, ok := rc.(*alistencrypt.SeekableDecryptReader)  // 暴露给外部用
    if !ok {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal: unexpected reader type"})
        return
    }
    prov := alistencrypt.NewAlistEncryptFileProvider(sr, size, showName)
    defer prov.Close()
    s.contentHandler.ServeFile(c.Writer, c.Request, prov)
}
```

**注**：`seekableDecryptReader` 当前是 unexported，需要在 `streamer.go` 把它 export 为 `SeekableDecryptReader`（或加一个公开包装接口）。

### Step 3: 删除/废弃 `ServeStream`（让 plugin 回归数据提供方职责）

**文件**：`internal/v2/plugins/alistencrypt/streamer.go:90-165`

**操作**：删除 `func (p *AlistEncryptPlugin) ServeStream(...)` 整个函数。

**理由**：
- HTTP 协议处理已经委托给 `ContentHandler.ServeFile`
- plugin 只需要负责"打开文件、解密、返回 reader + size + name"
- 与 v4 容器一致：plugin 不直接处理 HTTP

**注**：如果其他代码（cmd 端、test）调过 `ServeStream`，需要一并改用 `ContentHandler.ServeFile + Provider` 模式。本次先 grep 确认没有其他调用方再删。

### Step 4: 撤销 `streamer.go:144-145` 的局部 Content-Length fix

**原因**：范式统一后，Content-Length 由 `ContentHandler.ServeFile` 统一处理。

**操作**：
- 删除 `partialLen := end - start + 1`
- 删除 `w.Header().Set("Content-Length", strconv.FormatInt(partialLen, 10))` （Range 分支里）
- 删除 `remaining := partialLen`（改回 `end - start + 1`）

**注**：如果发现 streamer.go 在删除 ServeStream 时一并被精简，可以整文件重写。

### Step 5: content-type 检测

**问题**：`ContentHandler.ServeFile` 用 `utils.GetContentType(filepath.Ext(name))`，而 alist_encrypt 之前用 `detectContentTypeByName(showName, path)`。

**需要确认**：
- `utils.GetContentType` 是否覆盖 alist_encrypt 常见扩展名（mp4, mkv, mov, jpg, png, pdf 等）
- 如果不够，alist_encrypt 的 FileContentProvider 包装 `showName` 时附加正确的 content-type 字段（需要扩展 `FileContentProvider` 接口加 `GetContentType() string`，但这破坏了与 LocalFileProvider 一致性）

**fallback**：先看 `utils.GetContentType` 覆盖度。如果够了，streamer.go 的 `detectContentTypeByName` 可以删除。如果不够，先临时用 `filepath.Ext(name)` 推导，后续扩展 `FileContentProvider` 接口。

### Step 6: 验证 v4 容器预览不退化

**重要**：范式变更后必须保证 v4 容器预览不受影响。

**验证步骤**：
- curl `/stream?path=<v4 容器文件>` → 200 + 视频数据
- curl `/stream?path=<v4 容器文件>` 带 Range → 206 + 部分大小
- 浏览器：长按 v4 容器 → 选"预览" → ArtPlayer 播放

**如果 v4 退化**：立刻回滚（不要回头修 alist_encrypt，先恢复 v4 稳定）。

---

## 四、agent-browser 调试铁律（防止再打断会话）

> 来自上一轮调试的踩坑经验

### 4.1 启动顺序（不要颠倒）

```bash
# 1. 确认 preview 服务在跑（不要重启）
ps -ef | grep -E "(start-preview|air|encv|vite)" | grep -v grep
# 期待：6 个进程

# 2. 确认 Chrome 9222 在跑
curl -s http://localhost:9222/json/version | head -3

# 3. 用 --cdp 9222 连接（不要默认 open）
agent-browser --cdp 9222 open "http://localhost:5174/tabs/files"
```

### 4.2 绝对禁止操作

| 禁止 | 原因 |
|------|------|
| ❌ `pkill -f air` / `pkill -f vite` / `pkill -f encv` | 会破坏 preview server |
| ❌ `pkill -f agent-tool-host` | 沙箱基础设施 |
| ❌ 重命名/删除 `/storage/emulated/0/` 下的产物 | 已验过的产物，丢失破坏回归 |
| ❌ 改 `config.user.json` | 脚本铁律 |
| ❌ 用默认 `open` 命令（无 `--cdp`） | 拉新 Chrome 失败，连不上 5173 |
| ❌ `eval 'await ...'` 顶层 | eval 不支持 await |

### 4.3 推荐操作

| 场景 | 操作 |
|------|------|
| 调试长按菜单 | `window.__ENCV_TEST.simulateLongPress(fileName).then(()=>1)` |
| 设 input 值（Vue 响应式） | `el.value='x'; el.dispatchEvent(new Event('input',{bubbles:true}))` |
| 重置页面 | `agent-browser --cdp 9222 open http://localhost:5174/tabs/files` |
| 看 Network | `agent-browser --cdp 9222 network requests --type xhr,fetch,media` |
| 关闭 | `agent-browser --cdp 9222 close`（不是 `close`） |

### 4.4 每次重测前的快照

```bash
# 进程快照
ps -ef | grep -E "(start-preview|air|encv|vite)" | grep -v grep > /tmp/preview-pids.txt

# 文件快照
ls -la /storage/emulated/0/CAD放样* > /tmp/filestate.txt
ls -la /storage/emulated/0/hyYGPCwJPQ3* >> /tmp/filestate.txt
```

---

## 五、端到端验证清单

实施完成后必须跑过：

- [ ] 编译通过：air 重载 encv，stderr 无 error
- [ ] curl `/api/alist-encrypt/stream?path=...&password=8682268` → 200 + 51958979 bytes + Content-Type: video/mp4
- [ ] curl `/api/alist-encrypt/stream?path=...&password=8682268` 带 `Range: bytes=0-99` → 206 + 100 bytes + Content-Range: bytes 0-99/51958979 + Content-Length: 100
- [ ] 浏览器（agent-browser --cdp 9222）：长按 `hyYGPCwJPQ3+xrdAvfnn2.bin` → 菜单有「流式预览」「解密」两项
- [ ] 浏览器：流式预览 → 弹 promptPassword → 输 8682268 → 跳 /player → ArtPlayer 加载 → 视频可见可播
- [ ] v4 容器不退化：长按 v4 容器 → 选"预览" → ArtPlayer 播放（必须）
- [ ] 解密流程：长按 → 解密 → NewTaskModal（无前置 promptPassword）→ 输密码 → 任务完成 → TaskDetailModal 显示产物 → 跳转链接

---

## 六、不能动的资产（重复强调）

| 资产 | 原因 |
|------|------|
| `air` (pid 101849) | preview server 后端监视器 |
| `vite` (pid 101912) | preview server 前端 |
| `encv` (最新 pid) | 唯一后端实例 |
| `agent-tool-host` (pid 821) | 沙箱基础设施 |
| `Chrome @ 9222` | 已有 ENCV 页面 |
| `/storage/emulated/0/CAD放样.mp4` | 已还原的解密产物 |
| `/storage/emulated/0/hyYGPCwJPQ3+xrdAvfnn2.bin` | 原始加密测试文件 |
| `config.user.json` | 脚本铁律禁止改 |

---

## 七、等待用户批准

实施本 plan 前需要你确认：
1. **范式方向**：同意让 alist_encrypt 走统一 `ContentHandler.ServeFile`（与 v4 同路）
2. **删除 ServeStream**：同意 plugin 回归"数据提供方"职责，HTTP 协议由 ContentHandler 统一处理
3. **content-type 检测**：先验证 `utils.GetContentType` 覆盖度，不够再扩展接口
4. **v4 不退化**：同意把 v4 容器预览验证作为范式变更的硬性 gate
