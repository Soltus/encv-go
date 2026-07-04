# 彻底修复方案：集中所有 Android 自定义到 post-cap-sync.mjs

## 核心思路

**把 MainActivity/GoProcessPlugin 的复制从 CI YAML 移到 post-cap-sync.mjs**

post-cap-sync.mjs 是 Capacitor 的 `afterSync` hook，在 `cap sync android` 之后自动执行。目前它已经处理了：
- kotlin-android 插件注入
- kotlin-stdlib 依赖
- Logcat debug 库
- compileOptions / jvmTarget
- ndk abiFilters
- 签名配置
- debug AndroidManifest

**只需增加一步：复制 overlay 文件**

## 具体修改

### 1. `scripts/post-cap-sync.mjs` — 新增 overlay 文件复制（约 20 行）

```javascript
import { rmSync, copyFileSync, readdirSync } from 'fs'  // 新增导入

// --- Copy overlay files (MainActivity, GoProcessPlugin, proguard, network config) ---
const OVERLAY_DIR = join(__dirname, '..', 'android-overlay')
const JAVA_DIR = join(ANDROID_DIR, 'app', 'src', 'main', 'java', 'com', 'encvgo', 'app')

// 原子替换：先删再写，避免重复声明
if (existsSync(JAVA_DIR)) {
  rmSync(JAVA_DIR, { recursive: true, force: true })
}
mkdirSync(JAVA_DIR, { recursive: true })

// 复制 Kotlin 文件
for (const f of ['MainActivity.kt', 'GoProcessPlugin.kt']) {
  const src = join(OVERLAY_DIR, 'app', 'src', 'main', 'java', 'com', 'encvgo', 'app', f)
  if (existsSync(src)) {
    copyFileSync(src, join(JAVA_DIR, f))
    console.log(`  overlay: ${f}`)
  }
}

// 复制 proguard-rules.pro
const proguardSrc = join(OVERLAY_DIR, 'proguard-rules.pro')
if (existsSync(proguardSrc)) {
  copyFileSync(proguardSrc, join(ANDROID_DIR, 'app', 'proguard-rules.pro'))
  console.log('  overlay: proguard-rules.pro')
}

// 复制 network_security_config.xml
const xmlDir = join(ANDROID_DIR, 'app', 'src', 'main', 'res', 'xml')
mkdirSync(xmlDir, { recursive: true })
const xmlSrc = join(OVERLAY_DIR, 'app', 'src', 'main', 'res', 'xml', 'network_security_config.xml')
if (existsSync(xmlSrc)) {
  copyFileSync(xmlSrc, join(xmlDir, 'network_security_config.xml'))
  console.log('  overlay: network_security_config.xml')
}

// 验证：确保只有一个 MainActivity 类声明
const mainActivityPath = join(JAVA_DIR, 'MainActivity.kt')
if (existsSync(mainActivityPath)) {
  const content = readFileSync(mainActivityPath, 'utf-8')
  const count = (content.match(/class MainActivity/g) || []).length
  if (count !== 1) {
    console.error(`  ERROR: Found ${count} MainActivity class declarations (expected 1)`)
    process.exit(1)
  }
  console.log(`  verified: 1 MainActivity class declaration ✓`)
}
```

### 2. `.github/workflows/android.yml` — 删除 "Apply Android overlay" 步骤

删除整个 "Apply Android overlay" 步骤（包括 MainActivity 复制、GoProcessPlugin 复制、proguard 复制、network config 复制）。

AndroidManifest.xml 的权限和网络配置修改保留在 CI 中（因为涉及 XML merge，用 python 更方便）。或者也可以移到 mjs 中。

### 3. 已完成的修改（无需再改）
- ✅ GoProcessPlugin.kt: `checkPermissions` 添加 `override`
- ✅ post-cap-sync.mjs: `apply plugin` 语法修复
- ✅ post-cap-sync.mjs: jvmTarget = "21"
- ✅ post-cap-sync.mjs: compileOptions VERSION_21

## 为什么这更好

| | 现在（CI YAML） | 改后（post-cap-sync.mjs） |
|--|--|--|
| 执行时机 | cap sync 之后单独步骤 | afterSync hook 自动执行 |
| 删除方式 | `rm -f`（不可靠） | `rmSync({ recursive: true })`（彻底） |
| 写入方式 | `cp`（可能受缓存影响） | `copyFileSync`（Node.js 原生） |
| 验证 | 无 | 自动验证类声明数量 |
| 维护位置 | 分散（YAML + mjs） | 集中（只有 mjs） |
| 调试难度 | 高（需看两处） | 低（只看一处） |

## 验证标准

1. CI 构建通过，无 Kotlin 编译错误
2. APK 中包含 `GoProcessPlugin.class`
3. logcat 搜索 `ENCV-go` 能看到诊断日志
4. GoProcess 插件方法正常工作
