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

## 四、静默 Fallback 禁令（违反 = 功能幻觉）

> **核心原则：宣称支持的功能必须真的可用，不能默默降级让用户以为有但实际没有。**
> **Fallback 必须显式、可观测、有决策记录。**

### 4.1 什么是「无脑静默 Fallback」（禁止）

**定义**：某功能编译/初始化失败时，`set +e` 吞掉错误，默默切换到降级方案，既不报错也不高亮警告，最终产出的二进制/APK 缺少该功能，但用户从表面完全看不出来。

**典型反面案例**：
```bash
set +e  # ❌ 整个步骤都吞错误
build-libsql || true  # ❌ 失败了当没事
echo "LIBSQL_READY=0" >> $GITHUB_ENV  # ⚠️ 只设变量但不醒目
# 后面继续构建，最终 APK 没有 libsql，但没人知道
```

**危害**：
- **功能幻觉**：开发者以为"libsql 已经集成了"，实际每次 CI 出的包都是 SQLite-only
- **问题被延迟发现**：直到线上出问题才发现"怎么没有向量搜索？"
- **技术债务累积**：失败的构建一直在静默失败，没人修，最后变成"历史遗留问题"

### 4.2 Fallback 合法性判断标准

| 判断维度 | ✅ 合法 Fallback | ❌ 无脑静默 Fallback |
|---------|----------------|-------------------|
| **声明方式** | 代码/配置中显式声明"这是可选功能" | 文档/代码宣称"支持 X"，但实际偷偷降级 |
| **失败可见性** | CI 总结中**红色/橙色高亮**显示"X 功能未包含" | 只有翻日志才能看到一行不起眼的 warning |
| **失败计数** | 计入 CI 警告/失败统计，有追踪 | 静默失败，无任何统计 |
| **决策记录** | 有明确的"为什么允许降级"的注释 | 不知道谁加的、为什么加 |
| **用户感知** | 运行时能明确看到"当前使用的是降级方案" | 用户完全不知道功能被降级了 |

### 4.3 合法 Fallback 的 3 个必要条件

**SHALL** 满足全部 3 条才算合法：

1. **显式声明可选**：在代码注释 / 配置文件 / 文档中明确说明"该功能是可选的，失败时降级到 X"
2. **CI 高亮可见**：
   - 使用 `::warning::` 或 `::error::` 输出（GitHub Actions 会高亮显示）
   - 写入 `$GITHUB_STEP_SUMMARY`，在 workflow 总结页面一眼可见
   - 步骤显示为黄色（warning）而非绿色（success）
3. **有追踪机制**：
   - 输出明确的"为什么失败"的诊断信息
   - 有对应的 issue / TODO 跟踪修复
   - 不能无限期静默失败

### 4.4 核心功能 vs 可选功能

| 功能类型 | 定义 | Fallback 策略 |
|---------|------|-------------|
| **核心功能** | 产品主要价值所在，用户默认认为应该有 | **禁止静默 fallback**，失败直接让 CI 红 |
| **可选增强** | 锦上添花的功能，没有也能用 | 允许 fallback，但必须满足 4.3 的 3 个条件 |
| **实验性功能** | 还在开发中，默认关闭 | 允许 fallback，但必须明确标记为实验性 |

**本项目当前分类**：
- ✅ **核心功能**：加密/解密、文件管理、任务系统
- ⚠️ **可选增强**：libsql 向量搜索、MPV 插件、OpenList 插件
- 🧪 **实验性功能**：AI Agent、自动化测试

### 4.5 CI 脚本编写规范

**错误写法（静默失败）**：
```bash
set +e
build-something || true
echo "done"
```

**正确写法（显式 fallback）**：
```bash
set -e

# 尝试构建可选功能
BUILD_SUCCESS=0
build-something && BUILD_SUCCESS=1 || BUILD_SUCCESS=0

if [ $BUILD_SUCCESS -eq 0 ]; then
  echo "::warning::可选功能 X 构建失败，将使用降级方案"
  echo "::warning::失败原因: $(tail -5 /tmp/build.log)"
  echo "## ⚠️ 可选功能 X 构建失败" >> $GITHUB_STEP_SUMMARY
  echo "将使用降级方案 Y。" >> $GITHUB_STEP_SUMMARY
  echo "" >> $GITHUB_STEP_SUMMARY
  echo "失败原因：" >> $GITHUB_STEP_SUMMARY
  echo '```' >> $GITHUB_STEP_SUMMARY
  tail -20 /tmp/build.log >> $GITHUB_STEP_SUMMARY
  echo '```' >> $GITHUB_STEP_SUMMARY
fi
```

### 4.6 本项目已知的静默 Fallback 清单

| 位置 | 功能 | 当前状态 | 需修复 |
|------|------|---------|-------|
| `android.yml` libsql 步骤 | libsql 向量搜索 | 静默失败 + 降级 SQLite | ✅ 是 |
| `android.yml` MPV 插件步骤 | MPV 视频播放器 | `continue-on-error: true` + 有日志 | ⚠️ 半合法（插件本身是可选的） |
| `android.yml` OpenList 插件步骤 | OpenList 插件 | `continue-on-error: true` + 有日志 | ⚠️ 半合法 |

> **原则**：插件类的可选功能可以用 `continue-on-error: true`，但**核心/宣称支持的功能不能静默失败**。
> libsql 是"宣称支持的功能"（文档里写了支持），所以不能静默失败。

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
| [test-orchestration.md](file:///workspace/.trae/rules/test-orchestration.md) | **强相关** — CI 必须走 scripts/test-go.sh / test-all-go.sh |
| [development.md](file:///workspace/.trae/rules/development.md) | 弱相关（沙箱开发环境规范，与 CI 分离） |
| [android.md](file:///workspace/.trae/rules/android.md) | **强相关** — libsql fallback 规则、Android 构建规范 |
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
