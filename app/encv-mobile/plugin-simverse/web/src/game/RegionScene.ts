import Phaser from "phaser";
import { phaserEventBus, PHASER_EVENTS } from "./PhaserEventBus";

export interface RegionSceneData {
  regionId: string;
  regionName: string;
  regionType: string;
  backTo: string;
}

export class RegionScene extends Phaser.Scene {
  private regionData!: RegionSceneData;
  private buildings: Phaser.GameObjects.Container[] = [];
  private npcs: Phaser.GameObjects.Container[] = [];
  private isDragging = false;
  private lastPointerX = 0;
  private lastPointerY = 0;

  constructor() {
    super("RegionScene");
  }

  init(data: RegionSceneData): void {
    this.regionData = data;
  }

  create(): void {
    this.cameras.main.setBackgroundColor("#1a1a2e");
    this.setupInput();
    this.createRegionBackground();
    this.createBuildings();
    this.createNPCs();
    this.createBackButton();
    this.createRegionTitle();

    phaserEventBus.emit(PHASER_EVENTS.REGION_READY, this.regionData);
  }

  private createRegionBackground(): void {
    const { width, height } = this.scale;

    const bg = this.add.graphics();
    const gradientColors = this.getRegionColors();

    for (let y = 0; y < height; y += 2) {
      const t = y / height;
      const r = Phaser.Math.Linear(gradientColors.top[0], gradientColors.bottom[0], t);
      const g = Phaser.Math.Linear(gradientColors.top[1], gradientColors.bottom[1], t);
      const b = Phaser.Math.Linear(gradientColors.top[2], gradientColors.bottom[2], t);
      bg.fillStyle(Phaser.Display.Color.GetColor(r, g, b), 1);
      bg.fillRect(0, y, width, 2);
    }

    const ground = this.add.graphics();
    ground.fillStyle(0x2d3436, 1);
    ground.fillRect(0, height * 0.6, width, height * 0.4);

    const groundGradient = this.add.graphics();
    for (let y = height * 0.6; y < height; y += 2) {
      const t = (y - height * 0.6) / (height * 0.4);
      const alpha = 0.3 - t * 0.2;
      groundGradient.fillStyle(0x000000, alpha);
      groundGradient.fillRect(0, y, width, 2);
    }
  }

  private getRegionColors(): { top: [number, number, number]; bottom: [number, number, number] } {
    const type = this.regionData.regionType;
    const colors: Record<string, { top: [number, number, number]; bottom: [number, number, number] }> = {
      town: { top: [30, 30, 60], bottom: [20, 20, 40] },
      forest: { top: [20, 50, 30], bottom: [10, 30, 20] },
      mountain: { top: [50, 50, 60], bottom: [30, 30, 40] },
      dungeon: { top: [30, 20, 20], bottom: [15, 10, 10] },
      plains: { top: [40, 50, 30], bottom: [25, 35, 20] },
    };
    return colors[type] || colors.town;
  }

  private createBuildings(): void {
    const { width, height } = this.scale;
    const buildingCount = 5 + Math.floor(Math.random() * 4);
    const buildingTypes = ["🏠", "🏛️", "🏪", "⛩️", "🏰", "🗼"];

    for (let i = 0; i < buildingCount; i++) {
      const x = Phaser.Math.Between(80, width - 80);
      const y = Phaser.Math.Between(height * 0.4, height * 0.75);
      const type = buildingTypes[Math.floor(Math.random() * buildingTypes.length)];
      const scale = Phaser.Math.FloatBetween(1.5, 2.5);

      const container = this.add.container(x, y);

      const shadow = this.add.circle(0, 20, 30 * scale, 0x000000, 0.3);
      shadow.setScale(1, 0.3);

      const buildingText = this.add.text(0, 0, type, {
        fontSize: `${40 * scale}px`,
      }).setOrigin(0.5);

      const buildingName = this.generateBuildingName(i);
      const nameText = this.add.text(0, 50 * scale, buildingName, {
        fontSize: "14px",
        color: "#ffffff",
        backgroundColor: "rgba(0,0,0,0.6)",
        padding: { x: 8, y: 4 },
      }).setOrigin(0.5);

      container.add([shadow, buildingText, nameText]);
      container.setSize(80, 100);
      container.setInteractive({ useHandCursor: true });

      container.on("pointerdown", () => {
        this.tweenBuilding(container, buildingName);
      });

      this.buildings.push(container);
    }
  }

  private generateBuildingName(index: number): string {
    const names = ["酒馆", "武器店", "防具店", "道具店", "公会", "神殿", "旅馆", "商店", "民宅", "城堡"];
    return names[index % names.length];
  }

  private tweenBuilding(container: Phaser.GameObjects.Container, name: string): void {
    this.tweens.add({
      targets: container,
      scale: { from: 1, to: 1.1 },
      duration: 150,
      yoyo: true,
      onComplete: () => {
        phaserEventBus.emit(PHASER_EVENTS.BUILDING_CLICKED, { name });
      },
    });
  }

  private createNPCs(): void {
    const { width, height } = this.scale;
    const npcCount = 3 + Math.floor(Math.random() * 5);
    const npcEmojis = ["🧙", "🧝", "🧚", "👨‍🌾", "👩‍🍳", "🗡️", "🛡️", "🧑‍🎤"];

    for (let i = 0; i < npcCount; i++) {
      const x = Phaser.Math.Between(60, width - 60);
      const y = Phaser.Math.Between(height * 0.55, height * 0.85);
      const emoji = npcEmojis[Math.floor(Math.random() * npcEmojis.length)];

      const container = this.add.container(x, y);

      const shadow = this.add.circle(0, 15, 15, 0x000000, 0.3);
      shadow.setScale(1, 0.3);

      const npcText = this.add.text(0, 0, emoji, {
        fontSize: "32px",
      }).setOrigin(0.5);

      const nameText = this.add.text(0, 25, `NPC${i + 1}`, {
        fontSize: "12px",
        color: "#ffffff",
        backgroundColor: "rgba(0,0,0,0.5)",
        padding: { x: 6, y: 2 },
      }).setOrigin(0.5);

      container.add([shadow, npcText, nameText]);
      container.setSize(50, 60);
      container.setInteractive({ useHandCursor: true });

      container.on("pointerdown", () => {
        this.tweenNPC(container, i);
      });

      this.npcs.push(container);

      this.startNPCWander(container);
    }
  }

  private startNPCWander(container: Phaser.GameObjects.Container): void {
    const { width, height } = this.scale;
    const wander = () => {
      const targetX = Phaser.Math.Between(60, width - 60);
      const targetY = Phaser.Math.Between(height * 0.55, height * 0.85);
      const duration = Phaser.Math.Between(2000, 5000);

      this.tweens.add({
        targets: container,
        x: targetX,
        y: targetY,
        duration: duration,
        onComplete: () => {
          this.time.delayedCall(Phaser.Math.Between(1000, 3000), wander);
        },
      });
    };

    this.time.delayedCall(Phaser.Math.Between(500, 2000), wander);
  }

  private tweenNPC(container: Phaser.GameObjects.Container, index: number): void {
    this.tweens.add({
      targets: container,
      scale: { from: 1, to: 1.2 },
      duration: 150,
      yoyo: true,
      onComplete: () => {
        phaserEventBus.emit(PHASER_EVENTS.NPC_CLICKED, { id: `npc_${index}`, name: `NPC${index + 1}` });
      },
    });
  }

  private createBackButton(): void {
    const btn = this.add.text(30, 30, "← 返回", {
      fontSize: "18px",
      color: "#ffffff",
      backgroundColor: "rgba(0,0,0,0.6)",
      padding: { x: 16, y: 8 },
    }).setInteractive({ useHandCursor: true });

    btn.setScrollFactor(0);

    btn.on("pointerdown", () => {
      this.scene.start("WorldScene");
      phaserEventBus.emit(PHASER_EVENTS.BACK_TO_WORLD);
    });
  }

  private createRegionTitle(): void {
    const { width } = this.scale;
    const title = this.add.text(width / 2, 30, this.regionData.regionName, {
      fontSize: "24px",
      color: "#ffffff",
      fontStyle: "bold",
    }).setOrigin(0.5, 0);

    title.setScrollFactor(0);

    const typeText = this.add.text(width / 2, 65, this.getRegionTypeName(), {
      fontSize: "14px",
      color: "#aaaaaa",
    }).setOrigin(0.5, 0);

    typeText.setScrollFactor(0);
  }

  private getRegionTypeName(): string {
    const types: Record<string, string> = {
      town: "城镇",
      forest: "森林",
      mountain: "山脉",
      dungeon: "地牢",
      plains: "平原",
    };
    return types[this.regionData.regionType] || "未知区域";
  }

  private setupInput(): void {
    this.input.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
      if (pointer.button === 0) {
        this.isDragging = true;
        this.lastPointerX = pointer.x;
        this.lastPointerY = pointer.y;
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

    this.input.on("pointerup", () => {
      this.isDragging = false;
    });

    this.input.on("pointerupoutside", () => {
      this.isDragging = false;
    });
  }
}
