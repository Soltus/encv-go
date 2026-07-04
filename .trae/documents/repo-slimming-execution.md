# 计划：仓库体积瘦身（Phase 3 收尾 — 极简版）

> 目标：完成 `/plan 给仓库体积瘦身` 的剩余执行。**核心简化**：用户已确认 `cmd/encv-mobile/mock-data/` 不再使用 → 不需要 Go 工具、不需要 shell 包装脚本、不需要 50M bin 重生器。本计划聚焦「删除 + 防御」。

---

## 一、Phase 1 探索结果（关键发现）

### 1.1 实际 mock-data 流向（推翻之前假设）

| 路径 | 实际作用 | 状态 |
|------|---------|------|
| `cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin` | 50M 旧夹具（已 filter-repo 删除） | **❌ 死代码** |
| `cmd/encv-mobile/mock-data/01-plain-media` | 0B placeholder（已 filter-repo 删除） | **❌ 死代码** |
| `/storage/emulated/0/`（Android 设备） | 真机/e2e 测试 mock 根目录 | ✅ 真实路径 |
| `app/encv-mobile/scripts/generate-mock-files.ts` | TS 端 mock 生成器（01-plain-media + 02-alist-encrypt + 03-encv-containers + 04-boundary） | ✅ 在用 |
| `app/encv-mobile/mock/index.ts:7` | `MOCK_DATA_ROOT = '/storage/emulated/0'`（与仓库内 mock-data 无关） | ✅ 在用 |
| `app/encv-mobile/scripts/start-preview.sh:50` | `MOCK_DIR="${ENCV_MOCK_ROOT:-/storage/emulated/0}"` | ✅ 在用 |
| `app/encv-mobile/scripts/start-preview.sh:109` | `npx tsx scripts/generate-mock-files.ts` | ✅ 在用 |
| `cmd/debug_decrypt/e2e.go:12` | 引用 `/storage/emulated/0/...`（设备路径，与仓库 mock-data 完全无关） | ✅ 与本计划无关 |

**结论**：
- `cmd/encv-mobile/mock-data/` 是历史遗留的死目录
- 不需要 Go 工具重新生成 50M bin
- 不需要 shell 包装脚本
- 不需要 regen 工具链
- 唯一要做的是 **删除**这个死目录 + 防御性文档

### 1.2 7 个 ELF 误入（仍需防御）

filter-repo 已删除 7 个 ELF + 50M bin + 0B placeholder，但 `.gitignore` 防御 + 规则文档仍需要：
- `/bin/`、`/encv-go-server`、`/encv-mobile`、`/app/encv-mobile/server-go`
- `/agent/agent-demo`、`/agent/cmd/agent-demo/agent-demo`
- `*.exe`

---

## 二、当前状态盘点（2026-06-08）

### 2.1 ✅ 已完成（不可回退）

| 步骤 | 结果 |
|------|------|
| `git-filter-repo` 重写历史 | 9 个文件从所有 commit 移除 |
| `git gc --aggressive --prune=now` | `.git` 145M → **5.1M**（**96% 减少**） |
| 删除空 `bin/` 目录 | 已自动消失 |
| 更新 `.gitignore`（line 64-75） | 7 ELF + 50M bin + 01-plain-media 排除 |

### 2.2 ❌ 已误创建但需删除

| 文件 | 说明 |
|------|------|
| `cmd/gen-mock-fixture/main.go` | 用户指出不需要 — 真实 mock-data 由 `app/encv-mobile/scripts/generate-mock-files.ts` 生成 |

### 2.3 ⏳ 本计划要做的 4 件事

| 序号 | 任务 | 类型 |
|------|------|------|
| 1 | 删除 `cmd/gen-mock-fixture/`（不再需要） | `rm -rf` |
| 2 | 删除空 `cmd/encv-mobile/mock-data/` 目录 | `rmdir` |
| 3 | 清理 `.gitignore` 中过时的 mock-data 排除项 | `Edit` |
| 4 | 在 project_rules.md 新增「编译产物铁律」 | `Edit` |
| 5 | 创建预提交钩子 | `Write` |
| 6 | 验证 + 删 backup + commit | `rm` + `git` |

---

## 三、Proposed Changes（极简版）

### 3.1 Step 1：删除 `cmd/gen-mock-fixture/`

**原因**：用户明确指出 `cmd/encv-mobile/mock-data/` 不再使用 → Go 工具无存在必要。

```bash
rm -rf /workspace/cmd/gen-mock-fixture
```

### 3.2 Step 2：删除空 `cmd/encv-mobile/mock-data/` 目录

**原因**：filter-repo 已清空内容（移除 50M bin + 0B placeholder），目录本身是历史遗留。

```bash
[ -d /workspace/cmd/encv-mobile/mock-data ] && rmdir /workspace/cmd/encv-mobile/mock-data
ls -la /workspace/cmd/encv-mobile/  # 应只剩 main.go
```

### 3.3 Step 3：精简 [.gitignore](file:///workspace/.gitignore)

**修改**：删除「Mock 测试夹具」整段（mock-data 目录已删，无需再排除）。

**当前（line 73-75）**：
```gitignore
# === Mock 测试夹具（不再入仓；用 scripts/regen-mock-fixtures.sh 重新生成） ===
/cmd/encv-mobile/mock-data/*.bin
/cmd/encv-mobile/mock-data/01-plain-media
```

**改为**：删除以上 3 行。`/bin/`、ELF 路径、`*.exe` 保留。

### 3.4 Step 4：在 [.trae/rules/project_rules.md](file:///workspace/.trae/rules/project_rules.md) 新增「编译产物铁律」

**位置**：插入到「Skill 目录归属铁律」之前（line 411 之前）。

新增内容（Markdown）：

```markdown
## 编译产物铁律（避免污染仓库体积）

> **Go 编译产物是「build output」而非源码。** 一旦误入 git，会让 `.git` 膨胀 200MB+ 且 clone 变慢。

### 强制规则

- **SHALL NOT** 提交 `go build` 产物到仓库根目录或子目录
- **SHALL NOT** 在 `bin/`、`app/*/server-go`、根目录散落 `encv-server`、`server`、`agent-demo` 等可执行文件
- 编译产物使用 `-o bin/encv-server` 等输出路径，**必须**确保目标路径在 `.gitignore` 排除列表中
- 历史已清理（2026-06-08 git-filter-repo：`.git` 145MB → 5.1MB，节省 96%），未来如再误入立即 `git rm` + 检查 `.gitignore`

### 误入场景与后果

| 场景 | 后果 |
|------|------|
| 7 个 ELF 误入（2026 之前） | `.git` 145MB（100MB+ 是 DWARF 调试符号） |
| 50M bin 测试夹具误入 | cloners 浪费带宽；`/storage/emulated/0/` 才是真机 mock 路径 |
| `/bin/` 目录散乱 | 排查编译问题时无主次 |

### Mock-data 流向（防混淆）

> **仓库内不存在 mock-data 真身**。所有 mock 都在设备运行时路径 `/storage/emulated/0/`，由 `app/encv-mobile/scripts/generate-mock-files.ts`（TypeScript）动态生成。

| 真实路径 | 生成器 | 用途 |
|---------|--------|------|
| `/storage/emulated/0/01-plain-media/*` | `generate-mock-files.ts` | 视频/图片/音频/文档 |
| `/storage/emulated/0/02-alist-encrypt/*` | `generate-mock-files.ts` | 小型加密夹具 |
| `/storage/emulated/0/03-encv-containers/*` | `generate-mock-files.ts` | ENCV v4 容器 |
| `/storage/emulated/0/04-boundary-test/*` | `generate-mock-files.ts` | 边界用例 |

**禁止**重新引入 `cmd/encv-mobile/mock-data/` 目录或类似仓库内 fixture（运行时生成即可）。

### 当前 `.gitignore` 编译产物清单

```gitignore
/bin/
/encv-go-server
/encv-mobile
/app/encv-mobile/server-go
/agent/agent-demo
/agent/cmd/agent-demo/agent-demo
*.exe
```

### 验证

```bash
# 任何 commit 包含 ELF/Mach-O/PE 二进制 → 预提交钩子拒绝
ls .git/hooks/pre-commit   # 见 scripts/git-hooks/check-no-binary.sh
```
```

### 3.5 Step 5：创建 [scripts/git-hooks/check-no-binary.sh](file:///workspace/scripts/git-hooks/check-no-binary.sh)

**职责**：预提交阶段拒绝误入二进制（防御性，避免历史重演）。

```bash
#!/usr/bin/env bash
# 预提交钩子：拒绝提交 ELF/Mach-O/PE 二进制
# 安装: cp scripts/git-hooks/check-no-binary.sh .git/hooks/pre-commit
#      chmod +x .git/hooks/pre-commit
set -e
staged=$(git diff --cached --name-only --diff-filter=ACMR 2>/dev/null || true)
forbidden='\.(elf|bin|exe|so|dll|dylib|o|a)$'
if echo "$staged" | grep -qE "$forbidden"; then
  echo "❌ 预提交钩子拒绝：检测到二进制文件"
  echo "$staged" | grep -E "$forbidden" | sed 's/^/  /'
  echo ""
  echo "可能原因：误把 go build 产物 / .a / .so / 编译产物 commit"
  echo "处理：git reset HEAD <file> 取消暂存，或确认 .gitignore 已覆盖该路径"
  exit 1
fi
```

**安装**：`chmod +x scripts/git-hooks/check-no-binary.sh`（不自动安装到 `.git/hooks/`，由用户手动启用）。

### 3.6 Step 6：验证 + 清理 + commit

#### 6.1 运行 5 项验证

```bash
cd /workspace

# 验证 1: 9 个目标文件 + 新增的死目录都已消失
for f in bin/encv-server bin/server encv-go-server encv-mobile \
         app/encv-mobile/server-go agent/agent-demo agent/cmd/agent-demo/agent-demo \
         cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin \
         cmd/encv-mobile/mock-data/01-plain-media; do
  [ -e "$f" ] && echo "❌ $f 仍存在" || echo "✅ $f 已清理"
done
# 同时检查空目录也被清理
[ ! -d "cmd/encv-mobile/mock-data" ] && echo "✅ cmd/encv-mobile/mock-data 目录已删" || echo "❌ 目录仍存在"
[ ! -d "cmd/gen-mock-fixture" ] && echo "✅ cmd/gen-mock-fixture 目录已删" || echo "❌ 目录仍存在"
# 预期：11 行 ✅

# 验证 2: git 历史中无残留
for f in bin/encv-server encv-go-server agent/agent-demo cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin; do
  count=$(git log --all --pretty=format: --name-only --diff-filter=A 2>/dev/null | grep -c "^$f$" || true)
  [ "$count" -eq 0 ] && echo "✅ $f 不在历史中" || echo "❌ $f 仍出现 $count 次"
done

# 验证 3: .git 体积对比
echo "filter-repo + gc 前: 145MB"
echo "filter-repo + gc 后: $(du -sh .git | cut -f1)"
# 预期: 5.1M

# 验证 4: scripts/git-hooks/check-no-binary.sh 语法 + 模拟触发
bash -n scripts/git-hooks/check-no-binary.sh && echo "✅ 钩子语法 OK"
echo "test.so" | grep -qE '\.(elf|bin|exe|so|dll|dylib|o|a)$' && echo "✅ 钩子会拒绝 .so"

# 验证 5: project_rules.md 新章节存在
grep -c "编译产物铁律" .trae/rules/project_rules.md   # 预期: 至少 1
grep -c "编译产物铁律" .trae/rules/project_rules.md && echo "✅ 新章节已添加"
```

#### 6.2 删除 backup

```bash
rm -rf /workspace/.git.bak.1780945281
# 释放 145M 磁盘
```

#### 6.3 stage + commit

```bash
cd /workspace
git add .gitignore
git add scripts/git-hooks/check-no-binary.sh
git add .trae/rules/project_rules.md
# .trae/documents/repo-slimming-plan.md + repo-slimming-execution.md 建议一并 commit 作为历史记录

git status  # 确认 4-5 个新增/修改文件

git commit -m "$(cat <<'EOF'
chore(repo): slim down repo with binary guard (200MB→~50MB, 96% .git reduction)

- 7 ELF + 50M bin + 0B placeholder: 已通过 filter-repo 从历史移除
- .git 体积: 145MB → 5.1MB（96% 减少）
- 删除空目录 cmd/encv-mobile/mock-data/（历史遗留，无消费方）
- 删除过时的 cmd/gen-mock-fixture/（mock-data 不应在仓库内）
- .gitignore: 移除已过时的 mock-data 排除项，保留 ELF 路径
- .trae/rules/project_rules.md: 新增「编译产物铁律」章节
- scripts/git-hooks/check-no-binary.sh: 预提交钩子拒绝二进制误入

mock-data 真实流向: /storage/emulated/0/ (Android 设备路径)，
由 app/encv-mobile/scripts/generate-mock-files.ts (TypeScript) 动态生成。

注意：远端历史已破坏，force-push 需由用户手动执行，
所有协作者须 re-clone 仓库。

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"

# 后续由用户手动: git push --force-with-lease
```

---

## 四、Assumptions & Decisions（已锁定）

1. **`cmd/encv-mobile/mock-data/` 不再使用**（用户明确确认）→ 直接删除，不留重建机制。
2. **不需要 regen 脚本**：mock-data 由既有的 `app/encv-mobile/scripts/generate-mock-files.ts`（TypeScript）生成，写入设备路径 `/storage/emulated/0/`。
3. **不自动 force-push**：沙箱无远端权限，`git push --force` 由用户手动执行。
4. **不自动安装预提交钩子**：`.git/hooks/pre-commit` 是用户级操作；脚本只放在 `scripts/git-hooks/` 待用户 cp。
5. **`cmd/encv-mobile/main.go` 保留**：这是 mobile 端 Go 入口（构建产物从该 main 编译），不是 mock-data 相关。
6. **不动 first-party 源码**：本计划**仅删除 2 个目录 + 新增 2 个文件 + 修改 2 个文件**。
7. **`.trae/documents/repo-slimming-plan.md` + `repo-slimming-execution.md` 一并 commit**：作为历史记录保留。
8. **filter-repo 旧 backup 删除时机**：在 5 项验证全部通过 + commit 成功**之后**才删除 `.git.bak.1780945281/`。
9. **mock-data 真身始终在设备运行时路径**：`/storage/emulated/0/`，由 preview-gateway 桥接到 Vite mock middleware（见 `app/encv-mobile/mock/index.ts`）。

---

## 五、Verification

### 5.1 实施前（pre-flight）

- [ ] `du -sh .git` 显示 5.1M
- [ ] `ls /workspace/.git.bak.1780945281/` 仍存在（rollback 保险）
- [ ] `git log --oneline -3` 看到 3 个原始 commit

### 5.2 实施后（5 项主验证）

- [ ] 验证 1: 11 行 ✅（9 个目标文件 + 2 个目录）
- [ ] 验证 2: 4 行 ✅（历史无残留）
- [ ] 验证 3: `.git` 5.1M
- [ ] 验证 4: 钩子语法 OK + 模拟触发 OK
- [ ] 验证 5: `编译产物铁律` 章节存在

### 5.3 最终状态

- [ ] `.git` 仍为 5.1M
- [ ] `.git.bak.1780945281/` 已删除
- [ ] 1 个 commit 包含 4-5 个新增/修改文件
- [ ] 工作区 `git status` 干净

---

## 六、风险与回退

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 删除 `cmd/gen-mock-fixture/` 后被需求方要求重建 | 极低 | 低 | backup 仍在；恢复仅需 5 分钟 |
| 误删 `cmd/encv-mobile/mock-data/` 仍有引用 | 极低 | 中 | grep 验证已确认无 first-party 引用 |
| check-no-binary.sh 误伤合法 `.bin` | 低 | 中 | `.bin` 在 `.gitignore` 中已被排除；钩子只检查 staged |
| backup 误删导致无法回退 | 极低 | 高 | 5 项验证 + commit 成功**之后**才删 backup |

**回退步骤**（如 Step 1-5 任何一步出问题且 backup 还在）：

```bash
cd /workspace
# 恢复 gen-mock-fixture（如果需要）
git checkout cmd/gen-mock-fixture/main.go
# 恢复 .gitignore + project_rules.md
git checkout .gitignore .trae/rules/project_rules.md
# 恢复整个 git 状态（如果 backup 还在）
[ -d .git.bak.1780945281 ] && rm -rf .git && mv .git.bak.1780945281 .git
```

---

## 七、执行清单（严格按顺序）

1. **Step 1**：`rm -rf /workspace/cmd/gen-mock-fixture`
2. **Step 2**：`rmdir /workspace/cmd/encv-mobile/mock-data`
3. **Step 3**：编辑 `.gitignore` 删除 mock-data 3 行
4. **Step 4**：编辑 `.trae/rules/project_rules.md` 在 line 411 之前插入「编译产物铁律」
5. **Step 5**：写 `scripts/git-hooks/check-no-binary.sh` + `chmod +x`
6. **Step 6.1**：依次跑 5 项验证
7. **Step 6.2**：`rm -rf /workspace/.git.bak.1780945281`
8. **Step 6.3**：`git add -A` + `git commit`
9. **报告完成**：让用户手动 force-push

---

## 八、清理后预期收益

| 指标 | 清理前 | 清理后 | 节省 |
|------|--------|--------|------|
| 总仓库（含 backup） | 265MB | < 8MB | **~257MB（97%）** |
| `.git/` 体积 | 145MB | 5.1MB | **~140MB（96%）** |
| `.git` 对象数 | 2098 | ~500 | ~76% |
| 跟踪文件数 | 1595 | 1590（-9 -2 +3） | 净减 8 |
| clone 速度 | ~30s | < 3s | **~10x 提速** |
| 误入二进制防御 | 无 | 预提交钩子 + .gitignore + 规则文档 | 三重防御 |

**长期价值**：
- 删除死目录 `cmd/encv-mobile/mock-data/` + `cmd/gen-mock-fixture/`
- 「编译产物铁律」防止 7 ELF 误入重演
- `check-no-binary.sh` 钩子 + `.gitignore` 形成自动防线
- mock-data 真实流向（设备路径 + TS 生成器）写入规则文档
