# 修复 test.yml actions/checkout@v4 报错计划

## 问题诊断

### 错误现象
```
actions/checkout@v4 → git fetch 失败（重试 3 次）
fetch spec: +refs/heads/4/merge*:refs/remotes/origin/4/merge*
exit code 1
```

### 根因链路

1. **触发场景**：工作流由 `pull_request` 事件触发（PR #4）
2. **问题表达式**：3 个 job 共用的 `ref: ${{ inputs.branch || github.ref_name }}`
3. **失败机制**：
   - `actions/checkout@v4` 收到显式 `ref` 参数后，基于该值构造 git fetch spec
   - 在 PR 场景下，checkout 内部尝试解析 PR merge ref（格式 `refs/pull/{number}/merge`）
   - shallow clone（默认 `fetch-depth: 1`）+ merge ref 不存在 → 构造出异常 fetch spec → 失败
4. **影响范围**：layer1 / layer2 / layer3 全部 3 个 job 的 checkout 步骤均使用相同配置，全部会失败

## 修复方案：事件感知 ref 选择

### 核心思路
用 `github.sha`（绝对 commit SHA）替代 `github.ref_name`，彻底绕过 checkout 对 merge ref 的猜测逻辑。同时保留 `workflow_dispatch` 的 `inputs.branch` 手动指定能力。

### 各事件的 ref 取值策略

| 触发事件 | ref 取值 | 原因 |
|---------|----------|------|
| `workflow_dispatch` | `inputs.branch \|\| 'main'` | 用户通过 UI 指定分支；空值 fallback 到 main |
| `pull_request` | `github.sha` | PR merge commit SHA，始终存在且可靠 |
| `push` | `github.sha` | push 的 commit SHA，始终存在 |
| `schedule` | `'main'` | 定时任务固定用主分支 |

### 具体修改（3 处，模式完全相同）

#### 修改点 1：layer1-quick-tests job（当前 L44-L48）

```yaml
# 修改前
- name: Checkout repository
  uses: actions/checkout@v4
  with:
    ref: ${{ inputs.branch || github.ref_name }}

# 修改后
- name: Checkout repository
  uses: actions/checkout@v4
  with:
    ref: ${{ github.event_name == 'workflow_dispatch' && inputs.branch || github.sha }}
    fetch-depth: 1
```

> **workflow_dispatch 行为说明**：
> - 用户在 GitHub Actions UI 填写 `branch` 字段（如 `dev`）→ checkout `dev` 分支最新 commit
> - 用户留空 `branch`（默认值 `''`，falsy）→ 表达式 fallback 到 `github.sha`→ 实际 checkout 到 `main` 分支（因为 workflow_dispatch 的 sha 就是目标分支 HEAD）
> - `skip_layer1` / `skip_e2e` 输入不受影响，仍由各 job 的 `if` 条件控制

#### 修改点 2：layer2-full-regression job（当前 L127-L131）

同上模式。

#### 修改点 3：layer3-e2e-integration job（当前 L196-L200）

同上模式。

## 实施步骤

1. [ ] 修改 layer1-quick-tests 的 checkout 配置（L44-L48）
2. [ ] 修改 layer2-full-regression 的 checkout 配置（L127-L131）
3. [ ] 修改 layer3-e2e-integration 的 checkout 配置（L196-L200）
4. [ ] 验证 YAML 语法正确性
5. [ ] 提交修复并通过 GitHub Actions UI 手动触发验证

## 验证方式

修复提交后，通过 GitHub Actions UI 使用 `workflow_dispatch` 手动触发：
- **测试 A**：不填 branch（留空）→ 应正常 checkout main 并跑完全部测试
- **测试 B**：填写 branch = `dev`→ 应正常 checkout dev 分支并执行
- **测试 C**：创建/更新一个 PR → pull_request 触发应不再报 fetch 错误

## 为什么这个修复有效

- **`github.sha` 是绝对存在的 commit SHA**：不需要 ref 名字解析，不触发 merge ref 猜测逻辑
- **shallow clone 友好**：SHA checkout 只需要单个 commit，`fetch-depth: 1` 足够
- **workflow_dispatch 完整保留**：`inputs.branch` 优先级最高，手动指定分支仍然生效
- **最小改动**：只改 3 处 checkout 的 `ref` 和新增 `fetch-depth`，不影响其他步骤
