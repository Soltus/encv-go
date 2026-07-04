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

// 🆕 2026-07-04：从 Go 构建输出同步 ObjectBox JNI 库到 Android jniLibs
// build-objectbox-android.sh 输出到 pkg/tasksystem/store/objectbox/libs/android_<abi>/
// 此 task 在 merge*NativeLibs 之前执行，确保 linker 需要的 .so 存在。
//
// ABI 目录名映射：
//   android_arm64 → arm64-v8a
//   android_armv7 → armeabi-v7a
//   android_x86   → x86
//   android_x86_64 → x86_64
val ABI_MAP = mapOf(
    "android_arm64" to "arm64-v8a",
    "android_armv7" to "armeabi-v7a",
    "android_x86" to "x86",
    "android_x86_64" to "x86_64",
)

tasks.register("prepareObjectboxJniLibs") {
    description = "从 Go 构建产物复制 ObjectBox JNI 库到 jniLibs"
    doLast {
        val srcBase = file("${rootProject.projectDir}/../../../pkg/tasksystem/store/objectbox/libs")
        if (!srcBase.exists()) {
            logger.debug("prepareObjectboxJniLibs: srcBase $srcBase not found, skipping")
            return@doLast
        }
        srcBase.listFiles()?.forEach { archDir ->
            if (!archDir.isDirectory) return@forEach
            val targetAbi = ABI_MAP[archDir.name] ?: return@forEach
            val targetDir = file("src/main/jniLibs/$targetAbi")
            targetDir.mkdirs()
            archDir.listFiles { f -> f.name.endsWith(".so") }?.forEach { soFile ->
                val target = targetDir.resolve(soFile.name)
                if (!target.exists() || target.length() != soFile.length()) {
                    soFile.copyTo(target, overwrite = true)
                    logger.lifecycle("prepareObjectboxJniLibs: copied ${soFile.name} → $targetAbi/")
                }
            }
        }
    }
}

tasks.matching { it.name.startsWith("merge") && it.name.endsWith("NativeLibs") }.configureEach {
    dependsOn("prepareObjectboxJniLibs")
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
