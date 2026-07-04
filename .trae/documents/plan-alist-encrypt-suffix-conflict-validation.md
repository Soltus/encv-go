# Plan: Alist-Encrypt 后缀冲突检测与插件扩展名唯一性保障

## 问题分析

### 当前状态

| 层级 | 现状 | 问题 |
|------|------|------|
| **后端 registry.go** | `initializeExtensions()` 用 `map[ext]=true` 收集所有插件容器扩展名 | **无冲突检测**，后注册的静默覆盖先注册的 |
| **后端 alistencrypt/plugin.go:75** | `reservedSuffixes = {".sccgv": ".encv"}` | 只硬编码了 2 个值，**未包含其他 5 个插件的 ext** |
| **前端 Settings.vue（已删除）** | 原 `CONFLICT_SUFFIXES = ['.sccgv', '.encv']` | 不完整，且随重构被移除 |
| **前端 PluginSettings.vue** | 无任何校验逻辑 | 用户可随意输入冲突后缀并保存成功 |
| **config.user.json 实际数据** | `alist_encrypt.suffix = ".sccgv"` **与 video.ext 冲突！** | 已存在真实冲突 |

### 核心区分（用户明确要求）

```
源文件扩展名 → 允许冲突
  例：.ts 可能是文本文件（TypeScript代码）也可能是视频文件
  → ENC V 通过 MIME 检测 + 插件优先级链自动分辨
  → 这是插件系统的能力，不应阻断

插件容器扩展名 → 禁止冲突
  例：video 用 .sccgv 做加密容器后缀，alist_encrypt 也用 .sccgv
  → IsContainer() 无法区分到底该由哪个插件解密
  → 必须在配置层阻断
```

### 防御深度要求

```
攻击面：
  ① 前端 PluginSettings.vue UI 编辑     → 前端实时校验 + 保存阻断
  ② 前端 API 调用 POST /api/config 直接改   → 后端 API 层校验拒绝
  ③ 第三方编辑器直接修改 config.user.json  → 后端启动/重载时检测 + 拒绝启动或降级警告
```

## 实现方案

### Step 1: 后端 — 扩展名冲突检测核心逻辑

**文件**: `internal/v2/plugins/registry.go`

新增函数：

```go
// ValidateExtensionUniqueness 检查所有插件容器扩展名是否唯一
// 返回冲突的扩展名列表及占用它们的插件名
func ValidateExtensionUniqueness() (conflicts []ExtensionConflict) {
    // 遍历 Plugins，收集 ext → pluginName 映射
    // 如果发现重复，记录到 conflicts
    // 返回 conflicts 供调用方决定处理策略
}

type ExtensionConflict struct {
    Extension    string   // 冲突的扩展名（如 ".sccgv"）
    PluginNames  []string // 声明此扩展名的插件列表（通常 2 个）
}
```

**关键**：此函数是纯检测，不做策略决策（不 abort、不修改值），由调用方决定怎么处理。

### Step 2: 后端 — 启动时 + 配置热重载时校验

**文件**: `internal/v2/plugins/registry.go` — 修改 `InitializePlugins()`

在所有插件 Initialize 完成后调用 `ValidateExtensionUniqueness()`：

- **有冲突**：输出 `slog.Error` 日志（含具体哪些插件冲突），**但不 abort 启动**
  - 理由：ENCV 作为服务不应因配置错误完全不可用；冲突只在加密/解密路径出问题
  - 后续 `FindDecryptingPlugin()` 遇到冲突容器时返回明确错误
- **无冲突**：正常继续

**文件**: `internal/server/server_config_api.go` 或 `mobile_api.go` — 修改 `POST /api/config` handler

在接收新配置并准备 Apply 时：
1. 临时解析 `plugin_settings` 中各插件的 ext 字段
2. 调用 `ValidateExtensionUniqueness()` 检测
3. **有冲突** → 返回 `400 Bad Request` + JSON 错误体 `{error: "container extension conflict", details: [...]}`
4. **无冲突** → 正常保存

### Step 3: 后端 — 新增查询 API

**文件**: `internal/server/mobile_api.go`

新增接口：

```
GET /api/plugins/container-extensions
Response: {
  "extensions": {
    "video": ".sccgv",
    "audio": ".sccga",
    "image": ".sccgi",
    "wps": ".sccgwps",
    "pdf": ".sccgpdf",
    "text": ".sccgt",
    "alist_encrypt": ".bin"
  },
  "conflicts": [
    { "extension": ".sccgv", "plugins": ["video", "alist_encrypt"] }
  ]
}
```

实现方式：遍历 `Plugins` 列表，调用每个插件的 `GetContainerExtension()` 构建 map + 检测重复。

### Step 4: 前端 — 新增 usePluginExtensions composable

**文件**: `src/composables/usePluginExtensions.ts`（新建）

职责：
- 调用 `GET /api/plugins/container-extensions` 获取所有插件容器扩展名
- 提供 `getConflictingPlugins(suffix: string): string[]` 方法
- 缓存结果，config 变更时失效重拉

### Step 5: 前端 PluginSettings.vue — 后缀输入实时校验

**文件**: `src/views/PluginSettings.vue`

在渲染 `plugin_settings.alist_encrypt.suffix` 的 `<ion-input>` 旁：

1. **实时冲突检测**：`@ionInput` 触发时调用 `getConflictingPlugins(newValue)`
2. **冲突警告 UI**：如果返回非空数组，显示红色提示条（遵循 frontend-design.md 错误展示规范）：
   > ⚠️ 后缀 `.xxx` 与以下插件的容器扩展名冲突：video
   > 加密容器扩展名必须唯一，请修改后缀。
3. **保存阻断**：如果 suffix 存在冲突，保存按钮 disable + tooltip 提示

### Step 6: 清理 — 移除后端 alistencrypt 硬编码的 reservedSuffixes

**文件**: `internal/v2/plugins/alistencrypt/plugin.go`

- 删除 L75 的 `reservedSuffixes` 硬编码
- 保留 Initialize() 中的格式校验（以 `.` 开头、长度 ≤16）
- 将冲突检测改为 warning 日志 + 继续使用用户值（让 registry.go 的统一检测负责）

## 文件变更清单

| # | 文件 | 操作 | 说明 |
|---|------|------|------|
| 1 | `internal/v2/plugins/registry.go` | 修改 | 新增 `ValidateExtensionUniqueness()` + `ExtensionConflict` 类型 + `InitializePlugins()` 末尾调用 |
| 2 | `internal/server/mobile_api.go` | 修改 | 新增 `GET /api/plugins/container-extensions` handler + `POST /api/config` 校验拦截 |
| 3 | `internal/v2/plugins/alistencrypt/plugin.go` | 修改 | 删除 `reservedSuffixes`，Initialize 改为 warn-only |
| 4 | `app/encv-mobile/src/composables/usePluginExtensions.ts` | **新建** | 封装扩展名查询 + 冲突检测 |
| 5 | `app/encv-mobile/src/views/PluginSettings.vue` | 修改 | suffix 输入框加实时校验 + 冲突提示 UI + 保存阻断 |
| 6 | `app/encv-mobile/src/api/encv.ts` | 修改 | 新增 `fetchContainerExtensions()` API 函数 |

## 验证方式

1. **前端 UI**：设置 `suffix = .sccgv` → 显示与 video 冲突 → 保存按钮禁用
2. **前端 UI**：设置 `suffix = .myenc` → 无冲突 → 正常保存
3. **后端 API**：`curl -X POST /api/config` 带冲突配置 → 返回 400 + 错误详情
4. **后端启动**：config.user.json 含冲突 → 日志 Error 但服务正常启动
5. **`vue-tsc --noEmit`** 零错误
6. **`go build ./cmd/encv-mobile`** 编译通过
7. **浏览器预览** 页面正常显示冲突提示
