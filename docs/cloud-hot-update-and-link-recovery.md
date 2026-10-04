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

## I4 · 主应用 SPA 热更新 ⬜

- **目标**：主界面（Capacitor web 资源）也走云控，不再跟着 APK 走。
- **现状**：主 app 的 web 资源由 `cap sync` 打进 `assets/public`，经 Capacitor 本地
  asset loader 以 `https://localhost` 提供 ⇒ 改一行 JS 就要重打 APK。
- **做法**：包目录 `web-bundle`（`config.AppDataDir("web-bundle")`，已在 I2 白名单登记）
  ⇒ 主 WebView 入口改为"有热更包则加载 `http://127.0.0.1:<port>/`，否则回退 `assets/public`"
  —— 与 `PreviewAssetsActivity` 完全同一套路（它已经这么干且验证有效）。
- **注意**：`baseUrl.ts` 的 native 判定不能因此退化（真机 WebView origin 仍可能是
  `https://localhost`，见长期记忆"Capacitor 原生壳里 origin ≠ 后端地址"）。
- **遗留**：同上，需要引导版 APK 带这次入口改造。

---

## I5 · 云控策略（灰度 / 强制 / 回滚指令） ⬜

- 目标：manifest 支持 `minAppVersion`、按 deviceId 灰度、"强制升级"开关、云侧一键回滚
  （再下发一次旧版本即可，I2 已具备能力，缺的是策略与 UI）。
- 依赖：I2 的 `status` 台账已能给出"每台设备当前版本"，够做策略输入。

---

## 每轮收尾纪律（本项目通用，别跳）

1. **先红后绿**：新能力必须有能**先失败**的回归锁（本轮全部做过红验）。
2. **真机验证写清状态**：单测绿 ≠ 真机可用；每轮末尾必须写明"真机待验的是什么"。
3. **决策变更要在旧结论处标注**：本轮推翻了"绝不落盘"红线，已在
   `MOBILE_STRATEGY.md`、长期记忆、`internal/peerlink/store.go` 三处标注，避免下一会话照旧红线做事。
4. **环境事实**：手机在 NAT 后（出网 IP 是公网地址）、容器 `adb devices` 为空 ⇒
   所有设备端改动都只能靠单测 + 真机探针验证，装机是唯一真机验证途径。
