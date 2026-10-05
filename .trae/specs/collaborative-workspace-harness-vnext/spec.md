# 协同工作区 vNext：远程调试、云控热更新与 Agent Harness 重构规格

> 状态：Draft / 架构基线
>
> 日期：2026-10-06
>
> 协同工作区基线：`bec539e6a7eab0ab33249a05aa57c468edb6d81b`
>
> 参考仓库：`deepseek-ai/deepseek-harness@5badb15009ae1756c3afe0ae0cef1faafc290ccc`（本地 `/tmp/deepseek-harness`）

## 1. 文档定位

本文定义协同工作区下一代远程调试、云控热更新与 Agent 运行时的统一规格。它不是现有演示功能的增量美化，也不以已有 checklist 的勾选状态作为完成证据；它首先建立真实运行时、持久状态、控制协议和验收证据的共同基础，再迁移现有功能。

本文对以下旧规格中的 Agent 架构结论具有取代关系：

- `.trae/specs/go-in-process-agent/spec.md`
- `.trae/specs/mobile-agent-2026-gap-analysis/spec.md`
- `.trae/specs/codex-web-gap-analysis/spec.md` 中与 Agent 实时状态相关的部分

`docs/cloud-hot-update-and-link-recovery.md` 与 `.trae/specs/desktop-web-android-pairing/spec.md` 中已经通过真实设备验证的 PeerLink、Bundle 原子替换和设备自恢复结论继续有效；本文取代其尚未完成的云控策略、鉴权、发布、观测和 Agent 扩展方向。

### 1.1 事实标签

本文使用三种标签，禁止混用：

- **已证实**：由当前源码、自动化测试或已有真机记录直接证明。
- **待复现**：源码显示存在结构性风险，但尚未在当前版本真实运行复现。
- **目标**：本规格要求的新行为，当前不能宣称已实现。

任何实施进度只能由可定位的证据记录推进，不能仅靠 checklist 勾选、HTTP 200、构建成功、Mock 剧本或静态截图判定完成。

## 2. 当前系统基线

### 2.1 实际拓扑

```text
浏览器 / Android WebView
        │ REST + SSE
        ▼
preview-gateway（开发环境）
        │
        ▼
encv-go / internal/server
  ├─ 生产 Agent API 与工具循环
  ├─ PeerLink Hub / Edge
  ├─ 诊断工具
  └─ Bundle 仓库、下发和本地安装

/workspace/agent
  └─ 独立 Go module 与 agent-demo；当前未被 /workspace/cmd 或 /workspace/internal 导入
```

### 2.2 Agent 现状与缺口

| 事实 | 证据 | 判断 |
|---|---|---|
| 生产路由由 `internal/server` 实现 | `internal/server/agent_api.go` 注册 `/api/chat`、`/api/confirm`、`/api/resume` | 生产真源不在顶层 `agent/` module |
| 顶层 `agent/` 已实现另一套会话、Provider、Hook、JSONL、Replay | `agent/agent.go`、`agent/session_store.go`、`agent/provider_bridge.go` | 两套运行时并存，但没有生产接线；维护和测试结论会分叉 |
| 生产会话是进程内全局 map，并在空闲 30 分钟后回收 | `internal/server/agent_tool_loop.go` 的 `sessions`、`sessionIdleTTL` | 进程重启或 GC 后无法恢复真实执行状态 |
| legacy SSE 写入 `EventCache`，AG-UI 路径明确不写缓存 | `internal/server/agent_tool_loop.go` 的 `streamChat` | 默认协议与 `/api/resume` 的数据源不一致，不能形成统一续传保证 |
| 上游流 channel 满时直接丢事件 | `callOpenAIStream` 的非阻塞 `default` 分支 | UI 可能得到不可恢复的缺口；当前没有 gap frame |
| 前端会话正文由 `localStorage` 保存 | `useAgent.ts` 的 `saveState` / `loadState` | 浏览器副本被当成历史来源，无法跨端共享，也无法证明与服务端一致 |
| `appServerRealtimeReducer.ts` 仅被测试引用 | 生产 `useAgent.ts` 未导入它，而是维护另一套 sequence 状态 | “统一 reducer 已完成”的 checklist 与生产接线不一致 |
| 前端 `send()` 在 streaming/confirming 时拒绝非 queue 请求 | `useAgent.ts` 的 `send` | 当前 `steer` 不是对运行中步骤的真实引导 |
| 前端 queue 期待 HTTP 202，生产 `handleAgentChat` 没有 queue 分支 | `useAgent.ts#sendQueued` 与 `internal/server/agent_chat.go` | queue 契约在两套 Agent 实现之间错位 |
| 生产 Go 路径没有 subagent/team 实现 | `internal/` 仅出现 plan 名称解析；`AgentTaskMessage` 主要是前端展示类型 | 当前没有可运行、可恢复、可鉴权的多 Agent 协同 |
| Mock 剧本、分支、预设和真实工具混在生产路由 | `internal/server/agent_mock*.go`、`agent_chat.go`、`useAgent*.ts` | Mock 可以演示 UI，但不能证明真实 Provider、传输、恢复或工具生命周期 |
| 演示面比"内置剧本"更大：还有外置 YAML/JSON 剧本目录 + 热重载，以及独立 v2 多轮/分支引擎 | `mock_scenarios_integration.go`、`mock_scenario_loader.go`、`agent_mock_v2.go` | 剧本可来自 `mock_scenarios_dir`，v2 引擎支持 round/branch；原"12 个内置剧本"的描述低估了演示链路 |
| **已证实缺陷（Round 1 修复）**：`readAgentConfig` 用 `map[string]string` 解析 `agent_settings` | `internal/server/agent_config.go` | 任何非字符串字段（`mock_speed` / `enabled_tools` / `max_tool_calls_per_turn`）都会让整段解析失败 ⇒ API Key 静默为空 ⇒ `/api/chat` 503 `no_api_key`；用户现象是"填了 Key 却说没配置" |

### 2.3 远程调试现状与缺口

**已证实能力：**

- PeerLink 使用 Android 端主动出站 WSS，具备心跳、退避重连、单设备连接替换和单设备并发上限。
- 远程调用已有 `callId` 幂等表、执行端审批、超时、熔断与有限审计。
- `get_device_info` 能读取进程、版本、挂载、索引、PeerLink 和 Web Bundle 状态。
- `read_logs` 能在执行端完成级别、来源、关键词、时间和数量过滤。

**已证实缺口：**

- 当前能力是“远程调用两个诊断工具”，不是可附着、可持续跟随、可恢复的调试会话。
- 没有统一 `RuntimeTarget` 身份，无法严格关联设备、Go 进程、WebView 页面、当前资源包和一次调试连接。
- 没有远程 Console、Network、DOM/组件树、性能、崩溃快照或进程 profile 的统一 source 协议。
- 日志游标使用 `HH:MM:SS` 字符串，跨日、同秒多条和多源严格排序均没有稳定身份。
- 发起端审计只存内存，重启后消失。
- `isOperator()` 仅检查 `X-Peerlink-Operator: 1`；源码已明确标记真实鉴权尚未实现。
- `peerTokenFrom()` 接受 query token，Bundle 下载也把 token 放入 URL。
- AEAD 的 `Seal/Open` 只在测试和密钥代码中出现，`Caller.Call` 与 `ConnRegistry.WriteJSON` 发送的是明文 JSON；因此现状不能宣称 Hub 不可见业务载荷。

### 2.4 云控热更新现状与缺口

**已证实能力：**

- 控制面通过 PeerLink RPC 下发，数据面由设备主动 HTTP 拉取。
- 目录包和单文件包支持 SHA-256、大小限制、Zip-Slip 防护、必含文件检查、staging、同卷 rename、单份 backup 与手工回滚。
- 支持 `web`、`preview-assets`、`go-binary`，并有设备实际版本探针和持久化报告。
- 已有真机记录证明 Web Bundle 下发、回滚、台账恢复和版本探针曾经工作。

**已证实缺口：**

- 仓库目录扫描和 `.sha256` sidecar 是发布真源，没有签名清单、发布身份、不可变 artifact id 或密钥轮换。
- SHA-256 只能证明下载内容与 Hub 声明一致，不能证明发布者身份。
- `push` 是一次同步 RPC；没有持久 desired state、离线设备补偿、调度队列、灰度、暂停、继续或批量回滚。
- “RPC 返回成功”“文件已替换”“新版本已启动”“目标页面已加载”“业务健康”尚未拆成独立状态。
- Web 和 Go Binary 都需要重启后才真正切换，但当前没有统一激活握手和健康门禁。
- 只保留一份 backup，无法按 release 回退多代，也无法可靠回答当前 active/staged/previous 的完整状态。
- 部分元数据写失败被忽略；磁盘内容与台账可能分叉。
- Bundle 下载 token 位于 query string，可能进入代理、访问日志和诊断记录。

### 2.5 规格与证据现状

- 旧 spec/checklist 存在“总项已完成、子项仍未完成”“README 仍称 Phase 2 pending、源码却已继续实现”“测试文件存在但生产没有接线”等冲突。
- Mock 剧本覆盖了 UI 分支，但没有证明真实 Provider、真实工具、进程重启、网络断连、设备激活和回滚闭环。
- 本次分析只确认源码与既有文档事实；除已有文档明确记录的真机结果外，不新增“运行已验证”声明。
- **Round 1 已落地**：运行 profile 门禁（production 下 mock 恒为 `off`）、`internal/evidence` 证据包，以及上表 `readAgentConfig` 配置解析缺陷修复。详见同目录 [`progress.md`](progress.md)；文档会随实施继续纠偏。
- **Round 2 已落地**：`GET /api/runtime` 对外声明 `profile` / `mock_allowed`；前端（`useAgentChatView` + `AgentChat.vue`）在 production 下隐藏 mock 徽章、v2 剧本入口与预设 chip。详见 [`progress.md`](progress.md)。
- **Round 4 已落地**：新增「模拟服务商」`internal/stubprovider` —— OpenAI 兼容的替身大脑，只替代"说什么/调哪个工具"，工具调用与结果仍由**真实执行**产生（`ENCV_STUB_PROVIDER=1` 显式启用，`/api/runtime.stub_provider_enabled` 对外声明）。这是替代"假剧本"的正确演示方式：演示即真实链路回归。详见 [`progress.md`](progress.md)。
- **Round 3 已落地（纠偏）**：剧本新增 `expect` 断言（`internal/server/mock_scenario_assert.go`），真实工具结果必须过断言，失败记入 `MockEngine.LastFailures()` 并推 `mock_assert_failed`；同时**删除 `realExecutor=nil` 回退硬编码假数据**的路径。理由：只做"生产禁用"等于把垃圾剧本藏起来，剧本本身必须能验证东西。详见 [`progress.md`](progress.md)。

## 3. 从 DeepSeek Harness 借鉴什么

DeepSeek Harness 当前仍是开发者预览，本文借鉴其可验证的结构，不复制其 TypeScript/Cordis 实现，也不把其 API 稳定性当作本项目承诺。

### 3.1 必须吸收的模式

1. **能力 seam 完整化**：每项能力同时定义 Definition、Provider、Consumer；业务不直接越层调用具体实现。
2. **插件拥有可逆生命周期**：注册、监听、后台任务和资源句柄都归一个 scope；卸载必须等待释放完成。
3. **持久事实与实时信号分离**：Session/Deployment/Team 的权威状态来自追加日志；流式 chunk、进度和连接状态是可丢失的实时信号。
4. **模型可见即已记录**：凡进入下一次模型请求的用户消息、系统提示、工具调用、工具结果和上下文变更，都必须能从持久日志重建。
5. **单一 inbox**：start、queue、steer、远程消息和恢复都进入同一会话队列，不维护互相竞争的第二套消息状态机。
6. **工具流水线**：`pre-policy → approval → monotonic guards → execute → post-policy → immutable result → append`，拒绝和失败也形成完整结果。
7. **版本化持久格式**：当前 writer version 是代码真源；已发布 generation 不覆盖、不改名，迁移只走相邻版本。
8. **投影而非复制状态**：UI、列表、任务板、诊断页从事件投影读取，不各自维护权威副本。
9. **可继续子 Agent**：持久 Session 与进程内 Activation 分离；冷恢复不依赖原 transport/provider 仍在线。
10. **Team 的持久协作原语**：roster、mailbox、任务 DAG、CAS revision 和来源归因，而不是只渲染“子任务卡片”。
11. **调试 target 身份**：Host/Client source 使用稳定逻辑 id + transport generation；重连不伪装成同一物理连接。
12. **独立调试 Worker/服务**：调试 Host 主线程时，控制通道不能跟被暂停线程一起停止。
13. **发布元数据最后切换**：先上传不可变制品与差分数据，全部资格验证通过后再原子发布 channel 指针。
14. **真实 GUI 反馈闭环**：必须验证用户正在使用的确切 origin、进程、构建和页面 identity；HTTP 200 不能代表 UI 正确。

### 3.2 明确不照搬

- 不引入 Cordis 或把 Go 服务改写成 TypeScript。
- 不把任意代码执行的 CDP endpoint 暴露到公网；DeepSeek Inspector 自身也只允许 loopback。
- 不把 Electron 完整应用更新模型直接套到 Android Web/Go Bundle；只借鉴签名、不可变制品、channel 发布、资格验证和 journal。
- 不把 Agent Teams 的 `writeScopes` 仅作为提示。协同工作区的文件修改最终必须由工具层执行租约或冲突检查。
- 不把 DeepSeek Harness 的开发者预览 API 当成兼容标准；本项目拥有自己的版本协议。

## 4. 目标架构

```text
┌──────────────────────────── Surfaces ────────────────────────────┐
│ Web Console │ Android WebView │ CLI/Automation │ Debug Viewer   │
└──────────────────────────────┬───────────────────────────────────┘
                               │ authenticated typed API + streams
┌──────────────────────────── Gateway ─────────────────────────────┐
│ Identity/Auth │ Request Codec │ Stream Mux │ Rate/Quota │ Audit  │
└───────────────┬───────────────────────────────┬───────────────────┘
                │                               │
┌───────────────▼──────── Harness Runtime ──────▼───────────────────┐
│ Session Ledger │ Agent Runtime │ Tool Pipeline │ Policy/Approval │
│ Inbox          │ Jobs          │ Team Runtime  │ Projections     │
└───────────────┬───────────────────────────────┬───────────────────┘
                │ capability providers          │ command/events
┌───────────────▼──────────────┐   ┌────────────▼──────────────────┐
│ Local providers             │   │ PeerLink Control Plane        │
│ fs/shell/plugin/llm/debug   │   │ device/target/session/command │
└─────────────────────────────┘   └────────────┬──────────────────┘
                                               │ WSS control
                                  ┌────────────▼──────────────────┐
                                  │ Device Edge Runtime          │
                                  │ Debug Sources │ Reconciler   │
                                  │ Bundle Slots  │ Health Probe │
                                  └────────────┬──────────────────┘
                                               │ HTTPS artifact data
                                  ┌────────────▼──────────────────┐
                                  │ Immutable Artifact Store     │
                                  │ signed manifest + channels   │
                                  └───────────────────────────────┘
```

### 4.1 所有权

| 组件 | 唯一职责 | 禁止承担 |
|---|---|---|
| Gateway | 鉴权、wire codec、RPC/stream 关联、取消、限流 | 业务状态、Agent 对象缓存、部署决策 |
| Session Ledger | 追加、读取、版本迁移、flush、stream cursor | UI 状态、LLM Provider、工具执行 |
| Agent Runtime | inbox、turn/step 状态机、模型请求、恢复 | HTTP/SSE 格式、PeerLink 连接、Bundle 安装 |
| Tool Pipeline | 工具注册、策略、审批、执行、结果归一化 | 会话历史复制、页面渲染 |
| Team Runtime | roster、mailbox、任务 DAG、ownership | 直接写文件或绕过工具策略 |
| Debug Plane | target/source 注册、观测流、调试命令 | 发布决策、业务工具冒充调试命令 |
| Deployment Controller | desired state、rollout、状态聚合、回滚决策 | 直接改设备文件 |
| Edge Reconciler | 拉取、验签、staging、激活、健康验证、回报 | 自己决定目标版本或 rollout |
| UI Projection | 将权威状态投影为界面 | 把 `localStorage` 当服务端真源 |

## 5. 统一基础契约

### 5.1 稳定身份

必须使用不同的品牌化或强类型 id，禁止用裸字符串混用：

- `WorkspaceId`
- `SessionId`
- `AgentId`
- `RunId`
- `TurnId`
- `ToolCallId`
- `TeamId`
- `TeamTaskId`
- `DeviceId`
- `RuntimeTargetId`
- `DebugSessionId`
- `ArtifactDigest`
- `ReleaseId`
- `DeploymentId`
- `CommandId`

`DeviceId` 表示稳定设备身份；`RuntimeTargetId` 必须包含或关联 `DeviceId + processInstanceId + surfaceId + generation`，不得只用 IP、端口或显示名。

### 5.2 统一事件信封

所有持久与实时事件使用同一基础信封：

```json
{
  "schemaVersion": 1,
  "stream": "agent|team|debug|deployment",
  "streamId": "opaque-id",
  "sequence": 42,
  "eventId": "uuid",
  "occurredAt": "RFC3339Nano",
  "producerInstanceId": "opaque-id",
  "correlationId": "opaque-id",
  "causationId": "opaque-id",
  "type": "domain/event",
  "ignorable": false,
  "data": {}
}
```

约束：

1. `sequence` 在单个 `streamId` 内严格递增。
2. 已提交事件不可覆盖；修正通过新事件表达。
3. 未识别且 `ignorable=false` 的事件必须拒绝读取，不能静默跳过。
4. 持久事件与实时 frame 使用不同 `type` 空间；实时 chunk 不得伪装成已持久事实。
5. 客户端只保存 cursor 和可丢弃 projection cache，不保存第二份权威历史。
6. 任一副作用必须能用 `correlationId/causationId` 追到用户动作、Agent 工具调用或 Deployment command。

### 5.3 Stream follow

统一提供“快照 + 增量”语义：

- 首次订阅返回当前 projection snapshot 与 `headSequence`。
- 后续按 `afterSequence` 返回严格有序增量。
- 若 cursor 早于保留窗口，返回显式 `stream/gap`，客户端必须重新取快照。
- transport 断开不结束逻辑 stream；重连携带 stream id、producer instance 和 cursor。
- producer instance 改变时，客户端不得简单清空后继续猜；必须重新取权威 snapshot。
- legacy SSE 与 AG-UI 只能作为同一事件流的适配器，禁止各自拥有缓存和恢复规则。

### 5.4 命令与幂等

所有远程副作用统一使用持久 `Command`：

```text
created → admitted → running → succeeded
                    ├→ failed
                    ├→ rejected
                    ├→ cancelled
                    └→ rolled_back
```

- `CommandId` 由发起端生成，在执行端副作用边界持久去重。
- 相同 id + 相同摘要返回原结果；相同 id + 不同请求必须拒绝为冲突。
- 调用超时只表示发起端未拿到结果，不代表执行失败；重试必须查询或复用同一个 command。
- 审批、执行、结果、回滚均写入事件账本。
- 进程重启不能清除已成功副作用的幂等证据。

### 5.5 鉴权与授权

1. 删除 `X-Peerlink-Operator: 1` 作为授权依据；它只能在迁移期作为 UI 来源提示，不能授予权限。
2. Operator 使用短期登录会话或签名 token，包含 actor、scope、expiry 与 nonce。
3. Peer token 只允许在 `Authorization` header 或受保护的 WebSocket subprotocol 中传递，禁止 query string。
4. PeerLink v2 frame 必须使用已派生方向密钥做 AEAD，绑定 `peerId`、`connectionGeneration`、`sequence`、`method` 与 `commandId` 作为认证上下文。
5. 调试和更新权限按 capability lease 授予；默认只读、短时、目标限定、可撤销。
6. 任意代码执行、shell、Debugger pause/evaluate、二进制更新属于高风险能力，必须设备端显式批准；无人应答时拒绝。
7. 所有拒绝必须 fail closed，并给调用方结构化原因。

## 6. Agent Harness 规格

### 6.1 单一生产运行时

**目标：** 将 `/workspace/agent` 与 `internal/server/agent_*` 收敛为一个生产运行时。迁移期间推荐新建 `internal/harness/`，由 HTTP、PeerLink、CLI 和测试适配器共同调用。

要求：

- 只有一个 Session 类型、一个事件词汇、一个 inbox、一个工具注册表和一个审批策略入口。
- `internal/server` 只保留 transport adapter，不再持有 Agent 状态机。
- 顶层 `/workspace/agent` 的可复用实现经审计后迁入；未迁入代码删除，不保留第二套“备用实现”。
- 所有生产入口通过同一 composition root 创建运行时。
- 启动时输出可检查的 capability inventory 与协议版本。

### 6.2 插件和能力 seam

Go 侧插件至少提供：

```text
Plugin.Describe() -> id/version/requires/provides
Plugin.Start(scope) -> registrations
Scope.Dispose(ctx) -> wait for quiescence
```

首批 seam：

- `ModelProvider`
- `SessionPersistence`
- `ToolRegistry` / `ToolExecutor`
- `ApprovalProvider`
- `SandboxProvider`
- `WorkspaceProvider`
- `JobProvider`
- `SubagentProvider`
- `TelemetrySink`
- `DebugSource`
- `ArtifactStore`

每项 seam 必须有 Definition、至少一个 Provider、至少一个真实 Consumer；只有接口或只有 UI 均不算完成。注册返回 disposer，热卸载先关闭准入，再等待在途操作，最后撤销注册。

### 6.3 Session 与 Agent 生命周期

持久事件最小集合：

- `session/created`
- `turn/started`、`turn/ended`
- `step/started`、`step/ended`
- `system/message`
- `user/message`
- `assistant/settled`、`assistant/attempt`
- `tool/called`、`tool/result`
- `approval/requested`、`approval/decided`
- `inbox/enqueued`、`inbox/claimed`、`inbox/discarded`
- `agent/interrupted`、`agent/resumed`
- `session/compacted`

规则：

- 模型请求必须完全由 Session 日志投影生成，不接受前端把完整历史重新上传作为权威输入。
- live delta 可走瞬态 stream；成功、失败、取消和中断都必须形成 settlement。
- 进程重启时，未闭合 turn/step 被修复为 `interrupted`，不得伪装成功。
- queue 与 steer 都写入同一 inbox。queue 等待后续 turn；steer 只在最近 step 边界领取，无法领取时按明确策略转 queue 或拒绝。
- Session 存储采用 SQLite WAL 作为在线真源，提供版本化 JSONL 导出；不得再以浏览器 `localStorage` 或进程内 slice 作为权威历史。
- 每个已发布格式都提供 schema、相邻迁移和拒绝未来版本的测试。

### 6.4 工具执行流水线

固定顺序：

```text
validate schema
→ resolve session/workspace policy
→ pre-execute hooks
→ monotonic guards
→ optional approval
→ execute with deadline/cancellation
→ post-execute hooks
→ normalize immutable result
→ append tool/result
→ publish projection
```

要求：

- 工具注册时声明 effect：`read | write | process | network | device | update | debug-control`。
- 权限由策略与 guard 强制，不允许 `full-access` UI 值直接绕过破坏性工具确认。
- 工具结果必须区分 `success | rejected | failed | cancelled | timed_out`。
- 大输出进入 spill/artifact store，事件只保留摘要和引用。
- Tool handler 不直接写 SSE，不直接改 Vue 状态，不直接维护 Session history。
- Hook 错误不能一律吞掉；每个 hook 声明 `observe | transform | guard` 模式及失败策略。

### 6.5 Subagent 与协作 Team

第一阶段不追求自动“群聊”，先实现可恢复的协作原语：

- Subagent 有稳定 `SessionId`，一次运行的 `ActivationId` 与持久身份分离。
- Provider 明确声明能力；不支持的 persona、工具限制、结构化输出或继续执行必须提前拒绝。
- Team roster、mailbox、task board 写入 Lead Session 的持久日志。
- mailbox 使用 queued-minus-delivered 恢复语义，目标 inbox 接纳后才确认 delivered。
- task 使用单调 `revision` 做 CAS；`blockedBy` 必须无环。
- 每个任务声明 `readScopes` 与 `writeScopes`。展示层可给冲突警告，真正写操作还必须由 Tool Pipeline 校验活动写租约。
- Agent 之间只允许直接 parent/child 或同 Team 已授权成员通信；消息保留 sender identity。
- interrupt 只停止当前 activation，不清除未领取 inbox；恢复从同一 Session 继续。
- Team UI 只渲染投影，不可用前端数组替代 roster、mailbox 或任务真源。

### 6.6 Mock、Replay 与验收隔离

- `MockEngine` 移出生产 composition，只有 test/evaluation profile 可启用。
- Mock 事件必须携带 `synthetic=true`，并在 UI 与证据中永久可见，不能通过配置伪装真实运行。
- Replay 使用真实 Session event schema 和真实投影器，不维护另一套剧本专用状态机。
- Mock 可以验证渲染分支和故障注入，不能验收 Provider、网络、工具副作用、设备状态、热更新生效或多 Agent 协作。
- 产品级验收不得以预设 chip、硬编码工具结果或 mock 分支完成情况作为成功依据。

## 7. 远程调试规格

### 7.1 Runtime Target Registry

每个可调试目标注册：

```text
RuntimeTarget
  id
  deviceId
  kind: go-runtime | android-service | webview | web-surface | worker
  processInstanceId
  surfaceId
  generation
  appVersion
  runtimeVersion
  activeBundles[{name, version, digest, source}]
  capabilities[]
  startedAt
  lastSeenAt
```

同一页面刷新保留逻辑 `surfaceId`，但增加 `generation`；进程重启必须更换 `processInstanceId`。所有截图、DOM、Console、Network、日志、Agent 事件和部署激活回执都携带 target identity。

### 7.2 Debug Session

新增持久 `DebugSession`：

- 由 operator 对指定 `RuntimeTargetId` 创建。
- 声明能力集合、脱敏策略、过期时间和审批凭据。
- transport 断开后 session 保留；重新连接按 cursor 续传。
- 每个命令有 `CommandId`、deadline、状态和结果摘要。
- 默认只读；提升权限形成新的短期 lease，不修改原 session。

建议 API：

- `POST /api/v1/debug/sessions`
- `GET /api/v1/debug/sessions/{id}`
- `GET /api/v1/debug/sessions/{id}/follow?afterSequence=N`
- `POST /api/v1/debug/sessions/{id}/commands`
- `DELETE /api/v1/debug/sessions/{id}`

PeerLink 只承载这些 typed commands/events，不再把调试能力伪装成普通 Agent tool。

### 7.3 分级能力

**P0：只读运行事实**

- 结构化日志，使用 `(sourceId, generation, sequence)` cursor，不再使用 `HH:MM:SS` 作为增量主键。
- 进程、版本、uptime、内存、goroutine 数、挂载、索引、PeerLink、active bundle。
- 健康探针与最近失败分类。

**P1：WebView/Web Surface 观测**

- Console、未处理异常、网络请求元数据、页面 identity、加载资源 digest。
- 有界 body 捕获，默认脱敏 Authorization、Cookie、query secret 和表单字段。
- DOM/组件树使用 detached snapshot；变化以 generation 内增量发布。
- source 队列有条数和字节上限；溢出发布 `debug/gap`，绝不静默丢失。

**P2：高级诊断**

- Go profile、trace、goroutine dump、heap 摘要。
- Android service/logcat 的结构化快照。
- Debugger pause/evaluate 仅允许本机或设备端再次确认的临时高风险 lease；公网默认关闭。

### 7.4 当前页面验收闭环

任何 UI 改动验收必须先解析当前 target：

1. 获取用户当前页面的 `RuntimeTargetId`、origin、surface id、bundle digest 和模式。
2. 修改与构建必须关联同一 checkout 和 artifact digest。
3. 刷新/HMR 后，目标页面上报新的 generation 与实际加载 digest。
4. 浏览器断言真实 DOM、computed style 或交互结果。
5. 证据记录 target identity；替代端口、替代进程或裸 Vite HTTP 200 不得通过验收。

## 8. 云控热更新规格

### 8.1 制品与签名清单

Artifact 必须不可变并以 SHA-256 digest 定址。Release manifest 至少包含：

```text
schemaVersion
releaseId
bundleName
version
artifactDigest
artifactSize
signature
signingKeyId
platform / abi
minBootstrapVersion
minRuntimeVersion
requiredFiles
activationMode
healthProbe
createdAt
```

要求：

- 制品先上传到不可变路径，校验 size/digest/signature 后，再原子更新 channel 指针。
- channel 只引用 release id，不直接承载可变制品。
- 设备同时校验 digest 与签名；未知、撤销或过期 signing key 一律拒绝。
- `web`、`preview-assets`、`go-binary` 使用同一 envelope，但拥有不同的 staging、激活与健康适配器。
- 旧版本制品按保留策略保存，不能只依赖一份 `<name>-backup`。

### 8.2 Desired State，而非一次性 Push

Deployment Controller 为每台设备持久化 desired state：

```text
Deployment
  id
  releaseId
  selector / explicitDeviceIds
  strategy
  desiredState
  revision
  createdBy
  createdAt

DeviceDeployment
  deploymentId
  deviceId
  desiredReleaseId
  observedReleaseId
  phase
  attempt
  commandId
  lastErrorCode
  updatedAt
```

PeerLink `bundle_update` 退化为“立即唤醒 reconcile”的提示。设备上线、进程重启或定时 reconcile 时，都主动比较 desired 与 observed；离线期间的部署不会丢失。

### 8.3 设备状态机

```text
assigned
→ downloading
→ downloaded
→ verified
→ staged
→ activating
→ health-checking
→ committed
   ├→ failed
   ├→ rollback-pending
   └→ rolled-back
```

每次状态迁移先写本地 journal，再执行下一副作用。设备重启后从 journal 恢复，并按幂等规则继续或回滚。不得以 RPC response 代替 observed state。

### 8.4 激活与健康证明

- **Web Bundle**：切换 slot 后触发受控页面 reload；页面 bootstrap 回报 `surfaceId + generation + bundleDigest + version`，并通过真实 DOM/路由/关键 API smoke 后才 `committed`。
- **Go Binary**：Kotlin supervisor 启动候选进程，要求在期限内回报 `processInstanceId + binaryDigest + version + health`；失败自动回旧 slot 并再次启动。
- **Preview Assets**：加载真实入口并校验所需资源，不以文件存在或 HTTP 200 单独判成功。
- health probe 失败必须产生结构化错误、设备侧回滚和云端 observed state 更新。

### 8.5 Rollout

支持：

- `manual-one`
- `canary`：按稳定 DeviceId 哈希选择固定样本
- `percentage`
- `all`

每个 rollout 配置并发上限、批间等待、失败阈值、健康观察窗和自动暂停。强制更新只表示 UI/业务准入策略，不能绕过签名、兼容性、设备审批策略或健康门禁。

### 8.6 回滚

- 回滚目标是明确的 `ReleaseId`，不是模糊的“上一版”。
- 自动回滚和人工回滚都走相同状态机、journal、签名与健康检查。
- 设备至少保留 active、previous-known-good 和 staged 三个 slot；空间不足时按明确策略拒绝部署，不静默删除唯一 known-good。
- 云端展示 desired、observed、active digest、last-known-good、pending command 和证据时间，禁止只展示“曾推送版本”。

## 9. 迁移计划

### Phase 0：停止自欺式验收并封住高风险入口

1. 建立 `EvidenceRecord`，所有“完成”状态必须引用测试运行、浏览器/真机 target 和 artifact digest。
2. 将旧 checklist 标为历史，不再作为当前完成度真源。
3. Mock profile 与生产 profile 物理分离；生产启动拒绝 `mock_mode != off`。
4. 为当前 Agent/PeerLink/Bundle 路径补 capability inventory 和 runtime identity 只读端点。
5. 用真实认证替换 `X-Peerlink-Operator: 1`；停止在 query 中发送 peer token。
6. 为现有 PeerLink frame 增加协议版本和 feature negotiation；未协商能力明确拒绝。

**退出条件：** 未认证浏览器无法执行运维动作；Mock 结果无法生成 production evidence；操作员能确定正在观察哪台设备、哪个进程、哪个页面和哪个 Bundle。

### Phase 1：统一 Harness Kernel 与事件账本

1. 建立 `internal/harness` composition root、生命周期 scope 和 capability registry。
2. 建立版本化 SQLite Session/Event store、projection 和统一 follow stream。
3. 将生产 `internal/server/agent_*` 迁入 canonical Agent Runtime；HTTP/AG-UI/legacy 变为适配器。
4. 审计并迁移 `/workspace/agent` 中仍有价值的 Provider、Hook、Replay、Skill、Compaction；删除未迁移的重复实现。
5. 建立统一 Tool Pipeline 和 durable approval/result。
6. 前端改为消费 server snapshot + cursor，`localStorage` 只保留 UI 偏好和可丢弃缓存。

**退出条件：** 同一真实会话在断网重连和服务重启后得到一致投影；legacy 与 AG-UI 不再有不同恢复语义；生产路由只有一个 Agent Runtime。

### Phase 2：远程调试控制面

1. 实现 Runtime Target Registry 与 Debug Session。
2. 将 `get_device_info`、`read_logs` 迁为 DebugSource provider。
3. 增加 Web Surface source、结构化 Console/Network/页面 identity。
4. 实现 snapshot + delta、gap、generation 和 reconnect。
5. 打通 browser → gateway → hub → edge → source 的 correlation trace。

**退出条件：** 操作员可以在同一个 session 中重连、定位同一逻辑页面的新 generation、读取连续日志，并证明观察的是用户当前页面而非替代服务。

### Phase 3：Deployment Controller 与设备 Reconciler

1. 引入签名 manifest、不可变 Artifact Store 和 channel。
2. 引入 desired/observed state 与 durable command。
3. 设备安装器升级为 slot + journal + health gate。
4. 实现单设备、canary、百分比和全量 rollout。
5. 实现自动暂停、明确 ReleaseId 回滚和重启恢复。
6. Web 与 Go Binary 分别完成真实激活握手。

**退出条件：** 离线设备上线后自动收敛；坏签名不落地；激活失败自动回 known-good；重复命令不重复执行；云端显示真实 observed state。

### Phase 4：Subagent 与 Team Runtime

1. 增加 Subagent provider registry、one-shot 与 continuable 模式。
2. 增加持久 descriptor、cold resume、ownership 和 child-first teardown。
3. 增加 Team roster、mailbox、task DAG、CAS revision。
4. 增加 workspace write lease，并在文件修改工具中强制执行。
5. UI 从 Team projection 渲染真实成员、任务和消息，不再由 mock 事件制造 AgentTask。

**退出条件：** 主 Agent 重启后能恢复 Team；消息不重复不丢失；冲突写被工具层拒绝；中断一个成员不破坏其他成员与未领取任务。

### Phase 5：清理与兼容收口

1. 删除旧全局 session map、旧 Resume cache、重复 Provider bridge 和未接线 reducer。
2. 删除生产 Mock 路由和前端剧本控制面，只保留测试 profile。
3. 为旧 Session、Bundle report 和前端本地记录提供一次性导入或明确不兼容提示。
4. 更新所有旧 spec 的 superseded 标记和迁移链接。
5. 删除没有生产 Consumer 的接口、组件和“未来预留”字段。

## 10. 验收与证据

### 10.1 EvidenceRecord

每条验收记录必须包含：

```text
evidenceId
requirementId
workspaceCommit
artifactDigest
runtimeTargetId
producerInstanceId
mode: real | replay | synthetic
steps
observations
logRefs
result
recordedAt
```

规则：

- `synthetic` 只能满足 UI 单元/组件测试，不得满足 production capability。
- `replay` 可以满足投影兼容性，不得满足网络、设备、更新或副作用执行。
- 真机要求必须包含设备 id、APK/bootstrap 版本、active bundle digest 和时间。
- 浏览器要求必须包含 exact origin 与 surface generation。

### 10.2 必须先红后绿的回归矩阵

**Agent：**

- 断开 SSE 后按 cursor 续传，无重复、无静默缺口。
- 服务进程在模型流、审批、工具执行三个时点分别崩溃，恢复后产生明确 interrupted/continued 结果。
- legacy 与 AG-UI 对同一持久事件投影等价。
- queue FIFO；steer 只在 step 边界生效；无法 steer 时行为明确。
- 工具 guard、审批、执行和结果的顺序可由日志证明。
- Mock profile 无法被 production composition 加载。

**远程调试：**

- 页面刷新复用 logical surface id 并增加 generation。
- Host/Edge 重启后旧 cursor 触发 snapshot 或 gap，不伪装连续。
- 调试 source 队列溢出时产生 `debug/gap`。
- 用户拒绝或审批超时后，高风险命令零执行。
- 断点暂停 Host 时独立调试通道仍能发 resume。
- 验收脚本能识别“错误端口上的替代服务”。

**云控热更新：**

- 摘要正确但签名错误的制品被拒绝。
- 下载中断后可续传或安全重下，不污染 active slot。
- 在每个 journal 状态强制终止进程，重启后都能继续或回滚到确定状态。
- 重复 `CommandId` 不重复安装或回滚。
- Web 页面未回报目标 digest 时不得 committed。
- Go Binary 未回报新进程 identity/版本/健康时自动回旧 slot。
- canary 超过失败阈值后自动暂停，未开始设备不被触达。
- 本地回滚、清数据和离线后重连都能让云端 observed state 收敛。

**协同 Agent：**

- mailbox 在发送、目标接纳和 acknowledgement 各断点崩溃时不丢失、不重复模型可见消息。
- task CAS 拒绝陈旧 revision；DAG 拒绝环。
- 重叠 write scope 的两个 Agent 不能同时取得写租约。
- child 冷恢复保留身份、权限下界、inbox 和 parent 关系。
- Team UI 完全由投影恢复，不依赖旧页面内存。

### 10.3 真实环境矩阵

| 场景 | 最低环境 |
|---|---|
| Agent 运行时与 Session 恢复 | 真实 Provider 或协议级可控测试 Provider；真实工具副作用沙箱 |
| Web UI/HMR | 当前用户 origin 的真实浏览器，断言 DOM/样式/交互和 surface identity |
| PeerLink | Hub + 独立 Edge 进程；至少一次真实公网 WSS |
| Android 调试 | 真机或真实 APK；WebView、Foreground Service、冷启动 |
| Web Bundle 更新 | 真机，真实生产构建产物，不使用只有占位 `index.html` 的测试包 |
| Go Binary 更新 | 真机，引导版 watchdog、候选启动、自动回滚 |
| Rollout | 多设备模拟器集群 + 至少一台真机 canary |

## 11. 非功能要求

- 所有队列、日志、journal、body 和并发都有配置化硬上限。
- 所有长操作都有 deadline、取消和最终状态；客户端断开不等价于业务取消。
- 所有错误使用稳定 code，诊断文本不参与程序分支。
- 所有可恢复数据有 schema version、原子提交与迁移。
- 日志默认脱敏；秘密、token、Cookie、Authorization、绝对私有路径不进入云端记录。
- 生产状态页面必须同时展示 desired、observed、last evidence time 和 stale/gap 标记。
- 任何降级都必须显式显示；不支持的能力不得伪装成功。
- 构建通过是必要条件，不是功能验收。

## 12. 已拍板决策与待决项

### 12.1 已拍板

1. Android 优先，桌面仍是浏览器 + 云端 Hub；本期不引入 Electron/Wails。
2. 保留 PeerLink 作为设备出站控制通道，但升级协议、鉴权、加密和恢复语义。
3. Agent、Debug、Deployment 共用事件信封、身份、命令幂等和证据体系。
4. 生产只保留一个 Agent Runtime。
5. Mock 不再作为产品完成证据。
6. 更新采用签名不可变制品 + desired-state reconcile，不再以一次 RPC 成功代表发布成功。
7. 调试默认只读；任意执行能力默认远程关闭。

### 12.2 后续实施前需确定

1. SQLite 是否复用现有数据库文件，还是为 Harness/Control Plane 使用独立数据库。
2. Artifact Store 首个 Provider 使用本地目录、GitHub Release 还是对象存储。
3. 发布签名私钥托管位置与首批公钥轮换策略。
4. Operator 登录与会话体系接入现有哪个身份源。
5. Android WebView 可提供哪些观测 API；不能安全提供的 CDP 能力必须明确留空，不做假实现。

## 13. 参考映射

### 当前工作区

- Agent 生产路由：`internal/server/agent_api.go`
- Agent 状态机：`internal/server/agent_chat.go`、`agent_tool_loop.go`、`agent_confirm.go`、`agent_sse.go`
- 运行 profile 与 mock 门禁：`internal/server/agent_profile.go`（Round 1）
- Agent 配置解析：`internal/server/agent_config.go`（Round 1 修复非字符串字段致命解析）
- 证据记录：`internal/evidence`（Round 1）
- 重复 Agent module：`agent/`
- 前端 Agent 状态：`app/encv-mobile/src/composables/useAgent.ts`、`useAgentStream.ts`
- PeerLink：`internal/peerlink/`
- 远程调用：`internal/server/peerlink_agent_api.go`
- 远程诊断：`internal/server/agent_diag_bridge.go`
- Bundle 安装：`internal/bundle/apply.go`
- Bundle 云控：`internal/server/peerlink_bundle.go`
- 既有热更新台账：`docs/cloud-hot-update-and-link-recovery.md`

### DeepSeek Harness 参考

- 总体架构：`/tmp/deepseek-harness/docs/architecture.zh.md`
- Agent 生命周期：`/tmp/deepseek-harness/docs/agent-lifecycle.zh.md`
- 工具流水线：`/tmp/deepseek-harness/docs/tool-execution-pipeline.zh.md`
- Session 格式：`/tmp/deepseek-harness/docs/session-format-status.zh.md`
- Subagent：`/tmp/deepseek-harness/docs/subsystems/subagent.zh.md`
- Agent Teams：`/tmp/deepseek-harness/docs/subsystems/agent-team.zh.md`
- Workspace：`/tmp/deepseek-harness/docs/subsystems/workspace.zh.md`
- Telemetry：`/tmp/deepseek-harness/docs/subsystems/session-telemetry.zh.md`
- Sandbox：`/tmp/deepseek-harness/docs/subsystems/sandbox.zh.md`
- API Gateway：`/tmp/deepseek-harness/docs/api-gateway.zh.md`
- Inspector：`/tmp/deepseek-harness/packages/experimental/inspector/README.zh.md`
- HMR：`/tmp/deepseek-harness/packages/boot/hmr/README.zh.md`
- Plugin Manager：`/tmp/deepseek-harness/packages/boot/plugin-manager/README.zh.md`
- Desktop 更新：`/tmp/deepseek-harness/.agents/notes/implemented/architecture/2026-08-25-electron-desktop-packaging-and-updates.zh.md`
- GUI 验收事故复盘：`/tmp/deepseek-harness/docs/postmortem/0003-web-agent-gui-feedback-loop.zh.md`

## 14. 需求索引

| ID | 必须满足的结果 | 阶段 |
|---|---|---|
| `EVD-001` | 每项完成状态绑定可复查的 `EvidenceRecord`，并标明 real/replay/synthetic | P0 |
| `SEC-001` | Operator 使用真实身份与短期授权，任意自声明 header 不授予权限 | P0 |
| `SEC-002` | token、凭据和秘密不出现在 URL、普通日志与云端诊断载荷 | P0 |
| `LINK-001` | PeerLink v2 具备协议协商、generation、sequence、AEAD 和重放保护 | P0 |
| `CORE-001` | 全部生产入口只使用一个 Agent Runtime 和一个 composition root | P1 |
| `CORE-002` | capability seam 具备 Definition、Provider、Consumer 和可等待的释放生命周期 | P1 |
| `EVT-001` | Agent/Team/Debug/Deployment 使用版本化追加事件账本 | P1 |
| `EVT-002` | stream 支持 snapshot、cursor、gap、重连和 producer instance 切换 | P1 |
| `AGT-001` | Agent 使用一个 inbox 和明确的 turn/step/interrupt/resume 状态机 | P1 |
| `AGT-002` | 所有工具调用经过统一策略、审批、guard、执行、归一化与落账流水线 | P1 |
| `AGT-003` | legacy SSE 与 AG-UI 仅是同一权威事件流的适配器 | P1 |
| `AGT-004` | Mock/Replay 与 production composition 物理隔离，synthetic 永久可见 | P0/P1 |
| `DBG-001` | 所有调试数据绑定稳定 target identity 与连接 generation | P2 |
| `DBG-002` | 调试会话可授权、过期、撤销、重连和按 cursor 续传 | P2 |
| `DBG-003` | 日志、Console、Network、运行时状态等通过有界 DebugSource 发布，溢出显式报 gap | P2 |
| `DBG-004` | UI 验收绑定用户当前 origin、surface generation 与实际 bundle digest | P2 |
| `DEP-001` | 更新只消费签名、不可变、内容寻址的 Artifact 与 Release manifest | P3 |
| `DEP-002` | 云端持久化 desired state，设备持续上报 observed state 并自动收敛 | P3 |
| `DEP-003` | 设备使用 durable journal 与 active/staged/known-good slots 恢复更新事务 | P3 |
| `DEP-004` | Web/Go/Preview 激活均有目标专属健康证明，失败自动回滚 | P3 |
| `DEP-005` | 支持单设备、canary、百分比、全量 rollout 及失败阈值自动暂停 | P3 |
| `TEAM-001` | Subagent 持久身份与 Activation 分离，并支持授权冷恢复 | P4 |
| `TEAM-002` | Team mailbox 使用 queued-minus-delivered 语义，消息不丢不重 | P4 |
| `TEAM-003` | Team task 使用 CAS revision、无环依赖和 tombstone | P4 |
| `TEAM-004` | 文件写入由 Tool Pipeline 强制检查 workspace write lease | P4 |
| `MIG-001` | 旧 Session、Bundle report 与前端记录有显式迁移或拒绝策略 | P5 |
| `MIG-002` | 删除重复运行时、未接线 reducer、生产 Mock 控制面与无 Consumer API | P5 |

实现任务和验收项分别维护在同目录的 `tasks.md` 与 `checklist.md`。任何需求只有在 checklist 引用对应 `EvidenceRecord.evidenceId` 后才可标记完成。
