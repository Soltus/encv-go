# GitHub Actions CI 工作流铁律

> **核心原则：CI 工作流分 3 层（Layer1 必跑 / Layer2 + Layer3 标签触发），PR 自动触发只跑 Layer1 防恶意消耗。**
> **maintainer 决策合并前手动加 `ci:full` 标签触发 Layer2；加 `ci:e2e` 触发 Layer3。**
> **新测试系统（test-go.sh / test-all-go.sh + ENCV_TEST_FULL=1）是 CI 唯一合法入口。**

> 创建：2026-06-15（ci-workflow-anti-abuse-defense）

---

## 一、铁律（违反 = CI 红/绿语义错乱）

### 1.1 三层职责隔离（强制）

| 层 | Workflow | 触发事件 | 跑什么 | 决策者 | 时长 |
|----|----------|---------|--------|--------|------|
| **Layer1 必跑** | `pr-check.yml` | `pull_request` + `push` (main) | lint + 单包 go test + frontend vitest | GitHub 红/绿 | ~2-5min |
| **Layer2 标签** | `full-regression.yml` | 标签 `ci:full` + `workflow_dispatch` + `push` (main) | 全包 regression (`ENCV_TEST_FULL=1`) | maintainer | ~15-20min |
| **Layer3 E2E** | `e2e-integration.yml` | 标签 `ci:e2e` + `workflow_dispatch` + `push` (main) + `schedule` | 加密 roundtrip E2E | maintainer | ~20-30min |

### 1.2 防恶意消耗机制（3 重防御）

| 攻击向量 | 防御 | 效果 |
|---------|------|------|
| 陌生 fork spam PR | PR 只触发 Layer1（4 个 matrix 任务，~2-5 min 总时长） | 攻击成本低但 GitHub 配额消耗有上限 |
| 同一 PR 反复 push | workflow 自身 `concurrency: cancel-in-progress` | 新 push 自动取消旧 run |
| 标签滥用 | `pull_request` 而非 `pull_request_target`（无 secrets 暴露）+ 加 `concurrency` | 标签触发也受 PR 生命周期管控 |

**注意：不加 PR size 拦截** — 用户场景里大部分 PR 是巨型重构 / 跨包改造，size 拦截会误伤真实工作流。Layer1 单次成本有上限（~5 min × 4 matrix），可接受。恶意 PR 触发 Layer1 → 红灯 → 无法 merge → 自然终止。

### 1.3 标签命名约定（强制前缀 `ci:`）

| 标签 | 触发 Workflow | 颜色 | 含义 |
|------|---------------|------|------|
| `ci:full` | full-regression.yml | 绿 `#0E8A16` | 跑全包 regression |
| `ci:e2e` | e2e-integration.yml | 黄 `#FBCA04` | 跑 E2E |
| `ci:skip` | (未来扩展预留) | 红 `#D93F0B` | maintainer 显式跳过（暂未实现） |

**`ci:` 前缀必须保留** — 避免与 issue labels（`bug` / `enhancement` / `wontfix` 等）混淆。

**标签创建**：[scripts/create-ci-labels.sh](file:///workspace/scripts/create-ci-labels.sh) — 一次性创建（idempotent），仓库初始化时跑一次。

---

## 二、PR Review 流程（标准动作）

### 2.1 真 PR 流程

```
1. PR opened
   ↓
2. pr-check.yml 自动跑 Layer1 (2-5 min)
   ↓ Layer1 绿
3. maintainer review 代码
   ↓ 准备 merge
4. 加 ci:full 标签 → full-regression.yml 跑 Layer2 (15-20 min)
   ↓ Layer2 绿
5. (可选) 加 ci:e2e 标签 → e2e-integration.yml 跑 Layer3 (20-30 min)
   ↓ Layer3 绿（或跳过 E2E）
6. maintainer merge
   ↓
7. push to main → 3 个 workflow 都跑（最终验证）
```

### 2.2 4 种触发入口

| 入口 | 命令 | 适用 |
|------|------|------|
| **PR 标签** | 加 `ci:full` / `ci:e2e` 标签 | 真 PR 流程（**推荐**） |
| **Actions UI** | workflow 页面 → Run workflow | 紧急单独跑 main/dev |
| **Push to main** | merge 后自动触发全部 | merge 后最终验证 |
| **Schedule** (Layer3 only) | cron `0 4 * * *` | 每日 E2E 巡检 |

### 2.3 真 PR 加标签的命令

```bash
# 加 ci:full 标签
gh pr edit <PR-number> --add-label ci:full

# 同时加 ci:full + ci:e2e
gh pr edit <PR-number> --add-label ci:full,ci:e2e
```

---

## 三、新测试系统适配（强制）

### 3.1 Go 测试统一入口

[scripts/test-go.sh](file:///workspace/scripts/test-go.sh) + [scripts/test-all-go.sh](file:///workspace/scripts/test-all-go.sh) 是 CI 唯一合法入口：

| CI 场景 | 调用 | 退出码 |
|---------|------|--------|
| **Layer1 单包** | `bash scripts/test-go.sh ./internal/service` | 0=OK / 1=FAIL / 64=GUARD_REJECTED / 124=TIMEOUT / 137=OOM |
| **Layer2 全包** | `ENCV_TEST_FULL=1 bash scripts/test-all-go.sh` | 同上 |
| **Layer3 慢测** | `ENCV_TEST_FULL=1 bash scripts/test-go.sh ./internal/v2/plugins/video/ -run TestEncryptionE2E` | 同上 |

### 3.2 禁止

- ❌ CI workflow 直接 `go test ./...`（绕过守卫 → 沙箱断网）
- ❌ 改 `test-go.sh` 加 CI 跳过逻辑（同 dev-start-guard 收编原则）
- ❌ Layer2 不声明 `ENCV_TEST_FULL=1`（守卫 exit 64 抛错）
- ❌ `go test ./...` 写在 yaml 里（沙箱断网 + 守卫必抛错）

---

## 四、CI 中的降级规范

> **通用原则见 [graceful-degradation.md](./graceful-degradation.md) — 本节是 CI 场景下的子集。**

### 4.1 CI 降级的特殊要求

CI 是「功能是否真的可用」的第一道防线，所以降级可见性要求比运行时更严格：

| 要求 | 说明 |
|------|------|
| **Step Summary 必须有** | 不能只在日志里打一行，必须写 `$GITHUB_STEP_SUMMARY` |
| **必须用 ::warning:: / ::error::** | GitHub Actions 会高亮，翻页也能看到 |
| **步骤状态不能是绿色** | 降级了就应该是黄色（warning），不能假装成功 |
| **失败原因必须明确** | 不能只说"失败了"，要说清楚哪一步、为什么 |

### 4.2 三级降级在 CI 中的表现

| 级别 | CI 表现 |
|------|---------|
| **L1 功能禁用** | CI 直接红（fail），核心功能不能用就不能出包 |
| **L2 降级体验** | Step 黄（warning）+ Step Summary 明确说明缺了什么 |
| **L3 透明降级** | 可以绿，但日志里要有 debug 级别的记录 |

### 4.3 脚本编写规范

**错误（静默失败）**：
```bash
set +e
build-something || true
echo "done"
```

**正确（L2 降级，显式可观测）**：
```bash
set -e

BUILD_SUCCESS=0
build-something 2>&1 | tee /tmp/build.log && BUILD_SUCCESS=1 || BUILD_SUCCESS=0

if [ $BUILD_SUCCESS -eq 0 ]; then
  echo "::warning::可选功能 X 构建失败，降级方案已启用"
  echo "::warning::失败原因: $(tail -3 /tmp/build.log)"
  echo "## ⚠️ 可选功能 X 构建失败" >> $GITHUB_STEP_SUMMARY
  echo "" >> $GITHUB_STEP_SUMMARY
  echo "**降级影响**：哪些功能不可用、哪些还能用" >> $GITHUB_STEP_SUMMARY
  echo "" >> $GITHUB_STEP_SUMMARY
  echo "**失败原因**：" >> $GITHUB_STEP_SUMMARY
  echo '```' >> $GITHUB_STEP_SUMMARY
  tail -20 /tmp/build.log >> $GITHUB_STEP_SUMMARY
  echo '```' >> $GITHUB_STEP_SUMMARY
fi
```

---

## 五、应急（恶意消耗 attack 应对）

### 5.1 一键关停 workflow

```bash
gh workflow disable pr-check.yml
gh workflow disable full-regression.yml
gh workflow disable e2e-integration.yml

# 恢复
gh workflow enable pr-check.yml
gh workflow enable full-regression.yml
gh workflow enable e2e-integration.yml
```

### 5.2 cancel 正在跑的恶意 run

```bash
# 列出所有 running runs
gh run list --status in_progress

# cancel 指定 run
gh run cancel <run-id>
```

### 5.3 应急 commit 直接走 Layer1

恶意 PR 触发 Layer1 → 红灯 → 无法 merge → 自然终止（无需手动干预）。

---

## 六、与其他规则交叉

| 规则 | 关系 |
|------|------|
| [graceful-degradation.md](file:///workspace/.trae/rules/graceful-degradation.md) | **强相关** — 降级设计通用原则，本节是其子集 |
| [test-orchestration.md](file:///workspace/.trae/rules/test-orchestration.md) | **强相关** — CI 必须走 scripts/test-go.sh / test-all-go.sh |
| [development.md](file:///workspace/.trae/rules/development.md) | 弱相关（沙箱开发环境规范，与 CI 分离） |
| [android.md](file:///workspace/.trae/rules/android.md) | **强相关** — libsql 降级规则、Android 构建规范 |
| [combolite.md](file:///workspace/.trae/rules/combolite.md) | 无关 |
| [capacitor.md](file:///workspace/.trae/rules/capacitor.md) | 无关 |

---

## 七、引用

- [scripts/test-go.sh](file:///workspace/scripts/test-go.sh) — Go 测试唯一入口
- [scripts/test-all-go.sh](file:///workspace/scripts/test-all-go.sh) — 模块化测试编排
- [scripts/create-ci-labels.sh](file:///workspace/scripts/create-ci-labels.sh) — 一次性创建 ci:* 标签
- [scripts/ci-check-no-nodejs-crypto.sh](file:///workspace/scripts/ci-check-no-nodejs-crypto.sh) — Layer1 lint
- [.github/workflows/pr-check.yml](file:///workspace/.github/workflows/pr-check.yml) — Layer1
- [.github/workflows/full-regression.yml](file:///workspace/.github/workflows/full-regression.yml) — Layer2
- [.github/workflows/e2e-integration.yml](file:///workspace/.github/workflows/e2e-integration.yml) — Layer3
- [.github/workflows/lint-no-nodejs-crypto.yml](file:///workspace/.github/workflows/lint-no-nodejs-crypto.yml) — 独立 lint（Layer1 子集，可选保留）

> 拆分：2026-06-15（从单一 test.yml 拆三件套）
