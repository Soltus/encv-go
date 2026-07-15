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
        maven { url=uri ("https://mirrors.cloud.tencent.com/nexus/repository/maven-public/")}
        maven { url=uri ("https://mirrors.tencent.com/nexus/repository/gradle-plugins/")}
        maven { url = uri("https://mirrors.tencent.com/repository/maven-tencent/") }
        maven { url = uri("https://maven.aliyun.com/repository/public") }
        maven { url = uri("https://maven.aliyun.com/repository/google") }
        // com.github.*（如 getActivity:Logcat 及其传递依赖 EasyWindow）只在 JitPack 构建发布。
        // 腾讯云/阿里云的 maven-public 代理会为这些包返回 200 的 POM 但 404 的 aar/jar：
        // 一旦 Gradle 从代理拿到 POM，就会把该模块"钉"在代理仓库，后续 aar 下载只找代理、
        // 拿到 404 后不再回退到排在最后的 JitPack —— 这正是 build-logs 里"只搜了 maven-public"的原因。
        // 用 exclusiveContent 强制 com.github.* 只经由 JitPack 解析，既绕开代理"投毒"，
        // 也避免 JitPack 参与其它依赖的解析（保持镜像加速）。
        exclusiveContent {
            forRepository {
                maven { url = uri("https://jitpack.io") }
            }
            filter {
                includeGroupByRegex("com\\.github\\..*")
            }
        }
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
