# 致命漏洞修复：WebDAV 路由防御 + 启动容错

## 问题根因分析

### 攻击链路
```
用户在设置页输入 webdav.root="/"
  → 前端无验证，直接 PUT /api/config 保存成功
  → config.user.json 写入 {"webdav":{"root":"/"}}
  → 后端重启时 s.Start() 调用 checkWebdavRouteConflict("/")
  → cleanRoot="" → strings.HasPrefix("api","")=true → 返回冲突
  → Start() 返回 error → log.Fatalf() 进程退出
  → Android EncvGoService 重试启动 → 同样崩溃 → 永久死循环 ⚠️
```

### 三个具体问题

| # | 问题 | 根因位置 | 影响 |
|---|------|---------|------|
| 1 | WebDAV 路由字段显示路径选择图标 | `schemaParser.ts:L144` — `isPathField()` 把 `'root'` 列入 `pathKeys` | 用户误以为这是文件系统路径，输入 `/` |
| 2 | 配置保存无路由校验 | `server_config_api.go` — `handlePutConfigGin()` 只检查 JSON 合法性 | 恶意值 `"/"` 被写入磁盘 |
| 3 | 启动时路由冲突直接 Fatal | `servers.go:L24` + `server.go:L142-144` — 冲突时返回 error → `log.Fatalf()` | 无法恢复的崩溃循环 |

---

## 修复方案

### Step 1: 前端 — WebDAV root 字段不显示路径选择图标

**文件**: [schemaParser.ts](app/encv-mobile/src/config/schemaParser.ts) L143-146

**当前代码**:
```typescript
function isPathField(key: string): boolean {
  const pathKeys = ['output_path', 'dir', 'file', 'plugin_cache_dir', 'root']
  return pathKeys.includes(key) || key.includes('_path') || key.includes('_dir')
}
```

**问题**: `'root'` 在 `pathKeys` 中，导致 WebDAV 的 `root`（URL 路由前缀）被标记为 `isPath: true`，Settings.vue 显示 📁 文件夹选择按钮。

**修复**: 从 `pathKeys` 中移除 `'root'`。WebDAV 的 `root` 是 URL 路由（如 `/webdav/`），不是文件路径；而 WebDAV 的 `dir` 和 Server 的 `dir` 已经由 `_dir` 后缀匹配覆盖。

```typescript
function isPathField(key: string): boolean {
  const pathKeys = ['output_path', 'dir', 'file', 'plugin_cache_dir']
  return pathKeys.includes(key) || key.includes('_path') || key.includes('_dir')
}
```

### Step 2: 前端 — WebDAV root 输入时拦截唯一致命值

**文件**: [Settings.vue](app/encv-mobile/src/views/Settings.vue) `handleInput()`

**原则**: 前端只做 **确定性拦截**——唯一能导致后端崩溃的值就是 `/`。其他任何自定义路由（如 `/webdav/`、`/dav`、`/my-files/`）在技术上都合法，冲突检测交给后端 `checkWebdavRouteConflict` 精确判断。

**方案**: 在 `handleInput` 中增加对 `webdav.root` 字段的轻量校验：

```typescript
function handleInput(path: string[], field: FieldDef, event: CustomEvent) {
  const val = (event.target as HTMLInputElement).value
  if (path.length >= 2 && path[0] === 'webdav' && path[1] === 'root' && val) {
    const err = validateWebdavRoute(val)
    if (err) {
      showToast({ message: err, duration: 3000, color: 'danger' })
      return
    }
  }
  if (field.type === 'integer') {
    setFieldValue(path, val ? Number(val) : 0)
  } else {
    setFieldValue(path, val)
  }
}
```

辅助函数（只拦截确定致命的值）：
```typescript
function validateWebdavRoute(val: string): string | null {
  const t = val.trim()
  if (!t) return null
  if (t === '/' || t === '//') return 'WebDAV 路由不能为 "/"，这会导致服务崩溃'
  if (!t.startsWith('/')) return 'WebDAV 路由必须以 "/" 开头'
  return null
}
```

**校验策略说明**：
| 输入值 | 前端行为 | 后端行为 |
|--------|---------|---------|
| `/` | ❌ 拦截（确定致命） | N/A |
| `//` | ❌ 拦截（等价于 `/`） | N/A |
| `webdav`（无前导 `/`） | ❌ 提示必须以 `/` 开头 | N/A |
| `` （空） | ✅ 放行（=禁用 WebDAV） | ✅ |
| `/webdav/` | ✅ 放行 | ✅ checkWebdavRouteConflict 通过 |
| `/dav` | ✅ 放行 | ✅ checkWebdavRouteConflict 通过 |
| `/api-custom/` | ✅ 放行 | ✅ 不与 `/api/` 冲突 |
| `/api` | ✅ 放行 | ❌ 后端 API 返回 400 冲突错误 |

### Step 3: 后端 — 配置保存 API 增加路由校验

**文件**: [server_config_api.go](internal/server/server_config_api.go) `handlePutConfigGin()`

**位置**: 在 JSON 解析后（L52-55 之后）、写文件前（L77 之前），插入 WebDAV 路由校验：

```go
if wd, ok := raw["webdav"].(map[string]interface{}); ok {
    if root, ok := wd["root"].(string); ok && root != "" {
        if errMsg := validateWebdavRouteForSave(root); errMsg != "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
            return
        }
    }
}
```

校验函数（可放在 server.go 的 `checkWebdavRouteConflict` 附近或新建 `server_validation.go`）：

```go
func validateWebdavRouteForSave(root string) string {
    cleaned := strings.TrimSpace(root)
    if cleaned == "/" || cleaned == "//" {
        return "webdav root cannot be '/' (would capture all routes)"
    }
    if !strings.HasPrefix(cleaned, "/") {
        return "webdav root must start with '/'"
    }
    normalized := cleaned
    if !strings.HasSuffix(normalized, "/") {
        normalized += "/"
    }
    normalizedClean := strings.TrimSuffix(normalized, "/")
    for _, prefix := range knownRoutePrefixes {
        cleanPrefix := strings.TrimSuffix(prefix, "/")
        if strings.HasPrefix(cleanPrefix, normalizedClean) || strings.HasPrefix(normalizedClean, cleanPrefix) {
            return fmt.Sprintf("webdav root '%s' conflicts with system route '%s'", root, prefix)
        }
    }
    return ""
}
```

### Step 4: 后端 — 启动时 WebDAV 路由冲突降级处理（不崩溃）

**文件**: [server.go](internal/server/server.go) L121-146（Start() 函数中的 WebDAV 初始化块）

**当前行为**: `checkWebdavRouteConflict` 失败 → `return "", error` → 调用方 `log.Fatalf` → 进程退出

**修改为**: 冲突时 **禁用 WebDAV 并继续启动**，记录警告日志但不崩溃：

```go
if s.cfg.Webdav.Root != "" {
    // ... 解析 webdavDir/webdavPath ...

    if conflict := checkWebdavRouteConflict(s.webdavPath); conflict != "" {
        slog.Error("WebDAV route conflicts with existing route, DISABLING WebDAV to avoid crash",
            "webdav_path", s.webdavPath,
            "conflict_with", conflict,
        )
        s.webdavDir = ""
        s.webdavPath = ""
        // 可选：自动修正配置文件中的值
        _ = sanitizeWebdavRootInConfig(s.configPath)
    } else {
        slog.Info("WebDAV enabled", "dir", s.webdavDir, "path", s.webdavPath)
    }
}
```

自动修正函数（将错误的 webdav.root 重置为默认值）：

```go
func sanitizeWebdavRootInConfig(configPath string) error {
    data, err := os.ReadFile(configPath)
    if err != nil {
        return err
    }
    var cfg map[string]interface{}
    if err := json.Unmarshal(data, &cfg); err != nil {
        return err
    }
    wd, ok := cfg["webdav"].(map[string]interface{})
    if !ok {
        return nil
    }
    wd["root"] = ""  // 清空以禁用 WebDAV
    updated, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(configPath, append(updated, '\n'), 0644)
}
```

### Step 5: 增强 `checkWebdavRouteConflict` 对 "/" 的专门检测

**文件**: [server.go](internal/server/server.go) L92-101

在现有逻辑之前增加显式的 `/` 拦截：

```go
func checkWebdavRouteConflict(webdavRoot string) string {
    cleanRoot := strings.TrimSuffix(strings.TrimSpace(webdavRoot), "/")

    if cleanRoot == "" || cleanRoot == "/" {
        return "<root>"  // 特殊标记：表示是根路径
    }

    for _, prefix := range knownRoutePrefixes {
        cleanPrefix := strings.TrimSuffix(prefix, "/")
        if strings.HasPrefix(cleanPrefix, cleanRoot) || strings.HasPrefix(cleanRoot, cleanPrefix) {
            return prefix
        }
    }
    return ""
}
```

---

## 文件修改清单

| 文件 | 修改类型 | 内容 |
|------|---------|------|
| `src/config/schemaParser.ts` | Bug fix | `isPathField()` 移除 `'root'` |
| `src/views/Settings.vue` | Feature | 增加 `validateWebdavRoute()` + `handleInput` 校验 + 错误提示 |
| `internal/server/server_config_api.go` | Feature | `handlePutConfigGin()` 增加 WebDAV 路由校验 |
| `internal/server/server.go` | Bug fix | `checkWebdavRouteConflict()` 增强 "/" 检测; `Start()` WebDAV 冲突时降级而非崩溃 |

## 验证方式

1. **前端构建**: `npx vue-tsc --noEmit && npx vite build`
2. **Go 编译**: `cd /workspace && go build ./cmd/encv/`
3. **手动测试**:
   - 设置 webdav.root="/" → 前端应阻止并显示错误提示
   - 设置 webdav.root="//" → 前端应阻止
   - 设置 webdav.root="webdav"（无前导 `/`）→ 前端应提示必须以 `/` 开头
   - 设置 webdav.root="/webdav/" → 应正常保存
   - 设置 webdav.root=""（清空）→ 应正常保存（禁用 WebDAV）
   - 直接修改 config.user.json 中 `webdav.root="/"` → 重启不应崩溃，WebDAV 被禁用并自动修正配置
