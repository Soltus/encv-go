import Phaser from "phaser";
import type { SimverseNPC } from "@/composables/useSimverse";
import { deriveBuildFromNPC, ARCH_META } from "./builds";

// 横屏世界的 NPC 头像：以 P14 流派系统驱动外观——
// 彩色流派圆底 + emoji 头像 + 白色描边 + 清爽名牌，替代原本统一的绿/灰小圆点。
export class NPCSprite extends Phaser.GameObjects.Container {
  private npcData: SimverseNPC;
  private avatar: Phaser.GameObjects.Container;
  private circle: Phaser.GameObjects.Arc;
  private emojiText: Phaser.GameObjects.Text;
  private nameText: Phaser.GameObjects.Text;
  private behaviorText: Phaser.GameObjects.Text | null = null;
  private behaviorCN: string = "";
  private isHovered = false;
  private moveTween: Phaser.Tweens.Tween | null = null;
  private trail: Phaser.GameObjects.Graphics | null = null;
  private trailPoints: { x: number; y: number; alpha: number }[] = [];
  private archColor = 0x22c55e;

  constructor(scene: Phaser.Scene, x: number, y: number, npc: SimverseNPC) {
    super(scene, x, y);
    this.npcData = npc;

    const build = deriveBuildFromNPC(npc);
    const meta = ARCH_META[build.primary];
    this.archColor = npc.is_alive ? meta.color : 0x6b7280;

    this.trail = scene.add.graphics();
    this.add(this.trail);

    // 头像主体（呼吸/悬停时整体缩放）
    this.avatar = scene.add.container(0, 0);

    const r = 8;
    this.circle = scene.add.circle(0, 0, r, this.archColor);
    this.circle.setStrokeStyle(2, 0xffffff, 0.85);
    this.avatar.add(this.circle);

    this.emojiText = scene.add.text(0, 0, npc.is_alive ? meta.emoji : "💀", {
      fontSize: "12px",
      color: "#ffffff",
    });
    this.emojiText.setOrigin(0.5);
    this.avatar.add(this.emojiText);

    this.add(this.avatar);

    // 名牌：深色半透明胶囊背景，替代刺眼的黑描边
    this.nameText = scene.add.text(0, 16, npc.name, {
      fontSize: "10px",
      color: "#ffffff",
      backgroundColor: "#000000aa",
      padding: { x: 3, y: 1 },
      align: "center",
      fontStyle: "500",
    });
    this.nameText.setOrigin(0.5);
    this.add(this.nameText);

    this.behaviorText = scene.add.text(0, -22, "", {
      fontSize: "9px",
      color: "#ffffff",
      backgroundColor: "#000000cc",
      padding: { x: 3, y: 1 },
      align: "center",
    });
    this.behaviorText.setOrigin(0.5);
    this.behaviorText.setVisible(false);
    this.add(this.behaviorText);

    this.setSize(20, 40);

    this.setInteractive({ useHandCursor: true });

    this.on("pointerover", () => {
      this.isHovered = true;
      this.scene.tweens.add({
        targets: this.avatar,
        scale: 1.35,
        duration: 150,
        ease: "Power2.out",
      });
    });

    this.on("pointerout", () => {
      this.isHovered = false;
      this.scene.tweens.add({
        targets: this.avatar,
        scale: 1,
        duration: 150,
        ease: "Power2.out",
      });
    });

    if (npc.is_alive) {
      scene.tweens.add({
        targets: this.avatar,
        scale: { from: 1, to: 1.12 },
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
    const build = deriveBuildFromNPC(npc);
    const meta = ARCH_META[build.primary];
    this.archColor = npc.is_alive ? meta.color : 0x6b7280;
    this.circle.setFillStyle(this.archColor);
    this.emojiText.setText(npc.is_alive ? meta.emoji : "💀");
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
    const color = this.archColor;

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
        this.avatar.setVisible(true);
        this.avatar.setScale(1);
        this.nameText.setVisible(true);
        if (this.trail) this.trail.setVisible(true);
        if (this.behaviorText) this.behaviorText.setVisible(!!this.behaviorCN);
        break;
      case "medium":
        this.avatar.setVisible(true);
        this.avatar.setScale(1);
        this.nameText.setVisible(false);
        if (this.trail) this.trail.setVisible(true);
        if (this.behaviorText) this.behaviorText.setVisible(false);
        break;
      case "low":
        this.avatar.setVisible(true);
        this.avatar.setScale(0.6);
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
