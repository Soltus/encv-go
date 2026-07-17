import Phaser from "phaser";
import { gsap } from "gsap";
import type { SimverseNPC } from "@/composables/useSimverse";
import { BuildingSprite, type BuildingType } from "./BuildingSprite";
import { ARCH_META, ARCHETYPES } from "./builds";
import { DayNightCycle, type TimeOfDay } from "./DayNightCycle";
import { EventEffectManager, type EventEffectType } from "./EventEffectManager";
import { MiniMap } from "./MiniMap";
import { NPCSprite } from "./NPCSprite";
import { PHASER_EVENTS, phaserEventBus } from "./PhaserEventBus";
import { TERRAIN_COLORS, TerrainGenerator, TerrainType } from "./TerrainGenerator";
import { type OrgTerritory, TerritoryRenderer } from "./TerritoryRenderer";

export class WorldScene extends Phaser.Scene {
  private terrainGenerator!: TerrainGenerator;
  private mapWidth = 200;
  private mapHeight = 200;
  private tileSize = 32;
  private npcSprites: NPCSprite[] = [];
  private buildingSprites: BuildingSprite[] = [];
  private npcPool: NPCSprite[] = [];
  private maxVisibleNPCs = 200;
  private worldSeed = 12345;
  private isDragging = false;
  private lastPointerX = 0;
  private lastPointerY = 0;
  private minZoom = 0.3;
  private maxZoom = 2;

  private eventEffectManager!: EventEffectManager;
  private territoryRenderer!: TerritoryRenderer;
  private miniMap!: MiniMap;
  private dayNightCycle!: DayNightCycle;

  // 环境粒子（萤火/尘埃）：让空旷的世界持续有"呼吸感"，避免 HUD/世界发呆。
  private ambientGfx!: Phaser.GameObjects.Graphics;
  private ambientParticles: {
    x: number;
    y: number;
    vx: number;
    vy: number;
    r: number;
    baseAlpha: number;
    phase: number;
    speed: number;
  }[] = [];

  private npcMoveTimer = 0;
  private npcMoveInterval = 3000;

  private interactionGfx!: Phaser.GameObjects.Graphics;
  private interactionTimer = 0;
  private interactionEnabled = true;

  private isPinching = false;
  private initialPinchDistance = 0;
  private initialZoom = 1;
  private pinchMidpoint = { x: 0, y: 0 };

  constructor() {
    super("WorldScene");
  }

  init(data: { seed?: number; mapWidth?: number; mapHeight?: number }): void {
    if (data.seed) this.worldSeed = data.seed;
    if (data.mapWidth) this.mapWidth = data.mapWidth;
    if (data.mapHeight) data.mapHeight;
  }

  preload(): void {}

  create(): void {
    this.terrainGenerator = new TerrainGenerator(this.worldSeed);

    this.createTerrainTexture();
    this.createBuildings();
    this.setupCamera();
    this.setupInput();

    const worldW = this.mapWidth * this.tileSize;
    const worldH = this.mapHeight * this.tileSize;

    this.dayNightCycle = new DayNightCycle(this, worldW, worldH);
    this.dayNightCycle.setCycleDuration(60000);

    this.territoryRenderer = new TerritoryRenderer(this, this.terrainGenerator, this.mapWidth, this.mapHeight, this.tileSize);
    this.createSampleTerritories();

    this.eventEffectManager = new EventEffectManager(this);

    this.interactionGfx = this.add.graphics();
    this.interactionGfx.setDepth(5);

    this.miniMap = new MiniMap(this, this.terrainGenerator, this.mapWidth, this.mapHeight, this.tileSize);

    this.cameras.main.on("camerazoomupdate", () => {
      this.miniMap.updateViewport();
    });

    this.cameras.main.on("camerascroll", () => {
      this.miniMap.updateViewport();
    });

    this.createLegend();
    this.createVignette();
    this.createAmbientParticles();

    this.setupKeyboardShortcuts();

    phaserEventBus.emit(PHASER_EVENTS.WORLD_READY);

    this.time.delayedCall(2000, () => {
      this.spawnSampleEffects();
    });
  }

  private createSampleTerritories(): void {
    const orgColors = [0x8b5cf6, 0xec4899, 0xf59e0b, 0x06b6d4, 0x22c55e];
    const orgNames = ["紫月王国", "玫瑰联盟", "金阳帝国", "碧海商会", "翠林部落"];

    const settlements = this.terrainGenerator.findSettlementLocations(this.mapWidth, this.mapHeight, 5);

    const territories: OrgTerritory[] = settlements.map((s, i) => ({
      id: `org_${i}`,
      name: orgNames[i % orgNames.length],
      color: orgColors[i % orgColors.length],
      centerX: s.x,
      centerY: s.y,
      size: 15 + s.size * 8,
    }));

    this.territoryRenderer.setTerritories(territories);
  }

  private spawnSampleEffects(): void {
    this.buildingSprites.forEach((building, index) => {
      if (index % 3 === 0) {
        const types: EventEffectType[] = ["celebration", "fire", "birth", "discovery"];
        const type = types[index % types.length];
        this.eventEffectManager.spawnEffect({
          type,
          x: building.x,
          y: building.y,
          duration: 8000,
          intensity: 0.8,
        });
      }
    });
  }

  private createTerrainTexture(): void {
    const textureKey = "world-terrain";
    const graphics = this.add.graphics();

    for (let y = 0; y < this.mapHeight; y++) {
      for (let x = 0; x < this.mapWidth; x++) {
        const terrain = this.terrainGenerator.getTerrainType(x, y);
        const color = TERRAIN_COLORS[terrain];

        const variation = Math.random() * 0.1 - 0.05;
        const adjustedColor = this.adjustColor(color, variation);

        graphics.fillStyle(adjustedColor, 1);
        graphics.fillRect(x * this.tileSize, y * this.tileSize, this.tileSize, this.tileSize);
      }
    }

    graphics.generateTexture(textureKey, this.mapWidth * this.tileSize, this.mapHeight * this.tileSize);
    graphics.destroy();

    const terrain = this.add.image(0, 0, textureKey).setOrigin(0);
    terrain.setAlpha(0.9);

    this.add
      .grid(
        (this.mapWidth * this.tileSize) / 2,
        (this.mapHeight * this.tileSize) / 2,
        this.mapWidth * this.tileSize,
        this.mapHeight * this.tileSize,
        this.tileSize,
        this.tileSize,
        0xffffff,
        0,
        0xffffff,
        0.03
      )
      .setOrigin(0.5);
  }

  private adjustColor(color: number, amount: number): number {
    const r = ((color >> 16) & 255) / 255;
    const g = ((color >> 8) & 255) / 255;
    const b = (color & 255) / 255;

    const adjust = (c: number) => {
      const adjusted = c + amount;
      return Math.max(0, Math.min(1, adjusted));
    };

    return (Math.floor(adjust(r) * 255) << 16) | (Math.floor(adjust(g) * 255) << 8) | Math.floor(adjust(b) * 255);
  }

  private createBuildings(): void {
    const settlements = this.terrainGenerator.findSettlementLocations(this.mapWidth, this.mapHeight, 15);

    const villageNames = [
      "橡树村",
      "河边镇",
      "山谷村",
      "麦田村",
      "青石镇",
      "松柏林",
      "湖畔村",
      "风车镇",
      "玫瑰村",
      "晨曦镇",
      "雾灵山",
      "月光村",
      "艳阳镇",
      "丰收村",
      "清泉镇",
    ];

    settlements.forEach((settlement, index) => {
      const type: BuildingType = settlement.size >= 3 ? "city" : settlement.size === 2 ? "village" : "village";
      const name = villageNames[index % villageNames.length];

      const sprite = new BuildingSprite(
        this,
        settlement.x * this.tileSize + this.tileSize / 2,
        settlement.y * this.tileSize + this.tileSize / 2,
        type,
        name
      );
      this.add.existing(sprite);
      this.buildingSprites.push(sprite);
    });
  }

  private setupCamera(): void {
    const worldWidth = this.mapWidth * this.tileSize;
    const worldHeight = this.mapHeight * this.tileSize;

    this.cameras.main.setBounds(0, 0, worldWidth, worldHeight);
    this.cameras.main.setZoom(0.5);
    this.cameras.main.centerOn(worldWidth / 2, worldHeight / 2);

    this.cameras.main.on("camerazoomupdate", () => {
      phaserEventBus.emit(PHASER_EVENTS.CAMERA_ZOOM, this.cameras.main.zoom);
      this.updateNPCLOD();
    });

    this.cameras.main.on("camerascroll", () => {
      phaserEventBus.emit(PHASER_EVENTS.CAMERA_MOVE, {
        x: this.cameras.main.scrollX,
        y: this.cameras.main.scrollY,
      });
    });
  }

  private setupInput(): void {
    this.input.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
      if (this.checkPinchStart()) {
        return;
      }

      if (pointer.button === 1 || pointer.button === 2) {
        this.isDragging = true;
        this.lastPointerX = pointer.x;
        this.lastPointerY = pointer.y;
        this.game.canvas.style.cursor = "grabbing";
      }
    });

    this.input.on("pointerup", (pointer: Phaser.Input.Pointer) => {
      if (this.isPinching && !this.hasTwoActivePointers()) {
        this.isPinching = false;
        return;
      }

      if (this.isDragging && pointer.button !== 0) {
        this.isDragging = false;
        this.game.canvas.style.cursor = "default";
      }
    });

    this.input.on("pointermove", (pointer: Phaser.Input.Pointer) => {
      if (this.isPinching) {
        this.updatePinch();
        return;
      }

      if (this.isDragging) {
        const dx = pointer.x - this.lastPointerX;
        const dy = pointer.y - this.lastPointerY;

        this.cameras.main.scrollX -= dx / this.cameras.main.zoom;
        this.cameras.main.scrollY -= dy / this.cameras.main.zoom;

        this.lastPointerX = pointer.x;
        this.lastPointerY = pointer.y;
      }
    });

    this.input.on("wheel", (pointer: Phaser.Input.Pointer, gameObjects: any[], deltaX: number, deltaY: number) => {
      const zoomFactor = deltaY > 0 ? 0.9 : 1.1;
      const newZoom = Phaser.Math.Clamp(this.cameras.main.zoom * zoomFactor, this.minZoom, this.maxZoom);

      const worldPoint = this.cameras.main.getWorldPoint(pointer.x, pointer.y);
      this.cameras.main.zoom = newZoom;
      this.cameras.main.centerOn(worldPoint.x, worldPoint.y);
    });
  }

  private checkPinchStart(): boolean {
    const p1 = this.input.pointer1;
    const p2 = this.input.pointer2;

    if (p1 && p2 && p1.active && p2.active) {
      this.startPinch(p1, p2);
      return true;
    }
    return false;
  }

  private hasTwoActivePointers(): boolean {
    const p1 = this.input.pointer1;
    const p2 = this.input.pointer2;
    return !!(p1 && p2 && p1.active && p2.active);
  }

  private startPinch(p1: Phaser.Input.Pointer, p2: Phaser.Input.Pointer): void {
    this.isPinching = true;
    this.isDragging = false;

    this.initialPinchDistance = Phaser.Math.Distance.Between(p1.x, p1.y, p2.x, p2.y);
    this.initialZoom = this.cameras.main.zoom;
    this.pinchMidpoint = {
      x: (p1.x + p2.x) / 2,
      y: (p1.y + p2.y) / 2,
    };
  }

  private updatePinch(): void {
    const p1 = this.input.pointer1;
    const p2 = this.input.pointer2;

    if (!p1 || !p2 || !p1.active || !p2.active) {
      this.isPinching = false;
      return;
    }

    const currentDistance = Phaser.Math.Distance.Between(p1.x, p1.y, p2.x, p2.y);
    if (this.initialPinchDistance === 0) return;

    const zoomRatio = currentDistance / this.initialPinchDistance;
    const newZoom = Phaser.Math.Clamp(this.initialZoom * zoomRatio, this.minZoom, this.maxZoom);

    const worldPoint = this.cameras.main.getWorldPoint(this.pinchMidpoint.x, this.pinchMidpoint.y);
    this.cameras.main.zoom = newZoom;
    this.cameras.main.centerOn(worldPoint.x, worldPoint.y);
  }

  private setupKeyboardShortcuts(): void {
    this.input.keyboard?.on("keydown-SPACE", () => {
      // GSAP 驱动的镜头回退：与 returnToWorldView 一致，键盘快捷键也走平滑过渡
      this.returnToWorldView();
    });

    this.input.keyboard?.on("keydown-M", () => {
      this.miniMap.toggle();
    });

    this.input.keyboard?.on("keydown-N", () => {
      this.dayNightCycle.toggle();
    });

    this.input.keyboard?.on("keydown-T", () => {
      const isVisible = this.territoryRenderer.isVisible();
      this.territoryRenderer.setVisible(!isVisible);
    });

    this.input.keyboard?.on("keydown-ONE", () => {
      this.dayNightCycle.setTimeOfDay("dawn");
    });

    this.input.keyboard?.on("keydown-TWO", () => {
      this.dayNightCycle.setTimeOfDay("day");
    });

    this.input.keyboard?.on("keydown-THREE", () => {
      this.dayNightCycle.setTimeOfDay("dusk");
    });

    this.input.keyboard?.on("keydown-FOUR", () => {
      this.dayNightCycle.setTimeOfDay("night");
    });

    this.input.keyboard?.on("keydown-I", () => {
      this.interactionEnabled = !this.interactionEnabled;
      if (!this.interactionEnabled && this.interactionGfx) {
        this.interactionGfx.clear();
      }
    });

    this.input.keyboard?.on("keydown-L", () => {
      this.toggleLegend();
    });
  }

  // 流派图例：固定在左上角（不随相机缩放/平移），帮助识别彩色头像对应的流派。
  // 按 L 键开关。
  private legend?: Phaser.GameObjects.Container;
  private legendVisible = true;

  private createLegend(): void {
    const c = this.add.container(14, 14);
    c.setScrollFactor(0);
    c.setDepth(1000);
    c.setAlpha(0.92);

    const rowH = 18;
    const title = this.add.text(0, 0, "流派图例", {
      fontSize: "11px",
      color: "#ffffff",
      fontStyle: "bold",
    });
    title.setShadow(0, 1, "#000000", 3);
    c.add(title);

    ARCHETYPES.forEach((key, i) => {
      const meta = ARCH_META[key];
      const y = 20 + i * rowH;
      const dot = this.add.circle(9, y + 8, 7, meta.color).setStrokeStyle(1, 0xffffff, 0.85);
      const em = this.add.text(9, y + 8, meta.emoji, { fontSize: "10px" }).setOrigin(0.5);
      const label = this.add.text(22, y + 3, meta.name, {
        fontSize: "11px",
        color: "#ffffff",
      });
      label.setShadow(0, 1, "#000000", 3);
      c.add([dot, em, label]);
    });

    const totalH = 20 + ARCHETYPES.length * rowH + 6;
    const bg = this.add.graphics();
    bg.fillStyle(0x000000, 0.45);
    bg.fillRoundedRect(-6, -4, 86, totalH, 8);
    c.addAt(bg, 0);

    this.legend = c;
  }

  private toggleLegend(): void {
    if (!this.legend) return;
    this.legendVisible = !this.legendVisible;
    this.legend.setVisible(this.legendVisible);
  }

  // 暗角（vignette）叠加：给平坦的地形瓦片增加空间纵深感，缓解"辣眼睛"的平铺感。
  // 固定不随相机移动，置于图例之下。
  private createVignette(): void {
    const w = this.scale.width || 800;
    const h = this.scale.height || 600;
    const key = "world-vignette";
    if (this.textures.exists(key)) this.textures.remove(key);

    const canvasTex = this.textures.createCanvas(key, w, h);
    if (!canvasTex) return;

    const ctx = canvasTex.getContext();
    const grd = ctx.createRadialGradient(w / 2, h / 2, Math.min(w, h) * 0.3, w / 2, h / 2, Math.max(w, h) * 0.75);
    grd.addColorStop(0, "rgba(0,0,0,0)");
    grd.addColorStop(1, "rgba(0,0,0,0.35)");
    ctx.fillStyle = grd;
    ctx.fillRect(0, 0, w, h);
    canvasTex.refresh();

    this.add.image(0, 0, key).setOrigin(0).setScrollFactor(0).setDepth(900);
  }

  // 环境粒子（萤火/尘埃）：在世界空间持续缓缓漂浮、明灭，给空旷地图注入"活着"的呼吸感。
  private createAmbientParticles(): void {
    const worldW = this.mapWidth * this.tileSize;
    const worldH = this.mapHeight * this.tileSize;

    this.ambientGfx = this.add.graphics();
    this.ambientGfx.setDepth(1);

    const count = 90;
    for (let i = 0; i < count; i++) {
      this.ambientParticles.push({
        x: Math.random() * worldW,
        y: Math.random() * worldH,
        vx: (Math.random() - 0.5) * 6,
        vy: -4 - Math.random() * 8,
        r: 1 + Math.random() * 2,
        baseAlpha: 0.12 + Math.random() * 0.35,
        phase: Math.random() * Math.PI * 2,
        speed: 0.5 + Math.random() * 1.5,
      });
    }
  }

  private updateAmbientParticles(delta: number): void {
    if (!this.ambientGfx) return;
    const dt = delta / 1000;
    const worldW = this.mapWidth * this.tileSize;
    const worldH = this.mapHeight * this.tileSize;

    this.ambientGfx.clear();
    for (const p of this.ambientParticles) {
      p.x += p.vx * dt;
      p.y += p.vy * dt;
      p.phase += dt * p.speed;

      if (p.y < -10) {
        p.y = worldH + 10;
        p.x = Math.random() * worldW;
      }
      if (p.x < -10) p.x = worldW + 10;
      if (p.x > worldW + 10) p.x = -10;

      const a = p.baseAlpha * (0.5 + 0.5 * Math.sin(p.phase));
      this.ambientGfx.fillStyle(0xffe9a8, a);
      this.ambientGfx.fillCircle(p.x, p.y, p.r);
    }
  }

  setNPCBehaviors(behaviors: Map<number, string>): void {
    this.npcSprites.forEach(sprite => {
      const id = sprite.getNPCData().id;
      const cn = behaviors.get(id);
      if (cn !== undefined) {
        sprite.setBehavior(cn);
      }
    });
  }

  setNPCs(npcs: SimverseNPC[]): void {
    const worldCenterX = (this.mapWidth * this.tileSize) / 2;
    const worldCenterY = (this.mapHeight * this.tileSize) / 2;
    const spread = Math.min(this.mapWidth, this.mapHeight) * this.tileSize * 0.4;

    const displayNPCs = npcs.slice(0, this.maxVisibleNPCs);

    displayNPCs.forEach((npc, index) => {
      const angle = (index / displayNPCs.length) * Math.PI * 2 + Math.random() * 0.5;
      const radius = spread * (0.3 + Math.random() * 0.7);
      const x = worldCenterX + Math.cos(angle) * radius;
      const y = worldCenterY + Math.sin(angle) * radius;

      let sprite: NPCSprite;
      if (index < this.npcPool.length) {
        sprite = this.npcPool[index];
        sprite.setVisible(true);
        sprite.setActive(true);
        sprite.setPosition(x, y);
        sprite.setDepth(10);
        sprite.updateNPC(npc);
      } else {
        sprite = new NPCSprite(this, x, y, npc);
        sprite.setDepth(10);
        this.add.existing(sprite);
        this.npcPool.push(sprite);

        sprite.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
          if (pointer.button === 0) {
            phaserEventBus.emit(PHASER_EVENTS.NPC_CLICK, npc);
          }
        });
      }

      this.npcSprites[index] = sprite;
    });

    for (let i = displayNPCs.length; i < this.npcSprites.length; i++) {
      this.npcSprites[i].setVisible(false);
      this.npcSprites[i].setActive(false);
    }

    this.npcSprites.length = displayNPCs.length;
    this.updateNPCLOD();
    this.miniMap.updateNPCs(this.npcSprites);
    this.miniMap.updateBuildings(this.buildingSprites);
    // 同步领土到小地图（紫色描边圆圈），让玩家看到组织分布
    this.miniMap.updateTerritories(this.territoryRenderer.getTerritories());
  }

  private updateNPCLOD(): void {
    const zoom = this.cameras.main.zoom;

    let lodLevel: "full" | "medium" | "low" = "low";
    if (zoom > 0.8) lodLevel = "full";
    else if (zoom > 0.5) lodLevel = "medium";

    this.npcSprites.forEach(sprite => {
      sprite.setLODLevel(lodLevel);
    });
  }

  private updateNPCMovements(delta: number): void {
    this.npcMoveTimer += delta;

    if (this.npcMoveTimer >= this.npcMoveInterval) {
      this.npcMoveTimer = 0;

      const worldW = this.mapWidth * this.tileSize;
      const worldH = this.mapHeight * this.tileSize;

      this.npcSprites.forEach(sprite => {
        if (!sprite.visible || !sprite.active || sprite.isMoving()) return;
        if (Math.random() > 0.5) return;

        const moveRange = 100;
        let newX = sprite.x + (Math.random() - 0.5) * moveRange * 2;
        let newY = sprite.y + (Math.random() - 0.5) * moveRange * 2;

        newX = Phaser.Math.Clamp(newX, 50, worldW - 50);
        newY = Phaser.Math.Clamp(newY, 50, worldH - 50);

        sprite.moveNPCTo(newX, newY, 4000);
      });
    }
  }

  update(time: number, delta: number): void {
    super.update(time, delta);

    this.dayNightCycle.update(delta);
    this.eventEffectManager.update(delta);
    this.updateNPCMovements(delta);
    this.updateAmbientParticles(delta);
    this.miniMap.updateViewport();

    this.interactionTimer += delta;
    if (this.interactionTimer >= 800) {
      this.interactionTimer = 0;
      this.drawInteractions();
    }
  }

  // NPC 间交互事件流：把处于社交/交易等互动行为的 NPC 用连线可视化
  private drawInteractions(): void {
    if (!this.interactionGfx) return;
    this.interactionGfx.clear();
    if (!this.interactionEnabled) return;

    const social: NPCSprite[] = [];
    for (const s of this.npcSprites) {
      if (!s.visible || !s.active) continue;
      const b = s.getBehaviorCN();
      if (b && (b.includes("社交") || b.includes("交易") || b.includes("会谈") || b.includes("恋爱"))) {
        social.push(s);
      }
    }

    const maxLines = 60;
    const maxDist = 150;
    let drawn = 0;
    for (let i = 0; i < social.length && drawn < maxLines; i++) {
      for (let j = i + 1; j < social.length && drawn < maxLines; j++) {
        const a = social[i];
        const c = social[j];
        const d = Phaser.Math.Distance.Between(a.x, a.y, c.x, c.y);
        if (d <= maxDist) {
          const alpha = 0.5 * (1 - d / maxDist);
          this.interactionGfx.lineStyle(1.5, 0x60a5fa, alpha);
          this.interactionGfx.beginPath();
          this.interactionGfx.moveTo(a.x, a.y);
          this.interactionGfx.lineTo(c.x, c.y);
          this.interactionGfx.strokePath();
          drawn++;
        }
      }
    }
  }

  getZoom(): number {
    return this.cameras.main.zoom;
  }

  setZoom(zoom: number): void {
    this.cameras.main.setZoom(Phaser.Math.Clamp(zoom, this.minZoom, this.maxZoom));
  }

  centerOnNPC(npcId: number): void {
    const sprite = this.npcSprites.find(s => s.getNPCData().id === npcId);
    if (sprite) {
      // GSAP 驱动的镜头推近：平滑居中到 NPC + 放大缩放
      // 与 UI 侧 useSceneTransition.transitionToScene() 配合，构成"world → focus"镜头转场
      this.smoothCenterOn(this.cameras.main, sprite.x, sprite.y, 800);
      this.smoothZoom(this.cameras.main, 1.5, 600);
    }
  }

  /**
   * 返回世界俯瞰视角：GSAP 驱动的镜头回退（居中世界中心 + 缩放回 1.0）。
   * 用于 focus HUD 场景退回 world，与 UI 侧 useSceneTransition 同步触发。
   */
  returnToWorldView(): void {
    const worldCenterX = (this.mapWidth * this.tileSize) / 2;
    const worldCenterY = (this.mapHeight * this.tileSize) / 2;
    this.smoothCenterOn(this.cameras.main, worldCenterX, worldCenterY, 800);
    this.smoothZoom(this.cameras.main, 1.0, 600);
  }

  /**
   * GSAP 驱动的相机平滑居中：替代 Phaser 默认的 camera.centerOn()，
   * 提供 power3.inOut 缓动以匹配 encv-mobile 的 GSAP 动效基调。
   */
  private smoothCenterOn(camera: Phaser.Cameras.Scene2D.Camera, x: number, y: number, duration: number = 800): void {
    gsap.to(camera, {
      scrollX: x - camera.width / 2,
      scrollY: y - camera.height / 2,
      duration: duration / 1000,
      ease: "power3.inOut",
      overwrite: "auto",
    });
  }

  /**
   * GSAP 驱动的相机平滑缩放：替代 Phaser 默认的 camera.zoomTo()/setZoom()，
   * 提供 power2.inOut 缓动。覆盖 auto 避免与 wheel/pinch 即时缩放叠加冲突。
   */
  private smoothZoom(camera: Phaser.Cameras.Scene2D.Camera, zoom: number, duration: number = 600): void {
    gsap.to(camera, {
      zoom: zoom,
      duration: duration / 1000,
      ease: "power2.inOut",
      overwrite: "auto",
    });
  }

  getDayNightCycle(): DayNightCycle {
    return this.dayNightCycle;
  }

  getTimeOfDay(): TimeOfDay {
    return this.dayNightCycle.getTimeOfDay();
  }

  setDayNightEnabled(enabled: boolean): void {
    this.dayNightCycle.setEnabled(enabled);
  }

  toggleMiniMap(): boolean {
    return this.miniMap.toggle();
  }

  setMiniMapVisible(visible: boolean): void {
    this.miniMap.setVisible(visible);
  }

  setTerritoriesVisible(visible: boolean): void {
    this.territoryRenderer.setVisible(visible);
  }

  spawnEffect(type: EventEffectType, x: number, y: number, duration = 5000, intensity = 1): string {
    return this.eventEffectManager.spawnEffect({
      type,
      x,
      y,
      duration,
      intensity,
    });
  }
}
