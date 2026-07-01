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

## 五、相关规则索引

| 规则文件 | 内容 |
|---------|------|
| [test-orchestration.md](./test-orchestration.md) | Go 测试编排守卫、沙箱限制、合法入口 |
| [test.md](./test.md) | Mock 数据规范、后门协议、浏览器自动化流程 |
| [automation-workflow.md](./automation-workflow.md) | 自动化测试工作流、4 件套事件监听 |
| [ci-workflow.md](./ci-workflow.md) | CI 三层测试体系（Layer1/2/3） |
| [verification-discipline.md](./verification-discipline.md) | 验证纪律、问题排查方法论 |

---

## 六、演进方向

- [ ] Cypress E2E 测试报告自动生成（Mochawesome + 自定义 reporter）
- [ ] 性能基线存储（每次发版自动跑，对比历史基线）
- [ ] 回归测试自动化（PR 标签触发 Layer3 E2E）
- [ ] 前端 vitest 沙箱合法化（条件成熟后）

> 创建：2026-07-01
> 核心原则：Cypress 真实测试为性能结论唯一依据，Go bench 仅作补充参考
