# Go 测试编排铁律（来自沙箱断网实战踩坑）

> **核心原则：Go 测试必须模块化（单包 + -short）跑，禁止全包 `./...` 直跑沙箱。**
> **沙箱跑 `go test ./...` 动辄 380s+，耗尽网络配额触发断网。**
> **test-go.sh / test-all-go.sh 是唯一合法入口；违反者直接抛错 exit 64。**

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

### 1.2 守卫实现位置

**代码层（双层防护）**：

1. **Shell 层**：[scripts/test-go.sh:52-100](file:///workspace/scripts/test-go.sh#L52-L100) `is_multipkg` 检测 + exit 64
2. **推荐入口**：[scripts/test-all-go.sh:80-148](file:///workspace/scripts/test-all-go.sh#L80-L148) 默认走模块化（go list 拿包 + 循环单包）

**为什么不写 Go init() 守卫**：Go init() 必须被 import 才执行；写 init() 的 package 必须被所有 test 包 import → 全局污染 / 不可控 / 易被 `go test -run` 规避。Shell 守卫在 go test 进程启动**之前**拦截，无法绕过。

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
| [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) | 弱相关（vitest 模块化测试可参考本规则） |

---

## 六、相关引用

- [scripts/test-go.sh](file:///workspace/scripts/test-go.sh) — Go 测试唯一入口 + 守卫
- [scripts/test-all-go.sh](file:///workspace/scripts/test-all-go.sh) — 模块化编排入口
- [dev-start-guard.ts](file:///workspace/app/encv-mobile/src/lib/dev-start-guard.ts) — Vite dev 守卫（同源设计）

> 创建：2026-06-15（test-orchestration-defense）
