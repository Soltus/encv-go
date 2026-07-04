# Capacitor 插件分析与推荐

## 项目概况

ENCV-go 是一个**本地加密文件管理 + 媒体播放**应用：

| 核心功能 | 实现方式 |
|----------|----------|
| 加密/解密任务 | Go 后端 + WebSocket 实时推送 |
| 文件浏览 | Go 后端 HTTP API |
| 视频/音频播放 | Lynx 原生播放器 + ArtPlayer WebView 播放器 |
| 后台服务 | Android ForegroundService 运行 Go 二进制 |
| 权限管理 | 自定义 GoProcess 插件 |
| 配置管理 | Schema 驱动的设置页面 |

当前使用的 Capacitor 插件：**仅自定义的 GoProcess 插件**，零第三方 Capacitor 插件。

---

## 推荐插件

### ⭐ 强烈推荐（直接解决现有痛点）

#### 1. `@capacitor/share` — 官方分享插件

**推荐理由**：项目有文件管理功能，用户可能需要分享文件。当前没有分享能力。

**用途**：
- 分享加密/解密后的文件给其他应用
- 分享文件链接
- 替代自定义的文件导出逻辑

**集成难度**：极低（官方插件，`npm install` 即用）

```typescript
import { Share } from '@capacitor/share'
await Share.share({ title: '视频', url: 'file:///path/to/video.mp4' })
```

---

#### 2. `@capacitor-community/keep-awake` — 屏幕常亮

**推荐理由**：视频播放时屏幕自动熄灭是常见痛点。当前项目有 Player 页面，播放视频时应该保持屏幕常亮。

**用途**：
- 播放视频时阻止屏幕休眠
- 加密/解密任务执行时保持屏幕活跃（可选）

**集成难度**：极低

```typescript
import { KeepAwake } from '@capacitor-community/keep-awake'
// 进入播放器时
await KeepAwake.enable()
// 退出播放器时
await KeepAwake.disable()
```

---

#### 3. `@capacitor/local-notifications` — 官方本地通知

**推荐理由**：项目已有 `requestNotificationPermission()`（在 GoProcess 中），但只是请求权限，没有实际发送通知。加密/解密任务完成时应该发通知。

**用途**：
- 加密/解密任务完成通知
- 后台服务状态变更通知
- 替代 GoProcess 中的 `requestNotificationPermission()`，用官方插件统一管理

**集成难度**：低

```typescript
import { LocalNotifications } from '@capacitor/local-notifications'
await LocalNotifications.schedule({
  notifications: [{
    title: '加密完成',
    body: 'video.mp4 已加密',
    id: 1,
  }]
})
```

---

#### 4. `@capacitor-community/privacy-screen` — 隐私保护

**推荐理由**：加密文件管理应用天然需要隐私保护。当用户切换到其他应用时，最近任务列表不应显示应用内容。

**用途**：
- 阻止应用截图出现在最近任务列表
- 阻止屏幕录制捕获应用内容
- 增强用户信任感

**集成难度**：极低（一行代码）

```typescript
import { PrivacyScreen } from '@capacitor-community/privacy-screen'
await PrivacyScreen.enable()
```

---

### 🔶 值得考虑（提升用户体验）

#### 5. `@capacitor/filesystem` — 官方文件系统

**推荐理由**：项目已有 Go 后端处理文件，但前端有时需要直接访问文件系统（如缓存、临时文件）。

**用途**：
- 前端缓存下载的缩略图
- 保存用户偏好到本地文件
- 与 GoProcess 的文件操作互补

**注意**：Go 后端已覆盖大部分文件操作，此插件仅用于前端侧的轻量文件操作。

---

#### 6. `@capacitor/network` — 官方网络状态

**推荐理由**：应用依赖本地 Go 服务，网络状态变化时需要及时反馈。

**用途**：
- 检测网络断开/恢复
- WebSocket 断线重连策略
- 在 UI 中显示网络状态

```typescript
import { Network } from '@capacitor/network'
Network.addListener('networkStatusChange', status => {
  if (!status.connected) {
    // 提示用户网络断开
  }
})
```

---

#### 7. `@capacitor/status-bar` — 官方状态栏控制

**推荐理由**：播放器全屏时需要隐藏状态栏，提升沉浸感。

**用途**：
- 播放器全屏时隐藏状态栏
- 根据主题调整状态栏颜色（深色/浅色）

---

#### 8. `@capacitor/screen-orientation` — 屏幕方向控制

**推荐理由**：视频播放时应该自动切换为横屏，退出播放器后恢复竖屏。

**用途**：
- 播放视频时锁定横屏
- 文件浏览时锁定竖屏

```typescript
import { ScreenOrientation } from '@capacitor/screen-orientation'
// 进入播放器
await ScreenOrientation.lock({ orientation: 'landscape' })
// 退出播放器
await ScreenOrientation.unlock()
```

---

#### 9. `@capgo/capacitor-webview-guardian` — WebView 守护

**推荐理由**：Android 低内存时 WebView 会被系统杀死，导致应用白屏。这是 Capacitor 应用的常见问题。

**用途**：
- 检测 WebView 被杀死
- 自动恢复 WebView
- 提升应用稳定性

---

### 🟡 远期考虑（非当前优先级）

| 插件 | 场景 | 备注 |
|------|------|------|
| `@capacitor-firebase/messaging` | 推送通知 | 需要服务端支持，当前是本地应用 |
| `@capgo/capacitor-updater` | 热更新 | 应用上架后有用，绕过应用商店审核 |
| `@capacitor-community/sqlite` | 本地数据库 | 当前用 Go 后端 + 文件存储，暂不需要 |
| `@capgo/capacitor-file-picker` | 文件选择器 | 当前用 Go 后端浏览文件，但原生选择器体验更好 |
| `@capawesome/capacitor-android-foreground-service` | 前台服务 | 当前自定义实现，可考虑用社区方案替代 |
| `@capacitor/haptics` | 触觉反馈 | 任务完成/失败时的微交互 |
| `@capacitor/clipboard` | 剪贴板 | 复制分享链接、密码等 |

---

## 不推荐的插件

| 插件 | 原因 |
|------|------|
| AdMob / Stripe / 支付类 | 本地工具应用，无商业化需求 |
| 地图 / 定位类 | 不涉及地理位置 |
| 社交登录 / OAuth | 无账号系统 |
| Camera / Barcode | 不涉及拍照/扫码 |
| Bluetooth / NFC | 不涉及硬件通信 |

---

## 实施建议

**第一批（立即可做，投入小收益大）**：

1. `@capacitor-community/keep-awake` — 播放器体验提升，2 分钟集成
2. `@capacitor-community/privacy-screen` — 隐私保护，1 分钟集成
3. `@capacitor/share` — 文件分享能力，5 分钟集成

**第二批（需要少量代码适配）**：

4. `@capacitor/local-notifications` — 任务完成通知
5. `@capacitor/network` — 网络状态监控
6. `@capacitor/status-bar` + `@capacitor/screen-orientation` — 播放器沉浸体验

**第三批（远期）**：

7. `@capgo/capacitor-webview-guardian` — 稳定性提升
8. `@capgo/capacitor-updater` — 热更新（应用上架后）
