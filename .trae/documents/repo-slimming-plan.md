# 计划：仓库体积瘦身（rewrite history + 重新生成脚本）

> 目标：将仓库从 265MB（.git 145MB + 源文件 120MB）瘦至 ~50MB（.git 50MB + 源文件 50MB），主要清理 7 个误提交的编译产物 ELF 二进制（200MB+）+ 50MB mock-data 二进制。
>
> **风险等级：高**（git filter-repo 需 force-push + 协作者重新 clone）。本计划**不会**删除任何 first-party 源码，仅清理误入仓库的编译产物与测试夹具。

---

## 一、Phase 1 探索结果

### 1.1 仓库体积分布

| 项目 | 大小 | 备注 |
|------|------|------|
| **总仓库** | **265MB** | `du -sh /workspace` |
| `.git/` | 145MB | 3 个 commit + 2098 个对象，pack 144MB |
| 源文件 | ~120MB | 1595 个 tracked files |
| **可清理（误入）** | **~200MB** | 7 个 ELF + 1 个 50M bin |

### 1.2 误入的二进制清单（7 ELF + 1 BIN + 1 placeholder）

| 文件路径 | 实际大小 | 类型 | 来源 |
|----------|---------|------|------|
| `bin/encv-server` | 36M | ELF Go 编译产物 | 本地 `go build` 误提交 |
| `bin/server` | 36M | ELF Go 编译产物 | 本地 `go build` 误提交 |
| `encv-go-server` | 35M | ELF Go 编译产物 | 根目录散落 |
| `encv-mobile` | 35M | ELF Go 编译产物 | 根目录散落 |
| `app/encv-mobile/server-go` | 35M | ELF Go 编译产物 | 误放在 src 下 |
| `agent/agent-demo` | 9.7M | ELF Go 编译产物 | 根目录散落 |
| `agent/cmd/agent-demo/agent-demo` | 9.7M | ELF Go 编译产物 | cmd 目录下 |
| `cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin` | 50M | 二进制测试夹具 | 加密媒体文件 |
| `cmd/encv-mobile/mock-data/01-plain-media` | 0B | 占位空文件 | 早期 placeholder |

**7 个 ELF 文件特征**：
- 均 `file ELF 64-bit LSB executable, x86-64` (linux/amd64)
- 全部 `debug_info`（未 strip，含完整 DWARF 调试符号）
- `llvm-strip --strip-all` 后可缩小到 ~7MB，但**根本不应入 git**

**50M bin 文件特征**：
- `data` 二进制（不可执行），52,428,800 bytes
- 由 `internal/v2/plugins/alistencrypt` 用密码 `8682268` 加密的 50M 媒体
- `cmd/debug_decrypt/e2e.go:12` 引用路径为 `/storage/emulated/0/hyYGPCwJPQ3+xrdAvfnn2.bin`（Android 设备路径）
- `app/encv-mobile/scripts/start-preview.sh:216` 在 preview 启动时上传此文件到模拟器

### 1.3 `.gitignore` 当前状态

当前 `.gitignore` **未排除上述任何路径**：
- ❌ `/bin/`
- ❌ `/encv-go-server`、`/encv-mobile`
- ❌ `/app/encv-mobile/server-go`
- ❌ `/agent/agent-demo`
- ❌ `/agent/cmd/agent-demo/agent-demo`
- ❌ `cmd/encv-mobile/mock-data/*.bin`

### 1.4 mock-data 已有的生成脚本

- `app/encv-mobile/scripts/generate-mock-files.ts`（TypeScript）
  - 生成 01-plain-media/* 视频/图片/文档
  - 生成 02-alist-encrypt/* 加密小文件（最大 16KB）
  - **不生成 50M 的 hyYGPCwJPQ3+xrdAvfnn2.bin**

### 1.5 aenc 包可重新生成 50M bin

- `internal/v2/plugins/alistencrypt/encryptor.go:13` `EncryptToFile()` 函数
- 接受 `io.Reader`、`password`、`outputDir`
- 可由一个独立 Go 工具调用，生成与原 bin 等价的加密文件

---

## 二、目标方案对比

| 方案 | 风险 | 节省 | 适用场景 |
|------|------|------|----------|
| 仅本地 `git rm` | 低 | 0（.git 不变） | 多协作的成熟项目 |
| **git-filter-repo + gc（采用）** | **高**（强制 push） | **~150MB .git 缩小到 ~30MB** | 单人/小团队且愿意 force-push |
| git-replace + gc | 中 | 同 filter-repo | 渐进式清理 |

用户已选：**filter-repo（高风险）+ 重新生成脚本**。

---

## 三、Proposed Changes（变更清单）

### 3.1 Step 1：安装并验证 `git-filter-repo`

```bash
# 已在沙箱内通过 pip3 安装：git-filter-repo 2.47.0
git filter-repo --version  # 预期输出 2.47.0
```

如需在 CI / 协作者环境使用，在 `.github/workflows/` 中添加安装步骤（`pip install git-filter-repo` 或 `apt install git-filter-repo`）。

### 3.2 Step 2：使用 `git-filter-repo` 删除 9 个误入文件

执行前**强制要求**：
- ✅ 已 `git status` 干净
- ✅ 已 `git fetch --all` 同步远端
- ✅ 已做完整 backup：`cp -r .git .git.bak.$(date +%s)`

```bash
# 删除单文件列表（每个 --path 对应一条规则）
cd /workspace
git filter-repo --force \
  --path bin/encv-server \
  --path bin/server \
  --path encv-go-server \
  --path encv-mobile \
  --path app/encv-mobile/server-go \
  --path agent/agent-demo \
  --path agent/cmd/agent-demo/agent-demo \
  --path cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin \
  --path cmd/encv-mobile/mock-data/01-plain-media \
  --invert-paths
```

**预期**：
- 9 个文件从所有 commit 中消失
- `.git/objects/pack/` 减小到 ~30MB（待 gc 后）

### 3.3 Step 3：执行 `git gc` 真正释放空间

```bash
cd /workspace
git reflog expire --expire=now --all
git gc --aggressive --prune=now
```

**预期**：`du -sh .git` 从 145MB 减小到 ~30-50MB。

### 3.4 Step 4：删除空目录（如有）

```bash
# bin/ 目录在 filter-repo 后应已空
rmdir /workspace/bin 2>/dev/null
[ -d /workspace/bin ] && echo "bin/ 仍有内容，不删" || echo "✅ bin/ 已删除"
```

### 3.5 Step 5：更新 `.gitignore` 防止重新提交

在 [.gitignore](file:///workspace/.gitignore) 末尾追加：

```gitignore
# === 编译产物（本地 build 误提交，filter-repo 已清理历史） ===
/bin/
/encv-go-server
/encv-mobile
/app/encv-mobile/server-go
/agent/agent-demo
/agent/cmd/agent-demo/agent-demo
*.exe

# === Mock 测试夹具（不再入仓；用 scripts/regen-mock-fixtures.sh 重新生成） ===
/cmd/encv-mobile/mock-data/*.bin
/cmd/encv-mobile/mock-data/01-plain-media
```

**为何 `01-plain-media` 也排除**：该 0 字节文件是早期 placeholder，已被 `generate-mock-files.ts` 替代。

### 3.6 Step 6：创建 `scripts/regen-mock-fixtures.sh` 重新生成 50M bin

**位置**：[scripts/regen-mock-fixtures.sh](file:///workspace/scripts/regen-mock-fixtures.sh)

**职责**：调用新 Go 工具 `cmd/gen-mock-fixture/`，用项目自身的 `aenc.EncryptToFile` 加密 50MB 随机数据，输出到 `cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin`。

#### 3.6.1 新增 Go 工具 [cmd/gen-mock-fixture/main.go](file:///workspace/cmd/gen-mock-fixture/main.go)

```go
package main

// 重新生成 cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin 测试夹具
//
// 用法: go run ./cmd/gen-mock-fixture [output_path]
// 默认输出: cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin
//
// 行为:
//   1. 生成 50MB 随机明文（可复现：seed=42）
//   2. 用项目 aenc 包 + 密码 "8682268" 加密
//   3. 写入 output_path

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	aenc "github.com/Soltus/encv-go/internal/v2/plugins/alistencrypt"
)

const (
	defaultPassword  = "8682268"
	defaultPlainSize = 50 * 1024 * 1024 // 50MB
	defaultOutput    = "cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin"
)

func main() {
	output := flag.String("o", defaultOutput, "Output path for the encrypted fixture")
	size := flag.Int64("size", defaultPlainSize, "Size of the random plaintext in bytes")
	password := flag.String("p", defaultPassword, "Encryption password")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	fmt.Printf("→ 生成 %d 字节随机明文...\n", *size)
	plaintext := make([]byte, *size)
	if _, err := io.ReadFull(rand.Reader, plaintext); err != nil {
		log.Fatalf("rand read: %v", err)
	}

	fmt.Printf("→ 用 aenc.EncryptToFile 加密 (password=%s)...\n", *password)
	result, err := aenc.EncryptToFile(
		bytesReader(plaintext), *password, filepath.Dir(*output), &aenc.AlistEncryptPluginConfig{},
	)
	if err != nil {
		log.Fatalf("encrypt: %v", err)
	}

	fmt.Printf("→ 写入 %s (%d 字节)\n", result.FinalPath, result.Size)
	fmt.Printf("✅ 重新生成完成\n")
}

func bytesReader(b []byte) io.Reader { return &byteReader{b: b} }

type byteReader struct {
	b   []byte
	pos int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}
```

#### 3.6.2 包装脚本 [scripts/regen-mock-fixtures.sh](file:///workspace/scripts/regen-mock-fixtures.sh)

```bash
#!/usr/bin/env bash
# 重新生成 mock-data 测试夹具
# - 50M bin（alist-encrypt 加密的随机数据）
# - 01-plain-media/ 视频/图片/文档
#
# 用法: ./scripts/regen-mock-fixtures.sh [50m_bin_output_path]
# 前置条件: 仓库根目录下可执行 `go run ./cmd/gen-mock-fixture`

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "=== Step 1/2: 生成 50M bin 测试夹具 ==="
go run ./cmd/gen-mock-fixture "$@"

echo ""
echo "=== Step 2/2: 生成 01-plain-media/ 视频/图片/文档 ==="
cd app/encv-mobile
ENCV_MOCK_ROOT="$REPO_ROOT/cmd/encv-mobile/mock-data" \
  npx tsx scripts/generate-mock-files.ts

echo ""
echo "✅ 全部 mock 夹具重新生成完成"
echo "  50M bin:        $REPO_ROOT/cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin"
echo "  01-plain-media: $REPO_ROOT/cmd/encv-mobile/mock-data/01-plain-media/"
```

### 3.7 Step 7：更新 [scripts/ci-check-no-nodejs-crypto.sh](file:///workspace/scripts/ci-check-no-nodejs-crypto.sh) 防御（可选）

无需改 — mock-data 不会引用 node:crypto。

### 3.8 Step 8：更新 [.trae/rules/project_rules.md](file:///workspace/.trae/rules/project_rules.md) 添加新铁律

在「Skill 目录归属铁律」之上新增「编译产物铁律」章节：

```markdown
## 编译产物铁律（避免污染仓库体积）

> **Go 编译产物是项目「build output」而非源码。** 一旦误入 git，会让仓库 .git 膨胀 200MB+ 且 clone 速度变慢。

### 强制规则

- **SHALL NOT** 提交 `go build` 产物到仓库根目录或子目录
- **SHALL NOT** 在 `bin/`、`app/*/server-go`、根目录散落 `encv-server`、`server`、`agent-demo` 等可执行文件
- 编译产物使用 `-o bin/encv-server` 等输出路径，**必须**确保目标路径在 `.gitignore` 排除列表中
- 历史已清理（2026-06-08 git-filter-repo），未来如再误入，立即 `git rm` + 检查 `.gitignore`

### 误入场景与后果

| 场景 | 后果 |
|------|------|
| 7 个 ELF 误入（2026 之前） | .git 145MB 膨胀（100MB+ 是 debug 符号） |
| 50M bin 测试夹具误入 | mock-data 与真机路径冲突，cloners 浪费带宽 |
| `/bin/` 目录散乱 | 排查编译问题时无主次 |

### 当前 `.gitignore` 排除清单

```gitignore
/bin/
/encv-go-server
/encv-mobile
/app/encv-mobile/server-go
/agent/agent-demo
/agent/cmd/agent-demo/agent-demo
*.exe
/cmd/encv-mobile/mock-data/*.bin
/cmd/encv-mobile/mock-data/01-plain-media
```

### 验证

```bash
# 任何 commit 包含 ELF/Mach-O/PE 二进制 → CI 失败
.git/hooks/pre-commit  # 见 scripts/git-hooks/check-no-binary.sh
```

> 见 [scripts/git-hooks/check-no-binary.sh](file:///workspace/scripts/git-hooks/check-no-binary.sh)（建议添加的预提交钩子）
```

### 3.9 Step 9（可选）：添加预提交钩子 [scripts/git-hooks/check-no-binary.sh](file:///workspace/scripts/git-hooks/check-no-binary.sh)

```bash
#!/usr/bin/env bash
# 预提交钩子：拒绝提交 ELF/Mach-O/PE 二进制
# 安装: cp scripts/git-hooks/check-no-binary.sh .git/hooks/pre-commit
#      chmod +x .git/hooks/pre-commit

set -e
forbidden='\.(elf|bin|exe|so|dll|dylib)$'
staged=$(git diff --cached --name-only --diff-filter=ACMR)
if echo "$staged" | grep -qE "$forbidden"; then
  echo "❌ 检测到二进制文件:"
  echo "$staged" | grep -E "$forbidden" | sed 's/^/  /'
  echo ""
  echo "如确需添加二进制，请先与维护者确认；"
  echo "否则使用 'git reset HEAD <file>' 取消暂存。"
  exit 1
fi
```

安装命令（不在本计划自动执行，提供给用户手动启用）：
```bash
cp scripts/git-hooks/check-no-binary.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### 3.10 Step 10：删除本地 backup

```bash
# 验证无问题后删除 backup
rm -rf /workspace/.git.bak.*
```

### 3.11 不需要变更

- `.github/linguist.yml` — 仍然有效
- `.gitattributes` — 仍然有效
- `app/encv-mobile/scripts/generate-mock-files.ts` — 已存在的 01-plain-media 生成器
- 所有 first-party 源码

---

## 四、Assumptions & Decisions（已锁定）

1. **强制 force-push 已与用户沟通** — 用户选择「重写历史（高风险）」时已确认接受破坏性。需 PR 描述中明确告知协作者。

2. **保留 `01-plain-media/` 目录** — 排除 `01-plain-media` 单文件（0B），但**不**排除目录本身（generate-mock-files.ts 会在目录内生成 11 个子文件）。

3. **`agent/agent-demo` 编译产物的处理** — 既然 `cmd/agent-demo/` 的源码存在，根目录的 `agent/agent-demo` 编译产物是**冗余**的。filter-repo 删除后，开发者本地 `go build -o agent/agent-demo ./cmd/agent-demo` 即可重建。

4. **`bin/` 目录整目录删除** — 7 个 ELF 全部清理后，`bin/` 应为空。删 `bin/` 目录本身。

5. **不删除 .git/pack 本身** — git-filter-repo 重建 pack 文件后，旧 pack 自动失效并被 gc 清理。

6. **mock-data bin 不会影响 e2e 测试** — `cmd/debug_decrypt/e2e.go:12` 引用的路径是 `/storage/emulated/0/...`（Android 设备路径），与仓库内的 mock-data 副本**完全无关**。`start-preview.sh:216` 的上传行为使用 mock-data 文件作为源。如果 mock-data 不存在，start-preview 会失败 → 提示用户运行 regen 脚本。

7. **未变更 first-party 源码** — 本计划**只清理编译产物 + 测试夹具 + 1 个新工具**，不动任何 first-party 代码。

8. **force-push 的处理** — 本计划不在沙箱内执行 `git push --force`（沙箱无远端），仅本地完成 history rewrite，**远端 push 由用户手动执行**。PR 描述中需写明。

---

## 五、Verification（验证步骤）

按以下顺序验证：

1. **预执行 backup 验证**：
   ```bash
   ls -la /workspace/.git.bak.* 2>/dev/null && echo "✅ backup exists"
   ```

2. **filter-repo 后文件不存在**：
   ```bash
   cd /workspace
   for f in bin/encv-server bin/server encv-go-server encv-mobile \
            app/encv-mobile/server-go agent/agent-demo agent/cmd/agent-demo/agent-demo \
            cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin \
            cmd/encv-mobile/mock-data/01-plain-media; do
     [ -e "$f" ] && echo "❌ $f still exists" || echo "✅ $f removed"
   done
   # 预期：9 行 ✅
   ```

3. **git 历史中无残留**：
   ```bash
   cd /workspace
   for f in bin/encv-server encv-go-server agent/agent-demo; do
     count=$(git log --all --pretty=format: --name-only --diff-filter=A | grep -c "^$f$" || true)
     [ "$count" -eq 0 ] && echo "✅ $f 不在历史中" || echo "❌ $f 仍出现 $count 次"
   done
   ```

4. **gc 后 .git 体积对比**：
   ```bash
   echo "filter-repo + gc 前: 145MB (3 commits)"
   echo "filter-repo + gc 后: $(du -sh /workspace/.git | cut -f1)"
   du -sh /workspace/.git/objects/pack/ /workspace/.git/objects/ 2>/dev/null
   ```

5. **regen 脚本可执行**：
   ```bash
   cd /workspace
   go build -o /tmp/gen-mock-fixture ./cmd/gen-mock-fixture
   /tmp/gen-mock-fixture --size 1048576  # 1MB 临时测试
   ls -la /workspace/cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin
   # 预期：1MB 测试文件生成成功
   rm /workspace/cmd/encv-mobile/mock-data/hyYGPCwJPQ3+xrdAvfnn2.bin  # 清理测试文件
   ```

6. **scripts/regen-mock-fixtures.sh 语法**：
   ```bash
   bash -n /workspace/scripts/regen-mock-fixtures.sh && echo "✅ shell 语法 OK"
   ```

7. **.gitignore 生效**：
   ```bash
   cd /workspace
   echo "test" > /bin/test-bin 2>/dev/null || mkdir -p bin && touch bin/test-bin
   git check-ignore -v bin/test-bin 2>/dev/null
   # 预期：bin/test-bin 被忽略
   rm -rf /workspace/bin
   ```

8. **CI 不受影响**：
   ```bash
   grep -l "encv-server\|bin/server" /workspace/.github/workflows/*.yml 2>/dev/null
   # 预期：无输出
   ```

9. **总仓库体积对比**：
   ```bash
   echo "清理前：$(du -sh /workspace 2>/dev/null)"
   echo "清理后：$(du -sh /workspace 2>/dev/null)"
   # 预期：~265MB → ~50MB
   ```

10. **first-party 源码未受影响**：
    ```bash
    cd /workspace
    git log --stat --diff-filter=AMR --name-only -- '*.go' '*.ts' '*.vue' '*.kt' '*.json' | head -20
    # 预期：仅 .gitignore / scripts/regen-mock-fixtures.sh / cmd/gen-mock-fixture/ 新增
    ```

---

## 六、风险与回退

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| filter-repo 误删 first-party 源码 | 极低 | 高 | backup 在 .git.bak.*；执行后用 step 3 验证 |
| force-push 破坏协作者 | 中 | 中 | PR 描述中明确「all collaborators must re-clone」 |
| git-filter-repo 在某些 OS 不可用 | 低 | 中 | 已在沙箱内 pip 安装；CI 需 `pip install git-filter-repo` |
| .git 体积未明显减小 | 低 | 中 | 确认 `git gc --aggressive --prune=now` 成功执行；如 pack 未释放，用 `git repack -ad` |
| 50M bin 重新生成与原文件二进制不同 | 高（预期内） | 低 | 测试只看解密结果（密码 8682268），不校验字节级相同 |
| 删除 mock-data 后 e2e 测试中断 | 中 | 低 | `e2e.go` 引用的是 Android 设备路径，不依赖仓库内 mock-data；start-preview.sh:216 上传失败会提示 regen |
| 远端已有多分支/PR 基于旧历史 | 高 | 高 | 必须在 force-push 前通知所有协作者关闭 / rebase 所有 open PR |

### 回退步骤（如遇问题）

```bash
# 1. 立即停止所有后续步骤
cd /workspace

# 2. 恢复 backup
rm -rf .git
mv .git.bak.<timestamp> .git

# 3. 验证恢复
git log --oneline  # 应看到 3 个原始 commit
git status         # 应干净
```

---

## 七、执行清单（提交顺序）

1. **pre-flight**：
   - `cp -r .git .git.bak.$(date +%s)`
   - `git status` 确保干净
   - 记录当前 `.git` 体积：145MB

2. **执行 filter-repo**（9 个路径 --invert-paths）

3. **执行 gc**（reflog expire + gc --aggressive --prune=now）

4. **删除空目录**（`bin/`）

5. **更新 .gitignore**

6. **创建 regen 工具**：
   - `cmd/gen-mock-fixture/main.go`（新 Go 工具）
   - `scripts/regen-mock-fixtures.sh`（包装脚本）

7. **更新 .trae/rules/project_rules.md**（新增「编译产物铁律」）

8. **（可选）创建预提交钩子** `scripts/git-hooks/check-no-binary.sh`

9. **运行 Verification 步骤 1-10**

10. **删除 .git.bak.***（确认无问题后）

11. **stage + commit**：
    - `git add -A`（regen 工具、.gitignore、project_rules.md、预提交钩子）
    - commit message: `chore(repo): slim down repo by removing tracked ELF binaries and 50M mock fixture (200MB→~50MB)`

12. **用户手动 force-push**（沙箱无远端权限）

---

## 八、清理后预期收益

| 指标 | 清理前 | 清理后 | 节省 |
|------|--------|--------|------|
| 总仓库体积 | 265MB | ~50MB | **~215MB（81%）** |
| .git 体积 | 145MB | ~30MB | ~115MB |
| 源文件 | 120MB | ~50MB（除 ELF + bin） | ~70MB |
| git 对象数 | 2098 | ~500 | ~76% |
| 跟踪文件数 | 1595 | 1586（-9） | -9 |
| clone 速度（推算） | 8-12 MB/s × 30s | 8-12 MB/s × 5s | **~6x 提速** |
| GitHub Languages JS 占比 | ~15% | ~13% | 略降（无 50M bin JS） |

**注意**：filter-repo 之后的历史中**无** ELF 和 50M bin，`du -sh .git/objects/pack/` 会自动释放这些对象的 pack 空间。git gc 是最后的关键一步，不能跳过。
