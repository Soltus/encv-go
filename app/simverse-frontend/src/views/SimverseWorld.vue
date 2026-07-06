<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button :text="t('common.back')" />
        </ion-buttons>
        <ion-title>🌍 世界</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="togglePause">
            <ion-icon :icon="isPaused ? playOutline : pauseOutline" slot="icon-only" />
          </ion-button>
          <ion-button @click="resetWorld">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="world-content" :fullscreen="true">
      <canvas
        ref="canvasRef"
        class="world-canvas"
        @mousedown="onPointerDown"
        @mousemove="onPointerMove"
        @mouseup="onPointerUp"
        @mouseleave="onPointerUp"
        @touchstart.prevent="onTouchStart"
        @touchmove.prevent="onTouchMove"
        @touchend.prevent="onPointerUp"
      ></canvas>

      <div class="hud-overlay">
        <ion-card class="hud-card">
          <ion-card-content>
            <div class="hud-row">
              <span class="hud-label">粒子</span>
              <span class="hud-value">{{ particleCount }}</span>
            </div>
            <div class="hud-row">
              <span class="hud-label">FPS</span>
              <span class="hud-value">{{ fpsDisplay }}</span>
            </div>
            <div class="hud-row">
              <span class="hud-label">重力</span>
              <span class="hud-value">{{ gravity.toFixed(1) }}</span>
            </div>
          </ion-card-content>
        </ion-card>

        <div class="hint-text">
          点击/拖拽添加粒子 · 双指/滚轮调整重力
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { playOutline, pauseOutline, refreshOutline } from "ionicons/icons";
import { useI18n } from "@/composables/useI18n";

const { t } = useI18n();

const canvasRef = ref<HTMLCanvasElement | null>(null);
const isPaused = ref(false);
const gravity = ref(0.5);
const particleCount = ref(0);
const fpsDisplay = ref(60);

interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  color: string;
  mass: number;
}

let ctx: CanvasRenderingContext2D | null = null;
let particles: Particle[] = [];
let animationId: number | null = null;
let lastTime = 0;
let fpsFrames = 0;
let fpsLastUpdate = 0;
let canvasWidth = 0;
let canvasHeight = 0;
let pointerDown = false;
let pointerX = 0;
let pointerY = 0;
let spawnTimer = 0;

const colors = [
  "#8b5cf6",
  "#06b6d4",
  "#f97316",
  "#22c55e",
  "#ef4444",
  "#ec4899",
  "#eab308",
  "#14b8a6",
];

function randomColor(): string {
  return colors[Math.floor(Math.random() * colors.length)];
}

function createParticle(x: number, y: number, vx = 0, vy = 0): Particle {
  const radius = 4 + Math.random() * 8;
  return {
    x,
    y,
    vx: vx + (Math.random() - 0.5) * 4,
    vy: vy + (Math.random() - 0.5) * 2,
    radius,
    color: randomColor(),
    mass: radius * radius,
  };
}

function initParticles(count: number) {
  particles = [];
  for (let i = 0; i < count; i++) {
    particles.push(createParticle(
      Math.random() * canvasWidth,
      Math.random() * canvasHeight * 0.5,
    ));
  }
  particleCount.value = particles.length;
}

function resizeCanvas() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  canvasWidth = rect.width;
  canvasHeight = rect.height;
  canvas.width = rect.width * dpr;
  canvas.height = rect.height * dpr;
  if (ctx) {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
}

function updatePhysics(dt: number) {
  const g = gravity.value;
  const friction = 0.995;
  const bounce = 0.7;

  for (const p of particles) {
    p.vy += g;
    p.vx *= friction;
    p.vy *= friction;
    p.x += p.vx * dt;
    p.y += p.vy * dt;

    if (p.x - p.radius < 0) {
      p.x = p.radius;
      p.vx = -p.vx * bounce;
    }
    if (p.x + p.radius > canvasWidth) {
      p.x = canvasWidth - p.radius;
      p.vx = -p.vx * bounce;
    }
    if (p.y - p.radius < 0) {
      p.y = p.radius;
      p.vy = -p.vy * bounce;
    }
    if (p.y + p.radius > canvasHeight) {
      p.y = canvasHeight - p.radius;
      p.vy = -p.vy * bounce;
      p.vx *= 0.98;
    }
  }

  for (let i = 0; i < particles.length; i++) {
    for (let j = i + 1; j < particles.length; j++) {
      const a = particles[i];
      const b = particles[j];
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      const minDist = a.radius + b.radius;

      if (dist < minDist && dist > 0) {
        const angle = Math.atan2(dy, dx);
        const overlap = minDist - dist;
        const totalMass = a.mass + b.mass;

        a.x -= Math.cos(angle) * overlap * (b.mass / totalMass);
        a.y -= Math.sin(angle) * overlap * (b.mass / totalMass);
        b.x += Math.cos(angle) * overlap * (a.mass / totalMass);
        b.y += Math.sin(angle) * overlap * (a.mass / totalMass);

        const dvx = b.vx - a.vx;
        const dvy = b.vy - a.vy;
        const dotProduct = dvx * Math.cos(angle) + dvy * Math.sin(angle);

        if (dotProduct < 0) {
          const impulse = (2 * dotProduct) / totalMass;
          a.vx += impulse * b.mass * Math.cos(angle) * bounce;
          a.vy += impulse * b.mass * Math.sin(angle) * bounce;
          b.vx -= impulse * a.mass * Math.cos(angle) * bounce;
          b.vy -= impulse * a.mass * Math.sin(angle) * bounce;
        }
      }
    }
  }
}

function render() {
  if (!ctx) return;

  ctx.fillStyle = "rgba(15, 15, 35, 0.3)";
  ctx.fillRect(0, 0, canvasWidth, canvasHeight);

  const gradient = ctx.createRadialGradient(
    canvasWidth / 2,
    canvasHeight / 3,
    0,
    canvasWidth / 2,
    canvasHeight / 3,
    canvasWidth * 0.6,
  );
  gradient.addColorStop(0, "rgba(139, 92, 246, 0.05)");
  gradient.addColorStop(1, "rgba(15, 15, 35, 0)");
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, canvasWidth, canvasHeight);

  for (const p of particles) {
    ctx.beginPath();
    ctx.arc(p.x, p.y, p.radius, 0, Math.PI * 2);

    const grad = ctx.createRadialGradient(
      p.x - p.radius * 0.3,
      p.y - p.radius * 0.3,
      0,
      p.x,
      p.y,
      p.radius,
    );
    grad.addColorStop(0, p.color);
    grad.addColorStop(1, shadeColor(p.color, -30));
    ctx.fillStyle = grad;

    ctx.shadowColor = p.color;
    ctx.shadowBlur = p.radius;
    ctx.fill();
    ctx.shadowBlur = 0;
  }
}

function shadeColor(color: string, percent: number): string {
  const num = parseInt(color.replace("#", ""), 16);
  const amt = Math.round(2.55 * percent);
  const R = Math.max(0, Math.min(255, (num >> 16) + amt));
  const G = Math.max(0, Math.min(255, ((num >> 8) & 0x00ff) + amt));
  const B = Math.max(0, Math.min(255, (num & 0x0000ff) + amt));
  return `#${(0x1000000 + R * 0x10000 + G * 0x100 + B).toString(16).slice(1)}`;
}

function loop(timestamp: number) {
  if (isPaused.value) {
    animationId = requestAnimationFrame(loop);
    return;
  }

  if (!lastTime) lastTime = timestamp;
  const rawDt = (timestamp - lastTime) / 16.67;
  const dt = Math.min(rawDt, 3);
  lastTime = timestamp;

  fpsFrames++;
  if (timestamp - fpsLastUpdate > 500) {
    fpsDisplay.value = Math.round(fpsFrames * 1000 / (timestamp - fpsLastUpdate));
    fpsFrames = 0;
    fpsLastUpdate = timestamp;
  }

  if (pointerDown && spawnTimer <= 0) {
    for (let i = 0; i < 2; i++) {
      if (particles.length < 500) {
        particles.push(createParticle(pointerX, pointerY));
      }
    }
    particleCount.value = particles.length;
    spawnTimer = 2;
  }
  spawnTimer -= dt;

  updatePhysics(dt);
  render();

  animationId = requestAnimationFrame(loop);
}

function onPointerDown(e: MouseEvent) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  pointerDown = true;
  pointerX = e.clientX - rect.left;
  pointerY = e.clientY - rect.top;
  spawnTimer = 0;
}

function onPointerMove(e: MouseEvent) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  pointerX = e.clientX - rect.left;
  pointerY = e.clientY - rect.top;
}

function onPointerUp() {
  pointerDown = false;
}

function onTouchStart(e: TouchEvent) {
  const canvas = canvasRef.value;
  if (!canvas || !e.touches[0]) return;
  const rect = canvas.getBoundingClientRect();
  pointerDown = true;
  pointerX = e.touches[0].clientX - rect.left;
  pointerY = e.touches[0].clientY - rect.top;
  spawnTimer = 0;
}

function onTouchMove(e: TouchEvent) {
  const canvas = canvasRef.value;
  if (!canvas || !e.touches[0]) return;
  const rect = canvas.getBoundingClientRect();
  pointerX = e.touches[0].clientX - rect.left;
  pointerY = e.touches[0].clientY - rect.top;
}

function togglePause() {
  isPaused.value = !isPaused.value;
}

function resetWorld() {
  initParticles(120);
}

onMounted(() => {
  const canvas = canvasRef.value;
  if (!canvas) return;
  ctx = canvas.getContext("2d");
  if (!ctx) return;

  resizeCanvas();
  initParticles(120);

  window.addEventListener("resize", resizeCanvas);

  lastTime = 0;
  fpsLastUpdate = performance.now();
  animationId = requestAnimationFrame(loop);
});

onUnmounted(() => {
  if (animationId !== null) {
    cancelAnimationFrame(animationId);
    animationId = null;
  }
  window.removeEventListener("resize", resizeCanvas);
  particles = [];
  ctx = null;
});
</script>

<style scoped>
.world-content {
  --background: #0f0f23;
  --padding-top: 0;
  --padding-bottom: 0;
  --offset-top: 0;
  --offset-bottom: 0;
}

.world-content :deep(.inner-scroll) {
  height: 100%;
  padding: 0 !important;
}

.world-canvas {
  width: 100%;
  height: 100%;
  min-height: 100%;
  display: block;
  touch-action: none;
  cursor: crosshair;
}

.hud-overlay {
  position: absolute;
  top: 16px;
  right: 16px;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
  pointer-events: none;
}

.hud-card {
  margin: 0;
  pointer-events: auto;
  --background: rgba(15, 15, 35, 0.85);
  backdrop-filter: blur(8px);
}

.hud-card ion-card-content {
  padding: 12px 16px;
}

.hud-row {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: 12px;
}

.hud-label {
  color: rgba(255, 255, 255, 0.5);
}

.hud-value {
  color: #8b5cf6;
  font-weight: 600;
}

.hint-text {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  text-align: right;
  max-width: 200px;
  line-height: 1.4;
}
</style>
