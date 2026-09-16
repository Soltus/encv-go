import type Phaser from "phaser";

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

// 主题色（与 encv-mobile 紫色一致）+ 事件严重度色板：
// default 紫色 / warning 橙 / critical 红，与 NPCSprite 的行为色板相协调。
const THEME_COLOR_DEFAULT = 0x8b5cf6;
const THEME_COLOR_WARNING = 0xf59e0b;
const THEME_COLOR_CRITICAL = 0xef4444;

// 重要事件（war/fire）触发额外的扩散光环，强化"重大事件"的视觉冲击。
const MAJOR_EVENT_TYPES: EventEffectType[] = ["war", "fire"];

interface EffectRing {
  x: number;
  y: number;
  radius: number;
  maxRadius: number;
  color: number;
  alpha: number;
  life: number;
}

/**
 * 把事件类型映射到严重度颜色：
 * - imp-4 / critical（war、fire）→ 红
 * - imp-3 / warning（death）→ 橙
 * - default（celebration、birth、discovery）→ 紫
 */
function getEventImportanceColor(type: EventEffectType): number {
  switch (type) {
    case "war":
    case "fire":
      return THEME_COLOR_CRITICAL;
    case "death":
      return THEME_COLOR_WARNING;
    case "celebration":
    case "birth":
    case "discovery":
    default:
      return THEME_COLOR_DEFAULT;
  }
}

export class EventEffectManager {
  private scene: Phaser.Scene;
  private graphics: Phaser.GameObjects.Graphics;
  private particles: Particle[] = [];
  private rings: EffectRing[] = [];
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

    // 重要事件首次触发时生成一圈扩散光环，作为"重大事件"的额外视觉信号
    if (MAJOR_EVENT_TYPES.includes(config.type)) {
      this.spawnEffectRing(config.x, config.y, getEventImportanceColor(config.type), intensity);
    }

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

  private spawnEffectRing(x: number, y: number, color: number, intensity: number): void {
    this.rings.push({
      x,
      y,
      radius: 5,
      maxRadius: 40 + 30 * intensity,
      color,
      alpha: 0.65,
      life: 1,
    });
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
    // fire = critical：保留暖色火焰质感 + 主题红
    const colors = [0xff4500, 0xff6347, 0xffd700, 0xff8c00, THEME_COLOR_CRITICAL];
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
    // celebration = default：保留多彩喜庆感 + 主题紫
    const colors = [0xffd700, 0xff69b4, 0x00ffff, 0x7cfc00, THEME_COLOR_DEFAULT];
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
    // war = critical：主题红 + 主题紫做对比 + 暗灰做烟雾
    const colors = [THEME_COLOR_CRITICAL, 0x8b0000, THEME_COLOR_DEFAULT, 0x2f2f2f];
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
    // birth = default：柔粉/淡蓝/淡绿 + 主题紫
    const colors = [0xffb6c1, 0x87ceeb, 0x98fb98, THEME_COLOR_DEFAULT];
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
    // death = warning：主题橙 + 灰 + 暗紫
    const colors = [0x696969, 0x808080, THEME_COLOR_WARNING, THEME_COLOR_DEFAULT];
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
    // discovery = default：青/紫/绿/金 + 主题紫
    const colors = [0x00ffff, THEME_COLOR_DEFAULT, 0x00ff7f, 0xffd700];
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

    // 扩散光环：从中心向外扩张 + 衰减，叠加 ADD 混合模式增强"冲击波"观感
    for (let i = this.rings.length - 1; i >= 0; i--) {
      const ring = this.rings[i];
      const progress = 1 - ring.life;
      ring.radius = 5 + (ring.maxRadius - 5) * progress;
      ring.life -= delta / 900;
      ring.alpha = Math.max(0, ring.life * 0.65);

      if (ring.life <= 0) {
        this.rings.splice(i, 1);
        continue;
      }

      this.graphics.lineStyle(2.5, ring.color, ring.alpha);
      this.graphics.strokeCircle(ring.x, ring.y, ring.radius);
      // 内圈淡填充强化"光环"质感
      this.graphics.fillStyle(ring.color, ring.alpha * 0.18);
      this.graphics.fillCircle(ring.x, ring.y, ring.radius);
    }

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
    this.rings = [];
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
