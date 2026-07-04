# 修复 UI 布局 + Native Module 播放报错

## 现状
- ✅ **`root.render()` 生效！UI 终于渲染出来了！**
- ❌ 布局问题：蓝色区域只在顶部，约 3 倍播放图标高度（未全屏）
- ❌ 点击播放报错：`Player error: NativeModule: In module 'MpvPlayerModule' method '.'`

## 问题 1：布局未全屏

### 分析
用户看到的是 idle 状态的居中内容（▶ 按钮 + 文件名），但只显示在顶部一小块区域。说明 `<page style={{ flex: 1 }}>` 没有让页面填满 LynxView。

**原因**：`<page>` 是根元素，没有 flex 父容器，`flex: 1` 对它无效。需要显式设置宽高。

### 修复
**文件**：`lynx-player/src/components/AppComponent.tsx`

将：
```tsx
<page style={{ flex: 1 }}>
```
改为：
```tsx
<page style={{ width: "100%", height: "100%" }}>
```

同时 **PlayerContainer 的 CSS 也需要确保填满**：

**文件**：`lynx-player/src/App.css`
```css
.PlayerContainer {
  flex: 1;
  width: 100%;
  height: 100%;
  background-color: #001030;
  justify-content: center;  /* 默认居中，idle/loading/error 都用 */
  align-items: center;
}
```

并且移除 AppComponent.tsx 中动态切换 justifyContent 的逻辑（统一使用 CSS 居中）。

## 问题 2：Native Module 调用报错

### 分析
错误信息 `"NativeModule: In module 'MpvPlayerModule' method '.'"` 来自 Lynx 的 JSBridge 错误处理。点击播放时 `startPlayback()` 首先调用 `GoBackendModule.getBackendStatus(resolve)`，这个调用抛出了异常。

**可能原因**：
1. GoBackendModule 构造函数中 `context as? LynxContext` 返回 null，后续操作虽然加了 `?.` 但某些路径可能仍有问题
2. Method 注册名称或签名不匹配
3. Module 内部广播接收器注册失败

### 修复策略

#### Step A：在 JS 层增加详细错误捕获

**文件**：`lynx-player/src/components/AppComponent.tsx`

```tsx
const startPlayback = useCallback(async (data: InitData | undefined) => {
    if (!data) return;
    setPlayerState("loading");
    try {
      console.info("startPlayback: step 1 - getBackendStatus");
      const status = await new Promise<any>((resolve, reject) => {
        try {
          NativeModules.GoBackendModule.getBackendStatus((result: any) => {
            console.info("getBackendStatus result:", JSON.stringify(result));
            resolve(result);
          });
        } catch (e: any) {
          reject(e);
        }
      });
      // ... rest of logic
    } catch (e: any) {
      console.info("startPlayback caught:", e?.message || String(e), e?.stack || "");
      setPlayerState("error");
      setErrorMessage(String(e?.message || e));
    }
}, []);
```

#### Step B：检查 GoBackendModule 是否正确初始化

**文件**：`android-overlay/app/src/main/java/com/encvgo/app/GoBackendModule.kt`

确认 init() 方法中的静态持有者注册和广播接收器注册都有 try-catch。

#### Step C：确认 MpvPlayerModule.init() 不在主线程阻塞

当前 MpvPlayerModule.init() 在构造函数中被调用，而 Lynx 创建 Module 可能在任意线程。如果 MPVLib.create() 需要在主线程执行，可能会失败。

## 修复步骤

### Step 1：修复布局 — page 显式尺寸 + PlayerContainer 全屏

### Step 2：JS 层 startPlayback 分步日志 + 错误堆栈捕获

### Step 3：GoBackendModule/MpvPlayerModule 增加 init 阶段防御日志

### Step 4：移除调试背景色（确认渲染正常后）

## 预期效果

- UI 全屏显示，居中的播放按钮和文件名
- 点击播放后能看到详细的错误信息（而非截断的 toast）
- 根据新错误信息定位具体是哪个 Native Module / 哪个方法失败
