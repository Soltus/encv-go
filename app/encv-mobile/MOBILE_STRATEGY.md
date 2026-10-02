# ENCV Mobile 双线并行策略

## 概述

| 路线 | 定位 | 技术栈 | 目录 |
|------|------|--------|------|
| **Capacitor 独立产品线** | ENCV 独立品牌产品 | Ionic Vue + Capacitor | `app/encv-mobile/` |
| **OpenList Mobile Fork** | ENCV 生态发行版 | Flutter (基于 OpenList Mobile) | `app/openlist/` |

---

## 一、Capacitor 独立产品线

### 技术栈
- **UI 框架**: Ionic Vue
- **移动端框架**: Capacitor
- **后端服务**: ENCV-go Daemon (本地运行)

### 架构
```
Ionic Vue UI (Capacitor WebView)
        ↓ localhost HTTP
ENCV-go Daemon
Stream/WebDAV/API
```

### 功能路线

#### v1: "能播放"
- [ ] 浏览加密文件
- [ ] 解密流播放
- [ ] Seek 支持
- [ ] 基础播放功能

#### v2
- [ ] 离线缓存
- [ ] 下载功能
- [ ] 后台播放
- [ ] 字幕支持

#### v3
- [ ] 本地 mount
- [ ] MediaStore 集成
- [ ] Android share
- [ ] 外部播放器支持

---

## 二、OpenList Mobile Fork 生态发行版

### 技术栈
- **UI 框架**: Flutter (继承自 OpenList Mobile)
- **核心**: OpenList Core + ENCV Driver

### 架构
```
ENCV-go Core
       ↓
OpenList Driver
       ↓
┌──────────────┬──────────────────┐
│ OpenList Web │ OpenList Desktop │
└──────────────┴──────────────────┘
       ↓
OpenList Mobile Fork
       ↓
Flutter Android/iOS
```

### 核心原则
- **不重构 ENCV-go 架构**
- **保持统一 ENCV Driver/Core**
- **不单独设计移动底层架构**

---

## 三、关键技术问题

### 播放器策略
- Phase 1: 复用默认播放器，验证 ENCV stream 是否可直接播放
- Phase 2: 如需引入 Native Player (Android: ExoPlayer, iOS: AVPlayer)

### 移动缓存层
- 加密分片缓存 + Memory cache + Disk cache + LRU

### Android 权限
- Scoped Storage / 后台播放 / Doze mode / 大文件 IO / SAF URI / Android 13+ 权限

---

## 四、核心原则

⚠️ 不要让 Mobile 成为"特殊实现"，必须保持：

```
OpenList Core → 统一 ENCV Driver → 所有平台继承
```

---

## 五、桌面端（web）与双端互联

> 立项中（2026-10-02，P0 规划轮，未动代码）。**权威文档**：`.trae/specs/desktop-web-android-pairing/`
> （`spec.md` 契约 / `tasks.md` P0–P6 / `checklist.md` / `progress.md` 多轮迭代跟踪）。

新增第三条形态：**桌面端（web）**（在桌面浏览器里运行，非 dev 预览）、以及与安卓端的
**扫码配对互联**：

- **拓扑前提**：桌面端（web）跑在 **cnb 云开发环境（公网 HTTPS，与其 Go 后端同源）**，
  安卓端在 **NAT 后无公网地址** ⇒ 两端**不同网**，跨端一律走「**Hub（cnb）+ 手机主动出网的
  WSS 长连接**」，**禁止 LAN 直连**（https 页面请求 http 内网地址会被浏览器以混合内容拦截）。
- **配对**：桌面生成一次性票据二维码（内容 = 会合点 hub + pairingId + psk，**不含任何内网地址**，
  120s 过期）→ 安卓扫码（MLKit）→ HMAC 校验 + SAS 短码核对 → 双向 token 与 AEAD 密钥
  （**只存进程内存**）→ 心跳保活（前台 15s / 后台 60s）。
- **互通搜索索引**：双端在线时并发查询对端索引，结果是**跨端引用**（带来源徽章），
  **明确不做**挂载网络驱动器 / 统一命名空间。
- **远程 Agent**：可调用对端执行调试工具；敏感操作默认在**执行端**弹窗授权，
  可"信任此设备"，**信任仅到本端服务重启为止**。

关键边界：`baseUrl` = 本端自己的后端（语义不变）；`peer` = 已配对的另一台设备，
**peer 不得被写进 baseUrl / 探测链**。
