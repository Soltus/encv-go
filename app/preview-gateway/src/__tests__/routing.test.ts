/**
 * 黑名单路由机制单元测试（2026-06-09）
 *
 * 锁住关键不变量：
 *   1. / 必须是 Vite（SPA 入口）
 *   2. /player、/tabs/* 必须是 Vite
 *   3. /@vite/、/@fs/、/src/、/node_modules/、/favicon.ico 等 Vite 资源必须走 Vite
 *   4. /openlist-ui/*、/openlist/* 必须走特殊 upstream（pathRewrite）
 *   5. 后端端点（/api/、/stream、/decrypt、/ws 等）默认走 encv-go
 *   6. Query string 不影响路由（/stream?path=xxx 等同 /stream）
 *   7. 前缀匹配要严：/streamer 不应命中 /stream
 *   8. Plugin SPA 子资源：Cookie __plugin_spa=1 或 Referer /openlist-ui/ 命中走 plugin-vite
 *
 * 运行：`pnpm exec tsx --test src/__tests__/routing.test.ts`
 */

import assert from "node:assert/strict";
import { test } from "node:test";
import {
  ENCV_GO_UPSTREAM,
  matchesPrefix,
  matchesViteDeny,
  pickUpstream,
  SPECIAL_UPSTREAMS,
  VITE_DENY,
  VITE_UPSTREAM,
  type ViteDenyRule,
} from "../routing.js";

const PLUGIN_VITE = SPECIAL_UPSTREAMS.find(u => u.match === "/openlist-ui")!;
const OPENLIST_DIRECT = SPECIAL_UPSTREAMS.find(u => u.match === "/openlist")!;

// =============================================================================
// matchesPrefix 基础测试
// =============================================================================

test("matchesPrefix: exact match", () => {
  assert.equal(matchesPrefix("/stream", "/stream"), true);
  assert.equal(matchesPrefix("/api", "/api"), true);
});

test("matchesPrefix: prefix with trailing slash", () => {
  assert.equal(matchesPrefix("/api/", "/api"), true);
  assert.equal(matchesPrefix("/api/config", "/api"), true);
  assert.equal(matchesPrefix("/api/v1/users/123", "/api"), true);
});

test("matchesPrefix: should NOT match similar prefix", () => {
  // /streamer 不应匹配 /stream
  assert.equal(matchesPrefix("/streamer", "/stream"), false);
  // /apiv2 不应匹配 /api
  assert.equal(matchesPrefix("/apiv2", "/api"), false);
  // /api-other 不应匹配 /api
  assert.equal(matchesPrefix("/api-other", "/api"), false);
});

// =============================================================================
// matchesViteDeny 规则匹配
// =============================================================================

test("matchesViteDeny: exact rule", () => {
  const exact: ViteDenyRule = { match: "/favicon.ico", mode: "exact", why: "test" };
  assert.equal(matchesViteDeny("/favicon.ico", exact), true);
  assert.equal(matchesViteDeny("/favicon.ico/", exact), false);
  assert.equal(matchesViteDeny("/favicon.icon", exact), false);
});

test("matchesViteDeny: prefix rule", () => {
  const prefix: ViteDenyRule = { match: "/@vite/", mode: "prefix", why: "test" };
  assert.equal(matchesViteDeny("/@vite/client", prefix), true);
  assert.equal(matchesViteDeny("/@vite/", prefix), true);
  assert.equal(matchesViteDeny("/@vite-other", prefix), false); // 不以 / 结尾
  assert.equal(matchesViteDeny("/@vite", prefix), false); // 不带尾斜杠
});

test("matchesViteDeny: prefix rule matches file with extension (e.g. /manifest.json)", () => {
  // 关键：/manifest 必须命中 /manifest.json、/manifest.webmanifest、/manifest/foo
  // 但不能命中 /manifest-foo（无 `/` 或 `.` 隔开）
  const manifest: ViteDenyRule = { match: "/manifest", mode: "prefix", why: "test" };
  assert.equal(matchesViteDeny("/manifest", manifest), true);
  assert.equal(matchesViteDeny("/manifest.json", manifest), true);
  assert.equal(matchesViteDeny("/manifest.webmanifest", manifest), true);
  assert.equal(matchesViteDeny("/manifest/", manifest), true);
  assert.equal(matchesViteDeny("/manifest/foo", manifest), true);
  assert.equal(matchesViteDeny("/manifest-foo", manifest), false); // 不应匹配
});

// =============================================================================
// pickUpstream - SPA HTML 路由（必须走 Vite）
// =============================================================================

test("pickUpstream: / (root) → Vite", () => {
  assert.equal(pickUpstream("/", undefined, undefined).name, VITE_UPSTREAM.name);
});

test("pickUpstream: /player → Vite", () => {
  assert.equal(pickUpstream("/player", undefined, undefined).name, VITE_UPSTREAM.name);
  assert.equal(pickUpstream("/player?path=/test", undefined, undefined).name, VITE_UPSTREAM.name);
});

test("pickUpstream: /tabs/* → Vite", () => {
  assert.equal(pickUpstream("/tabs", undefined, undefined).name, VITE_UPSTREAM.name);
  assert.equal(pickUpstream("/tabs/", undefined, undefined).name, VITE_UPSTREAM.name);
  assert.equal(pickUpstream("/tabs/home", undefined, undefined).name, VITE_UPSTREAM.name);
  assert.equal(pickUpstream("/tabs/files", undefined, undefined).name, VITE_UPSTREAM.name);
  assert.equal(pickUpstream("/tabs/settings/server/http", undefined, undefined).name, VITE_UPSTREAM.name);
  assert.equal(pickUpstream("/tabs/devtools/prototype/abc", undefined, undefined).name, VITE_UPSTREAM.name);
});

// =============================================================================
// pickUpstream - Vite dev artifacts（必须走 Vite）
// =============================================================================

test("pickUpstream: Vite dev artifacts → Vite", () => {
  const vitePaths = [
    "/@vite/client",
    "/@fs/workspace/foo.ts",
    "/@id/virtual",
    "/@react-refresh",
    "/@client",
    "/src/main.ts",
    "/src/views/ArtPlayerView.vue",
    "/node_modules/.vite/deps/vue.js",
    "/node_modules/foo/bar.js",
    "/favicon.ico",
    "/sw.js",
    "/manifest.json",
    "/manifest.webmanifest",
    "/assets/index.js",
    "/public/logo.png",
  ];
  for (const p of vitePaths) {
    const up = pickUpstream(p, undefined, undefined);
    assert.equal(up.name, VITE_UPSTREAM.name, `path ${p} should go to Vite, got ${up.name}`);
  }
});

// =============================================================================
// pickUpstream - 特殊 upstream（plugin-vite, openlist direct）
// =============================================================================

test("pickUpstream: /openlist-ui/* → plugin-vite", () => {
  assert.equal(pickUpstream("/openlist-ui/", undefined, undefined).name, PLUGIN_VITE.name);
  assert.equal(pickUpstream("/openlist-ui/src/main.ts", undefined, undefined).name, PLUGIN_VITE.name);
  assert.equal(pickUpstream("/openlist-ui/@vite/client", undefined, undefined).name, PLUGIN_VITE.name);
});

test("pickUpstream: /openlist/* → openlist-direct (with pathRewrite)", () => {
  const up = pickUpstream("/openlist/foo", undefined, undefined);
  assert.equal(up.name, OPENLIST_DIRECT.name);
  // pathRewrite 必须存在且能把 /openlist/foo 剥成 /foo
  assert.ok(up.pathRewrite, "openlist-direct should have pathRewrite");
  assert.equal(up.pathRewrite("/openlist"), "/");
  assert.equal(up.pathRewrite("/openlist/foo"), "/foo");
  assert.equal(up.pathRewrite("/openlist/sites/123"), "/sites/123");
});

test("pickUpstream: /openlist-ui 子资源 Cookie/Referer 兜底 → plugin-vite", () => {
  // 没有显式 /openlist-ui 前缀，但带 __plugin_spa=1 cookie → 兜底
  const up1 = pickUpstream("/src/main.ts", "https://x.com/whatever", "__plugin_spa=1; foo=bar");
  assert.equal(up1.name, PLUGIN_VITE.name);

  // Referer 含 /openlist-ui/ → 兜底
  const up2 = pickUpstream("/src/main.ts", "https://x.com/openlist-ui/main", "foo=bar");
  assert.equal(up2.name, PLUGIN_VITE.name);
});

// =============================================================================
// pickUpstream - 后端端点（默认走 encv-go）
// =============================================================================

test("pickUpstream: 后端端点默认走 encv-go（无需手动添加）", () => {
  const backendPaths = [
    "/api/config",
    "/api/service-guard",
    "/api/container-version/supported",
    "/api/capacitor/config",
    "/api/v1/users",
    "/agent-api/api/models",
    "/agent-api/api/chat",
    "/agent-api/api/confirm",
    "/agent-api/api/resume",
    "/agent-api/test",
    "/preview/text",
    "/preview/text?file=/test.txt",
    "/preview/image",
    "/stream?path=/01-plain-media/sample.mp4",
    "/decrypt?path=/01-plain-media/sample.mp4",
    "/play",
    "/p/",
    "/p/abc",
    "/ws",
    "/health",
    "/ping",
    "/api/stream/external?path=/test",
    // 注意：/openlist/* 由 preview-gateway 透传到 OpenList fork (:5244)
    // 不属于 encv-go 后端端点（encv-go 内部只有 /openlist/local/status、/openlist/sites 端点）
    // → 走 openlist-direct upstream 走 :5244（真正的 OpenList 实例）
  ];
  for (const p of backendPaths) {
    const up = pickUpstream(p, undefined, undefined);
    assert.equal(up.name, ENCV_GO_UPSTREAM.name, `path ${p} should go to encv-go, got ${up.name}`);
  }
});

// =============================================================================
// pickUpstream - 边缘 case
// =============================================================================

test("pickUpstream: query string 不影响路由", () => {
  // /stream?path=xxx 必须命中后端（不会因为 ? 被误判为「不是 /stream」）
  assert.equal(pickUpstream("/stream?path=%252Ftest", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
  // /api/foo?bar=1 → 后端
  assert.equal(pickUpstream("/api/foo?bar=1", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
  // /tabs/files?x=1 → Vite
  assert.equal(pickUpstream("/tabs/files?x=1", undefined, undefined).name, VITE_UPSTREAM.name);
  // /@vite/client?v=123 → Vite
  assert.equal(pickUpstream("/@vite/client?v=123", undefined, undefined).name, VITE_UPSTREAM.name);
});

test("pickUpstream: 相似前缀不会误匹配", () => {
  // /streamer 不是 /stream
  assert.equal(pickUpstream("/streamer", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
  // /apitest 不是 /api
  assert.equal(pickUpstream("/apitest", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
  // /tabsfoo 不是 /tabs
  assert.equal(pickUpstream("/tabsfoo", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
});

test("pickUpstream: 未知 URL 默认走 encv-go（denylist 行为）", () => {
  // 未知 URL（如 /foo-bar-baz）走 encv-go → 拿 404
  // 这是 denylist 的明确权衡：宁可让 encv-go 返 404（明确错误），也不要错配
  // 让 Vite SPA-fallback 把后端调用变成 HTML。
  assert.equal(pickUpstream("/foo-bar-baz", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
  assert.equal(pickUpstream("/random-unknown-route", undefined, undefined).name, ENCV_GO_UPSTREAM.name);
});

test("pickUpstream: 空 url → Vite（保守）", () => {
  // 极端防御：req.url 为 undefined 时默认走 Vite，不会白屏
  assert.equal(pickUpstream(undefined, undefined, undefined).name, VITE_UPSTREAM.name);
});

// =============================================================================
// VITE_DENY 完整性测试
// =============================================================================

test("VITE_DENY: 至少包含核心 SPA 路径", () => {
  const matches = VITE_DENY.map(r => r.match);
  // SPA HTML
  assert.ok(matches.includes("/"), "must include / (root SPA)");
  assert.ok(matches.includes("/player"), "must include /player (ArtPlayerView)");
  assert.ok(matches.includes("/tabs"), "must include /tabs (Tabs SPA)");
  // Vite dev artifacts
  assert.ok(matches.includes("/@vite/"), "must include /@vite/");
  assert.ok(matches.includes("/src/"), "must include /src/");
  assert.ok(matches.includes("/node_modules/"), "must include /node_modules/");
  // 静态资源
  assert.ok(matches.includes("/favicon.ico"), "must include /favicon.ico");
});

test("VITE_DENY: 每条规则都有 why 注释", () => {
  for (const rule of VITE_DENY) {
    assert.ok(rule.why && rule.why.length > 0, `rule ${rule.match} should have a 'why' comment`);
  }
});

// =============================================================================
// 关键 regression 测试：未来 encv-go 加新端点时不会漏配
// =============================================================================

test("regression: 未来新加后端端点 /api/v2/new-endpoint 自动走 encv-go（无需改 gateway）", () => {
  // 这条用例锁住「denylist 优势」：未来 encv-go 新增任何后端端点，无需在 gateway
  // 配置任何东西，默认就会命中。
  const newEndpoints = [
    "/api/v2/new-endpoint",
    "/api/v3/ai/something",
    "/api/internal/secret-tool",
    "/stream/segmented", // 即使 encv-go 加子路径
    "/custom-route", // 完全未知的未来端点
  ];
  for (const p of newEndpoints) {
    const up = pickUpstream(p, undefined, undefined);
    assert.equal(up.name, ENCV_GO_UPSTREAM.name, `新端点 ${p} 必须自动走 encv-go`);
  }
});
