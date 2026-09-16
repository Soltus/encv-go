import Phaser from "phaser";
import type { BuildingSprite } from "./BuildingSprite";
import type { NPCSprite } from "./NPCSprite";
import { TERRAIN_COLORS, type TerrainGenerator, TerrainType } from "./TerrainGenerator";
import type { OrgTerritory } from "./TerritoryRenderer";

// 小地图主题色：与 encv-mobile 紫色主题一致。
const MINIMAP_THEME_PRIMARY = 0x8b5cf6;
const MINIMAP_BG_COLOR = 0x1a1a2e;

export class MiniMap {
  private scene: Phaser.Scene;
  private container: Phaser.GameObjects.Container;
  private mapGraphics: Phaser.GameObjects.Graphics;
  private territoryGraphics: Phaser.GameObjects.Graphics;
  private viewportGlow: Phaser.GameObjects.Rectangle;
  private viewportRect: Phaser.GameObjects.Rectangle;
  private background: Phaser.GameObjects.Rectangle;
  private npcDots: Phaser.GameObjects.Graphics;
  private buildingDots: Phaser.GameObjects.Graphics;
  private terrainGenerator: TerrainGenerator;
  private mapWidth: number;
  private mapHeight: number;
  private tileSize: number;
  private miniMapSize: number;
  private scaleX: number;
  private scaleY: number;
  private isDragging = false;

  constructor(scene: Phaser.Scene, terrainGenerator: TerrainGenerator, mapWidth: number, mapHeight: number, tileSize: number) {
    this.scene = scene;
    this.terrainGenerator = terrainGenerator;
    this.mapWidth = mapWidth;
    this.mapHeight = mapHeight;
    this.tileSize = tileSize;
    this.miniMapSize = 180;

    this.scaleX = this.miniMapSize / (mapWidth * tileSize);
    this.scaleY = this.miniMapSize / (mapHeight * tileSize);

    const screenW = scene.scale.width;
    const screenH = scene.scale.height;
    const posX = screenW - this.miniMapSize - 16;
    const posY = screenH - this.miniMapSize - 16;

    this.container = scene.add.container(posX, posY);
    this.container.setDepth(200);
    this.container.setScrollFactor(0);

    this.background = scene.add.rectangle(
      this.miniMapSize / 2,
      this.miniMapSize / 2,
      this.miniMapSize + 8,
      this.miniMapSize + 8,
      MINIMAP_BG_COLOR,
      0.9
    );
    this.background.setStrokeStyle(2, MINIMAP_THEME_PRIMARY, 0.5);
    this.container.add(this.background);

    this.mapGraphics = scene.add.graphics();
    this.container.add(this.mapGraphics);

    this.territoryGraphics = scene.add.graphics();
    this.container.add(this.territoryGraphics);

    this.buildingDots = scene.add.graphics();
    this.container.add(this.buildingDots);

    this.npcDots = scene.add.graphics();
    this.container.add(this.npcDots);

    // 视口矩形（玩家镜头区域）— 主题紫描边 + 淡紫辉光底层
    this.viewportGlow = scene.add.rectangle(this.miniMapSize / 2, this.miniMapSize / 2, 60, 40, MINIMAP_THEME_PRIMARY, 0.18);
    this.viewportGlow.setBlendMode(Phaser.BlendModes.ADD);
    this.container.add(this.viewportGlow);

    this.viewportRect = scene.add.rectangle(this.miniMapSize / 2, this.miniMapSize / 2, 60, 40, 0xffffff, 0.08);
    this.viewportRect.setStrokeStyle(1.5, MINIMAP_THEME_PRIMARY, 0.95);
    this.container.add(this.viewportRect);

    this.renderTerrainMiniMap();

    this.container.setInteractive(new Phaser.Geom.Rectangle(0, 0, this.miniMapSize, this.miniMapSize), Phaser.Geom.Rectangle.Contains);

    this.setupInteraction();

    scene.scale.on("resize", this.handleResize, this);
  }

  private renderTerrainMiniMap(): void {
    this.mapGraphics.clear();

    const step = 4;
    const pixelW = this.miniMapSize / (this.mapWidth / step);
    const pixelH = this.miniMapSize / (this.mapHeight / step);

    for (let y = 0; y < this.mapHeight; y += step) {
      for (let x = 0; x < this.mapWidth; x += step) {
        const terrain = this.terrainGenerator.getTerrainType(x, y);
        const color = TERRAIN_COLORS[terrain];
        const mx = (x / this.mapWidth) * this.miniMapSize;
        const my = (y / this.mapHeight) * this.miniMapSize;

        this.mapGraphics.fillStyle(color, 1);
        this.mapGraphics.fillRect(mx, my, pixelW + 0.5, pixelH + 0.5);
      }
    }
  }

  private setupInteraction(): void {
    this.container.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
      this.isDragging = true;
      this.updateCameraFromPointer(pointer);
    });

    this.scene.input.on("pointermove", (pointer: Phaser.Input.Pointer) => {
      if (this.isDragging) {
        this.updateCameraFromPointer(pointer);
      }
    });

    this.scene.input.on("pointerup", () => {
      this.isDragging = false;
    });
  }

  private updateCameraFromPointer(pointer: Phaser.Input.Pointer): void {
    const localX = pointer.x - this.container.x;
    const localY = pointer.y - this.container.y;

    if (localX < 0 || localX > this.miniMapSize || localY < 0 || localY > this.miniMapSize) {
      return;
    }

    const worldX = (localX / this.miniMapSize) * (this.mapWidth * this.tileSize);
    const worldY = (localY / this.miniMapSize) * (this.mapHeight * this.tileSize);

    this.scene.cameras.main.centerOn(worldX, worldY);
  }

  updateViewport(): void {
    const cam = this.scene.cameras.main;
    const viewW = cam.width / cam.zoom;
    const viewH = cam.height / cam.zoom;

    const rectW = viewW * this.scaleX;
    const rectH = viewH * this.scaleY;

    this.viewportRect.width = rectW;
    this.viewportRect.height = rectH;
    // 同步辉光底层尺寸，让视口紫色光晕跟随镜头区域
    this.viewportGlow.width = rectW + 4;
    this.viewportGlow.height = rectH + 4;

    const centerX = (cam.worldView.centerX / (this.mapWidth * this.tileSize)) * this.miniMapSize;
    const centerY = (cam.worldView.centerY / (this.mapHeight * this.tileSize)) * this.miniMapSize;

    this.viewportRect.x = centerX;
    this.viewportRect.y = centerY;
    this.viewportGlow.x = centerX;
    this.viewportGlow.y = centerY;
  }

  updateNPCs(npcs: NPCSprite[]): void {
    this.npcDots.clear();

    npcs.forEach(npc => {
      if (!npc.visible) return;

      const mx = (npc.x / (this.mapWidth * this.tileSize)) * this.miniMapSize;
      const my = (npc.y / (this.mapHeight * this.tileSize)) * this.miniMapSize;

      if (mx < 0 || mx > this.miniMapSize || my < 0 || my > this.miniMapSize) return;

      // 使用 NPC 当前生效配色（行为色 ?? 流派色），与 NPCSprite 圆底保持一致
      const color = npc.getDisplayColor();
      this.npcDots.fillStyle(color, 0.85);
      this.npcDots.fillRect(mx - 1, my - 1, 2, 2);
    });
  }

  updateBuildings(buildings: BuildingSprite[]): void {
    this.buildingDots.clear();

    buildings.forEach(building => {
      const mx = (building.x / (this.mapWidth * this.tileSize)) * this.miniMapSize;
      const my = (building.y / (this.mapHeight * this.tileSize)) * this.miniMapSize;

      if (mx < 0 || mx > this.miniMapSize || my < 0 || my > this.miniMapSize) return;

      // 与 BuildingSprite 配色保持同步：紫色梯度 + 金/青强调
      const typeColors: Record<string, number> = {
        village: 0x9f7aea,
        city: 0x8b5cf6,
        castle: 0xf59e0b,
        temple: 0x06b6d4,
      };
      const color = typeColors[building.getType()] || 0xffffff;

      this.buildingDots.fillStyle(color, 1);
      this.buildingDots.fillRect(mx - 2, my - 2, 4, 4);
    });
  }

  /**
   * 在小地图上叠加领土边界（统一紫色描边），让玩家快速看到组织分布。
   * 与 TerritoryRenderer 的边界配色一致，调用方（WorldScene）在 setNPCs 流程里调用。
   */
  updateTerritories(territories: OrgTerritory[]): void {
    this.territoryGraphics.clear();

    territories.forEach(territory => {
      const centerMX = (territory.centerX / this.mapWidth) * this.miniMapSize;
      const centerMY = (territory.centerY / this.mapHeight) * this.miniMapSize;
      const radius = (territory.size / this.mapWidth) * this.miniMapSize;

      // 半透明填充保留组织自身色（弱），边界统一主题紫
      this.territoryGraphics.fillStyle(territory.color, 0.18);
      this.territoryGraphics.fillCircle(centerMX, centerMY, radius);
      this.territoryGraphics.lineStyle(1, MINIMAP_THEME_PRIMARY, 0.75);
      this.territoryGraphics.strokeCircle(centerMX, centerMY, radius);
    });
  }

  private handleResize(gameSize: Phaser.Structs.Size): void {
    const posX = gameSize.width - this.miniMapSize - 16;
    const posY = gameSize.height - this.miniMapSize - 16;
    this.container.setPosition(posX, posY);
  }

  setVisible(visible: boolean): void {
    this.container.setVisible(visible);
  }

  toggle(): boolean {
    const visible = this.container.visible;
    this.container.setVisible(!visible);
    return !visible;
  }

  destroy(): void {
    this.scene.scale.off("resize", this.handleResize, this);
    this.container.destroy();
  }
}
