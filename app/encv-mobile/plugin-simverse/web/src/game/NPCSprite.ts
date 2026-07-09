import Phaser from "phaser";
import type { SimverseNPC } from "@/composables/useSimverse";

export class NPCSprite extends Phaser.GameObjects.Container {
  private npcData: SimverseNPC;
  private circle: Phaser.GameObjects.Arc;
  private nameText: Phaser.GameObjects.Text;
  private behaviorText: Phaser.GameObjects.Text | null = null;
  private behaviorCN: string = "";
  private isHovered = false;
  private moveTween: Phaser.Tweens.Tween | null = null;
  private trail: Phaser.GameObjects.Graphics | null = null;
  private trailPoints: { x: number; y: number; alpha: number }[] = [];

  constructor(scene: Phaser.Scene, x: number, y: number, npc: SimverseNPC) {
    super(scene, x, y);
    this.npcData = npc;

    const dotColor = npc.is_alive ? 0x22c55e : 0x6b7280;

    this.trail = scene.add.graphics();
    this.add(this.trail);

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

    this.behaviorText = scene.add.text(0, -24, "", {
      fontSize: "9px",
      color: "#ffffff",
      backgroundColor: "#00000099",
      padding: { x: 3, y: 1 },
      align: "center",
    });
    this.behaviorText.setOrigin(0.5);
    this.behaviorText.setVisible(false);
    this.add(this.behaviorText);

    this.setSize(12, 28);

    this.setInteractive({ useHandCursor: true });

    this.on("pointerover", () => {
      this.isHovered = true;
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

  getBehaviorCN(): string {
    return this.behaviorCN;
  }

  updateNPC(npc: SimverseNPC): void {
    this.npcData = npc;
    const dotColor = npc.is_alive ? 0x22c55e : 0x6b7280;
    this.circle.setFillStyle(dotColor);
    this.nameText.setText(npc.name);
  }

  setBehavior(behaviorCN: string): void {
    this.behaviorCN = behaviorCN;
    if (this.behaviorText) {
      if (behaviorCN) {
        this.behaviorText.setText(behaviorCN);
        this.behaviorText.setVisible(true);
      } else {
        this.behaviorText.setVisible(false);
      }
    }
  }

  moveNPCTo(targetX: number, targetY: number, duration = 3000): void {
    if (this.moveTween) {
      this.moveTween.stop();
    }

    const startX = this.x;
    const startY = this.y;
    const dist = Phaser.Math.Distance.Between(startX, startY, targetX, targetY);
    const actualDuration = (dist / 100) * duration;

    this.trailPoints = [];
    const steps = 10;
    for (let i = 0; i < steps; i++) {
      const t = i / steps;
      const cp1x = startX + (targetX - startX) * 0.3 + (Math.random() - 0.5) * 30;
      const cp1y = startY + (targetY - startY) * 0.3 + (Math.random() - 0.5) * 30;
      const cp2x = startX + (targetX - startX) * 0.7 + (Math.random() - 0.5) * 30;
      const cp2y = startY + (targetY - startY) * 0.7 + (Math.random() - 0.5) * 30;

      const it = 1 - t;
      const x = it * it * it * startX + 3 * it * it * t * cp1x + 3 * it * t * t * cp2x + t * t * t * targetX;
      const y = it * it * it * startY + 3 * it * it * t * cp1y + 3 * it * t * t * cp2y + t * t * t * targetY;

      this.trailPoints.push({ x, y, alpha: 0 });
    }

    this.moveTween = this.scene.tweens.add({
      targets: this,
      x: targetX,
      y: targetY,
      duration: actualDuration,
      ease: "Sine.easeInOut",
      onUpdate: () => {
        this.updateTrail();
      },
      onComplete: () => {
        this.moveTween = null;
        this.fadeTrail();
      },
    });
  }

  private updateTrail(): void {
    if (!this.trail || this.trailPoints.length < 2) return;

    for (let i = this.trailPoints.length - 1; i > 0; i--) {
      this.trailPoints[i].x = this.trailPoints[i - 1].x;
      this.trailPoints[i].y = this.trailPoints[i - 1].y;
      this.trailPoints[i].alpha = Math.min(1, this.trailPoints[i - 1].alpha + 0.1);
    }
    this.trailPoints[0].x = this.x;
    this.trailPoints[0].y = this.y;
    this.trailPoints[0].alpha = 0.6;

    this.drawTrail();
  }

  private fadeTrail(): void {
    this.scene.tweens.addCounter({
      from: 1,
      to: 0,
      duration: 500,
      onUpdate: (counter) => {
        const v = counter.getValue() as number;
        for (const p of this.trailPoints) {
          p.alpha = p.alpha * v;
        }
        this.drawTrail();
      },
    });
  }

  private drawTrail(): void {
    if (!this.trail) return;

    this.trail.clear();
    const color = this.npcData.is_alive ? 0x22c55e : 0x6b7280;

    for (let i = 0; i < this.trailPoints.length - 1; i++) {
      const p1 = this.trailPoints[i];
      const p2 = this.trailPoints[i + 1];
      const alpha = (p1.alpha + p2.alpha) / 2 * 0.4;
      const thickness = 2 * (1 - i / this.trailPoints.length);

      if (alpha <= 0) continue;

      this.trail.lineStyle(thickness, color, alpha);
      this.trail.beginPath();
      this.trail.moveTo(p1.x - this.x, p1.y - this.y);
      this.trail.lineTo(p2.x - this.x, p2.y - this.y);
      this.trail.strokePath();
    }
  }

  setLODLevel(level: "full" | "medium" | "low"): void {
    switch (level) {
      case "full":
        this.circle.setVisible(true);
        this.nameText.setVisible(true);
        this.nameText.setFontSize("10px");
        if (this.trail) this.trail.setVisible(true);
        if (this.behaviorText) this.behaviorText.setVisible(!!this.behaviorCN);
        break;
      case "medium":
        this.circle.setVisible(true);
        this.nameText.setVisible(false);
        if (this.trail) this.trail.setVisible(true);
        if (this.behaviorText) this.behaviorText.setVisible(false);
        break;
      case "low":
        this.circle.setScale(0.6);
        this.nameText.setVisible(false);
        if (this.trail) this.trail.setVisible(false);
        if (this.behaviorText) this.behaviorText.setVisible(false);
        break;
    }
  }

  stopMoving(): void {
    if (this.moveTween) {
      this.moveTween.stop();
      this.moveTween = null;
    }
  }

  isMoving(): boolean {
    return this.moveTween !== null;
  }
}
