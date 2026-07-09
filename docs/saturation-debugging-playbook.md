# 饱和调试方法论 · 技巧与实践

> **核心思想**：不猜问题，一次构建覆盖全链路诊断，收集足够运行时信息定位根因。
> **适用场景**：真机环境问题、沙箱无法复现的问题、跨层耦合问题（前端→Bridge→原生→框架→插件）。

---

## 一、什么是饱和调试

### 1.1 定义

**饱和调试（Saturation Debugging）** 是一种「宁可多测不可漏测」的调试策略：
- 在**一次构建**中注入全链路诊断探针
- 触发问题时**同时收集所有相关层的状态**
- 通过**结构化日志 + UI 可复制报告**快速定位根因

> 反模式：猜一个原因改一次代码，真机验证一次又一次 —— 每次只能验证一个假设，迭代极慢。

### 1.2 为什么需要饱和调试

| 问题类型 | 传统调试 | 饱和调试 |
|---------|---------|---------|
| 真机环境差异 | 猜 + 反复试 | 一次收集所有环境信息 |
| 跨层调用链断裂 | 逐层打 log，每次只加一层 | 一次打全所有层的 log |
| 偶发问题 | 碰运气复现 | 失败时自动触发诊断，留痕 |
| 新人接手 | 从头摸一遍 | 看诊断报告直接定位 |

---

## 二、饱和调试的三要素

### 2.1 要素一：手动诊断入口（UI 按钮）

用户可主动触发，适用于「问题稳定复现」的场景。

**设计规范**：

```
入口位置：设置页 / 扩展页 / 调试抽屉
按钮文案："🌍 SimVerse全链路饱和诊断"
点击反馈：立即展示 loading → 展示格式化结果
结果展示：
  - 等宽字体 <pre> 包裹
  - 带复制按钮（return false 防止弹窗关闭）
  - 步骤用 ═══ 分隔，状态用 ✅/⚠️/❌ 标记
```

**代码模式**（Kotlin 后端）：

```kotlin
@PluginMethod
fun debugXxxFlow(call: PluginCall) {
    val steps = mutableListOf<String>()
    steps.add("╔══════════════════════════════════════════════════╗")
    steps.add("║     Xxx 全链路饱和诊断 (v1)                       ║")
    steps.add("╚══════════════════════════════════════════════════╝")

    // 每一步都要：try-catch + 日志 + 步骤追加
    steps.add("═══ 1. 框架状态 ═══")
    try {
        val ok = checkFramework()
        steps.add("   ✅ 框架正常")
        satLog("S01", "框架正常")
    } catch (e: Exception) {
        steps.add("   ❌ 框架异常: ${e.message}")
        satError("S01", "框架异常", e)
    }

    // ... 更多步骤 ...

    steps.add("═══ N. 诊断小结 ═══")
    steps.add("   ✅ 通过: $passCount")
    steps.add("   ⚠️  警告: $warnCount")
    steps.add("   ❌ 失败: $failCount")

    call.resolve(JSObject().put("debugLog", steps.joinToString("\n")))
}
```

### 2.2 要素二：自动触发诊断（失败时自动收集）

适用于「偶发问题」「用户操作路径不明确」的场景。

**触发时机**：
- 核心操作失败时（如 `openWorld` 失败）
- 异常 catch 块中
- 关键状态异常时

**设计规范**：

```kotlin
private fun runAutoDiagnosis(reason: String): String {
    satError("AUTO-DIAG", "自动触发饱和诊断，原因: $reason")
    return try {
        // 轻量级诊断（不要太耗时，5-10 个关键检查点）
        val state = EncvComboLiteHost.getPluginFullState(PLUGIN_ID)
        "frameworkInit=${EncvComboLiteHost.isInitialized} | " +
        "pluginState=${state.status} | " +
        "pluginName=${state.name} | " +
        "pluginVersion=${state.version}"
    } catch (e: Exception) {
        "自动诊断失败: ${e.javaClass.simpleName}: ${e.message}"
    }
}

// 使用方式
fun someOperation(call: PluginCall) {
    try {
        // ... 业务逻辑 ...
        if (somethingFailed) {
            val diag = runAutoDiagnosis("something failed")
            call.reject("Error message. $diag")
            return
        }
    } catch (e: Exception) {
        val diag = runAutoDiagnosis("exception: ${e.javaClass.simpleName}")
        satError("OP", "failed: ${e.message}, $diag", e)
        call.reject("${e.message}. $diag")
    }
}
```

**关键原则**：
- 自动诊断是**轻量级**的（5-10 个检查点），不要阻塞用户
- 完整诊断留给手动入口
- 诊断结果要**附加到错误信息**里，用户复制粘贴就能反馈

### 2.3 要素三：统一 TAG 日志（logcat 可过滤）

所有饱和调试相关的日志都用统一前缀，方便 `adb logcat` 过滤。

**命名规范**：

```
格式：{Feature}-SAT
示例：SimVerse-SAT, OpenList-SAT, MPV-SAT

日志级别：
  - satLog(step, msg)  → Log.e（高亮，方便过滤）
  - satWarn(step, msg) → Log.w
  - satError(step, msg, throwable?) → Log.e + 堆栈
```

**Step 编号规范**：

```
S00-START / S00-END   诊断起止
S01-FRAMEWORK         框架层
S02-STATE             状态查询
S03-INSTALLED         安装列表
S04-LOADED            加载状态
S05-APK / S05-DEX     APK/DEX 检查
S06-ACTIVITY          Activity 解析
S07-HOST              Host 组件
S08-PROXY             代理Intent
S09-API               API 地址
S10-LOAD              加载测试
S11-REFLECT           反射检查
S12-DEVICE            设备信息
S13-ASSETS            前端资源 / APK 资产检查
S14-WV-PAGE           WebView 页面加载 (onPageStarted/Finished)
S15-WV-ERROR          WebView 资源加载错误 (onReceivedError/HttpError)
S16-WV-CONSOLE        WebView Console 消息 (JS error/warn)
AUTO-DIAG             自动触发诊断
OPEN-WORLD            业务操作（内嵌诊断调用）
```

**过滤命令**：

```bash
# 只看 SimVerse 饱和调试日志
adb logcat -s SimVerse-SAT:*

# 看所有饱和调试日志
adb logcat | grep -E '\w+-SAT'
```

---

## 三、诊断步骤设计方法论

### 3.1 分层原则：从外到内，从粗到细

```
Layer 0: 入口参数 / 上下文检查（activity / context 是否为 null）
Layer 1: 框架状态（ComboLite / PluginManager 是否初始化）
Layer 2: 插件存在性（是否安装 / 是否启用 / 是否加载）
Layer 3: 组件完整性（Activity 解析 / ClassLoader / entry class）
Layer 4: 业务配置（API 地址 / extras / Intent 构造）
Layer 5: 环境信息（设备型号 / SDK 版本 / ABI）
Layer 6: 前端资产（APK assets 目录结构 / index.html 存在性）
Layer 7: WebView 层（页面加载 / 资源错误 / JS console）
```

**每一层都要独立 try-catch**，不能因为前面失败就跳过后面。

### 3.2 互斥条件也要同时检查

> 正常逻辑里我们会 early return，但饱和调试要「哪怕条件不满足也检查下游」。

**反模式（正常业务逻辑）**：
```kotlin
if (!frameworkInit) {
    call.reject("framework not ready")
    return  // ← 正常逻辑没问题，但调试时信息不够
}
val pluginInfo = getPluginInfo() // ← 如果 framework 没初始化就看不到这里
```

**正确模式（饱和调试）**：
```kotlin
steps.add("═══ 1. 框架状态 ═══")
val frameworkOk = checkFramework()
steps.add("")

steps.add("═══ 2. 插件信息 ═══")
// 即使 frameworkOk == false，也要尝试（看具体报什么错）
try {
    val info = PluginManager.getPluginInfo(PLUGIN_ID)
    // ...
} catch (e: Exception) {
    steps.add("   ❌ FAILED: ${e.message}")
    // 记录下来，和框架状态交叉验证
}
```

### 3.3 诊断小结必须有

最后一步一定要统计 pass/warn/fail 数量，让用户一眼看到问题严重程度。

```kotlin
var passCount = 0
var failCount = 0
var warnCount = 0
for (line in steps) {
    if (line.contains("✅")) passCount++
    if (line.contains("❌")) failCount++
    if (line.contains("⚠️")) warnCount++
}
steps.add("   ✅ 通过: $passCount")
steps.add("   ⚠️  警告: $warnCount")
steps.add("   ❌ 失败: $failCount")
```

---

## 四、前端 UI 模式

### 4.1 诊断按钮 + 结果弹窗

```vue
<template>
  <ion-button expand="block" fill="outline" color="tertiary"
              @click="handleDebugXxx" size="small">
    🌍 Xxx全链路饱和诊断
  </ion-button>
</template>

<script setup lang="ts">
import { debugXxxFlow } from "@/plugins/Xxx";

async function handleDebugXxx() {
  try {
    const result = await debugXxxFlow();
    await showDebugResult("🌍 Xxx全链路饱和诊断", result);
  } catch (e: any) {
    await showDebugResult("🌍 诊断失败", { debugLog: e?.message || String(e) });
  }
}

async function showDebugResult(title: string, data: { debugLog: string }) {
  const alert = await alertController.create({
    header: title,
    message: `<pre style="font-size:10px;max-height:60vh;overflow:auto;white-space:pre-wrap;">${data.debugLog}</pre>`,
    buttons: [
      {
        text: "复制",
        handler: () => {
          navigator.clipboard?.writeText(data.debugLog);
          return false; // ← 关键：不关闭弹窗，用户可以反复复制
        },
      },
      "关闭",
    ],
  });
  await alert.present();
}
</script>
```

### 4.2 自动诊断结果的 UI 展示

当业务操作失败且附带自动诊断信息时，前端要把诊断信息和错误信息一起展示：

```ts
// 错误信息格式："用户可读的错误. [自动诊断 - 原因: xxx] frameworkInit=... | pluginState=..."
try {
  await openWorld(...);
} catch (e: any) {
  const msg = e?.message || String(e);
  // 自动诊断信息会附在错误消息末尾，用户复制时一起带走
  await showErrorAlert("启动失败", msg);
}
```

---

## 五、CI 配合：确保诊断代码随主包构建

### 5.1 诊断代码是生产代码

饱和诊断代码**不是注释掉的调试代码**，它：
- 存在于生产构建中
- 用户可以主动触发
- 失败时自动触发
- 不影响正常功能（try-catch 包裹）

### 5.2 CI 必须验证诊断代码能编译

- 诊断方法用 `@PluginMethod` 注解，和正常方法一样参与编译
- CI 编译失败 = 诊断代码有 bug，必须修复

### 5.3 插件诊断随插件构建

对于 ComboLite 插件，诊断入口放在**宿主的 Capacitor 插件**里（如 `SimVersePlugin`），
这样即使插件本身没安装，宿主也能诊断「插件为什么没安装」。

---

## 六、实战案例：SimVerse 饱和诊断

### 6.1 背景

SimVerse 启动后白屏（页面完全空白），沙箱环境完全正常，真机必现。
可能的问题横跨 7 层：前端 → Capacitor Bridge → ComboLite 框架 → 插件加载 → WebView 初始化 → WebView 资源加载 → 前端代码执行。

**关键诊断缺口**：前 12 步全绿但依旧白屏，说明问题出在 WebView 层（S14-S16），这是初期饱和诊断的盲区。

### 6.2 诊断步骤设计（16 步）

| 步骤 | 名称 | 检查内容 |
|------|------|---------|
| S01 | ComboLite 框架状态 | EncvComboLiteHost.isInitialized, PluginManager.isInitialized |
| S02 | 插件安装/加载状态 | getPluginFullState (status/name/version) |
| S03 | 已安装插件列表 | getAllInstallPlugins，目标插件是否存在 |
| S04 | 插件 Info 详细检查 | getPluginInfo，id/name/version/enabled/entryClass |
| S05 | APK / DEX 检查 | ClassLoader, entryClass 反射加载, methods/fields |
| S06 | Target Activity 解析 | PackageManager.resolveActivity |
| S07 | Host Activity 检查 | EncvHostActivity 可解析性 |
| S08 | createProxyIntent 构造 | component/extras/flags 全量打印 |
| S09 | API Base URL 检查 | SharedPreferences server_port 读取 |
| S10 | ensurePluginLoaded 测试 | 未加载则尝试加载并验证 |
| S11 | entryClass 反射方法检查 | 全量 public methods / fields |
| S12 | 设备信息 | SDK/BRAND/MODEL/ABIs |
| S13 | APK 资产检查 | assets/ 目录结构、index.html 是否存在、文件数量 |
| S14 | WebView 页面加载 | onPageStarted/onPageFinished 次数、URL、耗时 |
| S15 | WebView 资源错误 | onReceivedError/onReceivedHttpError 次数、最后错误 |
| S16 | WebView Console | JS error/warning 记录（最近 N 条） |
| S17 | 诊断小结 | pass/warn/fail 计数 |

### 6.3 WebView 层诊断三要素

**为什么需要专门的 WebView 层诊断？**
- WebView 内部是一个独立的渲染/JS 引擎，外部 Kotlin 代码无法直接看到内部状态
- 白屏可能是 CSS 加载失败、JS 执行错误、CORS 拦截、MIME type 错误等多种原因
- 前 12 步（框架+插件）全绿 ≠ WebView 里的前端正常运行

**三种诊断探针：**

| 探针 | 注入点 | 捕获内容 |
|------|--------|---------|
| WebViewClient | `onPageStarted` / `onPageFinished` / `onReceivedError` / `onReceivedHttpError` | 页面生命周期 + 资源加载错误 |
| WebChromeClient | `onConsoleMessage` | JS console.error / console.warn / 未捕获异常 |
| 原生悬浮按钮 | Activity 层面叠加一个半透明按钮（30% alpha，右上角） | 白屏时用户也能点开看诊断（不依赖前端 UI） |

**关键发现：`file://` + CORS 导致白屏**

```
pageStartedCount = 1, pageFinishedCount = 1, loadDurationMs = 9
errorCount = 5, consoleErrorCount = 5
lastError = code=-1, desc=net::ERR_FAILED, url=file:///android_asset/simverse/assets/index-xxx.css
console error: Access to CSS stylesheet at 'file:///android_asset/...' from origin 'null'
               has been blocked by CORS policy
```

**根因**：`file://` 协议的 origin 是 `null`，WebView CORS 策略将同目录下的 CSS/JS 加载视为跨域，全部拦截 → 页面白屏。

**修复**：`WebSettings.setAllowFileAccessFromFileURLs(true)` + `setAllowUniversalAccessFromFileURLs(true)`。

### 6.4 原生悬浮诊断按钮模式

> 白屏时前端 UI 完全不可用，必须有一个不依赖前端的诊断入口。

**实现方式**：
- Activity 的 Compose 布局中，用 `Box` 叠加 `AndroidView`（原生 `FrameLayout` + `Button`）
- 按钮：48dp 圆形，30% 透明度，右上角，"🔍" emoji
- 点击 → `AlertDialog` 展示诊断报告 + 复制按钮
- 长按也触发（防止点击被前端 WebView 消费）

**为什么不用 Compose 按钮？**：WebView 是 Android View，叠加在 Compose 层上面。Compose 按钮会被 WebView 覆盖（z-order 问题），所以必须用原生 AndroidView 叠加。

### 6.5 自动触发点
- `activity is null`
- `context is null`
- `ComboLite not initialized`
- `plugin not installed`
- `plugin disabled`
- `ensurePluginLoaded failed`
- `loadedInfo is null`
- 任何 Exception

每个失败点都调用 `runAutoDiagnosis(reason)`，把轻量级诊断结果附加到 reject 消息里。

### 6.6 统一 TAG

所有日志用 `SimVerse-SAT` TAG，过滤命令：
```bash
adb logcat -s SimVerse-SAT:*
```

---

## 七、铁律（Do & Don't）

### ✅ Do

1. **一次构建，全链路覆盖** — 不要每次只加一个 log
2. **每步独立 try-catch** — 前面失败不影响后面检查
3. **手动 + 自动双入口** — 稳定复现用手动，偶发用自动
4. **统一 TAG 前缀** — 方便 logcat 过滤
5. **结果可复制** — UI 弹窗必须带复制按钮，且 `return false` 不关闭
6. **诊断代码上生产** — 不是临时调试代码，是产品的一部分

### ❌ Don't

1. **不要猜问题** — 饱和调试的核心是「收集信息，不是验证假设」
2. **不要 early return** — 正常逻辑可以 early return，诊断逻辑必须跑完
3. **不要只打一行 log** — 至少要包含：步骤编号、状态、关键变量
4. **不要把诊断代码藏在注释里** — 要能在生产环境随时触发
5. **不要忘记自动诊断** — 失败时自动收集，比用户手动反馈快得多

---

## 八、相关文件参考

| 文件 | 作用 |
|------|------|
| `app/encv-mobile/android/app/src/main/java/com/encvgo/app/SimVersePlugin.kt` | SimVerse 饱和诊断完整实现（宿主侧，参考模板） |
| `app/encv-mobile/android/combolite-host/src/main/java/com/encvgo/combolite/diagnostic/DiagnosticKit.kt` | ComboLite 框架级诊断工具 |
| `app/encv-mobile/src/views/ExtensionsPage.vue` | 前端诊断按钮 + 弹窗实现 |
| `app/encv-mobile/src/plugins/SimVerse.ts` | 前端诊断方法类型定义 |
| `app/encv-mobile/plugin-simverse/src/main/java/com/encvgo/plugin/simverse/SimVerseWebViewClient.kt` | WebViewClient 诊断探针（S14/S15） |
| `app/encv-mobile/plugin-simverse/src/main/java/com/encvgo/plugin/simverse/SimVerseWebChromeClient.kt` | WebChromeClient 诊断探针（S16 console） |
| `app/encv-mobile/plugin-simverse/src/main/java/com/encvgo/plugin/simverse/SimVerseEmbedWebView.kt` | WebView 初始化 + 诊断状态单例（`SimVerseWebViewDiagnostic`） |
| `app/encv-mobile/plugin-simverse/src/main/java/com/encvgo/plugin/simverse/SimVerseActivity.kt` | 原生悬浮诊断按钮实现（白屏也能点） |

---

> 沉淀：2026-07-07（SimVerse 真机调试实战）
> 更新：2026-07-08（新增 WebView 层诊断 S14-S16 + 原生悬浮按钮模式 + file:// CORS 白屏案例）
