# 协同工作区 vNext 实施任务

> 对应规格：[`spec.md`](spec.md)
>
> 状态：尚未实施。任务完成必须同时更新 [`checklist.md`](checklist.md) 并附 `EvidenceRecord.evidenceId`；禁止仅勾选任务。

## Phase 0：事实基线与安全封口

### T0.1 建立证据系统（`EVD-001`）

- [x] 定义 `EvidenceRecord` schema、存储位置和查询入口。（Round 1：`internal/evidence`，含 Append/List/Filter，见 `progress.md`）
- [x] 自动采集 `workspaceCommit`、`artifactDigest`、`producerInstanceID`。（Round 1：构建期 VCS revision + 可执行文件 sha256 + hostname/pid）
- [ ] 为当前浏览器、Hub、Android、Agent 和 Bundle 生成 target/artifact identity。
- [ ] 把测试、浏览器、真机和 replay 证据显式分类（Record.Mode 已定义，尚未接入各验收入口）。
- [ ] 旧 checklist 增加“历史记录、非当前证据真源”标记。
- [ ] 暴露 HTTP / CLI 查询入口，并让首条需求验收绑定 `evidenceId`。

### T0.2 替换伪 Operator 鉴权（`SEC-001`、`SEC-002`）

- [ ] 为 Web Operator 建立真实登录会话、scope、expiry、CSRF 防护和撤销。
- [ ] 移除 `X-Peerlink-Operator: 1` 的授权语义。
- [ ] Peer token 从 query string 迁移到 header/WebSocket subprotocol。
- [ ] 给旧客户端提供有限迁移窗口；服务端记录使用旧协议的安全告警。

### T0.3 PeerLink v2 信封（`LINK-001`）

- [ ] 定义版本、connection generation、sequence、command id、nonce 和 capability negotiation。
- [ ] 将现有方向密钥真正接入 frame AEAD，并加入重放窗口。
- [ ] 未协商方法返回结构化 `unsupported-capability`，不走字符串猜测。
- [ ] 保留断网、重复帧、乱序帧、重启和密钥轮换的红绿测试。

### T0.4 Mock 与生产隔离（`AGT-004`）

- [ ] 定义独立 test/evaluation composition。
- [x] 后端：生产 profile 下 mock 恒为 `off`、`mock_resume` 返回 404、presets 返回空、外置剧本目录不加载。（Round 1：`agent_profile.go` + `effectiveMockMode`，见 `progress.md`）
- [x] 前端：透出 `runtimeProfile`，production 下隐藏/禁用 mock badge、presets chip 与 `setMockMode` UI。（Round 2：`/api/runtime` 暴露 `profile`/`mock_allowed` + 视图层门禁，见 `progress.md`）
- [ ] 视图层门禁补自动化用例（component/e2e，或把门禁下沉为可单测的纯函数）。
- [ ] 旧 mock HTTP 集成测试显式声明 test profile（Round 1 已改 `newMockTestHTTPServer` 与 custom 用例；全量复查待做）。
- [ ] 所有 synthetic event 和 EvidenceRecord 永久标记 `synthetic=true`。
- [ ] 删除“Mock 流程通过即可证明生产能力”的旧验收入口。

### T0.5 剧本可执行化：从“过场动画”变成验收资产（`AGT-004` 续）

> 起因：用户指出“剧本根本没有达到预期效果，生产禁用 mock 有啥意义”。
> 只做禁用 = 把垃圾剧本藏起来；必须让剧本跑完能证明真实工具链还活着。

- [x] 断言引擎：`Assertion` / `Evaluate` / 极简 JSONPath / 10 种算子，未知算子 fail closed。（Round 3）
- [x] schema → 运行时打通：`YAMLEvent.expect` → `MockEvent.Expect` → `pendingRealCall.expect`。（Round 3）
- [x] 真实结果过断言：失败写入 `MockEngine.LastFailures()` 并推 `mock_assert_failed` 事件。（Round 3）
- [x] 删除假数据兜底：`realExecutor=nil` 且 `execute_real=true` 时拒绝回退硬编码 result。（Round 3）
- [x] 首个带断言剧本：`02_list_files_query.yaml` 的 4 个真实工具调用补 `expect`。（Round 3）
- [ ] 剧本跑批入口（测试或 CLI）：跑全部剧本 → pass/fail + **无断言剧本清单**（当前 20/21 无断言）。
- [ ] 给剩余 `execute_real` 剧本（9 个）补 `expect`。
- [ ] 断言结果接入 `internal/evidence`（持久化，重启不丢）。
- [ ] `mock_assert_failed` 在前端 / DevLogs 可见化。

### T0.6 模拟服务商：用"替身大脑 + 真实链路"替代假剧本（`AGT-004` 续）

> 用户构想：模拟用户配置的 agent 服务商来调用，安卓受控端演示输出（不是假剧本）。
> 区别：mock 假剧本连工具结果都是写死的；模拟服务商只替代模型大脑，工具/结果/传输全真实。

- [x] `internal/stubprovider`：OpenAI 兼容 `/v1/chat/completions`，回合推进、流式/非流式、脚本注册、未知脚本 fail closed。（Round 4）
- [x] 服务端挂载：`ENCV_STUB_PROVIDER=1` 时挂 `/v1/chat/completions` + `/api/stub-provider/v1/chat/completions`。（Round 4）
- [x] `/api/runtime.stub_provider_enabled` 对外声明替身大脑状态。（Round 4）
- [x] 内置脚本：`fs_overview`、`storage`。（Round 4）
- [x] 集成证据：`mock_mode=off` + 无 `X-Mock-*` 头 + 真实文件名出现在输出 + 替身被请求 ≥2 次。（Round 4）
- [ ] 前端显式标注"模拟服务商（工具真实）"，与 mock 徽章区分。
- [ ] PeerLink 远程调试触发替身服务商的路径（服务端先验证，再上真机）。
- [ ] 安卓真机端到端：远程 invoke + 云控热更新 + 真实输出（需设备）。
- [ ] 替身运行结果接入 `internal/evidence`（`replay` 类证据）。

## Phase 1：Harness Kernel 与事件账本

### T1.1 建立单一 composition root（`CORE-001`、`CORE-002`）

- [ ] 新建 `internal/harness` 包边界与生命周期 scope。
- [ ] 定义并注册首批 capability seam。
- [ ] 所有注册返回 disposer；卸载关闭准入并等待在途操作。
- [ ] 启动时发布 capability inventory、协议版本和 provider identity。

### T1.2 建立版本化事件账本（`EVT-001`、`MIG-001`）

- [ ] 定义统一 EventEnvelope 和当前 writer version。
- [ ] 实现 SQLite WAL append、事务、读取、索引和 projection checkpoint。
- [ ] 实现 JSONL 导出，而不是把 JSONL 作为在线并发真源。
- [ ] 定义相邻版本迁移与拒绝未来版本策略。
- [ ] 为旧内存事件、顶层 `agent` JSONL 和前端记录实现导入或明确拒绝。

### T1.3 重建 Agent 生命周期（`AGT-001`）

- [ ] 实现一个 inbox、turn/step 状态机和每 Session 串行执行器。
- [ ] 实现 start、queue、steer、interrupt、resume 的明确状态转换。
- [ ] 进程重启修复未闭合 turn/step 为 interrupted。
- [ ] 模型请求只从权威日志投影，不再接受前端完整历史覆盖服务端状态。

### T1.4 统一工具流水线（`AGT-002`）

- [ ] 工具注册声明 schema、effect、deadline、result policy 和 UI presenter key。
- [ ] 固定 validate → policy → hooks → guards → approval → execute → post → result → append 顺序。
- [ ] 审批和副作用结果持久化；工具大输出进入 spill/artifact store。
- [ ] 迁移 fs、plugin、diagnostic、plan 工具；删除专用旁路。

### T1.5 统一投影与传输（`EVT-002`、`AGT-003`）

- [ ] 实现 snapshot + delta follow 协议及 gap 处理。
- [ ] legacy SSE 与 AG-UI 从同一事件/projection 生成。
- [ ] 前端改为服务端 projection + cursor；`localStorage` 只保留偏好和缓存。
- [ ] 删除未接线的 reducer，或把它改成唯一生产 reducer。

### T1.6 切换生产 Agent（`CORE-001`、`MIG-002`）

- [ ] `/api/chat`、`/api/confirm`、`/api/resume` 适配到新 Runtime。
- [ ] PeerLink Agent 调用通过 Tool Pipeline，不直接调用 `executeAgentTool`。
- [ ] 审计并迁移顶层 `/workspace/agent` 的有效代码。
- [ ] 契约与真实运行全绿后删除第二套状态机。

## Phase 2：远程调试控制面

### T2.1 Runtime Target Registry（`DBG-001`）

- [ ] 注册 Go runtime、Android service、WebView/Web surface 与 worker target。
- [ ] 实现 logical surface id、process instance id、generation 和 heartbeat。
- [ ] target snapshot 包含 app/runtime/bundle digest 与 capability 集合。

### T2.2 Debug Session 与流（`DBG-002`、`EVT-002`）

- [ ] 实现创建、查询、follow、command、关闭 API。
- [ ] capability lease 默认只读、短期、目标限定并可撤销。
- [ ] transport 断开后保持逻辑 session，并按 cursor 恢复。

### T2.3 DebugSource providers（`DBG-003`）

- [ ] 迁移 `get_device_info` 和 `read_logs`，使用稳定 source cursor。
- [ ] 增加 Web Console、异常、Network 元数据、页面 identity 和加载 digest。
- [ ] 增加 Go profile/trace/goroutine 摘要与 Android service 状态。
- [ ] 所有队列溢出发布 gap；敏感字段默认脱敏。

### T2.4 调试 UI 与真实页面验收（`DBG-004`）

- [ ] 展示 target、generation、数据新鲜度、gap 和权限 lease。
- [ ] 页面验证绑定 exact origin、surface generation 和 bundle digest。
- [ ] 实现“错误端口/替代服务”拒绝规则。
- [ ] 用当前真实浏览器和 Android 真机完成证据闭环。

## Phase 3：云控 Deployment Controller

### T3.1 Artifact 与 Release（`DEP-001`）

- [ ] 定义签名 Release manifest、内容寻址路径和 keyring。
- [ ] 构建后执行静态检查、签名、安装资格验证，再生成完成记录。
- [ ] 先发布不可变制品，最后原子更新 channel pointer。
- [ ] Bundle 下载改用 header 鉴权并支持 Range/断点续传。

### T3.2 Desired/Observed State（`DEP-002`）

- [ ] 持久化 Deployment 与 DeviceDeployment。
- [ ] `push` 改为创建/更新 desired state，并用 PeerLink 发送 wake hint。
- [ ] 设备上线、重启和定时任务都执行 reconcile。
- [ ] 云端 UI 同时展示 desired、observed、stale 和最后证据时间。

### T3.3 设备事务与多 slot（`DEP-003`）

- [ ] 建立 durable update journal。
- [ ] 使用 active、staged、previous-known-good slot。
- [ ] 每个状态迁移先落 journal，再执行副作用。
- [ ] 对每个中断点实现重启恢复和磁盘空间拒绝策略。

### T3.4 激活健康门禁（`DEP-004`）

- [ ] Web Bundle 通过页面 bootstrap、digest 和真实 DOM/API smoke 确认。
- [ ] Go Binary 通过 Kotlin supervisor、process instance、digest、版本和健康探针确认。
- [ ] Preview Assets 使用真实入口加载检查。
- [ ] 失败自动回 known-good，并生成完整 deployment events。

### T3.5 Rollout 与回滚（`DEP-005`）

- [ ] 实现 manual-one、canary、percentage、all 策略。
- [ ] 实现并发上限、观察窗、失败阈值、自动暂停、继续和终止。
- [ ] 回滚目标使用明确 ReleaseId，并复用安装状态机。
- [ ] 多设备模拟器 + 至少一台真机 canary 验收。

## Phase 4：Subagent 与 Team Runtime

### T4.1 Subagent provider（`TEAM-001`）

- [ ] 定义 provider 能力声明、one-shot run 和 continuable create spec。
- [ ] 不支持的能力在启动前显式拒绝。
- [ ] 持久 descriptor 记录 provider、上下文模式、模型、persona 和工具限制。

### T4.2 Activation 与冷恢复（`TEAM-001`）

- [ ] 稳定 Session 与进程内 Activation 分离。
- [ ] 实现 direct parent 鉴权、单 activation、child-first teardown。
- [ ] interrupt 保留未领取 inbox；重新投递可冷恢复。

### T4.3 Team 持久协作（`TEAM-002`、`TEAM-003`）

- [ ] Lead Session 持久化 roster、mailbox 和 task events。
- [ ] mailbox 在目标 inbox 接纳后确认 delivered。
- [ ] task 使用 CAS revision、DAG cycle check 和 tombstone。
- [ ] UI 只读取 Team projection。

### T4.4 Workspace 写租约（`TEAM-004`）

- [ ] task 声明规范化 read/write scopes。
- [ ] Team Runtime 分配有限期写租约并检测重叠。
- [ ] Tool Pipeline 在每次文件修改前强制校验租约。
- [ ] Agent 崩溃、取消和任务完成后可靠释放租约。

## Phase 5：迁移和删除

### T5.1 数据迁移（`MIG-001`）

- [ ] 为旧 Agent JSONL、浏览器 session 缓存和 Bundle report 定义一次性导入器。
- [ ] 不可迁移数据给出可操作错误，不静默丢弃。
- [ ] 发布格式版本与迁移证据固定到代码和文档。

### T5.2 删除重复实现（`MIG-002`）

- [ ] 删除旧全局 Agent session map 与 legacy EventCache。
- [ ] 删除未被生产使用的独立 runtime、reducer 和 Provider bridge。
- [ ] 删除生产 Mock API、剧本选择 UI 和无 Consumer 的预留接口。
- [ ] 更新旧规格、README、架构文档和运行手册。

## 依赖顺序

```text
T0.1 ─┬─> T1.1 ─> T1.2 ─> T1.3 ─> T1.4 ─> T1.5 ─> T1.6
      ├─> T0.2 ─> T0.3 ───────────────────────┐
      └─> T0.4                                │
                                               ├─> T2.x ─┐
                                               ├─> T3.x ─┼─> T5.x
                                               └─> T4.x ─┘
```

Phase 2 与 Phase 3 可在 Phase 1 的事件、身份、命令和鉴权契约稳定后并行；Phase 4 必须等待 Agent Session、inbox 与 Tool Pipeline 成为唯一生产真源。
