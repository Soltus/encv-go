# 计划：修复多候选选择机制编译错误 + 补充测试

> **状态**: 代码 Step 1-8 已写入，中断于两个编译错误。本计划覆盖修复 + 测试。

---

## 问题诊断

### 错误 1: Go 编译失败 — `PluginCandidate` 类型缺失

**根因链**：
1. `interfaces.go` 原先定义了 `PluginCandidate`（引用 `Plugin` 接口）
2. `registry.go` 定义了 `Plugin` 接口 → import interfaces.go → 循环依赖
3. 上次操作从 `interfaces.go` 删除了 `PluginCandidate`
4. 但忘记在 `registry.go` 中补回定义 → **Go 编译报错 `undefined: PluginCandidate`**

**影响文件**：
- `registry.go` L415, 427-429, 448-450, 473-475: 使用 `pluginInterfaces.PluginCandidate`
- `mobile_api.go` L810, 820, 833: 使用 `plugins.PluginCandidate`

### 错误 2: 前端 TS 编译失败 — 5 个错误

**根因**：`useTaskForm.ts` 将 `predictedPlugin` 和 `taskOptions` 从 `ref` 改为 `computed`，但 `Tasks.vue` 仍尝试直接赋值：

| 行号 | 错误代码 | 原因 |
|------|---------|------|
| L191, L223 | TS2322: `string \| null` 不可赋值给 `string` | 模板中 `predictedPlugin` 可能是 null |
| L582 | TS2540: Cannot assign to 'value' (read-only) | `predictedPlugin` 是 ComputedRef |
| L583 | TS2540: 同上 | `taskOptions` 是 ComputedRef |
| L587, 594 | TS2540: 同上 | 同上 |

**Tasks.vue 中直接赋值 computed 的位置**（`validateSourcePath` 函数内）：
```typescript
// L582-594: 错误路径下试图清空显示
predictedPlugin.value = null    // ❌ computed 只读
taskOptions.value = null        // ❌ computed 只读
```

---

## 实施步骤

### Step 1: 在 registry.go 添加 PluginCandidate 类型定义

**文件**: `/workspace/internal/v2/plugins/registry.go`

在 `Plugin` 接口定义结束后（第 118 行 `}` 之后），添加：

```go
// PluginCandidate 表示一个能处理给定文件的插件候选
// 用于多候选选择场景（与 FindEncryptingPlugin 返回单一结果不同）
type PluginCandidate struct {
	Plugin    Plugin `json:"-"`       // 插件实例（不序列化到 JSON）
	Name      string `json:"name"`    // 插件名称标识符
	MatchType string `json:"matchType"` // 匹配类型: "mime" | "extension" | "general" | "container"
	Priority  int    `json:"priority"` // 优先级: 0=精确匹配(P0), 1=通用(P1)
}
```

同时修改 `FindAllEncryptingPlugins` 返回类型和内部使用：
- 返回值改为 `[]PluginCandidate`（去掉 `pluginInterfaces.` 前缀）
- 内部构造时去掉 `pluginInterfaces.` 前缀

**修改 `mobile_api.go`**：
- L810: `[]plugins.PluginCandidate` → 保持不变（类型现在在 registry.go 的 plugins 包中）
- L820, 833: `plugins.PluginCandidate` → 保持不变

### Step 2: 修复前端 TS 错误 — Tasks.vue 不再直接赋值 computed

**核心原则**：`predictedPlugin` 和 `taskOptions` 是从 `candidates[selectedPluginIndex]` 派生的 computed，清空显示应通过清空 `candidates` 触发自动重算。

**文件**: `/workspace/app/encv-mobile/src/views/Tasks.vue`

**修改 `validateSourcePath()` 函数**（L575-L601）：

将所有 `predictedPlugin.value = null` / `taskOptions.value = null` 替换为重置 candidates：

```typescript
// 之前（错误）:
predictedPlugin.value = null
taskOptions.value = null

// 之后（正确）:
// 通过 useTaskForm.reset() 或直接清空 candidates 触发 computed 自动返回 null
```

具体替换 4 处：
1. **L582-583** (`!path` 分支): 删除两行赋值，改用 `resetTaskForm()` 已有逻辑或让 `candidates` 为空时 computed 自然返回 null
   - 实际上 `candidates` 初始就是空数组，computed 已经会返回 null。只需删除这两行。
2. **L586-587** (`!path.startsWith('/')` 分支): 同上，删除两行赋值
3. **L594-595** (`!exists` 分支): 同上，删除两行赋值

**模板中的 null 安全处理**（L188, L191, L221-223）：
- L188: `v-if="candidates.length === 1 && predictedPlugin"` — `predictedPlugin` 作为 boolean 检查已经自动处理 null（null 是 falsy）✅ 无需改
- L191: `{{ t('tasks.willBeHandledBy', { plugin: predictedPlugin }) }}` — 需要确保 predictedPlugin 为 null 时不渲染。外层 `v-if` 已保护 ✅
- L223: 同 L191 ✅

**结论**：模板已有 `v-if` 保护，只需删除脚本中对 computed 的直接赋值即可。

### Step 3: 验证 Go 编译

```bash
cd /workspace && go build ./...
```

### Step 4: 验证前端编译

```bash
cd /workspace/app/encv-mobile && npm run build
```

### Step 5: 创建 plugin_candidates_test.go

**文件**: `/workspace/internal/v2/plugins/plugin_candidates_test.go`（新建）

测试用例覆盖：

| 测试名 | 场景 | 预期结果 |
|--------|------|---------|
| TestVideoFile_MimeMatch_P0 | video/mp4 文件 → video 插件 MIME 精确匹配 | 1 候选, matchType="mime", priority=0 |
| TestTextFile_ExtensionMatch_P0 | .txt 文件 → text 插件扩展名匹配 | 1 候选, matchType="extension", priority=0 |
| TestArbitraryFile_GeneralP1 | 无扩展名未知文件 → alist_encrypt ShouldProcess=true | ≥1 候选含 alist_encrypt, matchType="general", priority=1 |
| TestVideoFile_IncludesGeneral | video/mp4 → P0 精确匹配 + alist_encrypt 通用 | video P0 + alist_encrypt P1, 不重复 |
| TestEmptyPath_NoCandidates | 空字符串路径 | 空列表 |
| TestNonExistentFile_NoCrash | 不存在的文件路径 | 不崩溃（MIME 检测失败时优雅降级）|

注意：需要 `initPluginsForTaskOptions(t)` 或类似初始化来确保插件可用。

### Step 6: 运行全部测试

```bash
cd /workspace && go test ./internal/v2/plugins/... -count=1 -v
cd /workspace && go test ./internal/service/... -count=1 -v
```

---

## 验收标准

- [ ] `go build ./...` 零错误
- [ ] `npm run build` 零错误
- [ ] `go test ./internal/v2/plugins/...` 全通过（含新增测试）
- [ ] `go test ./internal/service/...` 全通过（回归验证）

---

## 风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| PluginCandidate JSON 序列化 | 低 | API 响应格式变化 | `json:"-"` 标记已排除 Plugin 字段，mobile_api 手动构建 gin.H |
| 前端 computed 删除后行为变化 | 低 | 显示逻辑异常 | computed 从 candidates 派生，清空 candidates 自动触发 null 返回 |
| 测试初始化顺序 | 中 | 插件未 Initialize 导致 ShouldProcess 异常 | 复用 task_options_test.go 的 initPluginsForTaskOptions 模式 |
