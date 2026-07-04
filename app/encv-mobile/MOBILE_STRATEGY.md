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
