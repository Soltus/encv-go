# Mock 数据生成铁律

> **核心原则**：2 套 mock 生成逻辑必须同源——后端 Go / 前端 TS 共享字节或共享逻辑源。禁止任何一处独自写裸 header 字节冒充"合法媒体"。任何调后端 `/api/mock/*` 都必须显式带 `X-Confirm-Mock-Mutation: yes` header（防擅自生成）。

> **完整内容 + 历史踩坑**：[详情文档](../rule-library/mock-data-architecture.md)

---

## 一、2 套实现清单（2026-06-10 改造）

| 位置 | 用途 | 现状 |
|------|------|------|
| [app/encv-mobile/src/lib/mockDataGenerator.ts](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts) `createMP4/MKV/MP3/FLAC` | 前端运行时调用 / 单元测试 | ✅ 内嵌 base64 合法字节（4.8KB mp4 / 170B mkv / 45KB mp3 / 94B flac） |
| **[internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) `minimalMP4/MKV/MP3/FLAC`** | **后端 API 调给开发者选项** | **✅ ffmpeg 优先 + base64 fallback**（与前端 mockDataGenerator.ts 字节 1:1 同步） |
| ❌ ~~Node CLI `app/encv-mobile/scripts/generate-mock-files.ts`~~ | 2026-06-10 **已删除** | 与后端 API 重复入口，废弃。Node 端需要 mock 文件请直接调后端 API。 |

---

## 二、3 套字节必须同源（铁律）

> **任何一处写裸 header 字节冒充"合法媒体"都是定时炸弹。**

字节同步方法（2 选 1）：

### 方案 A：嵌入 base64 常量（当前实现）

```bash
# 1. 从前端提取合并 base64
cat > /tmp/extract-b64.mjs <<'EOF'
import { readFileSync, writeFileSync } from 'fs'
import { createMP4 } from '/workspace/app/encv-mobile/src/lib/mockDataGenerator.ts'
import { Buffer } from 'node:buffer'
writeFileSync('/tmp/MP4_B64.txt', Buffer.from(createMP4()).toString('base64'))
EOF
node /tmp/extract-b64.mjs

# 2. 格式化为 Go raw string literal（100 列）
python3 -c "
b64 = open('/tmp/MP4_B64.txt').read().strip()
for i in range(0, len(b64), 100): print(b64[i:i+100])
" > /tmp/b64-formatted.txt
```

复制到 [internal/server/mock_media_bytes.go](file:///workspace/internal/server/mock_media_bytes.go)：

```go
const MP4_B64 = `
<100 chars per line>
`
```

### 方案 B：把字节文件提到共享目录

```
/workspace/internal/server/mock_assets/
├── sample.mp4
├── sample.mkv
├── sample.mp3
├── sample.flac
```

Go 端用 `//go:embed` 加载，前端用 `import sampleMp4 from '?raw'`。但需要**单一可信源**（ffmpeg 生成一次，commit 进 git），否则 3 套实现变成 3 套字节。

---

## 三、调用入口

### 3.1 显式意图确认（防擅自生成）—— 🆕 2026-06-10 铁律

**后端**：
- `POST /api/mock/generate`：必须带 `X-Confirm-Mock-Mutation: yes` header，否则 403。
- `POST /api/mock/reset`：必须带 `X-Confirm-Mock-Mutation: yes` header，否则 403。
- 校验实现：[internal/server/mock_generator.go:handleMockGenerateGin](file:///workspace/internal/server/mock_generator.go) / handleMockResetGin。

**前端**：
- [src/api/mockGenerator.ts](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts) `generateMockFilesViaBackend` / `resetMockFilesViaBackend` 自动带 header。
- 第三方爬虫 / 误调 / curl 没带 header → 403。

### 3.2 调用入口清单（2026-06-10 收口）

| 入口 | 走哪 | 备注 |
|------|------|------|
| 开发者选项"生成 Mock"按钮 | 后端 API `/api/mock/generate` | [src/views/AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) → `generateMockFilesViaBackend` |
| 自动化测试 setup 阶段 | 后端 API | 同上 |
| Workflow Dashboard "Mock Server Files" | 后端 API | [src/views/WorkflowDashboard.vue](file:///workspace/app/encv-mobile/src/views/WorkflowDashboard.vue) |
| ~~Node CLI `scripts/generate-mock-files.ts`~~ | 已删除 | 2026-06-10 砍掉（与后端 API 重复入口） |
| ~~Vite plugin `mock/index.ts`~~ | 已删除 | dev mock 中间件也调 CLI，CLI 删后整个 Vite plugin 删 |
| ~~gateway preflight `ensureMockData`~~ | noop 桩 | [app/preview-gateway/src/preflight.ts](file:///workspace/app/preview-gateway/src/preflight.ts) 改 noop，gateway 启动不再自动写盘 |
| 前端运行时降级（单元测试） | [src/lib/mockDataGenerator.ts](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts) | Web preview 单元测试用 |

### 3.3 service-guard 不再查 mock 数据（2026-06-10 简化）

[internal/server/mobile_api.go:handleServiceGuardGin](file:///workspace/internal/server/mobile_api.go) 2026-06-10 改造：
- ❌ 删 01-plain-media marker 检查（4 子目录 + 文件数）
- ❌ 删 `mockScript` / `previewScript` 字段
- ✅ 只查 `servingDir === /storage/emulated/0`（mobile overlay 标准路径）

mock 数据是否就位不再影响 service-guard 判定。用户没主动按"生成 Mock"按钮时目录为空是预期行为。

---

## 四、扩展铁律

> **任何带 "minimal*" 前缀的函数（`minimalMP4` / `minimalPNG` / `minimalMP3` 等）都不能只生成 header 字节。**
> **header-only 的字节会让所有依赖完整 box tree 的下游（解码器、解密器、AI 解析器）静默失败。**

如需最简"合法"字节，使用 ffmpeg 或 [src/lib/mockDataGenerator.ts](file:///workspace/app/encv-mobile/src/lib/mockDataGenerator.ts) `createMP4/MKV/MP3/FLAC`，不允许 hand-roll header。

**裸 header 假数据的根因链 + 解密路径全失败链路 + ffmpeg 真机兼容矩阵** → 详见 [详情文档 §五/§六](../rule-library/mock-data-architecture.md#五为什么-mock-假数据会让所有解密路径失败)

---

## 五、引用其他规则

- [test.md](./test.md) — Mock 文件系统规范
- [development.md](./development.md) — Mobile Overlay 触发条件（mock 生成的 servingDir 上下文）

> 拆分：2026-06-11
