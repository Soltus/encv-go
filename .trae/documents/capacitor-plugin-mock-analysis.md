# Capacitor 插件 Mock 测试分析

## 问题

[Jest 手动模拟 Capacitor 插件](https://raw.githubusercontent.com/TacKana/capacitor-docs-zh/refs/heads/main/docs/main/guides/mocking-plugins.md) 这篇文章是否对我们项目有帮助？

## 结论：概念有价值，但当前优先级极低

文章解决的核心问题（Capacitor 插件无法被 Jest 自动 mock）在我们的项目中**已经天然解决**了。但"隔离外部依赖"这个测试思路本身是正确的，我们已经在 Go 后端测试中践行了。

---

## 详细分析

### 1. 文章解决了什么问题？

Capacitor 插件在 JS 层是 `Proxy` 对象，而 Jest/Vitest 的自动 mock 会再包一层 Proxy → **Proxy 的 Proxy**，导致报错。文章的方案是用手动 mock（`__mocks__/@capacitor/xxx.ts`）替代自动 mock，绕过 Proxy 限制。

### 2. 我们项目为什么不存在这个问题？

| 对比项 | 文章场景 | 我们项目 |
|--------|----------|----------|
| 插件类型 | 标准 Capacitor 插件（Storage、Toast 等） | **自定义插件 GoProcess** |
| JS 层实现 | 纯 Proxy，无 web fallback | **已有 `GoProcessWeb` 类**（[web.ts](file:///workspace/app/encv-mobile/src/plugins/web.ts)） |
| Mock 方式 | 需要 `__mocks__/@capacitor/xxx.ts` | **`GoProcessWeb` 本身就是天然的 mock** |

关键代码 — `GoProcess` 注册时已指定 web fallback：

```typescript
// GoProcess.ts L13-15
const GoProcess = registerPlugin<GoProcessPlugin>('GoProcess', {
  web: () => import('./web').then(m => new m.GoProcessWeb()),
})
```

`GoProcessWeb` 的所有方法都返回安全的默认值（`{ success: false }`、`{ running: false, port: 0 }`），**在浏览器/web 测试环境下自动生效，不需要额外 mock**。

### 3. 前端测试现状

| 检查项 | 状态 |
|--------|------|
| 测试框架（Jest/Vitest） | ❌ 未安装 |
| 测试文件（*.test.ts / *.spec.ts） | ❌ 零个 |
| test 脚本 | ❌ package.json 无 test 命令 |
| 测试工具（vue-test-utils） | ❌ 未安装 |

项目前端**完全没有测试基础设施**。要利用文章中的 mock 技术，需要先从零搭建测试环境。

### 4. 如果未来要做前端测试，需要什么？

假设要测试 [useServerStatus.ts](file:///workspace/app/encv-mobile/src/composables/useServerStatus.ts)（最依赖 GoProcess 的 composable）：

```typescript
// vitest 配置后，mock GoProcess 模块
vi.mock('@/plugins/GoProcess', () => ({
  isNative: () => true,
  restartBackend: vi.fn().mockResolvedValue({ success: true, port: 2025 }),
  stopBackend: vi.fn().mockResolvedValue({ success: true }),
  getBackendStatus: vi.fn().mockResolvedValue({ running: true, port: 2025 }),
}))
```

这和文章的思路一致（手动 mock 替代自动 mock），但用 Vitest 的 `vi.mock()` 而非 Jest 的 `__mocks__` 目录。我们的自定义插件不需要 `__mocks__` 目录方案，因为 `GoProcessWeb` 已经是手动 stub。

### 5. 为什么 Go 后端测试优先级更高？

| 维度 | 前端 | Go 后端 |
|------|------|---------|
| 业务逻辑复杂度 | 低（主要是 API 调用 + UI 状态） | **高**（加解密、任务状态机、WebSocket、OpenList 集成） |
| Bug 影响 | UI 显示异常 | **数据丢失/损坏** |
| 现有测试基础 | 零 | 已有 v2 加密模块测试 + MockBroadcaster |
| 投入产出比 | 低（需从零搭建，前端逻辑薄） | **高**（核心逻辑集中，已有接口抽象） |

## 建议

1. **当前聚焦 Go 后端测试**（已在进行中），这是投入产出比最高的方向
2. **前端测试暂缓**，等后端测试稳定后再考虑
3. **如果未来做前端测试**：
   - 使用 Vitest（Vite 项目原生支持，不需要 Jest）
   - `GoProcessWeb` 已是天然 mock，不需要 `__mocks__` 目录
   - 优先测试 `useServerStatus`、`encv.ts` 等含逻辑的模块
   - UI 组件测试优先级最低
