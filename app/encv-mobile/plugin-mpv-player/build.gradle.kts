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
val mpvDistDir = layout.projectDirectory.dir("web/dist").asFile
val mpvAssetsDir = layout.projectDirectory.dir("src/main/assets/mpv").asFile

tasks.register<Exec>("installMpvFrontendDeps") {
    description = "Install mpv frontend npm dependencies"
    group = "build"
    onlyIf { File(mpvFrontendDir, "pnpm-lock.yaml").exists() && !File(mpvFrontendDir, "node_modules").exists() }
    workingDir = mpvFrontendDir
    commandLine("pnpm", "install", "--prefer-offline")
}

tasks.register<Exec>("buildMpvFrontendOnly") {
    description = "Build mpv frontend (vite build)"
    group = "build"
    dependsOn("installMpvFrontendDeps")
    workingDir = mpvFrontendDir
    commandLine("pnpm", "build")
    inputs.dir(mpvFrontendDir)
    outputs.dir(mpvDistDir)
}

tasks.register<Copy>("buildMpvFrontend") {
    description = "Build mpv frontend and copy to plugin assets"
    group = "build"
    dependsOn("buildMpvFrontendOnly")
    from(mpvDistDir)
    into(mpvAssetsDir)
}

tasks.whenTaskAdded {
    if (name == "mergeDebugAssets" || name == "mergeReleaseAssets") {
        dependsOn("buildMpvFrontend")
    }
}
