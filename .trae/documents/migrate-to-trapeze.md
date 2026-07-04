# Trapeze 迁移计划：替换 post-cap-sync.mjs

## 背景

当前 `scripts/post-cap-sync.mjs` 用大量字符串替换和文件拷贝实现 Android 配置定制，存在以下问题：
- **不可维护**：600+ 行 Node 脚本，大量 `c.includes()` + `c.replace()` 模式
- **不健壮**：无法保证替换位置正确，容易产生重复注入
- **不透明**：无法预览变更，每次运行都要实际修改文件
- **违反 Capacitor 生态规范**：Trapeze 是 Capacitor 官方推荐的配置管理工具

Trapeze (`@trapezedev/configure`) 提供声明式 YAML 配置，支持 Android Gradle、Manifest、资源文件、XML 等操作的预览(`--diff`)和执行。

## 当前 post-cap-sync.mjs 功能拆分

| 功能 | Trapeze 可替代 | 保留 Node.js |
|------|--------------|-------------|
| root build.gradle kotlin plugin + jitpack | ✅ `gradle.insert` | |
| app build.gradle kotlin-android plugin | ✅ `gradle.insert` | |
| app build.gradle kotlin-stdlib + Logcat dep | ✅ `gradle.insert` | |
| app build.gradle buildConfig + USE_LYNX_PLAYER | ✅ `gradle.replace` | |
| app build.gradle compileOptions (JAVA 21) | ✅ `gradle.replace` | |
| app build.gradle ndk abiFilters | ✅ `gradle.replace` | |
| app build.gradle jniLibs.srcDirs | ✅ `gradle.insert` | |
| app build.gradle packaging options | ✅ `gradle.insert` | |
| app build.gradle signing config (release) | ✅ `gradle.insert` | |
| app build.gradle kotlin JVM target | ✅ `gradle.insert` | |
| app build.gradle sourceSets | ✅ `gradle.insert` | |
| app build.gradle Lynx SDK 依赖 | ✅ `gradle.insert` | |
| app build.gradle 移除 mpv-android-lib | ✅ `gradle.delete` | |
| Manifest `<meta-data>` Logcat (debug) | ✅ `manifest.inject` | |
| Manifest `<meta-data>` Logcat (release) | ✅ `manifest.inject` | |
| AndroidManifest.xml 覆盖拷贝 | ✅ `copy` | |
| proguard-rules.pro 覆盖拷贝 | ✅ `copy` | |
| layout XML 覆盖拷贝 | ✅ `copy` | |
| network_security_config.xml 覆盖拷贝 | ✅ `copy` | |
| config.mobile.json → assets | ✅ `copy` | |
| Kotlin 文件同步 (overlay → android) | ❌ 需递归拷贝 | ✅ 简化脚本 |
| jniLibs .so 同步 (overlay → android) | ❌ 需递归拷贝 | ✅ 简化脚本 |
| jni/ 目录同步 (overlay → android) | ❌ 需递归拷贝 | ✅ 简化脚本 |
| include/ 目录同步 (overlay → android) | ❌ 需递归拷贝 | ✅ 简化脚本 |
| Lynx bundle → assets | ✅ `copy` | |
| MainActivity 单例验证 | ❌ 脚本逻辑 | ✅ 简化脚本 |

## 实施步骤

### Step 1: 安装依赖

在 `app/encv-mobile/` 下安装：

```bash
cd app/encv-mobile
npm install @trapezedev/configure --save-dev
```

### Step 2: 创建 Trapeze 配置 YAML

在 `app/encv-mobile/trapeze.yaml`，分为 debug 和 release 两个变量环境：

```yaml
# trapeze.yaml
vars:
  BUILD_TYPE:
    default: debug
  ENCV_VERSION: ''

platforms:
  android:
    gradle:
      # Root build.gradle: kotlin plugin + jitpack
      - file: build.gradle
        target:
          buildscript:
        insert:
          - classpath: '"org.jetbrains.kotlin:kotlin-gradle-plugin:2.1.0"'
      - file: build.gradle
        target:
          allprojects:
        insert: |
          maven { url 'https://jitpack.io' }

      # app/build.gradle: kotlin plugin
      - file: app/build.gradle
        target:
          plugins:
        insert: |
          id 'kotlin-android'
        insertType: method

      # kotlin-stdlib + Logcat
      - file: app/build.gradle
        target:
          dependencies:
        insert:
          - implementation: '"org.jetbrains.kotlin:kotlin-stdlib:2.1.0"'
          - debugImplementation: "'com.github.getActivity:Logcat:13.0'"
      # 删除 mpv-android-lib maven 依赖 (通过 delete，精确匹配 implementation 行)
      - file: app/build.gradle
        delete: "implementation\\('io\\.github\\.abdallahmehiz:mpv[^)]*'\\)"

      # buildConfig USE_LYNX_PLAYER
      - file: app/build.gradle
        target:
          android:
            defaultConfig:
        insert: |
          buildConfigField "boolean", "USE_LYNX_PLAYER", "true"
        insertType: variable

      # compileOptions
      - file: app/build.gradle
        target:
          android:
        insert: |
          compileOptions {
              targetCompatibility JavaVersion.VERSION_21
              sourceCompatibility JavaVersion.VERSION_21
          }
      - file: app/build.gradle
        target:
          android:
        insert: |
          buildFeatures {
              buildConfig = true
          }

      # appcompat
      - file: app/build.gradle
        target:
          dependencies:
        insert:
          - implementation: "'androidx.appcompat:appcompat:1.6.1'"

      # ndk abiFilters
      - file: app/build.gradle
        target:
          android:
            defaultConfig:
        insert: |
          ndk {
              abiFilters 'arm64-v8a'
          }

      # Lynx SDK 3.7 依赖
      - file: app/build.gradle
        target:
          dependencies:
        insert:
          - implementation: "'org.lynxsdk.lynx:lynx:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-jssdk:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-trace:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:primjs:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-service-image:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-service-log:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-service-http:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-service-devtool:3.7.0'"
          - implementation: "'org.lynxsdk.lynx:lynx-devtool:3.7.0'"
          - implementation: "'com.facebook.fresco:fresco:2.3.0'"
          - implementation: "'com.facebook.fresco:animated-gif:2.3.0'"
          - implementation: "'com.facebook.fresco:animated-webp:2.3.0'"
          - implementation: "'com.facebook.fresco:webpsupport:2.3.0'"
          - implementation: "'com.facebook.fresco:animated-base:2.3.0'"
          - implementation: "'com.squareup.okhttp3:okhttp:4.9.0'"

      # jniLibs.srcDirs
      - file: app/build.gradle
        target:
          android:
        insert: |
          sourceSets {
              main {
                  jniLibs.srcDirs = ['src/main/jniLibs']
              }
          }

      # packaging options
      - file: app/build.gradle
        target:
          android:
        insert: |
          packaging {
              jniLibs {
                  useLegacyPackaging = true
                  pickFirsts += ['**/*.so']
              }
              resources {
                  pickFirsts += ['**/*.so']
              }
          }

      # Kotlin JVM target
      - file: app/build.gradle
        insert: |
          tasks.withType(org.jetbrains.kotlin.gradle.tasks.KotlinCompile).configureEach {
              kotlinOptions {
                  jvmTarget = "21"
              }
          }

      # Kotlin sourceSets
      - file: app/build.gradle
        insert: |
          android.sourceSets {
              main.java.srcDirs += 'src/main/java'
          }

      # Release signing config (仅 BUILD_TYPE=release)
      - when: '$BUILD_TYPE == "release" && $ENCV_VERSION != ""'
        file: app/build.gradle
        target:
          android:
        insert: |
          signingConfigs {
              release {
                  storeFile file('../../keystore/release.jks')
                  storePassword 'encv2025'
                  keyAlias 'encvrelease'
                  keyPassword 'encv2025'
              }
          }
      - when: '$BUILD_TYPE == "release" && $ENCV_VERSION != ""'
        file: app/build.gradle
        target:
          android:
            buildTypes:
              release:
        replace:
          minifyEnabled: true
      - when: '$BUILD_TYPE == "release" && $ENCV_VERSION != ""'
        file: app/build.gradle
        target:
          android:
            buildTypes:
              release:
        insert: |
          shrinkResources true
          signingConfig signingConfigs.release

    manifest:
      # Logcat debug floating + notify (仅 BUILD_TYPE=debug)
      - when: '$BUILD_TYPE == "debug"'
        file: AndroidManifest.xml
        target: manifest/application
        inject: |
          <meta-data
              android:name="LogcatWindowEntrance"
              android:value="true" />
          <meta-data
              android:name="LogcatNotifyEntrance"
              android:value="true" />

      # Logcat debug floating (release, always present)
      - when: '$BUILD_TYPE == "release"'
        file: AndroidManifest.xml
        target: manifest/application
        inject: |
          <meta-data
              android:name="LogcatWindowEntrance"
              android:value="false" />

    copy:
      # AndroidManifest.xml (来自 overlay)
      - src: android-overlay/app/src/main/AndroidManifest.xml
        dest: app/src/main/AndroidManifest.xml

      # proguard-rules.pro
      - src: android-overlay/proguard-rules.pro
        dest: app/proguard-rules.pro

      # layout
      - src: android-overlay/app/src/main/res/layout/lynx_player_activity.xml
        dest: app/src/main/res/layout/lynx_player_activity.xml

      # network_security_config.xml
      - src: android-overlay/app/src/main/res/xml/network_security_config.xml
        dest: app/src/main/res/xml/network_security_config.xml

      # Lynx bundle → assets
      - src: lynx-player/dist/player.lynx.bundle
        dest: app/src/main/assets/player.lynx.bundle

      # config.mobile.json → assets
      - src: assets/config.mobile.json
        dest: app/src/main/assets/config.mobile.json

    res:
      # Debug-only AndroidManifest (debug 构建时启用 Logcat floating window)
      # (通过 manifest inject 操作已覆盖，保留此作为未来扩展)
```

### Step 3: 简化为 KotlinSync 脚本

将 `post-cap-sync.mjs` 中 Trapeze 无法处理的部分提取为简化脚本 `scripts/sync-native.mjs`：

```js
import { rmSync, mkdirSync, copyFileSync, cpSync, readdirSync, statSync, existsSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ANDROID_DIR = join(__dirname, '..', 'android')
const OVERLAY_DIR = join(__dirname, '..', 'android-overlay')
const LYNX_BUNDLE_PATH = join(__dirname, '..', 'lynx-player', 'dist', 'player.lynx.bundle')
const JAVA_DIR = join(ANDROID_DIR, 'app', 'src', 'main', 'java', 'com', 'encvgo', 'app')

// 1. 清理并重建 java 目录
if (existsSync(JAVA_DIR)) rmSync(JAVA_DIR, { recursive: true, force: true })
mkdirSync(JAVA_DIR, { recursive: true })

// 2. 同步 Kotlin 文件 (overlay java/ → android java/)
function syncKtFiles(srcDir, destDir) {
  if (!existsSync(srcDir)) return
  function walk(dir, relBase) {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry)
      const rel = relBase ? join(relBase, entry) : entry
      if (statSync(full).isDirectory()) {
        walk(full, rel)
      } else if (entry.endsWith('.kt')) {
        const dest = join(destDir, rel)
        mkdirSync(dirname(dest), { recursive: true })
        copyFileSync(full, dest)
        console.log(`  kt: ${rel}`)
      }
    }
  }
  walk(srcDir, '')
}
syncKtFiles(join(OVERLAY_DIR, 'app/src/main/java/com/encvgo/app'), JAVA_DIR)
syncKtFiles(join(OVERLAY_DIR, 'app/src/main/java/is'), join(ANDROID_DIR, 'app/src/main/java/is'))

// 3. 同步 native 库 (jniLibs .so)
const overlayJni = join(OVERLAY_DIR, 'app/src/main/jniLibs')
const targetJni = join(ANDROID_DIR, 'app/src/main/jniLibs')
const ALLOWED_ABIS = ['arm64-v8a']
if (existsSync(overlayJni)) {
  for (const abi of readdirSync(overlayJni)) {
    if (!ALLOWED_ABIS.includes(abi)) continue
    const abiDir = join(overlayJni, abi)
    if (statSync(abiDir).isDirectory()) {
      const targetAbi = join(targetJni, abi)
      mkdirSync(targetAbi, { recursive: true })
      for (const so of readdirSync(abiDir)) {
        if (so.endsWith('.so')) {
          copyFileSync(join(abiDir, so), join(targetAbi, so))
          console.log(`  so: ${abi}/${so}`)
        }
      }
    }
  }
}

// 4. 同步 jni/ C++ 源码
const overlayJniSrc = join(OVERLAY_DIR, 'app/src/main/jni')
const targetJniSrc = join(ANDROID_DIR, 'app/src/main/jni')
if (existsSync(overlayJniSrc)) {
  if (existsSync(targetJniSrc)) rmSync(targetJniSrc, { recursive: true })
  cpSync(overlayJniSrc, targetJniSrc, { recursive: true })
  console.log(`  jni: synced`)
}

// 5. 同步 include/ 头文件
const overlayInc = join(OVERLAY_DIR, 'app/src/main/include')
const targetInc = join(ANDROID_DIR, 'app/src/main/include')
if (existsSync(overlayInc)) {
  if (existsSync(targetInc)) rmSync(targetInc, { recursive: true })
  cpSync(overlayInc, targetInc, { recursive: true })
  console.log(`  include: synced`)
}

// 6. MainActivity 单例验证
const mainActivity = join(JAVA_DIR, 'MainActivity.kt')
if (existsSync(mainActivity)) {
  const content = (await import('fs')).readFileSync(mainActivity, 'utf-8')
  const count = (content.match(/class MainActivity/g) || []).length
  if (count !== 1) {
    console.error(`ERROR: ${count} MainActivity declarations (expected 1)`)
    process.exit(1)
  }
  console.log('  MainActivity: 1 declaration ✓')
}

// 7. 验证 Lynx bundle
if (!existsSync(LYNX_BUNDLE_PATH)) {
  console.error('ERROR: Lynx bundle not found. Run: cd lynx-player && npm install && npm run build')
  process.exit(1)
}
```

### Step 4: 更新 package.json scripts

```json
{
  "scripts": {
    "sync:android": "node scripts/sync-native.mjs",
    "configure:android": "npx trapeze run trapeze.yaml --android-project android",
    "postcap": "npm run sync:android && npm run configure:android"
  }
}
```

CI 中的调用从 `node scripts/post-cap-sync.mjs` 改为 `npm run postcap`。

### Step 5: 更新 CI 配置

更新 GitHub Actions workflow 中的构建命令：
- 移除 `node scripts/post-cap-sync.mjs`
- 改为 `npm run postcap`

### Step 6: 验证

1. 预览 Trapeze 变更（不实际修改文件）：
   ```bash
   cd app/encv-mobile
   BUILD_TYPE=debug npx trapeze run trapeze.yaml --android-project android --diff
   ```

2. 完整运行并验证 Gradle sync：
   ```bash
   cd app/encv-mobile
   BUILD_TYPE=debug npm run postcap
   cd android && ./gradlew assembleDebug --dry-run
   ```

## 注意事项

1. **Java 依赖**：Trapeze 的 Gradle 操作依赖 Java 运行时（`JAVA_HOME`）。CI 环境需确保 Java 21 可用。

2. **Trapeze 的 `when` 条件**：当 `when` 条件不满足时，对应操作会被静默跳过，这是理想的行为。

3. **Gradle `insertType`**：非方法调用的 Gradle 行（如 `classpath "..."`）使用默认 `insertType: method`，而 `buildConfigField`/`ndexFilters` 等变量赋值行需要 `insertType: variable`。

4. **Gradle `delete`**：Trapeze 的 `delete` 使用正则表达式，需要正确转义。

5. **覆盖顺序**：`sync-native.mjs` 必须先运行（清理+重建目录），`trapeze` 后运行（覆盖 Manifest 等文件）。

6. **Lynx bundle 依赖**：Trapeze 的 `copy` 操作无法在目标不存在时创建中间目录，需确保 `mkdir -p` 已在 sync-native.mjs 中处理。
