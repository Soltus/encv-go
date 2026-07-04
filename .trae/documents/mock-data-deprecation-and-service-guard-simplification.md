# Mock 数据系统废弃 + Service-Guard 简化 Plan

> **核心目标**：彻底废弃整套 mock 数据生成（Node CLI + 后端 API + 前端 lib + 3 套字节同步铁律），service-guard 简化为"挂载路径检查"，不再有 01-plain-media marker / 重复生成 / 历史遗留。

---

## 一、Current State → Target State

| 维度 | 当前 | 目标 |
|------|------|------|
| **mock 数据生成** | 3 套（后端 / CLI / 前端 lib）+ 5 个调用入口 | **0 套 / 0 入口** — 完全废弃 |
| **service-guard 检查** | 找 01-plain-media marker + 4 个子目录 | **只查 `servingDir === /storage/emulated/0`** |
| **mock 字节** | 后端 base64 + CLI ffmpeg + 前端 lib 3 套必须同源 | 不需要任何 mock 字节 |
| **servingDir** | 期望 `/storage/emulated/0`（mobile overlay 生效） | 同左（依赖 pm2 env 透传） |
| **02-test-output** | 自动化测试运行产物（578 条目） | **保留**（用户可能想看历史） |
| **automation sourcePath** | DEFAULT_AUTOMATION_SOURCE 写死 encv-automation 命名空间 | 改为从 service-guard 返回的 servingDir 派生；真机安全改写仍走 withSafetyBoundary |
| **start-preview.sh step 2** | 跑 `npx tsx scripts/generate-mock-files.ts` | **删 step 2b/6**（不生成 mock） |
| **preflight ensureMockData** | gateway 启动时自动调 CLI | **删整个 preflight 模块** |
| **`__mock_data__`** | dev 隔离层 | **彻底删**（物理目录 + 代码 + 测试 + 注释） |

---

## 二、Proposed Changes

### Phase 1: 后端 - service-guard 简化 + 整套 mock API 删除

#### 1.1 [internal/server/mobile_api.go](file:///workspace/internal/server/mobile_api.go) — 简化 handleServiceGuardGin

**L150-330 改写**：从"查 01-plain-media marker + 4 个子目录" → "只查 servingDir === /storage/emulated/0"

```go
// handleServiceGuardGin 处理 GET /api/service-guard
//
// 2026-06-10 简化：只检查 servingDir 是否挂载到 /storage/emulated/0（mobile 真机 / dev preview 的标准路径）。
// 不再检查 01-plain-media marker —— 整套 mock 数据生成系统已废弃（用户负责放真文件）。
func (s *Server) handleServiceGuardGin(c *gin.Context) {
    servingDir := s.servingDir
    expectedDir := "/storage/emulated/0"
    
    // 1. servingDir 必须是绝对路径
    absDir, err := filepath.Abs(servingDir)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "ready": false,
            "detail": fmt.Sprintf("servingDir 解析失败: %v", err),
        })
        return
    }
    
    // 2. 校验：必须 == /storage/emulated/0
    if absDir != expectedDir {
        c.JSON(http.StatusForbidden, gin.H{
            "ready":        false,
            "servingDir":   absDir,
            "expected":     expectedDir,
            "detail":       fmt.Sprintf("servingDir=%q 不是 mobile 真机/预览标准路径 %q", absDir, expectedDir),
            "remediation": []gin.H{
                {
                    "scenario": "B1 — 用 mobile overlay 启动",
                    "command":  "make dev-mobile",
                    "explain":  "自动 ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 → ApplyMobileOverlay → servingDir=/storage/emulated/0",
                },
                {
                    "scenario": "B2 — 手工等价命令",
                    "command":  "ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start",
                    "explain":  "同上但手工设 env",
                },
            },
        })
        return
    }
    
    // 3. 校验：目录必须可读
    if _, err := os.Stat(absDir); err != nil {
        c.JSON(http.StatusForbidden, gin.H{
            "ready":      false,
            "servingDir": absDir,
            "detail":     fmt.Sprintf("servingDir 不可读: %v", err),
        })
        return
    }
    
    // ✅ 一切就绪
    c.JSON(http.StatusOK, gin.H{
        "ready":      true,
        "servingDir": absDir,
        "expected":   expectedDir,
    })
}
```

**清理删除**（mobile_api.go）：
- L188-260 删 `envDevPreview` / `envMobile` / mobile overlay 错误提示（合并到上面的 remediation）
- L260-330 删 01-plain-media marker 检查
- L335-360 删 markerChildren 列表
- 删 mockScriptRel / previewScriptRel / mockScript 字段

#### 1.2 [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) — 整个文件删除

- `rm internal/server/mock_generator.go`
- 删除原因：service-guard 不再查 mock，后端 API `/api/mock/generate` `/api/mock/reset` 也不再有调用方
- 同步删：
  - `internal/server/mock_generator_test.go`（连同测试）
  - `internal/server/mock_media_bytes.go`（内嵌 base64 字节）
  - `internal/server/server.go:337-338` 两行 route 注册

#### 1.3 [internal/server/server.go](file:///workspace/internal/server/server.go)

**L337-338 删**：
```diff
- r.POST("/api/mock/generate", s.handleMockGenerateGin)
- r.POST("/api/mock/reset", s.handleMockResetGin)
```

---

### Phase 2: 前端 - 删 3 套 mock lib + 收口 UI 按钮

#### 2.1 删 Node CLI 脚本
```bash
rm /workspace/app/encv-mobile/scripts/generate-mock-files.ts
# 同步清：
#   - 该文件的任何 import / spec 引用
#   - 相关 e2e 测试
```

#### 2.2 删前端 mock 字节 lib
```bash
rm /workspace/app/encv-mobile/src/lib/mockDataGenerator.ts
rm /workspace/app/encv-mobile/src/lib/__tests__/mockDataGenerator.test.ts
```

#### 2.3 删前端 API wrapper
```bash
rm /workspace/app/encv-mobile/src/api/mockGenerator.ts
```

#### 2.4 删前端 mock 降级 Vite plugin
```bash
rm /workspace/app/encv-mobile/mock/index.ts
# 同步检查 vite.config.ts 是否注册了 encv-mock-api plugin
```

#### 2.5 删 e2e 测试（强依赖 encv-automation mock 路径）
```bash
rm /workspace/app/encv-mobile/src/composables/__tests__/path-chain-e2e.test.ts
```

#### 2.6 [app/encv-mobile/src/api/encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts) — checkServiceGuard 简化

**L381-407 改写**：
```typescript
export interface ServiceGuardResult {
  ready: boolean
  servingDir: string
  expected: string
  detail?: string
  error?: string
}

export async function checkServiceGuard(): Promise<ServiceGuardResult> {
  console.info('[API] checkServiceGuard')
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/api/service-guard`)
  const data: ServiceGuardResult = await response.json()

  if (!data.ready) {
    const err = new Error(`ServiceGuard: ${data.detail}`) as Error & { code: string; payload: ServiceGuardResult }
    err.code = 'SERVICE_GUARD_BLOCKED'
    err.payload = data
    console.error('[API] checkServiceGuard BLOCKED —', data.detail)
    throw err
  }

  console.info('[API] checkServiceGuard OK — servingDir:', data.servingDir)
  return data
}
```

**L388 type 简化**：删 `marker?` / `found?` / `hint?` 字段（不再返回）

#### 2.7 [app/encv-mobile/src/composables/useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) — DEFAULT_AUTOMATION_SOURCE 改来源

**L77 改写**：
```typescript
// 不再写死 /storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4
// 改为：servingDir 由 checkServiceGuard 返回；用户配置 source 路径（或用首条 /api/files 列表的 mp4）
// 这里保留为 fallback 常量，但建议从 useApiBase().servingDir 派生
export const DEFAULT_AUTOMATION_SOURCE_FALLBACK = '/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4'
```

**L77 重命名**：`DEFAULT_AUTOMATION_SOURCE` → `DEFAULT_AUTOMATION_SOURCE_FALLBACK`（保持 export 兼容）

**或者更激进**：完全删 DEFAULT_AUTOMATION_SOURCE，AutomationTestsDetail.vue 必须从 service-guard 拿 servingDir + 用户在 UI 选 source 文件

#### 2.8 [app/encv-mobile/src/views/AutomationTestsDetail.vue](file:///workspace/app/encv-mobile/src/views/AutomationTestsDetail.vue) — 删生成/重置按钮

**改动**：
- L42-90 删 "Generate Mock" / "Reset Mock" 两个 ion-item 按钮
- L195-220 删 `import { generateMockFilesViaBackend, resetMockFilesViaBackend }`
- L224-230 删 `mockRoot` / `isGenerating` / `isResetting` / `mockStats` / `generateProgressText` / `mockGenerated`
- L334-375 删 `handleGenerateMock` / `handleResetMock`
- L99 改 `:disabled="isRunning || dynamicTestCases.length === 0 || !mockGenerated"` 去掉 `!mockGenerated`
- L101 改 icon color 不依赖 `mockGenerated`
- L105 删 `<span v-if="!mockGenerated" style="color: var(--ion-color-danger)">⚠ 请先生成 Mock 数据</span>`
- L368-388 改 mockRoot 来源：`const mockRoot = computed(() => checkServiceGuardResult.servingDir + '/encv-automation/')`

#### 2.9 [app/encv-mobile/src/views/WorkflowDashboard.vue](file:///workspace/app/encv-mobile/src/views/WorkflowDashboard.vue) — 同样删按钮

同上模式。

#### 2.10 [app/encv-mobile/src/App.vue](file:///workspace/app/encv-mobile/src/App.vue) — runServiceGuard 错误展示简化

**L300-310 改写**：
- 不再显示"生成 mock 数据"步骤
- 改为"用 mobile overlay 启动 / 检查 /storage/emulated/0 是否挂载"

---

### Phase 3: i18n 更新

#### 3.1 [app/encv-mobile/src/i18n/common.ts](file:///workspace/app/encv-mobile/src/i18n/common.ts) — 改写 `app.serviceGuardMessage`

**L258 (zh) 改写**：
```diff
- 'app.serviceGuardMessage': '后端服务目录未包含 mock 数据（缺少 01-plain-media）。Capacitor 预览需要后端 server.dir 指向 mock 数据目录。正确启动步骤：\n\n1. 生成 mock 数据：\ncd app/encv-mobile && npx tsx scripts/generate-mock-files.ts --dir /storage/emulated/0\n\n2. 启动后端（mobile overlay 自动生效）：\nENCV_DEV_PREVIEW=1 go run ./cmd/encv-mobile/\n\n3. 启动 Vite 前端：\nnpx vite --host 0.0.0.0\n\n注意：不要用 ENCV_CONFIG_PATH，不要改 config.user.json，不要用 npx cap serve。',
+ 'app.serviceGuardMessage': '后端 servingDir 不是 mobile 真机/预览标准路径 /storage/emulated/0。正确启动步骤：\n\n1. 用 mobile overlay 启动：\nmake dev-mobile\n\n2. 或手工等价命令：\nENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start\n\n注意：不要用 ENCV_CONFIG_PATH，不要改 config.user.json，不要用 npx cap serve。',
```

**L517 (en) 同步改写**：
```diff
- 'app.serviceGuardMessage': 'Backend service directory does not contain mock data (missing 01-plain-media). Capacitor preview requires server.dir to point to mock data. Correct startup steps:\n\n1. Generate mock data:\ncd app/encv-mobile && npx tsx scripts/generate-mock-files.ts --dir /storage/emulated/0\n\n2. Start backend (mobile overlay auto-applies):\nENCV_DEV_PREVIEW=1 go run ./cmd/encv-mobile/\n\n3. Start Vite frontend:\nnpx vite --host 0.0.0.0\n\nNote: Do NOT use ENCV_CONFIG_PATH, do NOT modify config.user.json, do NOT use npx cap serve.',
+ 'app.serviceGuardMessage': 'Backend servingDir is not the mobile device/preview standard path /storage/emulated/0. Correct startup steps:\n\n1. Start with mobile overlay:\nmake dev-mobile\n\n2. Or manual equivalent:\nENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start\n\nNote: Do NOT use ENCV_CONFIG_PATH, do NOT modify config.user.json, do NOT use npx cap serve.',
```

#### 3.2 删 devtools.mockRoot / devtools.generateMock / devtools.resetMock 等 i18n 键
（全仓 grep 用到的地方都删）

---

### Phase 4: 启动脚本 / 配置文件清理

#### 4.1 [app/encv-mobile/scripts/start-preview.sh](file:///workspace/app/encv-mobile/scripts/start-preview.sh) — 删 step 2

**L101-115 改写**：
```diff
- # ---------- Step 2: 生成 mock 数据 ----------
- # 沙箱首次启动时 ${MOCK_DIR} 还不存在（root 权限下可建），先 mkdir 再生成。
- if [[ ! -d "${MOCK_DIR}" ]]; then
-     step "2a/6 创建 mock 根目录 ${MOCK_DIR}（沙箱首次启动）"
-     mkdir -p "${MOCK_DIR}" || { echo "❌ 无法创建 ${MOCK_DIR}" >&2; exit 1; }
- fi
- step "2b/6 生成 mock 数据到 ${MOCK_DIR}"
- cd "${MOBILE_DIR}"
- npx tsx scripts/generate-mock-files.ts
- cd "${REPO_ROOT}"
-
- if [[ ! -d "${MOCK_DIR}/01-plain-media" ]]; then
-     echo "❌ 错误：mock 生成后仍缺少 ${MOCK_DIR}/01-plain-media 标记目录" >&2
-     exit 1
- fi
+ # ---------- Step 2: 不再生成 mock 数据 ----------
+ # 2026-06-10 改造：mock 数据系统废弃。service-guard 只查 servingDir == /storage/emulated/0
+ # 用户负责放真文件（或自动化测试 case 自己提供）。
+ # 不再 mkdir ${MOCK_DIR}（系统/真机已有此目录）
+ # 不再跑 npx tsx scripts/generate-mock-files.ts（脚本已删）
+ # 不再 verify ${MOCK_DIR}/01-plain-media（marker 已废弃）
```

**Step 编号**：从 2/6 → 2/5（其他 step 编号不变或重新数）

#### 4.2 [Makefile](file:///workspace/Makefile) — 简化 dev-mobile

**L33-37 改写**：
```diff
  dev-mobile:
- 	@echo "Generating mock data to mobile server.dir..."
- 	@cd app/encv-mobile && npx tsx scripts/generate-mock-files.ts --dir /storage/emulated/0
- 	@echo "Starting backend (mobile preview mode)..."
+ 	@echo "Starting backend (mobile preview mode)..."
  	ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start
```

#### 4.3 [ecosystem.config.cjs](file:///workspace/ecosystem.config.cjs) — 删 SKIP_MOCK_GEN / MOCK_ROOT env

**L77 / L83-86 改写**：
```diff
  env: {
      ...
-     SKIP_MOCK_GEN: '0',     // 1=跳过 mock 数据生成（CI 或预热场景）
      ...
-     // 真机规范：mock 数据直接写在 servingDir 下（mobile.server.dir=/storage/emulated/0）
-     // ⚠️ 不要用 encv-automation 子目录，那是「自动化测试入口」的命名空间，
-     //    不影响「启动后 mock 就位」（service-guard 检查的是 servingDir 根下的 01-plain-media）
-     ENCV_MOCK_ROOT: '/storage/emulated/0',
  }
```

#### 4.4 [app/preview-gateway/src/preflight.ts](file:///workspace/app/preview-gateway/src/preflight.ts) — 删 ensureMockData

**整个文件** 或 **L1-108 ensureMockData 函数**：
- 删 `ensureMockData` 导出
- 删 `isMockDataPresent` 内部函数
- 删 `runSync` 子进程封装（如果只 preflight 用，整个文件删）
- 删 `spawn` / `execSync` / `readdirSync` 等 import

**或者激进**：整个 preflight.ts 删（删 noop 也不影响主流程）

#### 4.5 [app/preview-gateway/src/server.ts](file:///workspace/app/preview-gateway/src/server.ts) — 删 Step 2 preflight 调用

**L48 / L514-519 改写**：
```diff
- import { ensureMockData } from './preflight.js'
  ...
- // ── Step 2: preflight（mock 数据生成）──
- if (process.env.SKIP_MOCK_GEN !== '1') {
-     await ensureMockData(paths.mobileDir)
- } else {
-     log('SKIP_MOCK_GEN=1, skipping mock data generation')
- }
+ // ── Step 2: （废弃）mock 数据生成 preflight —— 2026-06-10 整套 mock 系统废弃，service-guard 只查挂载路径 ──
+ log('(preflight skipped: mock data system deprecated)')
```

---

### Phase 5: 规范文档

#### 5.1 [mock-data-architecture.md](file:///workspace/.trae/rules/mock-data-architecture.md) — 整篇废弃

**整篇改写**（或重命名为 `mock-data-architecture.deprecated.md`）：
```markdown
# ⚠️ DEPRECATED — 2026-06-10 整套 mock 数据系统已废弃

历史回顾：本规则曾经定义"3 套 mock 数据生成逻辑必须同源"（后端 Go / Node CLI / 前端 lib），
5 个调用入口（开发者选项按钮 / 自动化测试 setup / Workflow Dashboard / Node CLI / 前端 mock 降级）。

2026-06-10 改造：
- 3 套 mock lib 全部删除（scripts/generate-mock-files.ts、src/lib/mockDataGenerator.ts、internal/server/mock_generator.go、internal/server/mock_media_bytes.go、app/encv-mobile/mock/index.ts）
- 后端 /api/mock/generate + /api/mock/reset 删除
- service-guard 简化为"只查 servingDir == /storage/emulated/0"
- 真机/沙箱 dev 都用真文件（用户自己准备）

新规则：[preview-gateway.md §Service-Guard 简化] 待建 / [mobile-overlay.md §servingDir 约束]
```

#### 5.2 [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) — 删 mock 相关

**改动**：
- §四 ext → 目录分类映射 整段删（不再有 mock 数据）
- §八 "测试用例数估算" 删
- §五 "StepRun 字段扩展" 保留
- §十一 bug 列表保留（与 mock 无关）
- §十二 bug 列表保留
- 加新章节："mock 系统废弃后，自动化测试 sourcePath 怎么算"
  - servingDir 来自 checkServiceGuard
  - 自动化测试 namespace = `${servingDir}/encv-automation/`
  - source 文件 = 用户在 UI 选（走 /api/files 浏览 /storage/emulated/0/encv-automation/）

#### 5.3 新建 [service-guard-simplified.md](file:///workspace/.trae/rules/service-guard-simplified.md)（可选）

或者把上面 Phase 1.1 的内容写到 .trae/rules/ 下面。

---

### Phase 6: pm2 env 透传 + 重启 backend

#### 6.1 诊断
```bash
# 1. 看 pm2 env
pm2 show preview-gateway | grep -A 20 'env'

# 2. 看 gateway spawn air 时 env
grep -n 'env' app/preview-gateway/src/server.ts | head -20

# 3. 看 .air-run.sh
cat .air-run.sh
```

#### 6.2 修复（按根因）
- A. `preview-gateway/src/server.ts` spawn air 时显式 `env: { ...process.env, ENCV_MOBILE: '1', ENCV_DEV_PREVIEW: '1' }`
- B. `.air-run.sh` 加 `export ENCV_MOBILE ENCV_DEV_PREVIEW`
- C. air.toml 加 `run.env` 段

#### 6.3 pm2 restart
```bash
pm2 restart preview-gateway
sleep 8  # 等 air rebuild + encv-go 启动
curl -s http://127.0.0.1:2025/api/service-guard | jq .
# 期望：ready=true, servingDir=/storage/emulated/0
```

#### 6.4 物理清理（destructive，必须确认后执行）
```bash
# 删 dev 历史
rm -rf /workspace/__mock_data__/
# 删上轮擅自写到 /workspace 的（错）
rm -rf /workspace/01-plain-media/ /workspace/02-alist-encrypt/ /workspace/03-encv-containers/ /workspace/04-boundary-test/
# 删 mock 系统废弃后无用的 17:18 老数据
rm -rf /storage/emulated/0/01-plain-media/ /storage/emulated/0/02-alist-encrypt/ /storage/emulated/0/03-encv-containers/ /storage/emulated/0/04-boundary-test/
rm -rf /storage/emulated/0/encv-automation/01-plain-media/ /storage/emulated/0/encv-automation/02-alist-encrypt/ /storage/emulated/0/encv-automation/03-encv-containers/ /storage/emulated/0/encv-automation/04-boundary-test/
# 保留 /storage/emulated/0/encv-automation/02-test-output/（用户测试产物）
```

---

## 三、Verification Steps

### 3.1 编译/类型检查
```bash
cd /workspace
# Go 后端
go build ./...
go vet ./...
# 前端
cd app/encv-mobile
npx vue-tsc --noEmit
npx vitest run
```

### 3.2 service-guard 验证
```bash
curl -s http://127.0.0.1:2025/api/service-guard | jq .
# 期望：
# {
#   "ready": true,
#   "servingDir": "/storage/emulated/0",
#   "expected": "/storage/emulated/0"
# }
```

### 3.3 mock API 验证（应 404）
```bash
curl -i -X POST http://127.0.0.1:2025/api/mock/generate -d '{}' -H 'Content-Type: application/json'
# 期望：HTTP/1.1 404 Not Found
```

### 3.4 自动化测试入口验证
- 打开 AutomationTestsDetail.vue
- 看不到 "Generate Mock" / "Reset Mock" 按钮
- "Run Workflow" 按钮不依赖 mockGenerated
- sourcePath 走 service-guard 返回的 servingDir 派生

### 3.5 OpenPreview 重发
按 preview-management.md 协议，链接 = `http://localhost:16666/`

---

## 四、风险评估

| 风险 | 缓解 |
|------|------|
| 自动化测试无 mock 数据可跑 | UI 加"无源文件"warning；source 文件走 /api/files 列表让用户选 |
| 删 mock lib 后 e2e 测试失败 | 同步删强依赖测试（path-chain-e2e.test.ts / mockDataGenerator.test.ts） |
| pm2 env 透传修不对，service-guard 仍 BLOCKED | 备选：先 pkill 旧 backend，`ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start` 后台 |
| mockRoot 计算改成 service-guard 派生，但 App.vue 启动早于 service-guard 返回 | 用 ref + 异步赋值；UI 显示 loading 态 |
| 用户之前有 cf 化测试报告（依赖 mock 数据） | 用户接受"重新设计"，无回退需求 |
| 删除 `__mock_data__` 后 `mockGenerator.ts` 还在引用 | grep 全仓确认 |

---

## 五、执行顺序（依赖图）

```
Phase 1.1 (service-guard 简化)
    ↓
Phase 1.2 (删 mock_generator.go 等 3 个文件)
    ↓
Phase 2.1-2.5 (删 Node CLI + 前端 lib + Vite plugin + e2e)
    ↓
Phase 2.6 (checkServiceGuard TS 简化)
    ↓
Phase 2.7-2.10 (删 UI 按钮 + 改 mockRoot 来源)
    ↓
Phase 3 (i18n 改写)
    ↓
Phase 4.1-4.5 (start-preview.sh / Makefile / ecosystem / preflight / server.ts)
    ↓
Phase 5 (规范文档废弃)
    ↓
Phase 6 (pm2 env 透传 + 重启 + 物理清理)
    ↓
Verification + OpenPreview
```

**关键路径**：1.1 → 1.2 → 6.2 → 6.3 → Verification（后端 service-guard 真正能 OK）

---

## 六、跨层参考

| 主题 | 文档位置 |
|------|---------|
| mobile overlay 触发 | [internal/config/config.go:289-311](file:///workspace/internal/config/config.go#L289-L311) |
| Makefile dev-mobile | [Makefile:33-37](file:///workspace/Makefile#L33-L37) |
| start-preview.sh | [app/encv-mobile/scripts/start-preview.sh](file:///workspace/app/encv-mobile/scripts/start-preview.sh) |
| ecosystem.config.cjs | [ecosystem.config.cjs](file:///workspace/ecosystem.config.cjs) |
| withSafetyBoundary | [app/encv-mobile/src/composables/usePathResolver.ts](file:///workspace/app/encv-mobile/src/composables/usePathResolver.ts) |
| checkServiceGuard | [app/encv-mobile/src/api/encv.ts:391-407](file:///workspace/app/encv-mobile/src/api/encv.ts#L391-L407) |
| DEFAULT_AUTOMATION_SOURCE | [app/encv-mobile/src/composables/useAutomationTests.ts:77](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L77) |
| 3 套 mock lib（已废） | [mock-data-architecture.md](file:///workspace/.trae/rules/mock-data-architecture.md) |
| 自动化测试工作流 | [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) |
