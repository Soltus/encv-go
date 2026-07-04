# 修复 config.user.json + 配置结构统一优化

## 问题分析

### 表面问题
之前为对齐 Vite proxy 端口直接修改了 `config.user.json`，导致大量字段丢失。

### 根本问题（用户指出）
- `config.mobile.json` 和 `config.user.json` 是**两套独立配置**，没有派生关系
- 用户无法在桌面端和移动端间同步配置
- 维护成本高、容易不一致

### 当前配置加载链路

```
桌面端:
  config.user.json (CWD或exe目录) → Go Load() → Config

移动端 (EncvGoService.kt):
  assets/config.mobile.json ──copy/merge──→ filesDir/config.user.json
                                                    ↓
                                          ENCV_CONFIG_PATH env → Go Load() → Config
                                          HOME=filesDir
                                          ENCV_MOBILE=1
```

**问题**: assets 里放的是 `config.mobile.json`，但运行时文件叫 `config.user.json`。两套文件需要手动同步。

---

## 优化方案：单一配置模板 + 平台感知运行时修正

### 核心思路

1. **只维护一个 `config.user.json`** 作为唯一配置模板（assets + 仓库根目录）
2. **删除 `config.mobile.json`**
3. **平台差异值由代码在运行时根据 `ENCV_MOBILE` 自动修正**，而非维护两份配置文件

### 配置字段分类

| 类别 | 字段 | 桌面端值 | 移动端值 | 处理方式 |
|------|------|---------|---------|----------|
| **通用** | password, port, plugin_settings, log.level, proxy.sites | 双端相同 | 双端相同 | 用户自行设置 |
| **路径(自动修正)** | server.dir | `"./"` (CWD) | `$HOME` (filesDir) | Go Load() 修正 |
| **路径(自动修正)** | output_path | `"./output"` | `$HOME/encv-output` | Go Load() 修正 |
| **路径(自动修正)** | webdav.dir | `"./output"` | `""` (禁用本地WebDAV) | Go Load() 修正 |
| **移动端特有** | webdav.username/password | 有值 | 空字符串 | 保持空即可 |
| **过时(删除)** | admin.port, webdav.port | ~~1808~~ / ~~1234~~ | — | 类型定义已无此字段 |

---

## 执行步骤

### Step 1: 恢复并优化 config.user.json

基于 GitHub main 版本，做以下调整：

**保留的字段（完整用户配置模板）：**

```json
{
  "$schema": "https://raw.githubusercontent.com/Soltus/encv-go/main/config.schema.json",
  "password": "my-encv_key，可以使用中文和标点符号✔",
  "output_path": "./output",
  "server": {
    "port": 2025,
    "dir": "/"
  },
  "admin": {
    "password": "123456"
  },
  "proxy": {
    "sites": {
      "pc": { "host": "http://localhost:5244", "description": "电脑上的openlist" },
      "vivo": { "host": "http://192.168.31.19:5244", "description": "手机上的openlist" },
      "pc_dev": { "host": "http://localhost:5234", "description": "电脑上定制版的openlist" }
    }
  },
  "log": {
    "level": "info",
    "file": "encv.log"
  },
  "webdav": {
    "root": "/webdav/",
    "dir": "./output",
    "username": "admin",
    "password": "123456"
  },
  "plugin_settings": {
    "video": {
      "ext": ".sccgv",
      "chunk_size_mb": 0,
      "light_main_chunk_enabled": true,
      "verify_after_pack": true,
      "track_extensions": ".ass,.srt,.dm.ass,.vtt",
      "skip_merge_for_split_mkv": false
    },
    "image": { "ext": ".sccgi" },
    "audio": { "ext": ".sccga" },
    "text": { "ext": ".sccgt" },
    "wps": { "ext": ".sccgwps" },
    "pdf": { "ext": ".sccgpdf" }
  }
}
```

**相比 GitHub main 的变更：**
- ❌ 删除 `admin.port: 1808`（AdminServer 类型已无 Port 字段）
- ❌ 删除 `webdav.port: 1234`（WebdavServer 类型已无 Port 字段）
- ❌ 删除 `plugin_settings.video.plugin_cache_dir: "D:/TEMP/encv-cache"`（Windows 绝对路径，跨平台不兼容）
- ✅ 其余全部保留

### Step 2: 还原 DefaultConfig 默认端口

[`config.go:83`](internal/config/config.go#L83) `Port: 2025` → 改回 `Port: 1999`

理由：DefaultConfig 是无配置文件时的兜底值，应与用户配置模板解耦。

### Step 3: Go 端 Load() 增加移动端路径自动修正

修改 [`internal/config/config.go:Load()`](internal/config/config.go#L101)，在 Unmarshal 之后增加：

```go
if os.Getenv("ENCV_MOBILE") == "1" {
    if cfg.Server.Dir == "/" || cfg.Server.Dir == "" || cfg.Server.Dir == "." {
        cfg.Server.Dir = os.Getenv("HOME")
    }
    if cfg.OutputPath == "" || cfg.OutputPath == "./encrypted" {
        cfg.OutputPath = filepath.Join(os.Getenv("HOME"), "encv-output")
    }
    if cfg.Webdav.Dir == "" || cfg.Webdav.Dir == "./output" {
        cfg.Webdav.Dir = ""
    }
}
```

这样：
- 移动端即使用户从未编辑配置文件，路径也会自动正确
- 桌面端行为不变（现有 L118-123 的 `/` → CWD 逻辑保留）
- 用户只需维护一份 config.user.json

### Step 4: Kotlin 侧改为使用统一 config.user.json

修改 [`EncvGoService.kt`](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt):

**4a. `copyDefaultConfig()` (L409)**:
```kotlin
assets.open("config.user.json").use { input ->   // ← 改为 config.user.json
```

**4b. `mergeConfigDefaults()` (L423)**:
```kotlin
val defaults = try {
    JSONObject(assets.open("config.user.json").bufferedReader().use { it.readText() })  // ← 改为 config.user.json
} catch ...
```

**4c. `writeFallbackConfig()` (L474)**:
保持不变（硬编码的最小 fallback，只在 assets 读取失败时使用）。

### Step 5: 删除 config.mobile.json

```bash
rm app/encv-mobile/assets/config.mobile.json
```

assets 中不再需要此文件。

### Step 6: 添加项目规则

在 [`.trae/rules/project_rules.md`](.trae/rules/project_rules.md) 末尾追加：

```markdown
## 配置模板保护（重要！）

- **严禁擅自修改 `config.user.json`**：该文件是唯一用户配置模板（桌面端+移动端共用），任何端口/路径/密码等值的修改必须通过用户明确指令执行
- 如需临时改变开发端口等参数，应使用环境变量 `ENCV_CONFIG_PATH` 指向临时配置文件，或命令行 `--config` 标志
- **不得创建独立的 `config.mobile.json` 或其他平台特定配置模板**：移动端适配通过 Go 端 `Load()` 中的 `ENCV_MOBILE` 路径自动修正实现
- 违反此规则导致的配置模板破坏将被视为严重错误
```

---

## 变更影响范围

| 文件 | 操作 | 说明 |
|------|------|------|
| `config.user.json` | **重写** | 恢复完整模板 + 删除3个过时字段 |
| `internal/config/config.go` | **修改** | DefaultConfig 端口还原 1999；Load() 增加 ENCV_MOBILE 路径修正 |
| `EncvGoService.kt` | **修改** | copyDefaultConfig/mergeConfigDefaults 改读 config.user.json |
| `app/encv-mobile/assets/config.mobile.json` | **删除** | 不再需要 |
| `.trae/rules/project_rules.md` | **追加** | 配置保护规则 |

## 验证

1. `diff config.user.json <(curl -sL https://raw.githubusercontent.com/.../config.user.json)` — 确认只有预期的 3 个字段差异
2. `go test ./internal/... -run TestConfig` — 配置相关测试通过
3. `go run ./cmd/encv/ start` — 使用恢复后的 config.user.json 在 `:2025` 启动成功
4. 前端 `npm run dev` → API 代理到 `:2025` 正常
