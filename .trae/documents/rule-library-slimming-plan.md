# 规则库瘦身计划（Rule Library Slimming）

> **目标**：将 `.trae/rules/*.md` 中超过 200 行的规则文件拆分为「索引 + 详情」两份，详情下沉到 `.trae/rule-library/`，减轻规则加载时占用的上下文。

---

## 一、Summary

| 维度 | 改造前 | 改造后 |
|------|--------|--------|
| 规则文件总数 | 17 个 | 17 个索引 + 12 个详情 = 29 个 |
| 规则索引行数总和 | 7833 行 | 预估 ≤ 2400 行（12 个拆分的平均 ~150 行 + 5 个未拆分的原始行数） |
| 上下文命中率 | 每次会话全部加载 | **索引全量加载**（2400 行），**详情按需加载**（通过相对链接） |
| 交叉引用方式 | 同目录相对路径 | 跨目录相对路径（`./X.md` / `../rule-library/X.md`） |

**关键决策**（已确认）：
- 详情目录：**`.trae/rule-library/`**（与 `.trae/rules/` 平级，独立管理）
- 拆分方式：每条规则拆为「索引 + 详情」2 份
- 索引行数：**< 200 行/份**
- 已 < 200 行的 3 个文件**保持原样**（不强行拆，遵循「最小复杂度」）

---

## 二、Current State Analysis（2026-06-11 当前实际状态）

### 2.1 文件清单与进度（17 个原始文件，wc -l 实测）

| 文件 | 当前行数 | 状态 | 详情文件 | 备注 |
|------|---------|------|---------|------|
| [mock-data-architecture.md](file:///workspace/.trae/rules/mock-data-architecture.md) | 118 | ✅ 已拆 | [rule-library/mock-data-architecture.md](file:///workspace/.trae/rule-library/mock-data-architecture.md)（240 行） | 完成 |
| [saturation-debugging.md](file:///workspace/.trae/rules/saturation-debugging.md) | 140 | ✅ 已拆 | [rule-library/saturation-debugging.md](file:///workspace/.trae/rule-library/saturation-debugging.md)（226 行） | 完成 |
| [task-group-collapse.md](file:///workspace/.trae/rules/task-group-collapse.md) | 142 | ✅ 保持 | — | 已 < 200 |
| [combolite.md](file:///workspace/.trae/rules/combolite.md) | 151 | ✅ 已拆 | [rule-library/combolite.md](file:///workspace/.trae/rule-library/combolite.md)（267 行） | 完成 |
| [android.md](file:///workspace/.trae/rules/android.md) | 152 | ✅ 已拆 | [rule-library/android.md](file:///workspace/.trae/rule-library/android.md)（231 行） | 完成 |
| [frontend-design.md](file:///workspace/.trae/rules/frontend-design.md) | 162 | ✅ 保持 | — | 已 < 200 |
| [compose-reference.md](file:///workspace/.trae/rules/compose-reference.md) | 189 | ✅ 保持 | — | 已 < 200 |
| [development.md](file:///workspace/.trae/rules/development.md) | 191 | ✅ 已拆 | [rule-library/development.md](file:///workspace/.trae/rule-library/development.md)（740 行） | 完成 |
| [test.md](file:///workspace/.trae/rules/test.md) | 191 | ✅ 已拆 | [rule-library/test.md](file:///workspace/.trae/rule-library/test.md)（287 行） | 完成 |
| [capacitor.md](file:///workspace/.trae/rules/capacitor.md) | 198 | ✅ 已拆 | [rule-library/capacitor.md](file:///workspace/.trae/rule-library/capacitor.md)（849 行） | 完成 |
| [kotlin-android.md](file:///workspace/.trae/rules/kotlin-android.md) | 220 | ⏳ 待精简 | — | 轻微超 20 行，只需去掉冗余 |
| [local-kotlinc-validation.md](file:///workspace/.trae/rules/local-kotlinc-validation.md) | 310 | ⏳ 待拆 | 待创建 | 中等文件 |
| [preview-management.md](file:///workspace/.trae/rules/preview-management.md) | 357 | 🔄 进行中 | 待创建 | 索引需再压 |
| [project_rules.md](file:///workspace/.trae/rules/project_rules.md) | 497 | ⏳ 待拆 | 待创建 | 大文件，11 个子主题 |
| [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) | 510 | ⏳ 待拆 | 待创建 | 14 个 bug + 5 段代码模式 |
| [trae_web_sandbox_network.md](file:///workspace/.trae/rules/trae_web_sandbox_network.md) | 663 | ⏳ 待拆 | 待创建 | 网络架构图 + 诊断数据 |
| [verification-discipline.md](file:///workspace/.trae/rules/verification-discipline.md) | 697 | ⏳ 待拆 | 待创建 | WebFetch 红线 + 反例 |

### 2.2 进度统计

| 维度 | 数量 | 占比 |
|------|------|------|
| ✅ 已完成（已拆 / 保持） | 10 个 | 59% |
| 🔄 进行中 | 1 个 | 6% |
| ⏳ 待处理 | 6 个 | 35% |
| **合计** | **17 个** | **100%** |

**索引行数总和**：4888 行（已达成 < 5000 行目标，对比原 7833 行减少 **37%**）

### 2.3 已存在的交叉引用

| 引用源 | 引用目标 | 当前相对路径 |
|--------|---------|-------------|
| [project_rules.md](file:///workspace/.trae/rules/project_rules.md) | trae_web_sandbox_network.md | `./trae_web_sandbox_network.md` |
| [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md) | task-group-collapse.md, mock-data-architecture.md | 同目录相对路径 |
| [local-kotlinc-validation.md](file:///workspace/.trae/rules/local-kotlinc-validation.md) | kotlin-android.md, verification-discipline.md | `../rules/...md` |

> 拆分后所有内部链接需重写：`./<topic>.md`（同 rules/）或 `../rule-library/<topic>.md`（下沉目标）。

---

## 三、Design Decisions

### 3.1 命名与结构

```
/workspace/.trae/
├── rules/                    ← 索引层（常驻上下文）
│   ├── combolite.md          (~150 行)
│   ├── capacitor.md          (~150 行)
│   ├── development.md        (~180 行)
│   ├── ... (17 个索引)
└── rule-library/             ← 详情层（按需加载）
    ├── combolite.md          (原文 §1.3 机制 + §3+ 下沉内容)
    ├── capacitor.md          (原文 §2+ 章节下沉)
    ├── ... (12 个详情)
    └── README.md             (可选：详情目录总览，3-5 行)
```

### 3.2 索引文件结构模板

每份索引文件统一结构：

```markdown
# <主题名>

> **核心原则**：<一句话核心>

> **完整内容 + 历史踩坑**：[详情文档](../rule-library/<topic>.md)

## 摘要

<3-5 条铁律 / 关键约束 / 一句话命令>

## 关键决策表

| 场景 | 决策 | 链接 |
|------|------|------|
| ... | ... | [详见 §X](../rule-library/<topic>.md#x) |

## 反模式（一眼看）

| ❌ 禁止 | ✅ 正确 | 详情 |
|--------|--------|------|
| ... | ... | [§X](../rule-library/<topic>.md#x) |

## 关键命令 / 速查

<最小可执行命令集>

## 引用其他规则

- [A 规则](./A.md) — <一句话关联>
- [B 详情](../rule-library/B.md) — <一句话关联>
```

### 3.3 详情文件结构

每份详情保留原文件的章节结构、代码块、bug 表格，**只移除**：
- 标题下的「核心原则」摘要（已搬至索引）
- 关键铁律的重复列表（已搬至索引）

详情文件顶部用 1 行 H1 + 1 行「本文件为 xxx 的详情」+ 原标题 + 原核心原则引用即可。

### 3.4 不在范围内（避免过度工程）

- ❌ 不创建总入口 README.md（会增加 1 份全量加载）
- ❌ 不拆分已 < 200 行的 3 个文件
- ❌ 不修改任何规则内容（仅重新组织位置）
- ❌ 不修改 `compose-reference.md` / `frontend-design.md` / `task-group-collapse.md`（保持原状）
- ❌ 不在 IDE / Trae 规则配置层注册新文件（依赖 IDE 默认扫描 `.trae/rules/*.md`）

---

## 四、Proposed Changes（逐文件方案）

> 格式约定：
> - **保留**：索引文件中保留的章节（带原行号参考）
> - **下移**：迁至 `.trae/rule-library/<topic>.md` 的章节
> - **链接**：索引中新增/保留的相对链接

### 4.1 [combolite.md](file:///workspace/.trae/rules/combolite.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-6 标题 + 核心原则摘要；L11-41 §1.1 禁止反射铁律（精简为 5 行原则 + ❌✅对比块）；L43-58 §1.2 禁止 Hook（精简）；L59-93 §1.3 R8 禁用铁律（保留表格：受影响的 4 类） |
| ✅ **下移** | L95-180 R8 破坏机制详细流程（@Metadata 重命名链路）；L181-380 实战案例 / 编译验证；L381-1270 后续章节（Kotlin lambda return / 实战踩坑历史 / AGP build option 配置） |

### 4.2 [capacitor.md](file:///workspace/.trae/rules/capacitor.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-6 核心原则；L9-88 §1.1/1.2 Modal 架构（保留 ❌✅ 对照块 + 关键 5 行代码）；L185-200 关键铁律总结 |
| ✅ **下移** | L100-184 长代码示例（Reactive State Object 模式完整版）；L201-859 §2+ 所有后续章节（SSE / eventBus / tab 切换等具体实现细节） |

### 4.3 [development.md](file:///workspace/.trae/rules/development.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-5 核心原则；L7-50 §1.1/1.2 mock 数量红线（保留表格 + 5 行 ❌✅）；L83-117 §2 阻塞式启动反模式（保留 4 种正确方式）；L120-170 §3 go run 铁律；L173-215 §4 端口表；L245-285 §5.1-5.2 启动序列（精简为 3 步流程） |
| ✅ **下移** | §1.3 推荐替代方案详细；§5.2.1 start-preview.sh 详细铁律（200+ 行）；§5.3 常见问题排查表；§6 WAF 双重编码（90 行）；§7 fork 适配（80 行）；§8 vite HMR 噪音（40 行） |

### 4.4 [verification-discipline.md](file:///workspace/.trae/rules/verification-discipline.md)（697 → 150 / 547）

| 操作 | 章节 |
|------|------|
| **保留** | L1-5 核心原则；L9-36 §1.1-1.3 铁律摘要（保留触发场景表 + 验证顺序 ASCII 图）；L40-55 §2 本地工具速查表 |
| **下移** | §1.2 验证顺序详细说明；§1.3 禁止反模式详细例子；§3 完整 WebFetch/WebSearch 红线（30+ 条规则详情） |
| **链接** | 「WebFetch 短超时纪律」→ `../rule-library/verification-discipline.md#webfetch-红线` |

### 4.5 [trae_web_sandbox_network.md](file:///workspace/.trae/rules/trae_web_sandbox_network.md)（663 → 150 / 513）

| 操作 | 章节 |
|------|------|
| **保留** | L1-3 核心结论；L9-41 §1 网络架构图（精简为 5 行 ASCII）；L43-71 §2 进程级网络策略矩阵（核心表保留）；L73-100 §3 自动注入环境变量（保留 4 行） |
| **下移** | §2 关键测试数据（30 行 DNS 输出 + Java 异常堆栈）；§3 完整环境变量列表 + 详细注释；§4-§8 详细诊断命令 + 排查步骤 |
| **链接** | 「Java/JVM 沙箱兼容矩阵」→ `../rule-library/trae_web_sandbox_network.md#java-兼容矩阵` |

### 4.6 [preview-management.md](file:///workspace/.trae/rules/preview-management.md)（601 → 180 / 421）🔄 进行中

| 操作 | 章节 |
|------|------|
| **保留** | L1-5 核心原则；L7-26 反模式 A（sleep / tail-f）；L36-48 反模式 B（nohup &）；L51-69 反模式 C（blocking web_server）；L72-98 反模式 D（pnpm build/preview）—— 每个反模式只保留：症状 1 句 + 根因 1 句 + 正确做法 1 句 |
| **下移** | 每个反模式的「变种」「实战案例」「为什么是白痴行为」等详细；§反查清单完整版 |
| **链接** | 「反模式 D 实战案例」→ `../rule-library/preview-management.md#反模式-d-2026-06-07-事故` |

**当前问题**：索引已写到 357 行（**超 200 行**），需进一步精简：
- 移除重复的章节过渡
- 压缩 §3 OpenPreview、§4 速查表、§5 env 注入的代码块
- 把 §6 自检清单、§7 go run、§8 Zombie、§9 DOM 锚定、§十 相关文件尽量简化为 1-2 句要点

### 4.7 [automation-workflow.md](file:///workspace/.trae/rules/automation-workflow.md)（510 → 180 / 330）

| 操作 | 章节 |
|------|------|
| **保留** | L1-5 核心原则（3 行）；L19-40 §2 4 件套监听铁律（保留后端事件表）；L191-198 §8 用例数估算（保留当前策略行）；L213-222 §10 扩展铁律（3 条 bullet） |
| **下移** | §1 5 个历史 bug 详细表；§3 动态工作流代码示例（旧 vs 新完整 60 行）；§4 ext 映射表；§5/§6/§7 类型扩展代码；§11 v2 / §12 v3 的所有 bug 详情 + 5 段代码模式 A-E |
| **链接** | 「4 件套事件 payload 详细」→ `../rule-library/automation-workflow.md#事件-payload` |

### 4.8 [project_rules.md](file:///workspace/.trae/rules/project_rules.md)（497 → 180 / 317）

| 操作 | 章节 |
|------|------|
| **保留** | L1-10 沙箱网络限制要点；L48-110 配置模板保护 + Mobile Overlay（保留 §3 数据流 ASCII 图 + 字段映射表 + 触发条件表 + 反模式 4 条）；L111-118 Go Build Tag 平台约束；L120-185 GitHub 项目搜索规范（保留 4 个工具允许场景表）；L210-222 UI 交互铁律；L226-353 防御性编程铁律（一/二/三/四 章节标题 + 关键表）；L354-388 沙箱前端访问规则；L390-407 测试覆盖铁律；L411-465 编译产物铁律；L466-497 Skill 目录归属铁律（保留核心规则 + 例外 + 验证命令） |
| **下移** | FFmpeg 版本备注的完整 30 行；Mobile Overlay §3 数据流的 5 行代码示例；§3 防御性编程「L1/L2/L3 三层防御架构」完整实现（30+ 行代码）；§4 preview-gateway 完整踩坑记录 + 路径清单 + 防御守卫实现；§6 WAF 完整章节；§7 fork 适配；测试覆盖「测试优先级」详细；编译产物详细清单 |
| **链接** | 「preview-gateway UPSTREAMS 守卫完整实现」→ `../rule-library/project_rules.md#preview-gateway-upstreams` |

### 4.9 [local-kotlinc-validation.md](file:///workspace/.trae/rules/local-kotlinc-validation.md)（310 → 150 / 160）

| 操作 | 章节 |
|------|------|
| **保留** | L1-5 核心原则；L7-22 §1 铁律（保留「本地 vs CI」对比表）；L23-65 §2 沙盒工具链（精简为 1 张表「工具/版本/位置」）；L97-114 §3.1 单文件验证命令（保留最小可执行命令）；L190-235 §5 Kotlin lambda return 铁律（保留三种 return 对比 + 错误诊断表） |
| **下移** | §2.2 重新安装命令（详细 30 行）；§3.2/3.3 整 module 验证 + flag 说明；§4 错误诊断模式（4 类错误详细表 + 过滤噪音命令）；§5.4 实战案例（OpenListNativeService.kt 详细） |
| **链接** | 「OpenListNativeService.kt 实战案例」→ `../rule-library/local-kotlinc-validation.md#实战案例-openlistnativeservicekt` |

### 4.10 [test.md](file:///workspace/.trae/rules/test.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-5 核心原则；L7-32 §1.1-1.2 实现位置 + 激活方式（保留 3 张表）；L118-138 §2.1-2.2 后门原则 + 命名空间；L245-281 §3 浏览器自动化测试协议（保留 6 步标准流程 + 判定标准表） |
| ✅ **下移** | §1.3 Mock 数据规范详细（每种文件类型示例）；§2.3 后门函数 5 个规格详情；§3.2 10 个详细测试场景表；§4 禁止事项详细 |

### 4.11 [mock-data-architecture.md](file:///workspace/.trae/rules/mock-data-architecture.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-5 核心原则；L7-15 §1 2 套实现清单表（带行号链接）；L61-70 §3 3 套字节必须同源（保留铁律 + 方案 A/B 选择）；L177-196 §7.1/7.2 调用入口 + 显式意图确认；L211-216 §8 扩展铁律 |
| ✅ **下移** | §2 2026-06-10 修复详细根因链路；§4 检测裸 header 假数据的测试代码；§5 为什么 mock 假数据会让所有解密路径失败（30 行根因链）；§6 ffmpeg 真机 / APK 兼容性表；§9 历史 `__mock_data__` 废弃 |

### 4.12 [saturation-debugging.md](file:///workspace/.trae/rules/saturation-debugging.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-5 核心原则；L7-30 §1.1-1.3 铁律（不猜 / 一次构建 / 复制按钮）；L77-87 §2.1 标准流程（保留 7 步 ASCII）；L218-227 §5 本项目已实现的诊断入口表 |
| ✅ **下移** | §1.4 应用内日志缓冲（具体配置）；§1.5 并行调试原则（详细实施规范 + 反模式 + 实战案例）；§3 常见陷阱（5 个陷阱的详细代码修复）；§4 日志导出规范（详细文件清单） |

### 4.13 [android.md](file:///workspace/.trae/rules/android.md) ✅ 已完成

| 操作 | 章节 |
|------|------|
| ✅ **保留** | L1-3 标题 + 引言；L5-79 §1 仓库顺序铁律（保留 ✅/❌ 正确 vs 错误配置 + ComboLite 依赖表）；L81-97 §2 版本管理（精简为 1 表）；L99-150 §3 AGP 构建选项约束（保留硬耦合说明 + 配置 ABCD 表）；L157-178 §4 常见错误模式（精简为 3 条 bullet） |
| ✅ **下移** | L180-231 §5 gomobile + sqlite 选型铁律（550+ 行对比表 + 验证命令 + 历史踩坑）—— 整章节下沉 |

### 4.14 [kotlin-android.md](file:///workspace/.trae/rules/kotlin-android.md)（220 → 180 / 40）

| 操作 | 章节 |
|------|------|
| **保留** | 全文（接近 200 行）—— 微调到 180 行内，移除「金标准文件」表的注释行；保留所有代码示例 |
| **下移** | 无（无需下移） |
| **链接** | 无（保持单文件） |

> 注：220 行只超 20 行，仅做轻度精简即可，不做内容下沉。

### 4.15-4.17 保持原状（3 个文件已 < 200 行）

- [compose-reference.md](file:///workspace/.trae/rules/compose-reference.md) - 189 行
- [frontend-design.md](file:///workspace/.trae/rules/frontend-design.md) - 162 行
- [task-group-collapse.md](file:///workspace/.trae/rules/task-group-collapse.md) - 142 行

**操作**：仅在每份文件顶部加 1 行「参考文档已自洽，无需详情」声明（可选），不改内容。

---

## 五、Cross-Reference Migration

### 5.1 引用规则重写表

| 当前位置 | 引用目标（旧） | 引用目标（新） |
|---------|---------------|---------------|
| `.trae/rules/project_rules.md` | `./trae_web_sandbox_network.md` | `./trae_web_sandbox_network.md`（索引仍在 rules/） |
| `.trae/rules/automation-workflow.md` | `./task-group-collapse.md` | `./task-group-collapse.md` |
| `.trae/rules/automation-workflow.md` | `./mock-data-architecture.md` | `./mock-data-architecture.md` |
| `.trae/rules/local-kotlinc-validation.md` | `./kotlin-android.md` | `./kotlin-android.md` |
| `.trae/rules/local-kotlinc-validation.md` | `./verification-discipline.md` | `./verification-discipline.md` |
| 索引指向同主题详情 | — | `../rule-library/<topic>.md` |
| 详情指向同主题索引 | — | `../rules/<topic>.md` |
| 详情指向他主题详情 | — | `../rule-library/<other>.md` |
| 详情指向他主题索引 | — | `../rules/<other>.md` |

### 5.2 文件级链接（code 参考）

原文中嵌入的 `file:///workspace/.trae/rules/xxx.md#L10-L20` 形式链接：
- 链接目标**文件未移动**：保持原样
- 链接目标**文件下沉到 library/**：更新为 `file:///workspace/.trae/rule-library/xxx.md`
- 跨行号引用：保持行号（拆分时不改原文行号偏移会变，但 IDE 用文件名定位，不影响点击）

### 5.3 验证

- `Grep "](../rule-library/"` 验证索引 → 详情链接数 ≥ 12
- `Grep "](../rules/"` 验证详情 → 索引链接数 ≥ 1
- `Grep "](./.*\.md)"` 验证同目录链接（保留旧引用）数 = 4

---

## 六、Verification Steps

### 6.1 文件结构验证

```bash
# 1. 索引层 17 个文件全部存在
ls /workspace/.trae/rules/*.md | wc -l   # 应输出 17

# 2. 详情层 12 个文件全部存在（与拆分列表一致）
ls /workspace/.trae/rule-library/*.md | wc -l   # 应输出 12

# 3. 所有索引文件行数 < 200
for f in /workspace/.trae/rules/*.md; do
  lines=$(wc -l < "$f")
  if [ "$lines" -ge 200 ]; then
    echo "❌ $f = $lines 行（>200）"
  else
    echo "✅ $f = $lines 行"
  fi
done
```

### 6.2 引用完整性验证

```bash
# 4. 所有 ../rule-library/ 链接目标存在
cd /workspace/.trae/rules
for link in $(grep -roh '\.\./rule-library/[^)#"]*\.md' . | sort -u); do
  target="/workspace/.trae/$link"
  if [ -f "$target" ]; then
    echo "✅ $link"
  else
    echo "❌ $link 目标不存在"
  fi
done

# 5. 详情文件反向引用索引
for f in /workspace/.trae/rule-library/*.md; do
  topic=$(basename "$f" .md)
  # 检查详情文件顶部是否含 "本文件为 <topic> 的详情" 或引用回索引
  if ! grep -q "\.\./rules/$topic\.md" "$f"; then
    echo "⚠️  $f 缺少回引 ../rules/$topic.md"
  fi
done
```

### 6.3 内容完整性验证

```bash
# 6. 验证原内容无丢失：grep 关键 bug ID（v1 #1-v5、v2 #1-#5、v3 #1-#4）
# 这些 ID 必须在 .trae/rule-library/automation-workflow.md 中找到
grep -c "^| \*\*#1\*\*" /workspace/.trae/rule-library/automation-workflow.md   # 应 ≥ 3

# 7. 验证关键代码块保留（caps Lock 锁定的 code fence）
grep -c '```ts' /workspace/.trae/rule-library/automation-workflow.md   # 应 ≥ 8

# 8. 验证索引行数总和
wc -l /workspace/.trae/rules/*.md | tail -1   # 应 < 2400
```

### 6.4 上下文减负估算

```bash
# 9. 旧 vs 新行数对比
echo "旧: $(cat /workspace/.trae/rules/*.md | wc -l) 行（全部加载）"
echo "新: $(cat /workspace/.trae/rules/*.md | wc -l) 行（索引加载）+ 详情按需"
```

---

## 七、Assumptions & Open Questions

### 7.1 假设

1. **IDE 默认扫描规则目录**：未注册额外配置，所有 `.trae/rules/*.md` 自动加载
2. **不存在外部引用**这些文件的硬编码路径（除了文档内部链接）
3. **`file:///` 链接协议**在 IDE 渲染规则时可用（点击跳转）
4. **Git 跟踪**目前已包含 `.trae/rule-library/`（新目录，需确认）

### 7.2 开放问题

- **是否需要在 commit 中同步更新**（commit message 提及拆分）？建议：单独一个 commit「refactor(rules): split rule files into index + library」，便于回溯
- **是否需要迁移脚本**（自动拆分工具）？建议：不需要，每份手工拆分（质量优先于速度）
- **是否保留 changelog 段**在索引文件里（标注拆分时间 + 关联 spec）？建议：是，每份索引底部加 1 行 `> 拆分：2026-06-11`

### 7.3 不影响范围

- 项目源码（Go / TS / Vue / Kotlin）**不修改**
- `.trae/documents/` 下的具体踩坑记录（[automated-test-entry-in-devtools.md](file:///workspace/.trae/documents/automated-test-entry-in-devtools.md) 等）**不修改**
- `.trae/specs/` 下的规格说明**不修改**
- `.trae/skills/` 下的技能定义**不修改**

---

## 八、Execution Plan（推荐步骤）

按以下顺序执行，便于逐步验证：

### Step 1: 创建目标目录 ✅ 已完成

```bash
mkdir -p /workspace/.trae/rule-library
```

### Step 2: 已完成 7 个小文件拆分

按"链接断裂风险"从低到高：android、combolite、capacitor、development、mock-data-architecture、saturation-debugging、test。

### Step 3: 处理 preview-management.md（进行中）

1. **进一步精简** `/workspace/.trae/rules/preview-management.md`（357 → < 200 行）
2. **创建** `/workspace/.trae/rule-library/preview-management.md` 详情文件

### Step 4: 处理剩余 5 个待拆文件

按风险从低到高：
1. `kotlin-android.md`（轻度精简到 180 行内，**无需拆**）
2. `local-kotlinc-validation.md`（310 → ~150）
3. `verification-discipline.md`（697 → ~150）
4. `trae_web_sandbox_network.md`（663 → ~150）
5. `automation-workflow.md`（510 → ~180）
6. `project_rules.md`（497 → ~180）

每个文件 4 步：
1. **Read** 完整文件（已读）
2. **Write** 索引文件到 `.trae/rules/<topic>.md`（精简版）
3. **Write** 详情文件到 `.trae/rule-library/<topic>.md`（完整版）
4. **运行 §6.2 链接验证**（确保新链接无断裂）

### Step 5: 全量验证

按 §6 全部验证步骤跑一遍。

### Step 6: 提交

```bash
git add .trae/rules/ .trae/rule-library/
git commit -m "refactor(rules): split rule files into index + library for context slimming"
```

---

## 九、风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| 拆分时遗漏章节 | 索引跳到不存在的 anchor | 详情文件保留原 H1/H2/H3 标题，索引链接用 `§<章节名>` 形式 |
| 跨文件链接断裂 | 点击跳转 404 | Step 3 第 4 步强制跑 §6.2 验证 |
| 索引过度精简丢失关键铁律 | 用户忘记铁律 | 索引至少保留「❌禁止 / ✅正确」对照块，详情可下移 |
| IDE 不识别新目录 | 详情不自动加载 | 详情**不需要**自动加载（按需通过相对链接触发） |
| Git diff 噪音 | PR 评审困难 | 拆分 + 精简**分开 commit**（先 split 保留原内容 → 再编辑） |
| preview-management.md 反复超 200 | 进度拖延 | 优先把当前 357 行 → 200 行内，再写详情 |

---

## 十、当前进度总结（2026-06-11）

✅ **已完成**（10 个文件）：
- 已拆分（7）：android, combolite, capacitor, development, mock-data-architecture, saturation-debugging, test
- 已保持（3）：compose-reference, frontend-design, task-group-collapse

🔄 **进行中**（1 个文件）：
- preview-management.md（索引需从 357 → < 200）

⏳ **待处理**（6 个文件）：
- kotlin-android.md（轻微精简 220 → 180）
- local-kotlinc-validation.md（拆 310 → ~150）
- verification-discipline.md（拆 697 → ~150）
- trae_web_sandbox_network.md（拆 663 → ~150）
- automation-workflow.md（拆 510 → ~180）
- project_rules.md（拆 497 → ~180）

**预计完成时间**：6 个文件 × 平均 5 分钟/文件 ≈ 30 分钟

**关键里程碑**：
- 当前索引行数总和：4888 行（目标 < 2400 行）
- 当前上下文减负：**37%**（7833 → 4888 行）
- 最终上下文减负目标：**~70%**（7833 → ~2400 行）
