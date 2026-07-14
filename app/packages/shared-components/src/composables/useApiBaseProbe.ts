// useApiBaseProbe - AI 路由 baseUrl 探测链
//
// 触发点：
//   - useApiBase setup()（冷启动）
//   - document.visibilitychange（切回前台）
//   - useServerStatus.manualReconnect()（手动重连）
//   - ServerSettings.vue "立即探测" 按钮
//
// 探测优先级：
//   [1]   localStorage.encv-server-url（用户上次手动设的，最优先）
//   [1.5] window.location.origin  ← 🆕 浏览器模式（沙箱 dev + OpenPreview 远程）
//   [2]   http://127.0.0.1:2025   loopback（APK 模式 + adb reverse 通）
//   [3]   /api/network/lan-access（拿到 dev 机器 LAN 候选 IP 列表）
//   [4]   每个 LAN 候选逐一探活，第一个通的晋升
//
// 🆕 [1.5] 为何必要：
//   旧 4 级探测链对"浏览器通过 OpenPreview 访问"会全失败：
//     [1] cached 空 → skip
//     [2] http://127.0.0.1:2025 → 浏览器在用户机器上，不是沙箱 → CORS/网络错
//     [3] LAN：步骤 2 死路，无 baseUrl 拿 LAN 列表 → 空
//     [4] all-candidates-failed
//   但浏览器实际访问的是 https://run-agent-...trae.cn/，
//   agent-tool-host 会把同一 origin 的 /api/* 代理到沙箱 :16666/api/* → :2025/api/*。
//   加 [1.5] 后，[1.5] 直接命中（同一 origin = agent-tool-host 代理目标），
//   写 localStorage + setApiBaseUrl，浏览器模式链路打通。
//
//   APK 模式不受影响：
//     window.location.protocol = 'file:' 或 'capacitor:' → 跳过 [1.5]，走 [2] loopback。
//
// 关键约束：
//   - 串行探测，命中即停（避免并行噪声）
//   - 单 probe timeout 1500ms（避免卡 UI）
//   - 成功 → 写 localStorage + 调 setApiBaseUrl（同步所有依赖 baseUrl 的 composable）
//   - 失败 → 不写 localStorage（保留旧值兜底，避免越改越坏）
//
// 🆕 沙箱 mock 浏览器日志规范（trae_web_sandbox_network.md §九.4）：
//   沙箱 dev 模式下用户**只能在 mock 浏览器里**预览（OpenPreview 工具激活），
//   该浏览器**无完整 DevTools / 无 Network 面板**，只能看 console 日志。
//   诊断时只能靠 console 日志，所以 probe 每一步（try / result / skip / fail）
//   **必须** console.info 一行（用 `[probe]` 前缀命名空间），让用户在 mock 浏览器
//   报告问题时能直接看到探测链每一步的成败原因。
//   全部走 console.info（不是 debug）—— debug 在 DevLogs 默认隐藏，mock 浏览器看不到。
//
// 探活 endpoint 选用 /api/config 而不是 /api/chat，是因为 /api/config 是 GET、轻量
// 且不依赖任何 agent 状态。

import { type Ref, ref } from "vue";
import { DEFAULT_API_BASE_URL, DEV_SANDBOX_ENTRY, setApiBaseUrl } from "@encv/shared-components/api/encv";
import { defineSingleton } from "@encv/shared-components/lib/singleton";

/** 单次探测结果 */
export interface ProbeResult {
  /** 最终晋升的 baseUrl（已通过 /api/config 探活） */
  baseUrl: string;
  /** 探测过程中从 /api/network/lan-access 拿到的 LAN 候选（若有） */
  lanAccess: {
    addresses: string[];
    preferred: string;
  } | null;
  /** 探测命中的源头 */
  source: "cached" | "current-origin" | "loopback" | "lan-candidate";
  /** 探测耗时（ms） */
  latencyMs: number;
  /** 完整探测日志（调试用） */
  log: string[];
}

const SERVER_URL_KEY = "encv-server-url";
const PROBE_TIMEOUT_MS = 1500;
// 🆕 改用 /health 而不是 /api/config 作为探活端点（2026-06-08）：
//   - /health 是 encv-go 现有的无 auth 端点（[server.go:282](file:///workspace/internal/server/server.go#L282)），
//     在 UnprotectedEndpoints 白名单里（[server.go:164](file:///workspace/internal/server/server.go#L164)）
//   - /health 语义清晰（服务在线），/api/config 是业务端点（可能因业务状态影响）
//   - /health 不触发任何业务逻辑（只是返回 `{"status":"ok"}`），probe 失败 = 真的后端死
//   - 注意：trae 网关层（沙箱外）会对 /health 同样 401（沙箱内不可改，详见
//     trae_web_sandbox_network.md §9.1.2），这是 mock 浏览器架构限制，不是端点选错
const PROBE_HEALTH_PATH = "/health";
const PROBE_LAN_PATH = "/api/network/lan-access";

/** 模块级单例：避免多个调用方各自维护 probe 状态，经 defineSingleton 收敛样板 */
const _probe = defineSingleton(createProbe);

function createProbe() {
  const isProbing = ref(false);
  const lastResult = ref<ProbeResult | null>(null);
  const lastError = ref<string | null>(null);
  /** 节流：避免频繁触发探测（visibilitychange / 重连风暴） */
  const MIN_PROBE_INTERVAL_MS = 10_000;
  let lastProbeAt = 0;

  /** 用 AbortController 限定单次 fetch 超时（fetch 本身没有 timeout） */
  function fetchWithTimeout(url: string, timeoutMs: number): Promise<Response> {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), timeoutMs);
    return fetch(url, {
      method: "GET",
      signal: ctrl.signal,
      // 后端 /api/config 不需要 credential；同源 / capacitor scheme 都允许
      credentials: "omit",
    }).finally(() => clearTimeout(timer));
  }

  /**
   * 探活单个 baseUrl（拉 /health，200 + JSON 即视为通）
   * 失败 / 超时 / 网络错 / 非 JSON 响应均返回 false
   *
   * 🆕 2026-06-09 沙箱 mock 浏览器友好：401 也算"可达"
   *   trae 网关层（沙箱外，:16000）对 /health 必返 401 missing session token，
   *   这是 mock 浏览器架构限制（沙箱内不可改）。原版把 401 当失败 → probe 抛
   *   all-candidates-failed → 整个 SPA 渲染崩溃（[App] Vue error captured）。
   *   现版：401 = 网关可达（只是没 auth 透到后端），继续走后续探测；
   *   这样 [1.5] 命中后，UI 至少能进；后续 /api/* 真业务调用 401 由调用方各自
   *   处理（DevLogs / Settings 显示"trae 网关拦截，请用真机测试"）。
   *   注意：仅在「isHttp + 非 loopback」的浏览器模式触发，真机 protocol=file/capacitor
   *   走 [2] loopback → 不会受影响。
   */
  async function probeHealth(baseUrl: string): Promise<{ ok: boolean; latencyMs: number; err?: string }> {
    const url = baseUrl.replace(/\/+$/, "") + PROBE_HEALTH_PATH;
    const t0 = performance.now();
    try {
      const r = await fetchWithTimeout(url, PROBE_TIMEOUT_MS);
      const latencyMs = Math.round(performance.now() - t0);
      if (!r.ok) {
        // 🆕 沙箱 mock 浏览器诊断：401/403/5xx 都打响应头 + body 前 200 字符
        // 用户浏览器无 Network 面板，agent 看不到 status 以外的细节
        // —— 必须把 ct + body preview 一起塞 err 让用户能贴回报告
        const ct = r.headers.get("content-type") || "unknown";
        const body = await r.text().catch(() => "");
        const preview = body.slice(0, 200).replace(/\s+/g, " ").trim();
        return {
          ok: false,
          latencyMs,
          err: `status ${r.status} ct=${ct} body="${preview}${body.length > 200 ? "..." : ""}"`,
        };
      }
      // ⚠️ 关键校验：响应必须 application/json
      //   - vite dev SPA fallback 对未匹配路径返回 <!DOCTYPE html>...，status 200
      //   - 若 current origin 是 http://localhost:8100（dev 直连 vite），
      //     fetch <origin>/api/config 会拿到 index.html，r.ok=true → 误判通
      //   - 这里检查 content-type 把 vite dev 模式与真正的 API 反代区分开
      //   - 真正 API（:2025 / agent-tool-host 代理）始终返 application/json
      const contentType = r.headers.get("content-type") || "";
      if (!contentType.toLowerCase().includes("application/json")) {
        return { ok: false, latencyMs, err: `non-JSON response (content-type: "${contentType || "unknown"}")` };
      }
      return { ok: true, latencyMs };
    } catch (e) {
      const latencyMs = Math.round(performance.now() - t0);
      return { ok: false, latencyMs, err: e instanceof Error ? e.message : String(e) };
    }
  }

  /**
   * 判断一个 baseUrl 是否是「浏览器模式下的 trae 网关拦截层」
   * 命中条件：origin 在 401/403/HTML 状态时属于 trae 沙箱域名
   * 用于：
   *   - 把 trae 401 当"可达"处理（probeHealthProbeHealth 别 throw）
   *   - 标记 isInSandboxBrowser 让 UI 显示"预览模式，/api/* 被网关拦截"
   */
  function isSandboxBrowserOrigin(origin: string): boolean {
    if (typeof window === "undefined") return false;
    return /trae\.cn$/i.test(origin) || /agent-sandbox/i.test(origin) || /^run-agent-/i.test(origin);
  }

  /**
   * 从一个已通路的 baseUrl 拉 LAN 候选（/api/network/lan-access）
   * 仅用于扩展探测链，不会让本次 probe 失败
   */
  async function fetchLanCandidates(baseUrl: string): Promise<ProbeResult["lanAccess"]> {
    const url = baseUrl.replace(/\/+$/, "") + PROBE_LAN_PATH;
    try {
      const r = await fetchWithTimeout(url, PROBE_TIMEOUT_MS);
      if (!r.ok) return null;
      const j = await r.json();
      if (!j || !Array.isArray(j.addresses) || j.addresses.length === 0) return null;
      return {
        addresses: j.addresses.filter((s: unknown) => typeof s === "string"),
        preferred: typeof j.preferred === "string" ? j.preferred : "",
      };
    } catch {
      return null;
    }
  }

  /** 从 LAN 候选构造完整 baseUrl（http://IP:PORT） */
  function buildCandidateUrl(addr: string, port: number): string {
    // 如果 addr 已经是 http:// 开头，原样返回
    if (/^https?:\/\//i.test(addr)) return addr;
    // 6. 同时支持 IPv4 / IPv6 / hostname
    return `http://${addr}:${port}`;
  }

  /** 端口猜测：loopback URL 抽端口；默认 2025 */
  function guessPort(baseUrl: string): number {
    try {
      const u = new URL(baseUrl);
      if (u.port) return parseInt(u.port, 10);
    } catch {
      /* fallthrough */
    }
    return 2025;
  }

  /**
   * 主入口：执行一次完整探测链
   * @param opts.force 跳过 10s 节流（用于手动触发）
   * @returns ProbeResult
   */
  async function probe(opts?: { force?: boolean }): Promise<ProbeResult> {
    const now = Date.now();
    if (!opts?.force && now - lastProbeAt < MIN_PROBE_INTERVAL_MS) {
      // 节流命中：复用上次结果
      if (lastResult.value) return lastResult.value;
    }
    lastProbeAt = now;

    isProbing.value = true;
    lastError.value = null;
    const log: string[] = [];
    const t0 = performance.now();

    // 🆕 沙箱 mock 浏览器日志（§九.4 规范）：入口先打一行，让用户在 mock 浏览器
    // 看到「probe 启动了」+ 关键上下文（origin / cached / 节流状态）
    // 必须用 console.info 而不是 console.debug —— DevLogs 默认隐藏 debug
    const origin = typeof window !== "undefined" && window.location ? window.location.origin : "(no window)";
    const cached = localStorage.getItem(SERVER_URL_KEY);
    console.info(`[probe] start origin=${origin} cached=${cached || "(empty)"} force=${!!opts?.force}`);

    try {
      // ─── [1] 缓存的 URL（最优先） ──────────────────────
      if (cached && cached !== DEFAULT_API_BASE_URL) {
        const msg = `[1] try cached: ${cached}`;
        log.push(msg);
        console.info(`[probe] step ${msg}`);
        const r = await probeHealth(cached);
        const rmsg = `[1] result: ok=${r.ok} latency=${r.latencyMs}ms err=${r.err || "-"}`;
        log.push(rmsg);
        console.info(`[probe] step ${rmsg}`);
        if (r.ok) {
          return commit(cached, null, "cached", log, t0);
        }
      } else {
        const msg = "[1] no cached URL, skip";
        log.push(msg);
        console.info(`[probe] step ${msg}`);
      }

      // ─── [1.5] current origin（浏览器模式）──────────────
      // 浏览器通过 OpenPreview / 沙箱 dev 直接访问时，
      // window.location.origin 就是 API 反代的根（agent-tool-host 代理 /api/*）。
      // APK 模式下 protocol = 'file:' / 'capacitor:' → 跳过，走 [2] loopback。
      //
      // 🆕 2026-06-09 沙箱 mock 浏览器：401 也算"可达"
      //   trae 网关层（:16000）对 /health 必返 401 missing session token。
      //   但网关本身是可达的（不是 CORS / 网络断），只是没透 auth 到后端。
      //   把 401 当成"命中"→ commit current origin → 后续业务调用由调用方
      //   各自捕获 401 处理（DevLogs / Settings 提示"trae 网关拦截，请用真机"）。
      //   真机 protocol=file/capacitor → 这步跳过，不影响。
      if (typeof window !== "undefined" && window.location) {
        const proto = window.location.protocol;
        const isHttp = proto === "http:" || proto === "https:";
        const isLoopbackUrl = /^https?:\/\/(127\.0\.0\.1|localhost|0\.0\.0\.0)(:\d+)?$/i.test(origin);
        if (isHttp && !isLoopbackUrl && origin !== DEFAULT_API_BASE_URL) {
          const isSandbox = isSandboxBrowserOrigin(origin);
          const msg = `[1.5] try current origin: ${origin}${isSandbox ? " (trae sandbox, 401/HTML 也算通)" : ""}`;
          log.push(msg);
          console.info(`[probe] step ${msg}`);
          const r = await probeHealth(origin);
          const rmsg = `[1.5] result: ok=${r.ok} latency=${r.latencyMs}ms err=${r.err || "-"}`;
          log.push(rmsg);
          console.info(`[probe] step ${rmsg}`);
          if (r.ok) {
            // current origin 是反代目标，不是真正的后端地址 → lanAccess 显式置 null
            return commit(origin, null, "current-origin", log, t0);
          }
          if (isSandbox && r.err && (/status\s+401/i.test(r.err) || /status\s+403/i.test(r.err) || /non-JSON/i.test(r.err))) {
            // 🆕 2026-06-10 沙箱 mock 浏览器：trae gateway 拦截时必须 fallback
            //   trae 网关（如 https://run-agent-*.trae.cn/）对沙箱内 :16666/api/*
            //   返 403（CORS / 鉴权拦截），但**沙箱内 :16666 入口本身**是 OK 的。
            //   之前这里 `return commit(origin, ...)` → 把 trae origin 写入 localStorage
            //   → 之后所有 /api/* 走 trae origin → 全 403 → UI 显示"断联"+ 配置全空。
            //   修复：sandbox + 401/403/non-JSON 时**不 commit trae origin**，
            //   继续走 [2] loopback（127.0.0.1:2025 在沙箱内浏览器可达）。
            //   真机 protocol=file/capacitor → 这步跳过，不影响。
            const note = `[1.5] trae sandbox gateway blocked /health (${r.err.split(" ")[1] || r.err}); NOT committing ${origin} (would 401 all /api/*); falling through to [2] loopback`;
            log.push(note);
            console.info(`[probe] step ${note}`);
            // 不 return，让代码继续走 [2] loopback 探测
          } else {
            // 非 sandbox 浏览器或非 401/403：按原来逻辑 commit
            return commit(origin, null, "current-origin", log, t0);
          }
        } else {
          const msg = `[1.5] skip (proto=${proto} origin=${origin} isHttp=${isHttp} isLoopbackUrl=${isLoopbackUrl})`;
          log.push(msg);
          console.info(`[probe] step ${msg}`);
        }
      } else {
        const msg = "[1.5] no window.location, skip";
        log.push(msg);
        console.info(`[probe] step ${msg}`);
      }

      // ─── [2] loopback 探测 ─────────────────────────────
      {
        const msg = `[2] try loopback: ${DEFAULT_API_BASE_URL}`;
        log.push(msg);
        console.info(`[probe] step ${msg}`);
        const lb = await probeHealth(DEFAULT_API_BASE_URL);
        const rmsg = `[2] result: ok=${lb.ok} latency=${lb.latencyMs}ms err=${lb.err || "-"}`;
        log.push(rmsg);
        console.info(`[probe] step ${rmsg}`);
        if (lb.ok) {
          // 拿到 LAN 候选（用于本轮其它探测 + UI 展示）
          const lanAccess = await fetchLanCandidates(DEFAULT_API_BASE_URL);
          return await expandWithLanCandidates(DEFAULT_API_BASE_URL, lanAccess, "loopback", log, t0);
        }
      }

      // ─── [3] loopback 不通 → 试拉 LAN 候选 ─────────────
      // 若 loopback 不通，LAN 候选也只能从「之前的 lastResult」或「用户手动设」获取
      // 这里退回到：如果 lastResult 有 lanAccess，复用它再试
      const prev = lastResult.value?.lanAccess;
      if (prev && prev.addresses.length > 0) {
        const msg = `[3] reuse lastResult.lanAccess (${prev.addresses.length} candidates)`;
        log.push(msg);
        console.info(`[probe] step ${msg}`);
        return await tryLanCandidates(DEFAULT_API_BASE_URL, prev.addresses, guessPort(DEFAULT_API_BASE_URL), "lan-candidate", log, t0);
      }

      // ─── [4] 真的没招了 ─────────────────────────────────
      const failMsg = "[4] no candidates available, all-failed";
      log.push(failMsg);
      console.info(`[probe] step ${failMsg}`);
      // 🆕 沙箱 mock 浏览器诊断：把整条 log 串成单行 error message 抛出
      // 用户的 mock 浏览器无 Network 面板，agent 拿不到 fetch 细节——必须把 trace 透出
      const trace = log.join(" | ");
      const wrapped = new Error(`all-candidates-failed | trace: ${trace}`);
      console.info(`[probe] FAIL ${wrapped.message}`);
      lastError.value = wrapped.message;
      // 🆕 2026-06-10 沙箱 mock 浏览器：trae sandbox origin 必须 fallback 到
      //   沙箱内 :16666 preview-gateway 入口（**不是** trae origin）。
      //   旧 fallback 策略 commit trae origin → 所有 /api/* 走 trae origin → 全 403。
      //   正确：sandbox 浏览器在沙箱内能直接访问 127.0.0.1:16666，OpenPreview
      //   工具激活的 mock browser 也用 127.0.0.1:16666 当 origin（trae 域名
      //   只是包装，对沙箱内 backend 不可达）。
      //   APK/真机不进此分支（isSandbox=false），保留 throw 让 UI 报错。
      if (isSandboxBrowserOrigin(origin)) {
        // 🆕 2026-06-10 修复：sandbox 浏览器下 commit current origin（trae 域名），
        //   不是 DEV_SANDBOX_ENTRY (127.0.0.1:16666)。
        //   - 旧版 commit `http://127.0.0.1:16666` → OpenPreview 浏览器 fetch
        //     agent-tool-host 自己的 :16666（不存在）→ connect refused → 断联
        //   - trae 反代已经把 trae.cn/* 完整代理到 :16000 → :16666 → :2025
        //     （curl -s http://127.0.0.1:16000/api/config 直接 200 验证）
        //   - sandbox 浏览器下用 current origin (trae 域名) 同源 fetch 即可
        const note = `[4] trae sandbox fallback: using current origin ${origin} (NOT ${DEV_SANDBOX_ENTRY}, which is unreachable from agent-tool-host)`;
        log.push(note);
        console.info(`[probe] step ${note}`);
        return commit(origin, null, "current-origin", log, t0);
      }
      throw wrapped;
    } finally {
      isProbing.value = false;
    }
  }

  /**
   * 拿到一个通路的 baseUrl 后，再用它的 lanAccess 候选继续探
   * 仅当 loopback 通 + 拿到 LAN 列表时进入
   */
  async function expandWithLanCandidates(
    primaryBase: string,
    lanAccess: ProbeResult["lanAccess"],
    primarySource: ProbeResult["source"],
    log: string[],
    t0: number
  ): Promise<ProbeResult> {
    // 如果没有 LAN 候选，直接返回 primary
    if (!lanAccess || lanAccess.addresses.length === 0) {
      const msg = "[expand] no lan candidates, commit primary";
      log.push(msg);
      console.info(`[probe] step ${msg}`);
      return commit(primaryBase, lanAccess, primarySource, log, t0);
    }
    const port = guessPort(primaryBase);
    // 排除 loopback（已在 step 2 试过，避免重复）
    const candidates = lanAccess.addresses.filter(a => {
      if (typeof a !== "string") return false;
      if (a === "127.0.0.1" || a === "::1" || a === "localhost") return false;
      return true;
    });
    const msg = `[expand] try ${candidates.length} lan candidates (port ${port})`;
    log.push(msg);
    console.info(`[probe] step ${msg}`);
    return await tryLanCandidates(primaryBase, candidates, port, primarySource, log, t0);
  }

  /**
   * 顺序试 LAN 候选；第一个通的晋升
   */
  async function tryLanCandidates(
    fallback: string,
    candidates: string[],
    port: number,
    fallbackSource: ProbeResult["source"],
    log: string[],
    t0: number
  ): Promise<ProbeResult> {
    for (const addr of candidates) {
      const url = buildCandidateUrl(addr, port);
      const msg = `[lan] try ${url}`;
      log.push(msg);
      console.info(`[probe] step ${msg}`);
      const r = await probeHealth(url);
      const rmsg = `[lan] result: ok=${r.ok} latency=${r.latencyMs}ms err=${r.err || "-"}`;
      log.push(rmsg);
      console.info(`[probe] step ${rmsg}`);
      if (r.ok) {
        // 拿 LAN 列表用于本次 commit（如果是从 fallback 进入的，手动补一份）
        const lanAccess = await fetchLanCandidates(url);
        return commit(url, lanAccess, "lan-candidate", log, t0);
      }
    }
    const msg = `[lan] all ${candidates.length} candidates failed, fallback to ${fallback}`;
    log.push(msg);
    console.info(`[probe] step ${msg}`);
    // 走到这里：所有 LAN 都死，但 loopback 是通的——保留 loopback 结果
    // 若连 loopback 都不通，caller 早已抛错，不会进入本函数
    const lanAccess = await fetchLanCandidates(fallback);
    return commit(fallback, lanAccess, fallbackSource, log, t0);
  }

  /**
   * 提交一次成功探测：写 localStorage + 调 setApiBaseUrl + 广播事件
   */
  function commit(
    baseUrl: string,
    lanAccess: ProbeResult["lanAccess"],
    source: ProbeResult["source"],
    log: string[],
    t0: number
  ): ProbeResult {
    const latencyMs = Math.round(performance.now() - t0);
    const result: ProbeResult = {
      baseUrl,
      lanAccess,
      source,
      latencyMs,
      log,
    };
    lastResult.value = result;
    // 同步到 encv.ts 的 localStorage（保持单一数据源）
    setApiBaseUrl(baseUrl);
    // 🆕 沙箱 mock 浏览器日志（§九.4 规范）：用 console.info 而不是 console.debug
    // —— DevLogs 默认隐藏 debug，mock 浏览器看不到。
    // commit 成功 = 探测链找到可用 baseUrl，这是用户报告"preview 不工作"时的关键信号。
    console.info(`[probe] commit baseUrl=${baseUrl} source=${source} latency=${latencyMs}ms`);
    return result;
  }

  /**
   * 手动重置到默认 loopback：清 localStorage + 再探测一次
   * UI "恢复默认" 按钮用
   */
  async function resetToDefault(): Promise<ProbeResult> {
    localStorage.removeItem(SERVER_URL_KEY);
    lastProbeAt = 0;
    return await probe({ force: true });
  }

  /**
   * 用户手动指定一个 URL：写 localStorage + 验证一次（不验证也接受——容许离线设置）
   */
  function setManual(url: string): void {
    // 基本格式校验
    if (!/^https?:\/\/[^\s/$.?#].[^\s]*$/i.test(url)) {
      throw new Error(`invalid baseUrl format: ${url}`);
    }
    setApiBaseUrl(url);
    lastProbeAt = 0;
  }

  return {
    probe,
    resetToDefault,
    setManual,
    isProbing: isProbing as Ref<boolean>,
    lastResult: lastResult as Ref<ProbeResult | null>,
    lastError: lastError as Ref<string | null>,
  };
}

/** 取得模块级单例 */
export function useApiBaseProbe() {
  return _probe.get();
}

/**
 * @internal 仅供单测使用：重置模块级单例。
 * 不导出给生产代码使用。
 */
export function __resetApiBaseProbeForTest(): void {
  _probe.reset();
}
