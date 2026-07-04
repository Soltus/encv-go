# Failure F1: sirv SPA fallback 把所有 asset 404 错误地返回 index.html

**Phase**: P2 (Vite sub-route middleware)
**Task**: Task 2.6 (静态服务验证)
**Date**: 2026-06-02
**Status**: ✅ 已修

---

## 复现命令

```bash
cd /workspace/app/encv-mobile
pnpm dev  # 后台跑
sleep 5
curl -sI http://localhost:8100/openlist-ui/assets/2fa-ZiFLQpje.js
curl -s  http://localhost:8100/openlist-ui/assets/2fa-ZiFLQpje.js | head -3
```

## 实际输出（v1 失败版）

```
HTTP/1.1 200 OK
Content-Type: text/html          ← 应该是 text/javascript
Etag: W/"15d-yb6ooqQfuM+EoLNma+N7lwGkq8g"   ← 跟 index.html 一样的 Etag

<!DOCTYPE html>                   ← 是 index.html，不是 JS 文件
<html lang="en">
  <head>
```

## 根因

1. **Vite middleware prefix 匹配不自动 strip 前缀**
   - `server.middlewares.use('/openlist-ui', handler)` 收到完整 URL `/openlist-ui/assets/foo.js`
   - 不是被 strip 成 `/assets/foo.js`

2. **sirv 拿完整 URL 找文件**
   - 找 `<OPENLIST_DIST>/openlist-ui/assets/foo.js`（不存在）

3. **`single: true` 触发 SPA fallback**
   - 任何 404 都返回 index.html
   - 包括「找不到 assets/ 下的真实文件」

4. **测试假阳性**
   - v1 测试 `curl -sf -o /dev/null` 只看 200 status code
   - 200 + HTML body 被误判为「通过」
   - 实际所有 OpenList 静态资源都坏了，只是看起来通

## 修复

### 修复 1: strip 前缀（核心修复）

[vite.config.ts:99](file:///workspace/app/encv-mobile/vite.config.ts#L99)：

```typescript
server.middlewares.use('/openlist-ui', (req, res, next) => {
  const orig = req.url || '/'
  // CRITICAL: strip the /openlist-ui prefix so sirv can resolve relative paths.
  req.url = orig.replace(/^\/openlist-ui\/?/, '/') || '/'
  serve(req as any, res as any, next)
})
```

### 修复 2: 同步 import sirv（次要修复）

[vite.config.ts:6](file:///workspace/app/encv-mobile/vite.config.ts#L6)：

```typescript
import sirv from 'sirv'  // 顶部同步导入，避免 lazy import 时序问题
```

原版用 `import('sirv').then(...)` 异步注册，HMR restart 时 middleware 可能错过首请求。

## 验证

v2 测试 11/11 通过，包括 `J4 body is JS, not HTML` 这条之前被掩盖的检查。

```bash
$ curl -sI http://localhost:8100/openlist-ui/assets/2fa-ZiFLQpje.js
HTTP/1.1 200 OK
Content-Type: text/javascript     ← 正确
Content-Length: 1311              ← 真实 JS 文件大小
Last-Modified: Tue, 25 Nov 2025 11:31:06 GMT   ← 真实 mtime

$ curl -s http://localhost:8100/openlist-ui/assets/2fa-ZiFLQpje.js | head -1
import{j as v,ba as c,...        ← 真实 JS 内容
```

## 教训

1. **测试必须看 body content，不只是 status code**：200 + 错误 body = 假阳性
2. **Prefix 路由要小心**：middleware 不会自动 strip 前缀，必须在 handler 内手动处理
3. **同步 import > 异步 import**（对配置类代码）：减少时序不确定性
4. **第一次跑通 ≠ 真的跑通**：v1 的 6/6 通过实际上是错的——必须做深入内容检查
