# 测试体系总纲（Test Master Plan）

> **核心原则：Cypress 真实测试为主，Go 基准测试为辅；所有测试必须可复用、可回归、可对比。**
> **严禁临时脚本 / 临时文件 / 手动命令行测试。任何性能结论必须出自正式测试报告。**

---

## 一、测试层级与定位

### 1.1 五层测试金字塔

```
        ┌─────────────────────────────┐
        │     E2E (Cypress)           │  ← 用户视角，真实浏览器，真实后端
        │  （性能 / 集成 / 回归）      │     衡量端到端用户体验
        └─────────────────────────────┘
                    ▲
        ┌─────────────────────────────┐
        │  Component (Cypress)        │  ← 组件级，Vue + Ionic 真组件
        │  （UI / 交互 / 状态）       │     验证组件契约
        └─────────────────────────────┘
                    ▲
        ┌─────────────────────────────┐
        │  Go 单元测试                 │  ← 函数级，逻辑正确性
        │  （功能 / 边界 / 异常）     │     保证代码质量
        └─────────────────────────────┘
                    ▲
        ┌─────────────────────────────┐
        │  Go 基准测试 (Benchmark)    │  ← 函数级性能，补充参考
        │  （吞吐 / 延迟 / 内存）     │     快速对比算法差异
        └─────────────────────────────┘
                    ▲
        ┌─────────────────────────────┐
        │  静态检查 / Lint / TypeCheck│  ← 代码风格与类型安全
        │  （golangci-lint / vue-tsc）│     第一道防线
        └─────────────────────────────┘
```

### 1.2 各层职责分工

| 层级 | 工具 | 主要用途 | 性能指标可信度 | 运行频率 |
|------|------|---------|--------------|---------|
| **E2E (Cypress)** | Cypress + Electron | 端到端用户旅程、真实 API 集成、性能对比 | ⭐⭐⭐⭐⭐ 最高 | 每次发版 / 性能优化后 |
| **Component** | Cypress Component | 组件 UI 渲染、交互逻辑、状态管理 | ⭐⭐ 不测性能 | 每次组件改动 |
| **Go 单元测试** | `go test` | 业务逻辑正确性、边界条件、异常处理 | ❌ 不测性能 | 每次 PR (Layer1) |
| **Go 基准测试** | `go test -bench` | 函数级吞吐 / 延迟 / 内存分配 | ⭐⭐⭐ 参考值 | 性能优化前后对比 |
| **静态检查** | golangci-lint / vue-tsc | 代码风格、类型安全、常见错误 | ❌ 不测性能 | 每次提交 |

### 1.3 铁律：性能结论必须出自 Cypress E2E

> **任何关于"XX 更快"、"YY 更慢"、"优化了 Z%"的结论，必须有 Cypress E2E 真实测试数据支撑。**
> **Go 基准测试仅作补充参考，不作为性能结论的最终依据。**

原因：
- Go bench 测的是函数级，无法反映端到端用户体验（API 延迟 / 前端渲染 / WS 推送 / DB 往返）
- Go bench 的环境是纯内存，没有真实磁盘 IO、网络延迟、并发竞争
- Cypress E2E 用真实浏览器 + 真实后端 + 真实数据库，数据最接近用户真实体验

---

## 二、测试目录结构规范

### 2.1 Cypress 目录

```
app/encv-mobile/cypress/
├── e2e/                          # E2E 测试（真实页面 + 真实后端）
│   ├── app-smoke.cy.ts          # 烟测：页面基本可用性
│   ├── db-engine-perf.cy.ts     # 性能：SQLite vs Turso 对比
│   ├── workflow-dag.cy.ts       # 功能：DAG 工作流调度正确性
│   └── ...
├── component/                    # 组件测试（挂载单组件）
│   ├── Tasks.cy.ts
│   ├── TaskTimeline.cy.ts
│   └── ...
├── support/                      # 测试基础设施
│   ├── commands.ts              # 自定义 cy.* 命令
│   ├── e2e.ts                   # E2E 全局 setup
│   ├── component.ts             # Component 全局 setup
│   ├── task-test-helpers.ts     # Task 领域 helper（fixtures/store/dom）
│   └── store-helpers.ts         # Pinia store 注入辅助
└── fixtures/                     # 静态测试数据（JSON / 文件）
    └── ...
```

### 2.2 Go 测试目录

```
internal/
├── service/
│   ├── task_manager_test.go           # 单元测试：TaskManager 逻辑
│   ├── task_manager_bench_test.go     # 基准测试：创建/更新性能
│   ├── task_manager_sql_test.go       # 集成测试：SQLite 存储
│   └── ...
└── ...
```

---

## 三、性能测试方法论

### 3.1 性能测试三要素

任何性能对比测试必须同时满足：

1. **相同硬件环境**：同一台机器、同一段连续时间、相同系统负载
2. **相同测试负载**：相同任务数、相同数据量、相同操作流程
3. **多次测量取中位**：至少跑 3 次，取中位数（排除 outliers）

### 3.2 性能指标定义

| 指标 | 含义 | 测量方式 |
|------|------|---------|
| **端到端耗时** | 从点击"开始"到所有任务完成的总时间 | Cypress `performance.now()` |
| **吞吐量** | 单位时间完成的任务数（tasks/sec） | `总任务数 / 总耗时` |
| **单任务平均耗时** | 每个任务的平均处理时间 | `总耗时 / 总任务数` |
| **峰值并发** | 同一时刻 running 状态的最大任务数 | 轮询 API 统计最大值 |
| **首任务延迟** | 从提交到第一个任务开始执行的时间 | 第一个 task status 从 queued → running |
| **尾任务延迟** | 最后一个任务完成时间 vs 平均完成时间 | P99 / P95 |

### 3.3 对比测试报告格式

所有性能对比必须产出结构化报告（JSON + Markdown），包含：

```json
{
  "testId": "db-engine-perf-2026-07-01",
  "timestamp": "2026-07-01T00:00:00Z",
  "environment": {
    "os": "Linux",
    "cpuCores": 8,
    "memoryGB": 16
  },
  "scenarios": [
    {
      "name": "SQLite 100 tasks",
      "engine": "sqlite",
      "taskCount": 100,
      "runs": [32000, 31500, 33000],
      "medianMs": 32000,
      "throughput": 3.125
    },
    {
      "name": "Turso 100 tasks",
      "engine": "turso",
      "taskCount": 100,
      "runs": [15000, 14800, 15200],
      "medianMs": 15000,
      "throughput": 6.667
    }
  ],
  "conclusion": "Turso 比 SQLite 快 2.13x（100 tasks 场景）"
}
```

---

## 四、测试执行规范

### 4.1 合法入口

| 测试类型 | 唯一合法入口 | 说明 |
|---------|------------|------|
| Go 单包测试 | `bash scripts/test-go.sh ./internal/<pkg>` | 带守卫，默认 -short |
| Go 全量测试 | `ENCV_TEST_FULL=1 bash scripts/test-all-go.sh` | CI 用，解除 -short |
| Go 基准测试 | `bash scripts/test-go.sh -bench ./internal/service` | 仅跑 benchmark |
| Cypress Component | `pnpm exec cypress run --component` | 沙箱可跑（轻量） |
| Cypress E2E | `pnpm exec cypress run --e2e` | 需后端 + Vite 同时运行 |
| 前端 type check | `pnpm exec npx vue-tsc --noEmit` | 沙箱唯一合法前端检查 |

### 4.2 严禁行为

1. ❌ **严禁临时脚本测试** — 不允许在 `/tmp`、`/workspace` 根目录写一次性测试脚本
2. ❌ **严禁手动 curl 循环测性能** — 性能数据必须出自 Cypress 正式测试
3. ❌ **严禁裸 `go test` 跑多包** — 必须走 `scripts/test-go.sh`（守卫保护）
4. ❌ **严禁用 `console.log` 打时间戳当性能数据** — 必须用 `performance.now()` + 结构化报告
5. ❌ **严禁单次测量下结论** — 至少 3 次取中位数

### 4.3 测试数据留存

- 所有性能测试报告存放在 `test-reports/performance/` 目录（按日期归档）
- 测试报告必须可复现（记录环境、版本、参数）
- 对比测试必须保留两侧的完整原始数据（不仅是结论）

---

## 五、Bug 复现铁律：Cypress E2E 先行（最高优先级）

> **用户原话（2026-07-03）："严禁猜测修改，必须先用 Cypress e2e 测试复现问题再修复。"**
>
> **任何 bug 修复 PR 必须先有复现该 bug 的 Cypress E2E 测试。先复现，再修复。不允许盲改。**

### 5.1 铁律（违反 = 严重错误）

| 步骤 | 行为 | 禁止 |
|------|------|------|
| ① 接到 bug 报告 | 读相关源码，理解数据流 | ❌ 直接改代码 |
| ② 写复现测试 | 在 `cypress/e2e/` 写 `.cy.ts`，断言期望行为 | ❌ 凭猜测改代码 |
| ③ 跑测试确认 RED | 测试失败 = 成功复现 bug | ❌ 跳过确认直接改 |
| ④ 修复代码 | 最小改动修复 | ❌ 顺手重构/加功能 |
| ⑤ 跑测试确认 GREEN | 测试通过 = 修复验证 | ❌ 不跑测试就提交 |

### 5.2 反模式（禁止）

- ❌ **盲改修复**：不写测试，直接改代码，"我觉得这样改对"
- ❌ **猜测根因**：不读源码，凭 bug 描述猜原因
- ❌ **改完不验证**：改了代码但不跑 Cypress 验证
- ❌ **复现测试写太弱**：只测 `cy.visit()` 不报错，不测实际 bug 行为

### 5.3 Cypress E2E 测试环境配置（沙箱踩坑）

> **测试环境没有 preview-gateway :16666，必须手动配置 API base URL。**

#### 5.3.1 设置 API base URL（必做）

前端 `getApiBaseUrl()` 在 dev 模式默认返回 `DEV_SANDBOX_ENTRY='http://127.0.0.1:16666'`（preview-gateway 端口）。测试环境没有网关，fetch 会 `ECONNREFUSED`。

**修复**：在 `beforeEach` 用 `window:before:load` 写 localStorage：

```typescript
beforeEach(() => {
  cy.on('window:before:load', (win) => {
    win.localStorage.setItem('encv-server-url', 'http://localhost:2025')
  })
})
```

> ⚠️ 必须在 `window:before:load`（页面脚本执行前）设置，不能在 `cy.visit` 之后设置。

#### 5.3.2 dismiss ErrorCaptureOverlay（WebSocket 失败浮窗）

测试环境没有 `/ws` 代理，WebSocket 连接失败 → `useErrorCapture.addError` → `<ErrorCaptureOverlay>` 浮窗显示，遮挡搜索框等交互元素。

**修复**：写 helper 在 `cy.visit` 后 dismiss 浮窗：

```typescript
function dismissErrorOverlay() {
  cy.get('body').then(($body) => {
    const closeBtn = $body.find('.error-overlay-close')
    if (closeBtn.length > 0) {
      cy.wrap(closeBtn).first().click({ force: true })
      cy.wait(300)
    }
  })
}
```

> 搜索功能走 HTTP API（不依赖 WS），所以 dismiss 浮窗不影响测试有效性。

#### 5.3.3 contenteditable 搜索框输入

`cy.type()` 在 contenteditable div 上可正常触发 Vue `onQueryInput`（包括 `{enter}`）。

```typescript
cy.get('[data-testid="search-input"]').click({ force: true })
cy.get('[data-testid="search-input"]').type('keyword{enter}', { delay: 50, force: true })
```

> `force: true` 防止 ErrorCaptureOverlay 残留时遮挡报 "cannot be interacted with"。

#### 5.3.4 cy.intercept spy 断言陷阱

`cy.get('@alias').should('have.been.called')` 在某些情况下报 "is not a spy or a call to a spy"，即使请求确实匹配了 intercept。

**原因**：Cypress intercept 的 spy 注册时机与请求匹配时机有竞态。

**修复**：优先用 UI 层断言（`cy.get('[data-testid="..."]').should('exist')`），不依赖 `@alias` spy 断言。如果必须验证 API 调用，用 `cy.wait('@alias')` 代替 `should('have.been.called')`。

### 5.4 启动 Cypress E2E 测试的命令

```bash
# 1. 启动后端（已运行则跳过）
# 2. 启动 Vite dev server（PM2_HOME 绕过 dev-start-guard）
PM2_HOME=/tmp/cypress-pm2 pm2 start "npm run dev" --name encv-dev --no-autorestart
sleep 6  # 等 vite ready

# 3. 跑 Cypress E2E（xvfb-run headless）
cd app/encv-mobile
CYPRESS_BASE_URL=http://localhost:8100 xvfb-run -a npx cypress run \
  --spec cypress/e2e/<spec>.cy.ts --browser electron

# 4. 清理
PM2_HOME=/tmp/cypress-pm2 pm2 delete encv-dev
```

---

## 六、相关规则索引

| 规则文件 | 内容 |
|---------|------|
| [test-orchestration.md](./test-orchestration.md) | Go 测试编排守卫、沙箱限制、合法入口 |
| [test.md](./test.md) | Mock 数据规范、后门协议、浏览器自动化流程 |
| [automation-workflow.md](./automation-workflow.md) | 自动化测试工作流、4 件套事件监听 |
| [ci-workflow.md](./ci-workflow.md) | CI 三层测试体系（Layer1/2/3） |
| [verification-discipline.md](./verification-discipline.md) | 验证纪律、问题排查方法论 |

---

## 七、演进方向

- [ ] Cypress E2E 测试报告自动生成（Mochawesome + 自定义 reporter）
- [ ] 性能基线存储（每次发版自动跑，对比历史基线）
- [ ] 回归测试自动化（PR 标签触发 Layer3 E2E）
- [ ] 前端 vitest 沙箱合法化（条件成熟后）

> 创建：2026-07-01
> 更新：2026-07-03（新增 §五 Bug 复现铁律：Cypress E2E 先行）
> 核心原则：Cypress 真实测试为性能结论唯一依据，Go bench 仅作补充参考；Bug 修复必须先 Cypress 复现
