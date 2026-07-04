# local-kotlinc-validation 详情

> 本文件为 [local-kotlinc-validation.md](../rules/local-kotlinc-validation.md) 的详情文档。
>
> 索引位于 [`.trae/rules/local-kotlinc-validation.md`](../rules/local-kotlinc-validation.md)。本文件汇总索引未包含的完整安装命令、整 module 编译命令、flag 详解、错误诊断实战、TestLambdaReturn 最小复现、OpenListNativeService.kt 实战修复案例。

---

## 二、沙盒环境工具链（完整命令）

### 2.1 已就位（无需重装）

| 工具 | 位置 | 版本 | 大小 |
|------|------|------|------|
| Java | 系统 OpenJDK | 17.0.2 | - |
| kotlin-compiler | `/tmp/kotlin/` (解压根) | 2.3.21 | 60 MB |
| kotlin-stdlib | `/tmp/klib/kotlin-stdlib.jar` | 2.3.21 | 1.8 MB |
| kotlin-reflect | `/tmp/klib/kotlin-reflect.jar` | 2.3.21 | 3.6 MB |
| kotlinx-coroutines | `/tmp/klib/coroutines.jar` | 1.10.0 | 1.5 MB |
| android.jar | `/tmp/platform-36/android-36/android.jar` | android-36 | 27 MB |

**总开销 ~95 MB**，全部本地，无网络依赖。

### 2.2 重新装一遍（沙盒重启后）

```bash
mkdir -p /tmp/klib

# kotlin-compiler-embeddable（解压到 /tmp/kotlin/）
curl -sSL --max-time 120 -o /tmp/kotlin-compiler.zip \
  "https://maven.aliyun.com/repository/public/org/jetbrains/kotlin/kotlin-compiler-embeddable/2.3.21/kotlin-compiler-embeddable-2.3.21.jar"
unzip -q -o /tmp/kotlin-compiler.zip -d /tmp/kotlin

# kotlin-stdlib + reflect（解 jar 即可用）
curl -sSL --max-time 60 -o /tmp/klib/kotlin-stdlib.jar \
  "https://maven.aliyun.com/repository/public/org/jetbrains/kotlin/kotlin-stdlib/2.3.21/kotlin-stdlib-2.3.21.jar"
curl -sSL --max-time 60 -o /tmp/klib/kotlin-reflect.jar \
  "https://maven.aliyun.com/repository/public/org/jetbrains/kotlin/kotlin-reflect/2.3.21/kotlin-reflect-2.3.21.jar"

# kotlinx-coroutines（K2JVMCompiler 自身依赖）
curl -sSL --max-time 60 -o /tmp/klib/coroutines.jar \
  "https://maven.aliyun.com/repository/public/org/jetbrains/kotlinx/kotlinx-coroutines-core-jvm/1.10.0/kotlinx-coroutines-core-jvm-1.10.0.jar"

# android.jar（compileSdk=36，从 Google 平台包取）
curl -sSL --max-time 180 -o /tmp/platform-36.zip \
  "https://dl.google.com/android/repository/platform-36_r02.zip"
unzip -q -o /tmp/platform-36.zip -d /tmp/platform-36
# → /tmp/platform-36/android-36/android.jar (27 MB)
```

### 2.3 第三方依赖（AAR → classes.jar）

插件代码常引用 `com.combo.core.*` (combolite) / `androidx.*` 等。这些来自 AAR，需解出 `classes.jar`：

```bash
cd /tmp/klib

# ComboLite core (BasePluginService, IPluginEntryClass)
curl -sSL --max-time 60 -o /tmp/combolite.aar \
  "https://maven.aliyun.com/repository/public/io/github/lnzz123/combolite-core/2.0.2/combolite-core-2.0.2.aar"
mkdir -p combolite-extr && unzip -o -q /tmp/combolite.aar -d combolite-extr classes.jar
mv combolite-extr/classes.jar combolite-core.jar
rm -rf combolite-extr /tmp/combolite.aar

# androidx.core (NotificationCompat, ContextCompat 等)
curl -sSL --max-time 60 -o /tmp/core.aar \
  "https://maven.aliyun.com/repository/google/androidx/core/core-ktx/1.13.1/core-ktx-1.13.1.aar"
mkdir -p core-extr && unzip -o -q /tmp/core.aar -d core-extr classes.jar
mv core-extr/classes.jar androidx-core.jar
rm -rf core-extr /tmp/core.aar

# androidx.localbroadcastmanager
curl -sSL --max-time 60 -o /tmp/lbm.aar \
  "https://maven.aliyun.com/repository/google/androidx/localbroadcastmanager/localbroadcastmanager/1.1.0/localbroadcastmanager-1.1.0.aar"
mkdir -p lbm-extr && unzip -o -q /tmp/lbm.aar -d lbm-extr classes.jar
mv lbm-extr/classes.jar androidx-lbm.jar
rm -rf lbm-extr /tmp/lbm.aar
```

**AAR 本质是 zip**（注意：阿里云镜像有些 .aar 文件实际内含 `classes.jar` 也有些含 `classes-proguard.jar`/`classes.jar` 两个，按需取）。

---

## 三、编译命令模板（完整）

### 3.1 单文件验证（最快反馈）

```bash
java -cp "/tmp/kotlin:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar" \
  org.jetbrains.kotlin.cli.jvm.K2JVMCompiler \
  -d /tmp/kotlin-out \
  -cp "/tmp/platform-36/android-36/android.jar:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar" \
  -no-stdlib -no-reflect \
  -jvm-target 17 \
  -Xskip-metadata-version-check \
  /path/to/SomeFile.kt
echo "exit=$?"
```

> `-no-stdlib -no-reflect` 告诉 kotlinc 我们已显式提供 stdlib/reflect（避免 "unable to find kotlin-home" 警告，且不会污染输出）。

### 3.2 整个 module 验证（plugin-openlist 8 个 kt 文件）

```bash
cd /workspace
java -cp "/tmp/kotlin:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar" \
  org.jetbrains.kotlin.cli.jvm.K2JVMCompiler \
  -d /tmp/kotlin-out \
  -cp "/tmp/platform-36/android-36/android.jar:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar:/tmp/klib/androidx-core.jar:/tmp/klib/androidx-lbm.jar:/tmp/klib/combolite-core.jar" \
  -no-stdlib -no-reflect \
  -jvm-target 17 \
  -Xskip-metadata-version-check \
  app/encv-mobile/plugin-openlist/src/main/java/com/encvgo/plugin/openlist/*.kt
```

### 3.3 关键 flag 说明

| Flag | 用途 |
|------|------|
| `-d <dir>` | 输出 class 文件目录（不存在会自动建） |
| `-cp <paths>` | 编译时类型上下文（jar 路径或目录，**冒号分隔**） |
| `-no-stdlib` | 不自动附加 stdlib（避免与 -cp 提供的 stdlib.jar 冲突） |
| `-no-reflect` | 不自动附加 reflect（同上） |
| `-jvm-target 17` | 与项目 compileOptions 保持一致 |
| `-Xskip-metadata-version-check` | 容忍 metadata 版本不匹配（避免 Kotlin 编译器版本警告） |

---

## 四、错误诊断模式（完整）

### 4.1 常见 4 类错误

| 错误类型 | 示例 | 修复 |
|---------|------|------|
| **unresolved reference** | `unresolved reference 'BasePluginService'` | 加 AAR 到 -cp |
| **type mismatch** | `argument type mismatch: actual type is 'Any', but 'Context' was expected` | 同上（unresolved 后类型降级为 Any） |
| **'return' is prohibited here** | `error: 'return' is prohibited here` (非 inline lambda) | **抽到独立 fun 用裸 return**（见 §五） |
| **type of val is not a subtype** | `type of 'val pluginModule' is not a subtype` | 同 unresolved（依赖缺失） |

### 4.2 过滤噪音

unresolved 错误会**级联**（一个缺失类型会让后续所有用到它的地方报 type mismatch）。先**补全依赖**到 -cp 让 unresolved 消失，**剩下的**才是真问题。

```bash
# 跑完检查
grep -cE "^.*: error:" /tmp/kotlinc.log  # 错误总数
grep "OpenListNativeService.kt:" /tmp/kotlinc.log  # 目标文件 errors
grep -i "prohibited" /tmp/kotlinc.log  # return 错误
```

### 4.3 快速判定 `return` 修复是否合法

写一个最小复现文件验证（参考 §五 TestLambdaReturn）：

```bash
cat > /tmp/TestReturn.kt <<'EOF'
package test
object TestReturn {
    fun bad() {
        Thread {
            return@bad  // ← 应报 'return' is prohibited here
        }.start()
    }
    fun good() {
        Thread { runInBackground() }.start()
    }
    private fun runInBackground() {
        return  // ← 普通 fun body return,合法
    }
}
EOF

java -cp "/tmp/kotlin:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar" \
  org.jetbrains.kotlin.cli.jvm.K2JVMCompiler -d /tmp/kotlin-out \
  -cp "/tmp/klib/kotlin-stdlib.jar" -no-stdlib \
  -jvm-target 17 /tmp/TestReturn.kt 2>&1 | head -20
```

**预期输出**：
```
/tmp/TestReturn.kt:4:17: error: 'return' is prohibited here.
                return@bad
```
- `bad()` 报错 → 证明 e6efb7f 模式非法
- `good()` 通过 → 证明 ee136c4 模式合法

---

## 五、Kotlin lambda return 铁律（核心踩坑）— 完整版

### 5.1 三种 return 形式对比

```kotlin
object LambdaReturn {

    // ✅ ① inline lambda 内的 labeled return（forEach / map / let / also / apply 等）
    fun inlineLambdaReturn() {
        listOf(1, 2, 3).forEach {
            if (it == 2) return@forEach  // ← 合法
        }
    }

    // ❌ ② 非 inline lambda 内的 return（含 labeled return）
    fun nonInlineLambdaReturn() {
        Thread {
            if (true) return@nonInlineLambdaReturn  // ← 'return' is prohibited here
        }.start()
    }

    // ✅ ③ 抽到独立 fun + 普通 fun body return
    fun extractedFunReturn() {
        Thread { runInBackground() }.start()
    }

    private fun runInBackground() {
        if (true) return  // ← 普通 fun body return,合法
    }
}
```

### 5.2 错误诊断

| 错误信息 | 含义 | 根因 | 修复 |
|---------|------|------|------|
| `'return' is prohibited here` | 非 inline lambda 内有 return | `Thread { ... }` 等 SAM lambda 不是 inline | 抽到独立 fun |
| `return is not allowed here` | 顶层/嵌套函数表达式体 | 表达式体不能有 return | 改 fun body `{}` |

### 5.3 易混淆的 inline vs 非 inline

| 形式 | inline? | labeled return 合法? |
|------|---------|---------------------|
| `listOf().forEach { ... }` | ✅ inline | ✅ `return@forEach` |
| `let { ... }` / `also { ... }` | ✅ inline | ✅ `return@let` |
| `Thread { ... }` | ❌ SAM | ❌ 任何 return |
| `Runnable { ... }` | ❌ SAM | ❌ |
| `Callable { ... }` | ❌ SAM | ❌ |
| `view.setOnClickListener { ... }` | ❌ SAM | ❌ |
| `suspend { ... }` lambda | ✅ inline | ✅ |

### 5.4 实战案例：OpenListNativeService.kt

**Phase 27 修复历史**（commit `e6efb7f` 不充分 → `ee136c4` 正解）：

- **e6efb7f（不充分）**：
  ```kotlin
  fun start(ctx: Context) {
      Thread {
          val binary = locateNativeBinary(ctx)
          if (binary == null) {
              return@start  // ← Kotlin 报 'return' is prohibited here
          }
          // ...
      }.start()
  }
  ```
  
- **ee136c4（正解）**：
  ```kotlin
  fun start(ctx: Context) {
      Thread {
          runStartInBackground(ctx)  // ← 委托到独立 fun
      }.start()
  }
  
  private fun runStartInBackground(ctx: Context) {
      val binary = locateNativeBinary(ctx)
      if (binary == null) {
          return  // ← 普通 fun body return,合法
      }
      // ...
  }
  ```

**关键启示**：
- 不要在 `Thread { ... }` / `Runnable { ... }` 等 SAM lambda 内放任何 return
- 早期 null check、参数提取、错误处理都委托到独立 fun
- SAM lambda 主体应只有 1-3 行调用

---

## 六、强制验证检查清单

每次改 .kt 文件后：

- [ ] 本地 kotlinc 编译通过（`exit=0` 或 0 真实错误）
- [ ] 关键文件（plugin-openlist 等）整 module 编译 0 unresolved
- [ ] 全局 grep 无 `prohibited` 错误
- [ ] 涉及 Thread/Runnable/Callable 等 SAM lambda 时确认无 `return@...`
- [ ] git add + commit 附带 **"本地 kotlinc 验证通过"** 标记
- [ ] push 前在 commit message 写 "local kotlinc validated"

---

## 七、参考

| 主题 | 文档 |
|------|------|
| 完整 Android 编码标准 | [kotlin-android.md](../rules/kotlin-android.md) |
| 验证纪律（核实先于生成） | [verification-discipline.md](../rules/verification-discipline.md) |
| Phase 27 集成背景 | [build-openlist-fork-as-android-native/spec.md](../../specs/build-openlist-fork-as-android-native/spec.md) |

> 拆分：2026-06-11
