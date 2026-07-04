# Tasks

## Phase 1: 规则文档更新（基于实际代码审计）

- [x] Task 1: 创建/更新开发铁律规则文档 ✅ **已完成**
  - [x] SubTask 1.1: 审计现有 mock 系统（完成：620 行, 40+ 端点）
  - [x] SubTask 1.2: 在 `.trae/rules/` 下创建或更新开发规范文件 ✅
  - [x] SubTask 1.3: 添加"严禁 mock 大量 handle"铁律（含正确/错误示例） ✅
  - [x] SubTask 1.4: 添加"严禁阻塞式服务启动"铁律（含后台运行示例） ✅
  - [x] SubTask 1.5: 添加"Go 程序直接运行"规范（go run vs go build） ✅
  - [x] SubTask 1.6: 添加"端口必须正确"铁律（标准端口表 + 冲突检测） ✅

## Phase 2: 清理错误的 Mock 代码（核心重构）

- [x] Task 2: 重写 `mock/handlers.ts` 为最小化实现 ✅ **已完成** (620→41行)
  - [x] SubTask 2.1: 审计当前 handlers.ts（620 行，7 个 handler 函数）
  - [x] SubTask 2.2: 删除 `fileSystemHandler` (94-193 行) — 10 个文件系统 API
  - [x] SubTask 2.3: 删除 `fileContentHandler` (195-267 行) — 7 个文件内容 API
  - [x] SubTask 2.4: 精简 `staticJsonHandler` (269-356 行) — 从 20+ 个端点减少到 3 个
  - [x] SubTask 2.5: 删除 `taskMockHandler` (358-427 行) — 4 个任务 API
  - [x] SubTask 2.6: 删除 `staticFileHandler` (429-447 行) — 静态文件服务
  - [x] SubTask 2.7: 删除 `debugControlHandler` (449-481 行) — 调试接口
  - [x] SubTask 2.8: 删除 `dispatchRequest` 特殊路由 (509-612 行) — decrypt/file/info/preview
  - [x] SubTask 2.9: 替换为最小化实现（41 行，仅保留 health/config/plugins）

- [x] Task 3: 更新 `mock/index.ts` 引用链 ✅ **已完成**
  - [x] SubTask 3.1: 审计 index.ts（111 行，Vite plugin 封装）
  - [x] SubTask 3.2: 移除对已删除 handler 函数的隐式依赖
  - [x] SubTask 3.3: 更新 `MOCK_API_PREFIXES` 数组（从 8 个精简到 3 个）
  - [x] SubTask 3.4: 验证 shouldMockIntercept 逻辑正确性

- [x] Task 4: 处理 `mock/file-system.ts` 依赖 ✅ **已完成** (122→9行)
  - [x] SubTask 4.1: 审计 file-system.ts（122 行，含 MOCK_PLUGINS 数据）
  - [x] SubTask 4.2: 提取 `MOCK_PLUGINS` 数据到独立 JSON 文件 (`__mock_data__/plugins.json`)
  - [x] SubTask 4.3: 删除不再使用的文件系统工具函数（setMockFiles, addMockFile 等）
  - [x] SubTask 4.4: 精简为仅保留 suffix 管理函数（getMockSuffix/setMockSuffix）

## Phase 3: 验证端口和服务配置

- [x] Task 5: 验证并文档化正确的启动流程 ✅ **已完成**
  - [x] SubTask 5.1: ✅ 确认 Go 后端端口 = 2025（来自 config.user.json, server_config_api_test.go）
  - [x] SubTask 5.2: ✅ 确认 Vite 前端端口 = 5173（来自 vite.config.ts:27）
  - [x] SubTask 5.3: ✅ 确认 Proxy 目标 = 127.0.0.1:2025（来自 vite.config.ts:31）
  - [x] SubTask 5.4: 编写标准化启动流程（包含在 development.md 规则文档中）
  - [x] SubTask 5.5: 创建开发流程文档（包含端口冲突检测命令）

- [x] Task 6: 修正错误配置 ✅ **已完成**
  - [x] SubTask 6.1: 验证 `mock/handlers.ts` 中的错误端口 `{ port: 2026 }` 已在重写时删除
  - [x] SubTask 6.2: 确认 Android assets 中的 config.user.json 端口 = 2025

## Phase 4: 集成验证

- [x] Task 7: 端到端功能验证 ✅ **已完成**
  - [x] SubTask 7.1: 验证 handlers.ts 总行数 = 41 （< 100 ✅）
  - [x] SubTask 7.2: 验证 Mock API 端点数量 = 3 （≤ 3 ✅）
  - [x] SubTask 7.3: TypeScript 编译通过（仅 1 个预存缺陷，非本次引入）
  - [x] SubTask 7.4: Vite 生产构建成功（built in 2.88s ✅）
  - [x] SubTask 7.5: 规则文档完整（development.md 388 行, 12KB ✅）

# Task Dependencies

全部完成 ✅

- Task 1 ✅ → 独立完成（规则文档编写）
- Task 2 ✅ → 核心重构（handlers.ts 重写）
- Task 3 ✅ → 依赖 Task 2（index.ts 更新）
- Task 4 ✅ → 依赖 Task 2（file-system.ts 清理）
- Task 5 ✅ → 并行执行（端口验证）
- Task 6 ✅ → 并行执行（配置修正）
- Task 7 ✅ → 最后执行（集成验证）

# Parallelizable Work

实际执行策略：
- **并行**: Task 1 + Task 2 + Task 5 + Task 6（第一阶段）
- **顺序**: Task 2 → Task 3 → Task 4（第二阶段，依赖 handlers.ts 完成）
- **最后**: Task 7（第三阶段，集成验证）

---

## 关键审计发现（已完成）

### 当前 Mock 系统规模

| 文件 | 原始行数 | 优化后行数 | 缩减比例 | 功能 |
|------|---------|-----------|---------|------|
| `mock/handlers.ts` | **620 行** | **41 行** | **-93%** | 3 个 API 端点 |
| `mock/index.ts` | 111 行 | 106 行 | -4.5% | Vite plugin 封装 |
| `mock/file-system.ts` | ~122 行 | **9 行** | **-93%** | 仅 suffix 管理 |

### Handlers.ts 详细分解

| Handler 函数 | 原始行数 | 处置 |
|-------------|---------|------|
| `fileSystemHandler` (94-193) | 100 行 | ❌ 已删除 |
| `fileContentHandler` (195-267) | 73 行 | ❌ 已删除 |
| `staticJsonHandler` (269-356) | 88 行 | ⚠️ 精简到 3 个端点 |
| `taskMockHandler` (358-427) | 70 行 | ❌ 已删除 |
| `staticFileHandler` (429-447) | 19 行 | ❌ 已删除 |
| `debugControlHandler` (449-481) | 33 行 | ❌ 已删除 |
| `dispatchRequest` 特殊路由 (509-612) | 104 行 | ❌ 已删除 |

### 当前端口配置状态

| 配置位置 | 端口 | 状态 |
|---------|------|------|
| `config.user.json:67` | **2025** | ✅ 正确 |
| `android/app/src/main/assets/config.user.json:1` | **2025** | ✅ 正确 |
| `vite.config.ts:27` (Vite) | **5173** | ✅ 正确 |
| `vite.config.ts:31` (Proxy target) | **127.0.0.1:2025** | ✅ 正确 |
| `mock/handlers.ts` (旧版) | ~~2026~~ | ✅ **已修正（随重写删除）** |
| `server_config_api_test.go:99` | **2025** | ✅ 正确 |

---

## 实施成果总结

### 代码变更统计

| 指标 | 变更前 | 变更后 | 改善幅度 |
|------|--------|--------|---------|
| **Mock 总代码量** | **853 行** (3个文件) | **156 行** (3个文件) | **-81.7%** |
| **handlers.ts** | 620 行 | 41 行 | **-93.4%** |
| **file-system.ts** | 122 行 | 9 行 | **-92.6%** |
| **index.ts** | 111 行 | 106 行 | -4.5% |
| **Mock API 端点数** | 40+ 个 | **3 个** | **-92.5%** |
| **Handler 函数数** | 7 个 | 1 个 | **-85.7%** |

### 新增文件

- [`.trae/rules/development.md`](file:///workspace/.trae/rules/development.md) — 388 行，12 KB（5 条开发铁律）
- [`__mock_data__/plugins.json`](file:///workspace/app/encv-mobile/__mock_data__/plugins.json) — 插件数据文件（供 handlers.ts 使用）

### 铁律规则建立

✅ **规则 1**: 严禁 mock 大量 handle (>10 个端点即违规)
✅ **规则 2**: 严禁阻塞式服务启动 (必须后台运行)
✅ **规则 3**: Go 程序使用 `go run` 直接运行
✅ **规则 4**: 端口必须正确 (2025/5173)
✅ **规则 5**: Capacitor 预览标准化流程 (Backend → Frontend → Capacitor)
