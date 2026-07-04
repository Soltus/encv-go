## Summary

- 选定方案：采用 `2.md` 为主的 `Android 12+ Foreground Service` 重构方案，并吸收 `3.md` 中“前端服务控制”和“状态反馈”的必要部分。
- 收敛原则：继续采用 Capacitor 的前提是 `Android 原生复杂度严格封装在 overlay 层`，不把 Service 方案扩散成“全前端重构 + 全链路改造”。
- 目标定义：
  - 后端进程脱离 `MainActivity` 生命周期，改由常驻前台服务持有和管理。
  - 支持第三方快速调用，且同时支持 `自定义 Scheme` 与 `显式 Intent` 两种入口。
  - 第三方触发结果同时通过 `前台页面展示` 和 `Android Broadcast` 两种方式回传。
  - 默认运行策略为 `常驻 + 可手动关闭`。
- 范围收敛：只考虑 `Android 12+`，不再为更早 Android 版本保留兼容分支。
- 低侵入边界：
  - Web UI、Vue 路由、页面结构、业务 API 不重做。
  - `src/plugins/GoProcess.ts` 对外接口尽量保持不变。
  - 前端只消费已有或兼容扩展的事件，不引入新的一套控制架构。

## Current State Analysis

### 现有实现

- Android 原生定制目前仅存在于 overlay 目录：
  - `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/MainActivity.kt`
  - `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt`
  - `app/encv-mobile/android-overlay/app/src/main/res/xml/network_security_config.xml`
- `MainActivity.kt` 当前同时承担了过多职责：
  - 注册 `GoProcessPlugin`
  - 从 assets 解压 `encv-go`
  - 在 `filesDir`/`cacheDir`/`externalFilesDir` 多目录尝试执行二进制
  - 持有 `Process`
  - 用 `waitForBackendAndNotify()` 对 `/health` 做最多 30 秒、每 500ms 一次的轮询
  - 通过 `window.dispatchEvent('encv:backend-ready')` 通知前端
- Capacitor 原生桥接 `GoProcessPlugin.kt` 已支持：
  - `restart`
  - `stop`
  - `getStatus`
  - 通知权限 / 存储权限查询和请求
- 前端桥接与页面已经存在：
  - `app/encv-mobile/src/plugins/GoProcess.ts`
  - `app/encv-mobile/src/plugins/web.ts`
  - `app/encv-mobile/src/composables/useServerStatus.ts`
  - `app/encv-mobile/src/views/ServerDetail.vue`
- `post-cap-sync.mjs` 已经会把 `MainActivity.kt` 和 `GoProcessPlugin.kt` 覆盖到 Android 工程，但当前不会生成：
  - `EncvGoService.kt`
  - 自定义 `AndroidManifest.xml`
  - 外部调用入口清单
- 仓库当前没有已有的 deep link、scheme、intent-filter、外部唤起协议实现。
- `capacitor.config.ts` 当前只有基础配置：
  - `appId = com.encvgo.app`
  - `androidScheme = https`
  - 没有为第三方入口预留协议或路由适配。

### 已确认的问题

- 当前后端进程与 `MainActivity` 绑定，不满足“稳定后台运行”的目标。
- 当前启停链路只适用于 App 内部调用，不满足“第三方快速调用”的目标。
- 当前没有外部触发协议，第三方既不能用自定义 URL，也不能用显式 Intent 精准控制服务。
- 当前没有 Broadcast 形式的结果回传，外部 Android 调用方无法无 UI 地监听结果。
- 当前服务化所需的关键基础设施全部缺失：
  - `Foreground Service`
  - `AndroidManifest` 权限与 service 声明
  - 外部 `intent-filter`
  - 外部动作协议与 extras 约定
  - 服务状态广播规范
- 如果为实现 Service 而同步重写前端状态层、页面层和路由层，Capacitor 的“低侵入”优势会被抵消。

### 三份方案与现状匹配度

- `1.md`：
  - 适合短期稳定性补丁
  - 但不能满足“稳定后台运行”和“第三方快捷调用”的核心目标
- `2.md`：
  - 是当前目标最匹配的基础方案
  - 能把进程生命周期从 `MainActivity` 迁移到系统级服务
- `3.md`：
  - 其中的前端控制链路和结果反馈思路可作为 Service 方案的上层配套
  - 但不能单独作为底层方案

## Proposed Changes

### 方案结论

- 选定方案：`Service 化重构方案`
- 服务策略：`Foreground Service + START_STICKY + 用户可手动停止`
- 外部调用：
  - 支持 `encvgo://` 自定义 Scheme
  - 支持显式 `Intent action`
- 结果回传：
  - 对前端页面继续发 `window.dispatchEvent('encv:backend-ready')`
  - 对 Android 外部调用方发应用内限定的结果 Broadcast
- 平台范围：仅支持 `Android 12+`
- 侵入控制原则：
  - 必改：Android overlay、Manifest、post-cap-sync、原生插件底层实现
  - 可少改：前端状态消费层
  - 不改或尽量不改：Vue 页面结构、路由、业务 API、全局应用入口架构

### 文件级改动计划

#### 1. `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/EncvGoService.kt`

- 目标：新增真正持有 Go 进程的前台服务，成为唯一后端生命周期管理者。
- 具体修改：
  - 新建 `EncvGoService.kt`
  - 在 `companion object` 中定义统一动作常量：
    - `ACTION_START`
    - `ACTION_STOP`
    - `ACTION_RESTART`
    - `ACTION_STATUS`
    - `ACTION_EXTERNAL_START`
    - `ACTION_EXTERNAL_RESTART`
  - 在 `companion object` 中定义统一广播常量：
    - `BROADCAST_BACKEND_READY`
    - `BROADCAST_BACKEND_STATUS`
    - `BROADCAST_EXTERNAL_RESULT`
  - `onStartCommand()` 根据 `Intent.action` 分流：
    - 启动服务
    - 停止服务
    - 重启服务
    - 查询状态
    - 处理第三方入口映射后的外部启动动作
  - 进入服务后在 5 秒内调用 `startForeground()`
  - 服务内部持有：
    - `Process`
    - 当前端口
    - 运行态
    - 最近错误
    - 输出缓冲
  - 服务负责：
    - `ensureConfigExists()`
    - `findExecutableBinary()`
    - `startGoProcess()`
    - `stopGoProcess()`
    - `restartGoProcess()`
    - `monitorProcessOutput()`
    - `waitForBackendReady()`
  - 就绪判定采用：
    - 日志关键字优先
    - `/health` 探测兜底
  - 状态变化时同时发送：
    - 前端桥接需要的内部广播
    - 第三方监听需要的结果广播
  - 通知文案至少覆盖：
    - 启动中
    - 已就绪
    - 已停止
    - 启动失败
- 原因：
  - 这是满足“稳定后台运行”的核心改造点

#### 2. `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/MainActivity.kt`

- 目标：把 `MainActivity` 从“进程管理者”降级为“前端桥接与外部唤起入口协调者”。
- 具体修改：
  - 删除 `Process` 持有和直接启动 Go 的逻辑
  - 保留 `registerPlugin(GoProcessPlugin::class.java)`
  - 启动时改为：
    - 注册 Service 状态广播接收器
    - 触发 `EncvGoService.ACTION_START`
  - 新增 `onNewIntent()` 处理外部唤起：
    - 解析 `encvgo://...`
    - 解析显式 `Intent action`
    - 统一转发给 `EncvGoService`
  - 继续向 WebView 分发：
    - `encv:backend-ready`
    - `encv:backend-status`
  - 不负责业务路由重写，只负责把服务状态桥接给现有前端
- 原因：
  - 这样才能让 Activity 被销毁后，服务依然独立运行

#### 3. `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt`

- 目标：Capacitor 插件不再直接操控 `MainActivity` 状态，而是通过 `Intent` 控制 `EncvGoService`。
- 具体修改：
  - `restart()`：
    - 发送 `ACTION_RESTART` 到服务
    - 监听服务结果广播后 resolve/reject
  - `stop()`：
    - 发送 `ACTION_STOP`
    - 成功后 resolve
  - `getStatus()`：
    - 读取服务最新状态缓存，或通过广播 / 共享状态返回
  - `checkPermissions()`：
    - 收敛为 Android 12+ 所需权限模型
  - `requestNotificationPermission()`：
    - 保留 Android 13+ `POST_NOTIFICATIONS`
  - `requestStoragePermission()`：
    - 仅保留当前确有业务需要的 Android 12+ 路径
    - 不再为更早系统保留分支逻辑
- 原因：
  - 当前插件 API 前端已使用，最稳妥的做法是保持 TS 接口尽量不变，只改底层路由
  - 这样可以把 Service 改造锁在 Android 原生层

#### 4. `app/encv-mobile/android-overlay/app/src/main/AndroidManifest.xml`

- 目标：新增 overlay Manifest，声明前台服务、权限和第三方入口。
- 具体修改：
  - 新建 Android overlay Manifest
  - 增加权限：
    - `android.permission.INTERNET`
    - `android.permission.FOREGROUND_SERVICE`
    - `android.permission.FOREGROUND_SERVICE_DATA_SYNC`
    - `android.permission.POST_NOTIFICATIONS`
  - 在 `<application>` 中声明：
    - `EncvGoService`
    - `android:foregroundServiceType="dataSync"`
    - `android:exported="false"` 或按外部调用需要谨慎设定
  - 在 `MainActivity` 增加第三方入口 `intent-filter`
    - 自定义 Scheme：`encvgo://`
    - 显式 action：如 `com.encvgo.action.START`、`com.encvgo.action.RESTART`
  - 约束导出边界：
    - 对需要暴露给第三方的 Activity/入口设 `exported=true`
    - 对内部服务本体保持最小暴露面
- 原因：
  - Service 方案离不开 Manifest 层声明
  - 第三方入口必须通过清单明确注册

#### 5. `app/encv-mobile/scripts/post-cap-sync.mjs`

- 目标：让 Android 生成工程始终能获得完整的 Service 化 overlay。
- 具体修改：
  - overlay 复制文件扩展为：
    - `MainActivity.kt`
    - `GoProcessPlugin.kt`
    - `EncvGoService.kt`
    - `AndroidManifest.xml`
  - 保留 `network_security_config.xml`
  - 如当前脚本会清空 Java 目录，需确保新 Service 文件一起复制，避免 Android 工程缺类
  - 如 Capacitor 生成的 Manifest 需要 patch 而非整体替换，则改为在脚本中做稳定的 XML 注入
- 原因：
  - Service 方案不只是一两个 Kotlin 文件，必须确保 sync 后 Android 工程结构完整

#### 6. `app/encv-mobile/src/plugins/GoProcess.ts`

- 目标：保持前端插件调用面稳定，补充服务化后的状态定义。
- 具体修改：
  - 保留：
    - `restartBackend()`
    - `stopBackend()`
    - `getBackendStatus()`
    - 权限方法
  - 如有必要扩展返回值：
    - `running`
    - `port`
    - `lastError`
    - `source`（manual / external）
  - Web fallback 保持轻量空实现
- 原因：
  - 避免前端大量重写

#### 7. `app/encv-mobile/src/composables/useServerStatus.ts`

- 目标：做最小兼容修改，只让现有状态层继续工作在 Service 架构上。
- 具体修改：
  - 继续监听 `encv:backend-ready`
  - 如有必要，仅增加对 `encv:backend-status` 的消费
  - 在 restart/stop 时等待服务状态回传，而不是假定 `MainActivity` 本地状态
  - 在第三方唤起后，如果页面已打开，能立即刷新状态并连接 WebSocket
- 原因：
  - 服务化后状态源从 Activity 本地变量变成 Service 广播
  - 这是前端最小必要改动，不引入新的 composable 架构

#### 8. `app/encv-mobile/src/views/ServerDetail.vue`

- 目标：尽量不改页面结构，仅复用现有启停 UI。
- 具体修改：
  - 保留启停按钮和权限面板
  - 仅在确有必要时补充服务态提示文案
  - 与 `restartBackend()` / `stopBackend()` 的异步状态绑定
- 原因：
  - 用户需要能手动关闭常驻服务
  - 不应为了 Service 把页面重做一遍

#### 9. `app/encv-mobile/src/composables/useI18n.ts`

- 目标：补齐 Service 状态和错误提示文案。
- 具体修改：
  - 新增或校准文案键值：
    - 服务启动中
    - 服务已运行
    - 服务停止
    - 服务启动失败
    - 第三方调用已接收
- 原因：
  - 当前页面会直接显示这些状态

## Assumptions & Decisions

- 决策 1：必须做 Service 化，因为目标已明确为“稳定后台运行 + 第三方快捷调用支持”。
- 决策 2：只考虑 `Android 12+`，不再为 Android 11 及更低版本增加兼容逻辑。
- 决策 3：第三方入口同时支持两套协议：
  - `自定义 Scheme`
  - `显式 Intent`
- 决策 4：默认后台策略为：
  - 服务常驻运行
  - 用户在设置页可手动停止
- 决策 5：默认结果回传同时支持：
  - ENCV 前台页面可见
  - Android 广播结果可监听
- 决策 6：`MainActivity` 不再持有 `Process`，进程管理权完全迁移给 `EncvGoService`
- 决策 7：前端 TypeScript API 优先保持稳定，减少无谓重构
- 决策 8：不因为 Service 化去重做 Vue 路由、页面结构和前端控制抽象；Capacitor 的价值就在于 Web 层尽量不动
- 假设 1：Go 后端启动日志中仍可识别 ready 关键字；若不稳定，则服务继续以 HTTP `/health` 作为兜底就绪判定
- 假设 2：当前 assets 解压 + 多目录执行策略在 Service 中仍需保留，直到未来单独推进 `.so` / `jniLibs` 重构
- 假设 3：显式 Intent 的 action 命名将统一收敛到 `com.encvgo.action.*` 命名空间

## Verification Steps

- Android 工程生成验证：
  - 执行 `npx cap sync android` 后确认生成工程包含：
    - `EncvGoService.kt`
    - 更新后的 `MainActivity.kt`
    - 更新后的 `GoProcessPlugin.kt`
    - Manifest 中的 service 与 intent-filter
- 编译验证：
  - Android Studio / Gradle 编译通过
  - Android 12+ 权限与前台服务声明无编译错误
- 启动验证：
  - 冷启动 App 后，服务自动进入前台并开始拉起 Go 后端
  - 通知栏能看到常驻服务状态
  - 前端收到 ready 事件并连上本地接口
- 手动控制验证：
  - 在 `ServerDetail.vue` 点击停止，服务停止、通知消失或状态变更、前端离线
  - 再点击重启，服务恢复运行并前端恢复在线
- Activity 生命周期验证：
  - 关闭或重建 `MainActivity` 后，服务仍继续运行
  - 再次进入 App 时能重新同步当前服务状态
- 第三方 Scheme 验证：
  - 通过 `encvgo://start`
  - 通过 `encvgo://restart`
  - 验证 App 被唤起、服务执行动作、前端展示结果
- 第三方显式 Intent 验证：
  - 发送 `com.encvgo.action.START`
  - 发送 `com.encvgo.action.RESTART`
  - 验证动作被正确路由到服务
- 广播回传验证：
  - 外部 Android 调用方可监听结果广播并收到成功/失败、端口、错误信息
- 异常验证：
  - 人为制造配置错误或二进制不可执行场景
  - 验证通知、前端页面、外部广播三处都能收到可读错误

## 最终建议

- 当前最适合的落地路线，不再是 `MainActivity` 内修修补补，而是直接推进 `Foreground Service` 正式重构。
- 实施顺序应为：
  - 先补 `EncvGoService.kt` 和 Manifest
  - 再把 `MainActivity` 改成桥接层
  - 再改 `GoProcessPlugin.kt`
  - 最后只做前端最小兼容调整
- 第三方支持不应作为后加特性，而应在第一版 Service 方案中一并纳入，否则后续还会再次改动 Manifest、入口路由和结果回传协议。
- 如果后续需求继续扩大到：
  - 多平台统一后台服务模型
  - 更深的原生媒体、文件系统、系统集成
  - 大量 Android / iOS 双端原生能力
  那时再重新评估 KMP 会更合理；但以当前仓库现状，仍应先验证“Android 原生层封装 + Capacitor Web 层不大动”的低侵入路径是否足够。
