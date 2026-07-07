// plugin-simverse/build.gradle.kts
//
// SimVerse ComboLite 插件：嵌入式 WebView + 本地 assets 承载 SimVerse 前端
//
// 架构参考 plugin-openlist / plugin-mpv-player：
//   - IPluginEntryClass.Content() 是 @Composable，必须开启 Compose 编译期插件
//   - 宿主 classloader 提供 combolite-core / core-ktx / compose-ui 等，compileOnly 即可
//   - WebView 加载 file:///android_asset/simverse/ 里的前端构建产物
//   - API 通过 HTTP 调用主 App 的 Go 后端 (http://127.0.0.1:<port>/)
//
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    alias(libs.plugins.combolite.aar2apk)
}

android {
    namespace = "com.encvgo.plugin.simverse"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        minSdk = libs.versions.minSdk.get().toInt()
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            isShrinkResources = false
        }
    }

    sourceSets {
        getByName("main") {
            assets.srcDirs("src/main/assets")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }

    buildFeatures {
        compose = true
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.fromTarget("21"))
    }
}

dependencies {
    compileOnly(libs.combolite.core)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.foundation)
    implementation(libs.compose.foundation.layout)
    implementation(libs.androidx.activity.compose)

    compileOnly("io.insert-koin:koin-core:4.1.0")

    compileOnly("androidx.core:core-ktx")
}

val simverseFrontendDir = layout.projectDirectory.dir("web").asFile

tasks.register("buildSimverseFrontend") {
    description = "Build simverse frontend and copy to plugin assets"
    group = "build"

    val distDir = File(simverseFrontendDir, "dist")
    val assetsDir = layout.projectDirectory.dir("src/main/assets/simverse").asFile

    inputs.dir(simverseFrontendDir)
    outputs.dir(assetsDir)

    doLast {
        val npmLock = File(simverseFrontendDir, "pnpm-lock.yaml")
        val nodeModules = File(simverseFrontendDir, "node_modules")
        if (!nodeModules.exists() && npmLock.exists()) {
            exec {
                workingDir = simverseFrontendDir
                commandLine("pnpm", "install", "--prefer-offline")
            }
        }

        exec {
            workingDir = simverseFrontendDir
            commandLine("pnpm", "build")
        }

        assetsDir.deleteRecursively()
        assetsDir.mkdirs()

        distDir.copyRecursively(assetsDir, overwrite = true)
        logger.lifecycle("SimVerse frontend copied to ${assetsDir.absolutePath}")
    }
}

tasks.whenTaskAdded {
    if (name == "mergeDebugAssets" || name == "mergeReleaseAssets") {
        dependsOn("buildSimverseFrontend")
    }
}
