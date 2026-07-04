# Failure F4: 测试用 /api/ping 假象 —— OpenList 没有这个 endpoint，GET 走 SPA fallback

**Phase**: P2 (Vite sub-route middleware) + 测试套件
**Task**: test-phase-2-v2.sh J2 case
**Date**: 2026-06-02
**Status**: ✅ 已修（换用真实 endpoint）

---

## 症状

J2 测试用例：
```bash
HEAD=$(curl -sI "${VITE_URL}/openlist-ui/api/ping")
# 期望：HTTP/1.1 200（无 upstream 沙箱时：502）
# 实际：HTTP/1.1 405 Method Not Allowed  ← 不在期望里
```

随后检查 GET 响应 body：
```bash
curl -s http://127.0.0.1:5244/api/ping | head -3
# 输出：<!doctype html>  ← 不是 JSON，OpenList 没有 /api/ping 这个 endpoint
```

## 根因

1. **J2 用 `/api/ping` 做测试 endpoint 是错的**：这个路径在 OpenList 不存在实际 handler
   - HEAD 请求：OpenList 路径不存在但 HTTP method 不允许 → 405
   - GET 请求：路径在静态文件查找里，匹配不到 → 走 SPA fallback → 返回 index.html
2. **真正的 OpenList 公共 endpoint** 是 `/api/public/settings`（返回 JSON 站点配置）
3. 修测试用 `/api/public/settings` 才能真正验证 Vite proxy 是否工作

## 修复

[test-phase-2-v2.sh](file:///workspace/.trae/specs/openlist-frontend-extraction-and-sandbox-preview/test-phase-2-v2.sh) J2 case：

```bash
# J2: /openlist-ui/api/public/settings - real OpenList endpoint, returns JSON
HEAD=$(curl -sI "${VITE_URL}/openlist-ui/api/public/settings")
check "J2 status" "$HEAD" "HTTP/1.1 200"
check "J2 content-type" "$HEAD" "Content-Type: application/json"
BODY=$(curl -s "${VITE_URL}/openlist-ui/api/public/settings" | head -1)
if [[ "$BODY" == "{"* ]]; then
  echo "[PASS] J2 GET returns JSON"
fi
```

## 验证

```bash
$ bash test-phase-2-v2.sh
[PASS] J1 status
[PASS] J1 content-type
[PASS] J1 body is OpenList HTML
[PASS] J2 status                ← 现在用 /api/public/settings
[PASS] J2 content-type
[PASS] J2 GET returns JSON       ← 真的验证 proxy 返回 JSON
[PASS] J3 SPA fallback
[PASS] J4 status
[PASS] J4 content-type is JS
[PASS] J4 body is JS, not HTML
[PASS] J5 encv-mobile SPA root
[PASS] J6 /openlist/sites/* proxy
[PASS] J7 /openlist-ui (no slash)
=== Summary ===
PASS: 13
FAIL: 0
```

## 教训

1. **测试 endpoint 必须先确认存在**：不要拍脑袋选一个路径，先用直接请求看是不是真 endpoint
2. **HEAD 状态码 vs GET body 要分别看**：405 (method) ≠ 404 (path) ≠ 200 (SPA HTML) — 表面是错误码也分情况
3. **测试要验证"业务行为"，不只是"协议"**：J2 之前只测 "proxy responds 200/502/504"，实际我们要的是 "proxy returns real OpenList data"——后者才是真的业务需求
4. **OpenList 的公共 endpoint**：[OpenList API docs](https://github.com/OpenListTeam/OpenList/blob/main/server/handles/public.go) 列出 `/api/public/settings` `/api/public/ping` 等。最常用：测试时用 `/api/public/settings` 或 `/api/admin/login`

### OpenList 公共 endpoint 速查

| Endpoint | Method | 返回 |
|----------|--------|------|
| `/api/public/settings` | GET | 完整站点配置 JSON |
| `/api/public/ping` | GET | `{"code":200,"message":"success","data":"pong"}` ← 真正的 ping 路径！ |
| `/api/fs/list` | GET | 文件列表（需 auth） |
| `/api/admin/login` | POST | 登录（需 username + password） |
| `/api/ping` | - | **不存在**（任何 method 都走 SPA fallback） |
