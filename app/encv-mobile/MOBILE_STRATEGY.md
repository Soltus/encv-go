# ENCV Mobile 双线并行策略

## 概述

| 路线 | 定位 | 技术栈 | 目录 |
|------|------|--------|------|
| **Capacitor 独立产品线** | ENCV 独立品牌产品 | Ionic Vue + Capacitor | `app/encv-mobile/` |
| **OpenList Mobile Fork** | ENCV 生态发行版 | Flutter (基于 OpenList Mobile) | `app/openlist/` |

---

## 一、Capacitor 独立产品线

### 技术栈
- **UI 框架**: Ionic Vue
- **移动端框架**: Capacitor
- **后端服务**: ENCV-go Daemon (本地运行)

### 架构
```
Ionic Vue UI (Capacitor WebView)
        ↓ localhost HTTP
ENCV-go Daemon
Stream/WebDAV/API
```

### 功能路线

#### v1: "能播放"
- [ ] 浏览加密文件
- [ ] 解密流播放
- [ ] Seek 支持
- [ ] 基础播放功能

#### v2
- [ ] 离线缓存
- [ ] 下载功能
- [ ] 后台播放
- [ ] 字幕支持

#### v3
- [ ] 本地 mount
- [ ] MediaStore 集成
- [ ] Android share
- [ ] 外部播放器支持

---

## 二、OpenList Mobile Fork 生态发行版

### 技术栈
- **UI 框架**: Flutter (继承自 OpenList Mobile)
- **核心**: OpenList Core + ENCV Driver

### 架构
```
ENCV-go Core
       ↓
OpenList Driver
       ↓
┌──────────────┬──────────────────┐
│ OpenList Web │ OpenList Desktop │
└──────────────┴──────────────────┘
       ↓
OpenList Mobile Fork
       ↓
Flutter Android/iOS
```

### 核心原则
- **不重构 ENCV-go 架构**
- **保持统一 ENCV Driver/Core**
- **不单独设计移动底层架构**

---

## 三、关键技术问题

### 播放器策略
- Phase 1: 复用默认播放器，验证 ENCV stream 是否可直接播放
- Phase 2: 如需引入 Native Player (Android: ExoPlayer, iOS: AVPlayer)

### 移动缓存层
- 加密分片缓存 + Memory cache + Disk cache + LRU

### Android 权限
- Scoped Storage / 后台播放 / Doze mode / 大文件 IO / SAF URI / Android 13+ 权限

---

## 四、核心原则

⚠️ 不要让 Mobile 成为"特殊实现"，必须保持：

```
OpenList Core → 统一 ENCV Driver → 所有平台继承
```

---

## 五、桌面端（web）与双端互联

> **状态（2026-10-02 更新）**：P0–P5 已落地（详见下方"已交付"），P6 端到端收尾中。
> **权威文档**：`.trae/specs/desktop-web-android-pairing/`
> （`spec.md` 契约 / `tasks.md` P0–P6 / `checklist.md` / `progress.md` 多轮迭代跟踪）。

新增第三条形态：**桌面端（web）**（在桌面浏览器里运行，非 dev 预览）、以及与安卓端的
**扫码配对互联**：

- **拓扑前提**：桌面端（web）跑在 **cnb 云开发环境（公网 HTTPS，与其 Go 后端同源）**，
  安卓端在 **NAT 后无公网地址** ⇒ 两端**不同网**，跨端一律走「**Hub（cnb）+ 手机主动出网的
  WSS 长连接**」，**禁止 LAN 直连**（https 页面请求 http 内网地址会被浏览器以混合内容拦截）。
- **配对**：桌面生成一次性票据二维码（内容 = 会合点 hub + pairingId + psk，**不含任何内网地址**，
  120s 过期）→ 安卓扫码（**ZXingLite**，2026-10-04 起；此前用 MLKit，依赖 Google Play 服务）
  → HMAC 校验 + SAS 短码核对 → 双向 token 与 AEAD 密钥
  （**只存进程内存**）→ 心跳保活（前台 15s / 后台 60s）。
- **互通搜索索引**：双端在线时并发查询对端索引，结果是**跨端引用**（带来源徽章），
  **明确不做**挂载网络驱动器 / 统一命名空间。
- **远程 Agent**：可调用对端执行调试工具；敏感操作默认在**执行端**弹窗授权，
  可"信任此设备"，**信任仅到本端服务重启为止**。
- **远程调用结果契约（2026-10-04 统一）**：`ok` **只表示**"RPC 送达 + 审批通过"
  （`edge.go`: `Ok = out.Err == nil`）。工具自己的**业务失败**是包成 `errJSON` 塞在 `result` 里的
  （如 `{"error":"mount_id is required"}`）⇒ 发起端识别该形状后把 `ok` 置 **false**，
  并把错误码/信息提到顶层 `errorCode` / `error`，`result` 原样保留。
  **HTTP 仍 200**（RPC 确实送达并被对端执行；4xx/5xx 只留给链路故障——离线 503 / 超时 504 / 对端拒绝 400）。
  ⇒ 调用方**只看 `ok` 就够**，不必再解析 `result`（此前只看 ok 会把工具失败当成功，真机实测踩到）。
- **远程调试诊断工具（2026-10-04 新增，只读、`needConfirm=false`）**：
  `get_device_info`（对端运行时画像：平台/版本/uptime/挂载点可用性/互联状态+最后失败原因/索引状态）
  与 `read_logs`（对端最近日志，支持 level 过滤 + since 增量，上限 200 条、单条 2000 字符）。
  ⚠️ 二者跑在**执行端二进制**里 ⇒ 对端 APK 未含此代码时调用会得 `unknown fs tool: ...`
  （真机实测现象），需重新构建安装 APK 后才可用。

### 已交付（代码落点）

| 能力 | 后端 | 前端 |
|---|---|---|
| 桌面形态 `formFactor=desktop` + 侧边 rail | — | `shared-components/src/composables/useFormFactor.ts`、`encv-mobile/src/views/Tabs.vue` |
| Hub / 票据 / 配对 / AEAD / SAS（全内存） | `internal/peerlink/{hub,crypto,types}.go`、`internal/server/peerlink_api.go` | `shared-components/src/composables/usePeerLink.ts` |
| Edge 长连接（**Go 侧**承载，非 WebView）+ 保活载体 = 既有前台服务 | `internal/peerlink/edge.go`、`internal/server/peerlink_edge_runtime.go` | — |
| 配对二维码 + SAS 核对 + 设备列表/解配 | 配对状态查询 `/api/peerlink/pairing/status` | `src/components/PeerPairingPanel.vue`、`src/views/PeerSettings.vue` |
| 扫码端（安卓）UI：扫码 / 粘贴配对码 → 本端 Go 作为 Edge 连 Hub（Task 2.6） | `POST /api/peerlink/edge/pair`、`GET /edge/status` | `src/components/PeerScanPanel.vue`、`src/peerlink/barcodeScanner.ts`（**自建 ZXingLite 插件**经 `registerPlugin`，web 构建不依赖原生包；原生实现 `BarcodeScannerPlugin.kt` + `QRScanActivity.kt`） |
| 联邦搜索 + 远端读（在线打开/缩略图） | `/api/peerlink/search`、`/api/peerlink/file`（`X-Peer-*` 来源头） | `useFederatedSearch.ts`、`PeerSourceBadge.vue`、`useFilesView.ts` |
| 远程 Agent 授权（**执行端**弹窗、90s 超时自动拒绝、破坏性强制确认、脱敏审计） | `internal/peerlink/agent.go`、`internal/server/peerlink_agent_api.go` | `src/composables/useRemoteApproval.ts`、`RemoteApprovalPrompt.vue` |
| 远程调用**结果契约统一**（工具业务失败 ⇒ `ok:false` + 顶层 `errorCode`/`error`，2026-10-04） | `internal/server/peerlink_agent_api.go`（`agentToolErrorOf`）、`internal/peerlink/agent.go` | 契约锁：`internal/server/peerlink_agent_contract_test.go` |
| 远程调试诊断工具 `get_device_info` / `read_logs`（只读，2026-10-04） | `internal/server/agent_diag_bridge.go`（派发见 `agent_plugin_bridge.go`） | 回归锁：`internal/server/agent_diag_bridge_test.go` |
| 互联状态持久化 + **重启自动恢复**（2026-10-04） | `internal/peerlink/store.go`、`internal/server/peerlink_state.go` | 回归锁：`internal/server/peerlink_state_test.go` |
| **云控热更新**（push 指令走 WS / zip 走 HTTP，2026-10-04） | `internal/server/peerlink_bundle.go`、通用安装器 `internal/bundle/apply.go` | 回归锁：`internal/server/peerlink_bundle_test.go`、`internal/bundle/apply_test.go` |

**迭代台账（多轮）**：`docs/cloud-hot-update-and-link-recovery.md`
（I1 互联自恢复 ✅ / I2 云控热更新 ✅ / I3 Go 二进制热更 ⬜ / I4 主 SPA 热更 ⬜ / I5 灰度与回滚策略 ⬜）。
| 安全收口：未配对 401 全量 / 零落盘结构性锁 / 限流+熔断 / 访问日志脱敏 / 降级矩阵持久性 UI | `internal/server/peerlink_{security,limits}*.go`、`gin_app.go`(sanitizedLogFormatter) | `src/composables/usePeerDegradation.ts`、`PeerDegradedNotice.vue` |

### 关键边界与红线（不可破）

- `baseUrl` = 本端自己的后端（语义不变，生产态默认**同源**）；`peer` = 已配对的另一台设备，
  **peer 不得被写进 baseUrl / 探测链**。
- ~~psk / token / 信任态 **只存进程内存**：进程重启 ⇒ 必须重新扫码 + 重新授权（`trust_device` 失效）。~~
  **⚠️ 2026-10-04 已被取代（用户：设计过于谨慎，服务端重启要能自动恢复连接）**：
  配对凭据**落盘**到应用私有目录（`config.AppDataDir("peerlink")`，0600、原子写、可撤销、
  `ENCV_PEERLINK_PERSIST=0` 可关闭）⇒ **重启后设备带旧 token 重连即可，不用重新扫码**；
  psk 仍不落盘；`trust_device` **仍未持久化**（恢复的是身份与通道，不是授权）。
  详见 `docs/cloud-hot-update-and-link-recovery.md` §I1 与 `internal/peerlink/store.go` 头注释。
- 长连接方向恒为 **手机主动出网**（手机在 NAT 后，出站永远可达，无需打洞；桌面/Hub 有公网地址）。
- CORS：`gin_app.go` 的 `AllowOriginFunc` **只放行** localhost / 127.0.0.1 / `https://*-plugin.local`，
  **没有**为 peer 放开 ⇒ 桌面端必须**同源**（或经本机后端代理）访问，不得 "浏览器直连对端 2025"。
- 远端命中的路径是**远端原始路径**（如 `/sdcard/…`），只加来源标注；**不搬运索引、不挂载、
  不伪装成本地路径**。

### 端到端真机验证（2026-10-02，脚本 `scripts/emu-peerlink-e2e.sh`）

拓扑：Hub = 宿主机真实 Go 进程；Edge = **模拟器内** x86_64 真实后端（Android 文件/权限/mount 语义 = 真机）；
Edge → Hub 用 `adb reverse`（模拟"手机主动出网到公网 Hub"）。**29 断言全绿**，覆盖：
出票 → 配对 → 长连接 → 联邦搜索真实命中 → 远端读真实字节 + `X-Peer-*` 来源头 →
远程 Agent 在**执行端**审批 → 已信任免打扰 → 进程重启后信任失效/老票据失效/自动重连 → 无身份 401。

过程中抓出并修掉两个真 bug（先红后绿 + 回归锁）：

1. **`servingDir` 初始化时序**（`internal/server/server.go`）：原先只在 `Start()` 赋值，
   而 `NewServer()` 里已在用它 ⇒ 空串 ⇒ **FTS5 启动后台建索引永远"no entries to index"（本地全文搜索 0 命中）**、
   mount root 退化成 cwd。⚠️ 极易误判为"索引坏了"——同一目录用 `/api/files` 列举完全正常。
2. **远程 Agent 决策被吞成 accept**（`internal/peerlink/edge.go`）：执行端 `trust_device` 后的第二次调用
   确实免确认，但回传 decision 恒为 `accept` ⇒ 调用端分不清"逐次同意"与"已信任自动放行"。
   修：新增 `peerlink.AgentInvokeOutcome` 把授权器真实决策（`auto`/`trust_device`…）一路透传。

### 尚未真机验证（P6 必验清单，沙箱无法覆盖）

- 安卓端 **ZXingLite 扫码**与相机权限（Task 2.6 UI 接线**已完成**：`PeerScanPanel.vue` 扫码 + 「粘贴配对码」
  降级路径，真实浏览器端到端 `pw-peer-scan.mjs` 9 断言全绿——票据/`/edge/pair`/Edge 长连接/`edge/status`
  /Hub 侧在线均为真实链路；**沙箱无相机**，"取图像"那步仍需真机）。
- 移动网（4G/5G、CGNAT / **IPv6-only**）真实建连、息屏/后台保活与电量表现（R2/R7/R8）。
- `trust_device` 在**杀 App / Application 重建**后的失效验证（进程级重启已验证通过）。

### 沙箱内跑安卓真机验证的必备前置（下次别再踩）

- `adb push` **不要**用短 timeout（开完机首传 68MB 会被 30s 掐断，文件静默消失）。
- 起后端要显式 `env HOME=/data/local/tmp`（adb shell 里 HOME 为空 ⇒ 应用数据落到只读 `/.local`
  ⇒ sqlite / 向量搜索 / FTS5 全部 `unable to open database file`）；未装 APK 时还要**预建**
  `/data/user/0/com.encvgo.app/files`。
- 后端端口是**自选**的（实测 1999/2025/2000），必须从 `successfully started` 日志解析，不能假设 2025。
- 配置要同时设**顶层 `server.dir`**（servingDir 来源）与 `mobile.server.dir`。
