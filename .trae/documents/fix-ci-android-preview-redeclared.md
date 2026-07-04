# 修复 CI Android 构建失败 + 根治方案：杜绝 build tag 冲突

## 一、当前问题

### 错误信息
```
Error: internal/service/decrypt_preview_mobile.go:10:6: Preview redeclared in this block
Error:  internal/service/decrypt_preview.go:40:6: other declaration of Preview
```

### 根因分析

| 文件 | Build Tag | 声明的函数 |
|------|-----------|-----------|
| `decrypt_preview_mobile.go` | `//go:build android` | `Preview()` |
| `decrypt_preview.go` | **无** ❌ | `Preview()`, `constructURLs()`, `inferSubtitlePath()` |

CI 的 [android.yml 第 132-135 行](file:///workspace/.github/workflows/android.yml#L132-L135) 设置 `GOOS=android`：
- mobile 文件匹配 android tag → 被包含
- 桌面端文件 **无 tag 约束** → 也被包含
- → 同一 package 中 `Preview()` 重复声明

### 为什么本地验证漏掉

本地运行 `go build ./internal/...` 时默认 `GOOS=linux`，mobile 文件被排除，冲突不暴露。

---

## 二、修复方案（分两层）

### Layer 1: 修复当前错误（立即）

**修改文件**: `/workspace/internal/service/decrypt_preview.go`

在文件顶部添加一行：
```go
//go:build !android
```

### Layer 2: 根治措施（防止复发）

#### 2.1 全面排查现有 stub 对

项目中已有的正确 stub 模式（互斥 build tag）：

| 桌面端文件 | Tag | 移动端/平台文件 | Tag | 状态 |
|-----------|-----|---------------|-----|------|
| `utils/memory_unix.go` | `!windows` | `utils/memory_windows.go` | `windows` | ✅ |
| `utils/build_info_stub.go` | `!android` | `utils/build_info.go` | `android` | ✅ |
| `utils/ffmpeg_dlopen_stub.go` | `!android` | `utils/ffmpeg_dlopen.go` | `android` | ✅ |
| `cmd/encv/cmd.go` | `!windows` | `cmd/encv/cmd_windos.go` 等 | `windows` | ✅ |
| `service/decrypt_preview.go` | **无** ❌ | `service/decrypt_preview_mobile.go` | `android` | ❌ **待修复** |

**结论**: 仅此一处遗漏。

#### 2.2 在 project_rules.md 中添加规则

在 `/workspace/.trae/rules/project_rules.md` 新增规则章节：

```markdown
## Go Build Tag 平台约束规则（重要！）

- **凡是有平台特定 stub 实现的函数，其对应的主实现文件必须添加互斥 build tag**
- 移动端存根文件使用 `//go:build android`
- 对应的桌面端实现文件必须使用 `//go:build !android`
- Windows 平台同理：`//go:build windows` ↔ `//go:build !windows`
- **禁止**只给 stub 文件加 tag 而主文件留空（会导致 GOOS 交叉编译时重复声明）
- 正确示例参考：`internal/utils/ffmpeg_dlopen.go` (android) ↔ `ffmpeg_dlopen_stub.go` (!android)
```

#### 2.3 在 CI 中添加交叉编译验证步骤

在 `.github/workflows/android.yml` 的 "Build Go binary" 步骤之前或之后，添加一个快速包级编译检查：

```yaml
# 可选：在 PR CI 或 main CI 中添加多平台编译冒烟测试
- name: Verify cross-platform compilation
  run: |
    echo "=== Linux (default) ==="
    go build ./internal/...
    echo "=== Android cross-compile check ==="
    CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build ./internal/service/...
    echo "=== Windows cross-compile check ==="
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/encv/
    echo "✅ All platform builds passed"
```

> 注：此步骤可放在非 Android 的通用 CI workflow 中（如 PR 校验），作为冒烟测试提前暴露 build tag 冲突。

---

## 三、实施步骤

### Step 1: 修复 decrypt_preview.go
- [ ] 在 `internal/service/decrypt_preview.go` 顶部添加 `//go:build !android`

### Step 2: 更新项目规则
- [ ] 在 `.trae/rules/project_rules.md` 末尾添加 "Go Build Tag 平台约束规则" 章节

### Step 3: 验证修复
- [ ] 运行 `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build ./internal/service/` 确认通过
- [ ] 运行 `go build ./internal/...` 确认桌面端仍正常
- [ ] 运行完整测试套件 `go test ./internal/v2/...` 确认无回归

### Step 4: (可选) 增强 CI
- [ ] 在适当的 CI workflow 中添加多平台编译冒烟测试步骤
