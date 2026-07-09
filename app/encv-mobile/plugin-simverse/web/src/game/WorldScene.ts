import Phaser from "phaser";
import { TerrainGenerator, TerrainType, TERRAIN_COLORS } from "./TerrainGenerator";
import { NPCSprite } from "./NPCSprite";
import { BuildingSprite, BuildingType } from "./BuildingSprite";
import { phaserEventBus, PHASER_EVENTS } from "./PhaserEventBus";
import { EventEffectManager, type EventEffectType } from "./EventEffectManager";
import { TerritoryRenderer, type OrgTerritory } from "./TerritoryRenderer";
import { MiniMap } from "./MiniMap";
import { DayNightCycle, type TimeOfDay } from "./DayNightCycle";
import type { SimverseNPC } from "@/composables/useSimverse";

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

  private npcMoveTimer = 0;
  private npcMoveInterval = 5000;

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

    this.territoryRenderer = new TerritoryRenderer(
      this,
      this.terrainGenerator,
      this.mapWidth,
      this.mapHeight,
      this.tileSize
    );
    this.createSampleTerritories();

    this.eventEffectManager = new EventEffectManager(this);

    this.interactionGfx = this.add.graphics();
    this.interactionGfx.setDepth(5);

    this.miniMap = new MiniMap(
      this,
      this.terrainGenerator,
      this.mapWidth,
      this.mapHeight,
      this.tileSize
    );

    this.cameras.main.on("camerazoomupdate", () => {
      this.miniMap.updateViewport();
    });

    this.cameras.main.on("camerascroll", () => {
      this.miniMap.updateViewport();
    });

    this.setupKeyboardShortcuts();

    phaserEventBus.emit(PHASER_EVENTS.WORLD_READY);

    this.time.delayedCall(2000, () => {
      this.spawnSampleEffects();
    });
  }

  private createSampleTerritories(): void {
    const orgColors = [0x8b5cf6, 0xec4899, 0xf59e0b, 0x06b6d4, 0x22c55e];
    const orgNames = ["紫月王国", "玫瑰联盟", "金阳帝国", "碧海商会", "翠林部落"];

    const settlements = this.terrainGenerator.findSettlementLocations(
      this.mapWidth,
      this.mapHeight,
      5
    );

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

    return (
      (Math.floor(adjust(r) * 255) << 16) |
      (Math.floor(adjust(g) * 255) << 8) |
      Math.floor(adjust(b) * 255)
    );
  }

  private createBuildings(): void {
    const settlements = this.terrainGenerator.findSettlementLocations(
      this.mapWidth,
      this.mapHeight,
      15
    );

    const villageNames = [
      "橡树村", "河边镇", "山谷村", "麦田村", "青石镇",
      "松柏林", "湖畔村", "风车镇", "玫瑰村", "晨曦镇",
      "雾灵山", "月光村", "艳阳镇", "丰收村", "清泉镇",
    ];

    settlements.forEach((settlement, index) => {
      const type: BuildingType =
        settlement.size >= 3 ? "city" : settlement.size === 2 ? "village" : "village";
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
      const newZoom = Phaser.Math.Clamp(
        this.cameras.main.zoom * zoomFactor,
        this.minZoom,
        this.maxZoom
      );

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
    const newZoom = Phaser.Math.Clamp(
      this.initialZoom * zoomRatio,
      this.minZoom,
      this.maxZoom
    );

    const worldPoint = this.cameras.main.getWorldPoint(this.pinchMidpoint.x, this.pinchMidpoint.y);
    this.cameras.main.zoom = newZoom;
    this.cameras.main.centerOn(worldPoint.x, worldPoint.y);
  }

  private setupKeyboardShortcuts(): void {
    this.input.keyboard?.on("keydown-SPACE", () => {
      this.cameras.main.centerOn(
        (this.mapWidth * this.tileSize) / 2,
        (this.mapHeight * this.tileSize) / 2
      );
      this.cameras.main.setZoom(0.5);
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
  }

  setNPCBehaviors(behaviors: Map<number, string>): void {
    this.npcSprites.forEach((sprite) => {
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
    const spread = (Math.min(this.mapWidth, this.mapHeight) * this.tileSize) * 0.4;

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
        sprite.updateNPC(npc);
      } else {
        sprite = new NPCSprite(this, x, y, npc);
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
  }

  private updateNPCLOD(): void {
    const zoom = this.cameras.main.zoom;

    let lodLevel: "full" | "medium" | "low" = "low";
    if (zoom > 0.8) lodLevel = "full";
    else if (zoom > 0.5) lodLevel = "medium";

    this.npcSprites.forEach((sprite) => {
      sprite.setLODLevel(lodLevel);
    });
  }

  private updateNPCMovements(delta: number): void {
    this.npcMoveTimer += delta;

    if (this.npcMoveTimer >= this.npcMoveInterval) {
      this.npcMoveTimer = 0;

      const worldW = this.mapWidth * this.tileSize;
      const worldH = this.mapHeight * this.tileSize;

      this.npcSprites.forEach((sprite) => {
        if (!sprite.visible || !sprite.active || sprite.isMoving()) return;
        if (Math.random() > 0.3) return;

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
    const sprite = this.npcSprites.find((s) => s.getNPCData().id === npcId);
    if (sprite) {
      this.cameras.main.centerOn(sprite.x, sprite.y);
      this.cameras.main.setZoom(1.2);
    }
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
