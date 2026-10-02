# Tasks

> 每完成一项 → 勾选本文件 + 同步 `checklist.md` + 在 `progress.md` 追加本轮记录。
> 门禁：Go = `bash scripts/test-go.sh`；前端 = `node scripts/check-all.mjs`（app 目录，可走 `app_check_all` MCP）；i18n = `python3 scripts/i18n-tool.py`（新增文案 zh-CN + en 齐全）。
> ⚠️ **拓扑前提**：桌面端跑在 cnb（公网 HTTPS、与 Go 后端同源），安卓端在 NAT 后无公网地址 → **LAN 直连已作废**，一切跨端走「Hub 中继 + 手机出站长连接」（见 spec §1）。

## P0 — 侦察与契约（本轮）

- [x] SubTask 0.1: 勘查现状并落地 spec/tasks/checklist/progress 四件套
- [x] SubTask 0.2: 核实初版事实（后端绑 0.0.0.0、CORS allowlist、ApiProxy 绝对 URL、搜索端点、agent 决策集、设备指纹、形态档）
- [x] SubTask 0.3: 用户拍板 A/B/C（CORS=经本机后端代理、扫码=MLKit 插件、QR=成熟依赖）
- [x] SubTask 0.4: **拓扑纠正**：确认桌面端在 cnb 公网（非 LAN）→ 重写连通性架构 + 补风险矩阵 R1–R16
- [x] SubTask 0.5: 与用户敲定 **E1–E5**（结论见 spec §8）：E1/E5=无固定域名、按 30m 回收最坏情况设计（Hub 地址可重指向 + 降级明示）；E2=长连接放 Go 侧；E3=本期不做 P2P；E4=取回默认不经 Hub（仅在线打开/缩略图）

## P1 — 桌面端（web）形态

- [x] Task 1.1: 形态探测 `formFactor=desktop`（新增 `shared-components/composables/useFormFactor.ts`，`data-form-factor` 写 `documentElement`；无既有占用，simverse 的 `landscape` 值被显式尊重不覆盖）
  - [x] 1.1.1 判定：`!isNative`（经 appCapabilities DI）&& `(pointer:fine)` && 宽度 ≥1024（`computeFormFactor` 纯函数）
  - [x] 1.1.2 单测：13 用例（纯函数 9 + DOM 4），入 `FAST_INCLUDE`
- [x] Task 1.2: 桌面布局 **第一阶段**（侧边导航 rail 224px 替代底部 Tab；`Tabs.vue` 桌面壳改用 `ion-router-outlet` 直载——`ion-tabs` 是 shadow DOM 内部布局不可达）
  - [x] 1.2.1 移动端 phone/pad **零回归**（390×844 实测：底部栏在位 y=787 h=57、无 rail）
  - [~] 1.2.2 内容区 max-width / master-detail 双栏 / ≥1440 三栏
    - [x] **max-width**：桌面壳 `--desktop-content-max: 1360px` + 居中（真机级验证：1920×1080 由 1696 → **1360 居中** x=392；1440×900 仍 1216，零回归）
    - [ ] master-detail 双栏（需按页改造：Files 列表 + 详情）
    - [ ] ≥1440 三栏
- [x] Task 1.3: 桌面交互（`/` 聚焦搜索、Esc 关浮层）—— `src/composables/useDesktopShortcuts.ts`
  - [x] `/` 聚焦**当前激活页**搜索框（`[data-testid="search-input"]`）；已在文本输入位 / 有浮层打开 / 带修饰键时不劫持
  - [x] Esc 关闭最上层 Ionic 浮层（alert → action-sheet → loading → picker → popover → modal，`overlay-hidden` 跳过）
  - [x] 仅桌面形态生效（`isDesktop` 可注入），手机端零行为变化
  - [x] 11 例单测（FAST）+ `pw-desktop-shortcuts.mjs` 真实浏览器 **7/7 PASS**
- [x] Task 1.4: **R16 处置** —— `getApiBaseUrl()` 生产态默认由 `http://127.0.0.1:2025`（隐含"用户本机跑后端"）改为 **`window.location.origin`（同源）**；非 http 协议（capacitor://）保留原绝对地址。契约测试 `src/api/__tests__/getApiBaseUrl.test.ts` 已改为锁新契约（web→同源 / 非 http→:2025），8/8 通过
  - **先红**：服务器托管源站（:8124 代理 /api）下把 localStorage 写回旧默认 `127.0.0.1:2025` → `GET /api/service-guard` **ERR_ABORTED**（跨源 + `gin_app.go` CORS allowlist 只放行 localhost/127.0.0.1）
  - **后绿**：同源默认后，同一源站 `/api/service-guard` **200**、`requestfailed` 为 0
  - ⚠️ **环境坑（已记录）**：本沙箱 `NODE_ENV=development`，**`vite build` 打出来的包 `import.meta.env.DEV` 仍为 true**（实测请求打到 `127.0.0.1:16666`）⇒ 验证生产行为必须 `NODE_ENV=production vite build`
- [x] Task 1.5: 门禁：`vue-tsc --noEmit` 0 错 + fast 单测 617 全绿 + Biome 0 问题；i18n 无新增 key（rail 复用 `tabs.*`）
- [x] Task 1.6: **真实浏览器复现验证**
- [x] Task 1.7: **补做 Capacitor 桌面端调研 + platform 维度修复**（官方无桌面平台；`computeFormFactor` 加 `platform`（web/electron 才是桌面候选）+ `appCapabilities.platform()` 注入 + 4 个新单测）。详见 spec §0.1
- [x] Task 1.8: **待决 E6 已拍板 = A（web）** —— 用户明确"linux 端主要是服务器，有 web 就行，不要求窗口" ⇒ 桌面端交付形态 = 浏览器，**桌面壳（Electron/Wails）本期不做**（见 spec §0.2 结论）
- [ ] Task 1.9（**选 C 的前置门禁，禁止跳过**）：目标桌面 OS 上用系统 WebKit 真实渲染本应用，验证 Tailwind v4 / daisyUI v5 的 `color-mix()` / OKLCH / `:has()` / cascade layers 生效（真实渲染 + `getComputedStyle` 断言）
  - ⚠️ **2026-10-02 实测：沙箱内被阻断** —— 容器禁止非特权 user namespace（`unshare --user` EPERM），WebKitGTK 2.52 的 WebProcess 必须经 bubblewrap ⇒ 任何 WebKitGTK 应用（含 Wails）在沙箱渲染不出来。**需真机 / 允许 userns 的环境**；探针代码已备好（`/tmp/wp/probe`，含"进程内 Go 服务 + 窗口加载 http://127.0.0.1:8899"形态 + CSS 能力上报页）。
  - ❌ **因 E6=A（桌面端只要 web）本机不再需要**：保留条目以免下任重复调研；只有将来要做 **Windows/macOS 原生壳** 时才重启（届时仍需真机）。
- [ ] Task 1.11: 调研 Wails v3 的 `ios`/`android` 子命令能力（同一份 Go + 前端能否编译到移动端）——若成立会影响"安卓端=Capacitor"的前提，需单独立项评估
  - ❌ **本期不做**（E6=A 后与桌面壳绑定，属"将来再说"）；仅作知识留存。
- [ ] ~~Task 1.10（选 B 的前置）：确认 `@capawesome/capacitor-electron` 许可条款 + 自研插件（GoProcess / ApiProxy / MLKit）桌面实现方案~~
  - ❌ **因 E6=A 作废**（不删除条目以防下任误判为"漏做"）；仅当将来要做 Windows/macOS 原生壳时重启。（本地 Chromium 1440×900：先红 `test-visual/desktop-before.png` 手机布局铺满 → 后绿 `desktop-after.png` rail 生效 + `mobile-after.png` 移动端不变；复现脚本 `pw-desktop-repro.mjs`）

## P2a — Hub 与会合（cnb 侧 + 手机侧长连接）

- [x] Task 2.1: Go `internal/peerlink/` 骨架：hub / peer registry / 内存 token store（**全部仅内存**）
  - [x] 2.1.1 `POST /api/peerlink/ticket`（桌面端申请票据：pairingId + psk + hub，120s、一次性；不传 hub 时按请求自动拼同源地址）
  - [x] 2.1.2 WSS `/api/peerlink/ws`：长连接端点（手机端**主动出网**方向，R1/R2）；帧协议 `hello_ok` / `ping`→`pong`
  - [x] 2.1.3 `POST /api/peerlink/pair`：HMAC proof 校验（票据取出即销毁）+ SAS 6 位（R5）
  - [x] 2.1.4 AEAD：`DeriveKeys`=HKDF-SHA256(psk) 双向密钥 + `Seal/Open`(AES-GCM)；Hub 侧只见密文（R5/R14）
  - [x] 2.1.5 `POST /api/peerlink/ping` 心跳 + `POST /api/peerlink/unpair`（token 立即作废）+ `GET /api/peerlink/peers`（脱敏，无密钥）+ `GET /api/peerlink/hello`（未鉴权探活）
  - [x] 2.1.6 单测：peerlink 包 9 例 + server 包 HTTP/WS 4 例（含真实 WS 建连 ping/pong、无 token 拒绝升级、重放 401、错 proof 401、解配后失效）
- [x] Task 2.2（后端部分）: **E2 决策落地** —— 新增 `internal/peerlink/edge.go`：Edge 客户端（长连接放 **Go 侧**），`https→wss` 自动转换、**主动出网**拨号、读帧回调、心跳前台 15s/后台 60s 自适应（`SetBackground`）、断线**指数退避 + 抖动**重连、`SendJSON`、`Close`
  - [x] 单测 5 例：建连收 `hello_ok` + 心跳、`ping→pong` 计数、断线重连（服务端主动断开后连接数 ≥2）、后台心跳频率低于前台、Close 后停止心跳
  - [x] **保活载体已确认**：`EncvGoService.kt` 已是前台服务（`startForeground(NOTIFICATION_ID, ...)`），Go 进程由它承载 ⇒ **无需新增 Kotlin 保活代码**（R7）
  - [x] **Go 侧接线（本次完成）**：`internal/server/peerlink_edge_runtime.go` —— `POST /api/peerlink/edge/pair`（拿票据去远端 Hub 配对，proof=HMAC）、`GET /edge/status`、`POST /edge/stop`；配对成功即启动 `peerlink.NewEdge` 常驻（重复配对先停旧连接）
    - [x] 默认处理器接真实实现：`OnSearch`= 本端 **FTS5 全文索引**（与 `/api/files/search-fulltext` 同源，limit≤200）、`OnRead`= `resolveUserPath` 后分片读（`MaxReadChunk` 上限，错误文本不含真实路径）、`OnAgentInvoke`= `PeerAgentInvokeHandler`（执行端授权）
    - [x] **R3**：`validateHubURL` 拒绝明文 `http://` 跨端地址（仅 https 或本机回环）
    - [x] token **只存内存**（进程重启需重新扫码配对，与 Task 5.2 一致）
    - [x] **两台进程端到端**（`peerlink_edge_runtime_test.go`，真实 WS）：A=Hub 出票 → B 拿票配对并启动 Edge → A 侧 peer online → A 联邦搜索 B（`/sdcard/...` 原样 + `Pixel` 标注）→ A 远端读 B（字节 + `X-Peer-Id`）→ A 远程 invoke B（ok/accept + 结果回传）→ status/stop
  - [x] **扫码 UI 调 `/edge/pair`（Task 2.6 UI 接线，2026-10-03 完成）**：`components/PeerScanPanel.vue`（扫码 + **「粘贴配对码」降级路径**，与扫码共用同一条 `connectWithText`）+ `src/peerlink/barcodeScanner.ts`（MLKit 经 `registerPlugin("BarcodeScanner")`，web 构建不静态依赖插件包；失败必须可见）+ `parsePairingQR`/`pairAsEdge`/`fetchEdgeStatus`（`usePeerLink`）
    - [x] **真实浏览器端到端**（`pw-peer-scan.mjs`，生产包 + 类网关源站 + 真实 Go 后端）：web 无相机提示可见 → 点扫码错误可见（非静默）→ **非法配对码可见报错（负向对照）** → 粘贴真实票据 → `/edge/pair` 200 → `/edge/status` running+connected（真 WS）→ **Hub 侧 peers 显示该 Edge 在线** → psk 不落盘（存储搜不到）→ 无 JS 错误。**9 断言全绿**
    - [ ] 真机相机扫码（MLKit + 权限）+ 息屏保活实测 ← P6 必验（沙箱无相机）
- [x] Task 2.3: 前端 `usePeerLink`（shared 抽象 + 应用注入）：`createPairingTicket` / `fetchPairingStatus` / `waitForPairing` / `fetchPeers` / `unpairPeer` + 响应式状态（`status`/`ticket`/`paired`/`peers`/`onlinePeers`）
  - [x] 边界守死：peer 不是 baseUrl 候选，全部走同源 `/api/peerlink/*`；psk 不落盘（单测断言 localStorage 无新增）
  - [x] 后端配套：`PairingStatus(pairingID)` 按**二维码里的秘密**查询配对结果（桌面端拿不到 peer token，故按 pairingID 查，不泄密）；新增 `GET /api/peerlink/pairing/status`；`peers`/`unpair` 支持 `X-Peerlink-Operator` 运维身份（**P5/R12 TODO：运维侧需真鉴权**）
  - [x] 单测 11 例入 `FAST_INCLUDE`，fast **632/632** 全绿；typecheck 0；Biome 0
  - [x] 显式 reset 钩子 `__resetPeerLinkStateForTests()`（isolate:false 下禁止 `vi.resetModules()`）
- [x] Task 2.4（进程内端到端部分）: 新增 `internal/server/peerlink_e2e_test.go` —— **真实 WebSocket 全链路**：票据 → 配对（带 proof）→ 配对状态可查 → **Edge 主动出网建连** → 心跳后 Hub 显示 `online=true` → 解配后 peers 清空
  - [x] 真二进制冒烟：`go build ./cmd/encv` 后实跑，`hello` 200、`ticket` 200（hub 自动同源、expiresIn=119）、未知 pairingId → 404、无身份 `peers` → **401**（红线成立）
  - [ ] 真机/模拟器 + `adb forward` 的移动端端到端（含扫码、息屏保活、IPv6-only）← 沙箱内无法覆盖
- [x] Task 2.5: 桌面端配对面板 + SAS 核对 —— `components/PeerPairingPanel.vue`（生成票据 → **真实二维码**（`qrcode@1.5.4`，已入 package.json）→ 120s 倒计时 → 轮询配对 → **SAS 6 位大字核对** → 信任/取消）+ `views/PeerSettings.vue`（设备列表 + 解配）+ 路由 `settings/peers` + Settings 入口 + i18n（peers.* zh/en 各 20 key）
  - [x] **真实浏览器端到端**（`pw-peerlink-e2e.mjs`，生产包 + 类网关源站 + 真后端）：出码（canvas 220×220 有真实深浅模块）→ Node 模拟扫码配对 200 → 页面 SAS 与**本地独立计算**一致 → 点信任 → 已信任态
  - [x] **抓出并修掉真 bug**：`import(/* @vite-ignore */ "qrcode")` 的 `@vite-ignore` 阻止 Vite 打包裸说明符 ⇒ 运行时解析失败且被我最初的空 catch 吞掉（表现为"画布全透明"）。修复=去掉 `@vite-ignore` + **渲染失败必须可见**（`qrDiag` 显示到 UI，禁止吞错）
  - [x] 安卓端扫码（MLKit）与相机权限 ← 仍需真机（checklist）

## P2b — 配对 UI（二维码 ↔ 扫码）

> ⚠️ 本节与 P2a/P2b 各条有重叠：Task 2.5/2.7/2.8 已在 Iteration 6（见 progress.md）随 P2a 完成；
> Task 2.6 扫码端 UI 于 2026-10-03（Iteration 18）完成。保留原条目作对照。

- [x] Task 2.5: 桌面端渲染二维码（`qrcode` 依赖），内容 = `hub + pairingId + psk + exp`（**不含内网地址**，R3）——Iteration 6 完成
- [x] Task 2.6: 安卓端扫码 UI（`@capacitor-mlkit/barcode-scanning@8.2.1`，`registerPlugin` 引入）+ **「粘贴配对码」降级**（Iteration 18）；
      WebView 相机权限**真机实测仍待 P6**
- [x] Task 2.7: SAS 6 位核对 UI（双端各显示，人工确认）——Iteration 6 完成
- [x] Task 2.8: 设备与配对页 `PeerSettings.vue`（已配对列表、在线状态、解配）——Iteration 6 完成
- [x] Task 2.9: 门禁（check-all 9 PASS / 0 FAIL）+ i18n（peers.scan*/paste*/code*/edge* zh+en 齐）；
      **真机扫码实测**（沙箱无相机 → 列入 P6 必验项）

## P2c — 风险收口（R1–R11）

- [ ] Task 2.10: **R3 回归锁**：配置/二维码中禁止 `http://` 对端地址（单测）
- [ ] Task 2.11: **R4**：确认 `gin_app.go` allowlist 未为 peer 放开；桌面只同源
- [ ] Task 2.12: **R6**：Hub 地址持久化 + 可重指向；固定域名优先（依赖 E1/E5）
- [ ] Task 2.13: **R8**：心跳自适应 + 移动网提示
- [ ] Task 2.14: **R10**：安全判定只用 nonce/单调计数（代码审查 + 单测）
- [ ] Task 2.15: **R11**：pairingId 一次性、解配即作废、Hub session 清理、单 peer 并发上限
  - [x] 单 peer **在途 RPC 上限** `MaxConcurrentCallsPerPeer = 4`，超限 → **429 `peer_busy`**（背压）
  - [x] 背压**不计入**熔断失败累计（判定顺序：先 429，再熔断记录）——否则一次限流会自己熔断健康对端
  - [x] 单 peer 只允许**一条活跃会话**：`SetExclusive` 顶掉旧连接 + `DeleteConn` 按连接身份删除（防旧连接 defer 误删新连接）
  - [x] 槽位必归还（突发后在途数归零、并发退去后恢复 200）
  - [x] **真 bug**：Edge 并发回写 websocket panic ⇒ `writeMu` 串行化所有写帧（res / 心跳 / SendJSON）；Hub 侧 hello_ok/pong/error 同样改走 `peerConns.WriteJSON`
  - [ ] Hub session 清理（断线/重连后的残留会话清理）与 pairingId 逐出策略 —— 未做

## P3 — 互通搜索索引（联邦）

- [x] Task 3.1（后端）: `GET /api/peerlink/search?peerId=&q=&kind=&limit=` —— 经 Edge 的**主动出网连接**反向查询对端本地索引，结果带 `peer{id,name,platform}` 标注；依赖新增 `internal/peerlink/conn.go`（活跃连接表 + 串行写）+ `rpc.go`（call-id 关联 + 2s 超时）+ `Edge.OnSearch` 注入点
  - [x] 降级语义明确：对端离线 → **503 peer_offline**；超时 → **504 peer_timeout**；未知 peer → 404；缺参 → 400（前端据此降级为"该端无结果"，不阻塞本端搜索）
  - [x] 单测/e2e 3 例：`Federated_OK`（远端路径原样返回、带 android 平台标注）、`PeerOffline_503`、`UnknownPeer_404_And_MissingParams_400`
  - [x] **结构性修复**：路由注册收敛为 `registerPeerlinkRoutes()`（生产与测试共用），消掉"路由表两套"的漂移（已三次导致假红）
- [x] Task 3.2: 前端 `useFederatedSearch`（`searchFederated`）：本端 + 各在线 peer **并发**查询，单端 **2s 超时**（AbortController），失败只降级不抛错
  - [x] 注入点：`setFederatedSearchProviders({fetch, peerlinkBase, localSearch})` + `__reset...ForTests()`（isolate:false 下禁 `vi.resetModules()`）
- [x] Task 3.3（数据层）: 合并去重 key = `peerId#path`（同 peer 内去重，**不同 peer 的同路径保留为两份跨端引用**）；每条命中带 `source`(local/peer) + `peerId/peerName` + 原始远端路径；`peerStatuses` 记录 ok/offline/timeout/error
  - [x] 单测 6 例入 `FAST_INCLUDE`，fast **638/638** 全绿；typecheck 0；Biome 0
  - [x] 顺带加固：唯一 wall-clock 断言（配对超时）从 200ms 放宽到 1000ms —— 实测只在"改文件后首次运行"（Vite transform 冷启动）偶发假红
  - [x] 来源徽章 UI：`components/PeerSourceBadge.vue`（本机=中性 / 安卓·<设备名>=主色）+ `PeerSettings.vue` 互联搜索区（输入 + 结果列表 + 对端状态 chip「在线/离线/超时」）+ i18n 补 key
  - [x] **真实浏览器验证**（`pw-federated-ui.mjs`，生产包）：本机命中灰徽章、远端命中紫徽章「安卓·Pixel」、远端路径**原样展示**、状态 chip「Pixel：在线」；截图 `test-visual/federated-ui2.png`。期间 i18n 缺 key（`[MISSING: PEERS.SEARCH]`）被截图当场抓出并补齐
  - [x] Files.vue 主搜索页接入（复用 PeerSourceBadge）：`useFilesView` 可选注入 `peerSearch` → **`peerHits` 独立容器**（不混进 `searchResults`/`displayFiles`）；搜索页独立展示区「来自已配对设备（N）」
    - [x] 踩坑修复：展示区必须放在空态 `v-if` **之前**（否则落在 `v-else` 分支，本端 0 命中时整块不渲染）；空态条件加 `&& peerHits.length === 0`
    - [x] 真实浏览器验证（`pw-files-federated.mjs`）：徽章「安卓·Pixel」+ `/sdcard/...` 原始路径 + 「在线打开」链接恒带 `peerId`；**无任何 `ion-item` 渲染 `/sdcard/` 路径**
- [x] Task 3.4: 远端命中只能「**在线打开 / 取回**」，路径始终带来源标识
  - [x] 后端 `GET /api/peerlink/file?peerId=&path=&offset=&length=`（RPC method=`read`）：`ReadRequest/ReadResult` + `DefaultReadChunk=256KB` / `MaxReadChunk=4MB`（**R13**）+ `ReadCallTimeout=10s`
  - [x] 响应**常驻来源头** `X-Peer-Id` / `X-Peer-Name` / `X-Peer-Remote-Path`（客户端任何后续处理都能看到字节来自哪台设备）
  - [x] 降级/拒绝语义：超限 **413**（`chunk_too_large`+`maxChunk`）、离线 503、超时 504、缺参 400
  - [x] 前端：远端命中只给「在线打开」入口，链接恒为 `peerId + 远端原始 path`（本端命中不给该入口）
  - [x] **真 bug 修复**：HTTP 头是 latin-1，中文远端路径直接入头会乱码 ⇒ `headerSafe()` 非 ASCII 百分号编码 + `Content-Disposition` 走 RFC 5987 `filename*=UTF-8''`；新增回归单测 `TestPeerlinkFile_NonASCIIHeader`
  - [x] Go e2e 4 例 + 真实浏览器验证（`pw-federated-ui.mjs`：断言链接含 peerId 与原始路径、实际取到字节与来源头）
- [ ] Task 3.5: **R13**：报文上限 + 分块 + 限流；大流量默认不经 Hub
- [x] Task 3.6: 契约回归锁（三道）：① 前端单测断言远端路径**原样保留**且命中带 `peerId/peerName` 标注；② Go e2e 断言 `/sdcard/...` 原样返回只加 peer 标注；③ `ListPeers` 单测断言无密钥字段。挂载/统一命名空间**无任何代码路径**（结构性排除）
- [ ] Task 3.7: 门禁 + 双端在线实测（后端部分已绿，真机联调待安卓侧接线）

## P4 — 远程 Agent 调用与授权

- [x] Task 4.1: `POST /api/peerlink/agent/invoke` + 结果回传（RPC `agent_invoke`，`AgentCallTimeout=120s`；离线 503 / 超时 504 / 缺参 400）
- [x] Task 4.2: 执行端审批挂起 + **执行端 UI 弹窗**：`useRemoteApproval`（轮询 `/agent/pending`，2s，401 静默降级）+ `RemoteApprovalPrompt.vue`（全局挂载在 `Tabs.vue`，任何页面都能弹）
  - [x] 弹窗展示：来源设备名 / 工具名 / **破坏性警示** / **超时倒计时**（后端 90s 自动 decline）
  - [x] 一次只弹一条（其余显示 `+N` 排队）；已决策的 callId 不再重复弹
- [x] Task 4.3: 挂起 **90s** 超时 → 自动 `decline`（`DefaultApprovalTimeout`）+ 审计记 `timeout`（**绝不默认同意**，有回归单测）
- [x] Task 4.4: 决策 `trust_device`（**进程内存，重启失效**）——`Approver.trusted`，与会话级 `sess.GrantedTools` 严格区分；有"新实例信任表为空"回归锁
- [x] Task 4.5: `useRemoteApproval` + 审批弹窗（复用 ApprovalCard 的按钮语义与 `modals.*` 文案；三按钮 = 拒绝 / 允许一次 / 信任此设备）
  - [x] 真实浏览器验证（`pw-remote-approval.mjs`）：弹窗出现 + 设备/工具/破坏性/倒计时 + 点「信任此设备」→ `approve` 负载 `{callId, decision:"trust_device"}` + 决策后不再重复弹
- [x] Task 4.6: 破坏性工具即使已信任也强制确认（`IsDestructive` 依据工具注册表 `needConfirm=true`）
- [x] Task 4.7: 审计日志（**脱敏**：只记工具名/字节数/JSON 顶层键，绝不记路径全文）+ `GET /api/peerlink/agent/audit`
- [~] Task 4.8: 单测 + 门禁 —— **后端 11 例（peerlink 7 + server 4）+ 前端 4 例已绿**（fast 642/642）；真机待 P6
  - [x] `internal/peerlink/agent.go`：`Approver`（挂起/决策/信任/审计）+ 报文 + 哨兵错误
  - [x] Edge `agent_invoke` 分发：`ErrDeclined/ErrCancelled` → `decline/cancel`；其它错误**截断 200 字符**回传（防路径/参数外泄）
  - [x] 执行端唯一入口 `Server.PeerAgentInvokeHandler`（先授权后执行，`executeAgentTool`）
  - [x] 本地接口 `/agent/pending|approve|trust|audit` 均要求运维头（否则 401）
  - ⚠️ **未接线**：`OnAgentInvoke` 尚未注入到生产 Edge（Edge 守护属 P2a，Task 2.x 未完成）→ 接线时注入 `s.PeerAgentInvokeHandler`

## P5 — 安全与降级收口（R12–R14）

- [x] Task 5.1: 未配对一律 401（全 `/api/peerlink/*` 集成测试）—— `TestPeerlink_UnpairedAll401` 表驱动覆盖 14 个受保护端点；并反向锁定 4 个**有意开放**端点（hello / ticket / pair / pairing-status）不得 401（否则无法完成首次配对）
- [x] Task 5.2: psk/token/信任态**零落盘**回归锁 —— 结构性锁：扫描 `internal/peerlink` 与 server 侧 peerlink 源文件，禁止任何 `os.WriteFile/os.Create/os.OpenFile/ioutil.WriteFile` 等写盘调用；运行时锁：`Hub.ListPeers()` 输出不得含 token/pskHex
- [x] Task 5.3: **R12**：速率限制 + 失败熔断
  - [x] `internal/server/peerlink_limits.go`：滑动窗口限流（search 30/分、file 60/分、agent 10/分）+ 熔断（连续 5 次失败 → 开路冷却 30s → 半开探测）；**全部只存进程内存**
  - [x] HTTP 表现：超限 **429** `rate_limited` + `Retry-After`；开路 **503** `peer_circuit_open`（**不再消耗调用超时预算**）；成功即清零
  - [x] 单测 3 例：`RateLimit_429`、`CircuitBreaker_Open503`（含冷却后半开探测）、`RateWindow_Unit`
- [x] Task 5.4: **R14**：访问日志脱敏（不记查询词/路径全文/**token**）
  - [x] **真 bug 修复**：`gin.Logger()` 会把查询串整条写进访问日志 ⇒ `/api/peerlink/ws?token=…`（**令牌**）、`search?q=…`（查询词）、`file?path=…`（远端路径全文）全部外泄。改为 `gin.LoggerWithConfig{Formatter: sanitizedLogFormatter}`：敏感路径/凭证参数只记路径
  - [x] ⚠️ 排查坑：gin 在 `SkipQueryString=false`（默认）时**已把查询串拼进 `param.Path`** ⇒ 必须用 `param.Request.URL.Path` 重新取纯路径，否则脱敏不生效
  - [x] 回归单测 `TestPeerlink_AccessLogSanitized`：敏感路径日志不含查询词/路径/token，且**非敏感路径不被过度脱敏**（仍记查询串，保排障能力）
- [x] Task 5.5: 降级矩阵**持久性** UI（**非 Toast**）
  - [x] `usePeerDegradation`（10s 轮询 peers + edge/status）+ `PeerDegradedNotice.vue` 常驻条幅，全局挂在 `Tabs.vue`
  - [x] 覆盖：对端离线 / 未连互联服务（重连中）/ **服务重启后需重新扫码** / 限流 / 熔断；可逐条收起，状态恢复后自动消失
  - [x] 真实浏览器验证（`pw-peer-degraded.mjs`）：条幅出现且带对端名 → **12s 后仍在**（证明非 Toast）→ 状态恢复后自动消失

## P6 — 端到端验证与文档同步

- [x] Task 6.1: 中继链路 / 联邦搜索 / 远程 Agent 授权的**真机级**端到端 ——
  新增 `scripts/emu-peerlink-e2e.sh`（Hub=宿主机真实 Go 进程，Edge=**模拟器内** x86_64 真实后端，
  `adb reverse` 模拟手机主动出网）。**29 PASS / 0 FAIL**。
  过程中抓出并修掉两个真 bug（均有先红后绿 + 回归锁，见 `progress.md` Iteration 17）：
  ① `internal/server/server.go` **servingDir 初始化时序**（晚于 NewServer 里的使用 ⇒ 空串 ⇒
  **本地全文搜索永远 0 命中** + mount root 退化成 cwd）；② `peerlink.AgentInvokeOutcome`
  **执行端真实决策被吞成 accept**（新增 outcome 结构一路透传 `auto`/`trust_device`）。
- [ ] Task 6.2: **真机（R15）**：扫码配对、移动网（4G/5G）建连、后台息屏审批超时 ← 沙箱无相机/无移动网
- [~] Task 6.3: `trust_device` 重启失效验证 —— **进程级已验证真机证据**：重启模拟器内 Go 进程后
  `GET /agent/trust` 为空、老票据不可用、同一工具**再次要求审批**（≠ Application 杀进程场景，仍需真机）
- [~] Task 6.4: `MOBILE_STRATEGY.md` 已更新（交付清单 / 边界红线 / P6 待验清单）；
  `checklist.md` 本轮同步；剩余 kill App 类真机项不在文档侧
- [x] Task 6.5: 记忆固化 —— `2026-10-02.md` §14（端到端脚本 + 两个真 bug 根因/修法/回归锁
      + 沙箱四个硬前置）+ `MEMORY.md` 长期条目（闸门脚本、环境前置、两处顺序/透传缺陷）
