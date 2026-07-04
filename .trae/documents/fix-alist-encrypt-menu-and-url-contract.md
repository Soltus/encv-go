# 修复计划：长按菜单不识别加密文件 + URL编码契约规范化

## 问题概述

### Bug #1：isAlistEncrypted() 始终返回 false（未修复）

**用户反馈**：上轮修复 `getDefaultValue()` 后问题仍然存在。

**完整链路追踪结果**：

```
用户长按文件 → handleLongPress()
  → getAllActions(file) → useFileFeatures.collectActions()
    → 遍历 registry → alist-encrypt feature.isActive(file)=true ✅
      → feature.getFileActions(file) → getAlistActions(file)
        → isAlistEncrypted(file)
          → getFieldValue(['plugin_settings', 'alist_encrypt', 'suffix'])
            → 从 config.value 中读取 ← 后端返回 ".bin" ✅
          → !!suffix && file.name.endsWith(suffix)
```

**后端 API 确认**：`GET /api/config` 返回 `plugin_settings.alist_encrypt.suffix = ".bin"` ✅

**真正的根因（两层）**：

#### 根因 A：`parseProperty()` 丢失 `default` 属性（schema 解析层）

[schemaParser.ts:80-89](app/encv-mobile/src/config/schemaParser.ts#L80-L89) 构建 FieldDef 对象时**没有复制 `resolved.default`**：

```typescript
const field: FieldDef = {
  key, label, description, type, required,
  sectionTitle, isPassword, isPath,
  // ❌ 缺失: default: resolved.default
}
```

虽然 [schema.json:87](app/encv-mobile/src/config/schema.json#L87) 定义了 `"default": ".bin"`，但解析后 `field.default === undefined`。

**影响链路**：
- `buildInitialConfig()` 调用 `getDefaultValue(field)` → 因 `field.default=undefined` → fallback 到 `case 'string': return ''`
- 当 `fetchConfig()` 失败或网络异常时 → `config.value = buildInitialConfig()` → suffix 变成空字符串
- 即使后端正常返回 `.bin`，如果存在任何 config 合并/覆盖逻辑，也可能丢失默认值

#### 根因 B：`getFieldValue` 无默认值回退（config 读取层）

[useConfig.ts:64-74](app/encv-mobile/src/composables/useConfig.ts#L64-L74) 的 `getFieldValue()` 直接从 `config.value` 读取，值为空时返回空，**不会回退到 schema 默认值**。

### Issue #2：URL 编码契约确认与规范化

**当前实际契约（已验证正确）**：
```
前端 proxySafeEncode(x) = encodeURIComponent(encodeURIComponent(x))  // 双重编码
  ↓ HTTP 请求
Gin c.Query("path") 自动 decode 一次（抵消外层 encodeURIComponent）
  ↓
DecodeGinQueryParam() = url.PathUnescape()（抵消内层 encodeURIComponent）
  ↓
完美还原原始路径 ✅
```

**关键字符验证**（以文件名 `hyYGPCwJPQ3+xrdAvfnn2.bin` 为例）：

| 步骤 | `+` 的状态 | `/` 的状态 |
|------|-----------|-----------|
| 原始路径 | `+` | `/` |
| encodeURIComponent #1 | `%2B` | `%2F` |
| encodeURIComponent #2 (proxySafeEncode) | `%252B` | `%252F` |
| Gin Query() 自动解码 | `%2B` | `%2F` |
| PathUnescape (我们的修复) | `+` ✅ | `/` ✅ |

**上一轮修复已正确的部分**：`QueryUnescape` → `PathUnescape`（`+` 不再被解码为空格）

**需要规范化的部分**：确保所有 handler 统一使用此契约，并在代码注释中明确记录。

---

## 修改计划

### 修改 1：schemaParser.ts — parseProperty() 复制 default 属性

**文件**：`app/encv-mobile/src/config/schemaParser.ts`

**位置**：`parseProperty()` 函数，第 80-89 行 field 对象构建处

**改动**：在 field 对象中添加 `default` 属性复制

```typescript
const field: FieldDef = {
  key,
  label: formatLabel(key),
  description: cleanDesc,
  type: (resolved.type || 'string') as FieldType,
  required: isRequired,
  sectionTitle: sectionTitle || undefined,
  isPassword: isPasswordField(key),
  isPath: isPathField(key),
  default: resolved.default,        // ← 新增：从 JSON Schema 复制默认值
}
```

### 修改 2：useConfig.ts — getFieldValue 增加 schema 默认值回退

**文件**：`app/encv-mobile/src/composables/useConfig.ts`

**位置**：`getFieldValue()` 函数，第 64-74 行

**改动**：当读取值为 `undefined`/`null`/空字符串时，回退到 schema 定义的默认值

```typescript
function getFieldValue(path: string[]): unknown {
  let current: unknown = config.value
  for (const key of path) {
    if (current && typeof current === 'object' && current !== null) {
      current = (current as Record<string, unknown>)[key]
    } else {
      return undefined
    }
  }
  // 新增：当值为空且存在 schema 默认值时回退
  if (current === undefined || current === null || current === '') {
    const schemaDefault = findSchemaDefault(path)
    if (schemaDefault !== undefined) return schemaDefault
  }
  return current
}

function findSchemaDefault(path: string[]): unknown {
  let fields: FieldDef[] | undefined = schemaFields.value
  for (let i = 0; i < path.length - 1 && fields; i++) {
    const child = fields.find(f => f.key === path[i])
    fields = child?.properties
  }
  const leaf = fields?.find(f => f.key === path[path.length - 1])
  return leaf ? getDefaultValue(leaf) : undefined
}
```

### 修改 3：path.go — 添加契约注释

**文件**：`internal/utils/path.go`

**位置**：`DecodeGinQueryParam()` 函数和 `SafeResolveToAbsPath()` 函数

**改动**：添加明确的编码契约文档注释

```go
// DecodeGinQueryParam 解码 Gin query 参数中的路径。
//
// 编码契约（前端 proxySafeEncode = 双重 encodeURIComponent）：
//   1. 前端: encodeURIComponent(encodeURIComponent(raw_path))
//   2. Gin c.Query() 自动反向解码一层
//   3. 本函数用 url.PathUnescape 反向解码第二层
//   4. 结果 = 原始路径 ✅
//
// 注意：必须使用 PathUnescape 而非 QueryUnescape，
// 因为 QueryUnescape 会将 '+' 解码为空格（HTML form 规范），
// 但文件名中的 '+' 是合法字符。
func DecodeGinQueryParam(raw string) string {
	s, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return s
}
```

### 修改 4（可选加固）：isAlistEncrypted 增加防御性日志

**文件**：`app/encv-mobile/src/features/alist-encrypt/useAlistEncrypt.ts`

**位置**：`isAlistEncrypted()` 函数第 26-30 行

**改动**：添加调试日志方便未来排查（生产环境为 debug 级别）

```typescript
export function isAlistEncrypted(file: FileItem): boolean {
  if (file.isDirectory) return false
  const suffix = getFieldValue(['plugin_settings', 'alist_encrypt', 'suffix']) as string
  console.debug(`[alist-encrypt] isAlistEncrypted: name=${file.name}, suffix=${repr(suffix)}, matches=${!!suffix && file.name.endsWith(suffix)}`)
  return !!suffix && file.name.endsWith(suffix)
}
```

---

## 验证步骤

1. **重新编译后端**：`go build -o /tmp/encv-mobile ./cmd/encv-mobile/`
2. **重启后端**：`ENCV_DEV_PREVIEW=1 /tmp/encv-mobile`
3. **刷新前端页面**（硬刷新 Cmd+Shift+R 清除缓存）
4. **验证 Bug #1**：
   - 浏览器 DevTools Console 输入：`document.querySelector('ion-app').__vue_app__.config.globalProperties.$store` 或直接在 Network 面板确认 `/api/config` 返回 `suffix: ".bin"`
   - 长按 `hyYGPCwJPQ3+xrdAvfnn2.bin` → 菜单应显示「流式预览」和「解密」两个操作
5. **验证 Issue #2**：
   - 点击「信息」→ FileInfo 页面正常加载（已在上一轮修复）
   - 文件名中的 `+` 正确显示（不变空格）
6. **回归测试**：
   - 普通文件（非 .bin 后缀）长按应只显示「加密」操作
   - 中文文件名的文件信息页正常加载
   - 目录的长按菜单不受影响

---

## 风险评估

| 修改 | 风险等级 | 影响 |
|------|---------|------|
| parseProperty 复制 default | 低 | 只影响初始配置构建和设置页默认值显示 |
| getFieldValue 回退默认值 | 低 | 只影响空值场景的防御性增强 |
| path.go 注释 | 无 | 纯注释变更 |
| isAlistEncrypted 日志 | 无 | 仅 debug 级别输出 |
