# 规则库瘦身最终执行计划（Rule Library Slimming — Final Phase）

> 本计划聚焦于**仅剩的 1 个待拆分文件** + **全量验证**。12 个文件已拆分完成，3 个文件保持原状。详见历史计划 [rule-library-slimming-plan.md](file:///workspace/.trae/documents/rule-library-slimming-plan.md)。

---

## 一、Summary

| 维度 | 改造前 | 改造后（最终） |
|------|--------|---------------|
| 规则文件总数 | 17 个 | 17 个索引 + 13 个详情 = 30 个 |
| 索引层总行数 | 7833 行 | **预计 ~2000 行**（13 个拆分平均 ~150 行 + 4 个保持原状） |
| 详情层总行数 | — | 预计 ~5500 行（按需加载） |
| 上下文减负 | 100% 全量加载 | **索引全量加载 ~2000 行**，详情按相对链接按需加载 |
| **总减负** | 7833 → 2000 | **~74%** |

---

## 二、Current State Analysis

### 2.1 实际状态（2026-06-11，`wc -l` 实测）

**规则目录 `/workspace/.trae/rules/`**（17 个文件，共 3216 行）：

| 文件 | 行数 | 状态 |
|------|------|------|
| `mock-data-architecture.md` | 118 | ✅ 已拆分 |
| `local-kotlinc-validation.md` | 127 | ✅ 已拆分 |
| `saturation-debugging.md` | 140 | ✅ 已拆分 |
| `task-group-collapse.md` | 142 | ✅ 保持原状（< 200） |
| `combolite.md` | 151 | ✅ 已拆分 |
| `android.md` | 152 | ✅ 已拆分 |
| `frontend-design.md` | 162 | ✅ 保持原状（< 200） |
| `preview-management.md` | 170 | ✅ 已拆分 |
| `compose-reference.md` | 189 | ✅ 保持原状（< 200） |
| `test.md` | 191 | ✅ 已拆分 |
| `development.md` | 191 | ✅ 已拆分 |
| `verification-discipline.md` | 192 | ✅ 已拆分 |
| `capacitor.md` | 198 | ✅ 已拆分 |
| `trae_web_sandbox_network.md` | 198 | ✅ 已拆分 |
| `kotlin-android.md` | 199 | ✅ 已精简（轻度 trim，无拆分） |
| `automation-workflow.md` | 199 | ✅ 已拆分 |
| **`project_rules.md`** | **497** | **⏳ 待拆分** |

**规则库目录 `/workspace/.trae/rule-library/`**（12 个详情文件，共 5277 行）：

| 详情文件 | 行数 |
|---------|------|
| `android.md` | 231 |
| `automation-workflow.md` | 454 |
| `capacitor.md` | 849 |
| `combolite.md` | 267 |
| `development.md` | 740 |
| `local-kotlinc-validation.md` | 302 |
| `mock-data-architecture.md` | 240 |
| `preview-management.md` | 422 |
| `saturation-debugging.md` | 226 |
| `test.md` | 287 |
| `trae_web_sandbox_network.md` | 620 |
| `verification-discipline.md` | 639 |

### 2.2 进度统计

| 维度 | 数量 | 占比 |
|------|------|------|
| ✅ 已完成（已拆 / 已保持 / 已精简） | 16 个 | 94% |
| ⏳ 待处理 | 1 个（project_rules.md） | 6% |
| **合计** | **17 个** | **100%** |

### 2.3 当前上下文减负（已达成）

- **改造前**：17 个文件，**7833 行**全量加载
- **当前（16/17 完成）**：16 个文件，**3216 行索引**全量加载 + 12 个详情按需
- **已减负**：（7833 - 3216）/ 7833 = **~59%**

### 2.4 project_rules.md 当前内容清单（497 行）

包含 11 个核心章节，分布在 L1-497：

| 行号 | 章节主题 | 推荐处置 |
|------|---------|---------|
| L1-9 | Trae Web 沙箱网络限制要点（4 行 + 链接） | 索引保留核心 4 条要点 + 链向详情 |
| L10-29 | FFmpeg 8.0 构建脚本备注（20 行） | **整段下移详情**（与 trae_web_sandbox_network.md 有重叠） |
| L31-38 | 移动端 ffmpeg 调用架构（8 行） | **整段下移详情** |
| L40-44 | vue-tsc 漏检 + 完整验证流程 | 索引保留精简版 |
| L46-52 | 配置模板保护铁律（7 行） | 索引保留核心 4 条 + 链向详情 |
| L54-110 | Mobile Overlay 机制（57 行：字段命名 / 触发条件 / 数据流 / 反模式） | 索引保留 ASCII 数据流 + 字段映射表 + 触发条件；下移 JSON 代码块 + 反模式 4 条 |
| L111-118 | Go Build Tag 平台约束（8 行） | 索引保留全文（已短） |
| L120-208 | GitHub 项目搜索 + 拉取源码铁律（89 行） | 索引保留 4 个工具允许场景表 + 决策树；下移完整反例 / Gradle 镜像 / 优先级详述 |
| L210-214 | UI 交互铁律（5 行） | 索引保留全文 |
| L216-224 | Jetpack Compose 编码规范（9 行） | 索引保留全文 |
| L226-352 | 防御性编程铁律（127 行：4 章节） | 索引保留「一/二/三」关键铁律 + 三层防御表；下移「四 preview-gateway」完整 4.1-4.4 |
| L354-388 | 沙箱前端访问规则（35 行） | 索引保留端口身份表 + 禁止行为；下移验证代码块 |
| L390-409 | 测试覆盖铁律（20 行） | 索引保留全文 |
| L411-464 | 编译产物铁律（54 行） | 索引保留强制规则 + Mock-data 流向表；下移 `.gitignore` 完整清单 + 验证命令 |
| L466-496 | Skill 目录归属铁律（31 行） | 索引保留强制规则 + 例外；下移完整验证 awk 脚本 |

---

## 三、Proposed Changes

### 3.1 新建 `/workspace/.trae/rule-library/project_rules.md`（详情文件）

**预计 ~350 行**（下沉原始 L10-29、L31-38、L104-110、L186-208、L304-352、L383-388、L457-464、L484-496 + 简短回链索引）

**内容**：
- 顶部 H1 + 1 行说明「本文件为 `project_rules.md` 的详情」
- 完整保留：FFmpeg 8.0 构建脚本备注（20 行）
- 完整保留：移动端 ffmpeg 调用架构（8 行）
- 完整保留：Mobile Overlay §3 数据流 ASCII + 反模式 4 条（13 行）
- 完整保留：GitHub 拉取源码铁律完整表 + Gradle 镜像代码块（30 行）
- 完整保留：防御性编程「四 preview-gateway UPSTREAMS 守卫」完整 4.1-4.4（50 行）
- 完整保留：沙箱前端访问规则验证代码块（10 行）
- 完整保留：`.gitignore` 完整清单 + awk 验证脚本（20 行）
- 顶部反向链接：`../rules/project_rules.md`

### 3.2 重写 `/workspace/.trae/rules/project_rules.md`（索引文件）

**目标：< 200 行（预计 ~190 行）**

**结构**（参考 `automation-workflow.md` 模板）：

```markdown
# 项目规则（Project Rules）

> 11 个跨领域铁律集合：沙箱网络 / FFmpeg / Mobile Overlay / Go Build Tag / GitHub / Compose / 防御性编程 / 沙箱前端访问 / 测试覆盖 / 编译产物 / Skill 目录
>
> **完整内容 + 代码示例 + 反模式实战**：[详情文档](../rule-library/project_rules.md)

## 一、沙箱网络限制（本地构建必读）

[4 条要点]

> 详细诊断数据 → [trae_web_sandbox_network.md](./trae_web_sandbox_network.md)（索引）
> 诊断证据 + 进程级策略矩阵 → [详情文档 §FFmpeg](../rule-library/project_rules.md#ffmpeg-80-构建脚本备注)

## 二、FFmpeg 8.0 备注（仅要点）

[3-4 条要点，链向详情]

> 完整 CFLAGS / 链接参数 / `--disable-asm` 原因 → [详情文档 §FFmpeg](../rule-library/project_rules.md#ffmpeg-80)

## 三、移动端 ffmpeg 调用架构（仅要点）

[3 条要点]

> 详细文件路径（ffmpeg_dlopen.go / video.go / build_info.go）→ [详情文档 §ffmpeg架构](../rule-library/project_rules.md#移动端-ffmpeg-调用架构)

## 四、前端构建验证

[vue-tsc 漏检 + 完整验证命令 1 行]

## 五、配置模板保护 + Mobile Overlay（核心）

[ASCII 数据流 + 字段映射表 + 触发条件表]

> 反模式 4 条 + JSON 代码示例 → [详情文档 §Mobile Overlay 反模式](../rule-library/project_rules.md#禁止的反模式)

## 六、Go Build Tag 平台约束

[5 条要点]

## 七、GitHub 项目搜索 + 拉取第三方源码铁律

[核心原则 1 句 + 工具允许场景表 + 决策树 5 行]

> 完整反例（2026-06-03 越界案例） + Gradle 镜像完整代码 → [详情文档 §GitHub](../rule-library/project_rules.md#拉取第三方源码铁律)

## 八、UI 交互铁律 + Compose 编码规范

[5+4 条要点]

## 九、防御性编程铁律（4 章核心）

[一/二/三章关键铁律 + 三层防御表]

> 第四章 preview-gateway UPSTREAMS 完整踩坑（4.1-4.4） → [详情文档 §四](../rule-library/project_rules.md#四preview-gateway-upstreams-路由完整性守卫)

## 十、Trae Web 沙箱前端访问规则

[端口身份表 + 禁止行为]

> 验证代码块 → [详情文档 §前端访问](../rule-library/project_rules.md#沙箱前端访问规则)

## 十一、测试覆盖铁律

[5 行表 + 优先级 4 行]

## 十二、编译产物铁律

[强制规则 4 条 + Mock-data 流向表]

> 完整 .gitignore 清单 + awk 验证脚本 → [详情文档 §编译产物](../rule-library/project_rules.md#编译产物铁律)

## 十三、Skill 目录归属铁律

[核心规则 4 条 + 例外]

## 十四、相关规则

- [trae_web_sandbox_network.md](./trae_web_sandbox_network.md) — 沙箱网络诊断
- [compose-reference.md](./compose-reference.md) — Compose 权威参考
- [verification-discipline.md](./verification-discipline.md) — 验证纪律
- [development.md](./development.md) — 阻塞式启动反模式
- [capacitor.md](./capacitor.md) — Capacitor 架构

> 拆分：2026-06-11
```

### 3.3 关键设计约束

1. **索引行数硬约束**：< 200 行（与历史拆分一致）
2. **保留所有 H2 章节标题**：便于 §X 锚点跳转
3. **代码示例下沉**：超过 5 行的代码块一律下沉到详情
4. **完整表格保留**：4 列以上的对比表必须在索引中保留
5. **所有相对链接使用** `../rule-library/...md` 形式

### 3.4 关键文件

| 路径 | 操作 | 预计行数 |
|------|------|---------|
| `/workspace/.trae/rules/project_rules.md` | **Write**（重写为索引） | ~190 |
| `/workspace/.trae/rule-library/project_rules.md` | **Write**（新建详情） | ~350 |

---

## 四、Verification Steps

### 4.1 结构验证

```bash
# 1. rules/ 17 个文件
ls /workspace/.trae/rules/*.md | wc -l   # 17

# 2. rule-library/ 13 个文件（新增 1 个）
ls /workspace/.trae/rule-library/*.md | wc -l   # 13

# 3. 所有索引 < 200 行
for f in /workspace/.trae/rules/*.md; do
  lines=$(wc -l < "$f")
  if [ "$lines" -ge 200 ]; then
    echo "❌ $f = $lines 行"
  else
    echo "✅ $f = $lines 行"
  fi
done
# 期望：所有 ✅，最大 ≤ 199

# 4. 索引行数总和
wc -l /workspace/.trae/rules/*.md | tail -1   # ~2000
```

### 4.2 链接完整性验证

```bash
# 5. 索引 → 详情链接
cd /workspace/.trae/rules
for link in $(grep -roh '\.\./rule-library/[^)#"]*\.md' . | sort -u); do
  target="/workspace/.trae/$link"
  if [ -f "$target" ]; then echo "✅ $link"
  else echo "❌ $link 目标不存在"; fi
done

# 6. 详情 → 索引反向引用
for f in /workspace/.trae/rule-library/*.md; do
  topic=$(basename "$f" .md)
  if ! head -20 "$f" | grep -q "\.\./rules/$topic\.md"; then
    echo "⚠️  $f 缺少回链 ../rules/$topic.md"
  fi
done
```

### 4.3 内容完整性验证

```bash
# 7. 关键 bug / 案例保留（grep 详情文件）
grep -c "preview-gateway" /workspace/.trae/rule-library/project_rules.md   # ≥ 10
grep -c "ENCV_MOBILE\|ApplyMobileOverlay" /workspace/.trae/rule-library/project_rules.md   # ≥ 5
grep -c "git clone" /workspace/.trae/rule-library/project_rules.md   # ≥ 3
grep -c "linguist-vendored" /workspace/.trae/rule-library/project_rules.md   # ≥ 3

# 8. 索引保留关键铁律
grep -c "shall not\|SHALL NOT" /workspace/.trae/rules/project_rules.md   # ≥ 4
grep -c "❌\|✅" /workspace/.trae/rules/project_rules.md   # ≥ 6
```

### 4.4 最终减负报告

```bash
# 9. 输出最终统计
echo "=== 索引层 ==="
wc -l /workspace/.trae/rules/*.md | tail -1
echo "=== 详情层 ==="
wc -l /workspace/.trae/rule-library/*.md | tail -1
echo "=== 原始总行数 ==="
echo "7833 行"
echo "=== 减负比例 ==="
python3 -c "
rules = $(wc -l /workspace/.trae/rules/*.md | tail -1 | awk '{print $1}')
print(f'{(7833 - rules) / 7833 * 100:.1f}%')
"
```

---

## 五、Assumptions & Decisions

### 5.1 假设

1. IDE 默认扫描 `.trae/rules/*.md`（Trae IDE 行为已验证）
2. `.trae/rule-library/` 详情**不**被 IDE 自动加载（按需通过相对链接触发）
3. `file:///` 协议在 IDE 渲染规则时可用

### 5.2 决策（已确认无需再次询问）

- 详情目录：`.trae/rule-library/`（用户已确认）
- 拆分方式：每条规则拆为「索引 + 详情」2 份（用户已确认）
- 索引行数：< 200 行（用户已确认）
- 已 < 200 行的 3 个文件保持原状（已确认）

### 5.3 不在范围内

- ❌ 不修改任何规则内容（仅重新组织位置）
- ❌ 不修改 `compose-reference.md` / `frontend-design.md` / `task-group-collapse.md`
- ❌ 不在 IDE / Trae 规则配置层注册新文件
- ❌ 不创建 README.md 总入口
- ❌ 不创建 git commit（用户未要求）

---

## 六、Execution Plan（精简版）

### Step 1：拆分 project_rules.md（1 个文件，5 分钟）

1. **Write** `/workspace/.trae/rule-library/project_rules.md`（详情，~350 行）
   - 完整保留 L10-29（FFmpeg 8.0 备注）
   - 完整保留 L31-38（移动端 ffmpeg 调用架构）
   - 完整保留 L54-110（Mobile Overlay + 反模式 4 条 + JSON 代码）
   - 完整保留 L186-208（Gradle 镜像 + 反例实战）
   - 完整保留 L298-352（防御性编程 §四 preview-gateway 4.1-4.4）
   - 完整保留 L374-388（沙箱前端访问验证代码）
   - 完整保留 L445-464（`.gitignore` + awk 脚本）
   - 完整保留 L484-496（linguist 验证完整脚本）
   - 顶部加 1 行：「本文件为 `project_rules.md` 的详情，参见 [索引](../rules/project_rules.md)」

2. **Write** `/workspace/.trae/rules/project_rules.md`（索引，~190 行）
   - 14 个 H2 章节，每章节保留核心铁律 + 链向详情
   - 关键表格（字段映射 / 触发条件 / 工具允许场景 / Mock-data 流向）保留
   - 关键 ASCII（Mobile Overlay 数据流）保留
   - 不超过 5 行的代码块保留（超过的链向详情）
   - 顶部加 1 行：「**完整内容**：[详情文档](../rule-library/project_rules.md)」

### Step 2：全量验证（1 分钟）

按 §四 全部验证步骤跑一遍：
- 17 个 rules/ 文件
- 13 个 rule-library/ 文件
- 所有索引 < 200 行
- 所有 `../rule-library/...md` 链接目标存在
- 所有详情文件顶部有 `../rules/<topic>.md` 反链
- 关键内容（bug ID / 关键函数名 / 表格行数）已保留
- 索引行数总和 ≤ 2200

### Step 3：报告

输出最终统计：
- 索引总行数（应 ≤ 2200）
- 减负比例（应 ~72%）
- 验证项全部通过

---

## 七、风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| 拆分时遗漏章节 | 索引跳到不存在的 anchor | 详情文件保留原 H2 标题，索引链接用 `§<章节名>` 形式 |
| 索引超 200 行 | 违背设计目标 | 严格遵守 5 行代码块原则；超 5 行的代码一律下移 |
| Mobile Overlay 字段映射表太宽 | 索引中表格爆行 | 保留表头 + 1-2 行示例行；完整表下沉到详情 |
| 详情文件回链缺失 | 用户点不进索引 | 详情顶部硬性加 1 行反链；Step 2 验证 grep 强制检查 |

---

## 八、当前进度总结

✅ **已完成**（16 个文件）：
- 已拆分（12）：android, automation-workflow, capacitor, combolite, development, local-kotlinc-validation, mock-data-architecture, preview-management, saturation-debugging, test, trae_web_sandbox_network, verification-discipline
- 已保持（3）：compose-reference, frontend-design, task-group-collapse
- 已精简（1）：kotlin-android（220 → 199）

⏳ **待处理**（1 个文件）：
- `project_rules.md`（497 → ~190 索引 + ~350 详情）

**关键里程碑**：
- 当前索引行数总和：3216 行（**~59% 减负**）
- 最终索引行数总和目标：≤ 2200 行
- 最终上下文减负目标：**~72%**（7833 → ~2200 行）

**预计完成时间**：1 个文件拆分 + 1 次全量验证 ≈ 5 分钟
