# 长期记忆（跨会话稳定事实）

## 架构权威文档（encv-mobile / shared-components 重构）
- **权威文档：`docs/migration-task-system.md`**（状态：执行中；2026-07-12 从 `app/docs/` 迁到仓库根 `/docs/` 统一文档位置，`app/docs` 目录已删）。核心设计 = **拆分抽象与实现**：
  - `@encv/shared-components` = 纯抽象层/库，只依赖 vue/pinia/ionic/通用第三方，**不依赖** `@/config` `@/constants` `@/router` 等应用上下文。
  - `encv-mobile` = 应用层，提供共享抽象所需的**注入上下文**（base URL/认证/容器版本等，经 `registerSharedTaskServices` 注入），组合 shared 抽象 + 应用专属业务。
  - 关键原则（§0）：抽象进 shared = **重写逻辑 + 依赖注入解耦**，不是搬运文件。
- **`docs/shared-components-boundary-spec.md` 是过时稿，与权威设计直接相反**（它主张"encv 业务全搬回 encv-mobile"，会摧毁已落地的 DI 共享抽象层）。**不要执行它的 Phase 1/2/3（搬 stores/api 出 shared）**。读它前先确认是否该被废弃/改写指向 migration-task-system.md。
- 已落地事实（2026-07-10~11）：Phase 1 ✅（shared/api/core 基座 + types/task.ts 领域类型）；Phase 2 store ✅（taskStore/runTasksStore 提升进 shared/stores，DI 解耦，encv-mobile 留 re-export 垫片）；非任务 api（shared/api/encv_*.ts 非任务域）仍为"暂存残留"待 Phase 6/A-ext 清理回退；`useEventBus` shared 孤儿副本待删（保留 encv-mobile canonical）。
- Module G（通用 composables 去重）：encv-mobile 原有多份与 shared 重复的 composables。**2026-07-11 已完成 G-1/G-2**：encv-mobile/src/composables 下 useToast/useClipboard/useDateFormat/relativeTime/activeStatus（re-export 垫片）+ useSearchInput（与 shared 逐字节相同）已删除，全靠 `@/*` 别名二级回退到 shared（tsconfig `"@/*":["./src/*","../packages/shared-components/src/*"]`）。剩余 G-3 VirtualLogList.vue、G-4 useEventBus。**⚠️ useEventBus 结论反转**：encv-mobile 的 useEventBus.ts 已不存在，11 处 `@/composables/useEventBus` 引用回退到 shared 副本=事实 canonical，故**保留 shared 的 useEventBus.ts，禁止删除**（原文档"删 shared 孤儿"指令有害）。**2026-07-12 已修正迁移文档**：原文档 §5/Phase6/目录树/IN-SCOPE 表/Module G 目标等 7 处仍写"删 shared 孤儿副本、保留 encv-mobile canonical / 待 Phase 6 删除 / 应清理"，与 §8.1.1 ② 的已验证反转直接矛盾（即实验报告 gap F 文档↔代码漂移）；已统一改为"shared 副本为事实 canonical、保留、禁止删除"，文档现已自洽。
- 删除前务必事实核查：① 确认 tsconfig `@/*` 有 shared 回退；② grep 确认无相对路径引用（`from "./..."`/`"../..."`）；③ 逐文件比对与 shared 等价。本会话已验证这套方法安全。
- **⚠️ 关键坑（2026-07-11 实测）：tsconfig 的 `@/*` 二级回退 ≠ Vite/vitest 运行时解析**。Vite/vitest 默认不读 tsconfig paths。`vite.config.ts` 有 `encv-alias-fallback` 插件（本地优先+shared 次之）故 dev/build 正常；但 `vitest.config.ts` 的 `resolve.alias['@']` 仅指向本地 src、无 shared 回退，且原 `plugins:[vue()]` 无该插件 → 测试环境 `@/composables/useX` 解析不到 shared。**修复**：提取共享插件 `encv-mobile/vite-plugins/encv-alias-fallback.ts`（`enforce:'pre'`）加入 vitest.config.ts 的 `plugins`。删除本地 composables 副本后，测试文件若用相对路径导入被删模块（如 `activeStatus.test.ts` 的 `./activeStatus`）必须改 `@/` 别名。
- **⚠️ vitest `@/` 解析失败的完整根因链与正确修复（2026-07-11，四次迭代才定位）**：
  - 现象：`pnpm check:all` 的 encv-mobile 单测多 suite FAIL，`Failed to resolve import "@/composables/useX"`（本地 composables 副本已按 Module G 删除、应回退 shared；后期连本地真实存在的文件如 `renderTurnItems`/`state-machine` 也解析不了）。
  - **决定性根因（第四次才定位）**：测试跑在 `'fast'` **project** 下，而 `plugins` 只配在**根配置**。`vitest`/`vite 8` 的 **projects 模式【不继承】根配置的 `plugins`**——每个 project 是独立 Vite 配置，必须各自带 `plugins`。于是 `'fast'` project 里没有任何插件，`@/...` 无人解析 → 全部失败（与"没加插件"完全一样；连本地文件都解析不了是此根因的判定特征）。
  - 前三次是干扰项（已排除）：① 最初以为"缺 `@/` fallback 插件"→ 加了插件但没用；② 误判"vitest 打包使插件 `__dirname` 指向 config 目录"→ 改显式传 `roots`（健壮性改进，非决定性）；③ 误判"test.alias/resolve.alias 的 `'@': SRC_DIR` 抢先解析"→ 移除 `'@'` 别名、去 `enforce`（与 vite.config 对齐，仍非决定性，因为根插件压根没被 project 调用）。
  - **对照 `vite.config.ts`**：它是**单配置、无 projects**，根 `plugins` 直接生效，所以 dev/build 一直正常。vitest 用 projects 才暴露差异。
  - **正确修复**：抽 `BASE_PLUGINS = [vue(), encvAliasFallback({ roots: [SRC_DIR, SHARED_SRC] })]`，**根配置 + 每个 project（fast/isolated）都引用同一份**。`encvAliasFallback` 仍保留"调用方显式传 `roots`"（不用插件自身 `__dirname`）的健壮性；并在 `resolveId` 解析失败时 `console.error('[encv-alias-fallback] NOT FOUND', source, dirs)` 便于诊断。
  - **通用教训**：① vitest 用 projects 时，**插件/alias 必须写到每个 project 配置里**，不能只写根配置；② 给 vitest 加 `@/` 多路径 fallback 时**不要同时在 alias 设 `'@'`**；③ 插件内**不要用自身 `__dirname`** 定位项目根（vitest 打包后指向 config 目录），由调用方显式传 roots。
- 删除 Module G 本地副本后的标准复验：`cd /workspace/app && pnpm check:all`（用户在自己终端跑；encv-mobile typecheck + 单测应全绿）。

## 工具/环境注意
- `execute_command` 在工具后端偶发 10s 启动超时（报 "failed to start within 10 seconds"，连 `mv`/`echo` 都失败），与 shell 配置无关，**且是瞬时的**：单条命令超时后**直接重试原命令**即可恢复（重试 2–3 次仍失败再判定环境失联）。**严禁用 `echo`/空命令做"探测"**——探测本身也会超时、且对恢复毫无意义，纯属浪费轮次。文件编辑/读取/搜索类工具完全不受影响，可并行使用。
- **门禁命令（含 `pnpm check:all`、构建、`make-shim`）由 agent 自己跑**，不要甩给用户。长耗时命令加超时参数（如 `timeout 600 ...`），超时即重试或报告失败，不要无脑阻塞。
- **⚠️ `read_lints` 绝不是门禁（2026-07-14 用户纠正 + 13:19 实测）**：`read_lints` 只承载 **TypeScript 语言服务 (tsserver) 诊断（Source: ts）**——能抓语法/类型红线（实测：用户在 `useTasksView.ts` L23 故意写非法字符，`read_lints` 报 `[ERROR] Line 23: Invalid character. (Source: ts, Code: 1127)`）。但 **`read_lints` 不接 Biome 诊断源——全程无任何 `Source: biome`**；Biome 的 lint 规则（noAssignInExpressions/useConst/noUselessElse 等）**不会**出现在 read_lints 里（故此前那些只有 Biome warning 的文件 read_lints 恒为 0，与文件是否打开无关）。**查 Biome 只能靠 `biome ci <path>` CLI（终端可用时）或用户 VS Code 自身红线**。`read_lints` 也不跑完整 `tsc --noEmit` 工程构建 / 单测，故仍**不能当「绿」的证据**。真实门禁是 **`node scripts/check-all.mjs`**（在 `app/` 根目录跑），报告写 `app/check-report.md`，逐套件完整日志在 `app/check-logs/<slug>.log`（如 `encv-mobile-unit-tests.log` 含每个测试文件 `✓/❯` 通过行，是确认某测试「是否被收集+通过」的权威来源）。**所有重构批次一律以 check-all.mjs 真实 PASS/FAIL 为准，不得用 read_lints 冒充门禁。**
- **Biome 配置环境事实（2026-07-14）**：biome 配置已迁到仓库根 `/workspace/biome.jsonc`（原在 `app/biome.jsonc`，用户判定根目录才正确，已删除 app 那份）。真实 IDE 设置 `.vscode/settings.json` 有 `"biome.enabled": true`（VS Code 侧 Biome 已启用，用户那边应能看到内联报错/Problems）。**`.ide/settings.json` 是镜像拷贝源，不被任何 IDE 实时读取**（改它无即时作用，需部署/同步到真实位置才生效）。终端在本环境常不可用 → Biome 查错依赖用户侧 VS Code 红线，或终端可用时 `biome ci`。
- **⚠️ `app_format` MCP 在本环境不真改文件（2026-07-16 续41 实测）**：`app_format` 报 `Formatted 4 files in 6ms. No fixes applied.` 但 Biome CI 仍 FAIL（格式不符）。**必须用 `pnpm exec biome check --write <path>`（经 app_exec MCP）才真改**；`app_format` 在此环境不可信，勿再单独依赖它修格式。
- 超时命令必须加超时参数（如 curl 加 `--max-time`）。

## 动效：gsap `from()` 的「终态陷阱」+ vitest isolate:false mock 污染（2026-10-03，长期）

- **⚠️ 挂到 `<ion-page>` 上的进场动效绝不能用 `from({opacity:0})`**：Ionic 转场开始前会给页面写
  **内联 `opacity:0`**，而 `from()` 把「当前计算值」当**终态** ⇒ 动画实际 **0→0**，`y` 正常归位但
  透明度永久卡 0 ⇒ **整页空白但可点击**（DOM 在、`elementFromPoint` 能命中、rAF 正常、tween 在跑）。
  判定特征：`transform` 会归零而 `opacity` 恒 0。正确写法 =
  `fromTo(el, {y, opacity:0}, {y:0, opacity:1, clearProps:"opacity,transform"})`（终态显式写 1）。
  落地：`packages/shared-components/src/directives/motion.ts::vPageTransition`（Files/AgentChat 曾中招）。
- **vitest FAST 项目（`isolate:false`）同模块 `vi.mock` 会互相污染**：新增任何 import
  `@encv/shared-components/motion/internal` 的用例都会让 `directive-reveal.test.ts` 的引擎 mock 失效，
  实测**两文件交替假红**；**也不能在用例里调真实 `setMotionDisabled()`**（污染全局开关）。
  ⇒ 需要 mock 引擎的用例放 **ISOLATED**（`ENCV_TEST_FULL=1` 才跑）；默认门禁要跑到的锁做成
  **源码扫描**形式（不 import 引擎模块）。
- vitest 下 `import.meta.url` **不是 file: scheme**（`readFileSync` → `ERR_INVALID_URL_SCHEME`），
  测试里定位真源用 `resolve(process.cwd(), "../packages/...")`。

## 双端互联（桌面 web ⇄ 安卓）立项事实（2026-10-02 起，长期）

- **权威规划文档**：`.trae/specs/desktop-web-android-pairing/`（`spec.md` 契约 / `tasks.md` P0–P6 / `checklist.md` / `progress.md` 多轮迭代跟踪 + 恢复入口）。任何"桌面端 / 扫码配对 / 互通搜索 / 远程 Agent"相关会话**先读 `progress.md`**。
- **核心边界（写死防跑偏）**：`baseUrl` = 本端自己的后端（既有语义，不动）；`peer` = 另一台已配对设备，**peer 绝不是 baseUrl 的候选**。`useApiBaseProbe` 的探测链**不得**把 peer 改写进 baseUrl。
- **"互通搜索索引" ≠ 挂载网络驱动器**：远端命中永远是**跨端引用**（`peerId + path` + 来源徽章），只可"在线打开/取回"，**禁止**合并统一命名空间、禁止伪装成本地路径（有契约回归锁）。
- **信任语义三分**：`accept`（一次）/ `accept_for_session`（会话级，已有 `sess.GrantedTools`）/ `trust_device`（**进程级，重启即失效**，新增）。token 与信任态**只存 Go 进程内存**，禁止落 localStorage / 配置文件；破坏性工具即使已信任也强制确认。
- **连通性既有地基**：Go 后端绑 `:port`(0.0.0.0) 故 LAN 可达但零鉴权（配对层先行）；`gin_app.go` CORS 只放行 localhost/127.0.0.1/`https://*-plugin.local`（桌面直连安卓需先解决）；`ApiProxyPlugin.resolveBackendUrl()` 已支持绝对 URL，安卓→对端天然绕 CORS。
- **⚠️ 拓扑事实（2026-10-02 用户纠正，推翻初版 LAN 假设——初版"两端同局域网 → LAN 直连"设计【已废除】）**：**桌面端（web）跑在 cnb 云开发环境（公网 HTTPS，与其 Go 后端同源，经 preview-gateway :16666 转发 `/agent-api`→:2025）；安卓端在 NAT/CGNAT 后、无公网地址 ⇒ 两端不同网**。因此所有跨端设计必须走「**Hub（cnb 上 Go 内的 peerlink hub，公网 WSS）+ 手机端主动出网建长连接**」，**禁止 LAN 直连**（且 https 页面请求 http 内网地址会被浏览器按混合内容拦截）。桌面 UI 只调同源 REST/SSE ⇒ 天然规避 CORS 与混合内容。
  - 长连接**放 Go 侧**（非 WebView）：息屏 / 后台 / Activity 重建不断链；UI 只负责渲染与授权弹窗，经 127.0.0.1 与本机 Go 交互。
  - 二维码内容从"内网地址"改为"**会合点 hub + pairingId + psk**"（带外通道，不依赖同网）。
  - **手机侧接线**：`POST /api/peerlink/edge/pair`（hub+pairingId+psk）→ 配对成功后由 Go 进程常驻 Edge；`GET /edge/status`、`POST /edge/stop`。token **只存内存** ⇒ 进程重启必须重扫。Hub 地址**禁止明文 http**（仅 https 或本机回环）。
  - **最易低估的风险 R6**：`.cnb.yml` `keepAliveTimeout: 30m` ⇒ cnb 开发环境 30 分钟无心跳即被回收，Hub 会消失 / 地址漂移 ⇒ Hub 地址须持久化且**可重指向**，固定域名优先。
  - **真机级端到端闸门（2026-10-02 起）**：`bash scripts/emu-peerlink-e2e.sh`（Hub=宿主机 Go 进程，
    Edge=**模拟器内** x86_64 后端，`adb reverse` 模拟手机主动出网）。**29 断言**覆盖中继/联邦搜索/
    远端读/远程授权/信任重启失效/401。任何 peerlink 改动后都要跑它——它抓出过两个单进程测试永远抓不到的 bug。
  - **⚠️ 沙箱内起"安卓后端"的四个硬前置**：① `adb push` 别用短 timeout（首传 68MB 会被掐断）；
    ② 预建 `/data/user/0/com.encvgo.app/files`（没装 APK 时不存在 ⇒ sqlite 打不开）；
    ③ 起进程必须 `env HOME=/data/local/tmp`（adb shell 里 HOME 为空 ⇒ 应用数据落只读 `/.local`
    ⇒ tasks DB/向量搜索/FTS5 全部 `unable to open database file`）；
    ④ 端口**自选**（1999/2025/2000 都出现过）⇒ 从 `successfully started` 日志解析。
    配置还要同时设**顶层 `server.dir`**（servingDir 的真正来源）与 `mobile.server.dir`。
  - **⚠️ 两处"顺序/透传"类缺陷（已修，防复发）**：① `Server.servingDir` **必须在 `NewServer` 里就赋值**
    （`NewServer` 里 FTS5 建索引 / mount bootstrap / FTSRebuilder 已在用它；只在 `Start()` 赋值 ⇒ 空串
    ⇒ **本地全文搜索永远 0 命中** + mount root 退化成 cwd；回归锁 `TestNewServer_ServingDirReadyBeforeStart`）；
    ② 远程 Agent 的**决策必须透传**（`peerlink.AgentInvokeOutcome.Decision`）：Edge 成功分支曾硬编码
    `accept` ⇒ 调用端分不清"逐次同意"与"已信任自动放行"（回归锁
    `TestPeerlinkAgentInvoke_DecisionPropagatedToCaller`）。
  - 云端视为**不可信中转**：全链路 AEAD（HKDF(psk) 派生）+ SAS 6 位人工核对 + Hub 不落盘 + 日志脱敏；大流量（文件取回）默认不经 Hub。
- **Capacitor 桌面端（2026-10-02 调研，长期）**：官方**没有**桌面平台（只有 android/ios/web，`getPlatform()` 只返回这三值）⇒ 桌面只有两条路：① **web 形态**（浏览器/cnb，官方一等公民，插件走 web 实现或 stub）；② **Electron**（`@capacitor/capacitor-electron` → `@capawesome/capacitor-electron`，兼容 Capacitor≥6 + Electron≥28，活跃；`getPlatform()==='electron'` 且 `isNative()===true`；只有带 Electron 实现或有 web fallback 的插件可用；80–150MB；许可未确认）。社区版 `@capacitor-community/electron` 已停滞不采用。
  - **铁律**：桌面形态判定**禁止只看 `isNative()`**（Electron 桌面 isNative=true 却该走桌面壳）；必须看 `platform`（`web`/`electron` 才算桌面候选），平台名经 `appCapabilities.platform()` 注入（`Capacitor.getPlatform()`），见 `computeFormFactor` + 其 4 个单测。
  - **前提已确认**：`index` 原生就是**基于内容 hash 的增量**（`cli.mjs` `getFileHash`→`storedHash===file.hash` 则 `skipped++`，只重解析/重嵌入变更文件，`removeStaleFiles` 清理已删文件）。所以查询前重索引对未变文件很廉价（只重 hash，不重嵌入）。**不是 mtime，是内容 hash，更可靠**。watch 守护进程不采用（长驻进程按"环境保持"规矩绝不能 kill，新增风险）。
  - **端到端实测（/tmp fixture）**：新增引用文件后**不手动 index** 直接 `references` → 自动变 2 处；删该文件后直接 `references` → 自动回落 1 处。证明"新鲜度 + stale 清理"双向生效。
  - **⚠️ `references` 三大用法坑（2026-07-12 实战踩坑，"输出不对劲"根因）**：
    1. **`references <符号>` 是精确符号名匹配**（查 `imports.name` 字段），符号名错就空/漏。真实导出常与臆想不同：`useRunSummaries.ts` 导出 `useRunSummaries` **和** `useRunSummariesSingleton`（useTasksList 实际用后者）；`lib/taskTypeLabel.ts` **根本无 `taskTypeLabel` 符号**，导出的是 `getTaskTypeLabel`/`getTaskTypeMeta`/`getTaskTypeIcon` 等一族 → `references taskTypeLabel` 恒空。**波及面分析改用 `references <@/别名模块> --module` 按模块查最全**（不管导入哪个符号，只要 import 该模块就命中）。
    2. **`references <file> --file` 必须传绝对路径**（`imports.file_path` 存绝对路径），传相对路径 `composables/x.ts` → 空。用 `"$(pwd)/composables/x.ts"`。
    3. **只解析静态 import**，动态 `import()`/字符串引用查不到；`--module` 返回的行数含同文件多符号重复，去重看唯一文件。
（首次约 10 分钟；**增量重跑实测 0.65s**，`skipped 2612 unchanged`）。跨仓库完整度更好（references useTaskStore 9 条 vs 单前端库 5 条）。

## 加密容器流式写入（2026-09-29 落地，长期契约）

- **两条写入路径并存**：`writer.WriteV4Container*(V4WriteParams)` 是**整块**（要三份字节同时在内存）；
  `writer.V4StreamWriter(V4StreamParams + V4StreamSink)` 是**流式**（Go 侧只持有一个 Segment 的缓冲）。
  产物布局**逐字节同构**：流式刻意复用整块路径的 `writeOneSegment`，段布局不可能漂移。
- **为什么 Sink 不是 `io.Writer`**：容器头必须先于数据出现，但头里的 ManifestOffset/GlobalCRC32
  只有写完全部数据才知道 → 约定「数据段先 Append，最后 `Finish(head, tail)`」，
  调用方按 `head ‖ 碎块 ‖ tail` 组装。**Append 不转移所有权**，下一次会复用缓冲，sink 必须拷贝。
- **技术前提**：AES-CTR 是流密码，同一个 `cipher.Stream` 连续 `XORKeyStream` 会推进 keystream，
  所以明文可以一片一片喂；CRC32/HMAC/md5 全部增量算。**每段必换 nonce**（CTR 复用 nonce = 二时间垫）。
  **流式不支持 zstd**（seekable zstd 要随机访存整段），需压缩只能走整块路径。
- **browser/wasm 侧**：`cmd/encv-wasm-container` 导出 `encryptBegin/encryptWrite/encryptEnd/encryptAbort`（薄封装，
  不含容器逻辑）。整块路径 1GB 明文即 `fatal error: out of memory`（32 位 wasm 内存顶）；
  流式实测 1GB 正常产出。验证脚本 `app/encv-mobile/pw-enc-stream.ts`（真实浏览器）。
- **读取侧也有增量版**：`openStream(password,{size})` + `streamFeed(handle,offset,bytes)`。
  协议是**同步的「内核声明缺口 → JS 喂字节 → 重试」** ——
  ⚠️ **不能让内核回调 JS 拿 Promise**：Go/wasm 无法在导出给 JS 的同步函数里挂起等 Promise，
  实测会让 Go 程序直接退出（之后每个调用都是 `Go program has already exited`）。
  实测 1GB 容器：整块 `open` 的 JS 堆增量 1012MB，流式 `openStream` 只要 4MB。
- **⚠️ "字节还没供给"(need) 必须原样上抛，绝不能当成普通失败退回兜底路径**（同一类坑踩了两次：
  ①内核 `plainLength` 退回"读全文"；②主线 `samplePlainSizes` 退回"逐段读段头"。
  后者的后果是每次重试只多命中一个段头 → O(n²)（1GB 容器 13 万次段头读）。
  主线约定 `reader.NeedBytesError`（`IsNeedBytes()`），按需源实现它。
- **wasm 内核的入参与返回值都要先确类型**：① `syscall/js.ValueOf` 不认识自定义结构体
  （塞进 map 会在 `Value.Set` panic）；② 取 `.Int()` 前必须确认它是数字（JS 侧忘 await
  传进 Promise 就会 panic）。**一次 panic = 整个内核实例报废**，后续调用全部失效。
- **写"能写完"的测试不足以证明"流式"**：实现可以 Write 里攒全文、Close 时一次性 Append，
  产物一样对。所以 `stream_v4_test.go` 专门锁了「单次 Append 有常数上界 + 中途就有多个 Append」。
- fixture 里**必须写 `WrappedDEK`**：不写 reader 会回退「password+salt 直接派生」，
  解出**正确长度的一堆乱码**（CTR 无认证时最难发现的一类失败）。
- **⚠️ v4 有两套内容组织层，keystream 模型不同，千万别混（2026-09-29 实测）**：
  - **segment 栈**（`writer.WriteV4ContainerTo` / 流式 writer / wasm 产出）：每段随机 nonce，
    keystream 每段重置；只能由 `reader.OpenV4Container + NewSegmentSeekableReader` 读。
  - **fragment 栈**（CLI 插件路径 / `DecryptReaderFactory`）：整条逻辑流共用 KVI 里那一个 iv，
    `buildCTRStreamAtOffset(key, iv, absoluteOffset)`。
  - 曾经用 `encv decrypt-v2` 解 wasm 产物 → **长度正确 + 内容全乱码 + 不报错**。
  - **已修（2026-09-29）**：`types.Fragment.Nonce`（base64）承载每段 nonce，
    由 `container/handle.AdaptV4ToV2` 从 v4 segment 复制；读取端四条路径
    （SequentialSeekable / VirtualSeekable / BulkDecryptor / Sequential）
    在分片带 nonce 时**按分片重置 keystream**（偏移取段内偏移；无法 seek 靠 discard 时取 0）。
    分片无 nonce = 旧行为，完全向后兼容。实测 CLI 解 wasm 产物 16MB/512MB 逐字节一致。
  - `decryptReaderFactory.ensureNonceIsCarried()` 现仅做适配层自校验（nonce 没带过来才报错）；
    **早先那版"多段即拒绝"的守卫已被此方案取代**。回归锁
    `internal/v2/reader/factory_nonce_stack_test.go`。
  - 判别手法：拿**同一个容器**用 Node 加载同一份 wasm 内核开一遍（脚本
    `app/encv-preview/verify-container.mjs`），能逐字节对上就说明容器没问题、是读取栈选错了。
- 同理 mac_salt 必须显式写进 manifest：留空会让 writer 再生成一个，
  造成「加密用一个 mac_key、校验用另一个」→ 打开 EnableHMAC 就永远验不过。

## HTTP Range / 流式供给契约（2026-10-01 修真 bug 后固化，长期）

- **`FileContentProvider` 的 `GetReader()` 与 `GetSeeker()` 必须返回「共享同一个读取位置」的对象。**
  `ContentHandler.ServeFile`（`internal/v2/handler/content.go`）的顺序是
  `reader := GetReader()` → `seeker := GetSeeker()` → `seeker.Seek(start)` → `io.Copy(w, reader)`；
  两次若返回不同实例，Seek 作用在没人读的流上 ⇒ **HTTP Range 静默失效**：
  响应头写 `bytes N-M/size`，实体体却从文件头返回（206 也照样返回，全绿假象）。
  曾因 `LocalFileProvider` 内存缓存分支各 new 一个 `bytes.Reader` 而中招（小文件 ≤3MB 全命中）。
  修法即 `cachedStream()` 单例；**回归锁 `internal/v2/handler/content_range_cached_test.go`**。
- **这类契约不能用 mock 测**：旧用例 `TestServeFile_SeekableProvider_SeeksCorrectly` 把同一个
  `bytes.Reader` 同时塞给 `ReaderVal`/`SeekerVal`，恰好绕开了 bug，全绿却没覆盖到。
  ⇒ 凡「两个方法必须共享状态」的契约，测试必须用**真实对象**构造。
- 排查手法：`curl -r N-M` 拿到的字节去明文里 `find`，若命中偏移 0 就是此 bug 的特征。

## 测试清单体检（2026-10-02，长期·定期做）

- **"绿"可能只是"没跑"的假象**：`app/encv-mobile/vitest.config.ts` 曾存在
  86 条目里 **41 条指向不存在的文件**（文件已提升到 `packages/shared-components`，
  清单没跟着改）⇒ 那些用例从来没运行过，却一直在"全绿"的报表里。
- 修正后暴露两类问题：① 真 bug（`EXT_TO_CATEGORY` 缺图片扩展名 ⇒ png 归到 misc）；
  ② 孤儿用例（依赖模块已删，导入即失败）。
- 做法：**定期扫一遍清单里每个路径是否存在**（几行 python 就能做），
  修路径 → 试跑 → 失败的**挂起并注明原因**（不要静默删，也不要拖红 CI）→ 记录待修清单。

## 全局共享文件句柄（globalFileHandlePool）的契约（2026-10-02 血的教训，长期）

- `internal/v2/reader/file_handle_pool.go` 按**路径**共享同一个 `*os.File`（引用计数）。
  同一个进程里（HTTP 服务就是如此）多个请求/多条流共用它 ⇒
  **任何"取引用/还引用"不配平或重复 Close，都是并发 bug**，不是"多打一条日志"。
- 三条硬约束：
  1. 从池取的句柄**只能 `Put` 归还，绝不能直接 `Close()`**（直接关会把别人正在用的 fd 干掉）。
  2. 一次"使用"对应一次 `Get`，由使用该句柄的 reader 的 `Close` 归还；
     "容器级"引用由 `fileContainerReader.Close()` 归还 —— 两层互不混用。
  3. 所有 decryptReader / provider 的 `Close()` **必须幂等**（上层真实存在重复 Close 的调用链：
     `defer prov.Close()` + `defer decryptReader.Close()`）。
- 症状对照：`read ...: file already closed` / `416 Seek Not Supported` / **206 但实体体被截断** /
  `io.Copy` nil deref panic（provider 读出错后 `GetReader()` 返回 nil）。
  这类 bug 只在并发下显现 ⇒ 断言要**多轮**，单轮会漏。

## 容器完整性契约（2026-10-02 决策 A 落地，长期）

- 视频/主链路（`encrypt-v2`）走的是 **v4 fragment 栈**（`SingleFileContainerWriterV4`），
  **不是** `WriteV4ContainerTo` 的 segment 栈（后者才有 `EnableHMAC`）。改完整性相关行为前
  先确认自己在哪条栈上 —— 我在这上面踩过一次（按 HMAC 去找，发现那条开关跟主链路无关）。
- 现在的契约：
  1. **写入端**必须写 `DataCRC32`（v4 分片也要写，且要带进 v4 manifest 的 `data_crc32`）。
  2. **`AdaptV4ToV2`** 必须把 segment 的 CRC 带到 fragment（不能写死 0）。
  3. **读取端**在"从分片起点整片读"时边读边校，不符 → `types.ErrDataCorrupted`。
  4. `DataCRC32 == 0` = 老容器/无元数据 ⇒ **一律跳过校验**（向后兼容的开关，别乱改）。
- 校验只能"边读边算"：v4 数据区是裸密文，没有 v2 BlockHeader，
  `verifyFragmentAt` 那套用不上；也别指望"打开容器时校验"。
- ⚠️ **必须"读满整片"就判定，不能只等 `io.EOF`**：HTTP 侧
  `io.Copy(w, io.LimitReader(reader, contentLength))` 在 contentLength == 明文长度时
  读满即停，不会再调底层 ⇒ 底层 EOF 永不发生 ⇒ 大文件校验永远不触发（实测过）。
- **分块 CRC（已落地，2026-10-02）**：写入端每 64KB 落一个块 CRC
  （`Fragment.BlockCRCSize/BlockCRC32` ← `Segment_v4` ← `AdaptV4ToV2`），
  读取端每读满一块就校 ⇒ 损坏处**立即**中断（8.6MB 实测：客户端只收到 4.3MB 就被截断，
  而整片校验时是收满 8.6MB 乱码）。
- **吐字节前的预校验（已落地）**：`serveEncryptedFile` 3.5 步用
  `factory.NewRawContainerReader()`（只读**密文**，因为 CRC 是密文的）整片核对后再
  `ServeFile` ⇒ 损坏直接 **422 data_corrupted**。
  触发条件：本地容器 + 不带 Range + ≤64MB + 有 CRC 元数据（老容器/远程流自动跳过）。
  代价：正常文件多一次顺序读（不解密）。
- 两条机制各管一段：**不带 Range → 422**；**带 Range → 206 + 在损坏块处截断**（头已发出，
  只能截断，状态码改不了）。别指望后者也返回 4xx。
- **segment 栈（compose / wasm）默认开 HMAC**：`Options` 三态（EnableHMAC/DisableHMAC），
  默认开；开了 MAC 后 `AdaptV4ToV2` 必须扣掉 `MacSize`（否则明文尾部多 10 字节）；
  `WriteV4ContainerTo` 必须把 CRC 回填到 manifest（否则 fragment 栈读取路径看不到）。
  注意：**MAC 在 fragment 栈读取路径上并不被校验**，实际检出的仍是 CRC。
- 白盒断言技巧：并发 bug 若难以稳定复现，可在同包测试里直接读
  `globalFileHandlePool.holds[path].count`，断言"重复 Close 不得再减引用"（确定性红/绿）。

## Android 模拟器"真机"测试通路（2026-10-01 建立，长期）

- 无 KVM 是硬事实（QEMU TCG，约 90× 慢），**模拟器内 WebView(Chromium) 初始化必崩**
  （crashpad + `SIGTRAP` pc=0、无 tombstone）→ **UI 级真机测试不可行**；
  但 **Go 后端在模拟器里完全正常**（Android 文件/权限/mount 语义等价真机）。
  ⇒ 混合通路：后端跑模拟器 + 前端跑宿主机 Chromium + `adb forward`。
- 五件套（2026-10-02 扩到五件）：
  `scripts/emu-backend-check.sh`（核心契约，11 组含 seek 逐字节）、
  `scripts/emu-backend-edge.sh`（协议边界：416/截断/suffix/HEAD/目录/穿越/缺参/**并发多轮**）、
  `scripts/emu-backend-large.sh`（**>3MB 流式分支**：全量 + 7 偏移 seek + suffix + 并发多轮）、
  `scripts/hybrid-e2e.sh`（编排，`--only-backend` / `--full` / `--keep`，自包含）、
  `scripts/emu-smoke.sh`（APK 冒烟，当前红 = 环境限制，环境改善应自转绿）。
  权威文档：`docs/android-emulator-testing.md`。
- ⚠️ **小文件（≤3MB，内存缓存）与大文件（>3MB，流式）是两套代码路径，必须都验** ——
  2026-10-01 的 Range bug 只在小文件分支，2026-10-02 的并发截断只在流式分支。
- ⚠️ **HTTP 层 bug 要靠"并发多轮"才抓得住**：共享文件句柄引用计数被多减一次的 bug
  单轮 4 条很可能侥幸全绿（实测失败率约 25%）。新脚本的并发段都是多轮（5 轮 / 3 轮）。
- ⚠️ `curl --data-urlencode` 默认是 **POST**，`/stream` 只认 GET ⇒ 19 字节
  `404 page not found` 的**假阳性**。带 `--data-urlencode` 必须加 `-G`（HEAD 同理）。
- `emuctl`（真源 `.ide/bin/emuctl`）：`start/wait` 末尾自动 `tune`（放宽 AM 超时/关无线/关动画），
  `install` 自动 AOT（`cmd package compile -m speed -f`）。**不做 AOT 冷启动必超时**
  （实测系统 Settings 47s 超时 → AOT 后 489ms）。改完同步：`install -m 755 .ide/bin/emuctl /usr/local/bin/emuctl`。
- 前端伺服**必须是薄反代**（`/stream|/api|/preview|/themes|/ping` → 后端），纯静态会把 `/stream`
  兜底成 index.html，播放器拿到 HTML → 必然"播放失败"。

## 主应用加解密架构事实（2026-09-29 盘点，长期）

- **主应用 = CLI（`cmd/encv` → `pkg/encv`）+ Capacitor 移动应用**。后者前端是
  `app/encv-mobile`（`src/api` + `GoProcess` 插件），它启动**内嵌 Go 后端**
  `cmd/encv-mobile` → `internal/server`。改"主应用"要想到这两处。
- **加解密在主应用里本来就是流式的**：插件 `postEncryptDirect` 是
  `for { src.Read(buf) → XOR → WriteFragmentData }`（定长缓冲直写），
  解密是 `io.Copy(outputFile, decryptedReader)`。**不要给主应用重复造流式。**
- **流式 ≠ 更省内存**（实测 40MB/400MB 峰值 RSS：插件 3.2MB→27MB 随规模增长；
  流式 1MB 段 31MB→33MB 恒定）→ 交叉点在 500MB~1GB，GB 级以上流式才划算。
- **解密供给早已具备**：后端 `/stream`、`/decrypt` → `ContentHandler.ServeFile`
  支持 HTTP Range（206，按**明文**偏移）。移动端边播边解 + 拖动靠它。
  原生播放器（mpv）要的是**绝对 URL**（`getAbsoluteStreamUrl`），
  给容器文件路径 mpv 根本播不了。
- 插件未 `Initialize` 时 `p.cfg` 为 nil，取口令会**空指针 panic**；高层入口已有
  `guardInitialized` 守卫（可选接口 `Initialized() bool`）。

## 任务系统 lift 重构状态补充（2026-07-13）

- **`lib/workflow/types` 真源分歧已调和（REFACTOR_LIFT.md #16）**：app 版 417 行 vs shared 版 291 行，现已统一——shared 为唯一真源（含 `UnifiedTreeNode`/`isUnifiedTreeNode`/`TestCaseSpec`/`TestCaseResult`/`ALL_PHASES`/`isPhase`/`WORKFLOW_STORE_KEY`/`isUnifiedTimelineEntry`），app 原位为 `export * from "@encv/shared-components/lib/workflow/types"` 垫片。
  - ⚠️ **#16 调和漏搬坑（2026-07-13 修复）**：app 原版 417 行有 `isUnifiedTimelineEntry` 类型守卫函数，shared 291 行版只搬了 `UnifiedTimelineEntry` 接口 + `isUnifiedTreeNode` 守卫、独漏该函数，导致 `unified-types.test.ts` 报 `isUnifiedTimelineEntry is not a function` + `TS2724`。已补回 `packages/shared-components/src/lib/workflow/types.ts`。**教训**：调和 `lib/workflow/types` 这类「app 版比 shared 版多 N 行」的真源分歧时，必须逐符号 diff 两侧 `export` 清单，不能只搬文档里列举的「知名类型」——函数/守卫极易漏搬，且门禁（单测 + typecheck）才会暴露。
- **`Phase` 表示统一决策（长期有效）**：shared 用 **const 对象 + 联合类型**（`export const Phase = {...} as const; export type Phase = ...`），**不用 enum**。理由：grep 全仓无 `Phase[...]` 反向映射 / `Object.keys(Phase)` / `instanceof Phase` 等 enum-only 用法，`Phase.Created` 值访问 / `Record<Phase,string>` / `toPhase():Phase` 在 union 形式下完全等价，且 const-object 更 library-friendly（无 enum 运行时）。**未来提升任何用到 `Phase` 的模块，统一用 const-object 形式，勿 reintroduce enum。**
- **`TaskTimeline.vue` / `TaskDetailModal.vue` 已提升进 shared**（`shared/components/`，import 全用 `@encv/shared-components/...`），app 副本删除、`components.d.ts` 改指 shared 路径、`tasks.*` i18n key 已在 `shared/i18n/tasks.ts` 双 locale 齐备。
- **任务系统 lift 组件层已全部完成（#11–#19，2026-07-13）**：`automation/*` 9 组件 + `group-detail/PipelineTab` 已于 #19 提升进 shared（`PipelineTab` 顺带把 `tasks.pipelineEmpty` 沉入 shared i18n）。至此所有任务系统相关组件/composables/lib/api/常量/i18n 均在 shared 并留 app 垫片，shared 非测试代码 `@/`-free。**`pnpm check:all` 已 8/8 全绿（2026-07-13 收尾）**。

## 去垫片纯化（结构性改革范式 · 2026-07-13）

- **垫片是「迁移的谎言」，现已进入去垫片阶段**：模块提升进 shared 后，app 原位的
  `export * / export {…} from "@encv/shared-components/..."` 垫片只是转发壳，不是真源。
  纯化目标 = **删除全部垫片**，让 `@/` 经二级回退（tsconfig `@/*` + vite/vitest 的
  `encv-alias-fallback` 插件）**直接解析到 shared 真源**。这样 shared 是唯一事实来源、
  app 只剩组合 + DI 胶水。
- **安全机制 `scripts/make-shim.mjs prune`**（已落地）：`prune`（dry-run 列出可删同名垫片 +
  需先改 importer 的错位垫片）；`prune --apply` **只删同名垫片**，错位垫片保持不动避免静默断链。
  `check-all` 在 0 垫片时输出「✔ 无残留垫片（已纯化）」。
- **⚠️ 2026-07-14 更新：73 转发壳已全部删除，`prune --apply` 的「删后 @/x 自动落 shared 零风险」前提已失效。**
  批 9 已摘除 `encv-alias-fallback` 的 shared 兜底分支（`vite.config.ts`/`vitest.config.ts` 的 `dirs`/`roots` 仅留本地 src），
  现在 `@/x` **只解析本地**，不再回退 shared。因此**删壳前必须先把所有 `@/<壳>` 与相对路径 importer 改写为
  `@encv/shared-components/<壳>`**（本次即此顺序：改 114 处 importer 跨 47 文件 → 删 73 壳，check-all PASS 8/FAIL 0 + vite build ✓）。
  纯壳判定用结构判定（只有 `export ... from "@encv/shared-components/..."`，无 import/本地引用/其它 export），
  精确排除 `api/encv.ts`(barrel)/`i18n/index.ts`/`useAgent.ts` 等混合文件。今后若再有壳需删，沿用「先改 importer 再删壳」。
- **同名 vs 错位判定**：shim 相对路径 == shared 真源相对路径 → 同名（可删）；否则错位。
  已知错位两例（#35 已手动清理）：`api/encv_core`→shared `api/core`（importer
  `FullTextIndexDetail.vue:256` 改 `@/api/core`）、`features/alist-encrypt/useAlistEncrypt`→
  shared `composables/useAlistEncrypt`（importers `useFilesView.ts:100`/`FileInfo.vue:205` 改
  `@/composables/useAlistEncrypt`）。清理后 `grep` 两路径全仓 0 命中。
- **后续**：终端恢复后跑 `node scripts/make-shim.mjs prune --apply` + `node scripts/make-shim.mjs
  check-all` + `pnpm check:all`（8/8）。若 check:all 报「找不到导出」，说明有未预见的错位垫片，
  回到 `prune` dry-run 列出的错位项按 #35 范式改 importer 再删（删除均 git 可还原，勿盲目回滚）。
- **✅ 逻辑抽象改革（批 J / 2026-07-14）：格式化单一真源**：shared 内文件大小格式化 3 处重复
  （`api/encv_files.formatFileSize` 经 `api/encv` barrel 公开导出 + `lib/buildReportZip.formatBytes` +
  `components/TaskPerformanceSection.formatBytes`）已收敛到 **`lib/format.ts` 的 `formatBytes(bytes?)`**（1024 进制、
  B/KB/MB/GB/TB、undefined→""、<=0→"0 B"、clamp 越界、toFixed(1)）。`formatFileSize` 是其公开别名（委托，消费方无感）。
  门禁 PASS 8/FAIL 0 + vite build ✓。日期格式化（`useDateFormat.formatDateTime` vs `PerformanceTab.formatTime`）
  与 `formatDateInput`（HTML date input 契约，不可动）仍分散，收敛 `formatTime→formatDateTime` 待用户拍板（涉及显示格式统一）。
- ⚠️ **`encv-alias-fallback` 插件是路径拼接式回退**：`@/<rel>` 先试 `encv-mobile/src/<rel>` 再试
  `packages/shared-components/src/<rel>`（含 `.ts/.vue/index` 候选）。删除同名垫片后 `@/x` 直接落
  shared；但**名称错位垫片删除前必须改 importer**，否则插件拼不出 shared 真源路径会断链。
- ✅ **`add_key` 两个引号 bug + `move-key` 重复注册缺陷（2026-07-13 踩坑，已于同日修复）**：原 `scripts/i18n_lib/addkey.py` 的 `add_key` 在批量下沉 i18n key 时会破坏 shared 字典——① value 含双引号（如 `"{query}"`）时双引号包裹插入 → TS 语法破坏（`Expected ',', got '{'`）；② shared 字典 en 键是 `en: {`（无引号），正则只匹配 `"en": {` → **en 部分从不插入**（MISSING_EN 根因）。**已修复**：`insert_key_into_section` 的 locale 正则改为 `["']?{locale}["']?` 同时匹配带/不带引号；value 含双引号时改用单引号包裹（与字典现有约定一致），同时含单双引号则转义双引号。另 `movekey.py` 的 `_register_shared_module` 原幂等判断只看 `, {module}]` 字面量、且数组末元素后无 `]` 会误判 → **已修复为数组元素级匹配** `(^|,\s*){module}(\s*,|\s*\]|\s*$)`，重复注册（如 `tasks` 出现两次）不再发生。**现在可安全用 `move-key "<prefix>." --from encv-mobile --to shared --keep --register` 批量下沉 i18n key**，无需再 `cp` 整文件绕过。
- **⚠️ flat-shared + 子目录 consumer 导入坑（2026-07-13 实测修复）**：shared `components/` 是**扁平**的（无 `automation/`/`group-detail/`/`tasks/` 子目录），但 #17/#18/#19 把这几个子目录组件提升进 shared 扁平层后，**consumer 视图仍用 `@/components/<subdir>/X.vue` 旧路径**——`encv-alias-fallback` 只做「本地 src → shared 同路径」精确回退，无法把 `@/components/group-detail/X` 映射到扁平的 `shared/components/X`，导致解析失败。已修复的 6 处：`PluginTestsDetail.vue:259`（`automation/StepMiniBadge`）、`GroupDetail.vue:145-147`（`group-detail/{PerformanceTab,TasksTab,PipelineTab}`）、`Tasks.vue:617-618`（`tasks/{TaskDebugPanel,TaskVirtualList}`）——全部改写为 `@encv/shared-components/components/<FlatName>.vue`。**今后提升带子目录的组件时，必须同步改写所有 consumer 的 import 到扁平 shared 路径**，否则构建/单测静默失败（`pnpm check:all` 才暴露）。`agent/`、`developer/`、`shared/` 子目录仍在 app 或 shared 中保留，其 `@/components/<subdir>/` 导入正常。

## 项目 skill 注册约定（2026-07-15）
- 项目内 skill 真源在 .agents/skills(capawesome/ionic/skill-creator)、.trae/skills(cypress/ffmpeg)、agent/.../video-encrypt。
- 注册清单 skills-lock.json(agentskills.io 规范)的 skillPath 必须指向真实目录(.agents/skills/...)，曾错误地写 skills/ 导致失效。
- 让 CodeBuddy 识别：在 .codebuddy/skills/<name> 建符号链接到上述真源(单一真源，无复制)。**已确认 CodeBuddy 正常识别 .codebuddy/skills 下的符号链接 skill**（用户 2026-07-15 验证）。
- **skill 由 MCP 管理（scripts/skill-manager.mjs，app-dev MCP 的 9 个 app_skill_* 工具）**：多 skill 路径列表(skillPaths，默认 .codebuddy/.agents/.trae skills)持久化于 .codebuddy/skill-registry.json；路径可增删、skill 可 CRUD(add 三法 import/npx/git)、全程 fs.watch 监视(跟随符号链接、抗原子保存)。扫描/列表/增删改均走此模块，是 skill 生命周期的权威。注意：扫描时跟随符号链接，故 .codebuddy/skills/* 符号链接也能被识别(共 31 个 skill)。
- app_exec MCP 安全门禁规则位于 scripts/app-dev-guard.mjs，server 热更新(监听目录按 basename 过滤，抗原子保存)，改规则即时生效无需重启；`app_guard_reload` 工具可手动重载并返回规则数。

## ESM-only + bun 工具链约定（2026-07-15，用户强化）
- **全仓 ESM-only，禁 CJS**。`encv-mobile/package.json` 有 `"type":"module"`；脚本/配置/测试一律 ESM（`import`，`import.meta.url` 代替 `__dirname`，禁 `require`）。用户明确反对 CJS（"难道 Capacitor 不支持 esm 吗？改为 esm"）。像素比对用 `pixelmatch@7`+`pngjs@7`（ESM-only）。
- **bun 运行 .ts**：bun 可直接跑 `.ts` 且基本无感替代 nodejs。`.ide/Dockerfile` 已在 `npm i -g pnpm` 附近加 `RUN npm install -g bun`（含验收 echo bun 版本）；当前沙箱已手动装 bun 可直接用。
- **高性价比脚本已 .mjs→.ts**（顶部注释「用 bun 运行：bun <path>」）：`test-visual/compare.ts`、`pw-smoke.ts`/`pw-direct.ts`/`pw-debug.ts`、`scripts/{sync-native,check-kotlin,biome-stats}.ts`。旧 `.mjs` 已删。`package.json` scripts 改用 `bun`（`capacitor:copy:after`/`check:kotlin`/`biome:stats`/`sync:android`/新增 `pw:smoke|direct|debug`）。`appearance.visual.ts` import 改 `./compare.ts`。**新写项目脚本一律 .ts + bun 头注释，勿再用裸 node/CJS。**
- 例外：`playwright.config.mjs`、`cypress.config.mts` 保持 `.m*`（Playwright/Cypress 配置约定，仍 ESM）；`scripts/dev-start-guard.js` 等非本次范围未动。

## MCP 安全门禁 kill 策略（2026-07-15，用户优化「平衡安全与体验」）
- `scripts/app-dev-guard.mjs` 现为 **async 门禁**（`guardAppExec` 返回 Promise）。非 kill 类破坏性命令仍走同步命中即拦截列表 `APP_EXEC_DENY`（rm -rf/危险 git/dd/shred/sudo/curl|sh/写块设备/fork 炸弹），共 8 条规则。
- **kill/pkill/killall/fuser 不再一刀切拦截**，改「先检查再放行」（`evaluateKill`）：① 环境连接进程（ssh 隧道/code-server/dev MCP/session daemon/自身及祖先进程树/pid≤1）**无条件拦截**（`CRITICAL_RE` + `ancestorPids`）；② 同一身份(uid)启的服务/端口 **无条件允许释放**；③ 其他身份：已死(zombie)/`kill -0` 无响应 → 允许，仍存活则保守拦截防误杀；④ 目标 >200 个（大范围 pkill）保守拦截。
- `app-dev-mcp.mjs` 调用改 `await guardRef.guardAppExec(raw)`，`app_exec` 工具描述已同步新策略。自测 `scripts/app-dev-guard.test.mjs`（async + 含 self-pid/pid1 必拦、死目标放行等确定性用例）经 `bun` 跑 **ALL PASS（8 rules, fail=0）**。门禁热更新照旧（改文件即时生效，或 `app_guard_reload`）。

## MCP 注册单一真源（2026-07-15）
- 注册后开新对话生效（同会话内工具列表缓存需刷新），不必重启 IDE。

## web-fetch MCP（2026-07-15）
- 高级 web_fetch 替代：`scripts/web-fetch-mcp.mjs`（stdio MCP server，注册名 `web-fetch`）。能力：retry(指数退避+Retry-After)、内容嗅探(magic bytes 复核声明的 content-type)、SPA 检测+可选 headless 渲染(puppeteer/playwright)、代理(HTTP/HTTPS CONNECT 隧道，零依赖)。核心函数导出，main 守卫启动 server，可 `node -e "import('/workspace/scripts/web-fetch-mcp.mjs')"` 单测。门禁/构建类走 app-dev，网页抓取走 web-fetch。

## 前端主题重构（ENCV 共享包 / daisyUI / Ionic 桥接 · 2026-07-15）
- 权威方案文档：`/workspace/ENCV前端主题重构方案.md`（daisyUI v5 + GSAP 重塑，共享包为主；Phase 0–5）。
- **⚠️ vite 8 (rolldown) 打包 vite.config 的 ESM interop 坑（长期）**：把 `@tailwindcss/vite` 接入 encv-mobile vite 配置（`daisyUiPlugin()`，来自 `packages/shared-components/src/vite-plugins/daisy-ui.ts`）时，rolldown 把该依赖默认导出 interop 弄坏 → 构建报 `tailwindcss_vite_..._index_mjs.default is not a function`。运行时 `import('@tailwindcss/vite').default` 确为 function（node 验证过），纯属配置打包 interop。**影响**：主应用无法经 Tailwind 管线接入 daisyUI。**已采用替代**：encv-mobile 改引纯 CSS 入口 `packages/shared-components/src/styles/theme-core.css`（`@import tokens.css + palette.css + bridge.css + components.css`，不依赖 Tailwind）统一调色板；插件 web 仍用 `daisyui.css`(@plugin daisyui/theme 经 Tailwind)。**后续要让主应用用 Tailwind 工具类，须先解决此 interop**（daisy-ui.ts 做 default 兜底，或 vite 配置 external 化 @tailwindcss/vite），勿重复踩坑。
- **调色板单一来源现状**：插件走 `daisyui.css` 的 `@plugin "daisyui/theme"`(encv/encv-dark)；主应用走 `palette.css`(纯 CSS 等价，值须与前者同步)。`bridge.css` 把 Ionic `--ion-color-*` 桥接到 daisyUI `--color-*`，主应用与插件共用。
- **🔒 技术栈解耦 ACL（2026-07-15 落地，app_check_all 全绿）**：设计目标「换 gsap+daisyui，下游应用/插件零改动」。
  - 动效：gsap 收敛进唯一 `packages/shared-components/src/motion/internal/gsap-engine.ts`（全仓唯一 `import gsap`）。对外契约 `src/motion/internal/types.ts` 的 `MotionEngine` 接口（引擎无关类型，无动画库 import）。12 个 composable + guard + index 全部经 `import { motion } from "./internal"`，公共签名仅用我们的类型（无 `gsap.TweenVars`/`ScrollTrigger`/`Flip` 泄漏）。`tokens.ts` 的 `EASE` 为语义键，由引擎 `EASE_MAP` 映射。`internal/index.ts` 的 `export { motion }` 是「换库唯一开关」。
  - **noop 引擎 + 全局开关（2026-07-15 续7 加）**：`motion/internal/noop-engine.ts` 实现 `MotionEngine` 全 no-op（直接落终态），桶导出 `noopMotion`（改 `internal/index.ts` 一行即全局换 no-op）。`motion/guard.ts` 新增运行时总闸 `setMotionDisabled(bool|null)`/`getMotionDisabled()`（null=跟随系统 reduced-motion；true=强制全关；false=强制开），`getMotionProfile().enabled` 据此算。**注意与 `registry.ts` 的 `setMotionEnabled(name, enabled)`（按命名动画开关）同名冲突——全局开关必须叫 `setMotionDisabled`**，否则 TS2308。
  - 主题：稳定视觉词汇 = CSS 变量(`palette.css`) + `.encv-*` 组件/工具类(`theme/components.css`，纯 CSS、零 `@apply`、只吃令牌)；daisyui 的 `@plugin` 块是唯一切换点。`daisyui.css` 已移除泄漏 daisyui 类的 `@layer components` 块。
  - **useTheme 解耦（2026-07-15 续7）**：`applyColor` 不再手搓 Ionic `--ion-color-primary*` 的 shade/tint（原 lighter/darker 数学已删），改为只写 daisyUI 语义令牌 `--color-primary` + JS 补 `--ion-color-primary`/`-rgb`/contrast/`-contrast-rgb`；shade/tint 由 `bridge.css` 的 `color-mix(var(--color-primary)...)` 自动派生。新增语义别名 `setPrimaryColor`(=setThemeColor)。换主色时 Ionic 与 daisyUI 组件共用同一派生链。
  - 换栈步骤：(a) 新增一个实现 `MotionEngine` 的文件，改 `internal/index.ts` 一行；(b) 重写 `daisyui.css` 的 `@plugin` 块（palette.css/components.css 不动）。下游零改动。
  - **⚠️ Ionic 内滚页面：揭示/入场动效绝不可用 window-scoped 滚动触发（2026-07-16 续47 实修）**：Ionic 的 `ion-content` 在内部 shadow DOM 的 `.inner-scroll` 滚动（**非 window**）。gsap `ScrollTrigger` 默认以 window 为 scroller → 在 Ionic 页面内**永远收不到滚动**，`onEnter` 永不触发。`useScrollReveal`/`vReveal` 曾把子元素立即置 `opacity:0` 再靠 `ScrollTrigger.onEnter` 揭示 → **整页空白但可点击**（DOM 都在、可点击）。**修复**：揭示类一律改用 `IntersectionObserver`（root:null + rootMargin 近似 `start:"top 90%`，相交即揭示、`once` 断开；无 IO 环境直接落可见态兜底）。`useScrollParallax` 仍用 ScrollTrigger，在 Ionic 下无视差但**不导致空白**，暂未改。新写 Ionic 内滚相关动效优先 IO，勿重蹈 ScrollTrigger 覆辙。
  - **⚠️ 臻彩显示（Vivid / P3 宽色域）真实生效（2026-07-16 续48/续49）**：外观页「臻彩显示」曾形同虚设，三处失效已修（续48）。续49 进一步：① Vivid `contrast()/saturate()` 根级滤镜对**所有**主题/背景/自定义色全自动生效（挂在 `.encv-vivid ion-page` 上）。② **P3 宽色域不再写死 7 色**：`useTheme.applyColor` 现用 `hexToP3Token(hex)` 对任意有效 hex 自动归一化派生 `color(display-p3 r g b)`（srgb 归一化值塞入更宽基色=更艳），开关对内置色/自定义取色/远程主题色**全自动**；非法 hex `removeProperty` 回退 srgb。`P3_PRIMARY_MAP` 硬编码表已删除（其 7 个坐标本就是归一化值，冗余）。③ **`color-gamut`/`prefers-color-scheme` 是只读媒体特性、非作者属性**（续48 根因）。**仍存的真缺口**：背景/渐变（`--ion-background-color`、body 渐变，由 `applyBgColor` 独立设置）**未进 P3 交换**——P3 模式下只吃到 vivid 滤镜提色、拿不到真·宽色域；要让渐变背景也享 P3 需额外把背景色塞入 `vivid.css` 的交换（更大改动，非本次范围，待办）。契约锁见 `encv-mobile/src/motion/__tests__/vivid.test.ts`（含"任意色自动派生"+"非法色回退"两条）；文档见 THEME_DEV.md §6.17。
  - **外观重构 = 表面材质（surface material，2026-07-17 启动，THEME_DEV.md §6.18）**：用户认定动态背景(BG_PRESETS)/主题/高斯模糊(全局开关)是共存冲突的平行子系统，应统一为「一个主题=一套表面材质」（含 `--material-bg`/`-p3`/`--material-blur`/`--material-saturate`/`--material-tint`/`--material-highlight`），参考 iOS/鸿蒙液态玻璃。
    - Phase 2 ✅：`--material-blur` 由 `useTheme.applyBgBlur` 写出（`--encv-bg-blur` 留作兼容别名）；全仓约 27 处裸 `backdrop-filter: blur(<px>)` 已全量改为 `blur(var(--material-blur, <原px>))`；回归锁 `surfaceMaterial.test.ts`（无裸字面量扫描，临时 `blur(5px)` 能被抓出→红，删除→绿）。
    - Phase 3 ✅（2026-07-17）：背景并入材质令牌。`applyBgColor` 写 canonical `--material-bg`（渐变额外写 `--material-bg-p3` 孪生，停色经 `hexToP3Token` 转 `display-p3`）；`--ion-background-color` 与 `body` 统一读**中转变量 `--material-bg-active`**（`vivid.css` 定义，P3 媒体块换 `--material-bg-p3`）。**关键认知**：P3 切换必须作用在「不被 inline 设置的中转变量」上，否则 `@media` 压不住内联 `--color-primary`/`--material-bg`（§6.17 `--color-primary` 的潜在失效点）；`bridge.css` 亮/暗块默认 `--ion-background-color: var(--material-bg-active, var(--color-base-100))`。契约锁见 `surfaceMaterial.test.ts`「背景并入材质令牌」块（8 用例合计全绿）。
    - 仍待办（需真实复现）：液态玻璃 `--material-highlight` 高光描边作为默认材质变体；App.vue 内 `.service-guard-blocked`/`.root-error-fallback` 等 fallback 页仍硬编码 `var(--color-base-100)`（不随 bg 预设）；稳定后移除 `--encv-bg-blur` 兼容别名写入。
  - **⚠️ 臻彩显示（vivid / P3）实现与优化（2026-07-17，真实浏览器先红后绿）**：
    - **实测无效的根因**（已修）：① 滤镜规则 `.encv-vivid ion-page` 命中不到——CE 注册模式（`registerIonicComponents`）把 `<ion-page>` 渲染成 `<div class="ion-page">`（**TAG 是 div，不是自定义元素名**），故**所有 `<ion-*>` TAG 选择器都要改 `.ion-*` 类**；② P3 交换 `@media :root.encv-p3 { --color-primary: var(--color-primary-p3) }` 被 `applyColor` **内联**的 `--color-primary` 压过 → 加 `!important` 越过，回退**不能** `var(--color-primary)` 自引用（无效声明），改由 `applyColor` 另存非循环 `--color-primary-srgb`；③ 刷新丢失：`initTheme` 读偏好后未调 `applyVividMode`，已改调。
    - **二次优化（实测生效后的增强）**：① 滤镜由单一 `vividIntensity` 拆为 `vividSaturation`(色彩浓度)+`vividContrast`(对比度) 两个独立滑块，CSS 变量 `--encv-vivid-sat` / `--encv-vivid-contrast`(0..1)，gsap 分别过渡；② 删除 P3「自动/始终开启/关闭」冗余选项组，`initTheme` 改为始终 `applyP3Mode("auto")`(`.encv-p3` 常驻，真实 P3 屏由 `@media` 决定)；③ 明暗分调：新增暗色专属规则 `:root.encv-vivid body.dark .ion-page`——亮色 contrast*0.25/saturate*0.45，暗色 contrast*0.12(收敛防过硬)/saturate*0.72(加大更跳)。
    - **通用铁律**：CSS 变量被 JS `setProperty` 内联写在 `:root` 时，做主题/媒体交换必须 `!important`，回退用另存的基色变量（不可自引用）。vitest 用例间 module 级 ref 会泄漏，涉及持久化偏好要用正确 localStorage key 显式存值。
    - 回归锁：`test-visual/vivid-diag.visual.ts`（Playwright 真实浏览器 4 用例）+ `vividScss.test.ts`/`vivid.test.ts`；详情 `THEME_DEV.md` §6.17 / §6.17b。
- Phase 0/1/ACL/noop 引擎/useTheme 解耦已落地，`app_check_all` 全绿。Phase 3 动效试点已接通首个真实下游 `encv-mobile/src/views/ExtensionsPage.vue`（`useScrollReveal` 对 `.extensions-list` 错峰淡入，含 `ready` 异步闸门）。Phase 4=组件迁移到 .encv-*/bg-base-* + Appearance 重组；Phase 5=用户主题/Snippets 闭环。
- **Phase 4 组件迁移已批量落地（2026-07-16）**：`scripts/migrate-ion-colors.ts`(bun 跑，无参全量 `.vue/.css/.scss`，排除 `src/main.ts`)把 `--ion-color-*` **引用**映射为 daisyUI `--color-*`(rgba 半透明→`color-mix`；Ionic 数字色 name-50/100/.../900、step-* 灰度、未定义的 `--ion-color-dark` 均兜底映射)。`bridge.css` 兜底所有 `--ion-color-*` 变体(含 `-rgb`)，故裸 `-rgb` 引用安全保留。
- **⚠️ Phase 4 迁移边界（关键长期认知）**：只迁移 `var(--ion-color-*)` **引用**；Ionic web component(shadow DOM)内部**只认 `--ion-color-*`**，其令牌由 `bridge.css` 全局定义或组件局部 `body.dark .x { --ion-color-light: ... }` 提供，**不能**改成 `--color-*`(Ionic 不认)。故 `ConfigFieldItem.vue` 里给 `.task-override-badge` shadow DOM 的局部 `--ion-color-light` 调色板定义必须保留。迁移后全仓仅剩 12 文件含 `--ion-color-*`(均为 bridge 兜底的 `-rgb` 或该局部定义)，无遗漏引用。
- **Biome 生成文件排除（2026-07-16）**：`src/components.d.ts` 是 `unplugin-vue-components` 自动生成、每次 `vite build` 重生成，格式不符导致 Biome CI FAIL。已在 `/workspace/biome.jsonc` 的 `files.includes` 加 `!!**/components.d.ts` 排除(⚠️ Biome 2.5.2 不支持 `overrides.include`，必须用 `!!` 否定模式)。`app_check_all` 现 9 PASS / 0 FAIL。
- **⚠️ Phase 4「组件迁移到 `.encv-*`」真实缺口（2026-07-16 续）**：迁移 `--ion-color-*`(变量名) 不等于采用 `.encv-*` 视觉词汇。`packages/shared-components/src/theme/components.css` 定义的 `.encv-card/.encv-panel/.encv-list-item/.encv-badge/.encv-chip/.encv-input/.encv-modal/.encv-divider` 此前**从未被任何组件使用（死代码）**——全仓 `.encv-` 命中多是 App.vue 的自定义 `.encv-toast/.encv-blur-surface/.encv-force-p3`（应用专属，非共享词汇）。**接入策略（关键长期认知）**：定制/语义组件（`ServerStatusCard` 拟物 3D、`ErrorStateCard`、`MockBranchChoiceBar` 竖向 chip、`SlashMenu` Teleport 浮层命令面板(强向上阴影+内边距布局)、`FileReferenceChip` 行内等宽 4px 小圆角、`OperationCard`/`FileListCard`/`GroupedOperationMessage` 内层列表/`BlockHeader`/`PlanBlock`/`ScrollToBottomButton` FAB/`ErrorMessage`(含 `--error-bg/--error-border` 专属 token)）刻意保留专属 scoped 样式，**不能机械套 `.encv-*`**（会毁掉独特设计/改坏布局/丢状态）；只把 `.encv-*` 用于**真正朴素/标准**的原子件（chip/badge/胶囊头），且采用「共享类提供表面/边框/圆角/hover/禁用 + 仅保留尺寸/间距/pulse 等 bespoke 覆盖」模式（scoped 覆盖因 `[data-v]` 特异性天然胜出全局类）。**已落地（6 组件）**：① `components.css` 补 `.encv-chip-primary`+`.encv-chip:disabled`+`.encv-panel-tint`+`.encv-badge-success/warning/error/neutral`(含 `body.dark` 变体)；② `CollapsedMessageToggle.vue`/`MockPresetBar.vue` chip→`.encv-chip(.encv-chip-primary)`（修掉 `MockPresetBar` 硬编码旧蓝 `rgba(79,140,255,…)`）；③ `StatusBadge.vue`→`.encv-badge-success/warning/neutral/error`（删 4 段 bespoke 色调+暗色覆盖）；④ `V2QuickActions.vue` `.v2Chip`→`.encv-chip`；⑤ `FileChangeSummaryMessage.vue`/`GroupedOperationMessage.vue` 摘要头胶囊→`.encv-chip(.encv-chip-primary)`（保留 pulse/expand 覆盖）。门禁始终 9/0。**结论（2026-07-16 01:13）**：朴素原子件已迁尽，剩余 agent 组件均属 bespoke/语义件——Phase 4「组件迁移」到此为合理收口，不再机械迁移；若日后要统一某类 bespoke 外观，应**先在 `components.css` 扩展对应共享类**（如 `.encv-card-flat`/`.encv-surface-error`），再接入，而非逐组件复制。agent 组件不在 `test:visual` baseline，视觉回归需用户真机/IDE 核对。⚠️ **该「共享类词汇」策略已于 2026-07-16 后续被推翻**：用户认定"品牌前缀共享类词汇 + 改名间接层"是掩耳盗铃的解耦——仅让前缀可改名、未消除组件对共享类词汇的耦合。最终决策=**直接废除 `.encv-*`/`ui-*` 共享类词汇**，组件改以设计令牌（`--color-*`/`--radius-*`）自包含 scoped 样式（见续24）。`
- **⚠️ `.encv-*` 类名即契约、改名成本高 —— 但可消除（2026-07-16 用户纠正我的"非消除"误判）**：`.encv-*` 是组件层事实 API 契约，消费方直接写类名字面量时改名须逐消费方改、爆炸半径随接入增长。但**这不是 CSS 死局**：`@extend`/`@custom-selector` 都不行（前者只搬硬名、模板仍写死；后者仅分组不能重命名 HTML 类），真正的解法是**给消费方加一层间接**：
  - **⚠️ 用户进一步质疑（续20 之后）："纯 css 契约有必要吗，且现代 css 已支持嵌套"**——成立，已据此修订 `THEME_DEV.md` §6.2：
    - native CSS 嵌套能替代 SCSS 嵌套，但**不能**替代 SCSS 选择器插值（CSS 无 `.#{$prefix}-chip` 语法），故单靠 native CSS 仍解不了 O(1) 前缀改名。
    - 契约含两条价值不同的约束：① daisyUI 无关(禁 `@apply`)＝真价值**保留**；② 纯 CSS/禁预处理＝价值弱（仅"易换构建工具"，且 `palette.css` 本就是 daisyUI 调色板纯 CSS 镜像，未彻底独立），**应放松**。
    - 关键推论：即便用 SCSS，消费方那层间接仍不可省（SCSS 只解 CSS 端单点；模板写死 `class="encv-chip"` 则前缀改名仍要改消费方）→ 消费端仍需 `:class="encv.chip"` 映射。
    - **修订推荐 6.2.y 已落地（2026-07-16）**：放松纯 CSS 契约、装 `sass-embedded`(devDep，encv-mobile + 经 Vite 8 optional peer 自动接)；`components.css`→`components.scss`（`$encv-prefix: "encv"` 变量 + `#{$encv-prefix}-*` 插值，CSS 端零手写 `.encv-*` 字面量，全部规则体逐字节保留）；新建 `theme/encv-classes.ts`（持 `ENCV_PREFIX` 常量 + 全组件类映射 exports `encv`，→ `encv.chip`）；5 个消费组件（`V2QuickActions`/`MockPresetBar`/`GroupedOperationMessage`/`FileChangeSummaryMessage`/`CollapsedMessageToggle`）`class="encv-*"`→`:class="encv.*"`（注：`BackToMain.vue` 的 `encv-iframe` 是插件本地 one-off，不属共享词汇，未迁）。`theme-core.css`/`daisyui.css` 的 `@import` 同步指向 `components.scss`（跨扩展 @import .scss 由 Vite 编译内联，已验证）。前缀改名=改 `$encv-prefix` 一处 + `ENCV_PREFIX` 一处 = **2 单点编辑、零 N 批改**。`app_check_all` 9 PASS / 0 FAIL。**⚠️ 同期修复 `pnpm-workspace.yaml` 的 `allowBuilds['@parcel/watcher']` 占位符**（非布尔值 `"set this to true or false"` 致 `pnpm install` 报 `ERR_PNPM_IGNORED_BUILDS`、全 gate 在 install 前置即 FAIL），改 `false` 与同段 `core-js/cypress/esbuild:false` 一致 → install 恢复、gate 9/0。原方案 A′ 留作"坚持零预处理"备选。详见 `THEME_DEV.md` §6.2.y / §6.2.x。
  - **已落地**：全部消费方（现 5 个）已接入间接层，前缀/单类名改名均 O(1)。详见 `packages/shared-components/src/theme/THEME_DEV.md` §6.1(风险)/§6.2(原 A 不足+A′ 备选+**6.2.x 契约再审视**+**6.2.y 修订推荐 SCSS(已落地)**)/§6.3(**方案 C 待办**：tint token 提升至 `palette.css` 每块 `[data-theme]`，消除 `#000/#fff`+8 段重复+`body.dark` 分叉、让用户主题免费正确上色；不改类名、不引预处理、不破契约)。⚠️ 该间接层方案已于 2026-07-16 后续被推翻——见续24：用户指"前缀改名间接层"是掩耳盗铃的解耦，最终**废除 `.encv-*`/`ui-*` 共享类词汇**，组件改以设计令牌自包含 scoped 样式；§6.3 方案 C（tint 提升 palette.css）仍成立且更值得做。
  - **续25(02:38)**：用户问"没有锚点主题如何定制？"→ 澄清定制锚点=令牌层(`--color-*`/`--radius-*`，每用户主题一块 `[data-theme]` 覆盖、运行时切 data-theme 零构建热切换)；abolition 只是把锚点从"类名字典"挪到"令牌名"，没丢。但发现 abolition 时我把 6 组件的 tint 前景写成 `color-mix(... 85%, #000)`+`body.dark` 仍不随主题翻转——已改为朝 `var(--color-base-content)`/`var(--color-base-100)` 派生并删 `body.dark`，这 6 个现在真正主题自适应，**不再需要方案 C**。但搜索发现另有 **约 13 个组件**(`SlashMenu`/`PlanBlock`/`OperationCard`/`MountListCard`/`MockBranchChoiceBar`/`FileReferenceChip`/`FileListCard`/`FileContentCard`/`ApprovalCard`/`AgentTaskMessage`/`ErrorMessage`/`AgentDebugPanel`…) 仍硬编码 `85%, #000/#fff` 前景对比,不随主题翻转(暗色用户主题下黑字压暗底)——同根因,待统一 sweep。
  - **续26(02:46) 用户强调·原则**：主题**≠**换色板，是多维度（颜色 / 字体族 / 字号阶梯 / 字距 / 行高 / 间距密度 / 圆角 / 动效强度 / 结构性 variant）。令牌契约必须覆盖**全维度**；把 `padding`/`font-size`/`gap`/`transition` 时长等以字面量内联进 scoped `<style>` 会使非颜色主题化**不可达**（对 `[data-theme]` 覆盖透明），比共享类层更糟。正确做法：新增 typography + spacing/density 令牌，组件全量消费令牌，主题靠覆盖令牌定制；motion 令牌已存在须改用 `--motion-dur-*`。结构性 variant 非令牌能表达，归「主题级组件覆盖」另议。
  - **续27(02:54) 用户再纠正·核心原则（已落地）**：要达**思源笔记同级**——用户用极简选择器**任意改任意元素**，甚至更好。死穴＝①`<style scoped>` 给选择器加 `[data-v-x]`（specificity 0,2,0），用户 `.xxx{}`（0,1,0）永远赢不了；②无稳定语义全局钩子；③令牌只是受控维度。**修正 abolition 误区**：杀「品牌耦合类」（`.encv-*`）正确，但不该连「稳定语义钩子」一起杀——正确形态是角色化、**全局**、专为被覆写而生的类（`.ui-*`）。**两层契约（落地于 `theme/surface.css` + `tokens.css`）**：①全局语义「表面」类 `.ui-chip/.ui-badge(+色调)/.ui-card/.ui-panel/.ui-button/.ui-toggle/.ui-bubble/.ui-header/.ui-input`，无 scoped、消费令牌；②typography/spacing/density 令牌（`--text-*`/`--font-*`/`--space-*`/`--density`/`--pad-*`/`--gap-*`）。**为何能任意改且无需 `!important`**：surface.css 全局无 `[data-v-x]`；用户主题/片段经 `useUserThemes`/`useSnippets` **运行时** `head.appendChild` 注入、晚于打包 CSS、同 specificity 后加载胜出。易用路径（超越 SiYuan）：只换字/密度覆写令牌即一次生效。组件 scoped 只留结构/interaction，视觉一律上提 `.ui-*`。证明：`MockPresetBar` chip 挂 `ui-chip` + 样例片段 `surface-override` 可一键验证。`THEME_DEV.md` §6.5 有定位器地图。
  - **续28(03:2x) #fff/#000 清零 + re-surface 纪律（已落地）**：组件 `<style>` 块内纯白/黑字面量一律路由到 `palette.css` 新增的全局中性令牌 `--color-white:#ffffff`/`--color-black:#000000`（默认纯色=零行为变化，主题可覆写）。**不动**：基础令牌层/定义源、snippet 主题文件、`useTheme.ts` 调色板数据、`plugin-simverse/.../game/*.ts`（Phaser canvas 不吃 CSS 变量）、一次性迁移脚本；且**只改 `.vue` 的 `<style>` 块**，避开 `<template>` 的 SVG `fill` 属性与 `<script>` 字符串（那里 `var()` 不生效），负向预查防误伤 `#fff7ed`/`#000000aa`。re-surface 已扩到主聊天：`AgentTaskMessage`→`ui-panel`、`FileReferenceChip`→`ui-chip ui-chip--mono`（新增 `--mono` 变体）；`surface.css` 的 `.ui-panel` 默认值须对齐原组件外观以零视觉回退。**纪律**：组件 scoped 只能留结构/interaction/状态覆写，可主题化表面属性必须上提到全局 `.ui-*`，否则 `[data-v-x]`(0,2,0) 仍封死用户覆写。`THEME_DEV.md` §6.6 有候选清单。
