# AI 路由真机失效 — 完成收尾计划

## 1. Summary

**前置计划**：[`/workspace/.trae/documents/ai-routing-real-device-fix.md`](file:///workspace/.trae/documents/ai-routing-real-device-fix.md) 已获批准，其中 S1–S8 已完成（探测链 composable、`useServerStatus` 接入、WS storage 监听、ServerSettings 页面、AgentChat "使用" 按钮、路由、i18n）。

**当前阻塞**：S9（测试）和 S10（构建 + 文档）未完成：
- `useApiBaseProbe.test.ts` 9 个测试全失败（mock 模式错位）
- `vue-tsc --noEmit` 5 个错误（2 个测试 + 3 个 ServerSettings.vue）

**目标**：把 9 个测试全过、`vue-tsc` 0 错误、`pnpm build` 通过，更新 plan 文档为"已实施"。

## 2. Current State Analysis（Phase 1 探索确认）

| 路径 | 状态 | 待修 |
|---|---|---|
| [`/workspace/app/encv-mobile/src/composables/useApiBaseProbe.ts`](file:///workspace/app/encv-mobile/src/composables/useApiBaseProbe.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/composables/useServerStatus.ts`](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/composables/useWebSocket.ts`](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/composables/useEventBus.ts`](file:///workspace/app/encv-mobile/src/composables/useEventBus.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/views/ServerSettings.vue`](file:///workspace/app/encv-mobile/src/views/ServerSettings.vue) | ✅ 已实施 | 3 个 vue-tsc 错 |
| [`/workspace/app/encv-mobile/src/views/AgentChat.vue`](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/router/index.ts`](file:///workspace/app/encv-mobile/src/router/index.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/i18n/settings.ts`](file:///workspace/app/encv-mobile/src/i18n/settings.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/i18n/agent.ts`](file:///workspace/app/encv-mobile/src/i18n/agent.ts) | ✅ 已实施 | 无 |
| [`/workspace/app/encv-mobile/src/composables/__tests__/useApiBaseProbe.test.ts`](file:///workspace/app/encv-mobile/src/composables/__tests__/useApiBaseProbe.test.ts) | ⚠️ 9/9 fail | 重写 mock 模式 |
| [`/workspace/.trae/documents/ai-routing-real-device-fix.md`](file:///workspace/.trae/documents/ai-routing-real-device-fix.md) | 📝 计划文件 | 标注"已实施" |

### vue-tsc 当前 5 错误（实测）
```
src/composables/__tests__/useApiBaseProbe.test.ts(134,63): error TS2740: Type 'Promise<never>' is missing Response properties
src/composables/__tests__/useApiBaseProbe.test.ts(145,43): error TS2740: 同上
src/views/ServerSettings.vue(149,13): error TS2322: 'string' is not assignable to 'boolean | undefined'  ← :clearinput 应为 :clear-input
src/views/ServerSettings.vue(186,36): error TS6133: 'onUnmounted' declared but never read
src/views/ServerSettings.vue(190,3): error TS6133: 'modalController' declared but never read
```

### 测试失败根因（实测）
```
TypeError: setApiBaseUrl.mockReset is not a function
 ❯ useApiBaseProbe.test.ts:68:33  ← vi.importActual 让 setApiBaseUrl 是原函数，没 .mockReset()
```
原方案用 `vi.fn()` 替换 `setApiBaseUrl`，导致 `setManual()` 内的 `setApiBaseUrl(url)` 不会写 localStorage（断言失败）。

## 3. Proposed Changes

### 3.1 修复 `useApiBaseProbe.test.ts` — mock 模式重写

**根因**：测试同时需要：
1. `setApiBaseUrl` 写 localStorage（原函数行为）
2. `setApiBaseUrl` 被 spy 观察（断言调用）
3. `getApiBaseUrl` 在测试里返回 loopback（不能是空串）

**方案**：用 `vi.spyOn(realModule, 'setApiBaseUrl')` 替换 `vi.mock` 整个 module——这样原函数跑、spy 也能观察。

**改动点**：

```typescript
// 旧（错）：用 vi.mock 替换整个 module
vi.mock('@/api/encv', async () => {
  const actual = await vi.importActual<typeof import('@/api/encv')>('@/api/encv')
  return { ...actual, getApiBaseUrl: vi.fn(() => '...'), ... }
})

// 新：用 spyOn 替代
import * as encv from '@/api/encv'
beforeEach(() => {
  vi.spyOn(encv, 'setApiBaseUrl').mockImplementation((url) => {
    localStorage.setItem('encv-server-url', url)  // 跑原行为
  })
  vi.spyOn(encv, 'getApiBaseUrl').mockReturnValue('http://127.0.0.1:2025')
})
```

**具体修复**：
1. 删除 `vi.mock('@/api/encv', ...)` 块（行 27-35）
2. `beforeEach` 中改用 `vi.spyOn(encv, 'setApiBaseUrl').mockImplementation(...)`（行 67-68）
3. 改 `networkError(): Promise<never>` 为 `(): never`（TS2740 报错）：
   ```typescript
   function networkError(): never {
     throw new TypeError('Failed to fetch')
   }
   // 用法：{ match: ..., respond: () => { throw new TypeError('Failed to fetch') } }
   ```
   或者更简单：保持 `Promise<never>` 返回类型，但 `fetchMock.mockImplementation` 用 `throw` 形式：
   ```typescript
   fetchMock.mockImplementation((url: string) => {
     if (url.includes('192.168.1.99')) return Promise.reject(new TypeError('Failed to fetch'))
     ...
   })
   ```
   直接放弃 `networkError` 辅助函数，每个 case 内联 reject。

### 3.2 修复 `ServerSettings.vue` — 3 个 vue-tsc 错

**3 处改动**：
1. 行 150：`:clearinput="true"` → `:clear-input="true"`（Ionic 实际 prop 名）
2. 行 186：从 `import { ref, computed, onMounted, onUnmounted } from 'vue'` 删除 `onUnmounted`
3. 行 190：从 `@ionic/vue` import 中删除 `modalController`

### 3.3 文档更新 — `ai-routing-real-device-fix.md`

**改动**：在文件头部加"实施完成"状态横幅，并补"实施记录"章节列出 10 步的实际产物（按之前会话的产出）。

### 3.4 验证步骤

```bash
# Step 1: 修测试
cd /workspace/app/encv-mobile
pnpm test --run src/composables/__tests__/useApiBaseProbe.test.ts
# 预期：9/9 通过

# Step 2: 修 vue-tsc
pnpm vue-tsc --noEmit
# 预期：0 错误

# Step 3: 构建检查
pnpm build
# 预期：通过（无 TS 错误、无构建错误）

# Step 4: 跑全部测试
pnpm test --run
# 预期：useApiBaseProbe 9 + 其它现存测试全过；不允许新引入失败
```

## 4. Assumptions & Decisions

| 决策 | 选择 | 理由 |
|---|---|---|
| **测试 mock 模式** | `vi.spyOn(realModule, fn).mockImplementation()` | 保留原函数副作用同时能被 spy 观察；最少改动 |
| **networkError 辅助函数** | 删除，每个 case 内联 `Promise.reject` | 类型 `Promise<never>` 不满足 fetch 的 `Promise<Response>` 期望；inline 写更清楚 |
| **ServerSettings 的 :clearinput** | 改为 `:clear-input`（Ionic 官方 prop 名） | vue-tsc 严格类型校验下 string 不被接受 |
| **未使用的 import** | 直接删 | TS6133 是不留 dead code 的硬约束；vue-tsc 配置严格 |
| **是否回退到 inline `<ion-modal :is-open>`** | 不 | modal 架构铁律已明确禁止跨 tab inline modal |
| **是否扩展 native bridge 探测** | 不 | native 模式 backend port 固定（已在 EncvGoService 完成），不需 LAN 探测 |
| **是否改 EncvGoService.kt / AndroidManifest** | 不 | 网络安全配置已有 `usesCleartextTraffic="true"`，LAN IP cleartext 通行 |

## 5. Verification（详细）

### 5.1 单元测试

```bash
cd /workspace/app/encv-mobile
pnpm test --run src/composables/__tests__/useApiBaseProbe.test.ts
```

**预期 9/9 通过**：
- `[1] cached 命中 → 立刻返回，不试 loopback`
- `[2] cached 失败 + loopback 命中 → 走 loopback`
- `[3] cached 失败 + loopback 失败 + LAN 候选命中 → 走 lan-candidate`
- `[4] 全部失败 → 抛 all-candidates-failed，不写 localStorage`
- `setManual 接受合法 URL` ← 关键：验证 localStorage 真的被原 `setApiBaseUrl` 写
- `setManual 拒绝不合法 URL`
- `resetToDefault 清 localStorage + 重探测`
- `10s 内重复 probe 默认走 throttle（无 force）`
- `force: true 跳过节流`

### 5.2 类型检查

```bash
pnpm vue-tsc --noEmit
```

**预期 0 错误**（修完后应只剩 0 项）。

### 5.3 构建

```bash
pnpm build
```

**预期**：vite 构建完成，0 TS 错误，产物在 `dist/`。

### 5.4 回归

- 跑全部测试（`pnpm test --run`）确认没有破坏其它现有测试
- 不动 `useApiBaseProbe.ts` 内部逻辑（测试修，不动 production code）

## 6. Out of Scope（明确不做）

- 重构 `useApiBaseProbe.ts`（不动 production code）
- 改 Android Kotlin 端（native 模式 backend port 固定）
- 加 mDNS / Bonjour（避免新依赖）
- 改 Go server 监听配置（已 `0.0.0.0`）
- 改 AndroidManifest 网络安全配置（已 `usesCleartextTraffic="true"`）
- 改 Go server `/api/network/lan-access`（已够用）

## 7. Implementation Order

1. **修复 `useApiBaseProbe.test.ts`**（mock 模式重写 + networkError 改 inline）
2. **修复 `ServerSettings.vue` 3 个 vue-tsc 错**（`:clearinput` + 删 `onUnmounted` + 删 `modalController`）
3. **跑测试 + vue-tsc 确认全过**
4. **跑 `pnpm build` 确认构建通过**
5. **更新 `ai-routing-real-device-fix.md`**（加"实施完成"状态 + 实施记录）

## 8. 实施产物清单（已完成部分 + 待完成）

| 步骤 | 文件 | 状态 |
|---|---|---|
| 1 | `composables/useApiBaseProbe.ts` | ✅ |
| 2 | `composables/useServerStatus.ts` 接入 probe | ✅ |
| 3 | `composables/useWebSocket.ts` storage 监听 | ✅ |
| 4 | `useAgentApiBase.ts` 启动触发 + visibilitychange | ✅（已并入 useServerStatus.onMounted） |
| 5 | `views/ServerSettings.vue` 手动入口 | ✅ |
| 6 | `views/AgentChat.vue` lanAccessPanel "使用" 按钮 | ✅ |
| 7 | `router/index.ts` 加 `/settings/server` | ✅ |
| 8 | i18n 增量 | ✅ |
| **9** | **`useApiBaseProbe.test.ts` 单测** | **🔄 修复中** |
| **10** | **构建检查 + 文档** | **⏳ 待做** |
