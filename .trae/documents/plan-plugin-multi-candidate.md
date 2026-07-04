# 插件多候选选择机制 — 计划

## 一、问题诊断

### 1.1 核心矛盾

**Alist-Encrypt 声明可以加密所有文件，但当前系统永远选不到它。**

```
Alist-Encrypt 的能力声明：
├── ShouldProcess(any_path) = true    ✅ "我可以处理任何文件"
├── SupportedMimePrefixes() = nil     ❌ 不声明 MIME 类型
└── SupportedExtensions() = nil       ❌ 不声明扩展名

FindEncryptingPlugin() 匹配流程：
├── 阶段1: MIME 匹配 → Alist-Encrypt 无声明 → ❌ 不进入 candidates
├── 阶段2: 扩展名匹配 → Alist-Encrypt 无声明 → ❌ 不进入 candidates
├── 阶段3: len(candidates) == 0 → 直接返回 error!
└── 阶段4: ShouldProcess() 检查 → 永远执行不到！
```

**Video 插件对比**（正常工作）：
```
Video 能力声明：
├── SupportedMimePrefixes() = ["video/", ...]  ✅ 声明了 MIME
├── SupportedExtensions() = ["mp4", "mkv", ...] ✅ 声明了扩展名
└── ShouldProcess(mp4) = true (排除 srt/ass/vtt)
→ FindEncryptingPlugin 正常匹配到 Video 插件
```

### 1.2 前端无手动选择能力

当前前端行为：
```
用户输入路径 → validateSourcePath()
  → predictPlugin(path, type) [防抖500ms]
    → POST /api/tasks/predict-plugin {sourcePath, type}
      → FindEncryptingPlugin(path) → 返回单一 pluginName
        → 前端显示 "将由 xxx 插件处理" (只读提示)
          → 用户无法更改！
```

**问题场景举例**：
| 文件 | 当前预测 | 用户可能想要 | 结果 |
|------|---------|-------------|------|
| `doc.txt` | **无匹配** (error) | 用 Alist-Encrypt 加密 | ❌ 无法创建任务 |
| `photo.png` | image 插件 | 用 Alist-Encrypt 加密 | ❌ 无法切换 |
| `music.mp3` | audio 插件 | 用 Alist-Encrypt 加密 | ❌ 无法切换 |
| `video.mp4` | video 插件 | 用 Alist-Encrypt 加密 | ❌ 无法切换 |

### 1.3 设计原则违反

> **插件声明式驱动，前端委托渲染** — 但 Alist-Encrypt 的 `ShouldProcess=true` 声明被完全忽略。

---

## 二、设计方案

### 2.1 核心思路：从"单一预测"到"多候选列表"

```
旧模型: path → [后端自动决定唯一最佳] → 单一插件名 → 前端只读显示
新模型: path → [后端返回所有能处理的插件列表] → 前端渲染选择器 → 用户确认/切换
```

### 2.2 多候选优先级排序规则

当多个插件都能处理同一文件时，按以下优先级排列：

| 优先级 | 条件 | 示例 |
|--------|------|------|
| **P0 (精确)** | MIME 或扩展名精确匹配 | `.mp4` → video, `.txt` → text |
| **P1 (通用)** | `ShouldProcess=true` 但未声明具体类型 | alist_encrypt 处理任意文件 |
| **P2 (兜底)** | 无匹配时返回空列表（不猜测） | — |

**用户看到的界面效果**：

**场景 A：单一候选（当前大多数情况）**
```
源文件: /sdcard/video.mp4
📋 将由 Video 插件处理 · 使用全局密码         ← 自动选中，只读提示（不变）
```

**场景 B：多候选（新能力）**
```
源文件: /sdcard/doc.txt
📋 选择加密插件:                          ← 新增选择器
  ◉ Text 插件 (.sccgt 容器) · 使用全局密码   ← P0 精确匹配（默认选中）
  ○ Alist-Encrypt (.bin 容器) · 独立密码      ← P1 通用插件
```

**场景 C：无候选（错误状态）**
```
源文件: /sdcard/unknown.xyz
⚠️ 没有找到能处理此文件的插件              ← 保持现有错误提示
```

---

## 三、实施方案

### Step 1: 后端 — 新增 FindAllEncryptingPlugins 多候选接口

**文件**: `internal/v2/plugins/registry.go`

```go
// PluginCandidate 表示一个能处理文件的插件候选，附带匹配原因
type PluginCandidate struct {
    Plugin     Plugin             // 插件实例
    MatchType  string             // "mime" | "extension" | "general" | "fallback"
    Priority   int                // 0=精确(P0), 1=通用(P1)
}

// FindAllEncryptingPlugins 返回所有能加密指定文件的所有插件候选（按优先级排序）
func FindAllEncryptingPlugins(inputPath string) []PluginCandidate {
    ext := strings.ToLower(filepath.Ext(inputPath))
    mimeType, _ := utils.DetectFileMIMEType(inputPath)

    var candidates []PluginCandidate

    // --- 阶段 1: MIME 精确匹配 (P0) ---
    if mimeType != "" {
        for _, p := range Plugins {
            for _, prefix := range p.SupportedMimePrefixes() {
                if strings.HasPrefix(mimeType, prefix) {
                    if p.ShouldProcess(inputPath) {
                        candidates = append(candidates, PluginCandidate{
                            Plugin: p, MatchType: "mime", Priority: 0,
                        })
                    }
                    break
                }
            }
        }
    }

    // --- 阶段 2: 扩展名精确匹配 (P0) ---
    if len(candidates) == 0 {
        extWithoutDot := ext
        if len(extWithoutDot) > 0 { extWithoutDot = extWithoutDot[1:] }
        if extWithoutDot != "" {
            for _, p := range Plugins {
                for _, supportedExt := range p.SupportedExtensions() {
                    if strings.ToLower(supportedExt) == extWithoutDot {
                        if p.ShouldProcess(inputPath) {
                            candidates = append(candidates, PluginCandidate{
                                Plugin: p, MatchType: "extension", Priority: 0,
                            })
                        }
                        break
                    }
                }
            }
        }
    }

    // --- 阶段 3: 通用插件 (P1) ---
    // 收集 ShouldProcess=true 但未在阶段1-2中匹配的"通用插件"
    for _, p := range Plugins {
        if !p.ShouldProcess(inputPath) { continue }
        alreadyIncluded := false
        for _, c := range candidates {
            if c.Plugin.Name() == p.Name() { alreadyIncluded = true; break }
        }
        if !alreadyIncluded {
            candidates = append(candidates, PluginCandidate{
                Plugin: p, MatchType: "general", Priority: 1,
            })
        }
    }

    return candidates
}
```

**关键设计决策**：
- **保留原有 `FindEncryptingPlugin` 不变**（内部其他调用点不需要改动）
- `FindAllEncryptingPlugins` 是新增的**平行接口**
- 阶段 3 确保 Alist-Encrypt (`ShouldProcess=true`, 无MIME/Ext声明) 总是出现在候选列表中
- 已在阶段 1-2 中匹配的插件不会重复添加（去重检查）

### Step 2: 后端 — 增强 predict-plugin API 返回候选列表

**文件**: `internal/server/mobile_api.go`

修改 `handlePredictPluginGin` 返回值格式：

```go
func (s *Server) handlePredictPluginGin(c *gin.Context) {
    // ... req 解析不变 ...

    var candidates []plugins.PluginCandidate
    if req.Type == "encrypt" {
        candidates = plugins.FindAllEncryptingPlugins(req.SourcePath)
    } else {
        // 解密场景仍用单一匹配（容器文件有明确归属）
        targetPlugin, err := plugins.FindDecryptingPlugin(req.SourcePath)
        if err != nil || targetPlugin == nil {
            c.JSON(200, gin.H{"candidates": nil, "error": err.Error()})
            return
        }
        candidates = []plugins.PluginCandidate{{Plugin: targetPlugin, MatchType: "container", Priority: 0}}
    }

    // 构建响应
    candidateList := make([]gin.H, 0, len(candidates))
    for _, cand := range candidates {
        opts := cand.Plugin.GetTaskOptions()
        candidateList = append(candidateList, gin.H{
            "name":        cand.Plugin.Name(),
            "matchType":   cand.MatchType,
            "priority":    cand.Priority,
            "taskOptions": opts,
        })
    }

    c.JSON(200, gin.H{
        "candidates": candidateList,
        // 向后兼容：单候选时也填充 pluginName/taskOptions
        "pluginName":  func() string {
            if len(candidateList) > 0 { return candidateList[0]["name"].(string) }
            return ""
        }(),
        "taskOptions": func() interface{} {
            if len(candidateList) > 0 { return candidateList[0]["taskOptions"] }
            return nil
        }(),
    })
}
```

**API 响应格式变更**（向后兼容）：

```json
// 旧格式（单候选，向后兼容字段保留）:
{ "pluginName": "video", "taskOptions": {...}, "candidates": [...] }

// 新格式（核心字段）:
{
  "candidates": [
    { "name": "text", "matchType": "extension", "priority": 0, "taskOptions": {...} },
    { "name": "alist_encrypt", "matchType": "general", "priority": 1, "taskOptions": {...} }
  ],
  "pluginName": "text",           // ← 兼容：第一个候选的名字
  "taskOptions": {...}            // ← 兼容：第一个候选的选项
}
```

### Step 3: 后端 — PluginCandidate 类型定义位置

**文件**: `internal/v2/plugins/interfaces/interfaces.go`（或 registry.go 内部）

```go
// PluginCandidate 表示一个能处理文件的插件候选
type PluginCandidate struct {
    Plugin    Plugin `json:"-"`           // 不序列化到 JSON
    Name      string `json:"name"`        // 插件名称
    MatchType string `json:"matchType"`   // 匹配方式: mime | extension | general | container
    Priority  int    `json:"priority"`    // 0=精确匹配, 1=通用兜底
}
```

### Step 4: 前端 API 层适配

**文件**: `app/encv-mobile/src/api/encv.ts`

```typescript
export interface PluginCandidate {
  name: string
  matchType: 'mime' | 'extension' | 'general' | 'container'
  priority: number
  taskOptions: TaskOptions | null
}

// 修改 PredictPluginResponse
export interface PredictPluginResponse {
  candidates: PluginCandidate[]        // ← 新增：完整候选列表
  pluginName: string | null            // ← 保留兼容
  taskOptions: TaskOptions | null      // ← 保留兼容（= candidates[0] 的值）
}

// predictPlugin 函数签名不变，返回类型自动适配
```

### Step 5: 前端 useTaskForm 改造

**文件**: `app/encv-mobile/src/composables/useTaskForm.ts`

**核心变更**：
- `predictedPlugin: ref<string | null>` → `candidates: ref<PluginCandidate[]>([])`
- 新增 `selectedPluginIndex: ref<number>(0)` — 默认选中第一个（最高优先级）
- `taskOptions` 改为 computed（从 `candidates[selectedPluginIndex]?.taskOptions` 派生）
- `predictPlugin` 成功时设置 candidates + 重置 selectedPluginIndex 为 0

```typescript
export function useTaskForm() {
  const candidates = ref<PluginCandidate[]>([])
  const selectedPluginIndex = ref(0)

  const predictedPlugin = computed(() => {
    if (candidates.value.length === 0) return null
    return candidates.value[selectedPluginIndex.value]?.name ?? null
  })

  const taskOptions = computed(() => {
    if (candidates.value.length === 0) return null
    return candidates.value[selectedPluginIndex.value]?.taskOptions ?? null
  })

  // extraValues 在切换插件时需要重置（不同插件的 ExtraFields 不同）
  watch(selectedPluginIndex, () => {
    const opts = taskOptions.value
    const defaults: Record<string, string> = {}
    opts?.extraFields?.forEach((f) => {
      if (f.defaultValue) defaults[f.key] = f.defaultValue
    })
    extraValues.value = defaults
  })

  async function doPredict(sourcePath: string, taskType: 'encrypt' | 'decrypt') {
    // ... 防抖逻辑不变 ...
    try {
      const result = await predictPlugin(sourcePath, taskType)
      candidates.value = result.candidates ?? []
      selectedPluginIndex.value = 0  // 默认选中第一个
      // extraValues 由 watch 触发初始化
    } catch { /* ... */ }
  }

  function reset() {
    candidates.value = []
    selectedPluginIndex.value = 0
    extraValues.value = {}
    primaryOverride.value = ''
    secondaryPassword.value = ''
  }

  return {
    candidates, selectedPluginIndex,
    predictedPlugin, taskOptions,  // 保持 computed 暴露（Tasks.vue 已使用）
    extraValues, primaryOverride, secondaryPassword,
    visibleExtraFields, versionOptions,
    predictPlugin: doPredict, getExtraPayload, reset,
  }
}
```

**注意**：`predictedPlugin` 和 `taskOptions` 从 `ref` 改为 `computed`，但对外接口（属性名+类型）基本不变，Tasks.vue 的模板引用无需大改。

### Step 6: 前端 Tasks.vue 模板改造

**文件**: `app/encv-mobile/src/views/Tasks.vue`

**6a. 替换只读提示为交互式选择器**

将当前的 `<ion-note class="plugin-hint">`（只读）替换为条件渲染：

```html
<!-- 场景 A: 单一候选 → 显示简洁提示（与现有一致） -->
<ion-note v-if="candidates.length === 1 && predictedPlugin && !sourcePathError"
           class="plugin-hint">
  <ion-icon :icon="informationCircle"></ion-icon>
  {{ t('tasks.willBeHandledBy', { plugin: predictedPlugin }) }}
  <!-- 密码策略提示保持不变 -->
</ion-note>

<!-- 场景 B: 多候选 → 显示插件选择器（新增） -->
<div v-else-if="candidates.length > 1 && !sourcePathError"
     class="plugin-selector">
  <ion-item>
    <ion-select
      :value="selectedPluginIndex"
      @ionChange="(e: Event) => selectedPluginIndex = (e as CustomEvent).detail.value"
      label-placement="stacked"
      :label="t('tasks.selectPlugin')"
      interface="action-sheet"
    >
      <ion-select-option
        v-for="(cand, idx) in candidates"
        :key="cand.name"
        :value="idx"
      >
        {{ formatPluginLabel(cand) }}
      </ion-select-option>
    </ion-select>
  </ion-item>
  <ion-note class="plugin-hint">
    {{ t('tasks.willBeHandledBy', { plugin: predictedPlugin }) }}
    <!-- 密码策略 hint 由 taskOptions computed 自动更新 -->
  </ion-note>
</div>

<!-- 场景 C: 无候选 → 错误提示已由 sourcePathError 处理 -->
```

**6b. 新增 formatPluginLabel 辅助函数**

```typescript
function formatPluginLabel(cand: PluginCandidate): string {
  const nameMap: Record<string, string> = {
    'video': 'Video 插件',
    'text': 'Text 插件',
    'audio': 'Audio 插件',
    'image': 'Image 插件',
    'pdf': 'PDF 插件',
    'wps': 'WPS 插件',
    'alist_encrypt': 'Alist-Encrypt',
  }
  const baseName = nameMap[cand.name] ?? cand.name
  const suffixHint = cand.matchType === 'general' ? ' (通用)' : ''
  return `${baseName}${suffixHint}`
}
```

**6c. 更新解构导入**

```typescript
const {
  candidates, selectedPluginIndex,    // ← 新增
  predictedPlugin, taskOptions,
  // ... 其余不变
} = useTaskForm()
```

### Step 7: i18n 新增翻译

**文件**: `app/encv-mobile/src/composables/useI18n.ts`

```typescript
'tasks.selectPlugin': '选择加密插件',           // 中文
'tasks.selectPlugin': 'Select Encrypting Plugin', // English
```

### Step 8: CSS 微调

**文件**: `app/encv-mobile/src/views/Tasks.vue` (<style>)

```css
.plugin-selector {
  margin-bottom: 8px;
}
.plugin-selector ion-select {
  --padding-start: 12px;
  width: 100%;
}
```

### Step 9: 测试

#### 9a. 后端测试

**文件**: `internal/v2/plugins/plugin_candidates_test.go`（新建）

```go
package plugins_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFindAllEncryptingPlugins_Mp4(t *testing.T) {
    initPluginsForTaskOptions(t)
    cands := plugins.FindAllEncryptingPlugins("/test/video.mp4")
    // Video 应该通过 MIME/扩展名精确匹配 (P0)
    names := extractNames(cands)
    assert.Contains(t, names, "video", "video should be a candidate for .mp4")
    // Alist-Encrypt 作为通用插件也应该出现 (P1)
    assert.Contains(t, names, "alist_encrypt", "alist_encrypt should be a general candidate")
    // Video 优先级高于 alist_encrypt
    if len(cands) >= 2 {
        assert.LessOrEqual(t, cands[0].Priority, cands[1].Priority,
            "exact match should come before general")
    }
}

func TestFindAllEncryptingPlugins_Txt(t *testing.T) {
    initPluginsForTaskOptions(t)
    cands := plugins.FindAllEncryptingPlugins("/test/doc.txt")
    names := extractNames(cands)
    // Text 通过扩展名匹配 (P0)
    assert.Contains(t, names, "text", "text should match .txt")
    // Alist-Encrypt 作为通用 (P1)
    assert.Contains(t, names, "alist_encrypt", "alist_encrypt should always appear")
}

func TestFindAllEncryptingPlugins_NoMatchUnknown(t *testing.T) {
    initPluginsForTaskOptions(t)
    cands := plugins.FindAllEncryptingPlugins("/test/file.unknown_ext")
    // 只有通用插件（alist_encrypt）应该出现
    assert.NotEmpty(t, cands, "at least general plugins should be available")
    for _, c := range cands {
        assert.Equal(t, 1, c.Priority, "unknown extension should only have general candidates")
    }
}

func extractNames(cands []plugins.PluginCandidate) []string {
    names := make([]string, len(cands))
    for i, c := range cands { names[i] = c.Plugin.Name() }
    return names
}
```

#### 9b. 前端测试

**文件**: `app/encv-mobile/src/__tests__/useTaskForm.test.ts`（补充）

关键新增用例：
- `doPredict` 返回多候选时 `candidates` 正确填充
- `selectedPluginIndex` 默认为 0
- 切换 `selectedPluginIndex` 时 `taskOptions` 自动更新
- 切换插件时 `extraValues` 重置为默认值
- `reset()` 清空 candidates

---

## 四、文件变更清单

| # | 文件 | 操作 | 说明 |
|---|------|------|------|
| 1 | `internal/v2/plugins/interfaces/interfaces.go` | **修改** | 新增 `PluginCandidate` 结构体 |
| 2 | `internal/v2/plugins/registry.go` | **修改** | 新增 `FindAllEncryptingPlugins()` 函数 |
| 3 | `internal/server/mobile_api.go` | **修改** | `handlePredictPluginGin` 返回候选列表（向后兼容） |
| 4 | `app/encv-mobile/src/api/encv.ts` | **修改** | 新增 `PluginCandidate` 接口；`PredictPluginResponse` 增加 `candidates` 字段 |
| 5 | `app/encv-mobile/src/composables/useTaskForm.ts` | **修改** | `predictedPlugin`/`taskOptions` 从 ref 改为 computed；新增 `candidates`/`selectedPluginIndex`；watch 切换重置 extraValues |
| 6 | `app/encv-mobile/src/views/Tasks.vue` | **修改** | 模板：单候选→只读提示 / 多候选→ion-select 选择器；新增 `formatPluginLabel`；解构增加 candidates/selectedPluginIndex |
| 7 | `app/encv-mobile/src/composables/useI18n.ts` | **修改** | 新增 `tasks.selectPlugin` 翻译 key |
| 8 | `app/encv-mobile/src/views/Tasks.vue` (<style>) | **修改** | 新增 `.plugin-selector` CSS |
| 9 | `internal/v2/plugins/plugin_candidates_test.go` | **新建** | FindAllEncryptingPlugins 多候选逻辑测试 |

---

## 五、不做的事情（边界）

- **不改 `FindEncryptingPlugin` 原函数** — 保留给内部调用（processEncrypt/Decrypt），避免影响已有逻辑
- **不改解密预测逻辑** — 解密场景容器文件有明确归属，不需要多候选
- **不在后端做"智能推荐排序"** — 只按 P0/P1 优先级排序，不做 ML 或用户偏好
- **不改移动端 action-sheet 外观** — 使用 IonSelect 默认样式即可
