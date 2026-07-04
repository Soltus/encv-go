# Go 测试编排铁律（来自沙箱断网实战踩坑）

> **核心原则：Go 测试必须模块化（单包 + -short）跑，禁止全包 `./...` 直跑沙箱。**
> **沙箱跑 `go test ./...` 动辄 380s+，耗尽网络配额触发断网。**
> **test-go.sh / test-all-go.sh 是唯一合法入口；违反者直接抛错 exit 64。**
> **🆕 2026-06-17：前端 vitest 同样禁止在沙箱跑（沙箱跑 vitest 同样会耗尽网络配额/触发断网）。**
> **前端 type check 走 `pnpm exec npx vue-tsc --noEmit`（沙箱合法入口）。**

> **完整内容 + 守卫实现 + 拓扑分批 + 历史踩坑**：[详情文档](../rule-library/test-orchestration.md)

---

## 一、铁律（违反 = panic 抛错 exit 64）

### 1.1 禁止模式

| 命令 | 行为 | 守卫响应 |
|------|------|---------|
| `go test ./...` | 全包 | ❌ 守卫抛错（CI 模式外） |
| `go test ./internal/...` | 多包 | ❌ 守卫抛错（CI 模式外） |
| `go test ./cmd/...` | 多包 | ❌ 守卫抛错（CI 模式外） |
| `go test ./internal/service/...` | 多包（递归子包） | ❌ 守卫抛错（CI 模式外） |
| `go test ./internal/service` | ✅ 单包合法 | -short + -failfast 默认 |
| `go test -run TestX ./internal/service` | ✅ 单包 + 过滤 | 透传 |
| `ENCV_TEST_FULL=1 go test ./internal/...` | CI 模式 | ✅ 放行（去 -short） |

### 1.2 守卫实现位置（🆕 2026-06-17 三层防护）

**代码层（三层防护，缺一不可）**：

1. **Shell 层（多包模式拦截）**：[scripts/test-go.sh:52-100](file:///workspace/scripts/test-go.sh#L52-L100) `is_multipkg` 检测 + exit 64
2. **Shell 层（env 标记注入）**：[scripts/test-go.sh:196-210](file:///workspace/scripts/test-go.sh#L196-L210) `export ENCV_TEST_INVOKED_BY=scripts/test-go.sh`（供第 3 层 Go init() 验证）
3. **Go init() 层（裸 go test 拦截）**：[internal/testguard/guard.go](file:///workspace/internal/testguard/guard.go) — 任何 import 了 testguard 的测试包，init() 强制检查 `ENCV_TEST_INVOKED_BY` 是否设置；未设置 → exit 64
4. **推荐入口**：[scripts/test-all-go.sh](file:///workspace/scripts/test-all-go.sh) 默认走模块化（go list 拿包 + 循环单包）

> **早期版本曾反对 Go init() 守卫（"必须被 import 才执行 / 全局污染 / 不可控"），2026-06-17 推翻**：
> - 用户多次反馈"go 完整测试拦截似乎没有考虑到所有调用方式，没有拦截你刚才" — 证明纯 shell 守卫对裸 `go test` 无效
> - 解决方案：在关键根包（`internal/v2/plugins` / `internal/v2/container/detector` / `internal/v2/container/handle` / `internal/v2/writer` / `internal/v2/reader` / `internal/v2/service` / `internal/v2/handler` / `internal/v2/crypto` / `internal/v2/types` / `internal/webdav` / `internal/service` / `internal/utils` / `internal/mount` / `internal/config` / `internal/skills` 等）的 `_test.go` 加 `_ "github.com/Soltus/encv-go/internal/testguard"` blank import
> - production binary 旁路：`os.Args[0]` 不含 `.test` 且无 `-test.*` flag → 不是 test binary → 放行（避免 `go run ./cmd/encv/ serve` 被误伤）

---

## 二、模块化测试编排（默认推荐）

### 2.1 日常开发

```bash
# ✅ 推荐：模块化编排（自动 go list 拿包 + 循环单包 + -short -failfast）
bash scripts/test-all-go.sh

# ✅ 推荐：单包测试
bash scripts/test-go.sh ./internal/service
bash scripts/test-go.sh ./internal/mount

# ✅ 推荐：单包 + -run 过滤
bash scripts/test-go.sh -run TestTaskManager_ResolveAbsPath_Mount ./internal/service

# ✅ 推荐：单包 + 跑 slow（解除 -short）
ENCV_TEST_LONG=1 bash scripts/test-go.sh ./internal/server
```

### 2.2 CI 模式（须显式声明）

```bash
# CI 模式：全量 + 解除 -short + 不失败早停
ENCV_TEST_FULL=1 bash scripts/test-all-go.sh

# CI 模式：单包全量
ENCV_TEST_FULL=1 bash scripts/test-go.sh ./internal/server
```

### 2.3 退出码

| 退出码 | 含义 | 触发场景 |
|--------|------|---------|
| 0 | OK | 全部 pass |
| 1 | TEST_FAILURE | 测试断言失败 |
| 64 | GUARD_REJECTED | 多包/全包未声明 ENCV_TEST_FULL |
| 124 | TIMEOUT | 超过 HARD_TIMEOUT/PACKAGE_TIMEOUT |
| 137 | OOM_KILLED / KILLED | 沙箱 OOM / 超时强杀 |

> 64 是 sysexits.h 的 `EX_USAGE`（调用方式错误）。这是**预期行为**，不是脚本 bug。

---

## 🆕 二点五、前端 vitest 不在沙箱跑（强制规则 · 2026-06-17）

### 二点五.1 铁律

> **沙箱内禁止跑 `pnpm exec vitest` / `pnpm exec npx vitest` / `npm test` / `yarn test` 等任何前端测试运行器。**
> **沙箱内前端 type check 唯一合法入口：`pnpm exec npx vue-tsc --noEmit`（或 `npx vue-tsc --noEmit`）。**

### 二点五.2 为什么 vitest 禁沙箱

| 风险 | 触发 |
|------|------|
| **网络配额耗尽** | vitest happy-dom / jsdom + 大型 test 套件需拉大量 npm 依赖；沙箱 `~600s/任务` 配额快速耗尽 |
| **transform 阻塞** | vitest watch 模式会卡在 `transforming` 几分钟，触发 dev-start-guard / service-guard |
| **内存爆** | happy-dom 实例 × N test 文件 → Vite Node process 涨到 4-6 GB → OOM kill（exit 137） |
| **污染 preview-gateway** | vitest 启动自带 dev server，跟 preview-gateway 抢 8100 端口冲突 |

### 二点五.3 正确流程

| 阶段 | 沙箱内 | 沙箱外（用户本机 / CI） |
|------|--------|---------------------|
| **写 test** | ✅ 写代码（`*.test.ts`） | ✅ |
| **type check** | ✅ `pnpm exec npx vue-tsc --noEmit` | ✅ |
| **单元测试** | ❌ **禁止** | ✅ `pnpm exec vitest` |
| **E2E（Playwright/Cypress）** | ❌ **禁止** | ✅ 单独 runner |

### 二点五.4 沙箱 type check 正确命令

```bash
# ✅ 沙箱内合法：纯 type check，不启动 vitest，不拉大依赖
cd /workspace/app/encv-mobile
pnpm exec npx vue-tsc --noEmit

# ✅ 沙箱内合法：等价（pnpm exec 包装）
pnpm exec vue-tsc --noEmit

# ❌ 沙箱内禁止（虽然 type check 模式也不跑 test，但命名暗示 vitest）
pnpm exec vitest --typecheck
pnpm exec vitest run
```

> 核心区分：`vue-tsc` 是 TS 编译器前端（轻量 ~5s），`vitest` 是测试运行器（重量 ~30-60s 起 + watch）。

### 二点五.5 违规示例（绝不允许）

| 违规命令 | 后果 |
|---------|------|
| `pnpm exec vitest run` | 触发 transform 阻塞 → 网络断网 / OOM |
| `pnpm exec vitest --typecheck` | 命名混淆，且实际仍走 vitest runner |
| `pnpm test` | 等价 vitest run |
| `pnpm exec npx vitest --run src/views/WebDavAutomationTestsDetail.spec.ts` | 单文件 vitest 仍触发依赖拉取 |

### 二点五.6 未来：vitest 沙箱合法化条件

- [ ] 沙箱网络配额从 ~600s 提升到 ~3600s
- [ ] vitest 沙箱模式（disable transform 缓存预热）
- [ ] Playwright headless 在沙箱中稳定运行
- [ ] 引入 `vitest --shard` + `--reporter=basic` 进一步压缩耗时

**当前（2026-06-17）：以上条件未满足，vitest 严禁在沙箱跑。**

### 二点五.7 与 Go 测试守卫的关系

| 维度 | Go (`go test`) | 前端 (`vitest`) |
|------|---------------|-----------------|
| **沙箱禁跑** | ✅ test-go.sh 守卫 exit 64 | ✅ 本规则明确禁止（无 shell 守卫，靠纪律） |
| **推荐入口** | `scripts/test-go.sh` | `vue-tsc --noEmit`（type only） |
| **合法子集** | 单包 + `-short` + `-run` | 无合法子集（沙箱直接禁） |
| **沙箱外** | `ENCV_TEST_FULL=1 bash scripts/test-all-go.sh` | `pnpm exec vitest run` |

---

## 🆕 二点六、Go init() 守卫（testguard）— 裸 `go test` 拦截

### 二点六.1 为什么需要

> **用户原话**：「go 完整测试拦截似乎没有考虑到所有调用方式，没有拦截你刚才」

**问题链**：
- `scripts/test-go.sh` 的 bash 守卫**只对 `bash scripts/test-go.sh` 入口有效**
- 用户直接 `go test ./internal/v2/plugins` 时 → bash 守卫完全不触发 → 裸 go test 进程跑（可能跑全包、超时、沙箱断网）
- 解决方法：**Go test binary 启动时 init() 拦截**

### 二点六.2 三态判定（testguard init()）

```go
// internal/testguard/guard.go
func init() {
    // 0) Production binary 旁路
    //    - os.Args[0] 包含 ".test"（如 detector.test）→ go test 标准模式
    //    - os.Args 含 -test.* flag → 任何含 test flag 的进程
    //    - os.Args[0] 包含 ".test."（如 detector.test.new）→ `go test -c -o` 产物
    isTestBinary := checkArgs(os.Args)
    if !isTestBinary {
        return  // production binary → 放行（go run / go build 产物不误伤）
    }

    // 1) 用户显式 bypass
    if os.Getenv("ENCV_TEST_BYPASS_GUARD") == "1" {
        return
    }

    // 2) 守卫主逻辑
    if os.Getenv("ENCV_TEST_INVOKED_BY") == "" {
        fmt.Fprintf(os.Stderr, "❌ 拦截裸 go test ...")
        os.Exit(64)  // EX_USAGE — 与 bash 守卫一致
    }
}
```

### 二点六.3 接入方式

| 包类型 | 接入 | 效果 |
|--------|------|------|
| **测试入口包** | 在任一 `_test.go` 加 `_ ".../testguard"` blank import | 该包的 test binary 启动时 init() 触发守卫 |
| **推荐覆盖范围** | `internal/v2/plugins` / `detector` / `handle` / `manifest` / `writer` / `reader` / `service` / `handler` / `crypto` / `types` / `webdav` / `service` / `utils` / `mount` / `config` / `skills` 等 | 17+ 关键根包全拦截 |
| **未覆盖包** | 暂时不在守卫范围（接受限制） | 裸 `go test ./internal/<未覆盖包>` 仍能跑（但需自负责任） |
| **production 启动** | 不在 test binary 上下文 → init() 提前 return | `go run ./cmd/encv/ serve` 不受影响 |

### 二点六.4 紧急 Bypass

```bash
# 紧急调试场景（仅限 CI fail / sandbox 异常）
ENCV_TEST_BYPASS_GUARD=1 go test -run TestX ./internal/<pkg>
# 输出：守卫放行，测试正常跑（不推荐常规使用）
```

### 二点六.5 跨包测试（多包 + bypass）

```bash
# 跨包调试时临时 bypass（绕过 test-all-go.sh 的 MODULAR_MAX_PKGS 守卫）
ENCV_TEST_BYPASS_GUARD=1 go test ./internal/v2/plugins/... ./internal/v2/container/...
# 注意：这只绕过 init() 守卫，不绕过 MODULAR_MAX_PKGS（那个在 test-all-go.sh）
```

### 二点六.6 验证

```bash
# ✅ 放行（scripts/test-go.sh 会 export ENCV_TEST_INVOKED_BY）
bash scripts/test-go.sh ./internal/v2/plugins  # PASS

# ❌ 拦截（裸 go test）
go test ./internal/v2/plugins  # FAIL + 完整拦截信息（exit 64 → go test 报 FAIL）

# ❌ 拦截（直接跑编译产物）
go test -c -o /tmp/foo.test ./internal/v2/plugins
unset ENCV_TEST_INVOKED_BY
/tmp/foo.test  # 拦截

# ✅ 放行（带 env）
ENCV_TEST_INVOKED_BY=scripts/test-go.sh /tmp/foo.test  # PASS

# ✅ 放行（带 bypass）
ENCV_TEST_BYPASS_GUARD=1 /tmp/foo.test  # PASS
```

---

## 三、为什么沙箱内禁止全包跑

### 3.1 沙箱网络配额

| 模式 | 单次耗时 | 沙箱网络风险 |
|------|---------|-------------|
| `go test ./internal/service -short` | 0.5-2s | ✅ 安全 |
| `go test ./internal/server -short`（核心包） | 0.5-2s | ✅ 安全 |
| `go test ./internal/server`（带 TestMinimalMediaIsPlayable） | 376s | ⚠️ 沙箱 OOM 风险 |
| `go test ./internal/...`（多包 12 个 + 各包慢测试） | **380s+** | ❌ 触发断网 |
| `go test ./...`（含 e2e + 集成 + bench） | **600s+** | ❌ 必断网 |

### 3.2 历史踩坑（2026-06-15）

**症状**：用户执行 `go test ./internal/...` → 380s 后沙箱断网，Claude 必须 reconnect。

**根因**：
1. server 包单跑 376s（含 TestMinimalMediaIsPlayable ffmpeg 环境测试）
2. 多个包累加 380s+
3. 沙箱网络配额耗尽（通常 ~600s/任务）
4. 网络断 → 当前 go test 进程 OOM kill → exit 137
5. Claude 自动 retry → 同样断网

**修复（2026-06-15）**：test-go.sh / test-all-go.sh 加 PKGS 守卫 + 默认 -short + 失败早停。

---

## 四、长期演进（不写"如何绕过"）

### 4.1 ✅ 正确演进

| 场景 | 演进方式 |
|------|---------|
| 新增 test 包 | 跑 `bash scripts/test-go.sh ./internal/<新包>` 验证 |
| 新增 slow bench | 用 `testing.Short()` 守卫 + `ENCV_TEST_LONG=1` 显式跑 |
| 新增 e2e 测试 | 放在独立 e2e package + `//go:build e2e` tag 隔离 |
| CI 跑全量 | `.github/workflows/ci.yml` 设 `ENCV_TEST_FULL=1` |

### 4.2 ❌ 错误演进（绝不允许）

| 错误 | 后果 |
|------|------|
| 直接 `go test ./internal/...`（绕过 test-go.sh） | 沙箱断网 |
| 改 test-go.sh 注释说 "CI=true 跳过守卫" | 跟 dev-start-guard 收编同源问题（已被收编） |
| 改 mount 用例多包到默认 -short 范围 | 应该用 ENCV_TEST_LONG 显式跑 |
| 加新规则文档教 "如何绕过" | 触发"可绕过文档收编" |

---

## 五、与其他规则的交叉

| 规则 | 关系 |
|------|------|
| [development.md §5](file:///workspace/.trae/rules/development.md) | 后端启动规范；test-go.sh 与之同级（都是"统一入口 + 守卫"模式） |
| [combolite.md](file:///workspace/.trae/rules/combolite.md) | 无关（combolite 集成） |
| [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) | 弱相关（vitest 模块化测试可参考本规则 §二点五） |
| [test-master-plan.md](./test-master-plan.md) | **强相关** — 测试体系总纲，Cypress 为主 Go bench 为辅 |

---

## 🆕 六、Cypress 测试编排（2026-07-01 新增）

### 6.1 Cypress 测试分类

| 类型 | 目录 | 用途 | 沙箱可跑 | 依赖后端 |
|------|------|------|---------|---------|
| **Component Testing** | `cypress/component/` | 单组件 UI / 交互 / 状态 | ✅ 是 | ❌ 否 |
| **E2E Testing** | `cypress/e2e/` | 整页交互 / 真实 API / 性能对比 | ⚠️ 需后端 | ✅ 是 |

### 6.2 Cypress 合法入口

```bash
# ✅ Component 测试（沙箱可跑，轻量）
cd app/encv-mobile
pnpm exec cypress run --component

# ✅ E2E 测试（需后端 + Vite 同时运行）
CYPRESS_BASE_URL=http://localhost:5173 \
CYPRESS_API_BASE=http://localhost:2025 \
pnpm exec cypress run --e2e

# ✅ 跑单个 spec 文件
pnpm exec cypress run --e2e --spec "cypress/e2e/db-engine-perf.cy.ts"

# ❌ 禁止：用临时脚本调 API 测性能
# ❌ 禁止：手动 curl 循环当测试
```

### 6.3 Cypress 性能测试铁律

> **性能结论必须出自 Cypress E2E 真实测试，Go benchmark 仅作补充参考。**
> 详见 [test-master-plan.md §一.3](./test-master-plan.md#13-铁律性能结论必须出自-cypress-e2e)

性能测试三要素（缺一不可）：
1. **相同硬件环境** — 同机同时段同负载
2. **相同测试负载** — 同任务数同流程同数据
3. **多次测量取中位** — 至少 3 次，取中位数

### 6.4 性能测试 spec 规范

每个性能测试 spec 必须：
- 有明确的 `describe` 描述测试场景和对比维度
- 用 `performance.now()` 计时（不是 `Date.now()`）
- 至少跑 3 次取中位数
- 输出结构化的性能指标（耗时 / 吞吐 / 峰值并发）
- 测试后清理状态（不污染下一次测试）

---

## 🆕 七、Go 基准测试（补充参考）

### 7.1 定位

Go benchmark 是**函数级性能参考**，用于：
- 快速对比两个算法实现的差异
- 验证单次优化的有效性（前后对比）
- 检测内存分配热点（`-benchmem`）

**Go benchmark 的数据不作为最终性能结论依据**，最终结论以 Cypress E2E 为准。

### 7.2 合法入口

```bash
# ✅ 单包 benchmark
bash scripts/test-go.sh -bench ./internal/service

# ✅ 指定 benchmark 函数
bash scripts/test-go.sh -bench=BenchmarkCreateBatch -benchmem ./internal/service

# ✅ 跑完整 benchmark（CI 用）
ENCV_TEST_FULL=1 bash scripts/test-go.sh -bench ./...
```

### 7.3 Benchmark 命名规范

```
Benchmark<场景描述>_<引擎/条件>_<规模>
```

示例：
- `BenchmarkCreateBatch_SQLite_100` — SQLite 引擎批量创建 100 任务
- `BenchmarkCreateBatch_Turso_1000` — Turso 引擎批量创建 1000 任务
- `BenchmarkUpdateProgress_DirtyThrottle_500` — 脏任务节流 500 任务

---

## 八、相关引用

- [scripts/test-go.sh](file:///workspace/scripts/test-go.sh) — Go 测试唯一入口 + 守卫
- [scripts/test-all-go.sh](file:///workspace/scripts/test-all-go.sh) — 模块化编排入口
- [dev-start-guard.ts](file:///workspace/app/encv-mobile/src/lib/dev-start-guard.ts) — Vite dev 守卫（同源设计）
- **前端 type check**：`pnpm exec npx vue-tsc --noEmit`（沙箱合法入口，详见 §二点五）

---

> 创建：2026-06-15（test-orchestration-defense）
> 更新：2026-06-17（新增 §二点五：vitest 不在沙箱跑）
