# SimVerse 安卓端架构调研 v0.1

> **调研日期**：2026-07-03
> **目标**：评估 SimVerse（模拟世界引擎）在安卓端的运行架构，包括存储预占用、后台存活、WorkManager 集成、独立 Activity 横屏模式，以及与特色微服务 agent toolcall 的兼容性。
> **基于现有架构**：EncvGo 应用（Capacitor + Ionic Vue + Go 后端前台服务 + ComboLite 插件框架）

---

## 一、现状总览

### 1.1 当前安卓架构

| 组件 | 现状 | 文件 |
|------|------|------|
| 主 Activity | `MainActivity`（BridgeActivity / Capacitor），单 WebView | [MainActivity.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt) |
| 播放器 Activity | `PlayerActivity`（独立 AppCompatActivity） | [PlayerActivity.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivity.kt) |
| Go 后端服务 | `EncvGoService`（前台服务，`specialUse` 类型，无 6h 限制） | [EncvGoService.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) |
| WorkManager | `EncvTaskCancelWorker`（任务取消持久化） + `GoProcessRestartReceiver`（重启通知） | [workers/](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/workers/) |
| 特色微服务内核 | `internal/kernel/`（Lifecycle 启停编排 + ToolWrapper agent tool 拦截） | [kernel/](file:///workspace/internal/kernel/) |
| SimVerse 后端 | `internal/simverse/`（FractalWorld + Chronicle + NPC CatchUp） | [simverse/](file:///workspace/internal/simverse/) |

### 1.2 关键约束

| 资源 | 约束值 | 来源 |
|------|--------|------|
| 平均内存（PSS） | < 100 MB | [00-overview.md](file:///workspace/docs/simulation/00-overview.md#L33-L34) |
| 峰值内存（PSS） | < 150 MB | 同上 |
| CPU 空闲模拟 | < 5%（8 核） | 同上 |
| NPC 规模目标 | 10,000,000（千万级） | 同上 |
| 世界数据持久化 | 99% 实体常驻磁盘（SQLite） | 同上 |

---

## 二、存储预占用方案

### 2.1 问题背景

SimVerse 世界数据（NPC 二进制 + 编年史 + 组织数据）可能达到数百 MB 甚至数 GB。安卓设备存储空间会被其他应用（微信、照片、视频等）动态消耗，如果不预留空间，世界运行到中途可能遭遇 `disk full` 导致数据损坏或进程崩溃。

### 2.2 方案对比

| 方案 | 原理 | 优点 | 缺点 | 适用场景 |
|------|------|------|------|----------|
| **A. StorageManager.allocateBytes()** | Android 8.0+ API，系统级分配，自动清理其他 app 缓存来腾空间 | 官方 API、可清理其他应用缓存、精确到字节 | 只能在应用私有目录生效、API 26+ | 主推荐方案 |
| **B. 占位文件 (Placeholder File)** | 提前创建一个大文件（如 500MB .reserve），需要空间时 truncate 释放 | 简单可靠、兼容所有 Android 版本、对其他 app 是硬占用 | 空间被"锁死"不能被系统灵活调度、用户可能困惑 | 兼容方案 / fallback |
| **C. 定期检查 + 优雅降级** | `StatFs` 监控剩余空间，低于阈值时暂停世界运行并提示 | 不占用额外空间、实现简单 | 被动防御，空间被吃完后才反应 | 辅助方案，必选 |
| **D. 分区存储 + 预留块** | EXT4 保留块（tune2fs -m） | 文件系统级硬保障 | 需要 root / 出厂配置，普通应用做不到 | 不适用 |

### 2.3 推荐方案：A + B + C 三层防御

```
┌─────────────────────────────────────────────────────────┐
│  Layer 3: 实时监控 + 优雅降级（必选）                    │
│  - 每 N tick 检查 StatFs.getAvailableBytes()            │
│  - 低于 RED 阈值（如 50MB）：暂停世界 + 弹通知           │
│  - 低于 YELLOW 阈值（如 200MB）：降低事件生成速率        │
└─────────────────────────────────────────────────────────┘
                           ▲
                           │
┌─────────────────────────────────────────────────────────┐
│  Layer 2: 占位文件兜底（兼容 API < 26）                  │
│  - 首次启动创建 world_reserve.tmp（默认 500MB）         │
│  - 当 getAllocatableBytes() 不足时，先释放占位文件       │
│  - 空间恢复后重新创建占位文件                            │
└─────────────────────────────────────────────────────────┘
                           ▲
                           │
┌─────────────────────────────────────────────────────────┐
│  Layer 1: StorageManager.allocateBytes()（主方案）      │
│  - 世界创建前调用 getAllocatableBytes() 预估可用空间     │
│  - 不足时启动 ACTION_MANAGE_STORAGE 让用户清理           │
│  - 世界数据库文件用 allocateBytes() 预分配               │
└─────────────────────────────────────────────────────────┘
```

### 2.4 关键代码模式（参考）

```kotlin
// StorageManager 预分配（API 26+）
val storageManager = context.getSystemService(StorageManager::class.java)
val uuid = storageManager.getUuidForPath(filesDir)
val allocatable = storageManager.getAllocatableBytes(uuid)

if (allocatable < REQUIRED_WORLD_SIZE) {
    // 启动系统存储管理界面
    val intent = Intent(StorageManager.ACTION_MANAGE_STORAGE).apply {
        putExtra(StorageManager.EXTRA_UUID, uuid)
        putExtra(StorageManager.EXTRA_REQUESTED_BYTES, REQUIRED_WORLD_SIZE)
    }
    startActivity(intent)
} else {
    // 为世界数据库预分配空间
    val dbFile = File(filesDir, "simverse/world.db")
    dbFile.parentFile?.mkdirs()
    val fd = ParcelFileDescriptor.open(
        dbFile, 
        ParcelFileDescriptor.MODE_READ_WRITE or ParcelFileDescriptor.MODE_CREATE
    )
    storageManager.allocateBytes(fd.fileDescriptor, REQUIRED_WORLD_SIZE)
    fd.close()
}
```

### 2.5 SimVerse 存储估算

| 数据项 | 单条大小 | 数量 | 预估总大小 |
|--------|----------|------|------------|
| NPC 二进制数据 | ~200 B | 10,000,000 | ~2 GB |
| 编年史事件 | ~64 B | 1,000,000 | ~64 MB |
| 组织数据 | ~512 B | 100,000 | ~50 MB |
| 索引（name, region 等） | 可变 | — | ~200 MB |
| **合计（百万 NPC）** | — | — | **~2.3 GB** |
| **合计（十万 NPC）** | — | — | **~250 MB** |

> **策略**：按世界规模动态计算所需空间。默认 10 万 NPC（~250 MB）可在大多数设备上运行；百万级以上需要用户确认并预留足够空间。

### 2.6 存储空间耗尽的处理流程

```
检测到空间不足（< RED 阈值）
    │
    ├─ 立即暂停世界 tick（停止写入）
    ├─ 持久化当前内存中的脏数据到磁盘（最后一次成功写入）
    ├─ 记录错误日志（最后一次成功的 tick 编号）
    ├─ 发送通知 + 前台服务更新通知
    └─ 前端弹窗提示：
         "存储空间不足，世界已暂停运行。
          请清理至少 X MB 空间后点击继续。"
              │
              ▼
    用户清理空间后返回
         │
         ├─ 重新检查空间
         ├─ 充足：恢复世界运行（从上次持久化的 tick 继续）
         └─ 不足：继续提示
```

### 2.7 数据完整性保障

- **WAL 模式**：SQLite 使用 WAL（Write-Ahead Logging）模式，写入崩溃不损坏已有数据
- **Checkpoint 机制**：每 N tick 做一次完整 checkpoint，确保数据落盘
- **世界版本号**：每次成功持久化递增 `world_persist_version`，启动时检测版本一致性
- **占位文件释放顺序**：先释放占位文件 → 再写数据 → 写成功后尝试重新建占位；写失败则回滚，数据仍完整

---

## 三、后台强杀与恢复机制

### 3.1 安卓后台杀进程的场景

| 场景 | 触发条件 | 概率 | 数据风险 |
|------|----------|------|----------|
| **系统内存回收 (LMK)** | 设备内存不足，按 oom_adj 优先级杀进程 | 中高 | 内存数据丢失（但磁盘数据安全） |
| **用户手动停止** | 设置 → 应用 → 强制停止 | 低 | 全停，下次冷启动 |
| **厂商定制省电** | 小米/华为/OPPO 等省电策略 | 中 | 同 LMK |
| **Android 15+ dataSync FGS 6h 限制** | dataSync 类型前台服务运行超 6h | 已规避 | 已改用 `specialUse` |
| **设备重启** | 用户关机 / 系统更新 | 低 | 全停 |

### 3.2 现有机制评估

#### ✅ 已有能力
1. **EncvGoService 前台服务**（`specialUse` 类型）：不受 6h 上限约束，持续运行
2. **WorkManager 持久化任务**：`EncvTaskCancelWorker` 证明 WorkManager 已集成，可用于持久化世界状态
3. **Go 后端独立进程**：Go 进程与 UI 进程分离，UI 被杀不影响后端（只要前台服务还在）
4. **ComboLite 插件框架**：插件服务独立生命周期管理

#### ❌ 当前缺失
1. **世界运行状态持久化**：SimVerse `FractalWorld` 目前是纯内存对象，被杀后从 0 开始
2. **自动恢复机制**：被杀后 WorkManager 可以拉起进程，但世界需要从磁盘 reload
3. **tick 级 checkpoint**：需要定义持久化频率和恢复点

### 3.3 推荐方案：三层存活保障

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 1: 前台服务 + 高优先级（主运行态）                    │
│  - EncvGoService 保持 foregroundServiceType=specialUse      │
│  - 通知栏显示世界运行状态（tick / NPC 数 / 时代）             │
│  - 用户可通过通知快速进入世界页面                            │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼ 被系统杀掉
┌─────────────────────────────────────────────────────────────┐
│  Layer 2: WorkManager 定期心跳 + 自动恢复                    │
│  - PeriodicWorkRequest（15 分钟最小间隔）检查世界进程状态    │
│  - 进程不在 → 启动 EncvGoService → 加载世界 → 继续运行      │
│  - 利用 WorkManager 内部 SQLite 持久化调度，重启后自动恢复   │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼ WorkManager 也受限（国产 ROM）
┌─────────────────────────────────────────────────────────────┐
│  Layer 3: 用户打开 App 时热恢复                              │
│  - 启动时检测是否有未完成的世界快照                          │
│  - 有 → 自动加载并继续运行                                  │
│  - 显示"世界已从第 X tick 恢复"提示                         │
└─────────────────────────────────────────────────────────────┘
```

### 3.4 世界持久化与恢复设计

#### 3.4.1 持久化内容

```go
// WorldSnapshot 世界快照（持久化到 SQLite）
type WorldSnapshot struct {
    WorldID       uint64    // 世界唯一 ID
    Tick          uint32    // 当前 tick 数
    CurrentEra    uint16    // 当前时代
    ConfigTier    string    // 性能配置 (low/mid/high)
    NPCCount      uint32    // NPC 总数
    EventCount    uint32    // 编年史事件总数
    CheckpointAt  time.Time // 快照时间
    // 热数据（内存中的活跃 NPC）单独存储
}
```

#### 3.4.2 持久化策略

| 策略 | 频率 | 触发时机 | 成本 |
|------|------|----------|------|
| **增量 tick 记录** | 每 tick | 运行中 | 低（只写事件 + NPC 状态变化） |
| **热数据 checkpoint** | 每 100 tick | 运行中 | 中（写活跃 NPC ~1000 个） |
| **全量 snapshot** | 每 1000 tick / 退到后台 | 手动或自动 | 高（写全部索引 + 元数据） |
| **最终 snapshot** | 主动暂停 / 退出世界 | 用户操作 | 高（确保完整） |

#### 3.4.3 恢复流程

```
App 启动 / WorkManager 拉起
    │
    ▼
检测是否有世界快照？
    ├─ 否 → 全新世界 / 显示创建界面
    └─ 是 → 恢复流程：
           1. 加载 WorldSnapshot 元数据
           2. 加载 NPC 数据库（SQLite BLOB）
           3. 加载编年史索引
           4. 重建热缓存（活跃 NPC 加载到内存）
           5. 从 snapshot.tick + 1 继续运行
           6. 前端显示"已从第 X tick 恢复"
```

### 3.5 WorkManager 集成详细设计

#### 3.5.1 新增 Worker

| Worker | 类型 | 用途 | 触发 |
|--------|------|------|------|
| `SimVerseHeartbeatWorker` | Periodic (15min) | 检查世界进程状态，被杀则拉起 | 世界运行时启用，暂停时取消 |
| `SimVerseCheckpointWorker` | OneTime | 触发一次世界 checkpoint | 退到后台时延迟调度 |
| `SimVerseCleanupWorker` | OneTime | 清理旧快照 / 占位文件重整 | 手动 / 存储不足时 |

#### 3.5.2 心跳 Worker 逻辑

```kotlin
class SimVerseHeartbeatWorker(ctx: Context, params: WorkerParameters) : CoroutineWorker(ctx, params) {
    override suspend fun doWork(): Result {
        // 1. 检查世界是否应该在运行（SharedPreferences 标记）
        val prefs = applicationContext.getSharedPreferences("simverse", MODE_PRIVATE)
        val shouldRun = prefs.getBoolean("world_should_run", false)
        if (!shouldRun) return Result.success()

        // 2. 检查 Go 后端进程是否存活
        if (!EncvGoService.isRunning) {
            // 3. 启动 Go 后端服务
            val intent = EncvGoService.createIntent(
                applicationContext, 
                EncvGoService.ACTION_START, 
                "workmanager_heartbeat"
            )
            ContextCompat.startForegroundService(applicationContext, intent)
            
            // 4. 等待后端就绪（轮询最多 30s）
            // 5. 发送"恢复世界"命令给后端 HTTP API
            // 6. 成功 → success，失败 → retry（指数退避）
        }
        
        return Result.success()
    }
}
```

#### 3.5.3 与现有 WorkManager 基础设施的关系

- **复用**：已有的 `WorkManager` 依赖（`androidx.work:work-runtime-ktx`）直接用
- **复用**：已有的 `GoProcessRestartReceiver` 模式（监听 BACKEND_READY 广播）
- **新增**：`SimVerseSharedPrefs` 存储世界运行意图（`world_should_run` / `world_id` 等）

---

## 四、独立 Activity + WebView 横屏模式

### 4.1 为什么需要独立 Activity

当前 `MainActivity`（Capacitor BridgeActivity）承载了所有功能，包括：
- 文件浏览 / 任务管理 / 设置（竖屏为主）
- DevTools / 日志（竖屏）
- 未来的 SimVerse 世界面板（横屏体验更好）

SimVerse 作为一个"世界模拟器"，参考手游的横屏沉浸体验：
1. **横屏**：更宽的视野展示世界地图 / 时间线 / 双栏信息
2. **沉浸式**：隐藏状态栏和导航栏，最大化内容区域
3. **独立任务栈**：从最近任务列表看是独立的窗口（类似 PlayerActivity）
4. **独立菜单系统**：手游风格的侧边 / 底部菜单，不沿用 Ionic tab 栏

### 4.2 架构设计

```
┌──────────────────────────────────────────────────────────────┐
│ MainActivity (Capacitor)                                    │
│ ├─ Tabs: Files / Tasks / Settings                          │
│ ├─ DevTools / 日志 / 设置                                   │
│ └─ "进入模拟世界" 按钮 → 启动 WorldActivity                 │
└──────────────────────────────────────────────────────────────┘
                           │
                           ▼  startActivity
┌──────────────────────────────────────────────────────────────┐
│ WorldActivity (独立 AppCompatActivity)                      │
│ ├─ 横屏锁定 (sensorLandscape)                               │
│ ├─ 沉浸式全屏 (WindowInsetsCompat)                         │
│ ├─ 独立 WebView 实例（加载同一套前端资源）                  │
│ │   └─ 路由 /simverse-world → 世界模拟器 SPA 页面           │
│ ├─ 手游式菜单系统（原生 Drawer + Floating Buttons）        │
│ └─ taskAffinity = 独立任务栈                                │
└──────────────────────────────────────────────────────────────┘
```

### 4.3 WorldActivity 设计要点

#### 4.3.1 Manifest 声明

```xml
<activity
    android:name=".WorldActivity"
    android:exported="false"
    android:launchMode="singleTop"
    android:taskAffinity="com.encvgo.app.world.task"
    android:documentLaunchMode="always"
    android:maxRecents="1"
    android:theme="@style/AppTheme.World.Fullscreen"
    android:configChanges="orientation|screenSize|smallestScreenSize|screenLayout|uiMode|keyboardHidden"
    android:screenOrientation="sensorLandscape"
    android:label="SimVerse">
</activity>
```

参考现有 `PlayerActivity` 的 `taskAffinity` 模式：[AndroidManifest.xml#L84-L120](file:///workspace/app/encv-mobile/android/app/src/main/AndroidManifest.xml#L84-L120)

#### 4.3.2 横屏 + 沉浸式实现

```kotlin
class WorldActivity : AppCompatActivity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        
        // 沉浸式全屏（手游式体验）
        WindowCompat.setDecorFitsSystemWindows(window, false)
        val controller = WindowInsetsControllerCompat(window, window.decorView)
        controller.hide(WindowInsetsCompat.Type.systemBars())
        controller.systemBarsBehavior = 
            WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        
        // WebView 设置
        webView = WebView(this)
        setContentView(webView)
        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        webView.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        
        // 加载同一套前端，但路由到世界页面
        // 方案 A: 加载 Capacitor 同一 URL + #/simverse-world
        // 方案 B: 独立加载精简版 HTML（推荐，减少加载时间）
        webView.loadUrl("file:///android_asset/public/index.html#/simverse-world")
        
        // 桥接：复用 Capacitor 的插件系统？
        // → 方案：直接 HTTP 调用本地 Go 后端 API（和主 WebView 一样）
        // 不通过 Capacitor 插件，减少依赖
    }
    
    // 屏幕点击 → 显示/隐藏系统栏（手游式）
    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (event.action == MotionEvent.ACTION_UP) {
            toggleSystemBars()
        }
        return super.onTouchEvent(event)
    }
}
```

#### 4.3.3 与主 App 的共享

| 资源 | 共享方式 | 说明 |
|------|----------|------|
| Go 后端 API | 共享同一 `EncvGoService` | HTTP localhost 调用，两个 WebView 共用一个后端 |
| 前端代码 | 共享同一套打包资源 | 通过路由区分不同页面 |
| Capacitor 插件 | 不共享（WorldActivity 不含 Bridge） | 世界页面只用 Go API，不走 Capacitor 插件 |
| 本地存储 | 不共享 localStorage（不同 WebView 实例） | 持久化走 Go API |
| 事件通信 | 通过 Go 后端 WS 广播 | 主 App → 世界页面 走 WS |

### 4.4 手游式菜单系统

#### 4.4.1 设计参考

类似《文明》/《模拟城市》等手游的横屏 UI 布局：

```
┌─────────────────────────────────────────────────────────────────┐
│  [菜单]  模拟世界名称                    Era 3 · Tick 12,345 [⚙] │ ← 顶部状态栏
├──────────┬──────────────────────────────────────────────────────┤
│          │                                                      │
│  世界地图│            时间线 / 事件流 / 数据面板                │
│  (左栏)  │            （主内容区，WebView 渲染）                │
│          │                                                      │
│          │                                                      │
├──────────┴──────────────────────────────────────────────────────┤
│  [NPC]  [组织]  [编年史]  [经济]  [设置]  [退出]                │ ← 底部功能栏
└─────────────────────────────────────────────────────────────────┘
```

#### 4.4.2 两种实现方案对比

| 方案 | 实现方式 | 优点 | 缺点 |
|------|----------|------|------|
| **A. 纯 WebView 内实现** | 所有菜单用 Vue 组件写在前端 | 开发快、跨平台一致、复用现有组件库 | 性能稍差（但世界页本身就是 Web） |
| **B. 原生 + Web 混合** | 底部/侧边菜单用原生 Compose，中间内容 WebView | 原生流畅、手游感强 | 开发成本高、两套代码维护 |

**推荐：方案 A（纯 WebView）**

理由：
1. SimVerse 的核心体验是数据模拟，不是 3D 渲染，Web 性能够
2. 复用现有 Ionic/Vue 组件库（ion-menu / ion-tabs / ion-fab 等）
3. 横屏模式下 CSS 适配即可，无需原生开发
4. 维护成本低，一套代码多端运行

菜单布局用 CSS Grid / Flex 实现横屏专用布局，通过 `@media (orientation: landscape)` 切换样式。

### 4.5 设置页 / 日志页复用

| 页面 | 复用方式 | 说明 |
|------|----------|------|
| **设置页** | 世界内设置抽屉 → 打开相同的 Settings.vue 组件 | 直接复用，样式适配横屏 |
| **DevLogs 页** | 世界内菜单 → 打开 LogViewer.vue 组件 | 直接复用 |
| **DevTools** | （可选）世界内调起主 App 的 DevTools | 切回 MainActivity |

**实现方式**：前端路由系统本身就支持这些页面，在世界 Activity 的 WebView 里也能访问。不需要原生层做任何事情。

### 4.6 横屏进入 / 退出流程

```
主 App (竖屏)
    │
    ├─ 用户点击 "进入模拟世界"
    │
    ├─ 前端调用 JS Bridge: world.launch(worldId)
    │   ↓
    ├─ GoProcessPlugin 启动 WorldActivity
    │   ↓
    ├─ WorldActivity 创建 → 横屏 → 沉浸式 → 加载 WebView
    │   ↓
    ├─ WebView 加载 /simverse-world 路由
    │   ↓
    └─ 前端连接 Go 后端 WS → 世界运行中
          │
          ├─ 用户点击 "退出世界"
          │   ↓
          ├─ 前端调用 world.exit()
          │   ↓
          ├─ 触发一次 checkpoint（持久化）
          │   ↓
          └─ finish() → 返回主 App
```

---

## 五、特色微服务 Agent Toolcall 兼容性分析

### 5.1 现有架构回顾

特色微服务内核（`internal/kernel/`）的核心特性：
- **Lifecycle 启停编排**：独立的 goroutine 池管理，支持优雅启停
- **ToolWrapper**：AI agent tool 调用拦截链（鉴权 / 日志 / 限流）
- **Context 链**：gin request → agent tool → DB 共享同一 ctx 和 RequestID
- **配置驱动**：`MaxToolCallsPerTurn` 等参数可配置

代码位置：
- [kernel.go](file:///workspace/internal/kernel/kernel.go)
- [tool_wrapper.go](file:///workspace/internal/kernel/tool_wrapper.go)
- [pool.go](file:///workspace/internal/kernel/pool.go)

### 5.2 SimVerse 与 Kernel 的关系

```
┌──────────────────────────────────────────────────────────┐
│ EncvGoService (Go 后端进程)                             │
│                                                         │
│  ┌──────────────┐  HTTP API  ┌──────────────────────┐  │
│  │ 特色微服务   │◄──────────►│  SimVerse 世界引擎   │  │
│  │ 内核 (Kernel)│            │  (FractalWorld)      │  │
│  │  - Agent     │            │  - NPC 系统          │  │
│  │  - ToolCalls │            │  - 编年史            │  │
│  │  - 生命周期   │            │  - 世界事件          │  │
│  └──────────────┘            └──────────────────────┘  │
│         ▲                              ▲               │
│         │ 同一进程内函数调用             │               │
│         └──────────────────────────────┘               │
│                    直接调用 / 事件总线                   │
└──────────────────────────────────────────────────────────┘
```

### 5.3 兼容性评估

| 维度 | 兼容性 | 说明 |
|------|--------|------|
| **进程模型** | ✅ 完全兼容 | 都在同一个 Go 进程（EncvGoService）内，函数级调用 |
| **生命周期** | ✅ 兼容 | Kernel Lifecycle 可管理 SimVerse 世界的启停（类似管理其他微服务） |
| **ToolCall 模型** | ✅ 兼容 | SimVerse 可以注册为一组 Tool（如 `world_tick` / `npc_query` / `chronicle_search`） |
| **Context 链** | ✅ 兼容 | ToolWrapper 把 agent context 传入 SimVerse 调用 |
| **存储** | ✅ 兼容 | 共用同一套 SQLite 驱动（glebarez/sqlite / libsql） |
| **后台运行** | ✅ 兼容 | 都依赖 EncvGoService 前台服务保活 |
| **WorkManager** | ✅ 兼容 | Kernel 的启停也走同一套 WorkManager 心跳恢复机制 |

### 5.4 Agent 操作世界的 Tool 示例

SimVerse 可以向 Kernel 注册以下 Tool：

```go
// 注册为特色微服务的 tool
func RegisterSimverseTools(k *kernel.Kernel) {
    k.RegisterTool(kernel.ToolDef{
        Name:        "world_get_state",
        Description: "获取当前世界状态（tick、时代、NPC数等）",
        Handler:     toolWorldGetState,
    })
    
    k.RegisterTool(kernel.ToolDef{
        Name:        "world_query_npcs",
        Description: "按条件查询 NPC 列表",
        Handler:     toolWorldQueryNPCs,
    })
    
    k.RegisterTool(kernel.ToolDef{
        Name:        "world_chronicle_search",
        Description: "搜索编年史事件",
        Handler:     toolWorldChronicleSearch,
    })
    
    k.RegisterTool(kernel.ToolDef{
        Name:        "world_control",
        Description: "控制世界（暂停/继续/调整速度/单步）",
        Handler:     toolWorldControl,
    })
}
```

### 5.5 结论：满足特色微服务 agent toolcall 需求

**✅ 结论：完全满足，且是天作之合。**

理由：
1. **架构对齐**：SimVerse 本身就是一个"特色微服务"——独立数据、独立逻辑、通过 API 对外暴露能力
2. **Tool 注册简单**：SimVerse 的 API 直接包装成 Kernel Tool，agent 可以像调用其他工具一样操作世界
3. **Agent 观察世界**：agent 可以通过 tool 查询世界状态、搜索编年史、观察 NPC 行为
4. **Agent 影响世界**：agent 可以通过 tool 调整世界参数、触发事件、控制运行速度
5. **统一生命周期**：Kernel Lifecycle 管理 SimVerse 的启停，和其他微服务一致
6. **与现有 WorkManager 恢复机制兼容**：世界被杀后和其他微服务一起恢复

---

## 六、实施路线图（建议）

### Phase 1：存储基础（先做）
- [ ] `StorageManager.allocateBytes()` 世界存储预分配
- [ ] 占位文件 fallback 机制
- [ ] 实时空间监控 + 优雅降级（YELLOW/RED 阈值）
- [ ] 世界数据库 WAL + checkpoint 机制

### Phase 2：持久化与恢复
- [ ] `WorldSnapshot` 数据结构 + 持久化 API
- [ ] 热数据 checkpoint（每 100 tick）
- [ ] 全量 snapshot（每 1000 tick / 退后台）
- [ ] 启动时自动检测并恢复世界

### Phase 3：WorkManager 后台保活
- [ ] `SimVerseHeartbeatWorker` 心跳检测 + 自动拉起
- [ ] `SimVerseCheckpointWorker` 退后台延迟 checkpoint
- [ ] SharedPreferences 存储世界运行意图
- [ ] 与现有 WorkManager 基础设施整合

### Phase 4：独立 WorldActivity
- [ ] `WorldActivity` 横屏 + 沉浸式实现
- [ ] 独立 WebView 加载世界页面
- [ ] 手游式菜单系统（前端 Vue 实现）
- [ ] 进入 / 退出世界的完整流程
- [ ] 设置页 / 日志页复用验证

### Phase 5：Kernel Tool 集成
- [ ] SimVerse 注册为 Kernel 微服务
- [ ] 实现 `world_get_state` / `world_query_npcs` / `world_chronicle_search` 等 Tool
- [ ] Agent 通过 Tool 观察和操作世界的端到端验证

---

## 七、待确认问题

以下问题需要与您确认后再进入详细设计：

1. **世界规模默认值**：默认创建世界时，NPC 数量定多少？（1 万 / 10 万 / 100 万）影响存储需求和性能
2. **横屏是否强制**：世界页面是否强制横屏？还是允许用户切换横竖屏？
3. **独立 Activity vs 单 Activity**：是否确定做独立 Activity？还是在 MainActivity 内通过横屏切换实现？
4. **后台运行策略**：用户关闭 App 后，世界是否继续在后台运行？还是只在前台运行？（影响电量消耗）
5. **与 Kernel 的集成优先级**：SimVerse 注册为 Kernel Tool 是近期目标还是远期目标？

---

## 八、引用

### 相关代码

- [EncvGoService.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) — 现有前台服务实现
- [MainActivity.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/MainActivity.kt) — 现有主 Activity
- [PlayerActivity.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/PlayerActivity.kt) — 独立 Activity 参考
- [workers/EncvTaskCancelWorker.kt](file:///workspace/app/encv-mobile/android/app/src/main/java/com/encvgo/app/workers/EncvTaskCancelWorker.kt) — 现有 WorkManager 参考
- [kernel/](file:///workspace/internal/kernel/) — 特色微服务内核
- [simverse/](file:///workspace/internal/simverse/) — SimVerse 世界引擎

### 外部参考

- Android Developers: [支持长时间运行的 worker](https://developer.android.com/guide/background/persistent/how-to/long-running)
- Android Developers: [StorageManager.getAllocateBytes()](https://developer.android.com/reference/android/os/storage/StorageManager#getAllocatableBytes(java.util.UUID))
- Android Developers: [沉浸式全屏模式](https://developer.android.com/training/system-ui/immersive)
- Android Developers: [应用专属存储空间](https://developer.android.com/training/data-storage/app-specific)
