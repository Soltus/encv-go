# 协同工作区 vNext 验收清单

> 对应规格：[`spec.md`](spec.md)；实施任务：[`tasks.md`](tasks.md)
>
> 状态：全部未实施。每个 `[x]` 后必须附 `(evidence: <EvidenceRecord.evidenceId>)`；没有证据编号不得勾选。

## 0. 证据纪律

- [ ] `EVD-001` EvidenceRecord schema 已实现并有版本号。
- [ ] 每条证据记录 `workspaceCommit`、`artifactDigest`、`runtimeTargetId` 和 `producerInstanceId`。
- [ ] 每条证据标明 `real`、`replay` 或 `synthetic`。
- [ ] Mock/Replay 证据无法满足真实网络、真实设备、真实副作用和生产发布要求。
- [ ] UI 验收记录 exact origin、surface id、generation 和加载的 bundle digest。
- [ ] 所有 bug 回归先红后绿，红绿输出均进入证据记录。

## 1. 安全与 PeerLink

- [ ] `SEC-001` 未登录请求即使带 `X-Peerlink-Operator: 1` 也无法列设备、远程调用、下发或回滚。
- [ ] `SEC-001` Operator token 过期、撤销和 scope 不足均 fail closed。
- [ ] `SEC-002` HTTP URL、代理日志、应用日志和审计记录中没有 peer token/credential。
- [ ] `LINK-001` v2 frame 完成版本和 capability negotiation。
- [ ] `LINK-001` frame 使用方向密钥 AEAD，并绑定连接 generation、sequence、method 和 command id。
- [ ] `LINK-001` 重放、篡改、乱序和跨连接 generation 的 frame 被拒绝。
- [ ] 旧 PeerLink 客户端在迁移窗口得到明确兼容/升级结果，不静默降级。

## 2. 单一 Agent Runtime

- [ ] `CORE-001` `/api/chat`、`/api/confirm`、`/api/resume`、CLI 与 PeerLink 共用同一 Runtime。
- [ ] `CORE-001` `/workspace/agent` 与 `internal/server/agent_*` 不再同时持有状态机。
- [ ] `CORE-002` 每个 capability 有 Definition、Provider、Consumer 和 disposer。
- [ ] `CORE-002` 插件卸载先关闭准入，再等待在途工作，最后撤销注册。
- [ ] 启动 inventory 能准确列出当前 Provider、版本和能力；缺失能力明确失败。

## 3. 事件、恢复与工具

- [ ] `EVT-001` 所有模型可见输入均可从 Session ledger 重建。
- [ ] `EVT-001` writer version、schema 和相邻 migration 都有契约测试。
- [ ] `EVT-001` 未来版本被拒绝；旧 generation 不覆盖、不改名。
- [ ] `EVT-002` 首次 follow 返回 snapshot + head sequence。
- [ ] `EVT-002` 重连按 cursor 返回有序增量，无重复、无静默缺口。
- [ ] `EVT-002` 超出保留窗口时返回 gap 并强制重新取 snapshot。
- [ ] `AGT-001` start、queue、steer 全部进入同一 inbox。
- [ ] `AGT-001` 进程在模型流、审批和工具执行期间崩溃后，恢复结果均明确且不伪装成功。
- [ ] `AGT-002` 工具严格经过 validate、policy、hooks、guards、approval、execute、post、result、append。
- [ ] `AGT-002` 重复 CommandId 不产生重复副作用。
- [ ] `AGT-003` legacy 与 AG-UI 对同一日志产生等价 projection。
- [ ] `AGT-004` production composition 无法加载 Mock；synthetic 标识不可移除。

## 4. 远程调试

- [ ] `DBG-001` Go、Android、WebView/Web surface target 都有稳定身份。
- [ ] `DBG-001` 页面刷新保留 logical surface id 并增加 generation；进程重启更换 process instance。
- [ ] `DBG-002` Debug Session 支持创建、过期、撤销、断线重连和 cursor 续传。
- [ ] `DBG-002` 高风险能力必须设备端明确批准；超时和无人应答均拒绝。
- [ ] `DBG-003` 日志使用稳定 source cursor，不再依赖 `HH:MM:SS` 排序。
- [ ] `DBG-003` Console、Network、异常、运行时状态和加载 digest 均来自真实 target。
- [ ] `DBG-003` source 溢出产生 gap，客户端不会把缺口显示为完整数据。
- [ ] `DBG-004` 验收脚本能拒绝错误端口、替代进程和错误 bundle。
- [ ] `DBG-004` 当前用户页面的真实 DOM/样式/交互通过同一 target 验证。

## 5. 云控热更新

- [ ] `DEP-001` Artifact 内容寻址且不可变，Release manifest 有可信签名。
- [ ] `DEP-001` 未知/撤销 key、坏签名、摘要不符、ABI/版本不兼容都在 staging 前拒绝。
- [ ] `DEP-001` channel pointer 只在全部制品上传和资格验证后原子发布。
- [ ] `DEP-002` Deployment 和 DeviceDeployment 可跨 Hub 重启恢复。
- [ ] `DEP-002` 离线设备上线后自动收敛 desired/observed state。
- [ ] `DEP-002` RPC timeout 不被记录成安装失败或成功，后续能查询同一 command。
- [ ] `DEP-003` active、staged、previous-known-good slot 与 journal 可跨设备进程重启恢复。
- [ ] `DEP-003` 在每个 journal phase 强制中断后，系统均恢复到确定状态。
- [ ] `DEP-004` Web Bundle 未上报目标 digest 与真实页面 smoke 前不得 committed。
- [ ] `DEP-004` Go Binary 未启动目标 process instance 并通过健康检查前不得 committed。
- [ ] `DEP-004` 激活失败自动回 known-good，且云端 observed state 最终一致。
- [ ] `DEP-005` canary 选择对同一 DeviceId 稳定。
- [ ] `DEP-005` 达到失败阈值自动暂停，未开始设备不受影响。
- [ ] 明确 ReleaseId 的人工回滚与自动回滚经过同一状态机和健康检查。
- [ ] 至少一次真实公网 WSS + Android 真机完成 Web/Go 更新闭环。

## 6. Subagent 与 Team

- [ ] `TEAM-001` one-shot 与 continuable provider 能力在启动前校验，不支持时明确拒绝。
- [ ] `TEAM-001` child Session 身份跨 Activation 与进程重启保持稳定。
- [ ] `TEAM-001` interrupt 只停止当前 Activation，未领取 inbox 保留并可恢复。
- [ ] `TEAM-002` mailbox 在 queue、accept、ack 三个故障点均不丢、不重复模型可见消息。
- [ ] `TEAM-002` 消息保留 sender identity，越权 parent/sibling/ancestor 被拒绝。
- [ ] `TEAM-003` task 更新要求 expected revision，陈旧 revision 被拒绝。
- [ ] `TEAM-003` blockedBy 环被拒绝，删除保留 tombstone。
- [ ] `TEAM-004` 重叠 write scope 无法同时取得写租约。
- [ ] `TEAM-004` 文件修改工具在执行前校验租约，而不是只显示冲突警告。
- [ ] Team UI 可从空前端状态完全由服务端 projection 恢复。

## 7. 迁移与删除

- [ ] `MIG-001` 每种旧持久数据都有导入测试或明确的不可兼容提示。
- [ ] `MIG-001` 迁移失败不改写源数据，重试幂等。
- [ ] `MIG-002` 旧全局 session map、旧 EventCache 和重复 runtime 已删除。
- [ ] `MIG-002` 未接线 reducer、生产 Mock 控制面和无 Consumer API 已删除。
- [ ] 旧规格和 README 均链接到当前规格，且没有互相矛盾的“当前状态”。

## 8. 最终门禁

- [ ] 全量静态检查、类型检查和测试通过。
- [ ] Agent、Debug、Deployment、Team 四类专项故障注入通过。
- [ ] 浏览器真实渲染验收通过。
- [ ] Android 真机冷启动、断网重连、更新与自动回滚通过。
- [ ] 安全评审确认没有 header 自授权、URL token、明文 PeerLink 业务帧和未审计高风险命令。
- [ ] 发布说明列出迁移、回退和数据兼容策略。
