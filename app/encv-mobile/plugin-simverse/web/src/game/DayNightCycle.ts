import Phaser from "phaser";

export type TimeOfDay = "dawn" | "day" | "dusk" | "night";

interface DayPhaseConfig {
  name: TimeOfDay;
  tintColor: number;
  tintAlpha: number;
  ambientLight: number;
}

// 昼夜配色：与 encv-mobile 紫色主题协调。
// dawn 紫粉，day 暖黄白带极淡紫调，dusk 橙紫过渡，night 深紫蓝。
// 各相位的 tintAlpha 经 BlendModes.MULTIPLY 叠加到场景，过渡时颜色在相邻相位间 lerp。
const DAY_PHASES: DayPhaseConfig[] = [
  { name: "dawn", tintColor: 0x9f7aea, tintAlpha: 0.25, ambientLight: 0.7 }, // 紫粉
  { name: "day", tintColor: 0xfff8dc, tintAlpha: 0.05, ambientLight: 1 }, // 暖黄白（轻微紫调）
  { name: "dusk", tintColor: 0xff7f50, tintAlpha: 0.3, ambientLight: 0.75 }, // 橙紫过渡
  { name: "night", tintColor: 0x1a1a3e, tintAlpha: 0.5, ambientLight: 0.35 }, // 深紫蓝
];

export class DayNightCycle {
  private scene: Phaser.Scene;
  private overlay: Phaser.GameObjects.Rectangle;
  private stars: Phaser.GameObjects.Graphics;
  private starPositions: { x: number; y: number; size: number; twinkleOffset: number }[] = [];
  private timeOfDay: TimeOfDay = "day";
  private cycleSpeed = 1;
  private cycleTime = 0.25;
  private cycleDuration = 120000;
  private enabled = true;
  private starsEnabled = true;

  constructor(scene: Phaser.Scene, worldWidth: number, worldHeight: number) {
    this.scene = scene;

    this.overlay = scene.add.rectangle(worldWidth / 2, worldHeight / 2, worldWidth, worldHeight, 0xffffff, 0);
    this.overlay.setDepth(50);
    this.overlay.setBlendMode(Phaser.BlendModes.MULTIPLY);

    this.stars = scene.add.graphics();
    this.stars.setDepth(49);
    this.generateStars(worldWidth, worldHeight);

    this.updateTint();
  }

  private generateStars(worldWidth: number, worldHeight: number): void {
    const starCount = 300;
    for (let i = 0; i < starCount; i++) {
      this.starPositions.push({
        x: Math.random() * worldWidth,
        y: Math.random() * worldHeight * 0.6,
        size: 0.5 + Math.random() * 1.5,
        twinkleOffset: Math.random() * Math.PI * 2,
      });
    }
  }

  update(delta: number): void {
    if (!this.enabled) return;

    this.cycleTime += (delta / this.cycleDuration) * this.cycleSpeed;
    if (this.cycleTime >= 1) {
      this.cycleTime -= 1;
    }

    this.updateTint();
    this.updateStars(delta);
  }

  private updateTint(): void {
    const phaseIndex = Math.floor(this.cycleTime * DAY_PHASES.length) % DAY_PHASES.length;
    const nextPhaseIndex = (phaseIndex + 1) % DAY_PHASES.length;
    const phaseProgress = (this.cycleTime * DAY_PHASES.length) % 1;

    const current = DAY_PHASES[phaseIndex];
    const next = DAY_PHASES[nextPhaseIndex];

    this.timeOfDay = current.name;

    const tintR = this.lerpColor((current.tintColor >> 16) & 255, (next.tintColor >> 16) & 255, phaseProgress);
    const tintG = this.lerpColor((current.tintColor >> 8) & 255, (next.tintColor >> 8) & 255, phaseProgress);
    const tintB = this.lerpColor(current.tintColor & 255, next.tintColor & 255, phaseProgress);
    const tintColor = (tintR << 16) | (tintG << 8) | tintB;
    const tintAlpha = Phaser.Math.Linear(current.tintAlpha, next.tintAlpha, phaseProgress);

    this.overlay.setFillStyle(tintColor, tintAlpha);

    if (this.starsEnabled) {
      const nightFactor = this.getNightFactor();
      this.stars.setAlpha(nightFactor);
    }
  }

  private updateStars(delta: number): void {
    if (!this.starsEnabled) return;

    this.stars.clear();
    const nightFactor = this.getNightFactor();

    if (nightFactor <= 0) return;

    const time = this.scene.time.now / 1000;

    this.starPositions.forEach(star => {
      const twinkle = 0.5 + 0.5 * Math.sin(time * 2 + star.twinkleOffset);
      const alpha = nightFactor * twinkle;

      this.stars.fillStyle(0xffffff, alpha);
      this.stars.fillCircle(star.x, star.y, star.size);
    });
  }

  private getNightFactor(): number {
    if (this.timeOfDay === "night") {
      const phaseProgress = (this.cycleTime * DAY_PHASES.length) % 1;
      return 0.6 + 0.4 * Math.sin(phaseProgress * Math.PI);
    }
    if (this.timeOfDay === "dusk") {
      const phaseProgress = (this.cycleTime * DAY_PHASES.length) % 1;
      return phaseProgress * 0.6;
    }
    if (this.timeOfDay === "dawn") {
      const phaseProgress = (this.cycleTime * DAY_PHASES.length) % 1;
      return (1 - phaseProgress) * 0.6;
    }
    return 0;
  }

  private lerpColor(a: number, b: number, t: number): number {
    return Math.floor(Phaser.Math.Linear(a, b, t));
  }

  getTimeOfDay(): TimeOfDay {
    return this.timeOfDay;
  }

  getCycleTime(): number {
    return this.cycleTime;
  }

  setCycleSpeed(speed: number): void {
    this.cycleSpeed = speed;
  }

  setCycleDuration(durationMs: number): void {
    this.cycleDuration = durationMs;
  }

  setTimeOfDay(time: TimeOfDay): void {
    const index = DAY_PHASES.findIndex(p => p.name === time);
    if (index >= 0) {
      this.cycleTime = index / DAY_PHASES.length;
      this.updateTint();
    }
  }

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    if (!enabled) {
      this.overlay.setAlpha(0);
      this.stars.setAlpha(0);
    }
  }

  setStarsEnabled(enabled: boolean): void {
    this.starsEnabled = enabled;
    if (!enabled) {
      this.stars.setAlpha(0);
    }
  }

  toggle(): boolean {
    this.enabled = !this.enabled;
    if (!this.enabled) {
      this.overlay.setAlpha(0);
      this.stars.setAlpha(0);
    }
    return this.enabled;
  }

  destroy(): void {
    this.overlay.destroy();
    this.stars.destroy();
  }
}
