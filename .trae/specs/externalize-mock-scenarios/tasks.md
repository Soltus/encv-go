# Tasks

按依赖顺序排列；每项都对应具体文件 / 函数 / 测试。

## Task Dependencies

- T1 → T2（schema 先于 loader）
- T2 → T3（MockEngine 集成需 loader）
- T1..T3 → T4（迁移工作）
- T4 → T5（CLI 集成）
- T5 → T6（配置 schema 增量）
- T1..T6 → T7（端到端验证）
- T1..T7 → T8（文档）

---

## T1. 剧本 YAML schema 定义

**目标**: Go struct + YAML tag 双向映射，约束字段形状

- [ ] **T1.1** 新建 `internal/server/mock_scenario_schema.go`
  - `LoadedScenario` 结构（id / description / keywords / steps）
  - `YAMLStep`（id / events / when_tool_error）
  - `YAMLEvent`（type / data map[string]any）
  - `YAMLBranchOption`（id / label / keywords / icon）
  - 所有字段带 `yaml:"..."` tag，snake_case
  - 校验函数 `Validate() error`
- [ ] **T1.2** 校验规则
  - 缺 `id` → 拒绝
  - `id` 重复 → log error 跳过
  - `steps` 为空 → 拒绝
  - `events` 为空 → 拒绝
  - `mock_branch_choice.options` < 2 → 拒绝
  - `text_delta.text` 含 `{{` → 拒绝（**严禁模板**）
- [ ] **T1.3** 单元测试
  - `TestSchema_ParseYAML_BasicFields`
  - `TestSchema_ParseYAML_AllEventTypes` — 覆盖 5 种 event type
  - `TestSchema_Validate_RejectsMissingID`
  - `TestSchema_Validate_RejectsEmptySteps`
  - `TestSchema_Validate_RejectsTemplateSyntax` — text_delta.text 含 `{{` 拒绝
  - `TestSchema_Validate_RejectsBranchWithLessThan2Options`

✅ **验收**: `go test ./internal/server/... -run TestSchema -v` 全过

---

## T2. 剧本加载器

**目标**: 扫描目录 + 解析 YAML/JSON + 校验 + 注册

- [ ] **T2.1** 新建 `internal/server/mock_scenario_loader.go`
  - `ScenarioLoader` 结构（dir / logger / mu / scenarios map）
  - `NewScenarioLoader(dir string) *ScenarioLoader`
  - `LoadAll(ctx) error` — 扫描 `*.yaml` + `*.json`，逐个解析
  - 错误聚合（不中断，单文件失败 log error 继续）
  - 重复 id → 第一个赢，第二个 log error
  - `scenariosFromGoFallback()` — 注入 Go 字面量剧本（向后兼容）
- [ ] **T2.2** 热重载 watcher（`Watch(ctx)`）
  - 用 `github.com/fsnotify/fsnotify`
  - 监听 `*.yaml` / `*.json` 变更
  - 触发 reload（新文件 / 修改文件）
  - 失败 log error 但不中断 watcher
  - 活跃 stream 不受影响（旧剧本继续）
- [ ] **T2.3** 单元测试
  - `TestLoader_LoadYAML_BasicFields`
  - `TestLoader_LoadYAML_AllEventTypes`
  - `TestLoader_LoadYAML_MultipleFiles`
  - `TestLoader_LoadJSON_EquivalentToYAML`
  - `TestLoader_RejectMissingID`
  - `TestLoader_RejectDuplicateID`
  - `TestLoader_RejectEmptySteps`
  - `TestLoader_RejectEmptyEvents`
  - `TestLoader_DirEmpty_UsesGoFallback`
  - `TestLoader_DirNotFound_UsesGoFallback`
  - `TestLoader_HotReload_FileChange` — fsnotify 触发 reload
  - `TestLoader_Priority_YAMLOverridesGo`

✅ **验收**: `go test ./internal/server/... -run TestLoader -v` 全过

---

## T3. MockEngine 集成 + 预设分支推进 + 真实工具调用

**目标**: MockEngine 接收已加载剧本；YAML 不出现 tool_result，**全部由真实工具执行自动产生**；branch-pick API 走预设选项

- [ ] **T3.1** 修改 `internal/server/agent_mock.go`
  - `MockEngine.scenarios` 改为 `map[string]*MockScenario`
  - 删除 `var mockScenarios = []*MockScenario{...}` 直接引用
  - `NewMockEngine(scenarios []*MockScenario)` 构造 map
  - 推流仍按 `Steps` 顺序
- [ ] **T3.2** 新增 `internal/server/agent_mock_executor.go`（**核心执行模型**）
  - 处理 step 的 events 列表时
  - 遇到 `tool_call` event：调 `ToolRegistry.Execute(name, args)`
  - **自动**生成 `tool_result` event（id / name / isError / result 全部来自真实执行）
  - 把 `tool_result` 推入流（**不**依赖 YAML 里的声明）
  - YAML 里的 `events` 列表**不**包含 `tool_result`（loader 校验时拒绝）
- [ ] **T3.3** 修改 `internal/server/agent_mock_v2.go`（或新增 branch handler）
  - 新增 `POST /api/agent/branch-pick` 端点
  - 入参：`{scenario_id, branch_id, option_id}`
  - **拒绝**任何 free-form text 字段（`user_text` / `option_text`）
  - 验证 `option_id` ∈ `mock_branch_choice.options` 列表
  - 跳到对应 step（按 `option_id` 匹配同名 step）
- [ ] **T3.4** 单元测试
  - `TestMockEngine_UsesLoadedScenarios`
  - `TestMockEngine_RejectsToolResultInYAML` — YAML 里有 tool_result event → 加载失败
  - `TestMockEngine_BranchPick_AdvancesToCorrectStep`
  - `TestMockEngine_BranchPick_RejectsUnknownOption` — 选不在 options 列表里的 option → 404
  - `TestMockEngine_BranchPick_RejectsFreeFormText` — POST 带 user_text 字段 → 400
  - `TestMockEngine_ToolCall_AutoGeneratesToolResult` — 调真实工具，result 来自工具
  - `TestMockEngine_ToolCall_FailureProducesIsErrorTrue` — 工具失败时 tool_result.isError=true
  - `TestMockEngine_NoHardcodedData_AllBuiltinScenarios` — 扫所有 YAML，匹配硬编码模式即失败

✅ **验收**: 现有 `TestMockEngine*` 测试 0 修改仍通过 + 新增 8 个测试通过

---

## T4. 内置剧本迁移（v1 + v2 → YAML，**删 tool_result**）

**目标**: 20 个剧本全部迁移到 YAML；Go 字面量里的 `tool_result` 事件**全部删除**（由真实工具执行产生）

- [ ] **T4.1** 新建 `internal/server/mock_scenarios/builtin/` 目录
  - 12 个 v1 剧本迁移到 YAML
  - 文件名：`01_default_friendly.yaml` ... `12_*.yaml`
  - **删除** Go 字面量里所有 `tool_result` 事件
  - **删除** Go 字面量里所有"模拟"数据（具体路径、文件名、错误信息、计数）
  - 保留：`stream_start` / `stream_end` / `text_delta`（改通用文案）/ `tool_call` / `mock_branch_choice`
- [ ] **T4.2** 新建 `internal/server/mock_scenarios/v2/` 目录
  - 8 个 v2 剧本迁移到 YAML
  - **删除** Go 字面量里所有 `tool_result` 事件（含 `isError: true` 模拟错误）
  - 真实工具执行失败时 engine 自动产生 `isError: true` 的 tool_result
  - v2 多轮 SetContext 全部移除（走 `mock_branch_choice` 选项 + option_id 推进）
- [ ] **T4.3** Go 字面量剧本降级
  - `agent_mock_scenarios.go` 加 deprecation 注释
  - `agent_mock_v2_scenarios.go` 加 deprecation 注释
  - 保留作为 fallback（YAML 目录为空时使用）
- [ ] **T4.4** 单元测试
  - `TestMigration_AllBuiltinScenarios_Loadable` — 12 个 YAML 全部解析通过
  - `TestMigration_AllV2Scenarios_Loadable` — 8 个 v2 YAML 全部解析通过
  - `TestMigration_BehaviorEquivalentToGoLiteral` — 同一 id 行为等价
  - `TestMigration_AllYAML_NoHardcodedData` — 扫所有 YAML，匹配硬编码模式即失败

✅ **验收**: 启动 log 显示 `Loaded 20 scenarios from YAML (overriding 0 Go-literal fallbacks)`

---

## T5. CLI flag + 服务集成

**目标**: `cmd/encv/main.go` 支持 `-mock-scenarios-dir`

- [ ] **T5.1** 修改 `cmd/encv/main.go`
  - 新增 `flag.String("mock-scenarios-dir", "", "YAML scenarios directory (empty = Go literal fallback)")`
  - 传给 `server.NewServer(opts)`
- [ ] **T5.2** 修改 `internal/server/server.go`
  - `ServerOptions` 加 `ScenariosDir string` 字段
  - `NewServer` 初始化 loader
  - 失败 → log.Fatal（启动失败）
- [ ] **T5.3** 单元测试
  - `TestMain_FlagParse` — 验证 flag 解析
  - `TestServer_NewServer_NoDir_UsesFallback`
  - `TestServer_NewServer_WithDir_LoadsYAML`

✅ **验收**: `go run ./cmd/encv -mock-scenarios-dir=./testdata/yaml` 启动成功

---

## T6. 配置 schema 增量

**目标**: config.json 暴露 1 个新字段

- [ ] **T6.1** 修改 `internal/config/config.go`
  - `AgentSettings` 加 `MockScenariosDir string`
- [ ] **T6.2** 修改 `internal/config/schema.json`
  - 加 1 个新字段 + 默认值 + description
- [ ] **T6.3** 修改 `app/encv-mobile/src/views/Settings.vue` 渲染
  - `mock_scenarios_dir` → 文本输入 + 「选择目录」按钮
- [ ] **T6.4** 单元测试
  - `TestConfig_DefaultMockScenariosDir`

✅ **验收**: 启动时加载 config.json 字段，Settings.vue 渲染正常

---

## T7. 端到端集成测试

**目标**: 真实 mount + 真实剧本 + 预设选项 + 热重载

- [ ] **T7.1** 准备 sandbox 目录（mp4/srt/log/json）
- [ ] **T7.2** E2E: YAML 剧本端到端
  - 启动服务，YAML 目录 = `./testdata/yaml_scenarios`
  - 提问"找视频" → 验证 step 序列
  - 验证 text_delta 是预设字符串（不是模板渲染）
- [ ] **T7.3** E2E: 预设选项分支推进
  - 启动 `search_recursive_mp4` 剧本
  - 推到 `mock_branch_choice` 步骤
  - POST `branch-pick` 选 "relax" → 跳到 step "relax"
  - 验证后续 step 正确推流
- [ ] **T7.4** E2E: free-form text 拒绝
  - POST `branch-pick` 带 `user_text: "blah"` 字段 → 400
  - POST `branch-pick` 带 `option_id: "unknown"` → 404
- [ ] **T7.5** E2E: 热重载
  - 启动服务，`-mock-scenarios-reload=true`
  - 写入新 YAML 文件
  - 下次请求用新剧本（旧 stream 不中断）

✅ **验收**: `go test ./internal/server/... -run TestE2E -v` 5+ 全过

---

## T8. 文档与示例

**目标**: 演示团队可独立添加剧本

- [ ] **T8.1** 新建 `internal/server/mock_scenarios/SCHEMA.md`
  - 完整字段说明
  - 5 种 event type 文档
  - 最佳实践（如何写一个剧本）
- [ ] **T8.2** 新建 `internal/server/mock_scenarios/EXAMPLE_basic.yaml`
  - 5 步最小剧本示例
  - 含 1 个 text_delta + 1 个 tool_call + 1 个 tool_result + 1 个 stream_end
- [ ] **T8.3** 新建 `internal/server/mock_scenarios/EXAMPLE_branch.yaml`
  - 含 `mock_branch_choice` + 2 个 step 选项
  - 演示预设选项如何工作
- [ ] **T8.4** 更新 `agent_mock_scenarios.go` 顶部注释
  - 加迁移指南
  - 指向 `mock_scenarios/SCHEMA.md`
  - 强调：**剧本不接 free-form text 输入**

✅ **验收**: 演示团队读 SCHEMA.md + 改 EXAMPLE 即能加新剧本，无需 Go 工程师
