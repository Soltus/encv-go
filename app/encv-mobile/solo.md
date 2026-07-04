# ENCV-Mobile CI/CD 构建踩坑记录

> 本文档记录在将 encv-mobile (Ionic Vue + Capacitor) 接入 GitHub Actions 自动构建 Android APK 过程中遇到的所有问题及解决方案。

---

## 0. 核心结论（先读这个）

### ⚠️ 头号问题：APK 未签名（反复出现）

**错误现象（多次出现）：**

```
BUILD SUCCESSFUL in 1m 58s
123 actionable tasks: 123 executed

Verifying signature...
DOES NOT VERIFY
ERROR: Missing META-INF/MANIFEST.MF
Error: Process completed with exit code 1.
```

**Gradle 构建成功但 apksigner 校验失败 → APK 没有被签名。**

这个问题跨越了多个方案迭代，每次都以为修好了但下次构建仍然出现：

| 时间点 | 使用的方案 | 结果 |
|--------|-----------|------|
| 第1次 | jarsigner 后处理 | ❌ AGP 8.x 不兼容 v1 签名 |
| 第2次 | 正则注入 signingConfigs | ❌ 正则破坏 build.gradle 结构 |
| 第3次 | `apply from` 外部 gradle 文件 | ❌ Gradle 8 属性加载时序，**同样 BUILD SUCCESSFUL 但未签名** |
| 第4次 | `apply from` + gradle.properties | ❌ 同上 + AndroidX 缺失 |
| 第5次 | 精确字符串锚点直接注入 build.gradle | ✅ 注入成功，但 **keystore 路径在 android/ 内被 cap add 覆盖** |
| 第6次（当前） | keystore 移到 android/ 外 + 直接注入 | 🔄 **待 CI 验证** |

### Capacitor cap sync 到底改了哪些文件？

通过阅读 [`update.js`](../node_modules/@capacitor/cli/dist/android/update.js) 源码确认：

| 文件 | cap sync 行为 | 能否安全修改？ |
|------|-------------|--------------|
| `android/app/build.gradle` | ❌ **完全不触碰** | ✅ **最安全** — 只在 `cap add android` 时生成一次 |
| `android/build.gradle` (root) | ❌ 不触碰 | ✅ 安全 |
| `android/gradle.properties` | ❌ 不触碰 | ✅ **最安全** |
| `android/variables.gradle` | ❌ 只读取不写入 | ✅ 安全 |
| `android/settings.gradle` | ❌ 不触碰 | ✅ 安全 |
| `android/capacitor.settings.gradle` | ⚠️ 每次覆盖 | ❌ |
| `android/app/capacitor.build.gradle` | ⚠️ 每次覆盖 | ❌ |

### 当前方案（第5次尝试，待验证）

既然 `app/build.gradle` 不会被 cap sync 覆盖，直接用**精确字符串替换**修改它：

```
post-cap-sync.mjs（Capacitor hook 触发）
  ├─ root/build.gradle: +kotlin-gradle-plugin
  └─ app/build.gradle:
      ├─ +kotlin-android plugin          (精确字符串替换)
      ├─ +kotlin-stdlib dependency       (精确字符串替换)
      ├─ +ndk { abiFilters 'arm64-v8a' } (精确字符串替换)
      ├─ versionCode/versionName        (简单正则)
      └─ release 模式时 (ENCV_VERSION 非空):
          ├─ "android {" → 注入 signingConfigs 块   (精确字符串锚点)
          └─ "minifyEnabled false" → 替换为完整配置 (精确字符串锚点)
```

**关键设计决策：**
- 所有修改都是幂等的（`c.includes()` 先检查）
- 使用精确字符串锚点（`"minifyEnabled false"`、`"android {"`），不用模糊正则
- 签名配置仅在 `ENCV_VERSION` 非空时注入（区分 debug/release）
- keystore 路径写死为 `file('../../keystore/release.jks')`（**必须在 android/ 目录外**）

---

## 1. APK 实机后端未连接

### 现象
APK 安装到真机后显示"后端未连接"，且 `lib/` 目录为空。

### 根因（3 个独立问题）

#### 1.1 AndroidManifest 缺少 `usesCleartextTraffic="true"`
- **原因**：Android 9+ 默认禁止明文 HTTP 流量，前端无法连接 `http://127.0.0.1:2025`
- **修复**：在 AndroidManifest.xml 的 `<application>` 标签添加：
  ```xml
  android:usesCleartextTraffic="true"
  android:networkSecurityConfig="@xml/network_security_config"
  ```
- **配套**：创建 `res/xml/network_security_config.xml` 允许 127.0.0.1/localhost 明文

#### 1.2 Android 10+ (API 29+) noexec 限制
- **原因**：从 Android 10 开始，`filesDir` 挂载为 noexec，无法直接执行二进制文件
- **修复**：MainActivity.kt 多位置回退策略：
  ```kotlin
  val candidateDirs = listOf(
      filesDir to "filesDir",
      cacheDir to "cacheDir",
      getExternalFilesDir(null) to "externalFilesDir",
  )
  ```

#### 1.3 "原生库是空的" 是正常现象
- Go 二进制打包在 `assets/encv-go`，不是 JNI 的 `lib/*.so`
- MainActivity.kt 在启动时从 assets 提取到 filesDir/cacheDir 后执行

---

## 2. heredoc 缩进导致 shell 语法错误

YAML `run: |` 块中 heredoc 终止符带缩进，bash 无法识别。**用 echo 替代 heredoc。**

---

## 3. jarsigner 签名失败 → 无效安装包

AGP 8.x 使用 v2/v3/v4 签名，jarsigner 只支持 v1。必须用 **Gradle 原生签名**（signingConfigs + signingConfig）。

---

## 4. 固定签名实现覆盖安装

keystore 通过 `actions/cache@v4` 跨 run 复用，artifact 不上传 keystore。

---

## 4.5 ⚠️ keystore 不能放在 android/ 目录内

### 错误现象
```
Execution failed for task ':app:validateSigningRelease'.
Keystore file '.../android/keystore/release.jks' not found for signing config 'release'.
```

### 根因

CI 执行时序：
```
1. 创建 keystore → app/encv-mobile/android/keystore/release.jks   ✅
2. cap add android → 重建整个 android/ 目录                        💥 keystore 被覆盖!
3. post-cap-sync.mjs 注入 signingConfigs (引用 ../keystore/)       → 文件已不存在
```

**`cap add android` 会从模板解压出全新的 `android/` 目录，之前放在 `android/keystore/` 的文件全部丢失。**

即使 cache 命中恢复了 `android/keystore/`，如果 CI 判定需要走 `rm -rf ./android && npx cap add android` 分支（比如 checkout 后无 android/），同样会丢失。

### 修复

keystore 放在 **android/ 外面**：`app/encv-mobile/keystore/release.jks`

| 项目 | 旧路径（❌） | 新路径（✅） |
|------|------------|------------|
| CI cache | `app/encv-mobile/android/keystore` | `app/encv-mobile/keystore` |
| CI 创建 | 同上 | 同上 |
| build.gradle 引用 | `file('../keystore/release.jks')` | `file('../../keystore/release.jks')` |

从 `android/app/build.gradle` 视角：`../` = `android/`，`../../` = `encv-mobile/`。

---

## 5. cap add android 重复添加错误

checkout 后 `./android` 存在但非完整项目。检查 `build.gradle + variables.gradle` 双文件判断有效性。

---

## 6. shrinkResources 要求 minifyEnabled 先开启

顺序必须是 `minifyEnabled true` → `shrinkResources true`。

---

## 7. 正则替换 build.gradle 的各种灾难（血泪史）

### 尝试过的所有失败方案

| # | 方案 | 失败原因 |
|---|------|---------|
| 1 | 行内插入 `release {` 后追加内容 | minifyEnabled 丢失或重复 |
| 2 | 整体替换 `release {}` 块（正则） | 匹配到 `signingConfigs.release` 而非 `buildTypes.release` |
| 3 | 先注入 signingConfigs 再替换 release | 把 minifyEnabled 写进 SigningConfig 对象 |
| 4 | 用 `buildTypes\s*\{...release\` 限定上下文 | 结构破坏，花括号不匹配 |
| 5 | **`apply from: '../encv-release.gradle'` 外部文件** | **Gradle 8 属性加载时序问题，BUILD SUCCESSFUL 但 APK 未签名（DOES NOT VERIFY / Missing META-INF/MANIFEST.MF）** |
| 6 | `apply from` + `gradle.properties` 引用凭据 | 同上 + 额外遗漏 `android.useAndroidX=true` |

### 为什么 `apply from` 方案看起来能编译但不签名？

这是最令人困惑的一点：
- Gradle 编译时不报错 → 说明语法正确
- `assembleRelease` 成功 → 说明 release 构建类型存在
- 但 `apksigner verify` 报 `Missing META-INF/MANIFEST.MF` → **signingConfig 根本没生效**

推测原因：Gradle 8 的配置阶段，`apply from` 引入的外部文件的属性引用可能在 `signingConfigs` 解析时尚未完成初始化，导致静默跳过签名步骤。Gradle 不会报错，只是不签名。

### 当前方案（第7次尝试）：精确字符串锚点直接注入

**核心思路变化**：不再使用任何外部文件或间接机制，把签名配置**字面量**写入 build.gradle 本体。

```javascript
// post-cap-sync.mjs 关键代码

// 条件：必须在 release 模式（ENCV_VERSION 非空）且锚点存在
if (version && c.includes('minifyEnabled false')) {

  // 步骤1: 在 "android {" 后面插入 signingConfigs 块
  const scBlock = `
    signingConfigs {
        release {
            storeFile file('../../keystore/release.jks')
            storePassword 'encv2025'
            keyAlias 'encvrelease'
            keyPassword 'encv2025'
        }
    }
  `
  c = c.replace("android {", "android {" + scBlock)

  // 步骤2: 把 "minifyEnabled false" 替换为启用签名的完整配置
  c = c.replace("minifyEnabled false",
    "minifyEnabled true\n        shrinkResources true\n        signingConfig signingConfigs.release")
}
```

**与之前所有方案的本质区别**：
- 签名凭据是**硬编码字面量**，不是属性引用，不存在加载时序问题
- `signingConfigs` 和 `signingConfig` 在**同一个文件**内定义和引用
- 不依赖 `apply from`、`gradle.properties` 等任何外部机制
- 使用精确字符串匹配而非正则，不会破坏结构

**待验证项**：
- [ ] CI 构建后 `apksigner verify --print-certs` 通过
- [ ] APK 可正常安装到真机并覆盖升级
- [ ] 多次构建签名一致性（同一 keystore）

---

## 8. Capacitor hooks 配置方式

### 错误
`capacitor.config.ts` 的 `hooks` 属性不存在于 TypeScript 类型定义。

### 正确（Capacitor 8 官方方式）
```json
// package.json
{ "scripts": { "capacitor:sync:after": "node scripts/post-cap-sync.mjs" } }
```

---

## 9. 文件放置规则

自定义文件放 `android-overlay/`，CI 步骤复制到 `android/`。`cap sync` 会清除 `android/` 中非 Capacitor 管理的文件。

**已废弃文件**（不再使用但保留在 android-overlay 中作为历史参考）：
- `encv-release.gradle` — 原 `apply from` 方案的产物，已弃用

---

## 10. apksigner 路径硬编码问题

`apksigner` 不在 PATH 中，且版本号会变。用 `find $ANDROID_HOME/build-tools -name "apksigner"` 动态查找。

---

## 11. gradle.properties 覆盖导致 AndroidX 缺失

我们的 `gradle.properties` 覆盖了 Capacitor 默认的，漏了 `android.useAndroidX=true`。Capacitor 依赖 AndroidX（appcompat、activity），没有此标志直接报错。

**教训**：覆盖任何配置文件时，必须保留原文件的所有必要属性。

当前 `android-overlay/gradle.properties` 内容：
```properties
# AndroidX (required by Capacitor)
android.useAndroidX=true
android.enableJetifier=true
```

注意：签名凭据已经不在 gradle.properties 中了（改由 post-cap-sync.mjs 硬编码到 build.gradle）。

---

## 12. Release APK 全部优化措施

| 优化项 | 效果 | 实现方式 |
|--------|------|---------|
| `-ldflags="-s -w"` | 二进制 ~30% | Go 编译参数 |
| `abiFilters 'arm64-v8a'` | APK ~30% | post-cap-sync 直接注入 |
| `minifyEnabled true` | R8 代码压缩 | post-cap-sync 直接注入 |
| `shrinkResources true` | 移除未使用资源 | post-cap-sync 直接注入 |

---

## 13. 构建步骤正确顺序（最终版）

```
Checkout → Setup tools (Node/Go/JDK/Android SDK)
    ↓
npm install && npm run build                    # 前端构建
go build (CGO_ENABLED=0, arm64)               # Go 后端二进制
    ↓
Restore/Create keystore (cache 或新建)         # 签名文件就位 (keystore/release.jks，在 android/ 外)
    ↓
npx cap sync android                           # 触发 afterSync hook
  → post-cap-sync.mjs (直接修改 build.gradle):
      - root/build.gradle: +kotlin plugin
      - app/build.gradle: +kotlin, +ndk, +signingConfigs(硬编码), +version
    ↓
cp overlay files (MainActivity + manifest + gradle.properties)
cp assets (Go binary + config)                  # 打包后端
    ↓
./gradlew assembleRelease                         # Gradle 原生签名 (signingConfigs.release)
apksigner verify --print-certs                   # 强制校验，失败则终止
    ↓
upload artifact (仅 APK，不含 keystore)
```

---

## 14. 调试清单：当 APK 再次未签名时

如果未来再次遇到 `DOES NOT VERIFY / Missing META-INF/MANIFEST.MF`：

1. **确认 post-cap-sync.mjs 执行了**
   - CI 日志中应有 `encv-post-cap-sync: applying Android customizations...`
   - 应有 `release: signing + minify + shrink applied`
   - 如果只有 `debug mode` → `ENCV_VERSION` 为空

2. **检查 build.gradle 内容**
   - 在 CI 中加一步 `cat app/build/build.gradle | grep -A10 signingConfigs`
   - 确认 `signingConfigs.release` 块存在且路径正确

3. **确认 keystore 文件存在**
   - `ls -lh keystore/release.jks`（注意：在 android/ **外面**）
   - cache 命中时应显示 "Keystore ready"

4. **检查 Gradle 是否真的执行了签名**
   - 在 `assembleRelease` 输出中搜索 `signing` 相关任务
   - 如果完全没有 signing 任务 → signingConfig 没生效

5. **常见回归原因**
   - Capacitor 升级后模板的 `"minifyEnabled false"` 锚点文字变了
   - `post-cap-sync.mjs` 的 `c.includes('minifyEnabled false')` 检查失败导致跳过
   - keystore cache 被清除后重新生成，但新 keystore 密码不匹配
   - **keystore 放回了 android/ 目录内，被 cap add 覆盖**（必须放外面！）
