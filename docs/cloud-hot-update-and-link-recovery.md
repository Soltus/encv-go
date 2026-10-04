# 云控热更新 + 互联自恢复（多轮迭代追踪）

> 立项：2026-10-04（用户指令）
> ① *「设计过于谨慎，我需要服务端重启自动恢复连接」*
> ② *「重新构建 APK 比较麻烦，先重点在已有基础上进一步实现云控热更新，同样文档追踪多轮迭代」*
>
> 本文是**迭代台账**：每轮记「目标 / 落地 / 验证 / 遗留」，下一轮从遗留接着做。
> 状态图例：✅ 已落地并有回归锁 · 🟡 部分落地 · ⬜ 未开始 · ⛔ 已否决（记原因，防止反复）

---

## 0. 地基：已有基础（动手前先摸清，避免重新发明）

| 已有能力 | 代码落点 | 它证明了什么 |
|---|---|---|
| 资源包整包替换**不换 APK** | `internal/server/preview_assets.go`、`PreviewAssetsActivity.kt` 头注释 | 可写数据目录 + 远端 zip + staging 原子替换这条路走得通 |
| 云端构建并发布资源包 zip | `.github/workflows/preview-assets.yml`（发到 release tag `preview-assets`） | "云侧产出包"已有流水线雏形 |
| Hub ↔ Edge 长连接 RPC | `internal/peerlink/{edge,rpc,conn}.go`、`peerCalls` | 云控**指令**有现成通道（手机在 NAT 后，只能走它主动出网那条连接） |
| Go 二进制由 Kotlin 拉起 | `EncvGoService.findExecutableBinary()`（优先 `nativeLibraryDir`，回退 `filesDir` + `copyBinaryFromAssets`） | 二进制**有可能**被替换（回退分支已经会写 filesDir），是把"最后一次 APK"变成"永远不用再打"的关键口子 |

**最重要的一条认知**：设备端任何 Go/Kotlin 改动，都还要**最后一次 APK**（引导版）。
热更新的价值 = 引导版之后，web 资源（以及后续 Go 二进制）都不再需要重新构建 APK。

---

## I1 · 互联状态持久化 + 重启自动恢复 ✅

- **目标**：服务端/后端进程重启后，已配对设备**带着旧 token 重连即可**，不再重新扫码。
- **决策变更（推翻旧红线）**：旧纪律是"psk / token / 密钥只存进程内存，绝不写盘"
  （`internal/peerlink/types.go`、`MOBILE_STRATEGY.md` 关键红线）。**已被本轮取代**：
  - 新纪律 = *把钥匙锁进抽屉，而不是把钥匙扔了*：
    ① 落盘位置 = 应用私有数据目录（`config.AppDataDir("peerlink")`，Android 在 app filesDir 下）；
    ② 文件 `0600`、目录 `0700`、原子写（临时文件 + rename）；
    ③ **可撤销**：unpair / 主动断开立即删除；
    ④ **可关闭**：`ENCV_PEERLINK_PERSIST=0` 退回旧纪律；
    ⑤ **psk 仍不落盘**（只在扫码那一次有用），落的是配对后派生的会话凭据 + 双向 AEAD 密钥。
  - 恢复的是**身份与通道**，**不是授权**：`trust_device` 仍是进程内存（重启后要点一次信任）。
- **落地**：`internal/peerlink/store.go`（读写 + 原子写 + 权限）、Hub `ExportState/RestoreState`、
  `internal/server/peerlink_state.go`（目录/关闭开关/恢复/保存/清除）、
  保存点 = `handlePeerlinkPair` / `unpair`；Edge 会话 = `/edge/pair` 成功后保存、`/edge/stop` 清除、
  `Server.Start()` 自动重连。
- **验证（先红后绿，均实跑）**：`internal/server/peerlink_state_test.go`
  - 跨"重启"恢复同一台设备 + 旧 token 继续可用；
  - **反向锁**：恢复出来的设备 `Online` 恒为 false（在线只能由长连接证明，不能从磁盘读出来）；
  - 解配后磁盘同步删除（否则"被解配的设备"重启即复活）；
  - 坏文件不得阻止启动（保守不恢复）；`ENCV_PEERLINK_PERSIST=0` 完全不落盘。
  - 红：关掉落盘 ⇒ "配对后应落盘 peers.json"；放开 Online 继承 ⇒ "Online 必须为 false"。
- **遗留**：真机验证需引导版 APK（Go 进程重启场景 = APP 冷启动 / 后端崩溃重启）。

---

## I2 · 云控热更新：通道 + 安装器 ✅（设备端待装机验证）

- **目标**：云端（Hub）能指名道姓让某台设备升到某个资源包版本；设备自己去拉包、校验、原子生效、失败回滚。
- **设计（控制面 / 数据面分离）**：
  - 控制面（WS RPC，几十字节）：`POST /api/peerlink/bundle/push` → `bundle_update` 指令
    `{name, version, sha256, size, required}`。走已有长连接，因为手机在 NAT 后、没有可被 Hub 直连的地址。
  - 数据面（HTTP，几 MB~几十 MB）：设备主动出网 `GET <hubURL>/bundle/download?name=&version=&token=`。
    理由：R13 控制面小报文；HTTP 才有 Content-Length / 断点续传，也不会长时间占住控制连接。
- **落地**：
  - `internal/bundle/apply.go` —— 通用安装器（由 preview-assets 的那套逻辑抽出）：
    `Download`（边读边限流 + 超限删半截文件）→ `sha256` 校验 → staging 解压（Zip-Slip 防护）
    → 必含文件校验 → 备份当前版 → **原子 rename** → 写 `version.json`；失败保持旧版，`Rollback` 可退回。
  - `internal/server/peerlink_bundle.go`（Hub 侧）：
    `manifest`（清单）/ `download`（zip，peer token）/ `report` / `push`（运维下发）/ `status`（云控视角）。
    仓库目录 `config.AppDataDir("bundles")`，文件命名 `<name>-<version>.zip` + `.sha256`。
  - `internal/peerlink/bundle.go` + `edge.go` —— RPC 报文与 `bundle_update` 分发。
  - `internal/server/peerlink_edge_runtime.go` —— 执行端 `peerLocalBundleUpdate`：
    包名→目录白名单（`preview-assets` / `web`），摘要缺失或包名未登记 ⇒ **rejected**（发起端 400、
    **不计入熔断**），下载/校验/切换失败 ⇒ 回滚 + 回报脱敏原因。
- **不变式（回归锁逐条钉死）**：先 staging 后切换；摘要不符完全不碰目标目录；缺必含文件不切换；
  超限不留半截文件；Zip-Slip 写不出去；失败可回滚到上一版。
- **验证（先红后绿）**：`internal/bundle/apply_test.go`（7 例）+ `internal/server/peerlink_bundle_test.go`（7 例）
  - 端到端：Hub 造包 → push → Edge 真的下载 → 落盘生效；摘要错 ⇒ 失败且旧版完好；
  - 反向锁：被拒绝的 push 是 **400 不是 502**，且**不连坐熔断**（紧接着的合法 push 仍 200）。
  - 红：去掉摘要校验 ⇒ "应报 ErrChecksumMismatch" + 设备端 "失败后旧版必须完好"。
- **遗留**：真机端到端需引导版 APK；`download` 目前只复用 `rateAllow("file")` 做粗粒度准入，
  **字节级流量记账还没接**（`peerlink_limits.go` 的按流量配额应覆盖 bundle，见 I3）。

---

## I3 · Go 二进制热更新（彻底摆脱 APK） ✅（待装机真机验证）

- **目标**：让 **Go 后端本身**也能云控更新 —— 这样 I1/I2 之后新增的后端能力不必等 APK。
- **已落地**：
  - **Kotlin**（`EncvGoService.findExecutableBinary()`）：`<filesDir>/encv-go` **优先**于 APK 内的
    `libencv-go.so`，但必须满足两道门禁：① 存在 `.version` sidecar（说明是云控通道放的）；
    ② `.abi` 等于 `Build.SUPPORTED_ABIS[0]`（架构不匹配 ⇒ `CANNOT LINK EXECUTABLE`）。
  - **失败自动回滚**：用热更二进制启动失败 ⇒ `publishFailure` 里把它 rename 成 `.bad-<ts>`，
    作废 sidecar，并用 APK 内二进制**重试一次**（`rollbackHotBinaryIfBroken`）。
    绝不能让设备卡在"后端起不来"的状态 —— 后端挂了连远程调试都救不回来。
  - **Go**（`bundle.ApplyFile`）：zip 解到 staging → 校验 sha256 + 必含文件 → 备份当前 →
    **rename 原子覆盖** → `chmod 755` → 写 `.version` / `.abi` sidecar；失败 `RollbackFile` 换回。
    ⚠️ rename 覆盖正在运行的可执行文件是安全的（进程继续持有旧 inode），
    **新二进制要重启进程才生效**（下次 APP 冷启动 / 服务重启自动生效，已打日志说明）。
  - 包名 `go-binary`；下发指令**必须带 ABI**，否则执行端 `rejected`（`missing_abi`）。
- **验证（先红后绿）**：`internal/bundle/apply_file_test.go` 4 例
  （原子替换 + sidecar + 回滚 / 摘要不符不动运行中的二进制 / 缺文件不动 / 无备份不造假文件）；
  `TestPeerLocalGoBinary_RejectedWhenUnknownOrNoABI`（拿不到 filesDir 或没带 ABI ⇒ rejected）。
  红验：去掉摘要校验 ⇒ "应报 ErrChecksumMismatch"；去掉备份 ⇒ "换执行体必须留备份"。
- **遗留**：
  - `:app:compileDebugKotlin` 在本环境**无法验证**（JitPack `getActivity:Logcat` 解析失败，
    与本改动无关的前置网络问题）⇒ Kotlin 改动只做了逐行复核，等用户在可联网环境构建。
  - 热更二进制**不会自动触发重启**：当前靠下次冷启动生效。若要"下发即生效"，
    需补一条 `GoProcessPlugin.restartBackend` 的触发路径（Go 侧回报 → TS/原生调重启）。

---

## I4 · 主应用 SPA 热更新 ✅（待装机真机验证）

- **目标**：主界面（Capacitor web 资源）也走云控，不再跟着 APK 走。
- **关键机制（与 PreviewAssetsActivity 那条路不同，别照抄）**：
  `bridge.setServerBasePath(path)` → Capacitor `WebViewLocalServer.hostFiles(path)`，
  把**任意应用私有目录**托管在**同一个 `https://localhost` 源**下。
  - 好处：origin 不变 ⇒ CORS / `baseUrl.ts` 判定 / 混合内容策略全不变，
    Capacitor 插件桥（GoProcess / BarcodeScanner / Filesystem…）照旧工作；
  - 反例（**不要这么做**）：`webView.loadUrl("http://127.0.0.1:<port>/web/")` 会换 origin，
    插件桥注入与 `WEBVIEW_SERVER_URL` 都绑本地源 ⇒ 主应用插件直接失效。
    `PreviewAssetsActivity` 能用独立 WebView，只因为那一页不需要任何插件。
- **落地**：`MainActivity.applyHotWebBundleIfPresent()` —— 目录
  `<filesDir>/.encv/web-bundle` 同时存在 `index.html` **和** `version.json` 才切换；
  目录里有 `.disabled` 标记则跳过（留一条不用重打 APK 的退路）；判定不过就用 APK 内资源。
- **可观测**：`get_device_info` 新增 `webBundle.{installed,version}` ——
  云控下发后用它**远程确认**生效版本（判定与 Kotlin 侧一致：缺 `index.html` 即算未安装）。
- **产包**：`scripts/build-web-bundle.sh [version] [--no-build]` → `web-<ver>.zip` + `.sha256`
  （dist 内容放 zip 根；base 保持 `/`，因为托管在 `https://localhost/` 根）。
- **验证**：`TestDiagGetDeviceInfo_WebBundleInstalled`（3 段：未装 / 已装回显版本 / 缺 index.html 判未装），
  先红后绿（去掉 index.html 判定 ⇒ "未安装时应 installed=false"）。
- **顺带修的真缺陷**：清单按**字典序**选最新版 ⇒ `v0.0.9` 会压过 `v0.0.10`、
  `v0.0.1-test` 压过 `v0.0.1-smoketest` ⇒ 云控可能下发**旧包**。
  改为语义版本比较（`versionNewer`：`TestVersionNewer` 10 例 + `TestPeerlinkBundle_Manifest_LatestBySemver`，
  红：改回字典序 ⇒ "最新版应为 v0.0.10, got v0.0.9"）。
- **遗留**：Kotlin 改动在本环境无法编译验证（JitPack 不可达，与改动无关）；
  热更包应用后需**重启 APP** 才切换（与 I3 同步：进程/页面重启即生效）。

---

## I5 · 云控策略（灰度 / 强制） ⬜

- 目标：manifest 支持 `minAppVersion`、按 deviceId 灰度、"强制升级"开关。
- 依赖：I6 的台账已能给出"每台设备当前版本"，够做策略输入。

---

## I6 · 热更**记录持久化** + **一键回滚** ✅（2026-10-05）

- **起因（真机实测）**：
  - 台账 `globalBundleReports` 是**纯内存** ⇒ 后端重启后 `reportCount` **1 → 0**，
    "这台设备装到哪版了""上次下发成功还是失败"全部消失，云控退化成"每次都当没推过"；
  - 回滚**只有**设备端 `RollbackFile`（装坏时的自保动作），**云端没有任何一键回滚入口**
    ⇒ 一次坏包下发后，运维只能重新打一个旧版本包再走一遍下载/校验/切换（分钟级），期间设备一直是坏的。
- **落地**：
  - 台账落盘：`internal/peerlink/store.go` 新增 `SaveState/LoadState`（通用命名状态，
    复用 0600 + 原子写同一套纪律，文件名禁含路径分隔符）；
    `internal/server/peerlink_bundle.go` 的 `saveBundleReports/restoreBundleReports`，
    启动时（`NewServer`）恢复，每次回报/下发/回滚后写盘。
  - 云控回滚：新增 RPC `bundle_rollback`（`internal/peerlink/bundle.go` + `edge.go` 分发 +
    `Supports()`），Hub 侧 `POST /api/peerlink/bundle/rollback`（运维）。
    ⚠️ 与 I2 同一个坑：`OnBundleRollback` 必须**同时**加进 `peerEdgeHandlers`、
    `edgeHandlers()` 默认值、**以及 `startEdgeLocked` 传给 `NewEdge`**（漏第三处就一律 not_supported）。
  - 设备端自回滚 + 状态查询：`GET /bundle/local`（本端装了哪版 / 有没有备份可退）、
    `POST /bundle/local/rollback`（移动端 UI 的回滚按钮走这条，不绕云端）。
  - 语义：目录型（web / preview-assets）整目录搬回上一版；
    文件型（go-binary）搬回备份**并删除 `.version`/`.abi` sidecar**
    ⇒ 回到 APK 内置二进制（不删 sidecar 会让 Kotlin 继续用一个"自称新版本"的回滚件）。
    没有备份可退 ⇒ **400 rejected**，不是 500（"没退路"不是设备故障，也不计入熔断）。
  - 台账判据修正：`lastVer` 的更新条件由「成功且未回滚」改为「**成功且生效版本非空**」——
    否则云控回滚成功后，云侧视角里设备版本会永远停在坏包那一版。
- **UI**：`PeerSettings.vue` 新增「热更新」区块（本端已装版本 + 回滚；云控：可用包 / 设备版本 /
  最近记录 + 一键回滚）；`usePeerLink.ts` 新增 `fetchBundleStatus / rollbackBundle /
  fetchLocalBundles / rollbackLocalBundle`；i18n `peers.bundle*`（zh + en）。
- **验证（先红后绿，均实跑）**：`internal/server/peerlink_bundle_rollback_test.go`
  - 台账跨"重启"恢复（含设备当前版本）；红验：把 `saveBundleReports` 改成 no-op ⇒ `got 0`；
  - 云控回滚端到端（Hub → Edge 真收到指令）+ 台账标 `rolledBack`；
  - 接线锁 `TestPeerEdge_WiresBundleRollback`；红验：去掉传给 Edge 的那行 ⇒ `Supports=false`；
  - 本端状态与自回滚（磁盘内容真的退回 v1）；没备份 ⇒ 400 `no_backup`。
- **遗留**：真机验证需引导版 APK；多版本备份（不止"上一版"）尚未支持。

---

## I7 · 配对票据持久化 + 失败原因可诊断 ✅（2026-10-05）

- **起因（真机事故，`docs/HANDOVER-cloud-hot-update.md` §4）**：热更后重启后端 → 手机连不上，
  `edge/pair failed: 502` + `pair_rejected:401`，后端日志
  `peerlink pair rejected … reason=peerlink: ticket not found (consumed or unknown)`。
- **两个真因**：
  1. I1 只把「会话」落盘，**票据仍在内存** ⇒ 后端一重启，桌面上正在展示的二维码立刻失效；
  2. `redeem()` 把「不存在 / 已使用 / 已过期」合成**同一个** sentinel ⇒ 日志与 UI 都分不清。
- **落地**：
  - 票据落盘：`store.go` 的 `tickets.json`（`pending` + `used`，与 peers.json 同目录/同 0600/原子写，
    `ENCV_PEERLINK_PERSIST=0` 同样关闭）；Hub 新增 `SetStore / RestoreTickets / LastPersistError`；
    **只落未消费的票据**，消费后只留 `pairingId + 过期时间`（**不含 psk**）；
    读回时按 `ExpiresAt` 清一遍 ⇒ **过期票据绝不被复活**。
  - 错误三态：`ErrTicketNotFound` / `ErrTicketExpired` / `ErrTicketUsed`，HTTP 分别
    `ticket_not_found` / `ticket_expired` / `ticket_used`，都带 `refreshQr:true`。
    ⚠️ **决策变更**：旧 R10 纪律刻意"不区分以防探测"（`hub_ticket_r10_test.go`）——
    **已被本节取代**（一次性语义不变；pairingId 是 128 位秘密，猜不中就没有探测入口），
    代价（真机连不上却查不出原因）远大于收益。旧结论处已标注。
  - 原因码跨端透传：`pairToRemoteHub` 解析远端响应体的 `error`，`/edge/pair` 回 `reason` + `refreshQr`；
    前端 `PairEdgeError`（`usePeerLink.ts`）+ `PeerScanPanel` 的票据类文案与"解决办法"提示。
- **验证（先红后绿）**：
  - 复现（修复前实跑）：重启后旧票据 `HTTP 401 ticket_used`、日志 `ticket not found (consumed or unknown)`；
    已用/不存在两个错误**文本完全相同**。
  - `internal/peerlink/hub_ticket_persist_test.go`（4 例：跨重启可用 / 三态可区分 /
    过期不复活 / 痕迹不含 psk）、`internal/server/peerlink_ticket_restart_test.go`（HTTP 层 3 例）、
    `internal/server/peerlink_edge_pair_reason_test.go`（原因码带回 + 旧后端不误标）。
  - UI 契约锁 `src/components/__tests__/PeerScanPanel.qrHint.test.ts`（真实 DOM）：
    票据失败 ⇒ 文案含"刷新二维码"且有"怎么做"提示；非票据失败 ⇒ **不得**误导成刷新二维码。
    红验：去掉组件里的原因码映射 ⇒ 回到"连接失败：…502 ticket_expired"（真机原样）。
- **遗留**：真机复验需引导版 APK。

---

## I8 · 设备指纹 ✅（2026-10-05，待装机验证）

- **起因**：deviceId 每次进程启动随机生成 ⇒ 同一台安卓机**重装 / 清缓存**后再配对，
  Hub 侧就多一条 peer 记录（真机实测同机两条：`ac5e1c75ede3` / `6454ab90716f`）。
- **落地**：`internal/server/peerlink_device.go` ——
  ① `ENCV_DEVICE_ID` 显式覆盖（CI/自测）→ ② Kotlin 注入的 `ENCV_ANDROID_ID`
  （取 sha256 前缀，**不透传系统标识原文**）→ ③ 落盘随机 UUID（`device.json`）
  → ④ 进程内随机（持久化不可用时退回旧行为）；
  `NewServer` 里 `s.peerHub.SetDeviceID(...)` ⇒ Hub 的 peerId 即设备指纹。
  Kotlin：`EncvGoService.deviceAndroidId()`（`Settings.Secure.ANDROID_ID`，排除坏值
  `9774d56d682e549c`），在 `ProcessBuilder.environment()` 处注入（与 `ENCV_APP_FILES_DIR` 同一处）。
- **边界**：指纹只用于"识别同一台设备"，**不是密钥、不承担鉴权**（鉴权仍靠票据派生的 token 与 AEAD 密钥）。
- **验证**：`internal/server/peerlink_device_test.go`（4 例：派生且稳定 / 覆盖优先 / 跨启动一致 / 空值不覆盖）。
- **遗留**：Kotlin 侧在本环境无法编译验证（JitPack 不可达，与改动无关）；真机需装机确认"重装后仍是同一个 peer"。

---

## 真机验证（2026-10-05 05:17–05:25，安卓真机 aa2ae6d94645）

> 设备端 Go 二进制**已含本轮 I6/I7/I8 改动**（判定依据见下），故三轮改动在真机上全部闭环。

| 项 | 真机结果 |
|---|---|
| 后端重启后手机自动重连 | ✅ `online:true`，`peerlink state restored peers=1`（I1 闭环） |
| I7 票据跨重启存活 | ✅ 重启**前**出的码，重启后 `POST /pair` = **200**（修复前必然 401） |
| I7 三态错误码 | ✅ 同一张码再来 = `401 ticket_used`；不存在的码 = `401 ticket_not_found`，均带 `refreshQr:true` |
| I7 票据恢复日志 | ✅ `peerlink tickets restored tickets=1` |
| I8 设备指纹 | ✅ 设备 deviceId = `aa2ae6d946454b02eb0f906a74980e84`（`a` + sha256 前缀 = ANDROID_ID 派生，非随机 UUID）；Hub 侧 peerId `r8c3…`（桌面端落盘随机） |
| I6 云控下发 | ✅ `push` ⇒ `appliedVersion:v0.0.1-hot`；再推 v0.0.2 ⇒ `previousVersion:v0.0.1-hot`（备份机制生效） |
| I6 云控一键回滚 | ✅ `rollback` ⇒ `ok:true, version:v0.0.1-hot, previousVersion:v0.0.2-hot`；首次回滚（无备份）正确给 `400 no_backup` |
| I6 台账持久化 | ✅ 重启后端后 `reportCount` 仍为 **3**（修复前 1→0），`deviceVer` 保持；日志 `bundle reports restored reports=3` |
| I4 热更生效可观测 | ✅ `get_device_info` 的 `webBundle={installed:true, version:…}` 能远程确认生效版本 |

- **判定"设备端含新代码"的方法**（比猜可靠）：云控回滚若回 `400 no_backup` = 设备端**认识** `bundle_rollback`；
  若回 `502 not_supported` = 旧二进制。本次是前者。
- **踩到的副作用（务必记住）**：把**测试包**（只有 `index.html`，内容是 `HOT-UPDATE-OK`）push 到真机后，
  手机下次冷启动会真的加载它 ⇒ 主界面变测试页。
  止损：用真实 `dist` 打包（`scripts/build-web-bundle.sh` 走 pnpm 会被无 TTY 卡住 ⇒
  直接 `./node_modules/.bin/vite build` 后手工打 zip + `.sha256`）push 覆盖。
  ⇒ **真机云控实验只用真实产物，或先在备用机上做。**
- **外部访问域名**：真机 Edge 记住的会合点即当前可用桌面 web 入口
  （本次为 `https://csw3mzwrjz-8100.cnb.run/`）；容器 hostname 拼出来的域名**不对**，
  用 `get_device_info` 的 `edge.hub` 或 `/api/peerlink/ticket` 的 `hub` 拿最可靠。

---

## 每轮收尾纪律（本项目通用，别跳）

1. **先红后绿**：新能力必须有能**先失败**的回归锁（本轮全部做过红验）。
2. **真机验证写清状态**：单测绿 ≠ 真机可用；每轮末尾必须写明"真机待验的是什么"。
3. **决策变更要在旧结论处标注**：本轮推翻了"绝不落盘"红线，已在
   `MOBILE_STRATEGY.md`、长期记忆、`internal/peerlink/store.go` 三处标注，避免下一会话照旧红线做事。
4. **环境事实**：手机在 NAT 后（出网 IP 是公网地址）、容器 `adb devices` 为空 ⇒
   所有设备端改动都只能靠单测 + 真机探针验证，装机是唯一真机验证途径。
