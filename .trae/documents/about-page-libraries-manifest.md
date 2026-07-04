# About 页库展示重构 — 数据源 manifest + 状态/重要性双重标记

> 状态:Phase 3 待批准 · Phase 4 待用户确认后执行
> 范围:`app/encv-mobile` 前端 + `internal/server` 后端 + `app/encv-mobile/android` Gradle
> 数据源:构建时生成的静态 manifest,纳入 git 跟踪,基于实际使用情况判定

---

## 1. Summary

把 `AboutDetail.vue` 的 3 个 section(原生引擎 / 后端库 / 前端库)重写,使其:
1. **新增 Android 库** section(Compose / Kotlin / AGP / ComboLite / Koin 等)
2. **每个库标注来源**(package.json / libs.versions.toml / build.gradle.kts / go.mod)
3. **构建时生成静态 manifest** 纳入 git 跟踪(差异化检测,容易审计)
4. **运行时判定依赖状态**:active(当前 source 能解析)/ broken(找不到,异常) / historical(manifest 中有,无 source 引用)
5. **标记依赖重要性**:core / light / transitive
6. **描述字段允许为空**:为空时前端 fallback 用 GitHub / npm / Maven Central API 解析;全部失败用占位符
7. **彻底消除硬编码版本号**:所有数据来自 manifest,不再手写

---

## 2. Current State Analysis

### 2.1 `AboutDetail.vue` 当前结构(L1-271)

| Section | 内容 | 问题 |
|---------|------|------|
| 原生引擎 | FFmpeg (来自 `/api/build-info` 动态) | OK,保留 |
| 后端库 | 8 个 Go 库,**硬编码**版本号 (`v1.12.0` `v1.10.2` 等) | 改用 manifest |
| 前端库 | 5 个 npm 库,**硬编码**版本号 | 改用 manifest |
| 🆕 Android 库 | **不存在** | 全新 section |

### 2.2 数据源清单

| 来源文件 | 涵盖的库 | 当前是否被前端读取 |
|----------|----------|---------------------|
| `app/encv-mobile/package.json` | Vue/Ionic/Capacitor/Artplayer/vconsole + 11 个 deps | ❌ 硬编码 5 个 |
| `app/encv-mobile/android/gradle/libs.versions.toml` | 27 个 Android libs (agp/kotlin/compose/coroutines/okhttp/bugly/...) | ❌ 完全没显示 |
| `app/encv-mobile/android/app/build.gradle.kts` | 13 个直接 deps (core-ktx/coroutines/koin/material-icons-extended) | ❌ 完全没显示 |
| `app/encv-mobile/android/combolite-host/build.gradle.kts` | combolite-core/kotlin-stdlib/kotlin-reflect/coroutines | ❌ 完全没显示 |
| `app/encv-mobile/plugin-mpv-player/build.gradle.kts` | compose-bom/compose-ui/koin 4.1.0 等 | ❌ 完全没显示 |
| `go.mod` | gin/cobra/go-mp4/go-exif/fsnotify/websocket/humanize/... | ❌ 硬编码 8 个 |
| `runtime/debug.ReadBuildInfo()` (运行时) | Go deps 含 Path/Version/Sum/Replace | ❌ 完全没读 |
| `/api/build-info` | FFmpeg 编译信息 (build-info.json) | ✅ 已用 |

### 2.3 i18n key 位置

- L192-228 (中文) + L454-490 (英文) 都在 `app/encv-mobile/src/i18n/common.ts`
- 现有 `about.*` key 会被复用 + 扩展

### 2.4 Android 端无 native bridge API 暴露 deps

`/workspace/app/encv-mobile/android/app/src/main/assets/capacitor.plugins.json` 只有 3 个 Capacitor 官方插件,无自研 native bridge 暴露 deps。

---

## 3. Proposed Changes

### 3.1 🆕 新建 `app/encv-mobile/src/generated/frontend-deps.json`

**生成方式:** vite plugin 在 `dev` 启动和 `build` 阶段读 `package.json`,转成下面的 schema,写到 `src/generated/frontend-deps.json`。**纳入 git 跟踪**。

**schema:**
```json
{
  "generated_at": "2026-06-17T10:30:00Z",
  "source_file": "package.json",
  "schema_version": 1,
  "items": [
    {
      "name": "vue",
      "version": "3.5.34",
      "version_range": "^3.5.34",
      "kind": "dependency",
      "importance": "core",
      "description": "前端框架"
    },
    {
      "name": "vconsole",
      "version": "3.15.1",
      "version_range": "^3.15.1",
      "kind": "dependency",
      "importance": "light",
      "description": "移动端调试面板"
    }
  ]
}
```

**importance 标注规则(基于 LLM 知识 + 项目代码扫描):**
- `core`: Vue / Ionic / vue-router / @capacitor/core / @capacitor/android (必备框架)
- `light`: vconsole / markstream-vue / @tdesign-vue-next/chat / @tanstack/vue-virtual / vue-virtual-scroller / @ionic/vue-router / @capacitor/* (除 core 外)/ @ajuarezso/capacitor-high-refresh-rate
- `transitive`: 实际 build 中由 npm 自动 hoist 的,本项目**不直接**import(此项前端 npm 不暴露,默认空)

**description 字段规则:**
- 默认空字符串 `""` (允许为空,符合用户原话)
- 显式标注的库带描述(从项目 LLM 知识补)
- description 为空时,前端 fallback 走 GitHub / npm API (见 3.5)

### 3.2 🆕 新建 `app/encv-mobile/android/app/src/main/assets/android-deps.json`

**生成方式:** Gradle task `generateAndroidDepsManifest` 在 `:app:preBuild` 阶段执行,读 `libs.versions.toml` + `app/build.gradle.kts` + `combolite-host/build.gradle.kts` + `plugin-mpv-player/build.gradle.kts`,转成 JSON,写到 `android/app/src/main/assets/android-deps.json`。**纳入 git 跟踪**。

**schema:**
```json
{
  "generated_at": "2026-06-17T10:30:00Z",
  "schema_version": 1,
  "items": [
    {
      "name": "androidx.compose.ui:ui",
      "version": "managed-by-bom",
      "version_range": "managed-by-bom",
      "source": "libs.versions.toml",
      "kind": "dependency",
      "importance": "core",
      "description": "Jetpack Compose UI 核心"
    },
    {
      "name": "io.github.lnzz123:combolite-core",
      "version": "2.0.2",
      "version_range": "2.0.2",
      "source": "libs.versions.toml",
      "kind": "dependency",
      "importance": "core",
      "description": "ComboLite 核心运行时 (kotlin-reflect 反射)"
    },
    {
      "name": "androidx.core:core-ktx",
      "version": "1.17.0",
      "version_range": "1.17.0",
      "source": "app/build.gradle.kts",
      "kind": "dependency",
      "importance": "transitive",
      "description": ""
    }
  ]
}
```

**importance 标注规则:**
- `core`: AGP / Kotlin / Compose BOM / ComboLite / coroutines (必备)
- `light`: 业务可选如 logcat / bugly (可降级)
- `transitive`: 不直接 import 的(如 core-ktx 在 build.gradle.kts 里写,本项目代码未直接 `import androidx.core.ktx`,标记为 transitive)

### 3.3 🆕 Go 后端 `/api/libraries` 端点

**文件:** `internal/server/libraries_api.go`

**实现:**
```go
//go:build android
// 略

func (s *Server) handleLibrariesGin(c *gin.Context) {
    items := []LibraryItem{}
    
    // 1. Go 自身 + 标准库
    goVer := runtime.Version()
    items = append(items, LibraryItem{
        Name: "Go", Version: goVer,
        Source: "runtime.Version()", Importance: "core",
        Description: "后端运行时",
    })
    
    // 2. runtime/debug.ReadBuildInfo 拿 deps
    bi, ok := debug.ReadBuildInfo()
    if ok {
        for _, dep := range bi.Deps {
            items = append(items, LibraryItem{
                Name: dep.Path, Version: dep.Version,
                Source: "go.mod", Importance: classifyGoDep(dep.Path),
                Description: "",  // 默认空
            })
        }
    }
    
    // 3. Android deps via query param (从 native bridge 传过来)
    if raw := c.Query("android_manifest"); raw != "" {
        var androidItems []LibraryItem
        if err := json.Unmarshal([]byte(raw), &androidItems); err == nil {
            items = append(items, androidItems...)
        }
    }
    
    c.JSON(200, gin.H{"items": items})
}
```

**关键决策:**
- Android deps 必须从 native 端 push 过来(因为 Go 后端没有直接读 Android assets 的能力)
- Capacitor native bridge 新增 `getAndroidDeps()` 方法读取 assets
- 前端 fetch 时把 native 返回的 JSON 拼到 query string `?android_manifest={json}`

**Go deps importance 分类规则:**
```go
func classifyGoDep(path string) string {
    coreLibs := map[string]bool{
        "github.com/gin-gonic/gin": true,           // HTTP 框架 - 核心
        "github.com/spf13/cobra": true,            // CLI - 核心
        "github.com/gorilla/websocket": true,       // WS - 核心
        "github.com/fsnotify/fsnotify": true,       // FS watch - 核心
    }
    if coreLibs[path] { return "core" }
    return "light"
}
```

### 3.4 🆕 Capacitor native bridge 端点 `getAndroidDeps()`

**文件:** `app/encv-mobile/src/plugins/GoProcess.ts` 扩展,`app/encv-mobile/android/app/src/main/java/.../GoPlugin.java` (新 native handler)

**TypeScript 侧:**
```ts
// plugins/GoProcess.ts 新增
export async function getAndroidDeps(): Promise<AndroidDepsManifest> {
  return await callNative<AndroidDepsManifest>('getAndroidDeps')
}
```

**Java 侧 (新建):** 在现有 `GoPlugin` 旁加 case:
```java
case "getAndroidDeps":
    try {
        InputStream is = getContext().getAssets().open("android-deps.json");
        byte[] bytes = new byte[is.available()];
        is.read(bytes);
        is.close();
        call.resolve(new JSObject(new String(bytes, "UTF-8")));
    } catch (IOException e) {
        call.reject("failed to read android-deps.json: " + e.getMessage());
    }
    break;
```

**Web mock (`web.ts`):** 返回 `null`(dev/web 模式没有 Android deps,前端 fallback)

### 3.5 🆕 前端 `useLibraries()` composable

**文件:** `app/encv-mobile/src/composables/useLibraries.ts`

**职责:**
1. fetch `/api/libraries`(Go deps,后端可选择性合并 android deps)
2. import `frontend-deps.json`(静态,build 阶段生成)
3. await `getAndroidDeps()`(native only,web null)
4. 合并去重(按 `name` 字段,后到的覆盖前面的 source 字段)
5. 计算 `status` 字段:
   - `active`: items 数组里有 + source 字段非空
   - `broken`: items 里有但 `version` 为空或 `source` 为空
   - `historical`: items 里有但任何 source 都引用不到
6. 计算 `descriptionFallback`:
   - 如果 `description` 为空,尝试顺序解析:npm → GitHub → Maven Central
   - 全部失败 → `"无描述"` 占位
   - 结果缓存到 `localStorage["encv_lib_desc_cache_v1"]`,TTL 7 天

**API:**
```ts
interface LibraryItem {
  name: string
  version: string
  versionRange?: string
  source: 'package.json' | 'libs.versions.toml' | 'build.gradle.kts' | 'go.mod' | 'runtime.Version()' | 'unknown'
  kind: 'dependency' | 'devDependency' | 'transitive'
  importance: 'core' | 'light' | 'transitive'
  status: 'active' | 'broken' | 'historical'
  description: string
  descriptionFallback?: string  // 解析后的 fallback
  descriptionStatus: 'explicit' | 'fetched' | 'placeholder'
}
```

### 3.6 🆕 `LibraryRow.vue` 组件(可复用)

**文件:** `app/encv-mobile/src/components/LibraryRow.vue`

**Props:**
- `item: LibraryItem`

**渲染:**
- lib name + version
- `lib-source-badge` (来源)
- `lib-status-badge` (active 绿 / broken 红 / historical 灰)
- `lib-importance-badge` (core 蓝 / light 紫 / transitive 灰)
- description (显式 / fallback / 占位符)

**设计原则:** 跟当前 `lib-title-row` 风格保持一致,不改视觉。

### 3.7 🆕 Gradle task `generateAndroidDepsManifest`

**文件:** `app/encv-mobile/android/build.gradle.kts` 末尾追加

```kotlin
// 🆕 2026-06-17: 生成 android-deps.json (供 About 页读取)
// 触发: :app:preBuild + :combolite-host:preBuild
// 产物: android/app/src/main/assets/android-deps.json
// 纳入 git 跟踪 — 团队 commit 时可 diff 审查
val generateAndroidDepsManifest by tasks.registering {
    val outputDir = layout.projectDirectory.dir("app/src/main/assets")
    val outputFile = outputDir.file("android-deps.json")
    
    inputs.files(
        file("gradle/libs.versions.toml"),
        file("app/build.gradle.kts"),
        file("combolite-host/build.gradle.kts"),
        file("../plugin-mpv-player/build.gradle.kts")
    )
    outputs.file(outputFile)
    doLast {
        outputDir.asFile.mkdirs()
        val items = parseAndroidDeps()  // 简易 KTS parser
        outputFile.asFile.writeText(
            Json { prettyPrint = true }.encodeToString(items)
        )
    }
}

tasks.named("preBuild") { dependsOn(generateAndroidDepsManifest) }
```

**parser 简化方案:**
- 用正则 `implementation\(libs\.(\w+(?:\.\w+)*)\)` 抽取 `libs.X` 引用
- 用正则 `"([^"]+):([^"]+)"` 抽取直接字符串坐标
- 查 `libs.versions.toml` 的 `[libraries]` section 找对应 version
- 不解析完整的 KTS AST(避免引入 KTS compiler 依赖)

### 3.8 🆕 vite plugin `generateFrontendDepsManifest`

**文件:** `app/encv-mobile/vite-plugins/frontend-deps-manifest.ts`

**实现:**
- 在 `configResolved` 钩子中:
  1. 读 `package.json`
  2. 对每个 dep 应用 importance 分类规则(3.1)
  3. 写到 `src/generated/frontend-deps.json`
- 监听 `package.json` change 重新生成(`buildStart` 钩子)
- 在 vite.config.ts `plugins: [...]` 数组中加,`enforce: 'pre'`

**`src/generated/frontend-deps.json` 本身**:
- `.gitignore` 写入忽略的话会让 git 历史变乱
- **不**忽略 — 纳入 git 跟踪(用户原话"清单纳入git跟踪")

### 3.9 ✏️ 修改 `AboutDetail.vue`

**重写结构(全部用 `LibraryRow` 组件):**

```vue
<ion-list>
  <ion-list-header>{{ t('about.androidLibs') }}</ion-list-header>
  <LibraryRow v-for="item in androidItems" :key="item.name" :item="item" />
</ion-list>

<ion-list>
  <ion-list-header>{{ t('about.frontendLibs') }}</ion-list-header>
  <LibraryRow v-for="item in frontendItems" :key="item.name" :item="item" />
</ion-list>

<ion-list>
  <ion-list-header>{{ t('about.backendLibs') }}</ion-list-header>
  <LibraryRow v-for="item in backendItems" :key="item.name" :item="item" />
</ion-list>
```

**不再有硬编码** — 全部数据来自 `useLibraries()`。

**保留:** 原生引擎 section (FFmpeg),`engine.runtimeStatus` 走 `/api/build-info`。

### 3.10 ✏️ i18n 扩展 (`common.ts` 中英两段)

**新增 key:**

| key | 中文 | 英文 |
|-----|------|------|
| `about.androidLibs` | Android 库 | Android Libraries |
| `about.libsSource` | 来源 | Source |
| `about.libSource.packageJson` | package.json | package.json |
| `about.libSource.libsVersionsToml` | libs.versions.toml | libs.versions.toml |
| `about.libSource.buildGradleKts` | build.gradle.kts | build.gradle.kts |
| `about.libSource.goMod` | go.mod | go.mod |
| `about.libSource.runtimeVersion` | runtime.Version() | runtime.Version() |
| `about.libStatus.active` | 正常 | Active |
| `about.libStatus.broken` | 异常 | Broken |
| `about.libStatus.historical` | 历史 | Historical |
| `about.libImportance.core` | 核心 | Core |
| `about.libImportance.light` | 轻度 | Light |
| `about.libImportance.transitive` | 传递 | Transitive |
| `about.libDescriptionPlaceholder` | 无描述 | No description |
| `about.libFetchingDescription` | 解析中… | Fetching… |

**保留 key:** `about.ffmpegDesc` `about.goRuntimeDesc` `about.backendLibs` `about.frontendLibs` `about.nativeEngine` `about.failedToLoad`(继续用)

---

## 4. Assumptions & Decisions

| 决策 | 理由 |
|------|------|
| manifest 纳入 git 跟踪 | 用户原话"清单纳入git跟踪";便于审计,diff 清晰 |
| 构建时生成 + 提交 + 运行时再读 | 不依赖 build 阶段 hook 链,build 系统断网也能跑 |
| description 允许为空,运行时 fallback | 用户原话"允许为空,为空时尝试使用github地址或者库地址" |
| historical 仅依据 source 找不到 | 用户原话"和库本身维护状态无关" |
| transitive 分类基于"直接 import 是否存在" | 项目代码扫描作为辅助;build.gradle.kts 写但代码未 import 标 transitive |
| Android deps 通过 native bridge 推给 Go | Go 后端无直接读 Android assets 能力;前端中转最简 |
| description fallback 缓存 localStorage 7 天 | 避免每次 About 页打开都请求 GitHub/npm |
| 保留 Go `runtime/debug.ReadBuildInfo()` | `-buildvcs=true` 默认开,Module 信息含 Path/Version/Sum/Replace |
| Gradle KTS 解析用正则,不用 KTS compiler | 避免引入新依赖;正则已足够抽取 `libs.X` 和 `"x:y"` 形式 |

---

## 5. Verification Steps

### 5.1 manifest 生成

```bash
# Frontend
cd /workspace/app/encv-mobile && cat src/generated/frontend-deps.json | jq '.items | length'
# 期望: 18 (12 deps + 6 devDeps)

# Android
cd /workspace/app/encv-mobile/android && ls app/src/main/assets/android-deps.json
# Gradle 跑 :app:preBuild 自动生成
# 期望: 文件存在,items 数 ~30
```

### 5.2 Go build

```bash
cd /workspace && bash scripts/test-go.sh ./internal/server
# 期望: exit 0,libraries_api.go 编译通过
```

### 5.3 前端 type-check + build

```bash
cd /workspace/app/encv-mobile && pnpm run build
# 期望: 0 error,LibraryRow.vue + useLibraries.ts 编译通过
```

### 5.4 runtime 验证

1. 启动 `pm2 start /workspace/ecosystem.config.cjs`
2. 浏览器打开 About 页 (`/tabs/settings/about`)
3. 验证 4 个 section 都显示:
   - 原生引擎 (FFmpeg)
   - Android 库 (~30 items,status badge + importance badge)
   - 前端库 (18 items,部分 description 显示 fallback)
   - 后端库 (10 items,Go + go.mod deps)
4. 故意把 `src/generated/frontend-deps.json` 里 `vue` 删掉:
   - reload → 前端库 section 出现 "historical" badge
5. 故意把 `android-deps.json` 里 `combolite-core` 删掉:
   - reload → Android 库 section 出现 "historical" badge
6. 故意把 `android-deps.json` 里 `xxx-fake:1.0.0` 加进去:
   - reload → Android 库 section 出现 "broken" badge (manifest 里有但 source 找不到)

### 5.5 description fallback 验证

1. 找一个 description 为空的 lib(比如 vconsole)
2. 第一次访问 → "解析中…",然后从 npmjs.com API 拿到描述
3. 第二次访问 → 立即显示(从 localStorage 缓存)
4. 断网 → 显示 "无描述" 占位符

### 5.6 i18n 验证

中文 / 英文切换后,badge 文案、section 标题、placeholder 都跟随切换。

### 5.7 沙箱预览

```bash
bash /workspace/scripts/previews.sh start
# OpenPreview 链接能打开 About 页,4 个 section 都显示
```

### 5.8 单元测试

```bash
cd /workspace/app/encv-mobile && pnpm test:run -- useLibraries
# 期望: status / importance / fallback 计算函数测试通过
```

---

## 6. File Manifest

| 操作 | 路径 | 行数估计 |
|------|------|---------|
| 🆕 新建 | `app/encv-mobile/src/generated/frontend-deps.json` | ~150 |
| 🆕 新建 | `app/encv-mobile/android/app/src/main/assets/android-deps.json` | ~250 |
| 🆕 新建 | `app/encv-mobile/vite-plugins/frontend-deps-manifest.ts` | ~80 |
| 🆕 新建 | `internal/server/libraries_api.go` | ~120 |
| 🆕 新建 | `app/encv-mobile/src/composables/useLibraries.ts` | ~180 |
| 🆕 新建 | `app/encv-mobile/src/components/LibraryRow.vue` | ~140 |
| ✏️ 修改 | `app/encv-mobile/vite.config.ts` | +3 行 (新 plugin) |
| ✏️ 修改 | `app/encv-mobile/android/build.gradle.kts` | +60 行 (新 task) |
| ✏️ 修改 | `app/encv-mobile/android/app/src/main/java/.../GoPlugin.java` (或 Kotlin) | +25 行 |
| ✏️ 修改 | `app/encv-mobile/src/plugins/GoProcess.ts` | +15 行 (新 native call) |
| ✏️ 修改 | `app/encv-mobile/src/plugins/web.ts` | +5 行 (mock) |
| ✏️ 修改 | `app/encv-mobile/src/views/AboutDetail.vue` | 重写,~200 行 |
| ✏️ 修改 | `app/encv-mobile/src/i18n/common.ts` | +30 keys ×2 (中英) |
| ✏️ 修改 | `internal/server/server.go` | +2 行 (新 route) |

总改动:7 新建 + 7 修改,约 +1200 行。

---

## 7. Out of Scope (后续 TODO)

- iOS 库展示(Capacitor 也支持 iOS,本任务只覆盖 Android)
- 描述 fallback 离线打包(目前每次访问都查 npm/GitHub,后续可预 build 阶段批量解析)
- 依赖安全审计(npm audit / Gradle dependencyCheck)
- 自动 PR 在 dep 升级时改 manifest(目前手动改)
- historical 库的"清理建议"按钮(本任务只标记,不动手)
- Capacitor 官方 plugins 的描述(@capacitor/share 等目前 description 都空)
