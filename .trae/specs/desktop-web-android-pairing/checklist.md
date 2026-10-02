# Checklist

## P0 — 契约

- [x] spec / tasks / checklist / progress 四件套已建
- [x] 现状事实已核实（同源 baseUrl、preview-gateway、后端绑定、CORS allowlist、ApiProxy 绝对 URL、搜索端点、agent 决策集、形态档）
- [x] 用户已拍板 A/B/C：CORS=经本机后端代理、扫码=`@capacitor-mlkit/barcode-scanning`、QR=成熟 `qrcode` 依赖
- [x] **拓扑纠正已写入 spec**（桌面端在 cnb 公网，非 LAN；LAN 直连作废）
- [x] 风险矩阵 R1–R16 已建立
- [x] 待决问题 **E1–E5** 已与用户敲定（E1/E5 无固定域名按最坏情况、E2 Go 侧长连接、E3 本期无 P2P、E4 取回不经 Hub）

## P1 — 桌面端（web）

- [x] `data-form-factor="desktop"` 判定生效（非原生 + pointer:fine + ≥1024；13 单测）
- [x] 侧边导航 rail 生效（桌面壳改用 `ion-router-outlet` 直载，`ion-tabs` shadow DOM 不可达已记录）
- [ ] master-detail 双栏 / 内容区 max-width（**下轮**）
- [x] 移动端 phone/pad **零回归**（390×844 真实浏览器实测，底部栏原样）
- [ ] `/` 聚焦搜索、Esc 关浮层可用
- [x] **R16**：生产态 API base 默认改为**同源**（服务器托管形态正确）；先红后绿已验证（旧默认 → ERR_ABORTED；新默认 → 200）
- [x] 契约测试 `src/api/__tests__/getApiBaseUrl.test.ts` 锁新契约（web→origin / capacitor://→:2025），8/8
- [x] 真实浏览器复现验证通过（desktop-before/after + mobile-after 三张截图 + DOM 探针）
- [x] typecheck + fast 单测（617 全绿）+ Biome 全绿；i18n 无新增 key（复用 `tabs.*`）

## P2a — Hub 与会合（后端）

- [x] `POST /api/peerlink/ticket`（120s、一次性）+ `GET /api/peerlink/hello` 就位
- [x] `POST /api/peerlink/pair`（HMAC proof）+ SAS 6 位派生
- [x] WSS `/api/peerlink/ws` 就位：有 token 建连并 `hello_ok` + `ping/pong`；无 token 拒绝升级
- [x] HMAC proof 错 → 401 `bad_proof`；票据重放 → 401 `ticket_used`；过期 → 401 `ticket_expired`
- [x] AEAD：`DeriveKeys`(HKDF) + `Seal/Open`(AES-GCM)；单测断言**密文不含明文**且**另一方向密钥解不开**
- [x] 解配后 token 立即作废（ping 转 401）
- [x] 受保护端点无 token 一律 401（peers/ping/unpair 全覆盖）
- [x] psk/token/密钥**只存进程内存**（Hub 结构体字段，无写盘路径）
- [x] 单测：peerlink 9 例 + server HTTP/WS 4 例全绿；`go build ./...` 0 错
- [x] 前端 `usePeerLink`（11 例单测，含 psk 不落盘断言）+ 配对状态轮询端点
- [x] 明确记录待办：运维侧（`X-Peerlink-Operator`）**尚无真鉴权**，P5/R12 必须补
- [x] Hub ⇄ Edge **端到端**（真实 WebSocket）：配对 → Edge 主动出网 → 心跳 → `online=true` → 解配清空
- [x] 真二进制冒烟：hello 200 / ticket 200 / 未知 pairingId 404 / 无身份 peers **401**
- [ ] 真机 + `adb forward` 的移动端端到端（扫码、息屏保活、IPv6-only）← 沙箱外
- [ ] SAS 6 位核对 UI ← Task 2.5
- [x] 心跳 15s/60s 自适应（`SetBackground`）+ 断线指数退避 + 抖动重连（Edge，5 例单测）
- [x] **R7** 保活载体已确认：`EncvGoService` 是前台服务 ⇒ Go 侧长连接无需新增 Kotlin 保活代码
- [ ] **R2** IPv6-only 建连处置（真机/移动网验证）← Task 2.2 剩余
- [ ] 息屏/后台存活真机实测 ← Task 2.2 剩余

## P2b — 配对 UI

- [x] 桌面端二维码**真实渲染**（`qrcode@1.5.4`；e2e 断言 canvas 220×220 且深浅模块都存在；120s 倒计时生效）
- [x] 桌面端 UI：票据过期刷新 / 等待扫码 / SAS 核对（大字 6 位）/ 信任与取消 / 设备列表 + 解配
- [x] i18n：`peers.*` zh-CN + en 各 20 key
- [ ] 安卓端扫码可用（MLKit 插件 + 相机权限）← 真机
- [ ] **R3**：二维码/配置中**不含任何 `http://` 对端地址**（回归锁）
- [ ] 安卓端扫码可用（MLKit 插件 + 相机权限）
- [ ] 设备页显示设备名/平台/在线状态，可解配
- [ ] i18n 双语齐全

## P2c — 风险收口（R1–R11）

- [ ] R3 回归锁通过
- [ ] R4：`gin_app.go` allowlist **未**为 peer 放开；桌面只同源
- [ ] R6：Hub 地址可持久化且可重指向（固定域名优先）
- [ ] R8：心跳自适应 + 移动网提示
- [ ] R10：安全判定不用时间戳（只用 nonce/单调计数）
- [ ] R11：pairingId 一次性、解配即作废、单 peer 并发上限

## P3 — 互通搜索索引

- [x] `/api/peerlink/search` 就位：经对端连接查询其**本地索引**，结果带 `peer` 标注（不搬运索引、不挂载）
- [x] 降级语义：离线 503 `peer_offline` / 超时 504 `peer_timeout` / 未知 peer 404 / 缺参 400（3 例 e2e）
- [x] 路由注册收敛为唯一函数（生产与测试共用），消除测试路由漂移
- [x] 前端 `useFederatedSearch`：并发 + 2s 超时降级 + 去重键 `peerId#path` + 来源标注（6 例单测）
- [ ] 来源徽章 UI（本机 / 安卓·<设备名>）与 Files 页接入 ← Task 3.3 UI
- [x] 契约锁：不产生本地路径语义 / 不引入挂载 → Task 3.6
- [x] 远端命中只有「在线打开」入口：请求恒带 `peerId + 远端原始 path`；R13 单次上限 → 413
- [x] 响应常驻来源头 `X-Peer-*`；非 ASCII 路径按 RFC 5987 / 百分号编码（有回归单测）
- [ ] 并发查询、单端超时 2s、**不阻塞**主结果
- [ ] 合并去重（key = `peerId + path`）+ 来源徽章
- [ ] 远端命中只能"在线打开 / 取回"，**不产生本地路径语义**（契约锁）
- [ ] ❌ 未引入挂载 / 统一命名空间（负向检查）
- [ ] **R13**：报文上限 + 分块 + 限流；大流量默认不经 Hub
- [ ] 双端在线实测通过

## P4 — 远程 Agent 与授权

- [ ] 远程 invoke 成功
- [ ] 敏感操作默认在**执行端**弹窗（不是调用端）
- [ ] 后台/息屏挂起 90s → 自动 decline（**绝不默认同意**）
- [ ] `trust_device` = 进程级，**重启即失效**（真机验证）
- [ ] 与 `accept_for_session` 语义不混淆
- [ ] 破坏性工具即使已信任也强制确认
- [ ] 审计日志脱敏落盘 + UI 可查
- [ ] 审批弹窗复用 ApprovalCard + `ui-*` 表面类

## P5 — 安全与降级

- [ ] 全 `/api/peerlink/*` 未配对 401 集成测试通过
- [ ] psk/token/信任态零落盘回归锁通过
- [ ] R12：速率限制 + 熔断生效
- [ ] R14：Hub 日志脱敏（不记查询词/路径全文）
- [ ] 降级矩阵 5 场景有**持久性 UI 状态**（非 Toast）

## P6 — 端到端与文档

- [ ] 模拟器：中继 / 联邦搜索 / 远程授权 全链路通过
- [ ] **真机（R15）**：扫码配对、4G/5G 建连、息屏审批超时
- [ ] `trust_device` 重启失效真机验证通过
- [ ] `MOBILE_STRATEGY.md` 已更新（拓扑入册）
- [ ] 当日 memory + `MEMORY.md` 已固化

## Notes（沙箱无法覆盖，必须真机）

- 相机扫码权限与识别率、移动网（CGNAT/IPv6-only）真实建连、息屏保活与电量表现
