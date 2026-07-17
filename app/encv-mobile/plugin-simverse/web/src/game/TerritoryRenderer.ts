import Phaser from "phaser";
import type { TerrainGenerator } from "./TerrainGenerator";

// 领土描边统一使用主题紫色：让所有组织边界视觉上归一到 encv-mobile 紫色主题。
const TERRITORY_BORDER_COLOR = 0x8b5cf6;

export interface OrgTerritory {
  id: string;
  name: string;
  color: number;
  centerX: number;
  centerY: number;
  size: number;
}

export class TerritoryRenderer {
  private scene: Phaser.Scene;
  private graphics: Phaser.GameObjects.Graphics;
  private terrainGenerator: TerrainGenerator;
  private territories: OrgTerritory[] = [];
  private territoryTexture: Phaser.GameObjects.Image | null = null;
  private mapWidth: number;
  private mapHeight: number;
  private tileSize: number;

  constructor(scene: Phaser.Scene, terrainGenerator: TerrainGenerator, mapWidth: number, mapHeight: number, tileSize: number) {
    this.scene = scene;
    this.terrainGenerator = terrainGenerator;
    this.mapWidth = mapWidth;
    this.mapHeight = mapHeight;
    this.tileSize = tileSize;

    this.graphics = scene.add.graphics();
    this.graphics.setDepth(5);
    this.graphics.setAlpha(0.25);
  }

  setTerritories(territories: OrgTerritory[]): void {
    this.territories = territories;
    this.renderTerritories();
  }

  getTerritories(): OrgTerritory[] {
    return this.territories;
  }

  private renderTerritories(): void {
    if (this.territoryTexture) {
      this.territoryTexture.destroy();
      this.territoryTexture = null;
    }

    const renderGraphics = this.scene.add.graphics();

    this.territories.forEach(territory => {
      this.drawVoronoiRegion(renderGraphics, territory);
    });

    const textureKey = "territory-overlay";
    const width = this.mapWidth * this.tileSize;
    const height = this.mapHeight * this.tileSize;

    renderGraphics.generateTexture(textureKey, width, height);
    renderGraphics.destroy();

    this.territoryTexture = this.scene.add.image(0, 0, textureKey).setOrigin(0);
    this.territoryTexture.setDepth(5);
    this.territoryTexture.setAlpha(0.25);

    this.graphics.destroy();
  }

  private drawVoronoiRegion(graphics: Phaser.GameObjects.Graphics, territory: OrgTerritory): void {
    const { centerX, centerY, color, size } = territory;
    const centerWorldX = centerX * this.tileSize;
    const centerWorldY = centerY * this.tileSize;
    const radius = size * this.tileSize;

    graphics.fillStyle(color, 0.4);

    const curvePoints: Phaser.Math.Vector2[] = [];
    const segments = 48;

    for (let i = 0; i < segments; i++) {
      const angle = (i / segments) * Math.PI * 2;
      const noise = this.terrainGenerator.getHeight(centerX + Math.cos(angle) * size * 0.5, centerY + Math.sin(angle) * size * 0.5, 0.05);
      const r = radius * (0.7 + noise * 0.5 + Math.random() * 0.1);
      const x = centerWorldX + Math.cos(angle) * r;
      const y = centerWorldY + Math.sin(angle) * r;
      curvePoints.push(new Phaser.Math.Vector2(x, y));
    }

    graphics.beginPath();
    graphics.moveTo(curvePoints[0].x, curvePoints[0].y);

    for (let i = 1; i < curvePoints.length; i++) {
      graphics.lineTo(curvePoints[i].x, curvePoints[i].y);
    }

    graphics.closePath();
    graphics.fillPath();

    // 边界统一使用主题紫色（半透明），让多组织共存时归一到主题视觉
    graphics.lineStyle(2, TERRITORY_BORDER_COLOR, 0.6);
    graphics.beginPath();
    graphics.moveTo(curvePoints[0].x, curvePoints[0].y);
    for (let i = 1; i < curvePoints.length; i++) {
      graphics.lineTo(curvePoints[i].x, curvePoints[i].y);
    }
    graphics.closePath();
    graphics.strokePath();
  }

  addTerritory(territory: OrgTerritory): void {
    this.territories.push(territory);
    this.renderTerritories();
  }

  removeTerritory(id: string): void {
    this.territories = this.territories.filter(t => t.id !== id);
    this.renderTerritories();
  }

  clear(): void {
    this.territories = [];
    if (this.territoryTexture) {
      this.territoryTexture.destroy();
    }
    this.territoryTexture = null;
  }

  setAlpha(alpha: number): void {
    if (this.territoryTexture) {
      this.territoryTexture.setAlpha(alpha);
    }
  }

  setVisible(visible: boolean): void {
    if (this.territoryTexture) {
      this.territoryTexture.setVisible(visible);
    }
  }

  isVisible(): boolean {
    return this.territoryTexture?.visible ?? true;
  }

  destroy(): void {
    this.clear();
    this.graphics?.destroy();
  }
}
