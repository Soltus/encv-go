# 计划：Kotlin 类型检查 + 修复 ffmpeg .so 缺失

## 问题一：本地无 Kotlin 类型检查

`callback.onSuccess(null)` 这种 API 臆造错误只能到 CI 才发现。

### 方案：`compileDebugKotlin` 快速类型检查脚本

#### Step 1：创建 `scripts/check-kotlin.mjs`

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

// 1. post-cap-sync 确保 android/ 目录最新
console.log('[1/2] Running post-cap-sync...')
try {
  execSync('node scripts/post-cap-sync.mjs', { cwd: ROOT, stdio: 'inherit' })
} catch (e) {
  console.error('\n❌ post-cap-sync failed')
  process.exit(1)
}

if (!existsSync(join(ANDROID_DIR, 'gradlew'))) {
  console.error('❌ android/gradlew not found')
  process.exit(1)
}

// 2. compileDebugKotlin — 只跑 Kotlin 编译，不打包 APK
console.log('\n[2/2] Running compileDebugKotlin...')
try {
  execSync('./gradlew compileDebugKotlin --no-daemon --stacktrace', {
    cwd: ANDROID_DIR,
    stdio: 'inherit',
    env: { ...process.env }
  })
} catch (e) {
  console.error('\n❌ Kotlin compilation failed — see errors above')
  process.exit(1)
}

console.log('\n✅ All Kotlin files type-checked successfully')
```

#### Step 2：package.json 添加 npm script

```json
"check:kotlin": "node scripts/check-kotlin.mjs"
```

用法：`npm run check:kotlin`（~30-60s，vs 全量构建 ~3-5min）

---

## 问题二：ffmpeg .so 缺失导致播放崩溃（P0）

### 错误信息（DevLogs 已成功捕获 ✅）

```
UnsatisfiedLinkError: dlopen failed: library "libavcodec.so" not found
  needed by /data/app/.../lib/arm64/libmpv.so
  at MPVLib.<clinit>(MPVLib.kt:12)     ← System.loadLibrary("mpv")
  at MpvPlayerModule.ensureMpvInitialized()
  at MpvPlayerModule.play()
```

### 根因分析

| 项目 | 数值 |
|------|------|
| AAR 大小 | **64.5 MB** |
| 当前 jniLibs | `libmpv.so`(8MB) + `libplayer.so`(tiny) = ~8MB |
| 差异 | **~56MB** = ffmpeg 全部动态库 |

[setup-mpv-libs.sh](file:///workspace/app/encv-mobile/scripts/setup-mpv-libs.sh) 第 27-28 行只提取了两个文件：

```bash
unzip -o "$AAR" "jniLibs/arm64-v8a/libmpv.so"   -d "$TARGET"
unzip -o "$AAR" "jniLibs/arm64-v8a/libplayer.so" -d "$TARGET"
```

**mpv 运行时依赖的完整动态库链**（来自 mpv-android Android.mk）：

```
libavutil.so        → 基础工具（内存、数学、工具函数）
libavcodec.so       → 编解码器（H.264/H.265/VP9/AV1/AAC/...）⚠️ 报错缺失的这个
libavformat.so      → 容器格式（MKV/MP4/TS/FLV/...）
libswresample.so    → 音频重采样
libswscale.so       → 视频色彩空间转换/缩放
libpostproc.so      → 视频后处理滤镜
libmpv.so           → mpv 播放器核心 ✅ 已有
libplayer.so        → JNI wrapper ✅ 已有
```

加载顺序依赖链：`libavutil → libswresample → libpostproc → libswscale → libavcodec → libavformat → libmpv`

### 方案：提取 AAR 中全部 .so 文件

#### Step 3：修改 `setup-mpv-libs.sh`

**删除**第 26-29 行的硬编码双文件提取：

```bash
# ❌ 删除这段
mkdir -p "$TARGET/jniLibs/arm64-v8a"
unzip -o "$AAR" "jniLibs/arm64-v8a/libmpv.so"   -d "$TARGET"
unzip -o "$AAR" "jniLibs/arm64-v8a/libplayer.so" -d "$TARGET"
```

**替换为**：

```bash
# ✅ 提取 AAR 中所有 .so 文件（含完整 ffmpeg 库）
mkdir -p "$TARGET/jniLibs/arm64-v8a"
unzip -o "$AAR" "jniLibs/arm64-v8a/*.so" -d "$TARGET"

echo ""
echo "Extracted .so files:"
find "$TARGET/jniLibs" -name "*.so" -exec ls -lh {} \;
```

效果：一次性提取 arm64-v8a 下所有 `.so`，包括未来 AAR 更新增减的库也自动覆盖。

#### Step 4：post-cap-sync.mjs 同步更新

当前 [post-cap-sync.mjs](file:///workspace/app/encv-mobile/scripts/post-cap-sync.mjs) 第 397 行只同步 `*.kt` 和特定资源文件。需要确认 jniLibs 目录是否被正确复制到 `android/app/`。

检查：`setup-mpv-libs.sh` 输出目标是什么目录？如果是直接输出到 `android-overlay/app/src/main/jniLibs/`，则 post-cap-sync 需要增加 jniLibs 目录的同步。

查看当前脚本的 jniLibs 复制逻辑：

```javascript
// 第 386-394 行 — 资源文件同步
for (const f of ['AndroidManifest.xml', 'proguard-rules.pro', 'network_security_config.xml']) { ... }
```

**需要在资源同步循环中添加 jniLibs 目录递归复制**：

```javascript
// 同步 jniLibs（ffmpeg/mpv 原生库）
const overlayJniLibs = join(OVERLAY_DIR, 'app', 'src', 'main', 'jniLibs')
const targetJniLibs = join(ANDROID_DIR, 'app', 'src', 'main', 'jniLibs')
if (existsSync(overlayJniLibs)) {
  if (existsSync(targetJniLibs)) rmSync(targetJniLibs, { recursive: true })
  cpSync(overlayJniLibs, targetJniLibs, { recursive: true })
  const soCount = readdirSync(targetJniLibs, { recursive: true }).filter(f => f.endsWith('.so')).length
  console.log(`  overlay: jniLibs (${soCount} .so files)`)
}
```

### 改动文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `scripts/check-kotlin.mjs` | **新建** | Kotlin 类型检查入口 |
| `package.json` | 修改 | 添加 `"check:kotlin"` script |
| `scripts/setup-mpv-libs.sh` | 修改 | 提取全部 `*.so` 替代硬编码 2 个文件 |
| `scripts/post-cap-sync.mjs` | 修改 | 添加 jniLibs 目录同步 |

### 预期效果

1. `npm run check:kotlin` → ~60s 内发现所有 Kotlin 类型/API 错误
2. CI 早期门控 → 类型错误在 assembleDebug 之前暴露
3. 播放视频 → `System.loadLibrary("mpv")` 成功找到所有依赖的 ffmpeg .so → 视频正常播放
4. 未来 AAR 更新 → 自动包含新版本的全部 .so，无需改脚本

### 包体积影响

| 架构 | 当前 | 修复后（预估） |
|------|------|----------------|
| arm64-v8a | ~8 MB | ~55-60 MB |
| armeabi-v7a | 不存在 | 可选添加 |

注：只保留 arm64-v8a（现代设备全覆盖），避免包体积翻倍。
