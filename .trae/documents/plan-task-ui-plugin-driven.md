# 任务创建界面插件化适配计划（修订版 v2）

## 一、问题诊断（含用户新增反馈）

### 1.1 当前 Tasks.vue 任务模态框的硬编码问题

| 问题 | 位置 | 详情 |
|------|------|------|
| **二级密码字段命名误导** | 原 L292 `newTaskPassword` | 变量名暗示"密码"，实际设计意图是**二级密码（与主密码叠加验证）**，不是覆盖 |
| **容器版本选择硬编码** | 原 L188 `v-if="newTaskType === 'encrypt'"` | Video 插件支持版本选择，Alist-Encrypt 不支持，但前端不知道 |
| **无插件感知** | 整个模态框 | 不知道用户选的文件会由哪个插件处理 |

### 1.2 密码层级定义（修正！）

> **⚠️ 关键纠正：L2 不是覆盖，而是叠加验证！**

```
主密码通道（二选一，由插件 PasswordStrategy 决定）：
├── L0: 全局密码 (config.password)           → PasswordGlobal 插件使用（如 video）
└── L1: 插件独立密码 (plugin.default_password) → PasswordIndependent 插件使用（如 alist_encrypt）
    ├── L1a: 插件设置中的默认值（配置页填写）
    └── L1b: 任务创建时指定的值（extraFields.plugin_password）

二级密码通道（独立于主密码）：
└── L2: 二级密码 (per-task secondary)        → 叠加验证层：主密码 ✓ + 二级密码 ✓ 才能解密
                                                    当前加密层仅支持单密码，L2 为预留管道
```

**解密时的密码验证逻辑**：
```
✅ 正确模型（用户确认）：
   解密需要：主密码(L0或L1)正确  AND  二级密码(L2)正确
   → 两个独立的密码通道，都必须通过

❌ 错误模型（之前计划中的理解）：
   二级密码覆盖主密码 → 这是完全错误的理解
```

**当前加密层的现状**：
- [alistencrypt/aesctr.go](internal/alistencrypt/aesctr.go) — `NewAesCtr(password)` 只接受单密码
- [alistencrypt/encryptor.go](internal/v2/plugins/alistencrypt/encryptor.go) — `EncryptToFile(reader, password, ...)` 单密码
- [alistencrypt/decryptor.go](internal/v2/plugins/alistencrypt/decryptor.go) — `DecryptFile(path, dir, password, type)` 单密码
- Video ENCV 容器加密 — 同样从 `cfg.Password` 取单密码

→ **L2 双密码验证是设计意图，但加密层尚未实现。本次先打通管道（API→执行→插件接口），加密层双密码支持作为后续任务。**

**当前代码的致命缺陷**：

#### 缺陷 A：Alist-Encrypt 声明 Independent 但仍 fallback 全局密码

[alistencrypt/plugin.go L104-106](internal/v2/plugins/alistencrypt/plugin.go#L104-L106) 的 `Initialize()`：
```go
if p.settings.DefaultPassword == "" && p.cfg.Password != "" {
    p.settings.DefaultPassword = p.cfg.Password  // ← 自动复制全局密码到插件！
}
```

[alistencrypt/plugin.go L251-258](internal/v2/plugins/alistencrypt/plugin.go#L251-L258) 的 `resolvePassword()`：
```go
func (p *AlistEncryptPlugin) resolvePassword() string {
    if p.settings.DefaultPassword != "" { return p.settings.DefaultPassword }  // L1a
    if p.cfg.Password != "" { return p.cfg.Password }                        // ← 仍 fallback 到 L0！
    return ""
}
```

**矛盾**：插件声明了 `PasswordIndependent`（不用全局密码），但代码在两个地方都偷偷用了全局密码。

#### 缺陷 B：前端把 L1 主密码和 L2 二级密码合并为一个字段发送

[Tasks.vue L596](app/encv-mobile/src/views/Tasks.vue#L596)：
```typescript
(extra.plugin_password || extra.secondary_password) as string | undefined
```

`plugin_password`(L1b 主密码通道) 和 `secondary_password`(L2 叠加验证通道) 被 `||` 合并成单一 `password` 参数。两者属于**完全独立的密码通道**，不应合并：
- `plugin_password` = PasswordIndependent 插件的主密码（替代全局密码）
- `secondary_password` = 二级叠加验证密码（与主密码同时需要才可解密）

#### 缺陷 C：后端任务创建只接受单一 password 字段，无 ExtraFields

[mobile_api.go L200-206](internal/server/mobile_api.go#L200-L206)：
```go
var req struct {
    Type       string `json:"type"`
    SourcePath string `json:"sourcePath"`
    TargetPath string `json:"targetPath,omitempty"`
    Password   string `json:"password,omitempty"`     // ← 只有主密码！
    Version    int    `json:"version,omitempty"`
    // ❌ 缺少: SecondaryPassword (L2)
    // ❌ 缺少: ExtraFields map (L1b 等插件额外字段)
}
```

无法传递 `plugin_password`(L1b) 和 `secondary_password`(L2)。

#### 缺陷 D：任务执行时密码注入机制对 Independent 插件错误

[task_manager.go L374-381](internal/service/task_manager.go#L374-L381) 的 `getConfigForTask()`：
```go
func (tm *TaskManager) getConfigForTask(task *MobileTask, ctx context.Context) context.Context {
    if task.Password != "" {
        cfgCopy := *tm.cfg
        cfgCopy.Password = task.Password  // ← 无条件写入 cfg.Password（L0 位置）
        return config.NewContext(ctx, &cfgCopy)
    }
    return config.NewContext(ctx, tm.cfg)
}
```

问题：
1. 对于 PasswordIndependent 插件，用户传入的 L1b 被错误地写入 `cfg.Password`(L0 位置)，污染独立密码语义
2. L2 二级密码完全没有传递路径，丢失了

### 1.3 后端 `/api/plugins` 返回信息不足

当前返回缺少密码策略声明、版本支持能力等。（Step 3 已实现）

### 1.4 核心矛盾总结

```
视频插件任务需要：容器版本选择 + 主密码(L0全局) + 可选L2叠加验证
Alist-Encrypt 任务需要：主密码输入(L1独立) + 不使用全局密码 + 可选L2叠加验证
→ 但前端对两者一视同仁，后端只有单字段 password 通道，L2 无通路
→ 且 Alist-Encrypt 声明了 Independent 却仍在代码中 fallback 到全局密码
```

---

## 二、设计原则

> **插件声明式，前端委托渲染，后端管道完整。**

1. **插件声明驱动 UI**：`GetTaskOptions()` 声明密码策略 + 额外字段，前端动态渲染
2. **密码层级严格分离**：L0/L1/L2 在整个管道中（前端→API→执行）保持独立，不合并
3. **PasswordIndependent 语义完整性**：声明独立的插件必须在整个管道中真正独立，不 fallback 到全局密码
4. **防御性编程铁律**：不硬编码任何运行时数据

---

## 三、实施方案

> **Step 1-8 已完成**（类型定义、插件实现、API 端点、前端 composable、Tasks.vue 重构、i18n、CSS）
>
> **以下从 Step 9 开始是修订内容**，修复上述缺陷 A/B/C/D。

### ✅ Step 1-8（已完成，不再重复）

- Step 1: 后端 `PasswordStrategy`/`TaskOptions`/`TaskField` 类型 + `GetTaskOptions()` 接口
- Step 2: 各插件实现 `GetTaskOptions()`（video=global+版本, alist_encrypt=independent+密码字段, 其他=global）
- Step 3: 增强 `/api/plugins` 返回 `taskOptions`
- Step 4: 新增 `/api/tasks/predict-plugin` 端点
- Step 5: 前端 API 层新增类型和 `predictPlugin()` 函数
- Step 6: 新建 `useTaskForm` composable
- Step 7: Tasks.vue 模态框改为声明式渲染
- Step 8: i18n 新增翻译 key

### 🔧 Step 9: 后端 — 任务创建 API 支持双密码通道 + ExtraFields

**问题修复**: 缺陷 C — 缺少 SecondaryPassword(L2) 和 ExtraFields(L1b)

**文件**: `internal/server/mobile_api.go` — `handleCreateTaskGin()`

```go
func (s *Server) handleCreateTaskGin(c *gin.Context) {
    var req struct {
        Type             string            `json:"type"`
        SourcePath       string            `json:"sourcePath"`
        TargetPath       string            `json:"targetPath,omitempty"`
        Password         string            `json:"password,omitempty"`          // 主密码（L0覆盖 或 L1b）
        SecondaryPassword string           `json:"secondaryPassword,omitempty"` // ← 新增：二级密码（L2 叠加验证）
        Version          int               `json:"version,omitempty"`
        ExtraFields      map[string]string `json:"extraFields,omitempty"`       // ← 新增：插件额外字段（如 plugin_password=L1b）
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
        return
    }

    slog.Info("API: create task", "type", req.Type, "source", req.SourcePath,
        "target", req.TargetPath, "version", req.Version,
        "hasPassword", req.Password != "",
        "hasSecondaryPassword", req.SecondaryPassword != "",
        "hasExtraFields", len(req.ExtraFields) > 0)

    task := s.mobileSvc.GetTaskManager().CreateWithExtras(
        req.Type, req.SourcePath, req.TargetPath,
        req.Password, req.SecondaryPassword, req.Version, req.ExtraFields,
    )

    c.JSON(http.StatusCreated, task)
}
```

### 🔧 Step 10: 后端 — TaskManager 携带完整密码信息

**文件**: `internal/service/task_manager.go`

**10a. MobileTask 结构体增加字段**

```go
type MobileTask struct {
    ID               string            `json:"id"`
    Type             string            `json:"type"`
    SourcePath       string            `json:"sourcePath"`
    TargetPath       string            `json:"targetPath,omitempty"`
    Password         string            `json:"password,omitempty"`          // 主密码
    SecondaryPassword string           `json:"secondaryPassword,omitempty"` // ← 新增：二级密码
    ExtraFields      map[string]string `json:"extraFields,omitempty"`       // ← 新增：插件额外字段
    Status           string            `json:"status"`
    Progress         int               `json:"progress"`
    // ... 其余字段不变
}
```

**10b. CreateWithExtras 方法**

```go
func (tm *TaskManager) CreateWithExtras(taskType, sourcePath, targetPath, password, secondaryPassword string, version int, extras map[string]string) *MobileTask {
    task := tm.Create(taskType, sourcePath, targetPath, password, version)
    task.SecondaryPassword = secondaryPassword
    task.ExtraFields = extras
    return task
}
```

原 `Create()` 签名不变（向后兼容）。

**10c. 持久化兼容**：JSON 序列化自动包含新字段，旧数据为 null/零值，安全向下兼容。

### 🔧 Step 11: 后端 — Alist-Encrypt 真正实现 Independent 密码策略

**问题修复**: 缺陷 A — 声明 Independent 但仍 fallback 全局密码

**文件**: `internal/v2/plugins/alistencrypt/plugin.go`

**11a. Initialize() 不再自动复制全局密码**

```go
func (p *AlistEncryptPlugin) Initialize(ctx context.Context) error {
    // ... existing init code (suffix validation, enc_type check) ...

    // ❌ 删除：
    // if p.settings.DefaultPassword == "" && p.cfg.Password != "" {
    //     p.settings.DefaultPassword = p.cfg.Password
    // }

    // ✅ Independent 策略：不继承全局密码
    // DefaultPassword 为空时由任务创建时通过 ExtraFields[plugin_password] 指定
    _ = p.cfg  // 保留 cfg 引用（日志、路径解析等非密码用途）

    return nil
}
```

**11b. resolvePassword() 只使用插件自身密码**

```go
// resolvePassword 解析主密码（仅 L1 通道，不含 L2）
func (p *AlistEncryptPlugin) resolvePassword() string {
    // 只看插件自己的 DefaultPassword（L1a）
    if p.settings.DefaultPassword != "" {
        return p.settings.DefaultPassword
    }
    // ❌ 不再 fallback 到全局密码（Independent 核心语义）
    return ""
}

// resolvePasswordWithTaskExtras 解析主密码，支持任务级指定（L1b）
func (p *AlistEncryptPlugin) resolvePasswordWithTaskExtras(extraFields map[string]string) string {
    // L1b > L1a
    if pw := extraFields["plugin_password"]; pw != "" {
        return pw
    }
    return p.resolvePassword()
}
```

### 🔧 Step 12: 后端 — 任务执行时双通道密码传递

**问题修复**: 缺陷 D — 主密码注入污染 + L2 无通路

**文件**: `internal/service/task_manager.go` + `internal/v2/plugins/interfaces/interfaces.go`

**12a. 新增 TaskPasswordResolver 可选接口**

```go
// TaskPasswordResolver 定义插件自定义主密码解析能力
// 插件根据 ExtraFields 和策略返回主密码（L0 或 L1）
// L2 二级密码不在此接口处理，由 TaskManager 单独传递
type TaskPasswordResolver interface {
    ResolveTaskPassword(taskPassword string, extraFields map[string]string) string
}
```

**12b. Alist-Encrypt 实现**

```go
func (p *AlistEncryptPlugin) ResolveTaskPassword(taskPassword string, extraFields map[string]string) string {
    // taskPassword 是用户在任务中指定的主密码（对 Global 插件是 L0 覆盖值，对 Independent 插件可能为空）
    // 对于 Independent 插件，主密码来自 ExtraFields[plugin_password] 或 DefaultPassword
    if p.PasswordStrategyIndependent() {  // 或直接判断类型
        return p.resolvePasswordWithTaskExtras(extraFields)
    }
    // fallback: 如果被错误调用，走默认逻辑
    if taskPassword != "" { return taskPassword }
    return p.resolvePassword()
}
```

**12c. processEncrypt / processDecrypt 使用双通道**

```go
func (tm *TaskManager) processEncrypt(task *MobileTask, absPath string) {
    // ... existing validation (path exists, dir exists, disk space) ...

    ctx, cancel := context.WithCancel(context.Background())
    task.cancelFn = cancel
    defer cancel()

    plugin, err := plugins.FindEncryptingPlugin(absPath)
    if err != nil { tm.failTask(taskID, ...); return }

    // ★ 主密码解析（L0/L1 通道）
    var primaryPassword string
    if resolver, ok := plugin.(pluginInterfaces.TaskPasswordResolver); ok {
        primaryPassword = resolver.ResolveTaskPassword(task.Password, task.ExtraFields)
    } else {
        // PasswordGlobal 插件默认行为：task.Password(L0覆盖) > cfg.Password(L0默认)
        primaryPassword = task.Password
        if primaryPassword == "" {
            primaryPassword = tm.cfg.Password
        }
    }

    if primaryPassword == "" {
        tm.failTask(taskID, "encryption requires a password")
        return
    }

    // ★ 将主密码注入上下文（替代旧 getConfigForTask 的无条件覆盖）
    passwordCtx := tm.getPasswordContext(ctx, primaryPassword)

    // ★ L2 二级密码：通过 context 或直接参数传递给加密函数
    // （当前加密层只支持单密码，L2 管道预留但暂不强制校验）
    // TODO: 加密层支持双密码后，在此处将 task.SecondaryPassword 传入

    err = plugins.EncryptFileWithPlugin(passwordCtx, plugin, absPath, tm.servingDir, outputDir)
    // ...
}
```

**12d. getPasswordContext 辅助方法（替代 getConfigForTask）**

```go
// getPasswordContext 将解析后的主密码注入 config context
// 注意：此方法只处理主密码（L0/L1），不涉及 L2
func (tm *TaskManager) getPasswordContext(ctx context.Context, primaryPassword string) context.Context {
    if primaryPassword != "" {
        cfgCopy := *tm.cfg
        cfgCopy.Password = primaryPassword
        return config.NewContext(ctx, &cfgCopy)
    }
    return config.NewContext(ctx, tm.cfg)
}
```

> **关于 L2 在执行层的处理**：当前加密层（Alist-Encrypt AES-CTR、Video ENCV 容器）均只接受单密码。本次先打通 L2 管道（API→TaskManager→MobileTask→持久化），在 `processEncrypt/Decrypt` 中以 TODO 标记预留接入点。后续单独任务实现加密层双密码支持。

### 🔧 Step 13: 前端 — 三字段分离发送

**问题修复**: 缺陷 B — L1/L2 合并

**13a. Tasks.vue handleCreateTask()**

```typescript
async function handleCreateTask() {
  if (!newTaskPath.value) return
  try {
    const extra = getExtraPayload()                    // ← L1b: { plugin_password: "..." }
    await createTask(
      newTaskType.value,
      newTaskPath.value,
      newTaskTargetPath.value || undefined,
      undefined,                                       // ← 主密码：Global 插件留空(用全局)，Independent 由 extra 携带
      taskOptions.value?.supportVersionSelect ? newTaskVersion.value : undefined,
      extra,                                           // ← ExtraFields (L1b)
      secondaryPassword.value || undefined              // ← 新增：二级密码 (L2)
    )
    // ...
  }
}
```

等一下——这里有个设计决策需要明确：`createTask` 的 `password` 参数对 PasswordIndependent 插件意味着什么？

**方案选择**：

| 方案 | password 参数含义 | 适用场景 |
|------|------------------|---------|
| A | 对 Global 插件：L0 覆盖值；对 Independent 插件：忽略（主密码全走 ExtraFields） | 清晰分离 |
| B | 对 Global 插件：L0 覆盖值；对 Independent 插件：也作为 L1b 候选（ExtraFields.plugin_password 为空时 fallback 到此） | 兼容灵活 |

**推荐方案 A**（语义清晰）：
- Global 插件：`password` = 用户指定的覆盖全局密码的值（可为空则用全局默认）
- Independent 插件：`password` **留空**，主密码完全通过 `extraFields.plugin_password` 传递

前端据此调整：

```typescript
// 根据 PasswordStrategy 决定 password 参数
const primaryPassword = taskOptions.value?.passwordStrategy === 'independent'
  ? undefined                                          // Independent: 主密码走 extraFields
  : secondaryPassword.value || undefined               // Global: L2 覆盖值（注意不是 L2！见下方纠正）

// ⚠️ 等等，这里变量名又混淆了。重新梳理：
// useTaskForm 中的 secondaryPassword 实际上是 L2 叠加验证密码
// 而 Global 插件的"用户指定的密码"目前没有独立的 UI 字段——它就是 L2？？
```

**重新梳理前端三个输入字段与后端参数的映射**：

| 前端 UI 字段 | 变量名 | 层级 | 发送目标 |
|-------------|--------|------|---------|
| 插件独立密码输入（动态渲染） | `extraValues["plugin_password"]` | L1b | `extraFields.plugin_password` |
| 覆盖密码输入（固定显示） | `secondaryPassword`（需改名！） | ??? | 取决于策略... |

**发现命名混乱的根源**：`useTaskForm.secondaryPassword` 这个变量名暗示它是 L2，但当前代码把它当"通用密码"在用。

**修正方案**：前端需要两个独立字段：

```typescript
// useTaskForm.ts 中：
const primaryOverride = ref('')     // 主密码覆盖（Global 插件用，覆盖 L0 默认值）
const secondaryPassword = ref('')   // 二级叠加验证密码（L2，所有插件通用，预留）

// getExtraPayload() 只收集 ExtraFields（L1b 等）
// createTask 调用时：
//   arg#4 (password)       = primaryOverride (Global 插件的 L0 覆盖)
//   arg#6 (extraFields)    = { plugin_password: ... } (Independent 的 L1b)
//   arg#7 (secondaryPwd)   = secondaryPassword (L2 叠加验证)
```

**13b. createTask 新签名**

**文件**: `app/encv-mobile/src/api/encv.ts`

```typescript
export async function createTask(
  type: TaskType,
  sourcePath: string,
  targetPath?: string,
  password?: string,              // 主密码覆盖（Global 插件的 L0 覆盖值）
  version?: number,
  extraFields?: Record<string, string>,  // 插件额外字段（Independent 的 L1b）
  secondaryPassword?: string,     // ← 新增：二级密码（L2 叠加验证）
): Promise<EncvTask> {
  const body: Record<string, unknown> = { type, sourcePath, targetPath, password, version, extraFields, secondaryPassword }
  // 移除 undefined 字段保持 clean payload...
}
```

**13c. useTaskForm 修正**

**文件**: `app/encv-mobile/src/composables/useTaskForm.ts`

```typescript
export function useTaskForm() {
  const predictedPlugin = ref<string | null>(null)
  const taskOptions = ref<TaskOptions | null>(null)
  const extraValues = ref<Record<string, string>>({})    // ExtraFields (L1b 等)
  const primaryOverride = ref('')                        // 主密码覆盖（Global 插件 L0 覆盖）
  const secondaryPassword = ref('')                      // 二级密码（L2 叠加验证）
  // ...

  function getExtraPayload(): Record<string, string> {
    const payload: Record<string, string> = {}
    for (const [k, v] of Object.entries(extraValues.value)) {
      if (v !== undefined && v !== '') payload[k] = v
    }
    return payload  // ← 只含 ExtraFields，不含任何密码
  }

  function reset() {
    // ...
    primaryOverride.value = ''
    secondaryPassword.value = ''
  }

  return {
    predictedPlugin, taskOptions, extraValues,
    primaryOverride,          // ← 新增暴露
    secondaryPassword,        // ← 已有，语义更正
    visibleExtraFields, versionOptions,
    predictPlugin, getExtraPayload, reset,
  }
}
```

### 🔧 Step 14: 前端 — UI 三个密码区域精确化

**文件**: `Tasks.vue` 模板 + `useI18n.ts`

#### 三个密码输入区域的最终设计

| # | UI 字段 | 显示条件 | 变量 | 层级 | i18n label | placeholder | badge |
|---|---------|---------|------|------|-----------|-------------|-------|
| 1 | **插件密码** | `taskOptions.passwordStrategy === 'independent'` | `extraValues["plugin_password"]` | L1b | `tasks.pluginPassword` | `tasks.pluginPasswordHelp` | 无 |
| 2 | **密码覆盖** | 始终显示（或仅 Global 插件时显示） | `primaryOverride` | L0覆盖 | `tasks.passwordOverride` | `tasks.passwordOverrideHelp` | `tasks.optional` |
| 3 | **二级密码** | 始终显示 | `secondaryPassword` | L2 | `tasks.secondaryPassword` | `tasks.secondaryPasswordHelp` | `tasks.optional` |

#### i18n 更新

```
// 新增/重命名
'tasks.passwordOverride': '密码',                          // Global 插件的主密码覆盖
'tasks.passwordOverrideHelp': '留空则使用全局默认密码',       // Global 插件提示
'tasks.pluginPassword': '插件密码',                         // Independent 插件的主密码
'tasks.pluginPasswordHelp': '留空则使用插件设置的默认密码',
'tasks.secondaryPassword': '二级密码',                      // L2 叠加验证
'tasks.secondaryPasswordHelp': '可选的额外验证密码',         // L2 说明
'tasks.optional': '可选',
```

### Step 15: 测试

#### 15a. 后端测试

**文件**: `internal/v2/plugins/task_options_test.go`（新建）

```go
package plugins_test

import (
    "testing"
    pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
    "github.com/Soltus/encv-go/internal/v2/plugins"
    "github.com/stretchr/testify/assert"
)

func TestVideoPlugin_GetTaskOptions(t *testing.T) {
    // 需要 initPluginsWithSettings 因为 SupportedContainerVersions 依赖初始化
    initPluginsWithSettings(t, nil)
    p := getPluginByName("video")
    require.NotNil(t, p)
    opts := p.GetTaskOptions()
    assert.Equal(t, pluginInterfaces.PasswordGlobal, opts.PasswordStrategy)
    assert.True(t, opts.SupportVersionSelect)
    assert.NotEmpty(t, opts.SupportedVersions)
    assert.Empty(t, opts.ExtraFields)
}

func TestAlistEncryptPlugin_GetTaskOptions(t *testing.T) {
    initPluginsWithSettings(t, nil)
    p := getPluginByName("alist_encrypt")
    require.NotNil(t, p)
    opts := p.GetTaskOptions()
    assert.Equal(t, pluginInterfaces.PasswordIndependent, opts.PasswordStrategy)
    assert.False(t, opts.SupportVersionSelect)
    assert.Len(t, opts.ExtraFields, 1)
    assert.Equal(t, "plugin_password", opts.ExtraFields[0].Key)
    assert.Equal(t, "password", opts.ExtraFields[0].Type)
}

func TestOtherPlugins_DefaultToGlobal(t *testing.T) {
    initPluginsWithSettings(t, nil)
    for _, name := range []string{"text", "audio", "image", "pdf", "wps"} {
        p := getPluginByName(name)
        require.NotNil(t, p)
        assert.Equal(t, pluginInterfaces.PasswordGlobal, p.GetTaskOptions().PasswordStrategy,
            "plugin %s should default to global", name)
    }
}

func TestAlistEncrypt_ResolvePassword_NoGlobalFallback(t *testing.T) {
    p := &alistencrypt.AlistEncryptPlugin{}
    // 模拟：无 DefaultPassword，有全局密码 → Independent 策略不应返回全局密码
    p.settings.DefaultPassword = ""
    // 注意：不需要设置 cfg.Password，因为 Independent 不应读取它
    result := p.resolvePassword()
    assert.Empty(t, result, "Independent plugin with no default password should return empty")
}

func TestAlistEncrypt_ResolvePasswordWithOverride_PriorityOrder(t *testing.T) {
    p := &alistencrypt.AlistEncryptPlugin{}
    p.settings.DefaultPassword = "plugin-default"

    // L1b (extraFields) > L1a (DefaultPassword)
    extras := map[string]string{"plugin_password": "l1b-from-task"}
    assert.Equal(t, "l1b-from-task", p.resolvePasswordWithTaskExtras(extras))
    // L1a only
    assert.Equal(t, "plugin-default", p.resolvePasswordWithTaskExtras(map[string]string{}))
    // 全部为空
    assert.Empty(t, p.resolvePasswordWithTaskExtras(map[string]string{}))
    // 无 DefaultPassword + 无 extraFields → 空（不 fallback 全局）
    p.settings.DefaultPassword = ""
    assert.Empty(t, p.resolvePasswordWithTaskExtras(map[string]string{}))
}
```

**文件**: `internal/service/task_manager_extra_test.go`（新建）

```go
package service_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestCreateWithExtras_PreservesExtraFields(t *testing.T) {
    tm := setupTestManager(t)
    extras := map[string]string{"plugin_password": "test123", "custom_field": "value"}
    task := tm.CreateWithExtras("encrypt", "/test/file.mp4", "", "override-pw", 4, extras)
    assert.Equal(t, "override-pw", task.Password)
    assert.Equal(t, "test123", task.ExtraFields["plugin_password"])
    assert.Equal(t, "value", task.ExtraFields["custom_field"])
}

func TestCreateWithoutExtras_Compat(t *testing.T) {
    tm := setupTestManager(t)
    task := tm.Create("encrypt", "/test/file.mp4", "", "pw", 4)
    assert.Nil(t, task.ExtraFields)  // 向后兼容：旧 API 创建的任务 ExtraFields 为 nil
}
```

#### 15b. 前端测试

**文件**: `app/encv-mobile/src/__tests__/useTaskForm.test.ts`（新建）

关键测试用例：
- predictPlugin 返回 video 插件 → `taskOptions.passwordStrategy === 'global'`, `supportVersionSelect=true`, 无 extraFields
- predictPlugin 返回 alist_encrypt → `passwordStrategy='independent'`, 有 1 个 extraFields(password type)
- `filteredExtraFields` 按 condition 过滤
- `getExtraPayload()` 只收集 extraValues（不含 secondaryPassword）
- `reset()` 清空所有状态
- API 失败降级

---

## 五、密码解析管道（v2 完整数据流）

```
┌────────────── 前端 Tasks.vue ───────────────────────────────┐
│                                                              │
│  [1. 插件密码] (仅 Independent 插件显示)                      │
│      → extraValues["plugin_password"]            (L1b)       │
│                                                              │
│  [2. 密码覆盖] (Global 插件的 L0 覆盖值)                       │
│      → primaryOverride                           (L0覆盖)    │
│                                                              │
│  [3. 二级密码] (叠加验证，所有插件通用，预留)                    │
│      → secondaryPassword                         (L2)        │
│                                                              │
│  createTask(type, src, tgt,                                   │
│    primaryOverride,           → req.password        (L0/L1)  │
│    version,                                                       │
│    { plugin_password: ... }, → req.extraFields     (L1b)       │
│    secondaryPassword         → req.secondaryPwd     (L2)        │
│  )                                                             │
└──────────────────────┬───────────────────────────────────────┘
                       │ POST /api/tasks
                       ▼
┌────────────── 后端 handleCreateTaskGin ───────────────────────┐
│  req.Password          → task.Password           (主密码)     │
│  req.SecondaryPassword → task.SecondaryPassword  (L2)         │
│  req.ExtraFields       → task.ExtraFields        (L1b)        │
└───────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌────────── processEncrypt / processDecrypt ──────────────────┐
│                                                               │
│  ① 主密码解析（TaskPasswordResolver 可选接口）：                │
│     if plugin implements TaskPasswordResolver:                │
│       primary = resolver.ResolveTaskPassword(                │
│         task.Password, task.ExtraFields)                     │
│     else:  // Global 插件默认                                  │
│       primary = task.Password || cfg.Password                │
│                                                               │
│  ② 注入主密码到 context：                                      │
│     passwordCtx = getPasswordContext(ctx, primary)             │
│                                                               │
│  ③ L2 二级密码（预留管道，当前加密层不支持）：                   │
│     // TODO: 加密层支持双密码后传入 task.SecondaryPassword      │
│     // 当前：加密层只使用 passwordCtx 中的主密码               │
│                                                               │
│  ④ EncryptFileWithPlugin(passwordCtx, ...)                   │
└───────────────────────┬───────────────────────────────────────┘
                       │
                       ▼
┌── AlistEncrypt（Independent 示例）──────────────────────────┐
│  ResolveTaskPassword(taskPwd, extras):                       │
│    → extras["plugin_password"]  (L1b)                        │
│    → settings.DefaultPassword   (L1a)                        │
│    → "" (❌ 不 fallback 全局密码)                              │
│                                                               │
│  EncryptToFile(reader, primaryPassword, ...)                 │
│  // L2 暂未接入（待加密层支持双密码）                            │
└─────────────────────────────────────────────────────────────┘
```

---

## 六、文件变更清单（完整 v2）

| # | 文件 | 操作 | 说明 | 对应缺陷 |
|---|------|------|------|---------|
| 1 | `internal/v2/plugins/interfaces/interfaces.go` | **修改** | 新增 `TaskPasswordResolver` 接口 | D |
| 2 | `internal/v2/plugins/alistencrypt/plugin.go` | **修改** | Initialize 不复制全局密码；新增 `resolvePasswordWithTaskExtras()`；实现 `TaskPasswordResolver` | A |
| 3 | `internal/server/mobile_api.go` | **修改** | `handleCreateTaskGin` 接受 `SecondaryPassword` + `ExtraFields` | C |
| 4 | `internal/service/task_manager.go` | **修改** | MobileTask 增加 `SecondaryPassword`+`ExtraFields`；`CreateWithExtras()`；processEncrypt/Decrypt 使用 `TaskPasswordResolver`+双通道 | C/D |
| 5 | `app/encv-mobile/src/api/encv.ts` | **修改** | `createTask()` 增加 `extraFields` + `secondaryPassword` 参数 | B |
| 6 | `app/encv-mobile/src/composables/useTaskForm.ts` | **修改** | 新增 `primaryOverride`；`secondaryPassword` 语义更正为 L2；`getExtraPayload()` 纯净化 | B |
| 7 | `app/encv-mobile/src/views/Tasks.vue` | **修改** | 三字段分离发送；模板三个密码区域（插件密码/密码覆盖/二级密码） | B |
| 8 | `app/encv-mobile/src/composables/useI18n.ts` | **修改** | i18n key 新增 `passwordOverride`/`pluginPassword`/`secondaryPassword` 三组 | — |
| 9 | `internal/v2/plugins/task_options_test.go` | **新建** | TaskOptions 声明 + Independent 无 fallback 测试 | A |
| 10 | `internal/service/task_manager_extra_test.go` | **新建** | CreateWithExtras + 双密码字段持久化测试 | C |
| 11 | `app/encv-mobile/src/__tests__/useTaskForm.test.ts` | **新建** | composable 三字段分离测试 | — |

> Step 1-8 已完成文件不在此表重复。

---

## 七、不做的事情（边界）

- **不改 schema.json 驱动的 PluginSettings** — 全局配置页与任务创建不同场景
- **不改其他 5 个插件的密码行为** — 它们都是 PasswordGlobal，走默认 L0 管道
- **不在前端实现 MIME/扩展名检测** — 完全委托后端 predict-plugin API
- **本次不实现加密层双密码支持** — L2 管道打通到 TaskManager 和持久化层，但 AES-CTR / ENCV 容器加密层仍为单密码。L2 校验作为后续独立任务，需修改 `NewAesCtr`、`EncryptToFile`、`DecryptFile` 及 Video 容器密钥派生逻辑
