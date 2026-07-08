import Phaser from "phaser";

export type EventEffectType = "fire" | "celebration" | "war" | "birth" | "death" | "discovery";

export interface EventEffectConfig {
  type: EventEffectType;
  x: number;
  y: number;
  duration?: number;
  intensity?: number;
}

interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  life: number;
  maxLife: number;
  size: number;
  color: number;
  alpha: number;
}

export class EventEffectManager {
  private scene: Phaser.Scene;
  private graphics: Phaser.GameObjects.Graphics;
  private particles: Particle[] = [];
  private activeEffects: Map<string, { config: EventEffectConfig; startTime: number }> = new Map();
  private maxParticles = 500;

  constructor(scene: Phaser.Scene) {
    this.scene = scene;
    this.graphics = scene.add.graphics();
    this.graphics.setDepth(100);
  }

  spawnEffect(config: EventEffectConfig): string {
    const id = `effect_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
    const duration = config.duration || 5000;
    const intensity = config.intensity || 1;

    this.activeEffects.set(id, { config, startTime: this.scene.time.now });

    const spawnParticles = () => {
      if (!this.activeEffects.has(id)) return;

      const count = Math.floor(5 * intensity);
      for (let i = 0; i < count; i++) {
        this.spawnParticle(config);
      }

      const elapsed = this.scene.time.now - this.activeEffects.get(id)!.startTime;
      if (elapsed < duration) {
        this.scene.time.delayedCall(100 + Math.random() * 100, spawnParticles);
      } else {
        this.activeEffects.delete(id);
      }
    };

    spawnParticles();

    return id;
  }

  private spawnParticle(config: EventEffectConfig): void {
    if (this.particles.length >= this.maxParticles) return;

    const { type, x, y } = config;

    let particle: Particle;

    switch (type) {
      case "fire":
        particle = this.createFireParticle(x, y);
        break;
      case "celebration":
        particle = this.createCelebrationParticle(x, y);
        break;
      case "war":
        particle = this.createWarParticle(x, y);
        break;
      case "birth":
        particle = this.createBirthParticle(x, y);
        break;
      case "death":
        particle = this.createDeathParticle(x, y);
        break;
      case "discovery":
        particle = this.createDiscoveryParticle(x, y);
        break;
      default:
        particle = this.createFireParticle(x, y);
    }

    this.particles.push(particle);
  }

  private createFireParticle(x: number, y: number): Particle {
    const colors = [0xff4500, 0xff6347, 0xffd700, 0xff8c00];
    return {
      x: x + (Math.random() - 0.5) * 20,
      y: y + (Math.random() - 0.5) * 10,
      vx: (Math.random() - 0.5) * 0.5,
      vy: -1 - Math.random() * 2,
      life: 1,
      maxLife: 800 + Math.random() * 400,
      size: 3 + Math.random() * 5,
      color: colors[Math.floor(Math.random() * colors.length)],
      alpha: 1,
    };
  }

  private createCelebrationParticle(x: number, y: number): Particle {
    const colors = [0xffd700, 0xff69b4, 0x00ffff, 0x7cfc00, 0xff6347];
    const angle = Math.random() * Math.PI * 2;
    const speed = 1 + Math.random() * 3;
    return {
      x,
      y,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed - 1,
      life: 1,
      maxLife: 1500 + Math.random() * 1000,
      size: 2 + Math.random() * 3,
      color: colors[Math.floor(Math.random() * colors.length)],
      alpha: 1,
    };
  }

  private createWarParticle(x: number, y: number): Particle {
    const colors = [0x8b0000, 0xff0000, 0x4a4a4a, 0x2f2f2f];
    const angle = Math.random() * Math.PI * 2;
    const speed = 2 + Math.random() * 4;
    return {
      x: x + (Math.random() - 0.5) * 30,
      y: y + (Math.random() - 0.5) * 20,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed - 2,
      life: 1,
      maxLife: 600 + Math.random() * 400,
      size: 2 + Math.random() * 4,
      color: colors[Math.floor(Math.random() * colors.length)],
      alpha: 1,
    };
  }

  private createBirthParticle(x: number, y: number): Particle {
    const colors = [0xffb6c1, 0x87ceeb, 0x98fb98, 0xfffacd];
    const angle = Math.random() * Math.PI * 2;
    const speed = 0.5 + Math.random() * 1.5;
    return {
      x,
      y,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed - 0.5,
      life: 1,
      maxLife: 2000 + Math.random() * 1000,
      size: 3 + Math.random() * 4,
      color: colors[Math.floor(Math.random() * colors.length)],
      alpha: 1,
    };
  }

  private createDeathParticle(x: number, y: number): Particle {
    const colors = [0x696969, 0x808080, 0x2f2f2f, 0x4b0082];
    return {
      x: x + (Math.random() - 0.5) * 15,
      y: y,
      vx: (Math.random() - 0.5) * 0.3,
      vy: -0.5 - Math.random() * 1,
      life: 1,
      maxLife: 1500 + Math.random() * 1000,
      size: 2 + Math.random() * 3,
      color: colors[Math.floor(Math.random() * colors.length)],
      alpha: 0.8,
    };
  }

  private createDiscoveryParticle(x: number, y: number): Particle {
    const colors = [0x00ffff, 0x7b68ee, 0x00ff7f, 0xffd700];
    return {
      x: x + (Math.random() - 0.5) * 10,
      y: y + (Math.random() - 0.5) * 10,
      vx: (Math.random() - 0.5) * 1,
      vy: -1 - Math.random() * 1.5,
      life: 1,
      maxLife: 1200 + Math.random() * 800,
      size: 2 + Math.random() * 3,
      color: colors[Math.floor(Math.random() * colors.length)],
      alpha: 1,
    };
  }

  update(delta: number): void {
    this.graphics.clear();

    for (let i = this.particles.length - 1; i >= 0; i--) {
      const p = this.particles[i];

      p.life -= delta / p.maxLife;
      p.alpha = p.life;

      p.x += p.vx * (delta / 16);
      p.y += p.vy * (delta / 16);

      p.vy += 0.02 * (delta / 16);

      if (p.life <= 0) {
        this.particles.splice(i, 1);
        continue;
      }

      this.graphics.fillStyle(p.color, p.alpha);
      this.graphics.fillCircle(p.x, p.y, p.size * p.life);
    }
  }

  clearAll(): void {
    this.particles = [];
    this.activeEffects.clear();
    this.graphics.clear();
  }

  getActiveEffectCount(): number {
    return this.activeEffects.size;
  }

  getParticleCount(): number {
    return this.particles.length;
  }

  destroy(): void {
    this.clearAll();
    this.graphics.destroy();
  }
}
