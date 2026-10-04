# 交接文档：云控热更新 + 互联重启自恢复

> 交接时间：2026-10-05 02:10 (+08:00)
> 交接原因：原 agent 在本轮反复编辑文件时多次写坏文件（重复片段、括号不匹配），
>          并且误用管道后的 `$?`（取到的是 `head` 的退出码）把持续的编译错误
>          误判成「编译通过」，据此向用户汇报了假进展。为避免继续制造风险，
>          现将全部上下文固化，交由新 agent 接手。
>
> 阅读顺序建议：§2（现状地基）→ §4（本次事故）→ §5（待办三件事）→ §6（坑与纪律）。

---

## 1. 环境与运行现状（交接时实测）

| 项 | 值 |
|---|---|
| 仓库根 | `/workspace` |
| 当前分支 | `v2` |
| HEAD | `ba05c124` |
| 工作区 | **干净**（无任何未提交改动，本轮所有坏改动已回退） |
| 后端 | `http://127.0.0.1:2025` → `/ping` = **200 OK** |
| 前端 | `http://127.0.0.1:8100` → `/` = **200 OK** |
| 后端日志 | `/tmp/encv-backend.log` |
| 前端日志 | `/tmp/encv-vite.log` |

启动方式（**已验证可用**）：

```bash
# 后端（mobile preview overlay）
cd /workspace && ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start
# 或：make dev-mobile

# 前端（必须显式 ENCV_STANDALONE_VITE=1，否则 dev-start-guard 会拦截裸 vite）
cd /workspace/app/encv-mobile && ENCV_STANDALONE_VITE=1 ./node_modules/.bin/vite --port 8100 --host 0.0.0.0
```

⚠️ 拓扑前提（决定一切设计）：
- 桌面端（web）跑在 **cnb 公网服务器**，与 Go 后端同源；
- 安卓端在 **NAT/CGNAT 之后，没有公网可达地址** ⇒ 跨端**一律**走
  「Hub（cnb 上本进程）+ 手机主动出网的长连接」，不存在 LAN 直连。
- 本环境 `adb devices` 为空、无 USB ⇒ **设备端（执行端）代码无法真机验证**。

## 2. 已有基础（动手前务必先确认，别重新发明）

| 已有能力 | 代码落点 |
|---|---|
| 资源包整包替换**不换 APK** | `internal/server/preview_assets.go`、`PreviewAssetsActivity.kt` |
| 云端构建并发布 zip | `.github/workflows/preview-assets.yml`（release tag `preview-assets`） |
| Hub ↔ Edge 长连接 RPC | `internal/peerlink/{edge,rpc,conn}.go`、`peerCalls` |
| Go 二进制由 Kotlin 拉起 | `EncvGoService.findExecutableBinary()` |

## 3. 已完成（已提交，本地分支 `v2`，未推送）

| 提交 | 内容 |
|---|---|
| `c9efc3f6` | 远程调用**结果契约统一** + **I1 互联持久化/重启自恢复** + **I2 云控通道+安装器** + 诊断工具 |
| `a07ce263` | 真机首测抓出的两个缺陷修复：① 清单按**字典序**选版本（v0.0.9 压过 v0.0.10）② 执行端 `OnBundleUpdate` **忘了传给 EdgeOptions**（真机一律 `not_supported`） |
| `2664326e` | **I3 Go 二进制热更新**：Kotlin `findExecutableBinary()` 改为 `<filesDir>/encv-go` 优先（需 `.version` + `.abi` sidecar，`.abi` 须等于 `Build.SUPPORTED_ABIS[0]`）；启动失败自动 rename 成 `.bad-<ts>` 并回退；Go 侧 `bundle.ApplyFile`（staging→sha256→备份→原子 rename→chmod 755→写 sidecar）；`RollbackFile` |
| `ba05c124` | **I4 主应用 SPA 热更新**：`MainActivity.applyHotWebBundleIfPresent()` 用 `bridge.setServerBasePath(dir)` 让 Capacitor 本地服务指向 `<filesDir>/.encv/web-bundle`；清单改**语义版本**比较 |

### 关键机制说明（I4 为什么这么做）

`bridge.setServerBasePath(path)` → Capacitor `WebViewLocalServer.hostFiles(path)`：
把**任意应用私有目录**托管在**同一个 `https://localhost` 源**下。

- ✅ origin 不变 ⇒ CORS / `baseUrl.ts` 判定 / 混合内容策略全不变；
  Capacitor 插件桥（GoProcess / BarcodeScanner / Filesystem…）照旧工作。
- ❌ **不要**用 `webView.loadUrl("http://127.0.0.1:<port>/web/")`：会换 origin ⇒
  插件桥注入与 `WEBVIEW_SERVER_URL` 都绑本地源 ⇒ **主应用插件全部失效**。
  （`PreviewAssetsActivity` 能用独立 WebView，只因为那一页不需要插件。）

## 4. 本次事故（真机，已取证）

**现象**：热更后重启后端 → 手机连不上；清 app 缓存重启 APP → 无法连接服务器。
报错：`edge/pair failed: 502`，`detail: "pair_rejected:401"`，`error: "pair_failed"`。

**日志证据**：
```
peerlink pair rejected  pairingId=43f9f0d3…  reason=peerlink: ticket not found (consumed or unknown)
[GIN] 401 | POST /api/peerlink/pair
```
（同一 pairingId 在 02:16:26、02:18:05 各失败一次）

**根因**：配对**票据（ticket / pairingId）只存进程内存**，后端一重启即清空；
手机仍拿旧票据重试 ⇒ 401 ⇒ 前端包成 502。

**本质**：I1 把「会话」持久化了，但「票据」没跟进 —— 设计上的不一致。

## 5. 待办（用户明确的顺序）—— **三件均已于 2026-10-05 落地**

> 落地细节与回归锁见 `docs/cloud-hot-update-and-link-recovery.md` 的 **I6 / I7 / I8** 三节。
> 三件都做了**先红后绿**（含真实 DOM 的 UI 契约锁），但**真机复验仍需引导版 APK**
> （设备端 Go/Kotlin 改动装机才生效；本环境 `adb devices` 为空）。

### ① 修复「连不上」 ✅（→ I7）
方向：让票据与会话一起落盘（同一目录、0600、原子写），重启后票据仍有效；
同时对「不存在 / 已过期 / 已使用」给出**分开**的错误码，前端提示「请刷新二维码」。

⚠️ 当前 `redeem()` 把「不存在」和「已使用」**混成一个 sentinel**（`ErrTicketUsed`：
`ticket not found (consumed or unknown)`），所以日志里无法区分 —— 排障时要先拆开。

### ② 热更记录与回滚（含桌面端 agent + 移动端 UI） ✅（→ I6）
- **记录**：`globalBundleReports` 目前是**纯内存**台账，重启即丢（实测重启后
  `reportCount` 从 1 变 0）⇒ 需落盘（可复用 `internal/peerlink/store.go` 的
  原子写纪律）。
- **回滚**：目前只有设备端 `RollbackFile`；**没有云控一键回滚接口**。
- 桌面端 agent：需要能查看热更历史 / 触发回滚。
- 移动端 UI：需要展示当前热更版本 / 回滚入口。

### ③ 设备指纹 ✅（→ I8）

**现状**：deviceId 每次安装重新生成随机 UUID（Web 端 localStorage / Go 端
每次进程启动的 `peerHub` peerId）⇒ 同一台安卓机重装 / 清缓存后重新配对，
Hub 侧就**多一条 peer 记录**。

真机实测同机遗留两条：
`ac5e1c75ede3`（pairedAt 00:48）与 `6454ab90716f`（pairedAt 02:04）。

**设计要点**：
- 优先用 Kotlin 注入的 `ANDROID_ID`（`Settings.Secure.ANDROID_ID`）：跨重装/清缓存稳定，
  仅在恢复出厂 / 换签名时变；注入方式走已有的 `ProcessBuilder.environment()`
  （与 `ENCV_APP_FILES_DIR` 同一处）。
- 兜底：落盘持久随机 UUID（`<AppDataDir("peerlink")>/device.json`，首次生成后固定）。
- ⚠️ 指纹只用于「识别同一台设备」，**不是密钥**、不承担鉴权；
  鉴权仍靠票据派生的 token 与 AEAD 密钥。

## 6. 环境事实与已踩的坑（务必先看，别重复踩）

1. **`adb devices` 为空、无 USB**，手机在 **NAT/CGNAT 之后**（后端日志里来源 IP 是公网
   `36.113.x`）⇒ **设备端（执行端二进制 / Kotlin）改动无法在本环境真机验证**，
   只能靠 Go 单测 + 真机探针；装机是唯一真机验证途径。
2. **Hub 侧改动要重启后端才生效**，而 token / psk / 信任态**全在进程内存**
   ⇒ **重启 = 手机必须重新扫码配对**（这正是本次事故的来源）。
   注：I1 已把「会话」落盘，重启后手机带旧 token 可自动恢复；
   **票据本身仍在内存** ⇒ 重新配对需要**重新出码**。
3. **杀后端进程别用 `pgrep -f exe/encv` / `lsof -ti :2025`**：
   - `lsof -ti :2025` 会把**持有到 :2025 连接**的 vite 也匹配进去（已误杀过一次）；
   - 端口上真正跑的进程要用 `/api/runtime` 的 `pid` + `started_at` 核对。
4. **容器里没有 `zip` 命令** ⇒ 造测试包用 `python3 -c "import zipfile"`。
5. **Hub 跑在 `ENCV_MOBILE=1`（make dev-mobile）下 ⇒ `config.AppDataDir()` 派生成安卓路径**
   （`/data/user/0/com.encvgo.app/files/.encv/...`），云侧包仓库会落在不存在的目录
   ⇒ 启动 Hub 时必须用 `ENCV_BUNDLES_DIR=<真实目录>` 显式覆盖。
6. **编译验证必须这样写**（否则会把错误误判成通过）：
   ```bash
   go build ./internal/... > /tmp/b.log 2>&1; echo "status=$?"; head -5 /tmp/b.log
   ```
   不能写 `go build ./internal/... | head` 之后 `echo $?`（取到的是 `head` 的退出码）。

## 7. 迭代台账位置

- 主台账：`docs/cloud-hot-update-and-link-recovery.md`
  - I1 互联持久化+重启自恢复 ✅（真机两次验证通过）
  - I2 云控热更新通道+安装器 ✅（端到端真机通过过一次）
  - I3 Go 二进制热更新 ✅（待装机真机验证）
  - I4 主应用 SPA 热更新 ✅（待装机真机验证）
  - I5 云控策略（灰度/强制/回滚） ⬜
- 本文（`docs/HANDOVER-cloud-hot-update.md`）= 交接补充。

## 8. 记忆落点

- 当日：`.codebuddy/memory/2026-10-05.md`（含事故根因）
- 长期：`.codebuddy/memory/MEMORY.md`
  - 「双端互联：票据失效事故 + 验证纪律（2026-10-05）」
  - 「远程 Agent 调用的结果契约（2026-10-04 统一，长期）」

---

## 交接结论

- 代码已回退到**干净且验证通过**的 `ba05c124`（`go build` + `go test` 全绿）。
- I1 已真机验证；I2/I3/I4 已落地但**需重建 APK 装机**才可真机验证。
- 剩余三件事（连不上 / 热更记录与回滚 / 设备指纹）尚未落地，需新 agent 按 §5 顺序推进。
