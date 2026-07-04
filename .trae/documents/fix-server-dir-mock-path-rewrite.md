# 修复：移动端预览 server.dir 显示桌面端根路径

## 问题精确定位

### 当前行为（已确认）

通过 Vite (`:5173`) 访问 `/api/config`：
```json
{
  "server": { "dir": "/", "port": 2025, ... },
  "mobile": { "server_dir": "__mock_data__/..." },  ← 已被 mock handler 重写 ✅
  ...
}
```

`server.dir = "/"` 是**后端原始文件中的值**，经 proxy 原样透传到前端。

### 关键认知

**移动端预览场景下**：
- 前端使用 `mobile.server_dir` 作为文件服务基础路径 → **已被正确重写为 `__mock_data__`**
- `server.dir` 是桌面端字段 → **移动端前端不使用此字段**
- 所以 `server.dir = "/"` 对移动端预览**无功能影响**

### 真正的问题

如果 `server.dir = "/"` 导致前端显示异常（如 Settings 页面显示错误路径、路径拼接错误等），那需要在 **前端侧** 处理（使用 `mobile.server_dir` 替代 `server.dir`），而不是在 mock 层伪造 `server.dir`。

---

## 待确认

在实施前需要确认：`server.dir = "/"` 具体导致了什么可见问题？

- A) Settings/ServerDetail 页面显示了错误的根目录路径？（纯显示问题）
- B) 文件列表 API 请求使用了错误的 base path？（功能性问题）
- C) 其他？

请确认具体症状后再制定精确修复方案。
