pluginManagement {
    repositories {
        mavenCentral()
        google()
        gradlePluginPortal()
        maven { url = uri("https://plugins.gradle.org/m2/") }
        maven { url = uri("https://maven.aliyun.com/repository/google") }
        maven { url = uri("https://maven.aliyun.com/repository/central") }
        maven { url = uri("https://maven.aliyun.com/repository/gradle-plugin") }
        maven { url = uri("https://maven.aliyun.com/repository/public") }
        maven { url = uri("https://mirrors.tencent.com/nexus/repository/maven-tencent/") }
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.PREFER_PROJECT)
    repositories {
        google()
        mavenCentral()
        // CI 环境在腾讯云，优先用腾讯镜像；阿里云镜像偶发 502 放在最后做兜底
        if (System.getenv("CI") == null) {
            maven { url = uri("https://mirrors.tencent.com/nexus/repository/maven-public/") }
        }
        maven { url = uri("https://mirrors.tencent.com/repository/maven-tencent/") }
        maven { url = uri("https://maven.aliyun.com/repository/public") }
        maven { url = uri("https://maven.aliyun.com/repository/google") }
        maven { url = uri("https://jitpack.io") }
        flatDir {
            dirs("${rootProject.projectDir}/capacitor-cordova-android-plugins/src/main/libs", "${rootProject.projectDir}/app/libs")
        }
    }
}

rootProject.name = "encv-mobile"

include(":app")
include(":capacitor-cordova-android-plugins")
include(":combolite-host")

// Plugin subprojects are ONLY included when explicitly requested (-PincludePlugins=true).
// Without this flag, Gradle's full task graph resolution pulls in
//   :plugin-openlist:compileReleaseKotlin (and :plugin-mpv-player:*)
// whenever any assemble/build task runs — even though :app does NOT declare
//   implementation(project(":plugin-openlist")).  A plugin Kotlin compile error
//   then blocks the entire main app build.
//
// Usage:
//   ./gradlew assembleDebug                        # ← main app only (fast)
//   ./gradlew -PincludePlugins=true assembleDebug   # ← main app + plugins
//   ./gradlew -PincludePlugins=true :plugin-openlist:compileReleaseKotlin  # ← plugin only
//
// NOTE: settings.gradle.kts does NOT have findProperty() (that's a Project API).
// We read from gradle.startParameter.projectProperties instead.
val includePlugins = gradle.startParameter.projectProperties["includePlugins"] == "true"

if (includePlugins) {
    include(":plugin-mpv-player")
    include(":plugin-openlist")
    include(":plugin-simverse")
}

project(":capacitor-cordova-android-plugins").projectDir = file("./capacitor-cordova-android-plugins/")
if (includePlugins) {
    project(":plugin-mpv-player").projectDir = file("../plugin-mpv-player")
    project(":plugin-openlist").projectDir = file("../plugin-openlist")
    project(":plugin-simverse").projectDir = file("../plugin-simverse")
}

apply(from = "capacitor.settings.gradle")
