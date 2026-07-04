import com.combo.aar2apk.PackageBuildType
import org.jetbrains.kotlin.gradle.tasks.KotlinCompile
import java.util.Properties
import java.io.FileInputStream

val keystorePropertiesFile = rootProject.file("keystore.properties")
val keystoreProperties = Properties()
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(FileInputStream(keystorePropertiesFile))
}

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    alias(libs.plugins.combolite.aar2apk)
}

android {
    namespace = "com.encvgo.app"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        applicationId = "com.encvgo.app"
        minSdk = libs.versions.minSdk.get().toInt()
        targetSdk = libs.versions.targetSdk.get().toInt()
        versionCode = 1
        versionName = "1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        androidResources {
            ignoreAssetsPattern = "!.svn:!.git:!.ds_store:!*.scc:.*:!CVS:!thumbs.db:!picasa.ini:!*~"
        }

        ndk {
            abiFilters += setOf("arm64-v8a")
        }

        buildConfigField("String", "BUGLY_APP_ID", "\"${System.getenv("BUGLY_APP_ID") ?: ""}\"")
    }

    signingConfigs {
        if (keystoreProperties.containsKey("storeFile")) {
            create("release") {
                storeFile = file(keystoreProperties["storeFile"] as String)
                storePassword = keystoreProperties["storePassword"] as String
                keyAlias = keystoreProperties["keyAlias"] as String
                keyPassword = keystoreProperties["keyPassword"] as String
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            isShrinkResources = false
            signingConfig = signingConfigs.findByName("release")
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }

    compileOptions {
        targetCompatibility = JavaVersion.VERSION_21
        sourceCompatibility = JavaVersion.VERSION_21
    }

    buildFeatures {
        buildConfig = true
        compose = true
    }

    sourceSets {
        getByName("main") {
            jniLibs.srcDirs("src/main/jniLibs")
        }
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true
            pickFirsts += setOf("**/*.so")
        }
        resources {
            pickFirsts += setOf("**/*.so")
        }
    }
}

packagePlugins {
    enabled.set(false)
    buildType.set(PackageBuildType.DEBUG)
    pluginsDir.set("debug_plugins")
}

dependencies {
    implementation(fileTree(mapOf("include" to listOf("*.jar"), "dir" to "libs")))
    implementation(libs.androidx.appcompat)
    implementation(libs.androidx.coordinatorlayout)
    implementation(libs.androidx.core.splashscreen)
    implementation(project(":capacitor-android"))
    testImplementation(libs.junit)
    testImplementation(libs.mockito.core)
    testImplementation(libs.mockito.kotlin)
    testImplementation(libs.kotlin.test)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    implementation(project(":capacitor-cordova-android-plugins"))
    implementation(libs.kotlin.stdlib)
    implementation(libs.kotlin.reflect)
    debugImplementation(libs.logcat)
    implementation(libs.okhttp)
    implementation(libs.androidx.work.runtime.ktx)
    implementation(libs.bugly.crashreport)

    implementation(libs.combolite.core)
    implementation(project(":combolite-host"))
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.androidx.activity.compose)
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    implementation("androidx.activity:activity-ktx:1.11.0")
}

apply(from = "capacitor.build.gradle")

try {
    val servicesJSON = file("google-services.json")
    if (servicesJSON.exists() && servicesJSON.readText().isNotEmpty()) {
        apply(plugin = "com.google.gms.google-services")
    }
} catch (e: Exception) {
    logger.info("google-services.json not found, google-services plugin not applied. Push Notifications won't work")
}

// 🆕 2026-07-04：ObjectBox SONAME 兼容
// Go 二进制以 -tags objectbox 编译时，链接的是 libobjectbox.so，
// 但该 .so 的 ELF SONAME 字段为 libobjectbox-jni.so。
// Android linker 按 SONAME 查找依赖，因此 APK 内需要同时存在 libobjectbox-jni.so。
// 此 script 在 mergeNativeLibs 后在每个架构目录下创建 libobjectbox-jni.so 副本。
gradle.projectsEvaluated {
    tasks.matching { it.name.startsWith("merge") && it.name.endsWith("NativeLibs") }.configureEach {
        doLast {
            val variantDir = buildDir.resolve("intermediates/merged_native_libs/${name.removePrefix("merge").removeSuffix("NativeLibs").lowercase()}/out")
            if (!variantDir.exists()) return@doLast
            variantDir.walkTopDown().forEach { f ->
                if (f.name == "libobjectbox.so" && f.parentFile != null) {
                    val shim = File(f.parentFile, "libobjectbox-jni.so")
                    if (!shim.exists()) {
                        f.copyTo(shim)
                        logger.lifecycle("libobjectbox-jni.so shim created in ${f.parentFile.name}")
                    }
                }
            }
        }
    }
}

tasks.withType<KotlinCompile>().configureEach {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_21)
    }
}

android.sourceSets {
    getByName("main") {
        java.srcDirs("src/main/java")
    }
}
