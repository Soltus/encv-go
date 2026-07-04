# 计划：添加本地 Kotlin 类型检查

## 问题

`callback.onSuccess(null)` 这种 API 臆造错误（Lynx `Callback` 只有 `invoke()` 没有 `onSuccess()`）只能到 CI 的 `assembleDebug` 全量构建才发现。开发阶段完全无本地检查能力。

## 方案：Gradle compileDebugKotlin 快速类型检查

### 核心思路

利用已有的 Gradle 构建链路，只跑 **Kotlin 编译阶段**（不打包 APK、不处理资源、不签名），速度比全量构建快得多，但能捕获所有类型错误：

```
全量构建: cap sync → gradlew assembleDebug  (~3-5分钟)
类型检查: cap sync → gradlew compileDebugKotlin  (~30-60秒)
```

### 为什么不用其他方案

| 方案 | 问题 |
|------|------|
| `kotlinc` 独立编译 | 需要 Android jar + Lynx SDK jar + mpv jar 全部手动凑 classpath，维护成本高 |
| 写 stub 类模拟 API | stub 本身可能过时，和真实 SDK 行为不一致 |
| IDE 检查 | 依赖开发者个人环境，不可自动化、不入 CI |
| **`compileDebugKotlin`** | ✅ 用真实依赖、Gradle 自动管理 classpath、CI 可跑、本地可跑 |

### 实施步骤

#### Step 1：创建 `scripts/check-kotlin.mjs`

新建文件，逻辑：

```javascript
// scripts/check-kotlin.mjs
import { execSync } from 'child_process'
import { existsSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ROOT = join(__dirname, '..')
const ANDROID_DIR = join(ROOT, 'android')

console.log('=== Kotlin Type Check ===\n')

// 1. 运行 post-cap-sync 确保 android/ 目录最新
console.log('[1/3] Running post-cap-sync...')
try {
  execSync('node scripts/post-cap-sync.mjs', { cwd: ROOT, stdio: 'inherit' })
} catch (e) {
  console.error('\n❌ post-cap-sync failed')
  process.exit(1)
}

// 2. 检查 android 目录存在
if (!existsSync(join(ANDROID_DIR, 'gradlew'))) {
  console.error('❌ android/gradlew not found. Run "npx cap sync android" first.')
  process.exit(1)
}

// 3. 运行 Kotlin 编译检查
console.log('\n[2/3] Running compileDebugKotlin...')
try {
  execSync('./gradlew compileDebugKotlin --no-daemon --stacktrace', {
    cwd: ANDROID_DIR,
    stdio: 'inherit',
    env: { ...process.env, ANDROID_HOME: process.env.ANDROID_HOME || '' }
  })
} catch (e) {
  console.error('\n❌ Kotlin compilation failed — see errors above')
  process.exit(1)
}

console.log('\n✅ All Kotlin files type-checked successfully')
```

关键点：
- 先执行 `post-cap-sync.mjs` 确保 overlay 文件已同步到 `android/app/`
- 然后调用 `compileDebugKotlin` Gradle task
- 退出码非零时明确报错

#### Step 2：在 `package.json` 添加 npm script

```json
{
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc --noEmit && vite build",
    "preview": "vite preview",
    "capacitor:sync:after": "node scripts/post-cap-sync.mjs",
    "check:kotlin": "node scripts/check-kotlin.mjs"
  }
}
```

开发者本地运行：`npm run check:kotlin`

#### Step 3：在 CI workflow 中添加早期检查步骤

在 [android.yml](file:///workspace/.github/workflows/android.yml) 的 `Build APK` 步骤之前（第 216 行附近），插入：

```yaml
# === Fast Kotlin type check (before slow full build) ===
- name: Kotlin type check
  run:
    cd app/encv-mobile && npm run check:kotlin
```

放在 `Copy assets, overlay files, and release config` 步骤之后、`Build Debug/Release APK` 之前。这样：
- 类型错误在 ~1 分钟内暴露，不必等 3-5 分钟的全量构建
- 后续的 build step 基本不会再有编译错误（资源/R 类问题除外）

### 使用场景

| 场景 | 命令 | 耗时 |
|------|------|------|
| 本地改完 .kt 文件后自检 | `cd app/encv-mobile && npm run check:kotlin` | ~30-60s |
| PR / push 前 | 同上 | ~30-60s |
| CI 早期门控 | workflow 自动运行 | ~60s |
| 完整构建（现有） | `gradlew assembleDebug` | ~3-5min |

### 前置条件

- 需要本地安装 Android SDK（`ANDROID_HOME` 已设置）
- 这与运行完整构建的前置条件相同，不需要额外安装
- 如果没有 Android SDK，脚本会给出清晰的错误提示

### 不覆盖的范围

此检查**能捕获**：
- ✅ API 调用错误（如 `onSuccess` → `invoke`）
- ✅ 类型不匹配
- ✅ 未解析的引用（如漏加文件导致 `Unresolved reference`）
- ✅ null 安全问题

此检查**不能替代**全量构建：
- ❌ 资源引用错误（R.layout.xxx 找不到）
- ❌ ProGuard/Shrinker 问题
- ❌ 打包/签名/Dex 问题
- ❌ 运行时错误
