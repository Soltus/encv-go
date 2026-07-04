# 修复长按菜单文本 + 任务创建插件选项 Bug

## Bug 1: 长按菜单显示 `alistEncrypt.encrypt` 而非"加密"

### 根因
`actions.ts` 使用 `t('alistEncrypt.encrypt')` 和 `t('alistEncrypt.decrypt')`，但：
1. `alistEncrypt.encrypt` 这个 i18n key 根本不存在于字典中
2. 更严重的架构问题：**加密/解密是通用操作，不应该绑定到 `alistEncrypt` 命名空间**

项目已有通用 key `files.encrypt`（"加密"）和 `files.decrypt`（"解密"），`alistEncrypt.encrypt`/`alistEncrypt.decrypt` 是冗余的、命名空间错误的重复定义。

### 修复
文件: `/workspace/app/encv-mobile/src/features/alist-encrypt/actions.ts`

将 `t('alistEncrypt.encrypt')` → `t('files.encrypt')`
将 `t('alistEncrypt.decrypt')` → `t('files.decrypt')`

同时清理 i18n 字典中对应的冗余 key（如果存在）：
- 删除 `'alistEncrypt.encrypt'`（不存在，无需删）
- 删除 `'alistEncrypt.decrypt'`（存在于 L72/L624）

---

## Bug 2: 选择 video 插件无配置项和容器版本选择 / 选择 alist_encrypt 错误使用 v4 容器

### 根因
后端 `handlePredictPluginGin`（`mobile_api.go:845-854`）中，`taskOptions` 直接传递了 Go struct（`pluginInterfaces.TaskOptions`），该 struct **没有 json tag**，导致 JSON 序列化输出 PascalCase（`SupportVersionSelect`、`SupportedVersions`），前端期望 camelCase（`supportVersionSelect`、`supportedVersions`），读取到 `undefined`。

对比 `handlePluginsGin`（L793-812），那里**手动构建了 gin.H 用 camelCase key**，是正确的。

**结果**：
- 前端读取 `taskOptions.supportVersionSelect` → `undefined` → 版本选择器不显示
- 前端读取 `taskOptions.supportedVersions` → `undefined` → `versionOptions` 为空
- 前端读取 `taskOptions.extraFields` → `undefined` → 额外配置项不显示
- `state.version` 硬编码为 `4`，syncState 不从 defaultVersion 同步 → alist_encrypt 任务错误发送 version=4

### 修复步骤

#### 2a. 后端：给 `TaskOptions` 和 `TaskField` 添加 json tag（根治）

文件: `/workspace/internal/v2/plugins/interfaces/interfaces.go`

```go
type TaskOptions struct {
    PasswordStrategy     PasswordStrategy `json:"passwordStrategy"`
    SupportVersionSelect bool             `json:"supportVersionSelect"`
    SupportedVersions    []int            `json:"supportedVersions,omitempty"`
    DefaultVersion       int              `json:"defaultVersion"`
    ExtraFields          []TaskField      `json:"extraFields,omitempty"`
}

type TaskField struct {
    Key          string   `json:"key"`
    Label        string   `json:"label"`
    Type         string   `json:"type"`
    Required     bool     `json:"required"`
    DefaultValue string   `json:"defaultValue"`
    Help         string   `json:"help"`
    Options      []string `json:"options,omitempty"`
    Condition    string   `json:"condition,omitempty"`
}
```

#### 2b. 后端：`handlePredictPluginGin` 中手动构建 taskOptions 的 gin.H

文件: `/workspace/internal/server/mobile_api.go`

添加 json tag 后直接传 struct 也能正确序列化，但为与 `handlePluginsGin` 保持一致且双重保险，两处都用 gin.H 手动构建 camelCase key。

修改 `handlePredictPluginGin` 中所有 `taskOptions: opts` 为：
```go
"taskOptions": gin.H{
    "passwordStrategy":     string(opts.PasswordStrategy),
    "supportVersionSelect": opts.SupportVersionSelect,
    "supportedVersions":    opts.SupportedVersions,
    "defaultVersion":       opts.DefaultVersion,
    "extraFields":          opts.ExtraFields,
},
```

涉及 3 处：加密候选列表、解密单候选、顶层 taskOptions。

#### 2c. 前端：`syncState()` 从 taskOptions.defaultVersion 更新 state.version

文件: `/workspace/app/encv-mobile/src/composables/useNewTaskModal.ts`

```typescript
function syncState() {
    // ... 现有逻辑 ...
    if (candidates.value.length > 0) {
        state.taskOptions = candidates.value[selectedPluginIndex.value]?.taskOptions ?? null
        const defaultVer = candidates.value[selectedPluginIndex.value]?.taskOptions?.defaultVersion
        if (defaultVer && defaultVer > 0) {
            state.version = defaultVer
        }
    }
}
```

#### 2d. 前端：`onSelectPlugin` 切换插件时同步版本

文件: `/workspace/app/encv-mobile/src/composables/useNewTaskModal.ts`

```typescript
onSelectPlugin: (idx: number) => {
    state.selectedPluginIndex = idx
    if (candidates.value.length > 0) {
        state.taskOptions = candidates.value[idx]?.taskOptions ?? null
        const defaultVer = candidates.value[idx]?.taskOptions?.defaultVersion
        if (defaultVer && defaultVer > 0) {
            state.version = defaultVer
        } else if (!candidates.value[idx]?.taskOptions?.supportVersionSelect) {
            state.version = 0
        }
    }
},
```

#### 2e. 前端：createTask 提交时条件化 version、传递 extraFields 和 secondaryPassword

文件: `/workspace/app/encv-mobile/src/composables/useNewTaskModal.ts`

```typescript
const shouldSendVersion = state.taskType === 'encrypt' && state.taskOptions?.supportVersionSelect
const extraPayload = Object.keys(state.extraValues).length > 0 ? state.extraValues : undefined
await createTask(
    state.taskType as TaskType,
    state.sourcePath,
    state.targetPath || undefined,
    undefined,
    shouldSendVersion ? state.version : undefined,
    pluginName,
    extraPayload,
    state.secondaryPassword || undefined,
)
```

---

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `app/encv-mobile/src/features/alist-encrypt/actions.ts` | `alistEncrypt.encrypt` → `files.encrypt`，`alistEncrypt.decrypt` → `files.decrypt` |
| `app/encv-mobile/src/composables/useI18n.ts` | 删除冗余的 `alistEncrypt.decrypt` key |
| `internal/v2/plugins/interfaces/interfaces.go` | TaskOptions + TaskField 添加 json tag |
| `internal/server/mobile_api.go` | handlePredictPluginGin 中 taskOptions 手动构建 camelCase gin.H |
| `app/encv-mobile/src/composables/useNewTaskModal.ts` | syncState 同步 defaultVersion；onSelectPlugin 同步版本；onSubmit 传递 extraFields/secondaryPassword/条件 version |
