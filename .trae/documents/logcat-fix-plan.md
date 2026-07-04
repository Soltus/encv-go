# 修复 Logcat 日志集成

## 核心诊断

**唯一关键问题：LogcatActivity 从未被启动**

```
项目依赖: com.github.getActivity:Logcat:13.0 (debugImplementation) ✅ 你在用这个库看日志
Manifest:  LogcatActivity 声明 (tools:node="replace") ✅ debug 构建正常合并
代码:    没有任何地方 startActivity(LogcatActivity) ❌ ← 这是 bug！

当前 "查看日志" 按钮调用的是:
  openLogViewer() → 写文本文件 → ACTION_VIEW 打开文本 → 内容为空 → 啥也看不到

应该调用的是:
  startActivity(LogcatActivity) → 启动 logcat 可视化查看器 → 实时浏览系统日志 ✅
```

**次要问题**：
- AppLogger 缓冲区几乎为空（PlayerEntry/EncvHostActivity/PluginLifecycleEngine 全部只用 android.util.Log 不写 AppLogger）
- Settings DevTools 入口 Web 端无 isNative 守卫

---

## 修复方案

### Task 1: GoProcessPlugin 新增 launchLogcatActivity() 方法

```kotlin
@PluginMethod
fun launchLogcatActivity(call: PluginCall) {
    try {
        val intent = Intent().apply {
            setClassName(context.packageName, "com.hjq.logcat.LogcatActivity")
            if (context !is android.app.Activity) addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        context.startActivity(intent)
        call.resolve(JSObject().apply { put("success", true); put("launched", true) })
        AppLogger.log("I", TAG, "launchLogcatActivity: started successfully")
    } catch (e: Exception) {
        AppLogger.log("E", TAG, "launchLogcatActivity failed: ${e.message}")
        call.resolve(JSObject().apply {
            put("success", false)
            put("error", "无法启动 LogcatActivity")
            put("errorDetail", e.message ?: "Unknown")
        })
    }
}
```

### Task 2: GoProcess.ts + web.ts 前端封装

```typescript
// GoProcess.ts
export async function launchLogcatActivity(): Promise<{ success: boolean; error?: string }> {
  try {
    await GoProcess.launchLogcatActivity()
    return { success: true }
  } catch (e) {
    return { success: false, error: String(e) }
  }
}

// web.ts interface + stub
launchLogcatActivity(): Promise<{ success: boolean; error?: string }>
async launchLogcatActivity() { return { success: false, error: 'Native only' } }
```

### Task 3: DevToolsDetail.vue 主按钮改为启动 LogcatActivity

```vue
<!-- Before -->
<ion-button @click="handleOpenLogViewer" ... >{{ t('devtools.openLogViewer') }}</ion-button>

<!-- After: 主按钮启动 LogcatActivity -->
<ion-button @click="handleLaunchLogcat" color="primary" fill="outline">
  {{ t('devtools.viewLogcat') }}
</ion-button>
<!-- 保留原按钮作为次要入口（查看应用内缓冲日志） -->
<ion-button @click="handleOpenLogViewer" size="small" fill="clear">
  {{ t('devtools.openAppLog') }}
</ion-button>
```

新增 i18n key:
- `devtools.viewLogcat`: '查看 Logcat' / 'View Logcat'
- `devtools.openAppLog`: '应用内日志' / 'App Internal Log'

### Task 4: LogBridge — 让 LogcatActivity 中能看到更多日志

新建 LogBridge.kt，双写 Log + AppLogger。替换 PlayerEntry/EncvHostActivity/PluginLifecycleEngine/MpvEmbedService/GoProcessPlugin.openPlayer 中的 `android.util.Log` 为 `LogBridge`。

这样你在 LogcatActivity 中用 tag 过滤 `ENCV-go` / `EncvHostActivity` / `PlayerEntry` 就能看到完整的 MPV 播放链路日志。

### Task 5: Settings DevTools 入口加 v-if="isNative()"

---

## 改动文件清单

| 文件 | 改动 |
|------|------|
| [GoProcessPlugin.kt](...) | 新增 `launchLogcatActivity()` |
| [GoProcess.ts](...) | 新增 `launchLogcatActivity()` 封装 |
| [web.ts](...) | 类型定义 + stub |
| [DevToolsDetail.vue](...) | 主按钮改为启动 LogcatActivity + i18n |
| [useI18n.ts](...) | 2 个新 key |
| `LogBridge.kt` (**新建**) | 统一日志桥接 |
| [PlayerEntry.kt](...) | `Log` → `LogBridge` |
| [EncvHostActivity.kt](...) | 同上 |
| [PluginLifecycleEngine.kt](...) | 同上 |
| [MpvEmbedService.kt](...) | 同上 |
| [Settings.vue](...) | DevTools 入口 `v-if="isNative()"` |

## 不改动的

- `libs.versions.toml` logcat 版本 — **不动**
- `build.gradle.kts` `debugImplementation(libs.logcat)` — **不动**，完全正确
- `AndroidManifest.xml` LogcatActivity 声明 — **不动**，debug 构建正常

## 验证

- [ ] 点"查看 Logcat" → **LogcatActivity 启动** → 能看到实时系统日志
- [ ] 用 MPV 播放视频 → LogcatActivity 中过滤 `PlayerEntry` 或 `ENCV-go` → 看到完整链路日志
- [ ] CI Warning 保持现状（可接受）不影响功能
- [ ] vue-tsc + vite build 通过
