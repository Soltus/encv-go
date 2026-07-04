# 移除 __mock_data__ 幽灵代码，统一使用 /storage/emulated/0

## 问题

i18n 服务守卫消息已正确写明 `--dir /storage/emulated/0`，但以下代码仍引用 `__mock_data__`（幽灵代码）：

1. `scripts/generate-mock-files.ts:8` — `MOCK_ROOT` 默认值为 `__mock_data__`
2. `mock/index.ts:7` — `MOCK_DATA_ROOT` 硬编码 `__mock_data__`
3. `mock/handlers.ts:5` — `MOCK_DATA_DIR` 硬编码 `__mock_data__`
4. `__mock_data__/` 目录 — 整个目录是旧数据，应删除

## 修复步骤

### Step 1: 修改 `scripts/generate-mock-files.ts`

1. 将第 8 行默认路径改为 `/storage/emulated/0`：
```ts
const MOCK_ROOT = '/storage/emulated/0'
```

2. 在 `main()` 函数开头（`parseArgs()` 之后），添加自动创建根目录逻辑：
```ts
ensureDir(root)
```

这样脚本自身负责创建目标目录，无需外部 `sudo mkdir`。

### Step 2: 修改 `mock/index.ts`

将第 7 行：
```ts
const MOCK_DATA_ROOT = path.resolve(__dirname, '../__mock_data__')
```
改为：
```ts
const MOCK_DATA_ROOT = '/storage/emulated/0'
```

### Step 3: 修改 `mock/handlers.ts`

将第 5 行：
```ts
const MOCK_DATA_DIR = path.resolve(__dirname, '../__mock_data__')
```
改为：
```ts
const MOCK_DATA_DIR = '/storage/emulated/0'
```

### Step 4: 删除 `__mock_data__/` 目录

```bash
rm -rf /workspace/app/encv-mobile/__mock_data__
```

### Step 5: 运行生成脚本

```bash
cd /workspace/app/encv-mobile && npx tsx scripts/generate-mock-files.ts
```

脚本会自动创建 `/storage/emulated/0` 及其子目录。

### Step 6: 验证

- 确认 `/storage/emulated/0/01-plain-media/` 存在
- 确认 `__mock_data__` 目录已不存在
- 确认 Go 后端 `ENCV_DEV_PREVIEW=1` 启动后 `server.dir` 指向 `/storage/emulated/0`（由 mobile overlay 自动生效）
- 确认前端服务守卫不再拦截

## 影响范围

- `generate-mock-files.ts` — 默认输出路径变更 + 自动创建目录
- `mock/index.ts` — Vite mock 插件数据源路径变更
- `mock/handlers.ts` — Mock API handler 数据源路径变更
- `__mock_data__/` — 删除旧数据目录
- 不修改 `config.user.json`（严禁修改）
- 不修改 Go 后端代码（mobile overlay 机制已正确）
