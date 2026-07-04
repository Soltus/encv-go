# 修复 Bug 2/5/6 计划

## Bug 2: 内置 MPV 播放失败路径为空

### 问题
播放失败时错误提示中没有显示播放路径/URL，用户无法判断是哪个文件播放失败。

### 修复方案
1. **增强错误信息**：在 `PlayerApp.tsx` 的 `startPlayback` 中，播放失败时在错误消息中包含 streamUrl
2. **检查 `play()` 回调**：检查回调参数，如果回调返回错误则设置 error 状态
3. **错误显示优化**：`PlayerControls.tsx` 的错误信息显示中包含文件名

### 修改文件
- `app/encv-mobile/lynx-player/src/player/PlayerApp.tsx` — 增强错误处理
- `app/encv-mobile/lynx-player/src/player/PlayerControls.tsx` — 错误信息显示优化

---

## Bug 5: 深色模式下文件长按操作文字灰色可见度差

### 问题
Ionic ActionSheet 在深色模式下按钮文字颜色不可见。

### 根因
之前添加的 CSS 变量名 `--ion-action-sheet-button-color` 可能不是 Ionic 7 的正确格式。Ionic ActionSheet 的组件级变量名是 `--button-color`，需要设置在 `ion-action-sheet` 元素上。

### 修复方案
在 `variables.css` 的 `body.dark` 中使用正确的 Ionic CSS 变量格式：
```css
body.dark ion-action-sheet {
  --button-color: #ffffff;
  --button-color-activated: #cccccc;
  --button-color-hover: #e0e0e0;
  --title-color: #aaaaaa;
}
```

### 修改文件
- `app/encv-mobile/src/theme/variables.css` — 修正 ActionSheet CSS 变量名

---

## Bug 6: WebDAV 测试连接结果总是连接成功

### 问题
后端 `TestWebDAV` 已修复检查状态码，但 `handleTestWebDAVGin` 即使测试失败也返回 HTTP 200 + `{"success": false, "error": "..."}`，前端只检查 `response.ok` 不检查 JSON 中的 `success` 字段。

### 修复方案
修改前端 `testWebDAVConnection`，解析 JSON 中的 `success` 字段：
```typescript
const data = await response.json()
if (data.success === false) {
  throw new Error(data.error || '连接失败')
}
```

### 修改文件
- `app/encv-mobile/src/api/encv.ts` — `testWebDAVConnection` 检查 JSON 中的 `success` 字段

---

## 实施顺序

1. Bug 6（WebDAV 测试连接）— 前端 API 修改
2. Bug 5（深色模式文字）— CSS 变量修正
3. Bug 2（MPV 播放路径为空）— 错误处理增强

## 构建验证

```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build
```
