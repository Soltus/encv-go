import Phaser from "phaser";
import { TerrainGenerator, TerrainType, TERRAIN_COLORS } from "./TerrainGenerator";
import type { NPCSprite } from "./NPCSprite";
import type { BuildingSprite } from "./BuildingSprite";

export class MiniMap {
  private scene: Phaser.Scene;
  private container: Phaser.GameObjects.Container;
  private mapGraphics: Phaser.GameObjects.Graphics;
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

  constructor(
    scene: Phaser.Scene,
    terrainGenerator: TerrainGenerator,
    mapWidth: number,
    mapHeight: number,
    tileSize: number
  ) {
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
      0x1a1a2e,
      0.9
    );
    this.background.setStrokeStyle(2, 0x8b5cf6, 0.5);
    this.container.add(this.background);

    this.mapGraphics = scene.add.graphics();
    this.container.add(this.mapGraphics);

    this.buildingDots = scene.add.graphics();
    this.container.add(this.buildingDots);

    this.npcDots = scene.add.graphics();
    this.container.add(this.npcDots);

    this.viewportRect = scene.add.rectangle(
      this.miniMapSize / 2,
      this.miniMapSize / 2,
      60,
      40,
      0xffffff,
      0.1
    );
    this.viewportRect.setStrokeStyle(2, 0xffffff, 0.8);
    this.container.add(this.viewportRect);

    this.renderTerrainMiniMap();

    this.container.setInteractive(
      new Phaser.Geom.Rectangle(0, 0, this.miniMapSize, this.miniMapSize),
      Phaser.Geom.Rectangle.Contains
    );

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

    const centerX = (cam.worldView.centerX / (this.mapWidth * this.tileSize)) * this.miniMapSize;
    const centerY = (cam.worldView.centerY / (this.mapHeight * this.tileSize)) * this.miniMapSize;

    this.viewportRect.x = centerX;
    this.viewportRect.y = centerY;
  }

  updateNPCs(npcs: NPCSprite[]): void {
    this.npcDots.clear();

    npcs.forEach((npc) => {
      if (!npc.visible) return;

      const mx = (npc.x / (this.mapWidth * this.tileSize)) * this.miniMapSize;
      const my = (npc.y / (this.mapHeight * this.tileSize)) * this.miniMapSize;

      if (mx < 0 || mx > this.miniMapSize || my < 0 || my > this.miniMapSize) return;

      const color = npc.getNPCData().is_alive ? 0x22c55e : 0x6b7280;
      this.npcDots.fillStyle(color, 0.8);
      this.npcDots.fillRect(mx - 1, my - 1, 2, 2);
    });
  }

  updateBuildings(buildings: BuildingSprite[]): void {
    this.buildingDots.clear();

    buildings.forEach((building) => {
      const mx = (building.x / (this.mapWidth * this.tileSize)) * this.miniMapSize;
      const my = (building.y / (this.mapHeight * this.tileSize)) * this.miniMapSize;

      if (mx < 0 || mx > this.miniMapSize || my < 0 || my > this.miniMapSize) return;

      const typeColors: Record<string, number> = {
        village: 0x8b5cf6,
        city: 0xec4899,
        castle: 0xf59e0b,
        temple: 0x06b6d4,
      };
      const color = typeColors[building.getType()] || 0xffffff;

      this.buildingDots.fillStyle(color, 1);
      this.buildingDots.fillRect(mx - 2, my - 2, 4, 4);
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
