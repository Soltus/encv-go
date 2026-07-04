# 沙箱执行：克隆三个 fork 到 app/openlist/ 固定子目录并验证

> **核心目标**：在沙箱中实际执行 fork 克隆（主 fork 走 build script 自动 clone，两个副 fork 手动 clone），验证 `app/openlist/.gitignore` 整体忽略生效、`build-openlist-aar.sh` 的新 WORK_DIR 默认值正确、go.mod 的 `replace github.com/Soltus/encv-go => ../../../` 相对路径天然解析成功。
>
> **前置**：所有代码 / 文档 / spec 改动已在 [fork-clone-path-refactor-to-app-openlist.md](file:///workspace/.trae/documents/fork-clone-path-refactor-to-app-openlist.md) 落地，本计划只负责「沙箱验证 + 真正执行 clone」。

---

## 一、当前状态盘点

| 维度 | 状态 | 证据 |
|------|------|------|
| `app/openlist/.gitignore` 三个 fork 目录 | ✅ 已落地 | [app/openlist/.gitignore](file:///workspace/app/openlist/.gitignore) 内容：`/dist`、`/Hi-Sillot-OpenList/`、`/Hi-Sillot-OpenList-Frontend/`、`/K-Sillot-OpenList-Desktop/` |
| `scripts/build-openlist-aar.sh` WORK_DIR 块 | ✅ 已落地（line 139-152） | [build-openlist-aar.sh:139-152](file:///workspace/scripts/build-openlist-aar.sh#L139-L152) 默认 `<repo>/app/openlist/Hi-Sillot-OpenList`；`OPENLIST_FORK_WORK_DIR` env 覆盖保留 |
| `scripts/build-openlist-aar.sh` verify-replace 块 | ✅ 已落地（line 181-205） | [build-openlist-aar.sh:181-205](file:///workspace/scripts/build-openlist-aar.sh#L181-L205) `*../../../*` 模式匹配 → 静默放行；非相对路径 → sed 兜底为绝对路径 |
| `scripts/build-openlist-aar.ps1` WORK_DIR 块 | ✅ 已落地（line 218-223） | [build-openlist-aar.ps1:218-223](file:///workspace/scripts/build-openlist-aar.ps1#L218-L223) PowerShell 镜像 |
| `scripts/build-openlist-aar.ps1` verify-replace 块 | ✅ 已落地（line 240-274） | [build-openlist-aar.ps1:240-274](file:///workspace/scripts/build-openlist-aar.ps1#L240-L274) |
| `app/openlist/README.md` §3/§4.2/§4.4/§11 文档 | ✅ 已落地 | [app/openlist/README.md §4.4](file:///workspace/app/openlist/README.md#L149-L186) 三个 fork 的 clone 命令模板 |
| `scripts/README.md` line 149-151 | ✅ 已落地 | [scripts/README.md:149-151](file:///workspace/scripts/README.md#L149-L151) 引用 `app/openlist/Hi-Sillot-OpenList/` |
| `spec.md` R3 风险条目 | ✅ 已标记为已解决 | [spec.md:459](file:///workspace/.trae/specs/integrate-openlist-as-combolite-plugin/spec.md#L459) |
| `app/openlist/build-encv-desktop.ps1` | ✅ 故意未动（D5 决定） | README §9 标注「已废弃」 |
| **三个 fork 实际 clone** | ❌ **未执行** | `/workspace/app/openlist/` 目录内仍只有 `.gitignore`、`README.md`、`build-encv-desktop.ps1` |
| **完整 build 跑通** | ❌ **未执行** | 上次会话只跑了静态验证（bash 语法、gitignore 内容、WORK_DIR 计算） |

---

## 二、Phase 1 探索补充

### 2.1 沙箱可用的 clone 方式

| 路径 | 适用 | 命令模板 |
|------|------|---------|
| HTTPS 匿名 | fork 公开仓库 | `git clone --depth 1 --branch <br> https://github.com/<org>/<repo>.git <dir>` |
| HTTPS + GITHUB_TOKEN | 私有仓库 / 加速 | `git clone --depth 1 --branch <br> https://x-access-token:${GITHUB_TOKEN}@github.com/<org>/<repo>.git <dir>`（同 [README §10](file:///workspace/app/openlist/README.md#L379-L430) 推荐的 URL 注入） |
| `gh` CLI | 沙箱有 `gh` 时 | `gh repo clone <org>/<repo> <dir> -- --depth 1 --branch <br>` |

> **优先 HTTPS 匿名**：三个 fork 都是公开仓库；GITHUB_TOKEN 仅在私有访问 / API rate limit 时使用。

### 2.2 沙箱环境前置检查

```bash
# 必须：git 已安装
command -v git >/dev/null 2>&1 || echo "MISSING git"

# 可选：GITHUB_TOKEN（影响私有仓库）
[[ -n "${GITHUB_TOKEN:-}" ]] && echo "GITHUB_TOKEN set" || echo "GITHUB_TOKEN unset (anonymous clone only)"

# 可选：gh CLI（备用 clone 方式）
command -v gh >/dev/null 2>&1 && echo "gh available" || echo "gh not available"
```

### 2.3 fork 仓库与目标子目录的对应表

| GitHub 仓库 | 默认 branch | 目标本地子目录 | 用途 |
|-------------|-------------|----------------|------|
| `Hi-Sillot/OpenList` | `dev` | `app/openlist/Hi-Sillot-OpenList/` | 主 fork（build script 自动 clone） |
| `Hi-Sillot/OpenList-Frontend` | `main` | `app/openlist/Hi-Sillot-OpenList-Frontend/` | 前端 fork（i18n 同步源） |
| `K-Sillot/OpenList-Desktop` | `main` | `app/openlist/K-Sillot-OpenList-Desktop/` | 桌面端 fork（已废弃，仅参考） |

> **注意**：K-Sillot 仓库可能 404（用户曾提到「项目已废弃」），失败时按 D1 决策静默跳过并记录。

---

## 三、Phase 2 决策

| # | 决策 | 取值 | 理由 |
|---|------|------|------|
| **D1** | 主 fork 实际 clone | ✅ 必做 | build script 验证需要真实存在；沙箱 `git clone` 一次性可走通 |
| **D2** | 副 fork clone（Frontend + Desktop） | ✅ 必做 | 验证 `.gitignore` 三个目录都生效 |
| **D3** | 完整 build 跑通 | ⚠️ 可选 | 需要 NDK + Go + Java 全套工具链，沙箱未必齐全；用 `CGO_ENABLED=0 go vet ./openlistlib/` 替代验证 `replace` 路径 |
| **D4** | 失败时清理 | ❌ 失败不删除 | fork 是工作区，即使 clone 失败也是诊断素材；最多在错误日志中说明 |
| **D5** | 主 fork clone 方式 | 通过 build script 跑 | 比手动 `git clone` 更贴近真实工作流；脚本会写日志便于验证 |
| **D6** | 副 fork clone 方式 | 手动 `git clone` | build script 不管副 fork |
| **D7** | 网络失败处理 | 单次失败 → 记录 + 继续下一个 fork | 沙箱网络可能限速；不让单个 404 阻塞整个流程 |

---

## 四、Phase 3 落地动作

### 4.1 沙箱环境检查（V0）

```bash
# 快速 sanity check
cd /workspace
command -v git && git --version
command -v go  && go version   || echo "go not in PATH (D3 完整 build 跳过)"
[[ -n "${GITHUB_TOKEN:-}" ]] && echo "TOKEN ok" || echo "TOKEN unset"
```

### 4.2 静态验证已落地的改动（V1-V5）

```bash
# V1: bash 语法
bash -n /workspace/scripts/build-openlist-aar.sh && echo "OK: bash syntax"

# V2: .gitignore 覆盖 3 个 fork 目录
grep -E '^/(Hi-Sillot-OpenList|Hi-Sillot-OpenList-Frontend|K-Sillot-OpenList-Desktop)/$' /workspace/app/openlist/.gitignore

# V3: WORK_DIR 默认值正确
SCRIPT_DIR="$(cd /workspace/scripts && pwd)"
_REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
echo "expected: ${_REPO_ROOT}/app/openlist/Hi-Sillot-OpenList"
# 与 build-openlist-aar.sh:148 实际计算结果比对

# V4: verify-replace 块存在 + 模式正确
grep -A 5 "Verify fork go.mod relative replace" /workspace/scripts/build-openlist-aar.sh | head -20

# V5: spec R3 标记为已解决
grep -E "R3.*已解决" /workspace/.trae/specs/integrate-openlist-as-combolite-plugin/spec.md
```

### 4.3 实际 clone 主 fork（验证 build script 路径）

**方式 A：走 build script 触发**（贴近真实 CI 流程）

```bash
cd /workspace
# build script 内部会 rm -rf + git clone
# 截取关键日志行（不需要完整 build）
bash scripts/build-openlist-aar.sh --output /tmp/openlist-aar-smoke-test 2>&1 | \
    grep -E "Workspace|Clone|relative replace|Verify fork" | head -20
```

**方式 B：手动 clone**（更轻量，无 NDK 依赖）

```bash
cd /workspace/app/openlist
git clone --depth 1 --branch dev https://github.com/Hi-Sillot/OpenList.git Hi-Sillot-OpenList
```

> **推荐 A**：验证 build script 自身的逻辑正确性；不需要 NDK 工具链也可（`--output` 路径不要求可达，clone 阶段先于 bind）。

### 4.4 验证 go.mod 相对 replace 解析（V6）

```bash
cd /workspace/app/openlist/Hi-Sillot-OpenList

# 检查 fork go.mod 仍有 ../../../ 相对 replace
grep -E '^replace[[:space:]]+github\.com/Soltus/encv-go' go.mod
# 期望输出: replace github.com/Soltus/encv-go => ../../../

# 验证 go 工具链认这个相对路径解析到 /workspace
CGO_ENABLED=0 go list -m github.com/Soltus/encv-go 2>&1
# 期望输出: github.com/Soltus/encv-go -> /workspace
# 或: error（如果 go.sum 缺 checksum；这不算失败，只要 grep 看到 ../../../ 即视为相对路径生效）
```

### 4.5 手动 clone 两个副 fork（V7）

```bash
cd /workspace/app/openlist

# 前端 fork（公开仓库，匿名 clone）
git clone --depth 1 --branch main https://github.com/Hi-Sillot/OpenList-Frontend.git Hi-Sillot-OpenList-Frontend \
    || echo "WARN: OpenList-Frontend clone failed (network or private)"

# 桌面 fork（可能 404，按 D7 跳过）
git clone --depth 1 --branch main https://github.com/K-Sillot/OpenList-Desktop.git K-Sillot-OpenList-Desktop \
    || echo "WARN: K-Sillot/OpenList-Desktop clone failed (repo may not exist)"
```

### 4.6 验证主仓库 git 状态洁净（V8）

```bash
cd /workspace
git status --porcelain | grep -E "app/openlist/" | grep -v "README.md" | grep -v "\.gitignore" | grep -v "build-encv-desktop.ps1"
# 期望：空（fork 目录全被 .gitignore 排除）

# 也可显式确认 .gitignore 命中
git check-ignore -v app/openlist/Hi-Sillot-OpenList/.git/config \
                 app/openlist/Hi-Sillot-OpenList-Frontend/.git/config \
                 app/openlist/K-Sillot-OpenList-Desktop/.git/config
# 期望：三行都打印对应的 .gitignore:line:pattern
```

### 4.7 验证 build script 的 env 覆盖点（V9）

```bash
cd /workspace

# 临时覆盖 WORK_DIR
OPENLIST_FORK_WORK_DIR=/tmp/fork-override-test bash scripts/build-openlist-aar.sh --output /tmp/aar-test 2>&1 | \
    grep -E "Workspace|/tmp/fork-override-test" | head -5
# 期望：Workspace 日志显示 /tmp/fork-override-test

# 清理临时覆盖产物
rm -rf /tmp/fork-override-test /tmp/aar-test
```

### 4.8 （可选）跑一次 CGO_ENABLED=0 go vet 替代完整 build

> 完整 build 需要 NDK + Java，沙箱未必有；go vet 用 Go 标准库足够验证 go.mod 相对 replace 是否能被 go 工具链解析。

```bash
cd /workspace/app/openlist/Hi-Sillot-OpenList
CGO_ENABLED=0 go vet ./openlistlib/... 2>&1 | head -30
# 期望：
#   - exit 0 + 无输出 → 相对 replace 天然解析成功
#   - 或报 missing go.sum entry（不致命，跑 go mod download 即可）
```

---

## 五、Verification 清单

| # | 验证项 | 期望 | 失败处理 |
|---|--------|------|---------|
| V0 | git / go / GITHUB_TOKEN 沙箱环境 | 工具齐 + GITHUB_TOKEN 可选 | 缺 git → 整个计划无法执行；缺 go → 跳过 V6/V8 |
| V1 | bash 语法 | 0 退出 | 改 build script 语法错误 |
| V2 | .gitignore 覆盖三 fork | 三行全匹配 | 补 .gitignore 行 |
| V3 | WORK_DIR 默认值 | `/workspace/app/openlist/Hi-Sillot-OpenList` | 改 build script 路径计算 |
| V4 | verify-replace 块 | 模式 `*../../../*` 存在 | 改 build script 验证块 |
| V5 | spec R3 已解决 | 匹配 "R3.*已解决" | 改 spec.md |
| V6 | go.mod 相对 replace 解析 | `../../../` 可见 + go list 解析到 /workspace | 检查 fork go.mod 是否被改 |
| V7 | 三个 fork clone | 全部完成 | K-Sillot 失败按 D7 跳过 |
| V8 | 主仓库 git 干净 | `git status` 无 fork 痕迹 | 检查 .gitignore 模式 |
| V9 | OPENLIST_FORK_WORK_DIR 覆盖 | 临时路径被采纳 | 改 build script 覆盖块 |

---

## 六、Risk & Decision

| # | 风险 | 缓解 |
|---|------|------|
| **R1** | 沙箱网络限速导致 clone 慢 / 失败 | `git clone --depth 1` 最小化流量；单个失败不阻塞；记录日志供后续诊断 |
| **R2** | fork clone 占用 `/workspace` 600MB+ 空间 | `.gitignore` 隔离 → 不污染主仓库；如空间紧可改 `OPENLIST_FORK_WORK_DIR=/tmp/sandbox-fork` |
| **R3** | 沙箱无 NDK 工具链 → build 脚本后半段 fail | 不强求完整 build；用 `go vet` 替代验证 replace 解析；build script 自身行为（clone + 路径设置）可独立验证 |
| **R4** | `K-Sillot/OpenList-Desktop` 仓库 404 | 按 D7 跳过 + 记录 WARN；不阻塞主 fork clone |
| **R5** | fork clone 写到 `app/openlist/` 后，意外 `git add` 误提交 | 跑 V8 验证 `.gitignore` 真的命中；若失误 commit 可 `git rm --cached -r` 撤回 |

---

## 七、Sequence（执行顺序）

1. 跑 §四.1 沙箱环境检查（V0）
2. 跑 §四.2 静态验证（V1-V5），快速确认代码改动已落地
3. 跑 §四.3 实际 clone 主 fork（触发 build script 或手动）
4. 跑 §四.4 验证 go.mod 相对 replace 解析（V6）
5. 跑 §四.5 手动 clone 两个副 fork（V7）
6. 跑 §四.6 验证 git 状态洁净（V8）
7. 跑 §四.7 验证 env 覆盖点（V9）
8. （可选）跑 §四.8 go vet 替代完整 build
9. 写最终回复总结 V0-V9 通过情况

---

## 八、输出物

- **成功路径**：V0-V9 全过 + 三个 fork 全部 clone + git status 干净 → 用户得到「沙箱已就绪」确认
- **部分成功路径**（如 K-Sillot 404）：V0-V8 过 + 副 fork 中至少一个失败 → 用户得到「主 fork 完整 + 副 fork 部分就绪」报告
- **失败路径**：V0 缺 git / V6 相对 replace 不工作 → 用户得到「需修复 build script」的明确指示
