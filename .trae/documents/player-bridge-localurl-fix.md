# 修复 bridge.localUrl 为 null 的 Plan

## 根因分析

### 为什么 `bridge.localUrl` 返回 null

查看 [BridgeActivity.java](https://unpkg.com/@capacitor/android@8.3.4/capacitor/src/main/java/com/getcapacitor/BridgeActivity.java) 源码：

```java
// BridgeActivity.java
protected Bridge bridge;  // ← 初始为 null

protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    // ... 初始化 WebView ...
    this.load();  // ← 在这里才创建 bridge
}

protected void load() {
    Logger.debug("Starting BridgeActivity");
    bridge = bridgeBuilder.addPlugins(initialPlugins).setConfig(config).create();
    // ↑ bridge 在这里被赋值！
    this.onNewIntent(getIntent());
}
```

**关键发现**：`bridge` 对象是在 `BridgeActivity.load()` 中通过 `bridgeBuilder.create()` 创建的。我们的 [PlayerActivity.kt](app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/PlayerActivity.kt#L48-L63) 覆写了 `load()` 但**没有调用 `super.load()`**，导致：

1. `bridge` 永远是 `null`（从未被创建）
2. `bridge?.localUrl` 自然返回 null
3. fallback 到硬编码 `https://localhost/player.html`，但此时 WebView 可能还没完全就绪

### 同时：`localUrl` 是 private 字段

从 [Bridge.java](https://unpkg.com/@capacitor/android@8.3.4/capacitor/src/main/java/com/getcapacitor/Bridge.java) 源码确认：

```java
public class Bridge {
    private String localUrl;   // ← private！没有公开 getter
    private String appUrl;     // ← private！
    // ...
}
```

即使 `bridge` 不为 null，`bridge.localUrl` 在 Kotlin 中也无法直接访问（private 字段）。之前代码能编译通过是因为 Kotlin 的 `?.` 安全调用把整个表达式变成了可空链。

---

## 修复方案

### 核心策略：必须调用 `super.load()` + 用已知 scheme 构造 URL

**Capacitor 配置** ([capacitor.config.ts](app/encv-mobile/capacitor.config.ts))：
```typescript
server: {
    androidScheme: 'https',  // → localServer URL 为 https://localhost/
}
```

这意味着 LocalServer 的 base URL 始终是 `{scheme}://localhost/`。我们不需要从 bridge 动态获取，直接用配置值即可。

### 修改文件：`PlayerActivity.kt` 的 `load()` 方法

```kotlin
override fun load() {
    super.load()  // ① 必须调用！创建 bridge、初始化 WebView、注册插件等
    try {
        val host = bridge?.config?.getString("hostname") ?: "localhost"
        val scheme = bridge?.config?.getString("androidScheme") ?: "https"
        val playerUrl = "$scheme://$host/player.html"
        Log.i(TAG, "load: bridge created, redirecting to $playerUrl")
        bridge?.webView?.loadUrl(playerUrl)
    } catch (e: Exception) {
        Log.e(TAG, "load: failed to redirect to player.html", e)
    }
}
```

**为什么这样可行**：
- `super.load()` 完成后，`bridge` 已创建、WebView 已初始化、LocalServer 已启动
- 紧接着调用 `webView.loadUrl(player.html)` 会取消正在加载的 `index.html`，直接加载 `player.html`
- 不存在竞态条件 — 所有在同一个主线程同步执行
- 使用 `bridge.config` 获取实际配置的 scheme 和 hostname（比硬编码更健壮）

> **备选**：如果 `bridge.config` 也不易访问，直接使用常量：
> ```kotlin
> val playerUrl = "https://localhost/player.html"
> ```

---

## 修改清单

| 文件 | 方法 | 变更 |
|------|------|------|
| `android-overlay/.../PlayerActivity.kt` | `load()` | 恢复调用 `super.load()`；用 `bridge.config` 或常量构造 playerUrl |

仅此一处修改。
