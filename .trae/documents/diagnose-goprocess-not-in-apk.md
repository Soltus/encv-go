# 修复：Go 进程启动成功但不响应 health（诊断信息全在 Logcat 但看不到）

## 日志分析

```
[14:16:00] {"port":0,"error":"stopped"}     ← stop 成功
[14:16:26] {"port":0,"error":"timeout"}      ← 30秒后超时（无 start_failed！）
[14:16:26] ERROR ...failed: Backend failed to start  ← lastStartError==null fallback
```

**事实链**：
- ❌ 没有 `"start_failed"` → `ProcessBuilder.start()` **没抛异常**
- ❌ `lastStartError == null` → 走的是 **timeout 分支**（L144-148），不是 catch 分支
- ✅ 进程**成功启动了**，但 `/health` 30 秒内无响应

**根因**：所有诊断信息（进程 stdout/stderr、退出码）都只写了 Logcat，但 Logcat 完全看不到。需要把关键信息通过 **notifyFrontend 传到前端**。

## 修复方案

### 改动 1：MainActivity.kt — 进程输出实时转发到前端

新增变量 + 修改输出读取线程：

```kotlin
// 新增属性
private var goProcessOutput = StringBuilder()
private var goProcessExitCode: Int? = null

// 输出读取线程 — 收集输出 + 每行通知前端
Thread {
    try {
        val reader = BufferedReader(InputStreamReader(goProcess?.inputStream))
        var line: String?
        while (reader.readLine().also { line = it } != null) {
            Log.i(TAG, "[go] $line")
            goProcessOutput.append(line).append("\n")
            // 关键错误关键词立即通知前端
            if (line.contains("error", ignoreCase = true) ||
                line.contains("fatal", ignoreCase = true) ||
                line.contains("panic", ignoreCase = true)) {
                notifyFrontend(0, "go_error:$line")
            }
        }
        goProcessExitCode = goProcess?.waitFor() ?: -1
        Log.w(TAG, "[go] Exited with code: $goProcessExitCode")
    } catch (e: Exception) {
        Log.w(TAG, "Error reading process output", e)
    }
}.start()
```

### 改动 2：MainActivity.kt — timeout 时携带完整诊断信息

```kotlin
private fun waitForBackendAndNotify() {
    for (attempt in 1..60) {
        // ... 现有逻辑 ...
    }
    // timeout — 收集所有可用诊断信息
    val isAlive = goProcess?.isAlive == true
    val exitInfo = if (goProcessExitCode != null) "exit=$goProcessExitCode" else "alive=$isAlive"
    val outputPreview = goProcessOutput.takeLast(500)
    val detail = "timeout:$exitInfo|output:${if (outputPreview.isEmpty()) "(empty)" else outputPreview}"
    Log.w(TAG, "Backend timeout: $detail")
    lastStartError = detail
    notifyFrontend(0, "timeout")   // CustomEvent 保持简洁
    readyCallback?.invoke(-1)
    readyCallback = null
}
```

### 改动 3：GoProcessPlugin.kt — timeout 时也透传 lastStartError

已有改动（读取 mainActivity.lastStartError），timeout 时也会被设置，无需额外修改。

### 改动 4：stop 时重置状态

```kotlin
fun stopGoDaemon() {
    intentionallyStopped = true
    backendReady = false
    goProcess?.let {
        if (it.isAlive) it.destroyForcibly()
    }
    goProcess = null
    goProcessOutput.clear()       // 新增
    goProcessExitCode = null      // 新增
    lastStartError = null         // 新增
    notifyFrontend(0, "stopped")
}
```

## 预期效果

跑完后前端日志应变为类似：

```
[14:16:00] {"port":0,"error":"stopped"}
[14:16:05] {"port":0,"error":"go_error:config.json not found"}     ← Go 输出中的错误！
[14:16:26] ERROR ...failed: timeout:exit=1|output:config.json not found\nport already in use
```

或（如果进程还活着但端口不通）：
```
ERROR ...failed: timeout:alive=true|output:(empty)
```

这样就能直接区分：
- **exit!=0 + 有输出** → Go crash（看输出内容定位原因）
- **alive=true + 无输出** → Go 卡住/死锁
- **alive=false + exit=0** → Go 正常退出但没监听端口
