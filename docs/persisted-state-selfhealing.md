# 持久化状态「坏值自愈」专项（文档驱动）

> 立项：2026-10-05（用户指令：先写文档驱动，再实施）
> 触发：同一天连出两次真机故障，根因同构 —— 见 §1。
> 本文是**设计与验收**文档：先定目标/判据/验收，再动手；实施完把状态回填到 §5 清单。

---

## 1. 事故与抽象

### 1.1 两次故障（都是真机，都不是"功能没写对"）

| # | 现象 | 根因 |
|---|---|---|
| A | `Failed to load config: proxy fetch failed: Failed to connect to /127.0.0.1:16666`，杀后台/重启都无效 | `getApiBaseUrl()` prod 分支 `if (stored) return stored` **无条件信任** localStorage；`:16666` 是沙箱 preview-gateway 端口，真机上不存在 |
| B | 手机"连不上"但 Hub 侧**零请求**；UI 仍显示"已连接到会合点 X"/"连接中…" | 扫码记住的会合点地址（edge-session.json）随域名变更而失效，设备无限重试，既不清除也不告知 |
| C | 回滚 web 包无效，仍然 `:16666`，且 WS 打 `wss://localhost/ws` | **dev 构建**（`import.meta.env.DEV=true`，APK 内置资源常是 debug 构建）跑在原生壳里 ⇒ `getApiBaseUrl()` 走 **dev 分支**，stored 空时直接 fallback `DEV_SANDBOX_ENTRY(:16666)`；`getWebSocketUrl()` 的 dev 分支还用 `location.host`。**第一次修复只改了 prod 分支**，这条路仍在漏 |

### 1.2 通用模式（本文要消灭的东西）

> **持久化的值被无条件信任 + 失效后无自愈 + 失效不可见**
> ⇒ 用户只能清数据/重装，日志里只有一句莫名其妙的 `connect refused`。

三要素同时成立才致命，因此**修复必须同时给三样**：

1. **校验** —— 读出来先判定"还能不能用"，不能用就当它没有；
2. **清除** —— 坏值必须被删除（否则下次启动还是它，重启永远无效）；
3. **可见** —— 日志/UI 指名道姓说是**哪个值**坏了、用户**下一步做什么**。

只做 1 不做 2 = 每次启动都踩一次；只做 1、2 不做 3 = 用户仍然不知道怎么办。

---

## 2. 适用范围

任何"跨启动存活"的值：

- 前端 `localStorage`（服务器地址、设备 id、主题、webdav 配置、任务快照、输入历史…）
- Go 侧落盘（`peers.json`、`edge-session.json`、`tickets.json`、`bundle-reports.json`、`device.json`）
- 原生侧（Kotlin 的 SharedPreferences / 文件）

## 3. 设计原则

1. **读即校验**：任何 `getItem`/`readJSON` 之后都要过一次"可用性判定"，不直接进业务逻辑。
2. **坏值即清除 + 留痕**：清除动作必须有 `console.warn` / `slog.Warn`，**把值本身与判定原因写进去**（排障时这是唯一线索）。
3. **失效要给出路**：UI 不能只说"失败"，必须给可点击的下一步（重新扫码 / 重新配对 / 重置连接）。
4. **判据保守**：宁可漏判，不可把"暂时抖动"误判成"永久失效"
   （手机没网、后端刚重启 ≠ 地址没了 ⇒ 必须靠"重试次数/持续时间"阈值区分）。
5. **不误伤既有契约**：用户手配的值（如本机后端端口 `:2031`）必须继续优先 —— 每条修复都要配**反向锁**。

---

## 4. 本轮（第二批）目标

| 编号 | 目标 | 验收（可观测） |
|---|---|---|
| **P0-3** | Hub 侧长期不在线 / **配对后从未连上**的设备要被标记并给出"建议重新配对" | `/api/peerlink/peers` 返回 `stale` + `staleReason`；桌面端互联页显示徽章与「重新配对」入口 |
| **P2** | 提供「重置连接（保留设备指纹）」入口，替代"清 APP 数据" | 一键清 server-url + 探测缓存 + Edge 会话；设备指纹 deviceId **保持不变** |
| **P1-1** | `useDeviceId` 落盘值损坏时重新生成（身份被污染会影响配对） | 坏值 ⇒ 生成新 id 并留 warn；合法值不变（反向锁） |

**非目标（本轮不做，留在 §5 待办）**：逐个加固所有 `JSON.parse(localStorage)`；票据里 `hub` 指向旧域名（新码不受影响）。

---

## 5. 同类风险总清单（实施后回填状态）

| 状态 | 项 | 位置 | 说明 |
|---|---|---|---|
| ✅ | 服务器地址被无条件信任 | `shared-components/src/api/core/baseUrl.ts` | 已加 `storedApiBaseUsable()`；4 条回归锁（含反向锁） |
| ✅ | 会合点地址失效无感知 | `internal/server/peerlink_edge_runtime.go` | `/edge/status` 吐 `stale`；UI 提示 + 遗忘重扫 |
| ✅ | 长期离线/从未连上的 peer 无引导 | `internal/peerlink/hub.go` `ListPeers` | P0-3：新增 `peerStale()`（`never_linked` / `offline_too_long`）+ 列表吐 `stale/staleReason/offlineSec`；桌面端徽章 + 「解除配对后重新扫码」引导。锁：`hub_stale_peer_test.go`（3 例 + 反向锁，红验：恒 false ⇒ 3 条红） |
| ✅ | 无"重置连接"入口 | `src/views/PeerSettings.vue` | P2：「重置连接信息（保留设备指纹）」= 清 server-url + 探测缓存（resetServerUrl）+ 忘记会合点（stopEdge），**deviceId 不动** |
| ✅ | deviceId 落盘值无校验 | `shared-components/src/composables/useDeviceId.ts` | P1-1：`isValidDeviceId()`（长度 8..200、无空白、无 JSON/引号污染）；坏值清除 + warn。锁：`useDeviceId.test.ts`（坏值清除 + 合法值不被误伤，红验：恒 true ⇒ 2 条红） |
| ⬜ | 票据里的 `hub` 指向旧域名 | `internal/peerlink/hub.go` `CreateTicket` | 新码自动带新域名，影响面小（未做） |
| ⬜ | 各 `localStorage` 的 `JSON.parse` 未兜底 | theme / webdav / workflow tasks / input history / libraries | 逐个确认 try-catch；坏值会让设置页崩 |
| ⬜ | Go 侧 `peers.json` 中 token 失效的设备 | `internal/peerlink/store.go` | 已由 P0-3 的"长期离线"间接覆盖 |

---

## 6. 验收方式（硬性）

1. **先红后绿**：每条修复都要有能先失败的回归锁，并在提交信息里写清红验结果。
2. **门禁**：Go `go test -short`（server 包）+ `vue-tsc`（shared + encv-mobile）+ `vitest` 全绿。
3. **真机**：云控下发到 `aa2ae6d94645`（需冷启动 APP 生效），确认：
   - 桌面端互联页对长期离线设备显示"建议重新配对"；
   - 「重置连接」点完后页面能自动找回本机后端，且设备指纹不变（`get_device_info.peerlink.peerId` 与重置前一致）。

---

## 7. 纪律（写死防跑偏）

- 报"连不上"类故障，**第一步是拿日志**，不是列可能性。
- 定位后必问三句：这个值**从哪来**？坏了**会被清吗**？UI **看得出**它坏了吗？
- 每次修完，把新发现的同类点**回写 §5**，不要只在记忆里留一句。

---

## 8. 第二轮实施记录（2026-10-05 06:2x）

| 项 | 结果 |
|---|---|
| P0-3 peer 僵死识别 | ✅ `peerStale()` + `/peers` 吐 `stale/staleReason/offlineSec`；桌面端显示「从未连上 / 长期离线」徽章与重新配对引导 |
| P2 重置连接 | ✅ `PeerSettings` 一键重置（清 server-url + 停止 Edge），**设备指纹保留** |
| P1-1 deviceId 校验 | ✅ 坏值不采用 + 清除 + warn；合法值（含历史裸 UUID）不受影响 |
| 门禁 | Go `peerlink`+`server` 全 ok（server 78s）；`vue-tsc` 双包 0 错误；`vitest` 692 全绿；i18n lint 0 问题 |
| 真机 | `web@v0.0.6-selfheal` 已云控下发（需冷启动 APP 生效） |
| 红验 | P0-3：恒 false ⇒ 3 条红；P1-1：恒 true ⇒ 2 条红 |
| 未做（留在 §5） | 逐个加固 `JSON.parse(localStorage)`；票据 `hub` 旧域名 |

---

## 9. 第三次实施记录（2026-10-05 06:3x，事故 C）

- **判据（关键取证手法）**：`getWebSocketUrl()` 的 dev 分支产出 `wss://${location.host}/ws`，
  与真机日志的 `wss://localhost/ws` **逐字一致** ⇒ 反推 `import.meta.env.DEV === true`
  ⇒ HTTP base 走的是 dev 分支的 `DEV_SANDBOX_ENTRY(:16666)`。
  ⇒ **教训：WS/HTTP 的 URL 形态本身就是"构建模式"的指纹，别只盯着一个入口。**
- **修复**：
  1. `getApiBaseUrl()` dev 分支：原生壳下**先拦截** —— 忽略沙箱端口与 WebView origin，
     落盘值仍过 `storedApiBaseUsable()`，坏值清除 + warn，最终回落 `DEFAULT_API_BASE_URL`；
  2. `getWebSocketUrl()`：dev 分支在原生壳下改用 `getApiBaseUrl()` 计算（HTTP/WS 同 base）；
  3. 热更包构建改为 `ENCV_STANDALONE_VITE=1 vite build`（注入 `VITE_ENCV_API_BASE=''`）
     ⇒ **即使产物是 dev 构建也不会走 :16666**，双保险。
- **回归锁**（`getApiBaseUrl.native.test.ts`，4 条）：
  dev+原生壳 ⇒ `:2025`；dev+原生壳 ⇒ WS 为 `ws://127.0.0.1:2025/ws`；
  dev+原生壳+落盘 16666 ⇒ 丢弃回落；**反向锁**：dev+沙箱浏览器保持同源不被误伤。
- **红验**：撤掉 dev 分支拦截 ⇒ 精确复现真机两条原文
  （`expected 'http://127.0.0.1:16666' to be 'http://127.0.0.1:2025'`、
   `expected 'wss://localhost/ws' to be 'ws://127.0.0.1:2025/ws'`）。
- **旁证**：云控 push 回报 `previousVersion: v0.0.5-stale` ⇒ 用户此前确实做过**本地回滚**
  （移动端热更区块的回滚按钮），把 v0.0.6 换成 v0.0.5 —— 这解释了"推了新包却没生效"。
- 门禁：vitest 696 全绿；vue-tsc 双包 0 错误；i18n 0 问题。
  真机：`web@v0.0.7-devshell` 已云控下发（待冷启动生效）。

### 由此新增的两条纪律
1. **修"读落盘值"时，必须把 dev / prod 两条分支都过一遍**（本次只修 prod ⇒ 漏了 dev）。
2. **热更包一律用 `ENCV_STANDALONE_VITE=1` 构建**：它对 prod 无副作用，对 dev 是救命。
