# Fork 本地克隆路径重构：从 /tmp 改到 app/openlist/ 固定子目录

> **核心目标**：让 `Hi-Sillot/OpenList` fork 的 `go.mod` 里 `replace github.com/Soltus/encv-go => ../../../` 这个相对路径**天然成立**，不再需要 sed 改写。同时为另外两个本地维护的 fork（前端 / 桌面）预留固定子目录。

## 一、Phase 1 探索总结

| 证据 | 出处 | 含义 |
|------|------|------|
| `$OpenListDistDir = Join-Path $BuildRoot "OpenList\dist\windows"` | [app/openlist/build-encv-desktop.ps1:16](file:///workspace/app/openlist/build-encv-desktop.ps1#L16) | 桌面端 Tauri 脚本**早就**把 fork 假设在 `$BuildRoot`（= `$PSScriptRoot` = `app/openlist/`）下的 `OpenList\` 子目录 |
| `replace github.com/Soltus/encv-go => ../../../` | Hi-Sillot fork go.mod | 相对路径**只有**在 fork 位于 `app/openlist/OpenList/` 时才能解析到 encv-go 根（`/workspace`） |
| `WORK_DIR="${TMPDIR:-/tmp}/openlist-aar-build"` | [scripts/build-openlist-aar.sh:139](file:///workspace/scripts/build-openlist-aar.sh#L139) | 当前 build 脚本**违反**上述设计，clone 到 /tmp |
| `sed ... 's\|^replace github.com/Soltus/encv-go.*|replace github.com/Soltus/encv-go => ${ENCV_GO_ROOT}\|'` | [scripts/build-openlist-aar.sh:170-178](file:///workspace/scripts/build-openlist-aar.sh#L170-L178) | **绕路代码**：因为 fork 不在标位置，必须用 sed 把相对路径改成绝对路径 |
| R3 风险：「`replace ... ../../../` 路径在 encv-mobile 仓库下不成立」 | [spec.md:459](file:///workspace/.trae/specs/integrate-openlist-as-combolite-plugin/spec.md#L459) | 已记录的已知 hack |
| `app/openlist/.gitignore` 只有 `/dist` | [app/openlist/.gitignore](file:///workspace/app/openlist/.gitignore) | 当前没有任何对 fork 目录的 ignore |
| `Hi-Sillot/OpenList-Frontend` / `K-Sillot/OpenList-Desktop` / `K-Sillot/OpenList-Mobile` 三层 fork 关系 | [app/openlist/README.md:55-98](file:///workspace/app/openlist/README.md#L55-L98) | 用户维护的 3 个 fork：主 fork (build 依赖) + 前端 (i18n 同步) + 桌面 (Tauri 参考) |

**结论**：build 脚本当前路径选择是个**历史 hack**，原始设计意图是「fork 就在 `app/openlist/` 下」。

## 二、Phase 2 决策（已与你确认）

| # | 决策 | 取值 | 备注 |
|---|------|------|------|
| D1 | 主 fork 目录名 | `app/openlist/Hi-Sillot-OpenList/` | 与 GitHub 仓库名同形；你指定 |
| D2 | 路径可覆盖 | 保留 `OPENLIST_FORK_WORK_DIR` env | 灵活 + 默认简单 |
| D3 | 三个 fork 都加 `.gitignore` | 手动 clone 的也加 | 一行一个，零成本 |
| D4 | 移除 sed replace 段 | 相对路径天然成立 | 删 hack，恢复设计 |
| D5 | `build-encv-desktop.ps1` | **不动** | 已废弃（README §9 标注）；Tauri 流程不在 Android AAR 链路 |
| D6 | 重复 build 策略 | 保持 `rm -rf + clone --depth 1` | 简单可靠；增量 fetch 优化留作后续 |
| D7 | 副 fork 目录布局 | `Hi-Sillot-OpenList-Frontend/` + `K-Sillot-OpenList-Desktop/` | 显式前缀 + 仓库名，避免歧义 |

## 三、Phase 3 落地动作

### 3.1 `/workspace/app/openlist/.gitignore` 扩展

当前内容（line 1）：
```
/dist
```

修改后：
```
/dist
/Hi-Sillot-OpenList/
/Hi-Sillot-OpenList-Frontend/
/K-Sillot-OpenList-Desktop/
```

> 三个 fork 目录整体忽略（含其 `.git/`），不会污染主仓库 `git status`。`/dist` 保留（桌面端遗留产物，仍存在但不再维护）。

### 3.2 `/workspace/scripts/build-openlist-aar.sh` 修改

**line 139** `WORK_DIR` 路径：

旧：
```bash
WORK_DIR="${TMPDIR:-/tmp}/openlist-aar-build"
```

新：
```bash
# 默认 fork 克隆到 app/openlist/Hi-Sillot-OpenList/（与 fork go.mod 的 replace ../../../ 相对路径匹配）。
# OPENLIST_FORK_WORK_DIR 覆盖：CI runner 想用 /cache/fork 复用 clone 时设置。
if [[ -n "${OPENLIST_FORK_WORK_DIR:-}" ]]; then
    WORK_DIR="${OPENLIST_FORK_WORK_DIR}"
else
    _REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
    WORK_DIR="${_REPO_ROOT}/app/openlist/Hi-Sillot-OpenList"
fi
log "== Workspace =="
log "  ${WORK_DIR}"
```

**line 144** `rm -rf "${SRC_DIR}"` 保留（清空旧 clone 后重新拉）。

**line 170-178** sed replace 段：删除，因为 `../../../` 在新位置下天然成立。替换为日志：

```bash
log "== Verify fork go.mod relative replace resolves correctly =="
# fork 应位于 app/openlist/Hi-Sillot-OpenList/，这样 go.mod 的
# `replace github.com/Soltus/encv-go => ../../../` 解析到 encv-go 根（/workspace）。
_REL_REPLACE="$(grep -E '^replace[[:space:]]+github\.com/Soltus/encv-go[[:space:]]+=>' "${GOMOD}" 2>/dev/null | head -n 1)"
if [[ "${_REL_REPLACE}" == *"/../"* ]] || [[ "${_REL_REPLACE}" == "../../../" ]]; then
    log "  (relative replace detected: ${_REL_REPLACE} → expected to resolve to /workspace)"
else
    log "  WARN: relative replace not found, fork go.mod may have been modified upstream"
    log "        got: ${_REL_REPLACE:-<no replace line>}"
fi
```

**Patch 2 段**（line 180-207，补 `require golang.org/x/mobile`）**保留**——这是为了 `gomobile bind` 通过，与路径无关。

**A2/B2 兜底**（line 347-401）**保留**——fork 历史版本的容错，不影响新 fork。

### 3.3 `/workspace/scripts/build-openlist-aar.ps1` 同步

**line 217 附近** `$workDir` 路径：

旧：
```powershell
$tmpRoot = $env:TMPDIR; if (-not $tmpRoot) { $tmpRoot = $env:TEMP }; if (-not $tmpRoot) { $tmpRoot = '/tmp' }
$workDir  = Join-Path $tmpRoot 'openlist-aar-build'
```

新：
```powershell
if ($env:OPENLIST_FORK_WORK_DIR) {
    $workDir = $env:OPENLIST_FORK_WORK_DIR
} else {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $workDir  = Join-Path (Join-Path $repoRoot 'app\openlist') 'Hi-Sillot-OpenList'
}
```

**sed replace 段**同样删除。

### 3.4 `/workspace/app/openlist/README.md` 文档更新

**§3 关系图**（line 93） Hi-Sillot/OpenList 行加注本地路径：
```
| **Hi-Sillot/OpenList** | 个人 fork ... 唯一被 build script clone（本地路径：`app/openlist/Hi-Sillot-OpenList/`） |
```

**§3 关系图** 新增 F2/R2 行的本地路径注解：
```
| **Hi-Sillot/OpenList-Frontend** | 本地路径：`app/openlist/Hi-Sillot-OpenList-Frontend/`（手动 clone，i18n 同步源） |
| **K-Sillot/OpenList-Desktop** | 本地路径：`app/openlist/K-Sillot-OpenList-Desktop/`（手动 clone，Tauri 参考；项目已废弃但仓库保留） |
```

**§4.2 沙箱推送命令模板**（line 116-130）改为：
```bash
# 1. 克隆到固定目录（影响 build script 路径解析）
cd /workspace/app/openlist
git clone --branch dev \
    https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git \
    Hi-Sillot-OpenList

# 2. 修改 + 提交
cd Hi-Sillot-OpenList
... (与原 line 122-125 相同)
```

**新增 §4.4 「三个 fork 的本地布局」**：
```bash
# 三层 fork 各自的固定路径（与 GitHub 仓库名同形，存放在 app/openlist/ 下）
cd /workspace/app/openlist

git clone --branch dev https://github.com/Hi-Sillot/OpenList.git          Hi-Sillot-OpenList         # 主 fork（build script 依赖）
git clone --branch main https://github.com/Hi-Sillot/OpenList-Frontend.git Hi-Sillot-OpenList-Frontend # 前端 fork（i18n 同步源）
git clone --branch main https://github.com/K-Sillot/OpenList-Desktop.git K-Sillot-OpenList-Desktop  # 桌面 fork（Tauri 参考，已废弃）

# 参考仓库（不参与集成，不需本地 clone）
# K-Sillot/OpenList-Mobile  ← 在线浏览即可
```

> **不存在时不会自动 clone**——你 fork 跟上游有 commit 差时手动维护；build script 只会 clone/更新 Hi-Sillot-OpenList（因为只有它参与 AAR build）。

**§11 故障表**（line 406-407）改写：
```
| 11 | `undefined: LogCallback` ... | 在 `app/openlist/Hi-Sillot-OpenList/openlistlib/event.go` 工作；commit `c2424d2` |
| 12 | `# github.com/mattn/go-sqlite3` ... | 在 `app/openlist/Hi-Sillot-OpenList/` 工作；commit `404daf0` |
```

### 3.5 `/workspace/scripts/README.md` 同步

149 行附近引用 `/tmp/openlist-aar-build/openlist/` 的位置替换为 `app/openlist/Hi-Sillot-OpenList/`。

### 3.6 spec `integrate-openlist-as-combolite-plugin/spec.md` 更新

**line 459 R3 风险**：
```
| R3 | ~~`replace github.com/Soltus/encv-go => ../../../` 路径在 encv-mobile 仓库下不成立~~ → **已解决**：fork 改克隆到 `app/openlist/Hi-Sillot-OpenList/`，相对路径天然成立；build script 的 sed 改写段随之删除 |
```

### 3.7 `/workspace/app/openlist/build-encv-desktop.ps1` 不动

该脚本期望 `$BuildRoot/OpenList\dist\windows`，新约定 fork 目录是 `Hi-Sillot-OpenList\`。**不修改**原因：
- README §9 已标注「已废弃，仅留作历史」
- 修改会带来 dead code 维护负担
- 若未来重新启用桌面端，应重写而非改此脚本

## 四、Verification

按推荐路径实施后：

```bash
# 1. fork 已在标位置（即使还没 clone，路径应可计算）
echo "默认 WORK_DIR 解析:"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
echo "  ${REPO_ROOT}/app/openlist/Hi-Sillot-OpenList"   # 应输出此路径

# 2. 跑一次 build 看 Workspace 日志
bash scripts/build-openlist-aar.sh --output /tmp/test-aar 2>&1 | grep -E "Workspace|Workspace|Hi-Sillot-OpenList"
# 期望: log 行显示 /workspace/app/openlist/Hi-Sillot-OpenList

# 3. fork 的 go.mod 相对路径现在天然正确
cd /workspace/app/openlist/Hi-Sillot-OpenList
CGO_ENABLED=0 go vet ./openlistlib/   # exit=0（因 replace ../../../ → /workspace 解析成功）

# 4. .gitignore 验证
cd /workspace
git status -s | grep -E "app/openlist/(Hi-Sillot-OpenList|K-Sillot-OpenList-Desktop)" 
# 期望: 空（即使 fork clone 了，git status 也不应有 fork 目录的痕迹）

# 5. OPENLIST_FORK_WORK_DIR 覆盖点
OPENLIST_FORK_WORK_DIR=/tmp/fork-override bash scripts/build-openlist-aar.sh --output /tmp/test-aar 2>&1 | grep Workspace
# 期望: /tmp/fork-override
```

## 五、Risk & Decision

| 编号 | 风险 | 缓解 |
|------|------|------|
| R1 | fork clone 留在 `app/openlist/`，比 /tmp 大 600MB+ | `app/openlist/.gitignore` 整体忽略；如 CI runner 空间紧，设 `OPENLIST_FORK_WORK_DIR=/tmp/ci-fork` 覆盖 |
| R2 | build script 写 `app/openlist/Hi-Sillot-OpenList` 需要写权限 | CI runner 当前工作目录有写权限；本地开发者同理；脚本内 `mkdir -p` 处理 |
| R3 | 旧脚本里若有人写死 `/tmp/openlist-aar-build/openlist` 路径会失效 | `app/openlist/README.md` §4.4 已说明新路径；故障表 #5 提示「核对路径」 |
| R4 | `K-Sillot/OpenList-Desktop` 仓库不存在（用户笔误成 K-Sillot） | 脚本不动它；用户手动 `git clone` 时如果 404 改回正确名 |
| R5 | 重复 build 时 `rm -rf` 太重 | 改 `git fetch && git reset --hard origin/dev` 是后续优化项；现在保持简单 |

## 六、Decision Log（执行时确认）

- [ ] D1：主 fork 目录名 = `Hi-Sillot-OpenList`
- [ ] D2：保留 `OPENLIST_FORK_WORK_DIR` 覆盖点
- [ ] D3：三个 fork 全部加 `.gitignore`（含手动 clone 的两个）
- [ ] D4：删除 sed replace 段
- [ ] D5：`build-encv-desktop.ps1` 不动
- [ ] D6：保持 `rm -rf + clone --depth 1`
- [ ] D7：副 fork 目录加 `Hi-Sillot-` / `K-Sillot-` 前缀

## 七、Sequence（执行顺序）

1. 改 `app/openlist/.gitignore`（最小影响，先做）
2. 改 `scripts/build-openlist-aar.sh`（WORK_DIR + 删 sed）
3. 改 `scripts/build-openlist-aar.ps1`（同步）
4. 改 `app/openlist/README.md`（§3、§4.2、§4.4 新增、§11）
5. 改 `scripts/README.md`（149 行附近）
6. 改 spec `R3` 风险条目
7. 不动 `build-encv-desktop.ps1`（D5 决定）
8. 跑 §四 全部 verification
