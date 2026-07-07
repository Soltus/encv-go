plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    alias(libs.plugins.combolite.aar2apk)
}

android {
    namespace = "com.encvgo.plugin.mpv"
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

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }

    sourceSets {
        getByName("main") {
            assets.srcDirs("src/main/assets")
        }
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
    implementation("androidx.compose.ui:ui-tooling")
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.appcompat)
    compileOnly("androidx.compose.material:material-icons-extended")
    compileOnly("androidx.core:core-ktx")
    compileOnly("androidx.activity:activity-ktx")
    compileOnly("org.jetbrains.kotlinx:kotlinx-coroutines-android")
    compileOnly("io.insert-koin:koin-core:4.1.0")
}

val mpvFrontendDir = layout.projectDirectory.dir("web").asFile

tasks.register("buildMpvFrontend") {
    description = "Build mpv frontend and copy to plugin assets"
    group = "build"

    val distDir = File(mpvFrontendDir, "dist")
    val assetsDir = layout.projectDirectory.dir("src/main/assets/mpv").asFile

    inputs.dir(mpvFrontendDir)
    outputs.dir(assetsDir)

    doLast {
        val npmLock = File(mpvFrontendDir, "pnpm-lock.yaml")
        val nodeModules = File(mpvFrontendDir, "node_modules")
        if (!nodeModules.exists() && npmLock.exists()) {
            exec {
                workingDir = mpvFrontendDir
                commandLine("pnpm", "install", "--prefer-offline")
            }
        }

        exec {
            workingDir = mpvFrontendDir
            commandLine("pnpm", "build")
        }

        assetsDir.deleteRecursively()
        assetsDir.mkdirs()

        distDir.copyRecursively(assetsDir, overwrite = true)
        logger.lifecycle("MPV frontend copied to ${assetsDir.absolutePath}")
    }
}

tasks.whenTaskAdded {
    if (name == "mergeDebugAssets" || name == "mergeReleaseAssets") {
        dependsOn("buildMpvFrontend")
    }
}
