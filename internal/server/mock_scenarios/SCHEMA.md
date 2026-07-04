# 剧本外置 Schema（剧本外置 spec）

> **目标读者**：演示团队 / 产品 / 任何想加新 mock 剧本的人。
> **不要求 Go 知识** — 改 YAML + 重启即可。

---

## 快速开始

1. **复制示例**：把 [EXAMPLE_basic.yaml](EXAMPLE_basic.yaml) 拷到 `internal/server/mock_scenarios/builtin/`
2. **改 id / 关键词 / 步骤** — 跟改 JSON 一样简单
3. **重启服务** — `encv start --mock-scenarios-dir=./internal/server/mock_scenarios/builtin`
4. **触发**：在 UI 输入 keywords 里的词 → 走你的新剧本

---

## 核心铁律（不可违反）

| 铁律 | 违反后果 |
|------|---------|
| ❌ **YAML 严禁出现 `tool_result` 事件** | loader 拒绝加载 |
| ❌ **YAML 严禁模板语法 `{{ ... }}` / `{% ... %}`** | loader 拒绝加载 |
| ❌ **YAML 严禁硬编码路径**（`Movies/2024/big.mp4`） | loader 拒绝加载 |
| ❌ **YAML 严禁硬编码文件大小**（`524MB`） | loader 拒绝加载 |
| ❌ **YAML 严禁硬编码计数**（`0 个匹配`） | loader 拒绝加载 |
| ❌ **YAML 严禁硬编码错误信息**（`ERROR: timeout`） | loader 拒绝加载 |
| ❌ **YAML 严禁 user_text / free-form text 字段** | loader 拒绝加载 |
| ✅ **工具参数（args）只能是"过滤条件"**（如 `ext: ".mp4"`） | 数据由工具真实产生 |
| ✅ **text_delta 必须是"通用 UI 文案"**（如"正在搜索..."） | 具体结果由 tool_result 渲染 |
| ✅ **分支必须是"预设选项 chip"**（用户在 options 列表点） | 不接 free-form text |

> **为什么这么严？** v1/v2 剧本的硬编码数据让 demo 名不副实 — 演示给客户看的"找到 Movies/2024/big.mp4"是编的。YAML 外置 + tool_result 自动生成真实结果 = 演示真实工具行为，客户不再被骗。

---

## 顶层 Schema

```yaml
id: my_scenario              # 必填，全局唯一
description: 一句话描述       # 可选
keywords:                    # 可选，触发关键词（任一命中即触发）
  - 找视频
  - search
exact_match: ""              # 可选，精确匹配字符串
regex: ""                    # 可选，正则匹配
presets:                     # 可选，初始 chip 列表
  - id: opt1
    label: "📁 选项1"
    user_text: "..."        # chip 点击后发送的预定义消息
    icon: "📁"
steps:                       # 必填，至少 1 个
  - id: step1
    events: [...]
    delay_ms: 0
```

### 字段说明

| 字段 | 类型 | 必填 | 用途 |
|------|------|------|------|
| `id` | string | ✅ | 全局唯一，触发匹配的 key |
| `description` | string | ❌ | 给运维/开发看的注释 |
| `exact_match` | string | ❌ | 精确匹配字符串（与 v1 一致） |
| `keywords` | string[] | ❌ | 关键词列表（任一命中即触发） |
| `regex` | string | ❌ | 正则匹配模式 |
| `presets` | YAMLPreset[] | ❌ | 初始 chip 列表（剧本激活时推给前端） |
| `branches` | YAMLBranch[] | ❌ | v2 兼容：分支选项（v2 多轮路径用） |
| `rounds` | int | ❌ | v2 兼容：总轮数 |
| `steps` | YAMLStep[] | ✅ | 步骤序列，至少 1 个 |

---

## Step Schema

```yaml
- id: step_unique_id        # 可选（但推荐）— 便于日志和 branch 跳转
  delay_ms: 200              # 可选，推流前等待毫秒数
  events:                    # 必填，至少 1 个 event
    - type: stream_start
      data: { scenario: my_scenario }
    - type: text_delta
      data: { text: "正在搜索..." }
    - type: tool_call
      data:
        id: call_1
        name: search_files
        args: { ext: ".mp4" }   # ← 用户输入的过滤条件
        execute_real: true       # 调真实工具（YAML 禁写 tool_result）
    - type: mock_branch_choice
      data:
        branch_id: post_search
        options:
          - id: relax
            label: "🎚️ 放宽条件"
            icon: "🎚️"
          - id: cancel
            label: "❌ 取消"
            icon: "❌"
    - type: stream_end
      data: { finishReason: stop }
```

---

## 5 种事件类型

### 1. `stream_start` / `stream_end` — 流开头/结尾

```yaml
- type: stream_start
  data: { scenario: my_scenario }
- type: stream_end
  data: { finishReason: stop, usage: { totalTokens: 42 } }
```

`finishReason` 取值：`stop` / `tool_calls` / `length` / `timeout`

### 2. `text_delta` — 推一段流式文本

```yaml
- type: text_delta
  data: { text: "正在搜索符合条件的视频..." }
```

✅ **允许的文案**：通用、过程性、不含具体数据

- "正在搜索..."
- "搜索完成"
- "处理中..."
- "请选择操作"
- "好的，已放宽条件"

❌ **禁止的文案**：含具体数据 / 模拟结果

- "找到 0 个文件" ← 计数是真实结果决定的
- "Movies/2024/big.mp4 是 500MB" ← 文件名/大小是真实数据
- "ERROR: timeout after 30s" ← 错误信息是真实工具产生的
- "成功重命名为 My New Title" ← 新名字是工具返回的

### 3. `tool_call` — 调一个工具（**自动产生 tool_result**）

```yaml
- type: tool_call
  data:
    id: call_srm_1
    name: search_files
    args: { ext: ".mp4", min_size: 104857600 }
    auto_run: true
    needsConfirm: false
    kind: fileSearch
    execute_real: true       # 默认 true（生产环境）
```

| 字段 | 必填 | 用途 |
|------|------|------|
| `id` | ✅ | call ID（前端追踪用） |
| `name` | ✅ | 工具名（注册到 `tools.GlobalRegistry`） |
| `args` | ✅ | 工具参数（**过滤条件**，不是数据） |
| `auto_run` | ❌ | true → 自动跑，false → 需用户确认 |
| `needsConfirm` | ❌ | 写操作必须为 true |
| `kind` | ❌ | `readOnly` / `fileSearch` / `fileChange` / `metadata` |
| `execute_real` | ❌ | true → 调真实工具；false → 仅推 tool_call 不执行（演示取消路径） |

**关键认知**：YAML 写 `tool_call` → MockEngine **自动**调真实工具 → **自动**生成 `tool_result` 事件 → 推给前端。YAML 禁写 `tool_result`。

✅ **允许的 args**（用户输入的过滤条件、不是数据本身）：

- `args: { ext: ".mp4" }` — 文件扩展名过滤
- `args: { min_size: 104857600 }` — 大小阈值（100MB）
- `args: { pattern: "S01E*" }` — 名字 pattern
- `args: { max_count: 5 }` — 限制数量

❌ **禁止的 args**（不应在剧本里指定具体文件）：

- `args: { path: "/Movies/2024/big.mp4" }` ← 不指定具体文件
- `args: { file: "MyVideo.mp4" }` ← 不指定具体文件
- `args: { files: [...] }` ← 不列举文件列表

### 4. `mock_branch_choice` — 推分支选项 chip 列表

```yaml
- type: mock_branch_choice
  data:
    branch_id: post_search    # 全局唯一
    options:
      - id: relax
        label: "🎚️ 放宽条件"
        icon: "🎚️"
      - id: change_ext
        label: "📁 改其他格式"
        icon: "📁"
      - id: cancel
        label: "❌ 取消"
        icon: "❌"
```

| 字段 | 必填 | 用途 |
|------|------|------|
| `branch_id` | ✅ | 全局唯一（chip 点击后用来路由） |
| `options` | ✅ | 选项列表，**至少 2 个** |
| `options[].id` | ✅ | 选项 ID（chip 点击后送回后端） |
| `options[].label` | ✅ | chip 显示文本 |
| `options[].icon` | ❌ | emoji 图标 |

### 5. （engine 自动生成）`tool_result`

**不要写在 YAML 里** — MockEngine 在 `tool_call` 后自动调真实工具，自动产生此事件。

```yaml
# ❌ 严禁 — loader 直接拒绝
- type: tool_result
  data:
    id: call_1
    result: '{"matches": [...]}'    # ← 这是模拟数据
```

---

## 完整示例

参见 [EXAMPLE_basic.yaml](EXAMPLE_basic.yaml) 和 [EXAMPLE_branch.yaml](EXAMPLE_branch.yaml)。

---

## 启用方式

### 方式 1：CLI flag

```bash
encv start --mock-scenarios-dir=./internal/server/mock_scenarios/builtin
```

可选：启用热重载

```bash
encv start \
  --mock-scenarios-dir=./internal/server/mock_scenarios/builtin \
  --mock-scenarios-hot-reload
```

热重载行为：检测到 `.yaml` / `.json` 变更 → 500ms 防抖 → 重新加载。**活跃 stream 不中断**（旧剧本继续），新 stream 用新剧本。

### 方式 2：配置文件（config.user.json / config.dev.json）

```json
{
  "agent_settings": {
    "mock_scenarios_dir": "./internal/server/mock_scenarios/builtin",
    "mock_scenarios_hot_reload": true
  }
}
```

### 优先级

```
CLI flag > config.user.json > config.dev.json
```

### 目录为空时

- YAML 目录不存在 / 无 `*.yaml` / `*.json` → **自动**降级到 Go 字面量剧本（12 个 v1 + 8 个 v2）
- 启动 log：`No YAML scenarios found, using 20 Go-literal fallbacks`
- **零回归**：现有 demo / 单元测试不受影响

---

## 校验失败的行为

启动时 YAML 校验失败 → log error，但**不**阻断启动：
- 单个文件失败 → 跳过该文件，其余继续
- 全部失败 → 降级到 Go 字面量

**CI 强校验**（红线测试）：
- `TestScenario_NoHardcodedData_AllBuiltinScenarios` — 扫所有内置 YAML，匹配硬编码模式 → 测试失败
- 这条测试是 CI 门禁，演示团队加新剧本必须通过

---

## 调试技巧

### 1. 启动 log 看加载情况

```
mock scenarios: YAML loaded and bound to MockEngine dir=./mock_scenarios count=12
```

### 2. 热重载触发

```
mock scenarios: change detected file=./mock_scenarios/01_xxx.yaml op=WRITE
mock scenarios: loaded from YAML count=12 dir=./mock_scenarios
```

### 3. 单个文件校验失败

```
mock scenarios: skip file (validation failed) path=./mock_scenarios/bad.yaml
  error=scenario "x" step #0 ev #2: field "text" contains hardcoded path "Movies/2024/big.mp4"
```

按错误提示修 → 重启或触发热重载。

---

## 与 v1 Go 字面量剧本的关系

**v1 Go 字面量剧本保留为 fallback**：
- `internal/server/agent_mock_scenarios.go` — 12 个 v1 剧本
- `internal/server/agent_mock_v2_scenarios.go` — 8 个 v2 剧本

**优先级**：YAML 同 id > Go 字面量

**降级触发**：
- `mock_scenarios_dir` 为空
- 目录存在但无 `*.yaml` / `*.json`
- 目录存在但所有文件校验失败

**不要删除 Go 字面量** — 是 fallback，是向后兼容的护城河。

---

## 加新剧本的标准流程

1. 复制 [EXAMPLE_basic.yaml](EXAMPLE_basic.yaml) → `internal/server/mock_scenarios/builtin/13_xxx.yaml`
2. 改 `id` / `description` / `keywords`
3. 改 `steps.events`（按你的剧本逻辑）
4. 重启 / 触发热重载
5. UI 触发 → 验证流程

### 验收清单

- [ ] `id` 全局唯一
- [ ] 至少 1 个 `step`，每个 `step` 至少 1 个 `event`
- [ ] 没有 `tool_result` 事件
- [ ] 没有 `{{` / `{%` 模板语法
- [ ] 没有硬编码路径 / 文件名 / 大小 / 计数 / 错误信息
- [ ] `mock_branch_choice` 的 `options` ≥ 2
- [ ] `tool_call` 有 `name` / `id` / `args`
- [ ] 演示流程：UI 触发 → 真实工具执行 → tool_result 渲染 → 分支 / 收尾

---

## 常见错误

| 错误 | 原因 | 修复 |
|------|------|------|
| `scenario "x": type="tool_result" is FORBIDDEN` | YAML 写了 tool_result | 删除该 event；由 engine 自动产生 |
| `field "text" contains template syntax "{{"` | text_delta.text 写了 `{{...}}` | 改成静态字符串 |
| `field "text" contains hardcoded path "Movies/2024/big.mp4"` | YAML 硬编码路径 | 改成 tool_call.args + 真实工具 |
| `field "text" contains hardcoded size "524MB"` | YAML 硬编码文件大小 | 同上 |
| `scenario "x" missing id` | 缺 id | 加上 `id: xxx` |
| `mock_branch_choice options must have at least 2 entries` | options 数组 < 2 | 加 chip |

---

## 跨层参考

| 主题 | 文档 |
|------|------|
| v1 Go 字面量剧本（fallback） | [agent_mock_scenarios.go](../agent_mock_scenarios.go) |
| v2 Go 字面量剧本（fallback） | [agent_mock_v2_scenarios.go](../agent_mock_v2_scenarios.go) |
| 加载器实现 | [mock_scenario_loader.go](../mock_scenario_loader.go) |
| Schema 与校验 | [mock_scenario_schema.go](../mock_scenario_schema.go) |
| 自动 tool_result 生成 | [agent_mock_executor.go](../agent_mock_executor.go) |
| branch-pick API | [agent_branch_pick.go](../agent_branch_pick.go) |
