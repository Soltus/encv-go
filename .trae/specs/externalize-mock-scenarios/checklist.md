# Checklist

实施完成时的验收清单。每条对应 spec 中的具体 Requirement / Scenario。

---

## 用户原话验证（最高优先级）

> "所谓的v2剧本还是硬编码！就是个笑话！"
> "剧本严禁走用户输入！增加不必要的复杂度"
> "是预设选项不是直接走完！"
> "依然有路径和文件名硬编码！必须完全调用工具获取！"
> "比如必须不依赖任何路径或文件名预期获取真正视频"

- [ ] ❌ **没有任何剧本**写在 Go 字面量 `var mockScenarios* = []*MockScenario{...}` 中
- [ ] ✅ 所有剧本都在 `internal/server/mock_scenarios/*.yaml`
- [ ] ✅ Go 字面量**仅**作为 fallback 保留，且加 deprecation 注释
- [ ] ❌ **没有任何**模板引擎（`internal/server/mock_scenario_template.go` **不存在**）
- [ ] ❌ **没有任何** `{{ .UserText }}` / `{{ .ToolResult.X }}` 模板引用
- [ ] ❌ **没有任何**剧本接 free-form text 推进
- [ ] ✅ 分支 = `mock_branch_choice` + 预设 `options` 列表（用户只能点 chip）
- [ ] ✅ `branch-pick` API **拒绝**任何 free-form text 字段
- [ ] ❌ **没有任何** `tool_result` 事件出现在 YAML 中（loader 拒绝 + CI 测试兜底）
- [ ] ❌ **没有任何**路径写死在 YAML（`Movies/2024/big.mp4` 等模式被正则拒绝）
- [ ] ❌ **没有任何**文件名写死在 YAML（`.mp4` / `.mkv` / `.json` 等具体文件后缀被拒绝）
- [ ] ❌ **没有任何**文件大小数字写死在 text_delta（`524MB` / `100MB` 等被拒绝）
- [ ] ❌ **没有任何**模拟计数（`"0 个匹配"` / `"1 个文件"` 等被拒绝）
- [ ] ❌ **没有任何**模拟错误信息（`"ERROR: connection timeout after 30s"` 等被拒绝）
- [ ] ✅ tool_result 由 MockEngine **自动**调真实工具产生
- [ ] ✅ `TestScenario_NoHardcodedData_AllBuiltinScenarios` 测试通过（红线）
- [ ] ✅ 演示团队加新剧本 = 写 YAML + 重启，无需 Go 工程师

---

## T1. 剧本 YAML schema 定义

- [ ] `internal/server/mock_scenario_schema.go` 存在
- [ ] `LoadedScenario` / `YAMLStep` / `YAMLEvent` / `YAMLBranchOption` 结构体定义
- [ ] 所有字段带 `yaml:"..."` tag，snake_case
- [ ] `Validate() error` 实现
- [ ] 缺 `id` / 空 `steps` / 空 `events` → 拒绝
- [ ] `mock_branch_choice.options` < 2 → 拒绝
- [ ] `text_delta.text` 含 `{{` → 拒绝（**严禁模板**）
- [ ] **events 列表含 `type: tool_result` → 拒绝**（YAML 禁写 tool_result）
- [ ] 路径/文件名/大小数字/计数/错误信息正则匹配 → 拒绝
- [ ] `go test -run TestSchema -v` 全过（6+ 用例）

---

## T2. 剧本加载器

- [ ] `internal/server/mock_scenario_loader.go` 存在
- [ ] `NewScenarioLoader(dir)` / `LoadAll(ctx)` / `Watch(ctx)` 三个方法
- [ ] YAML + JSON 双格式支持
- [ ] 错误聚合（不中断，单文件失败 log 继续）
- [ ] 重复 id → 第一个赢，第二个 log error
- [ ] 目录为空 → 自动注入 Go 字面量 fallback
- [ ] 目录不存在 → 同上
- [ ] fsnotify 热重载（`-mock-scenarios-reload=true` 时启用）
- [ ] 优先级：YAML > Go 字面量
- [ ] 启动 log 列出加载源 + 覆盖关系
- [ ] `go test -run TestLoader -v` 全过（12+ 用例）

---

## T3. MockEngine 集成 + 预设分支推进 + 真实工具调用

- [ ] `MockEngine.scenarios` 改为 `map[string]*MockScenario`
- [ ] 移除对 `var mockScenarios` 的直接引用
- [ ] `NewMockEngine(scenarios []*MockScenario)` 构造 map
- [ ] **`internal/server/agent_mock_executor.go` 存在**
- [ ] executor 处理 `tool_call` event 时调 `ToolRegistry.Execute(name, args)`
- [ ] executor **自动**生成 `tool_result` event（result 来自真实工具，不是 YAML 声明）
- [ ] YAML 里**不**出现 `tool_result` 事件（loader 拒绝）
- [ ] `POST /api/agent/branch-pick` 端点实现
- [ ] 入参只有 `{scenario_id, branch_id, option_id}`，**无** `user_text` 字段
- [ ] 拒绝 free-form text（返回 400）
- [ ] 拒绝未知 option_id（返回 404）
- [ ] 跳到对应 step（按 `option_id` 匹配同名 step）
- [ ] `TestMockEngine_RejectsToolResultInYAML` 通过
- [ ] `TestMockEngine_ToolCall_AutoGeneratesToolResult` 通过
- [ ] `TestMockEngine_ToolCall_FailureProducesIsErrorTrue` 通过
- [ ] `TestMockEngine_NoHardcodedData_AllBuiltinScenarios` 通过（**红线测试**）
- [ ] 现有 `TestMockEngine*` 测试 0 修改仍通过

---

## T4. 内置剧本迁移

- [ ] `internal/server/mock_scenarios/builtin/` 目录存在
- [ ] 12 个 v1 剧本 YAML 文件存在
- [ ] `internal/server/mock_scenarios/v2/` 目录存在
- [ ] 8 个 v2 剧本 YAML 文件存在
- [ ] v2 YAML **没有**任何 `tool_result` 事件
- [ ] v1 YAML **没有**任何 `tool_result` 事件
- [ ] v1/v2 YAML **没有**任何硬编码路径/文件名/大小/计数/错误信息
- [ ] `agent_mock_scenarios.go` 顶部 deprecation 注释
- [ ] `agent_mock_v2_scenarios.go` 顶部 deprecation 注释
- [ ] Go 字面量保留作为 fallback
- [ ] `TestMigration_AllBuiltinScenarios_Loadable` 通过
- [ ] `TestMigration_AllV2Scenarios_Loadable` 通过
- [ ] `TestMigration_BehaviorEquivalentToGoLiteral` 通过
- [ ] `TestMigration_AllYAML_NoHardcodedData` 通过（**红线**）
- [ ] 启动 log：`Loaded 20 scenarios from YAML (overriding 0 Go-literal fallbacks)`

---

## T5. CLI flag + 服务集成

- [ ] `cmd/encv/main.go` 新增 `-mock-scenarios-dir` flag
- [ ] `ServerOptions.ScenariosDir` 字段
- [ ] `NewServer` 初始化 loader
- [ ] 加载失败 → log.Fatal
- [ ] `go run ./cmd/encv -mock-scenarios-dir=./testdata/yaml` 启动成功
- [ ] `TestMain_FlagParse` / `TestServer_NewServer_*` 通过

---

## T6. 配置 schema 增量

- [ ] `AgentSettings.MockScenariosDir` 字段
- [ ] `schema.json` 新增 1 字段
- [ ] `Settings.vue` 渲染目录输入 + 选择按钮
- [ ] `TestConfig_*` 通过

---

## T7. 端到端集成测试

- [ ] sandbox 目录准备
- [ ] E2E: YAML 剧本端到端（text_delta 是预设字符串）
- [ ] E2E: 预设选项分支推进（POST option_id → 跳 step）
- [ ] E2E: free-form text 被拒绝（带 user_text → 400）
- [ ] E2E: 未知 option_id 被拒绝（404）
- [ ] E2E: 热重载
- [ ] `go test -run TestE2E -v` 全过（5+ 用例）

---

## T8. 文档与示例

- [ ] `mock_scenarios/SCHEMA.md` 存在
- [ ] `mock_scenarios/EXAMPLE_basic.yaml` 存在（5 步最小）
- [ ] `mock_scenarios/EXAMPLE_branch.yaml` 存在（预设选项示例）
- [ ] `agent_mock_scenarios.go` 顶部注释指向 SCHEMA.md
- [ ] 注释强调：**剧本不接 free-form text 输入**

---

## 类型检查

- [ ] `go build ./cmd/encv` 0 错误
- [ ] `vue-tsc --noEmit` 0 错误
- [ ] `pnpm test --run` 0 失败（mobile 前端）

---

## 关键约束再确认

| 约束 | 状态 |
|------|------|
| 剧本严禁走用户输入 | ❌ 严禁 / ✅ 拒绝 free-form text |
| 不引入模板引擎 | ❌ `mock_scenario_template.go` 不存在 |
| 不增加不必要的复杂度 | ✅ 只加 loader + YAML + executor |
| 分支 = 预设选项 chip | ✅ `mock_branch_choice.options` |
| 文本永远是预设字符串 | ✅ text_delta 不允许 `{{` |
| **YAML 禁写 tool_result** | ❌ 任何 `type: tool_result` 都被 loader 拒绝 |
| **路径/文件名走真实工具** | ❌ 任何硬编码路径被正则拒绝 |
| **文件大小/计数/错误信息走真实工具** | ❌ 任何硬编码数字/错误文本被正则拒绝 |
| **CI 强校验** | ✅ `TestScenario_NoHardcodedData` 扫所有 YAML |
| 向后兼容 | ✅ Go 字面量 fallback |
| 加新剧本不需要 Go 工程师 | ✅ 改 YAML 即可 |
