import Phaser from "phaser";
import { TerrainGenerator, TerrainType, TERRAIN_COLORS } from "./TerrainGenerator";
import { NPCSprite } from "./NPCSprite";
import { BuildingSprite, BuildingType } from "./BuildingSprite";
import { phaserEventBus, PHASER_EVENTS } from "./PhaserEventBus";
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

    phaserEventBus.emit(PHASER_EVENTS.WORLD_READY);
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
      if (pointer.button === 1 || pointer.button === 2) {
        this.isDragging = true;
        this.lastPointerX = pointer.x;
        this.lastPointerY = pointer.y;
        this.game.canvas.style.cursor = "grabbing";
      }
    });

    this.input.on("pointerup", (pointer: Phaser.Input.Pointer) => {
      if (this.isDragging && pointer.button !== 0) {
        this.isDragging = false;
        this.game.canvas.style.cursor = "default";
      }
    });

    this.input.on("pointermove", (pointer: Phaser.Input.Pointer) => {
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

    this.input.keyboard?.on("keydown-SPACE", () => {
      this.cameras.main.centerOn(
        (this.mapWidth * this.tileSize) / 2,
        (this.mapHeight * this.tileSize) / 2
      );
      this.cameras.main.setZoom(0.5);
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

  update(time: number, delta: number): void {
    super.update(time, delta);
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
}
