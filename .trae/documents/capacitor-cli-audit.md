# Capacitor CLI 命令使用审计

## 审计范围

审查项目中所有 Capacitor CLI 命令的使用，识别：
1. **不当使用** — 用错了命令或方式
2. **应该用但没用** — 能简化现有流程的命令
3. **冗余/遗留** — 不再需要的命令或配置

---

## 发现的问题

### 🔴 问题 1：CI 用 sed 注入 signing config，应该用 `cap build`

**当前做法**（[android.yml L121-135](file:///workspace/.github/workflows/android.yml#L121-L135)）：
```bash
# 1. sed 注入 signingConfigs 块
SIGNING_BLOCK='signingConfigs { release { ... } }'
sed -i "/android {/a\\        $SIGNING_BLOCK" "$APP_BUILD"

# 2. sed 替换 minifyEnabled + 添加 signingConfig
sed -i 's/\(release {[^}]*\)minifyEnabled false/\1minifyEnabled true\n            shrinkResources true\n            signingConfig signingConfigs.release/' "$APP_BUILD"

# 3. 手动 ./gradlew assembleRelease
./gradlew assembleRelease
```

**问题**：
- sed 注入是脆弱的字符串操作，依赖 `android {` 锚点位置
- 每次构建都修改 git 跟踪的 `build.gradle`，但修改不提交
- `cap build` 原生支持 `--keystorepath`/`--keystorepass`/`--keystorealias`/`--keystorealiaspass` 参数，内部调用 `./gradlew assembleRelease` 并自动处理签名

**应该用**：
```bash
npx cap build android \
  --androidreleasetype APK \
  --keystorepath ../../keystore/release.jks \
  --keystorepass encv2025 \
  --keystorealias encvrelease \
  --keystorealiaspass encv2025
```

**注意**：`cap build` 会先自动执行 `cap sync`，所以可以省略单独的 `cap sync` 步骤。但 `cap build` 不支持 `minifyEnabled`/`shrinkResources` 配置——这些需要在 `build.gradle` 中预设。

**方案**：将 `minifyEnabled true` + `shrinkResources true` 直接写入 git 中的 `build.gradle` release 块，然后用 `cap build` 替代 sed + `./gradlew`。

---

### 🟡 问题 2：`cap sync` 在 CI 中是多余的

**当前做法**（[android.yml L114-119](file:///workspace/.github/workflows/android.yml#L114-L119)）：
```bash
npx cap sync android
```

**问题**：`cap build` 内部会自动执行 `cap sync`（= `cap copy` + `cap update`），所以如果改用 `cap build`，单独的 `cap sync` 步骤可以移除。

但 Debug 构建仍需要 `cap sync`（因为 Debug 构建不用 `cap build`，而是直接 `./gradlew assembleDebug`）。所以：
- Release 构建：`cap build` 自动 sync，可移除单独的 `cap sync`
- Debug 构建：保留 `cap sync` + `./gradlew assembleDebug`

---

### 🟡 问题 3：`gradle-wrapper.jar` 未纳入 git

**当前状态**：`git status` 显示 `android/gradle/wrapper/gradle-wrapper.jar` 是 untracked。

**问题**：`gradle-wrapper.jar` 是 Gradle Wrapper 的核心二进制文件，没有它 `./gradlew` 无法运行。Capacitor 生成的 `.gitignore` 没有排除它，但 git 没有跟踪它。

**方案**：确保 `gradle-wrapper.jar` 被 git 跟踪。这是 Android 项目的标准做法。

---

### 🟡 问题 4：Trapeze 已不再需要

**当前状态**：
- [trapeze.yaml](file:///workspace/app/encv-mobile/trapeze.yaml) 存在，包含 10 个 Gradle 注入操作
- [package.json](file:///workspace/app/encv-mobile/package.json) 有 `configure:android` 脚本
- `@trapezedev/configure` 在 devDependencies 中

**问题**：既然 `android/` 已纳入 git，Trapeze 的所有修改已经在 `build.gradle` 中了。Trapeze 只在以下场景有用：
1. 从零 `cap add android` 后初始化配置 — 但现在不再需要了
2. Capacitor 大版本升级后重新生成 `android/` — 这是低频操作

**方案**：
- 保留 `trapeze.yaml` 作为文档/参考，但不再在 CI 或日常开发中使用
- 可以从 `devDependencies` 移除 `@trapezedev/configure`（减少 npm install 时间）
- 保留 `configure:android` 脚本作为手动恢复工具

---

### 🟢 问题 5：`cap copy` vs `cap sync` — 可以用更轻量的 `cap copy`

**当前做法**：CI 用 `cap sync`（= `cap copy` + `cap update`）

**分析**：
- `cap copy`：只复制 web 资源和配置文件到原生项目
- `cap update`：更新原生插件依赖（Cordova 插件、Capacitor 插件）
- `cap sync` = `cap copy` + `cap update`

在 CI 中，`package.json` 的依赖在 `npm install` 时已固定，`cap update` 会重新生成 `capacitor-cordova-android-plugins` 目录。但如果 `android/` 已纳入 git，这个目录已经在 git 中了，`cap update` 可能覆盖它。

**方案**：对于 Debug 构建，用 `cap copy` 替代 `cap sync`，避免不必要的插件更新。只在 `package.json` 中添加/删除了 Capacitor 插件时才需要 `cap sync`。

---

### 🟢 问题 6：缺少 `cap ls` 诊断

**当前状态**：CI 没有使用 `cap ls` 来验证插件安装状态。

**方案**：在 CI 的验证步骤中添加 `npx cap ls android`，确认 Capacitor 插件和 Cordova 插件列表正确。

---

### 🟢 问题 7：README 中的 `cap add` 说明需要更新

**当前状态**（[README.md L99](file:///workspace/app/encv-mobile/README.md#L99)）：
```
# 添加 Android 平台（首次）
npx cap add android
```

**问题**：既然 `android/` 已纳入 git，开发者 clone 后不需要 `cap add`，只需要 `cap sync`。

**方案**：更新 README，说明 `android/` 已在 git 中，首次开发只需 `npm install && npm run build && npx cap sync android`。

---

## 实施计划

### Step 1：将 release buildTypes 配置写入 build.gradle

在 `android/app/build.gradle` 的 release 块中预设：
```gradle
buildTypes {
    release {
        minifyEnabled true
        shrinkResources true
        proguardFiles getDefaultProguardFile('proguard-android.txt'), 'proguard-rules.pro'
    }
}
```

### Step 2：CI Release 构建改用 `cap build`

替换 sed 注入 + `./gradlew assembleRelease` 为：
```bash
npx cap build android \
  --androidreleasetype APK \
  --keystorepath ../../keystore/release.jks \
  --keystorepass $KEYSTORE_PASS \
  --keystorealias encvrelease \
  --keystorealiaspass $KEY_ALIAS_PASS
```

Debug 构建保持 `cap copy android` + `./gradlew assembleDebug`。

### Step 3：CI Debug 构建改用 `cap copy`

将 `cap sync` 改为 `cap copy`，避免不必要的插件更新。

### Step 4：确保 gradle-wrapper.jar 被 git 跟踪

`git add android/gradle/wrapper/gradle-wrapper.jar`

### Step 5：添加 `cap ls` 诊断步骤

在 CI 验证步骤中添加 `npx cap ls android`。

### Step 6：更新 README

更新本地开发说明，反映 `android/` 已在 git 中。

### Step 7：清理 Trapeze 依赖（可选）

- 从 `devDependencies` 移除 `@trapezedev/configure`
- 保留 `trapeze.yaml` 作为参考
- 保留 `configure:android` 脚本但标记为手动恢复工具
