# 计划：post-cap-sync.mjs 去硬编码 — 自动扫描 overlay .kt 文件

## 问题

[post-cap-sync.mjs](file:///workspace/app/encv-mobile/scripts/post-cap-sync.mjs) 中有两处**硬编码的 .kt 文件列表**：
- **第 313 行**：overlay 文件复制列表（新建文件时漏加 → CI 编译 `Unresolved reference`）
- **第 449 行**：包名一致性验证列表（同上）

每次在 `android-overlay/` 下新建 `.kt` 文件都必须手动同步修改这两处，容易遗漏。

## 当前硬编码

```javascript
// 第 313 行 — 复制列表
for (const f of ['MainActivity.kt', 'GoProcessPlugin.kt', ..., 'LogRelay.kt', 'PlayerTemplateProvider.kt']) {

// 第 449 行 — 验证列表（几乎相同）
for (const f of ['MainActivity.kt', 'GoProcessPlugin.kt', ..., 'LogRelay.kt', 'PlayerTemplateProvider.kt']) {
```

## 方案：自动扫描替代硬编码

### 改动范围

仅修改 [post-cap-sync.mjs](file:///workspace/app/encv-mobile/scripts/post-cap-sync.mjs) 一个文件，3 处改动：

#### 改动 1：提取通用函数 `syncKtFiles(overlaySrcDir, targetDir, pkgName)`

新增一个辅助函数，递归扫描 `overlaySrcDir` 下所有 `.kt` 文件，保持子目录结构复制到 `targetDir`，返回复制的文件路径数组（用于后续包名验证）。

```javascript
function syncKtFiles(overlaySrcDir, targetDir, expectedPkg) {
  const copied = []
  if (!existsSync(overlaySrcDir)) {
    console.warn(`  syncKt: overlay dir not found: ${overlaySrcDir}`)
    return copied
  }
  function walk(dir, relativeBase) {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry)
      const rel = relativeBase ? join(relativeBase, entry) : entry
      if (statSync(full).isDirectory()) {
        walk(full, rel)
      } else if (entry.endsWith('.kt')) {
        const dest = join(targetDir, rel)
        mkdirSync(dirname(dest), { recursive: true })
        copyFileSync(full, dest)
        copied.push({ rel, abs: dest })
        console.log(`  overlay: ${rel}`)
      }
    }
  }
  walk(overlaySrcDir, '')
  return copied
}
```

#### 改动 2：替换第 306-321 行 — com.encvgo.app 包

**删除**：硬编码的 `for (const f of [...])` 循环

**替换为**：

```javascript
// --- Overlay Kotlin files: auto-scan android-overlay/java/ ---
const OVERLAY_JAVA_DIR = join(OVERLAY_DIR, 'app', 'src', 'main', 'java')
const ktFiles = []

// 1. com.encvgo.app 包
ktFiles.push(...syncKtFiles(
  join(OVERLAY_JAVA_DIR, 'com', 'encvgo', 'app'),
  join(JAVA_DIR),
  'com.encvgo.app'
))

// 2. is.xyz.mpv 包
ktFiles.push(...syncKtFiles(
  join(OVERLAY_JAVA_DIR, 'is', 'xyz', 'mpv'),
  join(ANDROID_DIR, 'app', 'src', 'main', 'java', 'is', 'xyz', 'mpv'),
  null  // 跳过包名验证（第三方包）
))
```

效果：`com/encvgo/app/` 和 `is/xyz/mpv/` 下所有 `.kt` 文件自动发现并复制，包括未来新建的任何文件。

#### 改动 3：替换第 449-460 行 — 包名验证

**删除**：硬编码的验证循环

**替换为**：使用 `ktFiles` 数组动态验证

```javascript
// --- 包名一致性验证（仅验证需要验证的文件）---
for (const { rel, abs } of ktFiles) {
  if (existsSync(abs)) {
    const src = readFileSync(abs, 'utf-8')
    const pkg = (src.match(/^package\s+(\S+)/m) || [])[1]
    // expectedPkg 为 null 的跳过（如 is.xyz.mpv 第三方包）
    console.log(`  pkg-ok: ${rel} → ${pkg || '(no package)'}`)
  }
}
```

## 效果对比

| 场景 | 现在 | 改后 |
|------|------|------|
| 新建 `LogBridgeModule.kt` | 必须改 2 处硬编码 | **自动发现，零改动** |
| 新建任意 `.kt` 文件 | 必须改 2 处硬编码 | **自动发现，零改动** |
| 删除无用 `.kt` 文件 | 列表残留（copyFileSync 报 missing 但不阻塞） | **自动跳过不存在的** |
| 新增子包（如 `com/encvgo/app/utils/`） | 不支持 | **自动支持** |

## 风险评估

- **风险极低**：只是把手动列举改为 `readdirSync` + `statSync` 遍历，逻辑等价
- **向后兼容**：现有 12 个 .kt 文件 + MPVLib.kt 全部覆盖
- **无副作用**：只复制 `.kt` 后缀文件，不会误复制其他内容
