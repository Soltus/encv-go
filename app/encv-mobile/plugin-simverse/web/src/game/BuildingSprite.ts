import Phaser from "phaser";

export type BuildingType = "village" | "city" | "castle" | "temple";

// 建筑配色：与 encv-mobile 紫色主题协调。
// village/city 走紫色梯度，castle/temple 保留强对比色以维持辨识度。
const BUILDING_COLORS: Record<BuildingType, number> = {
  village: 0x9f7aea, // 软紫（settlement size=1）
  city: 0x8b5cf6, // 亮紫（settlement size>=3，主题 primary）
  castle: 0xf59e0b, // 金色（重要建筑）
  temple: 0x06b6d4, // 青色（与紫互补）
};

const BUILDING_SIZES: Record<BuildingType, number> = {
  village: 12,
  city: 18,
  castle: 22,
  temple: 20,
};

// 重要建筑（city/castle）的辉光更强，普通建筑（village/temple）保持柔和。
const BUILDING_GLOW_ALPHA: Record<BuildingType, number> = {
  village: 0.12,
  city: 0.22,
  castle: 0.25,
  temple: 0.14,
};

export class BuildingSprite extends Phaser.GameObjects.Container {
  private buildingType: BuildingType;
  private building: Phaser.GameObjects.Rectangle;
  private nameText: Phaser.GameObjects.Text;
  private glow: Phaser.GameObjects.Arc;

  constructor(scene: Phaser.Scene, x: number, y: number, type: BuildingType, name: string) {
    super(scene, x, y);
    this.buildingType = type;

    const size = BUILDING_SIZES[type];
    const color = BUILDING_COLORS[type];
    const glowAlpha = BUILDING_GLOW_ALPHA[type];

    this.glow = scene.add.circle(0, 0, size * 1.5, color, glowAlpha);
    this.glow.setBlendMode(Phaser.BlendModes.ADD);
    this.add(this.glow);

    this.building = scene.add.rectangle(0, 0, size, size, color);
    // 重要建筑（city/castle）用紫色描边强调，普通建筑用白色细描边
    const isImportant = type === "city" || type === "castle";
    this.building.setStrokeStyle(isImportant ? 2.5 : 2, isImportant ? 0x8b5cf6 : 0xffffff, isImportant ? 0.9 : 0.6);
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
      alpha: { from: glowAlpha, to: glowAlpha * 0.55 },
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
