import Phaser from "phaser";
import type { SimverseNPC } from "@/composables/useSimverse";

export class NPCSprite extends Phaser.GameObjects.Container {
  private npcData: SimverseNPC;
  private circle: Phaser.GameObjects.Arc;
  private nameText: Phaser.GameObjects.Text;
  private isHovered = false;

  constructor(scene: Phaser.Scene, x: number, y: number, npc: SimverseNPC) {
    super(scene, x, y);
    this.npcData = npc;

    const dotColor = npc.is_alive ? 0x22c55e : 0x6b7280;

    this.circle = scene.add.circle(0, 0, 6, dotColor);
    this.circle.setStrokeStyle(2, 0xffffff, 0.5);
    this.add(this.circle);

    this.nameText = scene.add.text(0, 12, npc.name, {
      fontSize: "10px",
      color: "#ffffff",
      align: "center",
      fontStyle: "500",
    });
    this.nameText.setOrigin(0.5);
    this.nameText.setStroke("#000000", 3);
    this.add(this.nameText);

    this.setSize(12, 28);

    this.setInteractive({ useHandCursor: true });

    this.on("pointerover", () => {
      this.isHovered = true;
      this.circle.setScale(1.4);
      this.scene.tweens.add({
        targets: this.circle,
        scale: 1.3,
        duration: 150,
        ease: "Power2.out",
      });
    });

    this.on("pointerout", () => {
      this.isHovered = false;
      this.scene.tweens.add({
        targets: this.circle,
        scale: 1,
        duration: 150,
        ease: "Power2.out",
      });
    });

    if (npc.is_alive) {
      scene.tweens.add({
        targets: this.circle,
        scale: { from: 1, to: 1.15 },
        duration: 1000,
        yoyo: true,
        repeat: -1,
        ease: "Sine.easeInOut",
      });
    }
  }

  getNPCData(): SimverseNPC {
    return this.npcData;
  }

  updateNPC(npc: SimverseNPC): void {
    this.npcData = npc;
    const dotColor = npc.is_alive ? 0x22c55e : 0x6b7280;
    this.circle.setFillStyle(dotColor);
    this.nameText.setText(npc.name);
  }

  setLODLevel(level: "full" | "medium" | "low"): void {
    switch (level) {
      case "full":
        this.circle.setVisible(true);
        this.nameText.setVisible(true);
        this.nameText.setFontSize("10px");
        break;
      case "medium":
        this.circle.setVisible(true);
        this.nameText.setVisible(false);
        break;
      case "low":
        this.circle.setScale(0.6);
        this.nameText.setVisible(false);
        break;
    }
  }
}
