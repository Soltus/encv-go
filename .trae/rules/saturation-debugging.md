# 饱和调试规则（Saturation Debugging）

> **核心思想：不猜问题，让运行时暴露问题。** 一次构建，饱和覆盖，收集足够信息。CI 构建是宝贵资源，没时间一个个猜。

> **完整内容 + 历史踩坑**：[详情文档](../rule-library/saturation-debugging.md)

---

## 一、铁律

### 1.1 不猜问题

遇到 bug 时，**禁止**凭直觉修改代码然后"试试看"。必须先加诊断入口，让运行时告诉你问题在哪。

**❌ 错误**：看到安装卡住 → 猜是广播问题 → 改广播标志 → 试了不行 → 再猜 → 再改
**✅ 正确**：看到安装卡住 → 加诊断按钮覆盖安装全链路 → 运行一次 → 从日志定位根因

### 1.2 一次构建饱和覆盖

所有诊断代码必须一次性加入，一次 CI 构建验证所有路径。不要分多轮提交。

### 1.3 每个诊断弹窗必须有复制按钮

诊断信息必须可复制，否则用户只能截图，无法有效反馈。

- Ionic alertController 的 handler 必须 `return false` 防止点击复制后弹窗关闭
- 复制内容使用 `navigator.clipboard.writeText()`
- 复制成功/失败都要有 toast 提示

### 1.4 应用内日志缓冲（⚠️ Android 14+）

Android 14+ 应用没有 `READ_LOGS` 权限，`logcat` 命令可能返回空。必须维护应用内日志缓冲：

- `GoProcessPlugin.appLogBuffer`：ConcurrentLinkedQueue，最大 3000 条
- `GoProcessPlugin.appLog(level, tag, msg)`：同时写 Log.x() 和缓冲
- `exportLogs()` 优先使用 appLogBuffer，logcat 作为补充

### 1.5 并行调试原则（Parallel Debugging）

> **当有多种技术方案可选时，应并行实施所有可行方案，通过前端配置/选项切换来选择方案，而非逐一尝试。**
> **这样 CI 一次构建即可验证所有方案的编译正确性和基本功能，避免反复提交浪费 CI 资源和上下文切换成本。**

**适用场景**：
1. 多种 UI 实现方案（Activity / Fragment / ComposeView）
2. 多种数据源方案（本地 / 网络 / 缓存）
3. 多种渲染方案（Canvas / SVG / WebGL）

**实施规范**：
- 每个方案**独立代码路径**，不互相依赖（一个方案崩溃不影响其他方案）
- **前端提供统一切换入口**（Settings.vue select），选项旁显示方案状态标签
- 后端根据 mode 参数分发到不同实现
- 日志中**必须打印当前使用的方案名**（如 `[ModeC-Activity]`），便于 logcat 过滤定位
- experimental 方案标记为默认不启用，但代码必须编译通过

**反模式（禁止）**：
- ❌ 先做 A → CI → 发现不行 → 改 B → 再 CI → 再改 C（浪费 N 次 CI）
- ❌ 只实现一种"最优方案"，其他方案等出问题再考虑
- ✅ 一次提交包含全部方案代码，CI 通过后用户自由切换测试

**实战案例 — MPV 播放器三轨并行** → 详见 [详情文档 §1.5](../rule-library/saturation-debugging.md#15-并行调试原则parallel-debugging)

---

## 二、诊断流程

### 2.1 标准流程

```
发现 Bug
  → 在相关页面加诊断按钮（@PluginMethod / 前端按钮）
  → 诊断按钮覆盖全链路，每步记录状态
  → 一次构建 → 用户运行 → 从诊断日志定位根因
  → 修复根因 → 移除或保留诊断按钮
```

### 2.2 诊断入口模板

**Android @PluginMethod**：
```kotlin
@PluginMethod
fun debugXxxFlow(call: PluginCall) {
    val steps = mutableListOf<String>()
    val results = JSObject()

    steps.add("=== 关键组件状态 ===")
    // 检查所有相关组件是否初始化
    steps.add("1. XxxManager.isInitialized = ...")

    steps.add("=== CapacitorPlugin requestCodes ===")
    // 检查 requestCodes 是否正确声明
    val annotation = this.javaClass.getAnnotation(CapacitorPlugin::class.java)
    steps.add("2. requestCodes = ${annotation?.requestCodes?.toList()}")

    steps.add("=== 关键路径可达性 ===")
    // 检查 Activity/Service 是否可解析
    // 检查文件是否存在
    // 检查权限是否授予

    results.put("debugLog", steps.joinToString("\n"))
    call.resolve(results)
}
```

**前端诊断按钮 + 弹窗 + 复制**：详见 [详情文档 §2.2](../rule-library/saturation-debugging.md#22-诊断入口模板)

---

## 三、常见陷阱速查

| # | 陷阱 | 修复 |
|---|------|------|
| 3.1 | `@CapacitorPlugin` 没声明 `requestCodes` → `onActivityResult` 永远不调 | 加 `requestCodes = [REQUEST_CODE_xxx]` |
| 3.2 | Android 13+ `RECEIVER_NOT_EXPORTED` 收不到隐式广播 | 改用 `startActivityForResult` 或 `RECEIVER_EXPORTED + setPackage` |
| 3.3 | 非 Activity Context 启 Activity 缺 `FLAG_ACTIVITY_NEW_TASK` | Plugin 用 `activity` 属性（非 `context`） |
| 3.4 | Android 14+ `logcat` 返回空（无 READ_LOGS 权限） | 用 `--pid=<PID>` + 应用内 appLogBuffer 兜底 |
| 3.5 | Ionic alertController 复制按钮关弹窗 | handler 末尾加 `return false` |

**完整 3.1-3.5 实战场景 + 代码修复** → 详见 [详情文档 §三](../rule-library/saturation-debugging.md#三常见陷阱本项目已踩过的坑)

---

## 四、本项目已实现的诊断入口

| 入口 | 位置 | 覆盖范围 |
|------|------|----------|
| `debugInstallFlow()` | GoProcessPlugin | PluginManager 状态、requestCodes、Activity 解析、APK 文件、权限 |
| `🔧 饱和调试：测试安装流程` | ExtensionsPage.vue | 调用 debugInstallFlow() 并显示结果 |
| `exportLogs()` | GoProcessPlugin | app_log + logcat + go_backend + devlogs + device_info |
| `saveDevLogs()` | GoProcessPlugin | 保存前端 DevLogs 到文件 |

**日志导出文件清单 + 关键方法** → 详见 [详情文档 §四](../rule-library/saturation-debugging.md#四日志导出规范)

---

## 五、引用其他规则

- [verification-discipline.md](./verification-discipline.md) — 日志排查纪律（先读后猜）
- [capacitor.md](./capacitor.md) — Capacitor 桥接相关陷阱

> 拆分：2026-06-11
