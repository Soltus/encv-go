# SimVerse Web — Phaser 4.2 集成开发规划

> **阶段**：预研规划期
> **目标**：用 Phaser 4.2 重写/增强 SimVerse Web 前端的世界视图，提供更好的 2D 游戏化交互体验
> **前置依赖**：`plugin-simverse/web` 现有 Vue 3 + Ionic 8 架构（Phaser 作为世界视图的渲染引擎，不替换整体框架）

---

## 一、为什么选择 Phaser 4.2

### 1.1 现状与痛点

当前 SimVerse 世界视图（`SimverseWorld.vue`）基于纯 DOM + CSS 实现：

| 维度 | 现状（DOM/CSS） | 痛点 |
|------|----------------|------|
| **渲染性能** | 数百个 DOM 节点 | NPC/建筑数量 > 200 时明显掉帧 |
| **动画能力** | CSS transition / animation | 复杂路径、粒子效果实现困难 |
| **交互体验** | click / hover | 拖拽、缩放、画布平移不流畅 |
| **地图渲染** | Grid + 背景图 | 地形层级、瓦片地图不支持 |
| **开发效率** | 手动计算坐标 | 无游戏对象、场景、相机等抽象 |

### 1.2 Phaser 4.2 带来的能力

| 能力 | 说明 | 对 SimVerse 的价值 |
|------|------|-------------------|
| **Canvas/WebGL 渲染** | 自动降级，硬件加速 | 数千个 NPC 同屏 60fps |
| **场景管理** | Scene 系统，多场景切换 | 世界地图 / 区域视图 / 战斗场景分离 |
| **游戏对象** | Sprite / Container / Graphics | NPC、建筑、组织等实体抽象 |
| **相机系统** | Camera + 缩放 + 拖拽 | 世界地图的平移缩放浏览 |
| **瓦片地图** | Tilemap + Tileset | 程序化生成地形 |
| **动画系统** | Tween + Timeline | NPC 移动、事件特效 |
| **输入系统** | 统一的指针/键盘/触摸 | 移动端手势支持 |
| **粒子系统** | Particle Emitter | 事件视觉化（火灾、庆典等） |
| **物理引擎** | Arcade / Matter | 可选，未来扩展用 |

### 1.3 版本选择：Phaser 4.x vs 3.x

| 维度 | Phaser 3.x | Phaser 4.x |
|------|-----------|-----------|
| **稳定性** | 稳定，生态成熟 | 较新，API 可能调整 |
| **性能** | 良好 | 更好（重渲染管线） |
| **TypeScript** | 有类型定义 | 原生 TypeScript，类型更好 |
| **包体积** | ~800KB | ~500KB（tree-shaking 友好） |
| **ESM 支持** | 一般 | 原生 ESM，Vite 友好 |

**选择 4.2 的理由**：
- SimVerse 本身在快速迭代期，可以接受一定的 API 变动
- 原生 TypeScript + ESM 与 Vue 3 + Vite 技术栈更匹配
- 4.x 是未来方向，早踩坑早受益
- 500KB 体积在插件 APK 中可接受

---

## 二、架构设计

### 2.1 整体架构：Vue + Phaser 混合

**原则**：Phaser 只负责世界视图的 2D 渲染，UI 壳、页面路由、表单设置等仍由 Vue + Ionic 负责。

```
┌─────────────────────────────────────────────────┐
│              SimVerse Web 前端                    │
├─────────────────────────────────────────────────┤
│                                                 │
│  上层：Vue 3 + Ionic 8（不变）                    │
│  ├── 路由 / 页面 / Tab / 设置 / 日志              │
│  ├── 状态管理 / API 调用 / i18n                 │
│  └── UI 组件（列表 / 表单 / 模态框）              │
│                         │                       │
│                         ▼                       │
│  世界视图层：Phaser 4.2（新增）                   │
│  ├── Scene（场景）                               │
│  │   ├── WorldScene（世界地图）                   │
│  │   ├── RegionScene（区域特写）                  │
│  │   └── BattleScene（战斗/事件特写）             │
│  ├── Game Objects（游戏对象）                     │
│  │   ├── NPCSprite（NPC 角色）                    │
│  │   ├── BuildingSprite（建筑）                   │
│  │   ├── OrgMarker（组织标记）                    │
│  │   └── EventEffect（事件特效）                  │
│  ├── Camera（相机系统）                           │
│  └── Tilemap（瓦片地图）                          │
│                                                 │
└─────────────────────────────────────────────────┘
```

### 2.2 Vue ↔ Phaser 通信方式

```typescript
// 1. Vue 侧：提供 usePhaserWorld composable
const { sceneRef, worldState, selectedNPC } = usePhaserWorld()

// 2. Phaser 侧：通过事件总线与 Vue 通信
//    - Phaser → Vue：事件总线 emit （"npc:click" / "world:pan"）
//    - Vue → Phaser：调用 scene 上的方法（scene.setWorldState()）

// 3. 状态同步
//    - 世界状态（tick/NPC 数/时代）：Vue → Phaser 单向流
//    - 交互事件（选中 NPC / 缩放级别）：Phaser → Vue 事件
```

### 2.3 目录结构

```
plugin-simverse/web/src/
├── views/
│   ├── SimverseWorld.vue          ← 改造：Vue 壳 + Phaser canvas
│   └── ...（其他页面不变）
├── game/                           ← 新增：Phaser 游戏代码
│   ├── main.ts                     ← Phaser 游戏实例创建
│   ├── scenes/
│   │   ├── WorldScene.ts           ← 世界地图场景
│   │   ├── RegionScene.ts          ← 区域特写场景
│   │   └── BaseScene.ts            ← 场景基类
│   ├── objects/
│   │   ├── NPCSprite.ts            ← NPC 精灵
│   │   ├── BuildingSprite.ts       ← 建筑精灵
│   │   ├── OrgMarker.ts            ← 组织标记
│   │   └── EventEffect.ts          ← 事件特效
│   ├── camera/
│   │   └── WorldCamera.ts          ← 世界相机控制（拖拽/缩放）
│   ├── tilemap/
│   │   └── ProceduralTilemap.ts    ← 程序化瓦片地图
│   └── utils/
│       ├── PhaserEventBus.ts       ← Phaser ↔ Vue 事件桥
│       └── StateSync.ts            ← 状态同步工具
├── composables/
│   ├── usePhaserWorld.ts           ← Vue 侧 Phaser 封装
│   └── ...
└── ...
```

---

## 三、分阶段实施计划

### Phase 0：技术验证（1-2 天）

**目标**：验证 Phaser 4.2 在 SimVerse 技术栈中的可行性

| 任务 | 产出 | 验收标准 |
|------|------|---------|
| Phaser 4.2 安装与基础配置 | `package.json` 依赖 | `npm run build` 成功 |
| 创建最小 Phaser 场景 | Hello World 场景 | Canvas 正常渲染，无报错 |
| Vue + Phaser 集成验证 | `SimverseWorld.vue` 嵌入 Canvas | Vue 生命周期与 Phaser Scene 生命周期正确联动 |
| 性能基准测试 | 1000 个精灵的 FPS 测试 | 安卓中端机 > 30fps |
| 事件桥验证 | Vue ↔ Phaser 双向通信 | 点击精灵 → Vue 收到事件 → 更新状态 → Phaser 响应 |

**风险点**：
- Phaser 4.2 的 API 稳定性（如遇阻塞回退到 3.x）
- WebView 中 WebGL 兼容性（低端机降级 Canvas）

### Phase 1：基础世界地图（3-5 天）

**目标**：用 Phaser 替换现有的 DOM 世界地图，保留核心功能

| 任务 | 说明 |
|------|------|
| 程序化地形生成 | 基于种子生成地形（平原/山地/水域），Tilemap 渲染 |
| 相机系统 | 拖拽平移、双指缩放、边界限制 |
| NPC 精灵渲染 | 从 useSimverse 获取 NPC 列表，渲染为精灵 |
| 建筑/区域标记 | 村庄、城市等建筑的渲染 |
| 点击选中 NPC | 点击 NPC 精灵 → 显示详情面板（复用现有 Vue 组件） |
| 时间/时代显示 | 在 Canvas 上叠加 HUD（或用 Vue DOM 叠加） |
| 性能优化 | 对象池、视锥剔除、LOD（远处简化） |

**与现有功能的对应**：

| 现有功能 | Phaser 实现 |
|---------|------------|
| 世界地图背景 | 程序化 Tilemap + 地形纹理 |
| NPC 列表（右侧） | 仍然是 Vue 列表（不变），但点击可以定位到地图 |
| 详情面板 | 仍然是 Vue 模态框（不变） |
| 时间显示 | Vue HUD 叠加在 Canvas 上方 |

### Phase 2：动画与交互增强（3-5 天）

**目标**：增加 Phaser 特有的动画和交互效果

| 任务 | 说明 |
|------|------|
| NPC 移动动画 | NPC 在地图上的移动路径动画（Tween） |
| 事件特效 | 重大事件的粒子特效（火灾、庆典、战争等） |
| 组织领地可视化 | 不同组织的势力范围（着色区域） |
| 迷你地图 | 右下角小地图，显示全局视野 |
| 时间流逝效果 | 昼夜循环、季节变化的视觉表现 |
| 双指手势 | 移动端手势支持（缩放、旋转） |

### Phase 3：场景分层与高级功能（5-7 天）

**目标**：多场景切换，更深度的游戏化体验

| 任务 | 说明 |
|------|------|
| 场景切换系统 | 世界地图 → 区域特写 → NPC 特写的场景过渡 |
| 区域特写场景 | 进入村庄/城市后的详细视图 |
| 编年史事件可视化 | 在地图上回放历史事件的时间线 |
| 干预模式可视化 | 神干预（投放资源、触发灾难）的视觉效果 |
| 存档/读档动画 | 保存加载的过渡动画 |
| 性能调优 | 低端机降质策略（减少粒子、降低分辨率） |

---

## 四、关键技术决策

### 4.1 渲染模式：WebGL + Canvas 降级

```typescript
const config: Phaser.Types.Core.GameConfig = {
  type: Phaser.AUTO,  // 优先 WebGL，降级 Canvas
  // 低端机检测 → 强制 Canvas + 降低分辨率
  scale: {
    mode: Phaser.Scale.FIT,
    autoCenter: Phaser.Scale.CENTER_BOTH,
  },
  // 像素艺术风格 → 禁用抗锯齿
  pixelArt: false,  // SimVerse 用矢量/扁平风格，开启抗锯齿
}
```

### 4.2 NPC 渲染策略

**问题**：千万级 NPC 不可能全部渲染。

**策略**：
1. **视野剔除**：只渲染相机视口内的 NPC
2. **LOD 分级**：
   - 近景：完整精灵 + 动画
   - 中景：简化精灵 + 无动画
   - 远景：圆点/色块标记
3. **聚合显示**：大量 NPC 聚集时显示为"人群"聚合对象
4. **分层渲染**：按重要性（玩家关注的 NPC 优先渲染）

### 4.3 程序化地图生成

```
地形生成管线：
  1. Perlin Noise 生成高度图
  2. 高度分层 → 水域/平原/丘陵/山地/雪山
  3. 温度/降水图 → 生物群系（森林/沙漠/草原）
  4. 放置聚落（村庄/城市）→ 基于水源和资源
  5. 生成道路网络 → 连接聚落
  6. 生成 Tilemap 数据 → Phaser 渲染
```

**种子化**：所有随机生成基于世界种子，保证可复现。

### 4.4 性能预算

| 指标 | 目标 | 说明 |
|------|------|------|
| **FPS（高端机）** | 60 | 旗舰安卓机 |
| **FPS（中端机）** | ≥ 30 | 骁龙 7 系以上 |
| **FPS（低端机）** | ≥ 20 | 可接受最低 |
| **内存增量** | < 20 MB | 相比纯 DOM 的额外内存 |
| **包体积增量** | < 500 KB | Phaser 引擎本身 |
| **首屏加载** | < 1s | Phaser 初始化时间 |

### 4.5 降级策略

Phaser 加载失败或性能不达标时，自动回退到现有的 DOM 世界视图：

```typescript
// usePhaserWorld.ts 内的降级逻辑
async function initPhaser(): Promise<boolean> {
  try {
    const game = new Phaser.Game(config)
    // 等待 1 帧，检测是否成功初始化
    await new Promise(r => requestAnimationFrame(() => r()))
    return true
  } catch (e) {
    console.warn("[Phaser] init failed, fallback to DOM mode:", e)
    return false
  }
}

// 组件内
const phaserReady = ref(false)

onMounted(async () => {
  phaserReady.value = await initPhaser()
  if (!phaserReady.value) {
    // 显示 DOM 版本的世界视图
  }
})
```

---

## 五、与现有系统的集成点

### 5.1 数据来源

继续使用现有的 `useSimverse` composable：
- NPC 数据：`useSimverse.npcList`
- 世界状态：`useSimverse.worldState`
- 编年史事件：`useSimverse.loadChronicleWorld()`
- 等等...

Phaser 层只做**渲染和交互**，不持有状态。

### 5.2 UI 面板

所有侧面板（NPC 详情、组织、设置、日志等）**继续使用 Vue + Ionic 组件**，通过绝对定位叠加在 Canvas 上方。

好处：
- 复用现有组件，不用重写
- 表单/列表/模态框等复杂 UI 用 Vue 更高效
- Phaser 专注于游戏渲染，职责清晰

### 5.3 DevLogs 页面

DevLogs 页面与 Phaser 无关，继续使用现有的 `SimverseDevLogs.vue`（Vue + Ionic 实现）。

---

## 六、风险与应对

| 风险 | 概率 | 影响 | 应对方案 |
|------|------|------|----------|
| Phaser 4.2 API 不稳定 | 中 | 高 | 封装一层抽象，必要时回退到 3.x |
| 低端机 WebGL 兼容性 | 中 | 中 | Canvas 降级 + 自动检测 |
| 性能不达标（千级 NPC） | 中 | 高 | LOD + 视锥剔除 + 对象池 |
| 开发周期超预期 | 高 | 中 | 分阶段交付，Phase 1 可用即可 |
| 包体积超标 | 低 | 低 | tree-shaking + gzip，500KB 可接受 |
| 与现有 Vue 架构冲突 | 低 | 中 | 严格分离，Phaser 只在 World 页面内 |

---

## 七、参考资源

- **Phaser 4.x 官方文档**：https://phaser.io/learn
- **Phaser 4.x GitHub**：https://github.com/phaserjs/phaser
- **Phaser 3 文档（参考，4.x 有变动）**：https://newdocs.phaser.io/docs/3.88.0/
- **Tilemap 设计工具**：Tiled（https://www.mapeditor.org/）
- **程序化生成参考**：《游戏编程模式》、《Procedural Generation in Game Design》

---

*文档版本：v0.1（规划期）*
*创建日期：2026-07-09*
