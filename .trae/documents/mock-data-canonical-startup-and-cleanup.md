# Mock 生成收口 + Service-Guard 简化 Plan（修订版）

> **核心目标**：用户原话 = "去掉 nodejs 生成脚本 + service-guard 只查挂载路径 + 不再检查 mock 数据"。
> **保留**：后端 `/api/mock/generate` API（真机 release 必需）+ 前端 `src/lib/mockDataGenerator.ts`（前端运行时）+ 前端两个 UI 按钮（用户主动点击）。
> **删除**：Node CLI 脚本 + 所有引用（preflight / start-preview.sh / Makefile / ecosystem / mock/index.ts Vite plugin 用的也是 CLI）。
> **简化**：service-guard 从"找 01-plain-media marker + 4 个子目录" → "只查 `servingDir === /storage/emulated/0`"。

---

## 一、Current State → Target State

| 维度 | 当前 | 目标 |
|------|------|------|
| **Node CLI 脚本** | `app/encv-mobile/scripts/generate-mock-files.ts`（31 文件，1.2MB，ffmpeg + lib） | **删除** |
| **后端 /api/mock/generate** | 真机 release 必需 | **保留** + 加 X-Confirm-Mock-Mutation header 强校验（防擅自生成） |
| **后端 /api/mock/reset** | 真机 release 必需 | **保留** + 同样 header 校验 |
| **后端 mock_generator.go** | minimalMP4/MKV/MP3/FLAC (ffmpeg 优先 + base64 fallback) | **保留** |
| **前端 src/lib/mockDataGenerator.ts** | 单元测试 / 运行时 | **保留** |
| **前端 src/api/mockGenerator.ts** | UI 按钮 wrapper | **保留** + 自动带 confirm header |
| **前端 mock/index.ts（Vite plugin）** | dev 时 /decrypt mock 中间件 + `ensureMockDataExists` 调 CLI | **删 Vite plugin 整个文件**（CLI 已删） |
| **前端 2 个 UI 按钮** | AutomationTestsDetail / WorkflowDashboard | **保留**（用户主动点击，自动带 confirm header） |
| **service-guard 检查** | 找 01-plain-media marker + 4 个子目录 | **只查 `servingDir === /storage/emulated/0`** |
| **`__mock_data__` 历史** | mockRootAllowList 残留 + 物理目录 + 注释 | **彻底删**（dev 隔离层已无意义） |
| **preflight gateway** | 启动时自动调 CLI 写 mock | **删**（CLI 已删，gateway 不再写 mock） |
| **start-preview.sh step 2** | 调 CLI 生成 mock | **删** |
| **Makefile dev-mobile** | 调 CLI 生成 mock | **删 CLI step** |
| **ecosystem.config.cjs** | SKIP_MOCK_GEN / ENCV_MOCK_ROOT env | **删**（不再需要） |
| **02-test-output 产物** | 自动化测试运行产物 | **保留**（用户测试产物） |

---

## 二、Proposed Changes（按执行顺序）

### Phase 1: 删 Node CLI 脚本 + 全链路引用

#### 1.1 物理删除脚本
```bash
rm /workspace/app/encv-mobile/scripts/generate-mock-files.ts
```

#### 1.2 [Makefile](file:///workspace/Makefile) — 简化 dev-mobile
```diff
  dev-mobile:
- 	@echo "Generating mock data to mobile server.dir..."
- 	@cd app/encv-mobile && npx tsx scripts/generate-mock-files.ts --dir /storage/emulated/0
- 	@echo "Starting backend (mobile preview mode)..."
+ 	@echo "Starting backend (mobile preview mode, no mock pre-generation)..."
  	ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start
```

#### 1.3 [app/encv-mobile/scripts/start-preview.sh](file:///workspace/app/encv-mobile/scripts/start-preview.sh) — 删 step 2

**L101-115 改写**（step 2 整段删，step 编号重排为 0/5, 1/5, 2/5, 3/5, 4/5）：
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
+ # ---------- Step 2:（已废弃）生成 mock 数据 ----------
+ # 2026-06-10：Node CLI 脚本已删。mock 数据改由后端 /api/mock/generate 提供（用户主动点 UI 按钮）。
+ # service-guard 不再检查 01-plain-media marker，只查 servingDir == /storage/emulated/0。
+ # 用户没主动按"生成 Mock"按钮时，目录是空的——这是预期行为。
```

**L50 MOCK_DIR env 同步删**：
```diff
- MOCK_DIR="${ENCV_MOCK_ROOT:-/storage/emulated/0}"
```

**Step 编号重排**：6 → 5（其他 step 编号同步改）

#### 1.4 [app/preview-gateway/src/preflight.ts](file:///workspace/app/preview-gateway/src/preflight.ts) — 删 ensureMockData

**整个文件简化**（保留文件但 ensureMockData 改成 noop stub，避免 gateway server.ts 引用爆错）：
```typescript
/**
 * preflight.ts — 2026-06-10 改造：mock 数据生成已废弃
 *
 * 历史职责：gateway 启动前自动调 `npx tsx scripts/generate-mock-files.ts` 写 mock。
 * 当前职责：no-op（保留文件以保持 module 路径兼容；调用方若传 ensureMockData() 也直接 resolve）
 */
export async function ensureMockData(_mobileDir: string): Promise<void> {
  // no-op: mock 数据由用户主动调后端 /api/mock/generate 生成
  return
}
```

#### 1.5 [app/preview-gateway/src/server.ts](file:///workspace/app/preview-gateway/src/server.ts) — 简化 preflight 调用

**L514-519 改写**：
```diff
- // ── Step 2: preflight（mock 数据生成）──
- if (process.env.SKIP_MOCK_GEN !== '1') {
-     await ensureMockData(paths.mobileDir)
- } else {
-     log('SKIP_MOCK_GEN=1, skipping mock data generation')
- }
+ // ── Step 2:（已废弃）preflight mock 生成 —— 2026-06-10 改造：mock 数据由用户主动生成，gateway 不再自动写盘 ──
+ log('(preflight: noop, mock data is generated on-demand via UI button /api/mock/generate)')
```

#### 1.6 [ecosystem.config.cjs](file:///workspace/ecosystem.config.cjs) — 删 SKIP_MOCK_GEN / ENCV_MOCK_ROOT

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

**L77-78 / L83-86 注释同步删**。

#### 1.7 [app/encv-mobile/mock/index.ts](file:///workspace/app/encv-mobile/mock/index.ts) — 删整个文件

```bash
rm /workspace/app/encv-mobile/mock/index.ts
```

**vite.config.ts 同步检查**（grep `encv-mock-api` / `mock/index` import，删掉）：
```bash
grep -rn "mock/index" /workspace/app/encv-mobile/
grep -rn "encv-mock-api" /workspace/app/encv-mobile/
```

#### 1.8 [app/encv-mobile/src/composables/useAutomationTests.ts](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts) — 注释微调

**L77 + 注释**：DEFAULT_AUTOMATION_SOURCE 保留，但加注释说明：
```typescript
/**
 * 自动化测试默认源文件。
 * 真实运行时由后端 /api/mock/generate 生成（用户主动按 UI 按钮）。
 * mockRoot 计算 = DEFAULT_AUTOMATION_SOURCE.split('/').slice(0, 5).join('/') + '/'
 *            = /storage/emulated/0/encv-automation/
 */
export const DEFAULT_AUTOMATION_SOURCE = '/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4'
```

---

### Phase 2: 后端 service-guard 简化

#### 2.1 [internal/server/mobile_api.go](file:///workspace/internal/server/mobile_api.go) — handleServiceGuardGin 改写

**L150-360 整段重写**（从"找 01-plain-media marker + 4 个子目录" → "只查 servingDir"）：

```go
// handleServiceGuardGin 处理 GET /api/service-guard
//
// 2026-06-10 简化：只检查 servingDir 是否挂载到 /storage/emulated/0（mobile 真机 / dev preview 的标准路径）。
// 不再检查 01-plain-media marker —— mock 数据由用户主动调后端 /api/mock/generate 生成（带 X-Confirm-Mock-Mutation）。
// 不再期望任何特定子目录存在 —— 用户/真机可能没有 mock 文件。
func (s *Server) handleServiceGuardGin(c *gin.Context) {
    servingDir := s.servingDir
    
    // 1. 解析成绝对路径
    absDir, err := filepath.Abs(servingDir)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "ready":  false,
            "detail": fmt.Sprintf("servingDir 解析失败: %v", err),
        })
        return
    }
    
    expectedDir := "/storage/emulated/0"
    
    // 2. 必须 == /storage/emulated/0
    if absDir != expectedDir {
        envDevPreview := os.Getenv("ENCV_DEV_PREVIEW") == "1"
        envMobile := os.Getenv("ENCV_MOBILE") == "1"
        c.JSON(http.StatusForbidden, gin.H{
            "ready":          false,
            "servingDir":     absDir,
            "expected":       expectedDir,
            "envDevPreview":  envDevPreview,
            "envMobile":      envMobile,
            "detail":         fmt.Sprintf("servingDir=%q 不是 mobile 真机/预览标准路径 %q", absDir, expectedDir),
            "remediation": []gin.H{
                {
                    "scenario": "B1 — 用 mobile overlay 启动（推荐）",
                    "command":  "make dev-mobile",
                    "explain":  "自动 ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 → ApplyMobileOverlay → servingDir=/storage/emulated/0",
                },
                {
                    "scenario": "B2 — 手工等价命令",
                    "command":  "ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start",
                    "explain":  "同上但手工设 env（pm2 gateway spawn air 时确保透传）",
                },
            },
        })
        return
    }
    
    // 3. 目录必须可读
    if _, statErr := os.Stat(absDir); statErr != nil {
        c.JSON(http.StatusForbidden, gin.H{
            "ready":      false,
            "servingDir": absDir,
            "detail":     fmt.Sprintf("servingDir 不可读: %v", statErr),
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

**删除**（mobile_api.go）：
- L260-330 旧 marker 检查（hasMarker 变量、dirNames 列表、displayNames 截断）
- L332-360 旧 markerChildren 列表
- mockScriptRel / previewScriptRel 字段引用

---

### Phase 3: 后端 `__mock_data__` 死代码清理

#### 3.1 [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) — 删 dev 隔离层

**L9 注释**：
```diff
- //   - dev 模式：<project>/__mock_data__/01-plain-media 等
- //   - 真机：    /storage/emulated/0/encv-automation/01-plain-media 等
+ //   - 真机 / dev preview：<servingDir>/01-plain-media/ 等
+ //   - 自动化测试命名空间：<servingDir>/encv-automation/01-plain-media/ 等
```

**L53-62 mockRootAllowList**：
```diff
- // dev 模式：项目根 + "__mock_data__/"
- // 真机：/storage/emulated/0/encv-automation/
- // 其他路径一律 403。
- var mockRootAllowList = []string{
-     "__mock_data__",                                // dev: 相对项目根（运行时被转为绝对路径）
-     "/storage/emulated/0/encv-automation",         // 真机
-     "/sdcard/encv-automation",                     // 真机 symlink 兼容
-     "/data/local/tmp/encv-automation",             // 调试用
- }
+ // 允许写入的根目录白名单（绝对路径前缀）：
+ //   1. /storage/emulated/0（servingDir 根，给 Files 浏览器用）
+ //   2. /storage/emulated/0/encv-automation（自动化测试命名空间，withSafetyBoundary 改写后的目标）
+ //   3. /sdcard/encv-automation（真机 symlink 兼容）
+ //   4. /data/local/tmp/encv-automation（调试用）
+ // 其他路径一律 403。
+ var mockRootAllowList = []string{
+     "/storage/emulated/0",
+     "/storage/emulated/0/encv-automation",
+     "/sdcard/encv-automation",
+     "/data/local/tmp/encv-automation",
+ }
```

**L82-110 validateMockRoot**：
```diff
  func validateMockRoot(root string) error {
      if root == "" {
          return fmt.Errorf("root is empty")
      }
      clean := filepath.Clean(root)
-     if !filepath.IsAbs(clean) {
-         // dev 模式：相对路径转绝对
-         abs, err := filepath.Abs(clean)
-         if err != nil {
-             return fmt.Errorf("invalid root path: %w", err)
-         }
-         clean = abs
-     }
+     // 2026-06-10 改造：mockRoot 必须是绝对路径（已废弃 dev 模式相对路径）
+     if !filepath.IsAbs(clean) {
+         return fmt.Errorf("root %q must be absolute path (e.g. /storage/emulated/0)", root)
+     }
      ...
  }
```

#### 3.2 [internal/server/mock_generator_test.go](file:///workspace/internal/server/mock_generator_test.go) — 删 dev 用例

**L39 删**：
```diff
- {"mock_data_dev", "__mock_data__", true},
```

#### 3.3 [app/encv-mobile/src/api/mockGenerator.ts](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts) — 注释改

**L10-11**：
```diff
- * 安全：后端 white-list 校验 root 前缀（dev: __mock_data__/，真机: encv-automation/）。
+ * 安全：后端 white-list 校验 root 前缀（必须是绝对路径，在 /storage/emulated/0[/encv-automation] 等白名单内）。
+ * 显式意图：必须带 X-Confirm-Mock-Mutation header（防擅自生成）。
```

---

### Phase 4: 前端 UI / i18n 简化

#### 4.1 [app/encv-mobile/src/api/encv.ts](file:///workspace/app/encv-mobile/src/api/encv.ts) — checkServiceGuard TS 类型简化

**L381-389 ServiceGuardResult type**：
```typescript
export interface ServiceGuardResult {
  ready: boolean
  servingDir: string
  expected: string
  detail?: string
  error?: string
}
```

（删 `marker?` / `found?` / `hint?` 字段——后端不再返回）

#### 4.2 [app/encv-mobile/src/i18n/common.ts](file:///workspace/app/encv-mobile/src/i18n/common.ts) — serviceGuardMessage 改写

**L258 (zh) 改写**：
```diff
- 'app.serviceGuardMessage': '后端服务目录未包含 mock 数据（缺少 01-plain-media）。Capacitor 预览需要后端 server.dir 指向 mock 数据目录。正确启动步骤：\n\n1. 生成 mock 数据：\ncd app/encv-mobile && npx tsx scripts/generate-mock-files.ts --dir /storage/emulated/0\n\n2. 启动后端（mobile overlay 自动生效）：\nENCV_DEV_PREVIEW=1 go run ./cmd/encv-mobile/\n\n3. 启动 Vite 前端：\nnpx vite --host 0.0.0.0\n\n注意：不要用 ENCV_CONFIG_PATH，不要改 config.user.json，不要用 npx cap serve。',
+ 'app.serviceGuardMessage': '后端 servingDir 不是 mobile 真机/预览标准路径 /storage/emulated/0。正确启动步骤：\n\n1. 用 mobile overlay 启动（推荐）：\nmake dev-mobile\n\n2. 或手工等价命令：\nENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start\n\n注意：不要用 ENCV_CONFIG_PATH，不要改 config.user.json，不要用 npx cap serve。',
```

**L517 (en) 同步改写**（英文版同样改，不提 mock 数据生成）。

#### 4.3 [app/encv-mobile/src/App.vue](file:///workspace/app/encv-mobile/src/App.vue) — runServiceGuard 错误展示简化

**L300-310**：从"提示生成 mock" → "提示修 servingDir"。**提示模板**对应 i18n key 即可，无需硬编码。

---

### Phase 5: 后端 /api/mock/* 加 X-Confirm-Mock-Mutation 防护

#### 5.1 [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) — 两个 handler 加 header 校验

**handleMockGenerateGin L170 头部加**：
```diff
  func (s *Server) handleMockGenerateGin(c *gin.Context) {
+     // 🆕 2026-06-10：显式意图确认
+     //   - 防止 preflight / 第三方爬虫 / 误调触发数据生成
+     //   - 前端 UI 按钮自动带 X-Confirm-Mock-Mutation: yes
+     //   - Node CLI 已废弃，不存在自动调用方
+     if c.GetHeader("X-Confirm-Mock-Mutation") != "yes" {
+         slog.Warn("Mock generate rejected: missing confirm header")
+         c.JSON(http.StatusForbidden, gin.H{
+             "error": "X-Confirm-Mock-Mutation header required (UI 按钮自动带；防擅自生成)",
+         })
+         return
+     }
+
      var req mockGeneratorRequest
      ...
```

**handleMockResetGin L243 头部加**：同样 confirm header 校验。

#### 5.2 [internal/server/mock_generator_test.go](file:///workspace/internal/server/mock_generator_test.go) — 新增测试

```go
func TestMockGenerate_RequiresConfirmHeader(t *testing.T) {
    // 没带 header → 403 + error 提示
    // 带 yes → 200 + 正常生成
}

func TestMockReset_RequiresConfirmHeader(t *testing.T) {
    // 没带 header → 403
    // 带 yes → 200
}
```

#### 5.3 [app/encv-mobile/src/api/mockGenerator.ts](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts) — 前端 fetch 自动带 header

**L42-52 generateMockFilesViaBackend**：
```diff
  const res = await fetch(`${baseUrl}/api/mock/generate`, {
      method: 'POST',
      headers: {
          'Content-Type': 'application/json',
          'Accept': 'text/event-stream',
+         'X-Confirm-Mock-Mutation': 'yes',  // 🆕 显式意图确认（防擅自生成）
      },
      ...
```

**L93-105 resetMockFilesViaBackend**：同样加 header。

---

### Phase 6: 规范文档

#### 6.1 [.trae/rules/mock-data-architecture.md](file:///workspace/.trae/rules/mock-data-architecture.md) — 改 §一 / §七

**§一 3 套实现清单**：
```diff
- | [app/encv-mobile/scripts/generate-mock-files.ts](file:///workspace/app/encv-mobile/scripts/generate-mock-files.ts) `createValidMP4/MKV/MP3/FLAC` | Node CLI 调 ffmpeg 生成 | ✅ ffmpeg 优先生成可播放媒体 + base64 fallback |
+ | ❌ Node CLI 已删（2026-06-10）| — | — |
```

**§七 调用入口**：
```diff
  ## 七、调用入口

+ ### 7.1 显式意图确认（防擅自生成）
+
+ **铁律**：调后端 `/api/mock/generate` 或 `/api/mock/reset` 必须带 `X-Confirm-Mock-Mutation: yes` header。
+ 后端 403 拒绝没带 header 的请求。
+
+ | 调用方 | 带 header 吗？ | 备注 |
+ |--------|---------------|------|
+ | 前端 UI 按钮（AutomationTestsDetail / WorkflowDashboard） | ✅ 自动带 | 用户主动点击 |
+ | 第三方爬虫 / 误调 | ❌ 无 header | 403 拒绝 |
+ | Node CLI | N/A | 2026-06-10 已废弃 |
+
+ ### 7.2 历史：`__mock_data__` 已废弃
+
+ 2026-06-10 改造：dev 隔离层 `<project>/__mock_data__/` 已从 mockRootAllowList / 物理目录 / 注释全链路清除。
+ mockRoot 必须是绝对路径，dev 模式相对路径传参直接 400。
```

#### 6.2 [.trae/rules/automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) — 加 note

加一句：
> **2026-06-10 note**：service-guard 不再查 mock 数据。自动化测试 sourcePath 仍走 `withSafetyBoundary({forceAutomation:true})` 强制改写到 `<servingDir>/encv-automation/`。**用户必须先主动按"生成 Mock"按钮**（带 X-Confirm-Mock-Mutation header）才能跑自动化测试。

---

### Phase 7: pm2 env 透传 + 重启 backend

#### 7.1 诊断
```bash
# 1. 看 pm2 env（应该已经有 ENCV_MOBILE/ENCV_DEV_PREVIEW）
pm2 show preview-gateway | grep -A 20 'env'

# 2. 看 gateway spawn air 时 env
grep -n 'env' /workspace/app/preview-gateway/src/server.ts | head -20
# 看 buildChildSpecs / spawnSubprocess 实现

# 3. 看 .air-run.sh
cat /workspace/.air-run.sh
```

#### 7.2 修复（按根因）

3 个候选：
- **A**: `preview-gateway/src/server.ts` spawn air 时没传 `env: process.env` —— 修复：显式 `env: { ...process.env, ENCV_MOBILE: '1', ENCV_DEV_PREVIEW: '1' }`
- **B**: `.air-run.sh` 没 export env —— 修复：加 `export ENCV_MOBILE=${ENCV_MOBILE:-1}` / `export ENCV_DEV_PREVIEW=${ENCV_DEV_PREVIEW:-1}`
- **C**: air.toml 加 `run.env`

定位后针对性修。

#### 7.3 pm2 restart
```bash
pm2 restart preview-gateway
sleep 8  # 等 air rebuild + encv-go 启
curl -s http://127.0.0.1:2025/api/service-guard | jq .
# 期望：ready=true, servingDir=/storage/emulated/0, envDevPreview=true, envMobile=true
```

---

### Phase 8: 物理清理

```bash
# 1. 删 dev 历史
rm -rf /workspace/__mock_data__/

# 2. 删上轮擅自写到 /workspace 的（错）
rm -rf /workspace/01-plain-media/ \
       /workspace/02-alist-encrypt/ \
       /workspace/03-encv-containers/ \
       /workspace/04-boundary-test/

# 3. 删 17:18 老 mock 数据（重生）
rm -rf /storage/emulated/0/01-plain-media/ \
       /storage/emulated/0/02-alist-encrypt/ \
       /storage/emulated/0/03-encv-containers/ \
       /storage/emulated/0/04-boundary-test/

# 4. 删 automation 命名空间老 mock（重生）
rm -rf /storage/emulated/0/encv-automation/01-plain-media/ \
       /storage/emulated/0/encv-automation/02-alist-encrypt/ \
       /storage/emulated/0/encv-automation/03-encv-containers/ \
       /storage/emulated/0/encv-automation/04-boundary-test/

# 5. 保留 /storage/emulated/0/encv-automation/02-test-output/（用户测试产物）
```

---

### Phase 9: 重新生成 mock 数据（用户主动调后端 API）

#### 9.1 UI 按钮触发（用户自己点）

AutomationTestsDetail.vue / WorkflowDashboard.vue 的"生成 Mock"按钮 → 调 `/api/mock/generate {root: /storage/emulated/0, type: all}`（自动带 confirm header）→ SSE 流式生成 20 文件 / 1.2 MB 到 `/storage/emulated/0/01-plain-media/`。

或者：
#### 9.2 直接 curl（不走 UI 也要带 header）
```bash
# 生成到 /storage/emulated/0/01-plain-media/（service-guard 之前找的位置，现在只作 Files 浏览器数据源）
curl -X POST http://127.0.0.1:2025/api/mock/generate \
  -H "Content-Type: application/json" \
  -H "X-Confirm-Mock-Mutation: yes" \
  -d '{"root":"/storage/emulated/0","type":"all"}'

# 生成到 /storage/emulated/0/encv-automation/01-plain-media/（automation sourcePath 命名空间）
curl -X POST http://127.0.0.1:2025/api/mock/generate \
  -H "Content-Type: application/json" \
  -H "X-Confirm-Mock-Mutation: yes" \
  -d '{"root":"/storage/emulated/0/encv-automation","type":"all"}'
```

**用户决定什么时候生成**（service-guard 不再强制要求"启动时 mock 必须就位"）。

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

### 3.3 mock API 验证
```bash
# 没带 confirm header → 403
curl -i -X POST http://127.0.0.1:2025/api/mock/generate \
  -H "Content-Type: application/json" \
  -d '{"root":"/storage/emulated/0","type":"all"}' | head -5
# 期望：HTTP/1.1 403, "X-Confirm-Mock-Mutation header required"

# 带 confirm header → 200 + SSE
curl -X POST http://127.0.0.1:2025/api/mock/generate \
  -H "Content-Type: application/json" \
  -H "X-Confirm-Mock-Mutation: yes" \
  -d '{"root":"/storage/emulated/0","type":"all"}' | head -c 500
# 期望：event: progress ... event: done
```

### 3.4 Node CLI 删除验证
```bash
ls /workspace/app/encv-mobile/scripts/generate-mock-files.ts 2>&1
# 期望：No such file

grep -rn 'scripts/generate-mock-files' /workspace --include='*.{ts,sh,js,cjs,mjs,c}' 2>&1
# 期望：空
```

### 3.5 `__mock_data__` 删除验证
```bash
grep -rn '__mock_data__' /workspace --include='*.{go,ts,js,cjs,mjs}' 2>&1
# 期望：空

ls /workspace/__mock_data__ 2>&1
# 期望：No such file
```

### 3.6 pm2 env 透传验证
```bash
pm2 show preview-gateway | grep -A 5 'ENCV_MOBILE\|ENCV_DEV_PREVIEW'
# 期望：env 列表里有 ENCV_MOBILE: '1' / ENCV_DEV_PREVIEW: '1'

curl -s http://127.0.0.1:2025/api/service-guard | jq '.envDevPreview, .envMobile'
# 期望：true, true（如果 backend 进程透传成功）
```

### 3.7 OpenPreview 重发
按 [preview-management.md](file:///workspace/.trae/rules/preview-management.md) 协议，链接 = `http://localhost:16666/`

---

## 四、风险评估

| 风险 | 缓解 |
|------|------|
| pm2 env 透传链诊断耗时 | 备选：先 pkill 旧 backend，`ENCV_MOBILE=1 ENCV_DEV_PREVIEW=1 go run ./cmd/encv start` 后台 |
| `__mock_data__` 全链路删除漏了某处 | grep 全仓搜 + 跑 `go test ./...` 验证 |
| 后端 /api/mock/* 加 confirm header 破坏现有 e2e 测试 | grep `curl.*mock/generate` + 同步改测试加 header |
| 删 Node CLI 后 start-preview.sh 残留引用 | 跑 bash -n start-preview.sh 语法检查 |
| 删除 `mock/index.ts` 后 vite.config.ts 仍引用 | grep `encv-mock-api` / `mock/index` |
| service-guard 简化后某些 UI 仍读 `data.marker` / `data.found` | grep `serviceGuard` `\.marker` `\.found` 确认 |
| 用户没主动生成 mock，但 Files 浏览器打开空 | UI 提示"按生成 Mock 按钮填数据" |

---

## 五、执行顺序（依赖图）

```
Phase 1 (删 Node CLI 全链路)
    ↓
Phase 2 (service-guard 简化)
    ↓
Phase 3 (删 __mock_data__ 死代码)
    ↓
Phase 4 (前端 UI / i18n 简化)
    ↓
Phase 5 (后端 confirm header + 前端自动带)
    ↓
Phase 6 (规范文档更新)
    ↓
Phase 7 (pm2 env 透传诊断修复)
    ↓
Phase 8 (物理清理)
    ↓
Phase 9 (用户决定是否主动生成 mock)
    ↓
Verification + OpenPreview
```

**关键路径**：1 → 2 → 3 → 7.3 → Verification

---

## 六、跨层参考

| 主题 | 文档位置 |
|------|---------|
| mobile overlay 触发 | [internal/config/config.go:289-311](file:///workspace/internal/config/config.go#L289-L311) |
| Makefile dev-mobile | [Makefile:33-37](file:///workspace/Makefile#L33-L37) |
| start-preview.sh | [app/encv-mobile/scripts/start-preview.sh](file:///workspace/app/encv-mobile/scripts/start-preview.sh) |
| ecosystem.config.cjs | [ecosystem.config.cjs](file:///workspace/ecosystem.config.cjs) |
| preview-gateway preflight | [app/preview-gateway/src/preflight.ts](file:///workspace/app/preview-gateway/src/preflight.ts) |
| 后端 mock_generator.go | [internal/server/mock_generator.go](file:///workspace/internal/server/mock_generator.go) |
| 后端 service-guard handler | [internal/server/mobile_api.go:170-360](file:///workspace/internal/server/mobile_api.go) |
| 前端 checkServiceGuard | [app/encv-mobile/src/api/encv.ts:391-407](file:///workspace/app/encv-mobile/src/api/encv.ts#L391-L407) |
| 前端 mockGenerator wrapper | [app/encv-mobile/src/api/mockGenerator.ts](file:///workspace/app/encv-mobile/src/api/mockGenerator.ts) |
| Vite mock plugin（已删） | [app/encv-mobile/mock/index.ts](file:///workspace/app/encv-mobile/mock/index.ts) |
| DEFAULT_AUTOMATION_SOURCE | [app/encv-mobile/src/composables/useAutomationTests.ts:77](file:///workspace/app/encv-mobile/src/composables/useAutomationTests.ts#L77) |
| mock-data 规范 | [.trae/rules/mock-data-architecture.md](file:///workspace/.trae/rules/mock-data-architecture.md) |
| 自动化测试工作流 | [.trae/rules/automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) |
