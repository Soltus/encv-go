# 播放器彻底隔离 — 半独立应用架构（含设置 + 加密视频 Range 支持）

## 架构总览

```
ENCV Mobile App
├── 主应用 (MainActivity)              播放器应用 (PlayerActivity)
│   ├── index.html                     ├── player.html
│   ├── src/main.ts                    ├── src/player-main.ts
│   ├── App.vue                        ├── PlayerApp.vue (根组件)
│   ├── router/index.ts                └── router/player.ts (独立路由)
│   │   ├── /tabs/files               │     ├── /player          → StandalonePlayer
│   │   └── /tabs/settings            │     └── /player/settings → PlayerSettings
│   ├── views/Tabs.vue                └── 无 TabBar，全屏播放
│   └── WebSocket + 权限请求             独立后端交互，无 WS
│                                       共享层：
│                                     ├── composables/plugins/api/theme (复用)
```

---

## 文件变更清单

### 新建文件（5 个）

#### 1. `player.html`
#### 2. `src/player-main.ts`
#### 3. `PlayerApp.vue`
#### 4. `src/router/player.ts` — 路由 `/player` + `/player/settings`
#### 5. `src/views/PlayerSettings.vue` — 播放器独立设置

### 修改文件（5 个）

#### 6. `vite.config.ts` — 多入口构建（main + player）
#### 7. `PlayerActivity.kt` — `load()` 加载 `player.html`
#### 8. `router/index.ts` — 删除 `/standalone/player`
#### 9. `StandalonePlayer.vue` — 添加设置入口按钮
#### 10. 🔴 `mobile_service.go` — **加密文件 Range 支持修复**

---

## 🔴 关键 Bug: `/api/stream/external` 不支持加密视频解密

### 问题现状

| 端点 | 加密 .encv 文件处理 | Range 支持 |
|------|-------------------|------------|
| `/stream?path=...` | ✅ 检测容器 → 解密流式传输 | ✅ |
| `/api/stream/external?path=...` | ❌ 直接 `http.ServeFile`（返回乱码） | ⚠️ 有但无用 |

[server_handle.go L106-L112](file:///workspace/internal/server/server_handle.go#L106-L112) 中 `/stream` 的逻辑：
```go
_, detectErr := detector.DetectContainer(cleanedFilePath)
if detectErr != nil {
    http.ServeFile(w, r, cleanedFilePath)  // 非加密文件，直接服务
    return
}
s.serveEncryptedFile(w, r, cleanedFilePath)    // 加密文件，解密后服务 ✅
```

但 [mobile_service.go L622](file:///workspace/internal/service/mobile_service.go#L622) 中 `StreamExternalFile`：
```go
http.ServeFile(w, r, absPath)  // ❌ 不管是否加密，全部直接服务
```

**结果**：第三方打开 `.encv` 加密视频时，播放器拿到的是未解密的二进制数据 → 播放失败。

### 修复方案

在 `StreamExternalFile` 中增加加密容器检测，与 `/stream` 端点逻辑一致：

```go
func (s *MobileService) StreamExternalFile(w http.ResponseWriter, r *http.Request, filePath string) error {
    // ... 参数校验、路径清理、存在性检查 ...

    // 检测是否为 ENCV 加密容器
    if _, detectErr := s.containerDetector.DetectContainer(absPath); detectErr == nil {
        // 是加密文件 → 复用 Server 的 serveEncryptedFile 逻辑
        return s.serveEncryptedFile(w, r, absPath)
    }

    // 非加密文件 → 正常 media type 检查 + ServeFile
    ext := strings.ToLower(filepath.Ext(absPath))
    // ... mediaExtensions 白名单检查 ...
    http.ServeFile(w, r, absPath)
    return nil
}
```

需要给 `MobileService` 注入依赖：
- `containerDetector`（用于检测 ENCV 容器）
- 或直接引用 `Server.serveEncryptedFile` 方法

**最简方案**：让 `MobileService` 持有对 `*Server` 的引用（已有隐式关系），或注入必要的接口。

---

## 实施步骤

- [ ] Step 1: 创建 `player.html`
- [ ] Step 2: 创建 `src/player-main.ts`
- [ ] Step 3: 创建 `PlayerApp.vue`
- [ ] Step 4: 创建 `src/router/player.ts`（/player + /player/settings）
- [ ] Step 5: 创建 `src/views/PlayerSettings.vue`（独立 localStorage 配置）
- [ ] Step 6: 修改 `vite.config.ts` 多入口构建
- [ ] Step 7: 修改 `PlayerActivity.kt` load() 加载 player.html
- [ ] Step 8: 从 `router/index.ts` 删除 /standalone/player
- [ ] Step 9: 调整 StandalonePlayer.vue（设置按钮 + 偏好读取）
- [ ] Step 10: 🔴 修复 `StreamExternalFile` 加密文件解密 + Range 支持
- [ ] Step 11: 构建验证（vue-tsc + vite build + go build）
- [ ] Step 12: 本地合并模拟验证
