import Phaser from "phaser";

export type BuildingType = "village" | "city" | "castle" | "temple";

const BUILDING_COLORS: Record<BuildingType, number> = {
  village: 0x8b5cf6,
  city: 0xec4899,
  castle: 0xf59e0b,
  temple: 0x06b6d4,
};

const BUILDING_SIZES: Record<BuildingType, number> = {
  village: 12,
  city: 18,
  castle: 22,
  temple: 20,
};

export class BuildingSprite extends Phaser.GameObjects.Container {
  private buildingType: BuildingType;
  private building: Phaser.GameObjects.Rectangle;
  private nameText: Phaser.GameObjects.Text;
  private glow: Phaser.GameObjects.Arc;

  constructor(
    scene: Phaser.Scene,
    x: number,
    y: number,
    type: BuildingType,
    name: string
  ) {
    super(scene, x, y);
    this.buildingType = type;

    const size = BUILDING_SIZES[type];
    const color = BUILDING_COLORS[type];

    this.glow = scene.add.circle(0, 0, size * 1.5, color, 0.15);
    this.add(this.glow);

    this.building = scene.add.rectangle(0, 0, size, size, color);
    this.building.setStrokeStyle(2, 0xffffff, 0.6);
    this.building.setAlpha(0.9);
    this.add(this.building);

    this.nameText = scene.add.text(0, size / 2 + 6, name, {
      fontSize: "11px",
      color: "#ffffff",
      align: "center",
      fontStyle: "600",
    });
    this.nameText.setOrigin(0.5);
    this.nameText.setStroke("#000000", 3);
    this.add(this.nameText);

    this.setSize(size, size + 20);

    this.setInteractive({ useHandCursor: true });

    scene.tweens.add({
      targets: this.glow,
      scale: { from: 1, to: 1.2 },
      alpha: { from: 0.15, to: 0.08 },
      duration: 2000,
      yoyo: true,
      repeat: -1,
      ease: "Sine.easeInOut",
    });
  }

  getType(): BuildingType {
    return this.buildingType;
  }
}
