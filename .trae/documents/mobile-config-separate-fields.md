# 配置架构优化：移动端独立路径字段

## 问题回顾

### 之前方案（有缺陷）
在 `Load()` 中用 `ENCV_MOBILE` 环境变量**运行时覆盖**通用字段（`server.dir`、`output_path`、`webdav.dir`）：

```go
if os.Getenv("ENCV_MOBILE") == "1" {
    cfg.Server.Dir = os.Getenv("HOME")        // ← 覆盖同一字段
    cfg.OutputPath = filepath.Join(home, ...)   // ← 覆盖同一字段
    cfg.Webdav.Dir = ""                         // ← 覆盖同一字段
}
```

**缺陷**：用户在移动端编辑 `config.user.json` 后同步到桌面端（或反向），移动端路径值会**直接污染**桌面端字段。因为修改的是**同一个 JSON 字段**。

### 更优方案：独立的 `mobile` 配置段

新增 `mobile` 段，包含移动端专用路径字段。桌面端完全忽略该段，移动端读取它作为覆盖值：

```json
{
  "server": { "port": 2025, "dir": "/" },
  "output_path": "./output",
  "webdav": { "dir": "./output" },

  "mobile": {
    "server_dir": "/storage/emulated/0",
    "output_path": "/storage/emulated/0/encv-output",
    "webdav_dir": ""
  }
}
```

**优势**：
- 桌面端读取 `server.dir`/`output_path`/`webdav.dir` → 完全不受 `mobile` 段影响
- 移动端读取 `mobile.server_dir`/`mobile.output_path`/`mobile.webdav_dir` → 作为覆盖
- **双向同步安全**：不会出现跨平台路径污染

---

## 执行步骤

### Step 1: Go 类型定义 — 新增 MobileConfig

**文件**: [`internal/v2/types/types.go`](internal/v2/types/types.go)（或 [`internal/config/config.go`](internal/config/config.go)）

新增结构体：

```go
// MobileConfig 移动端专用配置段，桌面端忽略。
// 用于覆盖 server.dir / output_path / webdav.dir 等平台敏感路径。
type MobileConfig struct {
    // ServerDir 覆盖 server.dir，默认为 $HOME
    ServerDir string `json:"server_dir"`
    // OutputPath 覆盖 output_path，默认为 $HOME/encv-output
    OutputPath string `json:"output_path"`
    // WebdavDir 覆盖 webdav.dir，空字符串表示禁用本地 WebDAV
    WebdavDir string `json:"webdav_dir"`
}
```

在 `Config` 结构体中添加字段：

```go
type Config struct {
    // ... 现有字段 ...
    Preview *PreviewConfig `json:"preview,omitempty"`
    Mobile *MobileConfig  `json:"mobile,omitempty"`  // 新增
}
```

### Step 2: Go Load() — 用 mobile 字段替代运行时修正

**文件**: [`internal/config/config.go:Load()`](internal/config/config.go#L101)

**删除** 之前的 ENCV_MOBILE 运行时修正代码块（L125-136），**替换**为：

```go
if os.Getenv("ENCV_MOBILE") == "1" && cfg.Mobile != nil {
    home := os.Getenv("HOME")
    if cfg.Mobile.ServerDir != "" {
        cfg.Server.Dir = cfg.Mobile.ServerDir
    } else if cfg.Server.Dir == "/" || cfg.Server.Dir == "." {
        cfg.Server.Dir = home
    }
    if cfg.Mobile.OutputPath != "" {
        cfg.OutputPath = cfg.Mobile.OutputPath
    } else if cfg.OutputPath == "" || cfg.OutputPath == "./encrypted" {
        cfg.OutputPath = filepath.Join(home, "encv-output")
    }
    if cfg.Mobile.WebdavDir != "" {
        cfg.Webdav.Dir = cfg.Mobile.WebdavDir
    }
}
```

关键区别：
- **只读取 `cfg.Mobile.*` 字段的显式值**
- 不再盲目地用 `$HOME` 覆盖通用字段
- 桌面端即使误设了 `mobile` 段也不会生效（由 `ENCV_MOBILE` 守卫）

### Step 3: 更新 config.user.json 模板

**文件**: [`config.user.json`](config.user.json)

在现有内容基础上追加 `mobile` 段：

```json
{
  "... 现有字段不变 ...",

  "mobile": {
    "server_dir": "/storage/emulated/0",
    "output_path": "/storage/emulated/0/encv-output",
    "webdav_dir": ""
  }
}
```

### Step 4: Kotlin 侧 mergeConfigDefaults 增加 mobile 段合并

**文件**: [`EncvGoService.kt`](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt#L423)

在 `mergeConfigDefaults()` 的末尾（`changed` 写回之前），增加 `mobile` 段合并逻辑：

```kotlin
val existingMobile = existing.optJSONObject("mobile")
val defaultMobile = defaults.optJSONObject("mobile")
if (defaultMobile != null) {
    val targetMobile = existingMobile ?: JSONObject().also {
        existing.put("mobile", it)
        changed = true
    }
    if (!targetMobile.has("server_dir")) {
        targetMobile.put("server_dir", defaultMobile.optString("server_dir", ""))
        changed = true
    }
    if (!targetMobile.has("output_path")) {
        targetMobile.put("output_path", defaultMobile.optString("output_path", ""))
        changed = true
    }
    if (!targetMobile.has("webdav_dir")) {
        targetMobile.put("webdav_dir", defaultMobile.optString("webdav_dir", ""))
        changed = true
    }
}
```

同样更新 `writeFallbackConfig()` (L474)，增加 `mobile` 段的最小 fallback：

```kotlin
put("mobile", JSONObject().apply {
    put("server_dir", "/storage/emulated/0")
    put("output_path", "/storage/emulated/0/encv-output")
    put("webdav_dir", "")
})
```

### Step 5: 验证

| 场景 | 预期行为 |
|------|----------|
| 桌面端启动（无 ENCV_MOBILE） | 忽略 `mobile` 段，使用 `server.dir="/"→CWD`, `output_path="./output"` |
| 移动端启动（ENCV_MOBILE=1, 有 mobile 段） | 使用 `mobile.server_dir`, `mobile.output_path`, `mobile.webdav_dir` |
| 移动端启动（ENCV_MOBILE=1, 无 mobile 段） | `cfg.Mobile == nil`，跳过整个分支，走原有默认行为 |
| 桌面端读取含 mobile 段的 config | `mobile` 段被 Unmarshal 但不影响任何运行时行为 |
| `go test ./internal/...` | 全部 PASS |
| 后端 `:2025` 启动正常 | ✅ |

---

## 变更范围

| 文件 | 操作 |
|------|------|
| [`internal/v2/types/types.go`](internal/v2/types/types.go) | 新增 `MobileConfig` 结构体 + `Config.Mobile` 字段 |
| [`internal/config/config.go`](internal/config/config.go) | 替换 Load() 中的 ENCV_MOBILE 修正逻辑 |
| [`config.user.json`](config.user.json) | 追加 `mobile` 段 |
| [`EncvGoService.kt`](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) | mergeConfigDefaults + writeFallbackConfig 增加 mobile 段处理 |

## 数据流对比

### 旧方案（已废弃，本次修复）

```
config.user.json:
  { "server": { "dir": "/" }, "output_path": "./output" }

                    ↓ Load() + ENCV_MOBILE 运行时覆盖
                    ↓ 同一个字段被修改！

移动端实际值:  server.dir="$HOME", output_path="$HOME/encv-output"
桌面端实际值:  server.dir="CWD",     output_path="./output"
                  ↑ 如果同步过来就炸了
```

### 新方案（本次实施）

```
config.user.json:
  {
    "server": { "port": 2025, "dir": "/" },
    "output_path": "./output",

    "mobile": {
      "server_dir": "/storage/emulated/0",
      "output_path": "/storage/emulated/0/encv-output",
      "webdav_dir": ""
    }
  }

桌面端 Load():  使用 server.dir="/", output_path="./output"  (忽略 mobile)
移动端 Load():  使用 mobile.server_dir, mobile.output_path (覆盖)

↑ 双向同步安全，互不干扰
```
