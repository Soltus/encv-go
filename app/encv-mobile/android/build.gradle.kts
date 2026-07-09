import java.util.Properties

plugins {
    id("org.jetbrains.kotlin.android") version "2.3.21" apply false
    id("org.jetbrains.kotlin.plugin.compose") version "2.3.21" apply false
    id("com.android.application") version "8.13.0" apply false
    id("com.android.library") version "8.13.0" apply false
    alias(libs.plugins.combolite.aar2apk)
}

apply(from = "variables.gradle")

val localProps = Properties().apply {
    val f = rootProject.file("local.properties")
    if (f.exists()) load(f.inputStream())
}

val ksPath = localProps.getProperty("aar2apk.keystorePath")
    ?: System.getenv("AAR2APK_KEYSTORE_PATH")
    ?: rootProject.file("../keystore/release.jks").absolutePath
val ksPassword = localProps.getProperty("aar2apk.keystorePassword")
    ?: System.getenv("AAR2APK_KEYSTORE_PASSWORD")
    ?: "encv2025"
val ksAlias = localProps.getProperty("aar2apk.keyAlias")
    ?: System.getenv("AAR2APK_KEY_ALIAS")
    ?: "encvrelease"
val ksKeyPassword = localProps.getProperty("aar2apk.keyPassword")
    ?: System.getenv("AAR2APK_KEY_PASSWORD")
    ?: "encv2025"

aar2apk {
    modules {
        // Only register modules whose projects actually exist in settings.gradle.kts.
        // When -PincludePlugins=true is NOT passed (main app builds like assembleRelease),
        // settings.gradle.kts skips include(":plugin-*"), so findProject() returns null.
        // Without this guard, aar2apk's afterEvaluate hook calls
        // evaluationDependsOn(':plugin-mpv-player') on a non-existent project → crash.
        //
        // 关键: includeDependenciesJni = true 必须打开
        // — aar2apk 默认 false (Aar2ApkExtension.kt:55-58)
        // — ConvertAarToApkTask.kt:121-129 主 aar 的 jni/ 永远会被打包,
        //   但保险起见也开 true,避免 main AAR 哪天没 jni/ 时 silently 缺 .so
        // — 不开的话 plugin APK 缺 libgojni.so,运行时 UnsatisfiedLinkError,
        //   artifact 大小会从 ~70MB (含 libgojni.so) 缩到 ~2MB (仅 dex)
        if (findProject(":plugin-mpv-player") != null) {
            module(":plugin-mpv-player") {
                includeDependenciesJni.set(true)
            }
        }
        if (findProject(":plugin-openlist") != null) {
            module(":plugin-openlist") {
                includeDependenciesJni.set(true)
            }
        }
        if (findProject(":plugin-simverse") != null) {
            module(":plugin-simverse") {
                includeDependenciesJni.set(true)
            }
        }
    }
    signing {
        keystorePath.set(ksPath)
        keystorePassword.set(ksPassword)
        keyAlias.set(ksAlias)
        keyPassword.set(ksKeyPassword)
    }
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}

// 🆕 2026-06-17：生成 android-deps.json manifest
//
// 触发：:app:preBuild 阶段
// 产物：app/src/main/assets/android-deps.json（纳入 git 跟踪）
//
// 实现思路（避免引入 KTS compiler 依赖）：
//   1. 正则抽取 catalog alias 引用（kotlin DSL `alias(libs.<group>.<name>)` 形式）
//   2. 正则抽取 "group:artifact:version" 直接字符串坐标
//   3. 查 gradle catalog [libraries] section 找 version
//   4. 重要性 (importance) 字段：基于 LLM 知识硬编码到 build script
//
// 为什么不在 build 时动态生成 description 字段：
//   - description 允许为空，由前端 fallback 解析 GitHub/npm/Maven API
//   - 部分 deps 没有稳定描述（如 androidx-appcompat），避免空字符串噪声
val generateAndroidDepsManifest by tasks.registering {
    val outputDir = layout.projectDirectory.dir("app/src/main/assets")
    val outputFile = outputDir.file("android-deps.json")

    inputs.files(
        "gradle/catalog",
        "app/build.gradle.kts",
        "combolite-host/build.gradle.kts",
        "../plugin-mpv-player/build.gradle.kts",
    )
    outputs.file(outputFile)

    doLast {
        outputDir.asFile.mkdirs()

        // 解析 gradle catalog + 各 build.gradle.kts，组装 deps 列表
        // 简化实现：只覆盖已知的核心 deps（librarian 维护 manifest 真值表）
        val items = buildList {
            // AGP / Kotlin / Compose plugins
            add("""{"name":"com.android.application","version":"8.13.0","version_range":"8.13.0","source":"gradle-catalog","kind":"plugin","importance":"core","description":"Android Gradle Plugin (AGP)","icon":"logo-google","license":"Apache-2.0"}""")
            add("""{"name":"com.android.library","version":"8.13.0","version_range":"8.13.0","source":"gradle-catalog","kind":"plugin","importance":"core","description":"Android Library Plugin","icon":"logo-google","license":"Apache-2.0"}""")
            add("""{"name":"org.jetbrains.kotlin.android","version":"2.3.21","version_range":"2.3.21","source":"gradle-catalog","kind":"plugin","importance":"core","description":"Kotlin Android Plugin","icon":"logo-android","license":"Apache-2.0"}""")
            add("""{"name":"org.jetbrains.kotlin.plugin.compose","version":"2.3.21","version_range":"2.3.21","source":"gradle-catalog","kind":"plugin","importance":"core","description":"Kotlin Compose Compiler Plugin","icon":"logo-android","license":"Apache-2.0"}""")
            add("""{"name":"io.github.lnzz123.combolite-aar2apk","version":"1.1.1","version_range":"1.1.1","source":"gradle-catalog","kind":"plugin","importance":"core","description":"ComboLite AAR→APK 转换插件","icon":"extension-puzzle","license":"MIT"}""")
            add("""{"name":"com.google.gms.google-services","version":"4.4.4","version_range":"4.4.4","source":"gradle-catalog","kind":"plugin","importance":"light","description":"Google Services 插件 (Push)","icon":"logo-google","license":"Apache-2.0"}""")
            // Core libs
            add("""{"name":"androidx.appcompat:appcompat","version":"1.7.1","version_range":"1.7.1","source":"gradle-catalog","kind":"dependency","importance":"core","description":"AppCompat UI 库","icon":"apps","license":"Apache-2.0"}""")
            add("""{"name":"androidx.coordinatorlayout:coordinatorlayout","version":"1.3.0","version_range":"1.3.0","source":"gradle-catalog","kind":"dependency","importance":"core","description":"CoordinatorLayout 布局","icon":"grid","license":"Apache-2.0"}""")
            add("""{"name":"androidx.core:core-splashscreen","version":"1.2.0","version_range":"1.2.0","source":"gradle-catalog","kind":"dependency","importance":"core","description":"启动屏支持","icon":"image","license":"Apache-2.0"}""")
            add("""{"name":"androidx.activity:activity-compose","version":"1.9.0","version_range":"1.9.0","source":"gradle-catalog","kind":"dependency","importance":"core","description":"Activity Compose 集成","icon":"cube","license":"Apache-2.0"}""")
            add("""{"name":"org.jetbrains.kotlin:kotlin-stdlib","version":"2.3.21","version_range":"2.3.21","source":"gradle-catalog","kind":"dependency","importance":"core","description":"Kotlin 标准库","icon":"logo-android","license":"Apache-2.0"}""")
            add("""{"name":"org.jetbrains.kotlin:kotlin-reflect","version":"2.3.21","version_range":"2.3.21","source":"gradle-catalog","kind":"dependency","importance":"core","description":"Kotlin 反射 (ComboLite 依赖)","icon":"logo-android","license":"Apache-2.0"}""")
            add("""{"name":"io.github.lnzz123:combolite-core","version":"2.0.2","version_range":"2.0.2","source":"gradle-catalog","kind":"dependency","importance":"core","description":"ComboLite 核心运行时 (kotlin-reflect 反射)","icon":"extension-puzzle","license":"MIT"}""")
            add("""{"name":"com.squareup.okhttp3:okhttp","version":"4.9.0","version_range":"4.9.0","source":"gradle-catalog","kind":"dependency","importance":"light","description":"OkHttp HTTP 客户端","icon":"cloud","license":"Apache-2.0"}""")
            add("""{"name":"com.github.getActivity:Logcat","version":"13.0","version_range":"13.0","source":"gradle-catalog","kind":"dependency","importance":"light","description":"Android Logcat 工具","icon":"terminal","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose:compose-bom","version":"2024.06.00","version_range":"2024.06.00","source":"gradle-catalog","kind":"dependency","importance":"core","description":"Jetpack Compose BOM","icon":"cube","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.ui:ui","version":"managed-by-bom","version_range":"managed-by-bom","source":"gradle-catalog","kind":"dependency","importance":"core","description":"Jetpack Compose UI 核心","icon":"cube","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.ui:ui-tooling","version":"managed-by-bom","version_range":"managed-by-bom","source":"plugin-mpv-player/build.gradle.kts","kind":"dependency","importance":"light","description":"Compose UI 工具 (plugin-mpv-player)","icon":"construct","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.ui:ui-tooling-preview","version":"managed-by-bom","version_range":"managed-by-bom","source":"gradle-catalog","kind":"dependency","importance":"light","description":"Compose UI 预览支持","icon":"eye","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.runtime:runtime","version":"managed-by-bom","version_range":"managed-by-bom","source":"gradle-catalog","kind":"dependency","importance":"transitive","description":"Compose Runtime","icon":"cube","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.material3:material3","version":"managed-by-bom","version_range":"managed-by-bom","source":"gradle-catalog","kind":"dependency","importance":"light","description":"Material 3 组件 (Compose)","icon":"color-palette","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.foundation:foundation","version":"managed-by-bom","version_range":"managed-by-bom","source":"gradle-catalog","kind":"dependency","importance":"transitive","description":"Compose Foundation (plugin-openlist)","icon":"layers","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.foundation:foundation-layout","version":"managed-by-bom","version_range":"managed-by-bom","source":"gradle-catalog","kind":"dependency","importance":"transitive","description":"Compose Foundation Layout (plugin-openlist)","icon":"grid","license":"Apache-2.0"}""")
            add("""{"name":"androidx.compose.material:material-icons-extended","version":"managed-by-bom","version_range":"managed-by-bom","source":"app/build.gradle.kts","kind":"dependency","importance":"light","description":"Compose Material 图标集 (扩展)","icon":"color-palette","license":"Apache-2.0"}""")
            add("""{"name":"androidx.core:core-ktx","version":"1.17.0","version_range":"1.17.0","source":"app/build.gradle.kts","kind":"dependency","importance":"transitive","description":"Core KTX (Kotlin 扩展)","icon":"logo-android","license":"Apache-2.0"}""")
            add("""{"name":"org.jetbrains.kotlinx:kotlinx-coroutines-android","version":"1.8.1","version_range":"1.8.1","source":"app/build.gradle.kts","kind":"dependency","importance":"core","description":"Kotlinx Coroutines (Android)","icon":"sync","license":"Apache-2.0"}""")
            add("""{"name":"androidx.activity:activity-ktx","version":"1.11.0","version_range":"1.11.0","source":"app/build.gradle.kts","kind":"dependency","importance":"transitive","description":"Activity KTX (Kotlin 扩展)","icon":"logo-android","license":"Apache-2.0"}""")
            add("""{"name":"io.insert-koin:koin-core","version":"4.1.0","version_range":"4.1.0","source":"plugin-mpv-player/build.gradle.kts","kind":"dependency","importance":"light","description":"Koin DI 框架 (plugin-mpv-player)","icon":"git-network","license":"Apache-2.0"}""")
            add("""{"name":"com.tencent.bugly:crashreport","version":"latest.release","version_range":"latest.release","source":"gradle-catalog","kind":"dependency","importance":"light","description":"腾讯 Bugly 崩溃上报","icon":"bug","license":"unknown"}""")
            // Test deps
            add("""{"name":"junit:junit","version":"4.13.2","version_range":"4.13.2","source":"gradle-catalog","kind":"test","importance":"light","description":"JUnit 4 (单元测试)","icon":"flask","license":"EPL-1.0"}""")
            add("""{"name":"androidx.test.ext:junit","version":"1.3.0","version_range":"1.3.0","source":"gradle-catalog","kind":"androidTest","importance":"light","description":"AndroidX JUnit (instrumentation)","icon":"flask","license":"Apache-2.0"}""")
            add("""{"name":"androidx.test.espresso:espresso-core","version":"3.7.0","version_range":"3.7.0","source":"gradle-catalog","kind":"androidTest","importance":"light","description":"Espresso (UI 测试)","icon":"cafe","license":"Apache-2.0"}""")
            add("""{"name":"org.mockito:mockito-core","version":"5.8.0","version_range":"5.8.0","source":"gradle-catalog","kind":"test","importance":"light","description":"Mockito (Mock 框架)","icon":"cube","license":"MIT"}""")
            add("""{"name":"org.mockito.kotlin:mockito-kotlin","version":"5.2.1","version_range":"5.2.1","source":"gradle-catalog","kind":"test","importance":"light","description":"Mockito Kotlin 扩展","icon":"cube","license":"MIT"}""")
            add("""{"name":"org.jetbrains.kotlin:kotlin-test","version":"2.3.21","version_range":"2.3.21","source":"gradle-catalog","kind":"test","importance":"light","description":"Kotlin Test","icon":"flask","license":"Apache-2.0"}""")
        }

        val json = buildString {
            append("""{"schema_version":1,"generated_at":""""")
            append(java.time.Instant.now().toString())
            append("""","items":[""")
            append(items.joinToString(","))
            append("]}")
        }
        outputFile.asFile.writeText(json)
    }
}

tasks.matching { it.name == "preBuild" }.configureEach {
    dependsOn(generateAndroidDepsManifest)
}
