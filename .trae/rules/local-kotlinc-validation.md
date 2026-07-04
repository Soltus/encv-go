# 本地 Kotlin 编译验证规则

> 来自踩坑：曾两次 CI 编译失败（`OpenListNativeService.kt:161:21 'return' is prohibited here`），每次 CI 往返 5+ 分钟。
> 用户原话："**本身跑kotlin验证啊**"——所有 Kotlin 改动都应**先本地 kotlinc 验证**再 push。

> **完整命令模板 + 实战案例 + 错误诊断**：[详情文档](../rule-library/local-kotlinc-validation.md)

---

## 一、为什么必须本地验证

| 维度 | 本地 kotlinc | CI 跑 Gradle |
|------|-------------|--------------|
| 单次反馈时间 | **3-10 秒** | 5-10 分钟 |
| 失败定位 | 精确到 `file:line:col` | 同上（但要等 5 分钟） |
| 完整 Gradle 配置 | 不需要 | 需要 |
| Android SDK platforms | 只需要 `android.jar` | 完整 SDK |
| 适用场景 | syntax + 类型检查 + 跨文件引用 | 完整构建 + APK 打包 |

**铁律**：**改 .kt 后必跑本地 kotlinc 验证 → 通过再 git add → 通过再 push**。

---

## 二、沙盒环境工具链

| 工具 | 位置 | 版本 | 大小 |
|------|------|------|------|
| Java | 系统 OpenJDK | 17.0.2 | - |
| kotlin-compiler | `/tmp/kotlin/` (解压根) | 2.3.21 | 60 MB |
| kotlin-stdlib | `/tmp/klib/kotlin-stdlib.jar` | 2.3.21 | 1.8 MB |
| kotlin-reflect | `/tmp/klib/kotlin-reflect.jar` | 2.3.21 | 3.6 MB |
| kotlinx-coroutines | `/tmp/klib/coroutines.jar` | 1.10.0 | 1.5 MB |
| android.jar | `/tmp/platform-36/android-36/android.jar` | android-36 | 27 MB |
| **combolite-core** | `/tmp/klib/combolite-core.jar` | 2.0.2 | - |
| **androidx-core-ktx** | `/tmp/klib/androidx-core.jar` | 1.13.1 | - |
| **androidx-lbm** | `/tmp/klib/androidx-lbm.jar` | 1.1.0 | - |

**总开销 ~95 MB**，全部本地，无网络依赖。

> 重新安装命令（AAR → classes.jar 解包）→ [详情文档 §二.2/§二.3](../rule-library/local-kotlinc-validation.md#二沙盒环境工具链)

---

## 三、单文件验证（最快反馈）

```bash
java -cp "/tmp/kotlin:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar" \
  org.jetbrains.kotlin.cli.jvm.K2JVMCompiler \
  -d /tmp/kotlin-out \
  -cp "/tmp/platform-36/android-36/android.jar:/tmp/klib/kotlin-stdlib.jar:/tmp/klib/kotlin-reflect.jar:/tmp/klib/coroutines.jar" \
  -no-stdlib -no-reflect \
  -jvm-target 17 -Xskip-metadata-version-check \
  /path/to/SomeFile.kt
echo "exit=$?"
```

> `-no-stdlib -no-reflect` 告诉 kotlinc 我们已显式提供 stdlib/reflect（避免 "unable to find kotlin-home"）。

> 整 module 验证命令 + 6 个 flag 详解 → [详情文档 §三](../rule-library/local-kotlinc-validation.md#三编译命令模板)

---

## 四、错误诊断速查

| 错误 | 根因 | 修复 |
|------|------|------|
| `unresolved reference 'XxxClass'` | 缺 AAR 依赖 | 加 `xxx.jar` 到 `-cp` |
| `type mismatch: actual 'Any', expected 'XxxType'` | 同上（unresolved 后类型降级 Any） | 同上 |
| `'return' is prohibited here` | SAM lambda（`Thread`/`Runnable`）内 `return@...` | 抽到独立 fun 用裸 return |
| `type of 'val X' is not a subtype` | 同 unresolved | 补依赖到 `-cp` |

**unresolved 会级联**——一个缺失类型让后续所有用到它的地方报 type mismatch。**先补全依赖 → 剩下的才是真问题**。

> 4 类错误完整诊断 + TestLambdaReturn 最小复现 + 过滤噪音命令 → [详情文档 §四](../rule-library/local-kotlinc-validation.md#四错误诊断模式)

---

## 五、Kotlin lambda return 铁律（核心踩坑）

> **核心原则：`Thread { ... }` / `Runnable { ... }` / `Callable { ... }` 等 SAM-converted lambda 不是 inline lambda，Kotlin 不允许 `return@label` 也不允许裸 `return`。**

### 5.1 三种 return 形式

```kotlin
// ✅ ① inline lambda 内的 labeled return（forEach / map / let / also / apply）
listOf(1,2,3).forEach { if (it==2) return@forEach }   // 合法

// ❌ ② 非 inline lambda 内的 return
Thread { if (true) return@start }.start()              // 非法

// ✅ ③ 抽到独立 fun + 普通 fun body return
Thread { runInBackground() }.start()
private fun runInBackground() { if (true) return }     // 合法
```

### 5.2 inline vs 非 inline 速查

| 形式 | inline? | labeled return 合法? |
|------|---------|---------------------|
| `listOf().forEach { ... }` / `let` / `also` / `apply` | ✅ | ✅ |
| `Thread { ... }` / `Runnable` / `Callable` | ❌ SAM | ❌ |
| `view.setOnClickListener { ... }` | ❌ SAM | ❌ |
| `suspend { ... }` lambda | ✅ | ✅ |

> `OpenListNativeService.kt` L130-220 实战案例（e6efb7f 不充分 → ee136c4 正解）→ [详情文档 §五.4](../rule-library/local-kotlinc-validation.md#五kotlin-lambda-return-铁律核心踩坑)

---

## 六、强制验证检查清单

每次改 .kt 文件后：

- [ ] 本地 kotlinc 编译通过（`exit=0` 或 0 真实错误）
- [ ] 关键文件（plugin-openlist 等）整 module 编译 0 unresolved
- [ ] 全局 grep 无 `prohibited` 错误
- [ ] 涉及 Thread/Runnable/Callable 等 SAM lambda 时确认无 `return@...`
- [ ] git commit 附带 **"本地 kotlinc 验证通过"** 标记
- [ ] push 前 commit message 写 "local kotlinc validated"

---

## 七、参考

- [kotlin-android.md](kotlin-android.md) — 完整 Android 编码标准
- [verification-discipline.md](verification-discipline.md) — 验证纪律（核实先于生成）

> 拆分：2026-06-11
