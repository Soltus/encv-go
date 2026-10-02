# 桌面端（Web，cnb 公网）+ 安卓端扫码互联 Spec

> slug: `desktop-web-android-pairing`
> 状态：**P0 规划（未动代码）**｜跟踪入口：本目录 `progress.md`
> 关联：`MOBILE_STRATEGY.md`、`docs/shared-components-boundary-spec.md`、`app/preview-gateway/README.md`、`.cnb.yml`

---

## 0. 部署拓扑（决定性事实，2026-10-02 用户纠正后核实）

> ⚠️ **推翻初版假设**：初版按"两端同局域网 → LAN 直连"设计。**事实是桌面端跑在 cnb 云开发环境，与安卓端不在同一网络**，LAN 直连方案整体作废。

| 端 | 运行位置 | 后端 | 对外可达性 |
|---|---|---|---|
| **桌面端（web）** | 用户浏览器 → **cnb 云开发环境（公网 HTTPS）** | 同源：preview-gateway `:16666` 把 `/agent-api` 转发到容器内 encv-go `:2025` | 有公网 HTTPS 入口 |
| **安卓端** | 真机 / 模拟器 App（WebView `https://localhost`） | 设备内 encv-go `127.0.0.1:2025` | **在 NAT/CGNAT 后，无公网可达地址** |

已核实的代码事实：
- `useAgentApiBase.ts:27-29`：web(dev/cnb) → 同源 `/agent-api`；APK → `http://127.0.0.1:2025`。**桌面页与其后端同源**。
- `preview-gateway` 是单端口对外入口（`/` → vite `:8100`，`/api`、`/agent-api` → encv-go `:2025`）。
- `internal/register/server_start.go:22` `net.Listen("tcp", ":port")` → 容器内绑 0.0.0.0，但**只在容器内可达**，不是用户 LAN。
- `.cnb.yml:27` `keepAliveTimeout: 30m` → **cnb 开发环境 30 分钟无心跳会被回收**，hub 地址/可用性会漂移（重大风险，见 §5 R6）。

**推论（本 spec 的全部设计都建立在这三条上）**：
1. 桌面端**永远只跟同源的 cnb 后端说话**（REST/SSE）→ 天然无 CORS、无混合内容（https 页面请求 http 被浏览器禁）。
2. 只有**手机端在 NAT 后**；hub（cnb）有公网 WSS。**手机主动出网建长连接**即可 100% 打通，无需打洞。
3. 二维码的价值从"传内网地址"变成"**传会合点 + psk**"（带外通道），这反而更稳——不依赖两端同网。

---

## Why

ENCV-go 目前是"单机自包含"形态（安卓 = WebView + 本机 Go；桌面 = cnb 上的 web + 容器 Go），两端之间**没有任何通路**。用户要三件事：

1. **正式桌面端（web）形态**（不是 dev 预览），且能选"数据/能力从哪来"（cnb 后端 or 已配对的安卓端）。
2. **安卓扫码连接**：扫一下就互认，不手敲地址。
3. **在线时双端互通搜索索引 + Agent 可远程调用对端调试**：
   - 互通索引 **≠ 挂载网络驱动器**：远端结果永远是**跨端引用**（来源徽章），不合并命名空间、不伪装本地路径。
   - 敏感操作默认在**执行端弹窗授权**，可"信任此设备"，**信任仅到本端进程重启为止**。

---

## 0.1 Capacitor 桌面端调研（2026-10-02 补做，此前遗漏）

> 用户质疑："你调研过 Capacitor 桌面端吗？" —— **此前没有**，默认把"桌面端（web）"当成浏览器 web 平台。补调研如下。

**事实 1：Capacitor 官方没有桌面平台。** 官方三平台 = Android / iOS / Web；
`npx cap add <platform>` 只接受 android / ios；`Capacitor.getPlatform()` 只返回
`'android' | 'ios' | 'web'`（`isNativePlatform()` 也只有这三态）。所以"桌面端"必然是
**web 形态**或**第三方平台**。

**事实 2：第三方桌面平台有两条路线**

| 路线 | 载体 | 关键事实 | 与本项目 |
|---|---|---|---|
| **A. Web（当前采用）** | 浏览器 / cnb 托管 | 官方一等公民；插件走 web 实现或 stub（本项目 `ApiProxy.web.ts`、`GoProcessWeb` 已是这条路） | 与"桌面端在 cnb"拓扑一致；无本地文件系统/本地后端 |
| **B. Electron（Capawesome）** | `@capawesome/capacitor-electron` | 兼容 Capacitor ≥6（本项目 8 OK）+ Electron ≥28，活跃维护（2026-07 文档）；`getPlatform()==='electron'`、`isNativePlatform()===true`；**只有带 Electron 实现或有 web 实现的插件可用**（自研 `GoProcess`/`ApiProxy`/MLKit 只有 Android 实现，需补）；体积 80–150MB；原生 Node 插件需自行 rebuild；**许可条款未确认**（需用户/法务确认，Insiders 可能性存在） | 桌面本地可跑 encv-go、本地索引、可做 P2P；代价=打包分发 + 插件实现 + 许可 |
| ~~C. `@capacitor-community/electron`~~ | 社区版 | 要求 Capacitor 5.4+、v4 后不自带插件；Capawesome 官方提供"从社区版迁移"指南 ⇒ 视为**已停滞** | 不采用 |
| **C. Wails（Go 原生）** | `github.com/wailsapp/wails`（v3 **beta**，见下） | Go 后端 + 系统 WebView（Win=WebView2/Chromium、macOS=WKWebView、Linux=WebKitGTK）；MIT；12MB 级；**桌面专用**（无移动端） | **与本项目后端同为 Go**，契合度最高（见 §0.2） |
| （备选）Tauri | Capawesome 亦有 Tauri 平台 / Rust 社区版 + Go sidecar | 体积远小于 Electron，但生态/插件面更小 | 仅备选，未评估 |

### 0.2 B（Electron）vs C（Wails）评估（2026-10-02）

> 权威事实：**Wails v3 仍为 beta** —— GitHub Releases 最新 `v3.0.0-beta.27`（2026-10-01），官方说明
> "This is pre-release software. The API is stable, but you may still encounter issues before the final 3.0 release"。
> 稳定线是 v2（单窗口；v3 才有多窗口）。仓库 36.4k star，活跃。

| 维度 | B. Electron（Capawesome） | C. Wails（v3 beta / v2 稳定） | 对本项目的判断 |
|---|---|---|---|
| **与 Go 后端的契合** | 后端仍是独立进程（Node 壳 + Go 子进程），跨语言 IPC/HTTP | **Go 后端可进程内运行**（同一二进制），无跨语言边界 | **C 明显优**：可顺带消灭桌面端现有一堆坑——CORS、端口扫描 2025+、`ApiProxy` 插件绕 CORS 的历史包袱 |
| **体积 / 启动** | 80–150MB（自带 Chromium+Node） | ~12MB，亚秒启动（用系统 WebView） | C 优 |
| **前端栈兼容** | Chromium ⇒ 与现有 Chromium 114 基线一致（Tailwind v4 / daisyUI v5 稳） | **Linux 是 WebKitGTK，不是 Chromium** ⇒ `color-mix()` / OKLCH / `:has()` / cascade layers 等基线特性需**真机验证**（这是 C 的头号风险） | B 稳、C 待验；**必须真实复现验证后才能选 C**（复现优先铁律） |
| **Capacitor 插件** | 插件需 Electron 实现或 web fallback；自研 `GoProcess`/`ApiProxy`/MLKit 只有 Android 实现 | **不是 Capacitor 平台**，插件体系完全不适用；但桌面若进程内跑 Go，`ApiProxy`/`GoProcess` 这类"桥"整体可退休 | C 改动面更大但更彻底（前提：桌面不再走 Capacitor 插件） |
| **API 契约** | 沿用 HTTP/SSE（与安卓端一致） | 可用 bindings 也可**继续 HTTP/SSE**（推荐后者，保持单契约） | 打平；推荐都保留 HTTP 契约 |
| **成熟度 / 许可** | 活跃；**许可条款未确认**（Capawesome，可能需 Insiders） | v3 **beta**（API 声明稳定但频繁发版，近月约每 3–7 天一个 beta）；MIT | B 成熟度略优但许可不明；C 许可清晰但版本是 beta |
| **打包 / CI** | electron-builder，需按 OS 打包（wine 可部分交叉） | 需 CGO + 各 OS WebView 运行库，**必须按 OS 原生构建**（三平台 CI） | 打平（都要多 OS CI）；本项目现有 CI 仅 Linux（cnb） |
| **移动端** | 桌面专用 | 桌面专用 | 打平：安卓端仍是 Capacitor，不受影响 |

#### 0.2.1 实测记录（2026-10-02，Wails v3.0.0-beta.27 真机跑）

| 项 | 实测结果 |
|---|---|
| CLI 安装 | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` ✅；`wails3 doctor` 通过（Go 1.25.1 / CGO=1 / Debian 13） |
| Linux 依赖 | **GTK4 + WebKitGTK 6.0**（`libgtk-4-dev` + `libwebkitgtk-6.0-dev`）——注意**不是**旧教程里的 webkit2gtk-4.1；缺了这两个 `go install` 会在 cgo 阶段直接失败 |
| 脚手架 / 构建 | `wails3 init -n probe -t vanilla` ✅；`go build` ✅；**产物 16.7MB**（Electron 80–150MB 量级对比） |
| 运行时日志 | Wails 正确识别 `GTK=4.18.6 WebKitGTK=2.52.6`，进程内 HTTP 服务正常监听（证明"Go 后端进程内运行 + 窗口加载 http://127.0.0.1:port" 这条形态跑得通） |
| **窗口/渲染** | ❌ **沙箱内无法渲染**：`bwrap: Creating new namespace failed: Operation not permitted` → WebProcess 起不来 → SIGTRAP。`unshare --user true` 同样 EPERM ⇒ **容器禁止非特权 user namespace**，而 WebKitGTK 2.52 的 WebProcess 必须经 bubblewrap 启动。这是**环境限制，不是 Wails 缺陷**（Wails v3 源码未暴露关闭 sandbox 的选项） |
| CSS 基线验证（Task 1.9） | **被阻断**：无法在沙箱内用真实 WebKit 验证 `color-mix()`/OKLCH/`:has()`/cascade layers。**必须真机（或允许 userns 的环境）验证，禁止据推断下结论** |
| 附带发现 | v3 CLI 带 `ios` / `android` 子命令（文档亦称同一份 Go+前端可编译到 iOS/Android）⇒ **值得单独调研**：可能改变"安卓端=Capacitor"的前提（未验证，不作为结论） |

#### ✅ E6 已拍板：A（web）—— 桌面端不要窗口（2026-10-02 用户明确）

> 用户原话：**"linux 端主要是服务器，有 web 就行，不要求窗口"**。

**结论**：
1. **E6 = A（web）**：桌面端交付形态 = **浏览器**（Linux 仅作服务器托管，用户在任意桌面浏览器里用）。**B（Electron）/ C（Wails）本期不做**。
2. 由此**作废的前置任务**：Task 1.9（WebKit CSS 基线验证，且沙箱本来也做不了）、Task 1.10（Electron 许可与插件确认）。保留记录但标记"因 E6=A 不再需要"，不删除（防下任误执行）。
3. **桌面端无法访问用户本机文件**（浏览器沙箱）⇒ 桌面端的数据来源只有两处：**服务器上的 encv-go（cnb 同源）** 或 **已配对的安卓端**——这与 §1 的 Hub 拓扑完全一致，P2 设计不受影响。
4. 若将来要 **Windows/macOS 原生壳**，再回看 §0.2 的 B/C；届时仍需先过 Task 1.9/1.10。
5. （保留参考）架构契合度上 **C > B**：后端同为 Go 可进程内运行；已实测 Wails v3 beta 构建可行、产物 16.7MB（见 §0.2.1）。这是"将来也许要用"的备选结论，不是本期路线。

**事实 3（对代码的直接影响，已修）**：桌面形态判定**不能只看 `isNative()`** ——
Electron 桌面下 `isNative()===true` 但形态是桌面。`computeFormFactor` 已新增
`platform` 维度（`web`/`electron` 才是桌面候选），并由 `appCapabilities.platform()`
（app 侧 `Capacitor.getPlatform()`）注入；4 个新单测覆盖（electron→desktop、android/ios→永不 desktop、未注入回退 isNative）。

**新增待决 E6**：桌面端形态走哪条？（A web / B Electron / C Wails，对比见 §0.2）
- 走 A（web，当前默认）：维持 Hub/中继架构，零分发、与 cnb 拓扑一致；桌面无本地能力。
- 走 B（Electron）：桌面成为本地一端（本地 encv-go + 本地索引），P2P 可行性大增；需补插件实现 + 打包分发 + **确认 Capawesome 许可**。
- 走 C（Wails）：后端同为 Go ⇒ **进程内运行**，消灭 CORS/端口扫描/ApiProxy 桥；但需先验证 ①Linux WebKitGTK 的前端基线兼容（真机渲染验证）②v3 beta 可接受；桌面端 Capacitor 插件层可退休。
- **无论 A/B/C，形态层（formFactor/rail）与连通层已解耦** —— 已落地工作通用，P2 可继续推进不阻塞。
- **E6 前置验证项（选 C 前必须做，禁止靠推断）**：在目标桌面 OS（至少 Linux）用系统 WebKit 渲染本应用首页，验证 Tailwind v4 / daisyUI v5 的 `color-mix()` / OKLCH / `:has()` / cascade layers 实际生效（真实渲染 + `getComputedStyle` 断言，不用截图猜）。

---

## 1. 连通性架构：Hub + 出站长连接（修正版）

```
用户浏览器（HTTPS 公网）
   │  同源 REST / SSE
   ▼
cnb 容器：preview-gateway :16666 ──► encv-go :2025（含 peerlink Hub）
                                        │  WSS /api/peerlink/ws（公网，TLS）
                                        │  ↑ 手机端【主动出网】建立（唯一需要打洞的方向 = 无）
                                        ▼
                        安卓 App：encv-go（设备内） ← WebView UI 经 127.0.0.1 交互
```

- **Hub** = cnb 上 encv-go 内的 `internal/peerlink/hub`：WSS 终点 + 配对会合 + 密文中继。
- **Edge** = 手机端 encv-go 内的 peerlink 客户端：**主动出网**建 WSS，心跳保活。
- **桌面 UI** 只调同源 REST/SSE；跨端调用由 Hub 经已建立的 WSS 转发给 Edge。

**长连接放 Go 侧，不放 WebView**（关键决策）：息屏/切后台/Activity 重建都不影响链路；UI 只负责渲染与授权弹窗，经 `127.0.0.1` 本机 API 与 Go 交互。

### 为什么这版能规避掉一整类问题

| 问题 | LAN 直连（作废） | Hub + 出站长连接（采用） |
|---|---|---|
| CORS | 桌面 origin `https://xxx` → 手机 `http://192.168.x.x` 被 `gin_app.go` allowlist 拒（只放行 localhost/127.0.0.1） | 桌面只同源；手机只连本机 127.0.0.1 → **无跨源面** |
| 混合内容 | https 页面请求 http 内网地址 → 浏览器直接拦 | 全链路 TLS/WSS |
| NAT | 手机无公网地址，桌面根本连不上 | 手机**出网**，无需入站可达 |
| 地址漂移 | 依赖内网 IP | 只依赖 hub 域名（见 R6 的漂移对策） |

---

## 2. 配对握手（草案 v2，适配公网拓扑）

二维码内容（**不再放 addrs[]**，公网场景无意义）：

```
{ v:2, hub:"https://<cnb-host>/api/peerlink", pairingId, psk:<base32 32B>, exp:120s, hubFp }
```

流程：
1. **桌面端**向同源 Hub 申请票据（`POST /api/peerlink/ticket`）→ 得到 `pairingId + psk + hub` → 渲染二维码（120s、一次性）。
2. **手机扫码**（`@capacitor-mlkit/barcode-scanning`）→ 本机 Go 用 `hub` 建 WSS → 发 `pair{ pairingId, deviceId, name, platform, proof: HMAC(psk, pairingId+"|"+deviceId) }`。
3. **Hub** 校验 proof → 通知桌面端 → **桌面端弹窗确认**（显示对方设备名 + 指纹）。
4. 双方 `HKDF(psk)` 派生 **AEAD 密钥** + 两个方向 token；桌面端/手机端各显示 **6 位 SAS 短码**供人工核对（防 hub 侧 MITM）。
5. **心跳**：Edge 15s（前台）/ 60s（后台）；连续 3 次失败 → offline + 指数退避重连。
6. **解配**：`unpair` → 双端清理 + token 立即作废 + Hub session 清除。

**存储纪律（硬红线）**：配对关系（peerId/名称/hub 地址）可持久化；**psk / token / 信任态只存进程内存**，禁止落 `localStorage`、`config.user.json`、任何磁盘文件。

---

## 3. 互通搜索索引（联邦 ≠ 挂载）

- Edge 侧暴露 `GET /api/peerlink/search?q=&type=file|fulltext|vector&limit=`，**复用既有响应契约**（`/api/files/search-fulltext` 等），结果项附加 `peer{id,name,platform}`。
- 桌面端 `useFederatedSearch()`：本端 + 各在线 peer **并发**（单端超时 2s；超时/离线只降级为"该端无结果"，**绝不阻塞**主结果），按 `peerId+path` 去重，来源徽章区分"本机 / 安卓·<设备名>"。
- 远端命中**只是引用**：动作只有"在线打开 / 取回到本端"，路径始终带来源标识。

**非目标（写死防跑偏）**：❌ 不挂载 WebDAV/网络驱动器；❌ 不做统一命名空间；❌ 不自动同步索引或文件内容；❌ 不把远端路径伪装成本地路径。

### 流量分级（公网拓扑下的新增约束）

| 数据面 | 通道 | 规则 |
|---|---|---|
| 搜索查询/结果、Agent RPC、授权 | Hub **中继**（E2E 密文） | 小报文，默认允许 |
| 缩略图/元数据 | Hub 中继 | 允许（尺寸上限） |
| 文件"取回"/流媒体 | **默认不经 Hub** | 优先 P2P（见 §6 E3）；无 P2P 时提示"仅在线打开"，或显式限速 + 二次确认 |

---

## 4. Agent 远程调用与授权

- 桌面侧发起：`POST /api/peerlink/agent/invoke` → Hub → Edge；Edge 把工具投进**本端**工具循环执行。
- `NeedConfirm=true` 且调用方未获信任 → **执行端挂起**并推 `peerlink:approval_request` → **手机端 UI 弹窗**（复用 ApprovalCard 视觉 + `ui-*` 表面类）。
  - 手机在后台/息屏：**不自动同意**；挂起 90s 超时 → 自动 `decline` + 通知（用户回来可补批）。
- 决策集：`accept` / `accept_for_session`（会话级，已有 `sess.GrantedTools`）/ **`trust_device`（进程级，重启即失效）** / `decline` / `cancel`。**前两者不可混用**。
- 信任态存 Go 进程内存；UI 明示"信任至本端服务重启"，设置页可查看/撤销"本次运行已信任的设备"。
- 破坏性工具（删除/格式化类）**即使已信任也强制确认**。
- 全部远程调用（含信任后的自动执行）落审计日志（谁/对谁/工具/参数摘要/决策/耗时）。

---

## 5. 风险与故障矩阵（R1–R16）

> 每条 = 触发条件 → 后果 → 检测 → 处置 → 落分期。**P2 必须逐条有对策才能算完成。**

| # | 风险 / 故障 | 后果 | 检测 | 处置 | 分期 |
|---|---|---|---|---|---|
| R1 | 手机 NAT / CGNAT / 对称 NAT | 入站不可达（**出站长连接不受影响**） | — | 主链路本就走手机出网；仅 P2P 增强受影响 | P2 |
| R2 | 移动网 IPv6-only / 双栈 | WSS 建连失败 | 建连错误分类 | Hub 与客户端支持 IPv6 + happy-eyeballs；失败退 IPv4 | P2 |
| R3 | 混合内容（https 页请求 http 对端） | 浏览器直接拦截 | 单测 + 契约锁：配置/二维码中**禁止出现 `http://` 对端地址**（`https://` 或本机回环除外） | 全链路强制 TLS | P2 |
| R4 | CORS | 请求被拦 | 集成测试 | 桌面只同源；安卓只连 127.0.0.1（ApiProxy）；`gin_app.go` allowlist **不**为 peer 放开 | P2/P5 |
| R5 | **云端不可信**（hub 可读明文） | 隐私泄露 | 威胁模型评审 | 全链路 AEAD（psk 派生）；hub 只转发密文、不落盘；SAS 6 位核对 | P2 |
| R6 | **cnb 沙箱 30m 无心跳被回收**（`.cnb.yml:27`） | Hub 整体消失 / 地址漂移 → 已配对手机再也连不上 | 心跳超时 + 域名解析失败 | ① hub 地址持久化且**可更新**（扫码/设置可重指向）；② 固定自定义域名优先；③ 降级为"仅本端可用"并明示 | P2/P5 |
| R7 | 手机息屏 / 后台被杀 | 链路断 | 心跳超时 | Go 侧长连接 + Foreground Service 保活（Android 12+ 权限坑见 `.trae/documents` 与 `2.md`）；指数退避重连 | P2 |
| R8 | 电量 / 移动流量消耗 | 用户抱怨 | 心跳统计 | 前台 15s / 后台 60s 自适应；移动网提示；大数据二次确认 | P2/P5 |
| R9 | WebView 生命周期（Activity 重建） | 若长连接放 WebView 会断 | 设计评审 | 长连接放 Go 侧（§1 决策），UI 只做渲染与弹窗 | P2 |
| R10 | 时钟 / NTP 漂移 | 时间型校验误判 | — | 安全判定只用 nonce + 单调计数，**不用时间戳** | P2 |
| R11 | 重复配对 / 密钥轮换 / 解配残留 | 旧 token 仍可用 | 集成测试 | pairingId 一次性 + 短时有效；解配立即作废；Hub session 清理；单 peer 并发上限 | P2/P5 |
| R12 | 公网端点被扫描 / 滥用 | 资源耗尽 | 速率统计 | pairingId 短时效 + 失败计数熔断 + 单 IP 限速 + 未配对一律 401 | P5 |
| R13 | 大报文（取回文件）打爆 Hub 内存 | Hub OOM | 报文尺寸上限 | 分块 + 限流 + 断点；大流量默认不走 Hub（§3） | P3/P5 |
| R14 | 日志泄露（查询词 / 路径全文上云） | 隐私 | 日志审查 | Hub 日志脱敏：只记 peerId/字节数/耗时，不记查询词与路径 | P5 |
| R15 | 真机验证缺口（沙箱无相机 / 无移动网） | 扫码与移动网路径未验 | — | 模拟器 + `adb forward` 验 P2P/中继路径；扫码与移动网列为**必须真机**项并在 checklist 明示 | P6 |
| R16 | 桌面端"无本机后端"旧分支 | 需求理解错误 | 设计评审 | **作废**：桌面端后端恒为 cnb 同源 Go（`useAgentApiBase` web 分支已如此） | P1 |

---

## 6. 分期（多轮迭代，每轮独立可验收）

| 阶段 | 主题 | 交付物 |
|---|---|---|
| P0 | 侦察 + 契约 | spec/tasks/checklist/progress（**本轮**） |
| P1 | 桌面端形态 | `formFactor=desktop` + 侧边导航/双栏；桌面浏览器实测 |
| **P2a** | **Hub 与会合** | cnb 侧 peerlink Hub（WSS + ticket + pair + 密文中继）+ 手机端出站长连接 + 心跳/重连 |
| **P2b** | **配对 UI** | 桌面二维码（`qrcode` 依赖）+ 安卓扫码（`@capacitor-mlkit/barcode-scanning`）+ SAS 核对 + 设备页/解配 |
| **P2c** | **风险收口** | R1–R11 逐条对策落地 + 回归锁 |
| P3 | 互通搜索 | 联邦搜索契约 + 来源徽章 + 超时降级 + 流量分级 |
| P4 | 远程 Agent | 远程 invoke + 执行端弹窗 + `trust_device`（重启失效）+ 后台超时 decline + 审计 |
| P5 | 安全与降级 | R12–R14、token 零落盘回归锁、离线/换网/Hub 漂移降级矩阵 |
| P6 | 端到端验证 | 真机（扫码/移动网）+ 模拟器（中继/P2P）+ 文档同步 |

---

## 7. 模块落点

| 层 | 新增/修改 | 说明 |
|---|---|---|
| Go | `internal/peerlink/`（新） | hub / edge client / pairing / trust store / AEAD / 心跳 / 联邦搜索 / agent RPC / 审计 |
| Go | `internal/server/routes.go` | 注册 `/api/peerlink/*`（REST）+ `/api/peerlink/ws`（WSS）；**不动** `gin_app.go` 的 CORS allowlist |
| 前端 shared | `usePeerLink.ts`、`useFederatedSearch.ts`、`useRemoteApproval.ts` | 纯抽象 + DI，守 `docs/migration-task-system.md` 边界 |
| 前端 app | `views/PeerSettings.vue`（设备/配对/二维码）、`views/ServerSettings.vue`（**baseUrl 语义不变**） | 桌面显示二维码；安卓侧只是 UI 壳 |
| Android | 扫码插件；长连接由 Go 侧承担（不新增原生网络代码） | Foreground Service 保活权限成本（R7） |
| 依赖 | 前端 `qrcode`（桌面生成）；安卓 `@capacitor-mlkit/barcode-scanning` | 均已与用户确认采纳 |

---

## 8. 待决问题 E1–E5 —— 已拍板（2026-10-02）

| # | 决策 | 结论 | 落地要求 |
|---|---|---|---|
| **E1/E5** | Hub 可用性与域名 | **无固定域名，cnb 30m 无心跳即回收 → 按最坏情况设计** | Hub 地址持久化且**可重指向**（设置页/重新扫码）；Hub 不可用时降级为「仅本端可用」并**持久性 UI 明示**；不做"域名恒定"假设 |
| **E2** | 手机端长连接归属 | **Go 后端侧** | 息屏/后台/Activity 重建不断链；成本 = Foreground Service 权限与保活（R7，Android 12+ 权限坑见 `2.md`） |
| **E3** | P2P（WebRTC）是否本期 | **本期不做** | 控制面走中继即可；不留 P2P 半成品分支 |
| **E4** | 文件"取回"是否经 Hub | **默认禁止穿透云端** | 远端命中只提供「在线打开 / 缩略图」；R13 限流分块仍做（控制面小报文） |

---

## 9. 安全红线（贯穿全部分期）

1. 未配对 = 完全不响应（`/api/peerlink/*` 一律 401），**不因"有 TLS/在云上"而放宽**。
2. psk / token / 信任态**只存进程内存**；任何写盘都是红线（回归锁：重启进程后信任态必须为空）。
3. 二维码 120s 过期、一次性；**不得包含任何内网地址**。
4. Hub 视为**不可信中转**：一切 payload E2E 加密 + SAS 核对 + Hub 不落盘 + 日志脱敏。
5. 被信任 ≠ 全放行：破坏性工具强制确认；后台/超时一律 `decline`，**绝不默认同意**。
6. 大流量默认不穿透云端（§3 流量分级）。
