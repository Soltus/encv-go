# 配置系统改进：`config.dev.json` + 合并覆盖机制

## 一、现状分析

### 当前问题
1. **无团队共享的 dev 配置**：新开发者接入时需要从零配置 `config.user.json`
2. **`config.user.json` 是全量副本**：76 行完整配置，维护成本高
3. **无分层合并机制**：`FindConfigPath()` 只查找单一 `config.user.json`

### 关键文件
| 文件 | 作用 | Git 状态 |
|------|------|----------|
| [config.go](internal/config/config.go) | 配置加载逻辑 | 已提交 |
| [config.user.json](config.user.json) | 开发者全量配置 | **保持提交（不忽略）** |
| [.gitignore](.gitignore) | 忽略规则 | **不添加 config 相关规则** |

---

## 二、改进目标

1. **新增 `config.dev.json`**：提交到 Git，**仅包含 `mobile.server_dir`**（极简 ~5 行）
2. **实现合并覆盖**：`config.user.json` → `config.dev.json`（**dev 覆盖 user，dev 优先级最高**）
3. **两个文件都提交 Git**：接力开发复用，都不加入 `.gitignore`

---

## 三、详细实现步骤

### Step 1: 创建 `config.dev.json`（极简）

**文件路径**: `/workspace/config.dev.json`
**Git 状态**: 提交（不加入 `.gitignore`）
**内容**: 仅包含移动端开发必需的 `server_dir` 字段

```json
{
  "mobile": {
    "server_dir": "/storage/emulated/0"
  }
}
```

**设计决策**：
- 只有 `mobile.server_dir` 一个差异化字段
- 其余全部继承 `DefaultConfig()` 或被 `config.user.json` 覆盖
- 新开发者 clone 后直接可用（移动端开发场景）

### Step 2: `.gitignore` 无需修改

确认 `.gitignore` 中**不出现**以下任何条目：
- ~~`config.user.json`~~ ❌ 不添加
- ~~`config.dev.json`~~ ❌ 不添加

两个配置文件都作为项目的一部分提交到版本控制。

### Step 3: 实现合并加载逻辑

**修改文件**: `/workspace/internal/config/config.go`

#### 3.1 新增 `mergeConfig()` — JSON Merge Patch（RFC 7386 简化版）

```go
// mergeConfig 将 overlay 中的字段递归合并到 base 上
// 语义：
//   - null 值不覆盖（保留 base）
//   - object 递归合并
//   - 标量/数组直接替换
func mergeConfig(base, overlay *Config) *Config {
    if overlay == nil {
        return base
    }

    baseData, err := json.Marshal(base)
    if err != nil {
        return base
    }
    overlayData, err := json.Marshal(overlay)
    if err != nil {
        return base
    }

    var baseMap, overlayMap map[string]interface{}
    if json.Unmarshal(baseData, &baseMap) != nil || json.Unmarshal(overlayData, &overlayMap) != nil {
        return base
    }

    deepMerge(baseMap, overlayMap)

    resultData, _ := json.Marshal(baseMap)
    var result Config
    if json.Unmarshal(resultData, &result) != nil {
        return base
    }
    result.Provider = base.Provider // json:"-" 字段保护
    return &result
}

func deepMerge(base, overlay map[string]interface{}) {
    for k, ov := range overlay {
        if ov == nil {
            continue
        }
        bv, ok := base[k]
        if !ok {
            base[k] = ov
            continue
        }
        bm, bo := bv.(map[string]interface{})
        om, oo := ov.(map[string]interface{})
        if bo && oo {
            deepMerge(bm, om)
        } else {
            base[k] = ov
        }
    }
}
```

#### 3.2 修改 `Load()` — 支持自动查找 + 合并

```go
// Load 加载配置。优先级（低→高）：
//   DefaultConfig() → config.user.json → config.dev.json（dev 最高优先级）
// 显式指定路径时走单文件模式（向后兼容）
func Load(configPath string) (*Config, error) {
    cfg := DefaultConfig()

    if configPath != "" {
        return loadSingleFile(cfg, configPath)
    }

    candidates := findMergeCandidates()
    if candidates == nil {
        slog.Info("No config files found, using defaults")
        return finalize(cfg), nil
    }

    if candidates.User != "" {
        cfg = loadAndMerge(cfg, candidates.User)
    }
    if candidates.Dev != "" {
        cfg = loadAndMerge(cfg, candidates.Dev)  // dev 后合并，覆盖 user
    }

    return finalize(cfg), nil
}

type mergeCandidates struct {
    Dev  string
    User string
}

func findMergeCandidates() *mergeCandidates {
    dirs := searchDirs()
    var c mergeCandidates
    for _, dir := range dirs {
        if c.Dev == "" && exists(filepath.Join(dir, "config.dev.json")) {
            c.Dev = filepath.Join(dir, "config.dev.json")
        }
        if c.User == "" && exists(filepath.Join(dir, "config.user.json")) {
            c.User = filepath.Join(dir, "config.user.json")
        }
        if c.Dev != "" && c.User != "" {
            break
        }
    }
    if c.Dev == "" && c.User == "" {
        return nil
    }
    return &c
}

func loadAndMerge(base *Config, path string) *Config {
    data, err := os.ReadFile(path)
    if err != nil {
        return base
    }
    var overlay Config
    if json.Unmarshal(data, &overlay) != nil {
        return base
    }
    return mergeConfig(base, &overlay)
}

func loadSingleFile(cfg *Config, path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return finalize(cfg), nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to read '%s': %w", path, err)
    }
    if json.Unmarshal(data, cfg) != nil {
        return nil, fmt.Errorf("failed to parse '%s': %w", path, err)
    }
    return finalize(cfg), nil
}

// finalize 执行 Load 的后处理逻辑（Server.Dir 规整 + MobileOverrides）
func finalize(cfg *Config) *Config {
    if cfg.Server.Dir == "/" {
        cfg.Server.Dir, _ = os.Getwd()
    }
    if os.Getenv("ENCV_MOBILE") == "1" && cfg.Mobile != nil {
        ApplyMobileOverrides(cfg)
    }
    return cfg
}

func searchDirs() []string {
    var dirs []string
    if wd, _ := os.Getwd(); wd != "" {
        dirs = append(dirs, wd)
    }
    if exe, _ := os.Executable(); exe != "" {
        dirs = append(dirs, filepath.Dir(exe))
    }
    return dirs
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
```

#### 3.3 保持向后兼容的 API

原有调用方式无需改动：

```go
// 生产环境：显式指定路径 → 单文件模式（不变）
cfg, err := config.Load("/etc/encv/config.json")

// 开发环境：自动查找 → 合并模式（新增能力）
cfg, err := config.Load("")  // 自动查找 dev + user 并合并
```

---

## 四、合并效果示例

### 场景：新开发者 clone 后直接运行

```
DefaultConfig():
  server.port=1999, server.dir="./", output_path="./encrypted", log.level="info"
  mobile=nil, plugin_settings={}

↓ 合并 config.user.json (如果存在):
  ... user 文件中的字段覆盖默认值 ...

↓ 再合并 config.dev.json (仅 mobile.server_dir，最高优先级):
  mobile.server_dir="/storage/emulated/0"  (覆盖 user 中的同名字段)
```

### 场景：只有 dev 没有 user

```
DefaultConfig() → config.dev.json → 最终配置
(省略的字段保持默认值)
```

### 场景：生产环境显式指定路径

```
Load("/etc/encv/config.json") → 单文件模式，不走合并（完全不变）
```

---

## 五、文件变更清单

| 操作 | 文件 | 说明 |
|------|------|------|
| **新建** | `config.dev.json` | 极简 dev 配置（仅 `mobile.server_dir`，~5 行） |
| **不修改** | `.gitignore` | 两个 config 文件都提交 Git |
| **修改** | `internal/config/config.go` | 新增 `mergeConfig()`、`deepMerge()`、`loadAndMerge()`、`findMergeCandidates()`；重构 `Load()` 入口 |

---

## 六、验证清单

- [ ] `config.dev.json` 和 `config.user.json` 都不被 `.gitignore` 忽略
- [ ] 无 `config.dev.json` 时行为与原来完全一致（向后兼容）
- [ ] 只有 `config.dev.json` 时正确合并（mobile.server_dir 生效，其余用默认值）
- [ ] `config.dev.json` + `config.user.json` 都存在时 **dev 覆盖 user**（dev 优先级最高）
- [ ] `Load("/explicit/path.json")` 走单文件模式不受影响
- [ ] `ApplyMobileOverrides()` 在合并后仍正确执行
- [ ] `Provider`（`json:"-"`）在合并中不丢失
- [ ] `go build ./...` 编译通过
- [ ] `go test ./internal/config/...` 测试通过
