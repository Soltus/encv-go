# Phase 2 Spec — 前后端联通 + API 层

> **阶段目标**：建立前后端通信协议，实现 API 层 + WebSocket 推送，让前端可以查询世界状态
> **状态**：v0.1 — 规划中
> **前置依赖**：Phase 1（后端核心引擎）
> **后续阶段**：Phase 3（细节补充 + 性能优化）

---

## 一、阶段目标

本阶段从纯后端进入前后端联通阶段：

1. ✅ **建立 API 层**：REST + WebSocket 双协议
2. ✅ **世界状态可查询**：NPC / 组织 / 区域 / 关系网 / 格系统
3. ✅ **事件推送机制**：世界变化实时推送到前端
4. ✅ **前后端性能协同**：根据前端帧率动态调整后端模拟速度

## 二、技术栈选择

### 2.1 后端 API

| 组件 | 选型 | 说明 |
|------|------|------|
| HTTP 框架 | Gin / Echo | 轻量高性能，Android Go 可移植 |
| WebSocket | gorilla/websocket | 事件推送专用 |
| 序列化 | JSON + msgpack | JSON 通用，msgpack 高性能 |
| SSE | 原生 SSE | 流式事件（备选） |

### 2.2 前端集成

| 组件 | 选型 | 说明 |
|------|------|------|
| 前端框架 | Ionic Vue + Capacitor | 现有移动端架构 |
| 状态管理 | Pinia | Vue 生态标准 |
| API 客户端 | useProxiedFetch | 现有封装，支持 native 模式 |
| WebSocket | useWebSocket | 现有 composable |
| SSE headers | `Accept: text/event-stream` | 按 Capacitor 铁律 |

## 三、API 设计

### 3.1 REST API 端点

```
/api/simverse/
├── GET  /world/state           # 世界状态概览（tick/人口/资源/统计）
├── GET  /world/config          # 世界配置（性能层级/模拟参数）
├── POST /world/config          # 修改世界配置
├── POST /world/control         # 世界控制（暂停/继续/加速/跳转）
│
├── GET  /npc/list              # NPC 列表（分页/筛选）
├── GET  /npc/:id               # NPC 详情
├── GET  /npc/:id/relationships # NPC 关系网
├── GET  /npc/:id/timeline      # NPC 生命事件时间线
├── GET  /npc/:id/brain         # NPC 脑内格系统（概览）
├── GET  /npc/:id/brain/region  # 脑区详情
│
├── GET  /org/list              # 组织列表
├── GET  /org/:id               # 组织详情
├── GET  /org/:id/members       # 组织成员
│
├── GET  /region/list           # 区域列表
├── GET  /region/:id            # 区域详情
├── GET  /region/:id/stats      # 区域统计（人口/经济/关系）
│
├── GET  /events                # 事件流（分页，历史事件）
└── GET  /perf/metrics          # 性能指标（CPU/内存/tick rate）
```

### 3.2 WebSocket 事件推送

```
ws://localhost:port/api/simverse/ws

服务端推送事件：
├── world:tick              # 世界 tick 通知（节流：10/s）
├── world:stats_update      # 统计数据更新（1/s）
├── npc:update              # NPC 状态变化（焦点 NPC 才推）
├── npc:life_event          # NPC 生命事件（出生/死亡/结婚/生子）
├── org:update              # 组织状态变化
├── event:new               # 新事件发生
└── perf:metrics            # 性能指标推送
```

### 3.3 焦点管理 API

前端通过 API 告诉后端"我在关注谁"，后端据此调整推送精度：

```
POST /api/simverse/focus
{
  "npcs": [
    { "id": 1, "level": "player" },
    { "id": 2, "level": "core" },
    { "id": 3, "level": "near" },
    ...
  ]
}
```

| 焦点层级 | 推送频率 | 数据精度 | 格模拟深度 |
|---------|---------|---------|-----------|
| player | 实时 (60/s) | 全量 | L3 深度 |
| core | 高频 (10/s) | 高精度 | L2 深度 |
| near | 中频 (2/s) | 中精度 | L1 深度 |
| distant | 低频 (0.2/s) | 低精度 | 不模拟格 |
| none | 不推送 | 统计 | 不模拟格 |

## 四、前后端性能协同

### 4.1 动态性能调度

```
前端 → 后端：上报当前状态
├── app_state: foreground / background
├── ui_fps: 58 (当前帧率)
├── battery_level: 85%
├── thermal_level: normal
└── user_idle: false / true (用户是否在操作)

后端 → 调整：
├── 前台活跃：1.0x 速率，高推送频率
├── 前台静置：3.0x 速率，低推送频率
└── 后台：0.6x 速率，几乎不推送
```

### 4.2 流量控制

| 场景 | 推送策略 | 数据量估算 |
|------|---------|-----------|
| 前台看单个 NPC | 该 NPC 全量 + 脑区数据 | ~50 KB/s |
| 看 NPC 列表 | 列表项精简数据 | ~20 KB/s |
| 看世界地图 | 区域统计数据 | ~10 KB/s |
| 后台 | 仅关键事件推送 | < 1 KB/s |
| 前台静置 | 统计数据 + 重要事件 | ~5 KB/s |

### 4.3 错误恢复

- WebSocket 断开：自动重连 + 状态补全（catch-up 模式）
- 网络波动：本地缓存 + 增量同步
- 后端重启：版本号校验 + 全量重同步

## 五、与其他阶段的关联

### 依赖 Phase 1
- 世界引擎核心
- NPC / 组织 / 区域数据模型
- Catch-up 模拟机制
- 性能调度器

### 输出给 Phase 3
- API 性能基线
- 推送延迟数据
- 前后端联调问题列表
- 优化需求清单

### 输出给 Phase 4
- 完整 API 文档
- TypeScript 类型定义
- 前端 composable 封装

## 六、验收标准

1. ✅ 所有 REST API 端点可用
2. ✅ WebSocket 事件推送正常
3. ✅ 前后端联调通过（基本查询 + 事件推送）
4. ✅ 焦点管理 API 正常工作
5. ✅ 后台 60% / 前台 100% / 前台闲时 300% 切换正常
6. ✅ 前端 FPS 在前台模式下稳定 > 55fps
7. ⬜ 内存占用达标（Android 端 < 150 MB 总内存）

## 七、风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| WebSocket 在 native 模式不稳定 | 中 | 高 | SSE 备选 + useProxiedFetch 流式 |
| 推送数据量过大导致卡顿 | 高 | 中 | 节流 + 分级推送 + 虚拟列表 |
| 前后端状态不一致 | 中 | 高 | 版本号校验 + 增量 diff + 全量兜底 |
| Capacitor 原生桥接性能瓶颈 | 低 | 中 | 批量传输 + msgpack 压缩 |
| 电池消耗过快 | 中 | 高 | 后台降频 + 推送节流 + 批量 wakeup |
