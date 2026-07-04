# 剧本外置 Spec（数据驱动版，零模板引擎）

## Why

`agent-mock-mode` v1 + `agent-tools-scenarios-v2` v2 共 20 个剧本**全部以 Go 字面量**写在 `internal/server/agent_mock_scenarios.go` 和 `agent_mock_v2_scenarios.go`。演示团队加新剧本必须找 Go 工程师重编二进制，迭代速度被锁死。

**用户原话**："所谓的v2剧本还是硬编码！就是个笑话！"

**修法**：把剧本从 Go 源码搬到 YAML/JSON 文件，加剧本 = 改配置文件 + 重启（或热重载），不需要重编 Go。

**关键约束**（用户明确要求）：
- ❌ **剧本严禁走用户输入**（不接 free-form text）
- ❌ **不引入模板引擎**（不需要 `{{ .UserText }}` 这种东西）
- ❌ **不增加不必要的复杂度**（loader 一份、YAML 一份、够用就行）
- ✅ **剧本 = 预设场景序列**（像剧本杀 / 互动剧，所有对话、所有选项都是预设的）
- ✅ **分支 = 预设选项 chip**（用户只能点，不能输入）
- ✅ **数据真实化 = tool_result 走真实工具**，文本是固定字符串
- ✅ **完全向后兼容** — Go 字面量剧本作为 fallback（YAML 目录空时仍用旧剧本），零回归

---

## What Changes

### 新增

- `internal/server/mock_scenario_schema.go` — Go struct 与 YAML 字段双向映射
- `internal/server/mock_scenario_loader.go` — 扫描目录 + 解析 + 校验 + 注册
- `internal/server/mock_scenarios/builtin/*.yaml` — 12 个 v1 剧本迁移
- `internal/server/mock_scenarios/v2/*.yaml` — 8 个 v2 剧本迁移
- `internal/server/mock_scenarios/SCHEMA.md` — 完整字段说明
- `internal/server/mock_scenarios/EXAMPLE_basic.yaml` — 5 步最小示例

### 修改

- `internal/server/server.go` — `NewServer` 接收 `scenariosDir` 参数
- `cmd/encv/main.go` — 新增 `-mock-scenarios-dir` flag
- `internal/config/schema.json` — 新增 `mock_scenarios_dir` 字段
- `app/encv-mobile/src/views/Settings.vue` — 渲染新字段
- `agent_mock_scenarios.go` / `agent_mock_v2_scenarios.go` — 加 deprecation 注释

### 不修改

- `execute_real` 机制
- MockEngine 状态机本身
- ToolRegistry
- 真实 LLM 路径
- 前端事件 payload 形状

---

## ADDED Requirements

### Requirement: 剧本 YAML schema

`internal/server/mock_scenario_schema.go` SHALL 定义 Go struct。

**核心铁律（用户原话）**：
- ❌ **剧本里不写死任何路径、文件名、文件内容、文件大小数字、错误信息文本**
- ❌ **剧本里不出现 `tool_result` 事件**——它由 MockEngine 在工具执行后**自动生成**
- ❌ **剧本里不出现任何"模拟"的 result 字段**（哪怕是 `"{}"` 占位）
- ✅ 剧本只声明"调哪个工具 + 工具参数 + 调完后说什么"（UI 文案）
- ✅ 工具参数（args）是用户输入的过滤条件（如 `ext: ".mp4"`），**不是数据**
- ✅ 真实结果由 `ToolRegistry` 走 `execute_real` 路径产生
- ✅ text_delta 是**通用 UI 文案**（"正在搜索..." / "搜索完成"），不含具体文件名/计数

#### Scenario: 剧本结构（核心）

```yaml
id: search_recursive_mp4        # 必填，全局唯一
description: 搜索 100MB+ 的 mp4  # 可选
keywords:                       # 触发关键词（与原 Keywords 字段一致）
  - search_recursive_mp4
  - 找视频

steps:                           # 必填，步骤序列（从头执行到尾）
  - id: search_and_branch        # 步骤 ID
    events:                      # 该步骤推流的事件序列
      - type: stream_start
        data: { scenario: search_recursive_mp4 }
      - type: text_delta
        # ✅ 通用 UI 文案，不含具体文件名/计数
        data: { text: "正在搜索符合条件的视频..." }
      - type: tool_call
        # ✅ 调工具 + 工具参数（参数是过滤条件，不是数据）
        data:
          id: call_srm_1
          name: search_files
          args: { ext: ".mp4", min_size: 104857600 }
        # ❌ 不写 tool_result！由 MockEngine 调真实工具后自动生成
      - type: text_delta
        # ✅ 通用 UI 文案（结果在 tool_result event 里由前端渲染）
        data: { text: "搜索完成。请选择操作：" }
      - type: mock_branch_choice
        data:
          branch_id: post_search
          options:                # 预设选项（chip 列表），用户只能点
            - id: relax
              label: 放宽条件
              icon: 🎚️
            - id: change_ext
              label: 改其他格式
              icon: 📁
            - id: cancel
              label: 取消
              icon: ❌
      - type: stream_end
        data: { scenario: search_recursive_mp4 }

  - id: relax                    # 用户选了 "relax" 后跳到这
    events:
      - type: stream_start
        data: { scenario: search_recursive_mp4 }
      - type: text_delta
        data: { text: "好的，已放宽到不限大小，重新搜索中..." }
      - type: tool_call
        data:
          id: call_srm_2
          name: search_files
          args: { ext: ".mp4" }   # 不限大小
        # 又是 tool_call，engine 自动产生 tool_result
      - type: text_delta
        data: { text: "搜索完成。请选择操作：" }
      - type: mock_branch_choice
        data: { ... }
      - type: stream_end

  - id: change_ext
    events: [...]  # 同上模式
  - id: cancel
    events: [...]  # 同上模式
```

#### Scenario: 字段约束

- 缺 `id` → 拒绝加载
- `id` 重复 → 第一个赢，第二个 log error 跳过
- `steps` 为空 → 拒绝加载
- `events` 为空 → 拒绝加载
- `mock_branch_choice` 的 `options` 至少 2 个、不能为空
- **禁止** `tool_result` 事件出现在 events 中（由 engine 生成）
- **禁止** `text_delta.text` 含 `{{`（严禁模板）

#### Scenario: 关键事件类型

| 事件 | 出现位置 | 用途 |
|------|---------|------|
| `stream_start` / `stream_end` | YAML | 标记一轮流的开头/结尾 |
| `text_delta` | YAML | 推**通用 UI 文案**（"正在搜索..."） |
| `tool_call` | YAML | 声明要调的工具 + 参数（args 是过滤条件） |
| `tool_result` | **engine 自动生成** | 工具真实执行结果（YAML 里**禁止**写） |
| `mock_branch_choice` | YAML | 推分支选项（chip 列表） |

#### Scenario: tool_result 的产生流程（核心）

```
YAML 里有 tool_call（name=search_files, args={ext:".mp4"}）
   ↓
MockEngine 推 tool_call event 到前端
   ↓
MockEngine 调 ToolRegistry.Execute("search_files", args)
   ↓
工具扫描真实文件系统（不是 YAML 里的假数据）
   ↓
返回真实结果 [{path:"/real/path/电影A.mp4", size:524288000}, ...]
   ↓
MockEngine 推 tool_result event（result=真实结果）
   ↓
前端渲染 tool_result（显示真实文件名/大小）
```

**关键点**：
- 路径、文件名、文件大小、文件计数**全部由工具真实产生**
- YAML 不写任何具体文件名（`Movies/2024/big.mp4` ❌）
- YAML 不写任何具体计数（`"0 个匹配"` ❌ —— 因为这个数字是真实结果决定的）
- YAML 不写任何具体错误文本（`"ERROR: connection timeout after 30s"` ❌）
- 这些值**只能**由 `ToolRegistry` 走真实执行产生

#### Scenario: text_delta 文案规范

✅ **允许的文案**（通用、过程性、不含具体数据）：
- "正在搜索..."
- "搜索完成"
- "处理中..."
- "请选择操作"
- "好的，已放宽条件"

❌ **禁止的文案**（含具体数据 / 模拟结果）：
- "找到 0 个文件" ← 计数是真实结果决定的
- "Movies/2024/big.mp4 是 500MB" ← 文件名/大小是真实数据
- "ERROR: timeout after 30s" ← 错误信息是真实工具产生的
- "成功重命名为 My New Title" ← 新名字是工具返回的
- 任何含具体路径、文件名、文件大小数字、计数的字符串

#### Scenario: tool_call 参数 vs 数据

✅ **允许的 args**（用户输入的过滤条件、不是数据本身）：
- `args: { ext: ".mp4" }` — 文件扩展名过滤
- `args: { min_size: 104857600 }` — 大小阈值（100MB 数值）
- `args: { pattern: "S01E*" }` — 名字 pattern
- `args: { max_count: 5 }` — 限制数量

❌ **禁止的 args**（不应在剧本里指定具体文件）：
- `args: { path: "/Movies/2024/big.mp4" }` ← 不指定具体文件
- `args: { file: "MyVideo.mp4" }` ← 不指定具体文件
- `args: { files: [...] }` ← 不列举文件列表

**`path` 参数说明**：工具的 `path` 参数是 mount 根或子目录，**不是**具体文件：
- ✅ `args: { path: "/mnt/sandbox/Movies" }` ← 搜索 Movies 目录（通用）
- ❌ `args: { path: "/mnt/sandbox/Movies/2024/big.mp4" }` ← 指定具体文件
- 工具对每个 path 会扫描整个子树，不是单文件查询

---

### Requirement: 数据真实性校验（CI 强约束）

`internal/server/mock_scenario_validator.go` SHALL 在加载时 + CI 测试中双重校验。

#### Scenario: 加载时校验

加载 YAML 时正则匹配以下模式，匹配到 → 拒绝加载 + log error：

| 模式 | 含义 | 反例 |
|------|------|------|
| 路径匹配 `/\w+/\w+/\w+\.\w{2,4}` | 类似 `Movies/2024/big.mp4` | `text: "找到 Movies/2024/big.mp4"` |
| 路径匹配 `\.(mp4\|mkv\|json\|log\|txt\|bin)$` | 具体文件后缀 | `path: "/old/file.mp4"` |
| 数字 + 字节单位 `\d+\s*(MB\|KB\|GB\|bytes?)` | 模拟文件大小 | `text: "文件大小 524MB"` |
| `\d+ 个(文件\|匹配\|结果)` | 模拟计数 | `text: "找到 0 个文件"` |
| `"ERROR\|失败: .+"` | 模拟错误文本 | `text: "ERROR: connection timeout"` |
| `tool_result` 事件关键字 | 整事件禁止 | `type: tool_result` 出现在 events |
| 含 `{{` | 模板 | `text: "找到 {{ .count }} 个"` |

#### Scenario: CI 测试

`TestScenario_NoHardcodedData` 单元测试：扫描所有内置 YAML，匹配以上正则，**任何一个匹配 → 测试失败**。

- 这条测试是**红线**：YAML 里出现任何硬编码数据 → 整个项目 CI 挂
- 强制演示团队写真正调工具的剧本

#### Scenario: 静态分析脚本

`scripts/check-scenarios-no-hardcoded-data.sh` 单独跑：
```bash
# 扫所有 internal/server/mock_scenarios/**/*.yaml
# 匹配上述正则，发现即报错
# 在 pre-commit hook + CI 流水线跑
```

#### Scenario: 允许的"数字"

部分数字是**用户输入的过滤条件**（在 tool_call.args 里），不是数据：
- `min_size: 104857600` ← 100MB 阈值
- `max_count: 5` ← 限制 5 个
- `ext: ".mp4"` ← 扩展名

这些不算硬编码数据。校验脚本区分：
- `tool_call.args` 里的数字 ✅
- `text_delta.text` / `tool_result.data.result` ❌
- YAML 里其他地方的数字 ❌（除非是 schema 字段如 `rounds: 1`）

---

### Requirement: MockEngine 自动生成 tool_result（核心执行模型）

**用户原话**："必须完全调用工具获取！比如必须不依赖任何路径或文件名预期获取真正视频"

`MockEngine` SHALL **自动**在 `tool_call` 事件之后**调用真实工具**并**自动生成** `tool_result` 事件。

#### Scenario: 执行流程（核心）

```
1. MockEngine 读取 YAML 当前 step 的 events
2. 顺序处理：
   - stream_start → 推送
   - text_delta → 推送（文本是 YAML 写死的通用 UI 文案）
   - tool_call → 推送 + 调 ToolRegistry.Execute(name, args)
   - **engine 自动**生成 tool_result 事件（result = 真实工具结果）→ 推送
   - text_delta → 推送
   - mock_branch_choice → 推送
   - stream_end → 推送
3. 等用户点 chip / keyword 触发下一步
```

**关键点**：
- YAML 里的 `events` 列表**不允许**出现 `tool_result` 事件
- `tool_result` 由 engine 在执行 `tool_call` 时**自动产生**
- tool_result 的 `result` 字段是**工具真实返回**（不是 YAML 占位）
- tool_result 的 `id` / `name` / `args` / `isError` 都来自对应 `tool_call` 事件

#### Scenario: execute_real vs fallback 路径

- `tool_call.name` 必须在 `ToolRegistry` 注册
- 走真实执行（`ToolRegistry.Execute`）→ `tool_result.result` 是**真实工具结果**
- 若工具执行异常 → `tool_result.isError: true` + `result` 是错误信息
- 没有任何"模拟"的 result（哪怕是 `"{}"` 占位）

#### Scenario: 与 v1/v2 的对比

| | v1/v2 (现状) | 新设计 (本 spec) |
|---|------------|--------------|
| 路径/文件名/计数 | 写在 Go 字面量里（如 `Movies/2024/big.mp4`） | 走真实工具 |
| 错误信息 | 写死（`"ERROR: connection timeout after 30s"`） | 工具真实返回 |
| tool_result 事件 | 出现在 Go 数组里（mock data） | **YAML 禁止**出现，engine 生成 |
| text_delta | 含具体数字（`"找到 0 个"`） | 通用 UI 文案（`"搜索完成"`） |
| 多轮 | `SetContext` 假装用户选 | `mock_branch_choice` + 预设 chip |

---

### Requirement: 剧本加载器

`internal/server/mock_scenario_loader.go` SHALL 在启动时扫描目录。

#### Scenario: 启动加载

```
NewServer(opts):
  loader := NewScenarioLoader(opts.ScenariosDir)
  scenarios, err := loader.LoadAll()
  失败 → log.Fatal
  成功 → 注册到 MockEngine.scenarios map[id]*MockScenario
  同时注入 Go 字面量 fallback（mockScenarios + mockScenariosV2）
```

#### Scenario: 优先级

- 同 `id` 时：YAML > Go 字面量
- 启动 log：`Loaded 20 scenarios from YAML (overriding 0 Go-literal fallbacks)`

#### Scenario: 目录为空

- YAML 目录不存在 / 无 `*.yaml` / `*.json` → 全部用 Go 字面量
- 启动 log：`No YAML scenarios found, using 20 Go-literal fallbacks`

#### Scenario: 热重载（可选）

- `-mock-scenarios-reload=true` 启动 fsnotify watcher
- 检测到 `*.yaml` 变更 → 重新解析（不重启进程）
- 活跃 stream 走旧剧本（不中断）
- 新 stream 用新剧本

---

### Requirement: 剧本分支（预设选项，不是 free-form 输入）

**核心约束（用户明确要求）**：
- ❌ **剧本严禁走用户输入**（不接 free-form text，不接真实 user_text 推进）
- ✅ **分支 = 预设选项 chip**（用户在 YAML 写好的 options 列表里点一个）
- ✅ **step → 选 option → 跳到对应 step**（确定性，无意外）
- ✅ **文本永远是预设字符串**（不用 `{{ }}` 模板）

#### Scenario: 分支推进流程

```
剧本执行到 step "post_search"（含 mock_branch_choice）
   ↓
推 mock_branch_choice 事件，前端展示 3 个 chip
   ↓
用户点击 "放宽" chip
   ↓
前端 POST /api/agent/branch-pick {scenario: "search_recursive_mp4", branch_id: "post_search", option: "relax"}
   ↓
后端 MockEngine 收到 pick，根据 "relax" 跳到 step "relax"
   ↓
继续推 step "relax" 的 events
```

- **没有任何 free-form text 输入**
- 用户**只能**在 `mock_branch_choice.options` 列表里选
- YAML 里写死有哪些选项，每个选项对应哪个 step
- 这就是剧本游戏的本质：**剧本 = 预定义剧情树，用户在岔路口点预设选项**

#### Scenario: 关键词触发（同 v1/v2 现状）

- 剧本匹配通过 `keywords` 字段（前缀 / 完全匹配）
- YAML `keywords` 数组保留与 Go 字面量完全一致的行为

---

### Requirement: 单元测试

#### Scenario: 加载器测试

- [ ] `TestLoader_LoadYAML_BasicFields`
- [ ] `TestLoader_LoadYAML_AllEventTypes`
- [ ] `TestLoader_LoadYAML_MultipleFiles`
- [ ] `TestLoader_LoadJSON_EquivalentToYAML`
- [ ] `TestLoader_RejectMissingID`
- [ ] `TestLoader_RejectDuplicateID`
- [ ] `TestLoader_RejectEmptySteps`
- [ ] `TestLoader_RejectEmptyEvents`
- [ ] `TestLoader_DirEmpty_UsesGoFallback`
- [ ] `TestLoader_DirNotFound_UsesGoFallback`
- [ ] `TestLoader_HotReload_FileChange`
- [ ] `TestLoader_Priority_YAMLOverridesGo`

#### Scenario: 分支测试

- [ ] `TestScenario_Branch_OptionsArePrescript` — 选项必须 ≥2 个
- [ ] `TestScenario_Branch_PickAdvancesToCorrectStep`
- [ ] `TestScenario_Branch_FreeFormUserInputIsRejected` — 后端拒绝接 free-form text 推进剧本

#### Scenario: 端到端测试

- [ ] `TestE2E_YAMLScenario_RunEndToEnd` — 触发 YAML 剧本 → 验证 step 序列
- [ ] `TestE2E_BranchPick_AdvancesToCorrectStep`
- [ ] `TestE2E_HotReload_NewScenarioAdded`
- [ ] `TestE2E_FreeFormUserInput_NotConsumedByScenario` — 验证剧本不响应 free text

---

## MODIFIED Requirements

### Requirement: MockEngine 接收已加载剧本

**BEFORE**: `var mockScenarios = []*MockScenario{...}`（Go 字面量全局变量）
**AFTER**: `MockEngine.scenarios map[string]*MockScenario`（loader 注入）

#### Scenario: 向后兼容

- 当 `scenariosDir` 不存在或为空时，loader 把 `mockScenarios` + `mockScenariosV2` 注入到 `MockEngine.scenarios`
- 所有现有测试（无 ScenariosDir 字段）继续工作

#### Scenario: 优先级

- YAML 同 id 剧本 > Go 字面量剧本
- 加载 log：`Mock scenario 'X' overridden by YAML`

---

### Requirement: Go 字面量剧本降级为 fallback

**BEFORE**: `mockScenariosV2` 是主源，编译进二进制
**AFTER**: `mockScenariosV2` 是 fallback，YAML 目录为空时才用

#### Scenario: 不删除

- **保留** `agent_mock_scenarios.go` + `agent_mock_v2_scenarios.go` 作为 fallback
- 注释更新：`// 当 ScenariosDir 为空时使用此 fallback；推荐迁移到 YAML`
- 单元测试 / 快速启动 / 旧 demo 数据继续工作

---

## REMOVED Requirements

**用户原话**："剧本严禁走用户输入！增加不必要的复杂度"

- ❌ **删除** T3 模板插值引擎 — `internal/server/mock_scenario_template.go` 不再存在
- ❌ **删除** `{{ .UserText }}` / `{{ .ToolResult.matches[0] }}` 之类模板引用
- ❌ **删除** `RenderString` / `RenderEvent` 之类模板 API
- ❌ **删除** T5 v2 user_text 真实化（旧的 `edit_metadata_wizard` SetContext 也走 fallback）
- ❌ **删除** `tojsonFunc` 自定义 func
- ❌ **删除** T1.2 / T3.2 中所有模板相关测试

**保留**：
- ✅ YAML 外置（核心价值）
- ✅ loader 加载 + 校验（含硬编码数据正则）
- ✅ 预设选项 chip（mock_branch_choice）
- ✅ 关键词触发
- ✅ tool_result **自动**由真实工具产生（YAML 禁写）
- ✅ Go 字面量 fallback
- ✅ 热重载（可选）

---

## 约束与限制（再次加强）

1. **完全向后兼容** — Go 字面量剧本 + `execute_real` + MockPreset + ToolRegistry 必须继续工作
2. **加载失败不启动** — YAML 解析错误 → log.Fatal（不静默回退）
3. **剧本不接 free-form text** — 后端 API `branch-pick` 只接受 `option` ID，不接受 text
4. **文本永远是预设字符串** — text_delta.text 字段不允许模板语法（拒绝 `{{` 字符）
5. **YAML 不出现 tool_result 事件** — 全部由 engine 自动生成
6. **YAML 不硬编码任何数据** — 路径、文件名、文件大小、计数、错误信息文本
7. **CI 强校验** — `TestScenario_NoHardcodedData` 扫所有 YAML，匹配即测试失败
8. **YAML 字段命名** — snake_case
9. **演示团队可独立操作** — 加新剧本 = 写 YAML + 重启（或开热重载）

---

## 关键文件 / 函数

| 文件 | 作用 |
|------|------|
| `internal/server/mock_scenario_schema.go` | `LoadedScenario` / `YAMLStep` / `YAMLEvent` struct + Validate() |
| `internal/server/mock_scenario_loader.go` | `NewScenarioLoader(dir)` / `LoadAll(ctx)` / `Watch(ctx)` |
| `internal/server/agent_mock.go` | `MockEngine.scenarios` 改为 map（loader 注入） |
| `internal/server/agent_mock_scenarios.go` | 加 deprecation 注释（保留 fallback） |
| `internal/server/agent_mock_v2_scenarios.go` | 加 deprecation 注释（保留 fallback） |
| `internal/server/mock_scenarios/builtin/*.yaml` | 12 个 v1 剧本迁移 |
| `internal/server/mock_scenarios/v2/*.yaml` | 8 个 v2 剧本迁移 |
| `internal/server/mock_scenarios/SCHEMA.md` | 完整 schema 文档 |
| `internal/server/mock_scenarios/EXAMPLE_basic.yaml` | 5 步最小剧本示例 |
| `cmd/encv/main.go` | 新增 `-mock-scenarios-dir` flag |
| `internal/config/schema.json` | 新增 `mock_scenarios_dir` 字段 |

---

## 与现有 spec 的关系

| 现有 spec | 关系 |
|----------|------|
| `agent-mock-mode` | **修改点** — Go 字面量剧本降级为 fallback，YAML 为主源 |
| `agent-tools-scenarios-v2` | **修改点** — v2 剧本迁移到 YAML，**不动** SetContext / branch 机制（保留作为 YAML 数据） |
| `multi-engine-chat-architecture` | 无关 |
| `go-in-process-agent` | 无关 |
| `mock-router-refactor` | 无关（前端 mock 路由） |
