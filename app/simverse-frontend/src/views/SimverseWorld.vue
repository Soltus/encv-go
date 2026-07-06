<template>
  <ion-page class="world-page">
    <ion-content :fullscreen="true" class="world-content">
      <div class="game-container">
        <canvas ref="canvasRef" class="game-canvas"></canvas>

        <div class="top-bar">
          <div class="resource-group">
            <div class="resource-item">
              <span class="resource-icon">⏱️</span>
              <span class="resource-label">{{ t("simverse.tick") }}</span>
              <span class="resource-value">{{ worldState?.tick ?? 0 }}</span>
            </div>
            <div class="resource-item">
              <span class="resource-icon">👥</span>
              <span class="resource-label">{{ t("simverse.population") }}</span>
              <span class="resource-value">{{ worldState?.npcCount ?? 0 }}</span>
            </div>
            <div class="resource-item">
              <span class="resource-icon">🌍</span>
              <span class="resource-label">{{ t("simverse.era") }}</span>
              <span class="resource-value">{{ eraName }}</span>
            </div>
          </div>
          <div class="top-actions">
            <button class="icon-btn" @click="togglePause">
              {{ isPaused ? "▶" : "⏸" }}
            </button>
            <button class="icon-btn" @click="exitWorld">
              ✕
            </button>
          </div>
        </div>

        <div class="event-panel" v-if="recentEvents.length > 0">
          <div class="event-header">📜 {{ t("simverse.recentEvents") }}</div>
          <div class="event-list">
            <div v-for="ev in recentEvents" :key="ev.id" class="event-item">
              <span class="event-dot"></span>
              <span class="event-text">{{ ev.title }}</span>
            </div>
          </div>
        </div>

        <div class="bottom-menu">
          <button class="menu-item" @click="openPanel('npc')">
            <div class="menu-icon">👤</div>
            <div class="menu-label">{{ t("simverse.npc") }}</div>
          </button>
          <button class="menu-item" @click="openPanel('org')">
            <div class="menu-icon">🏰</div>
            <div class="menu-label">{{ t("simverse.org") }}</div>
          </button>
          <button class="menu-item gacha" @click="openPanel('gacha')">
            <div class="menu-icon sparkle">✨</div>
            <div class="menu-label">{{ t("simverse.gacha") }}</div>
          </button>
          <button class="menu-item" @click="openPanel('chronicles')">
            <div class="menu-icon">📖</div>
            <div class="menu-label">{{ t("simverse.chronicles") }}</div>
          </button>
          <button class="menu-item" @click="openPanel('settings')">
            <div class="menu-icon">⚙️</div>
            <div class="menu-label">{{ t("simverse.settings") }}</div>
          </button>
        </div>

        <div v-if="activePanel" class="side-panel" :class="{ open: !!activePanel }">
          <div class="panel-header">
            <span class="panel-title">{{ getPanelTitle(activePanel) }}</span>
            <button class="close-btn" @click="activePanel = null">✕</button>
          </div>
          <div class="panel-content">
            <template v-if="activePanel === 'npc'">
              <div v-for="npc in displayNPCs" :key="npc.id" class="npc-card">
                <div class="npc-avatar">{{ npc.name?.[0] || '?' }}</div>
                <div class="npc-info">
                  <div class="npc-name">{{ npc.name }}</div>
                  <div class="npc-status">
                    <span class="status-dot" :class="npc.status"></span>
                    {{ npc.status }}
                  </div>
                </div>
              </div>
              <div v-if="displayNPCs.length === 0" class="empty-state">
                {{ t("simverse.noData") }}
              </div>
            </template>
            <template v-else-if="activePanel === 'gacha'">
              <div class="gacha-section">
                <div class="gacha-banner">
                  <div class="banner-title">🎲 {{ t("simverse.gachaTitle") }}</div>
                  <div class="banner-desc">{{ t("simverse.gachaDesc") }}</div>
                </div>
                <div class="gacha-buttons">
                  <button class="gacha-btn single" @click="doGacha(1)">
                    <span class="btn-icon">🎴</span>
                    <span class="btn-text">{{ t("simverse.singlePull") }}</span>
                    <span class="btn-cost">100 💎</span>
                  </button>
                  <button class="gacha-btn ten" @click="doGacha(10)">
                    <span class="btn-icon">🎴×10</span>
                    <span class="btn-text">{{ t("simverse.tenPull") }}</span>
                    <span class="btn-cost">900 💎</span>
                    <span class="btn-badge">{{ t("simverse.guaranteedRare") }}</span>
                  </button>
                </div>
                <div v-if="gachaResults.length > 0" class="gacha-results">
                  <div class="results-title">{{ t("simverse.results") }}</div>
                  <div class="results-grid">
                    <div v-for="(item, i) in gachaResults" :key="i" class="result-card" :class="item.rarity">
                      <div class="result-icon">{{ item.icon }}</div>
                      <div class="result-name">{{ item.name }}</div>
                      <div class="result-rarity">{{ item.rarity }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </template>
            <template v-else-if="activePanel === 'settings'">
              <div class="setting-group">
                <div class="setting-label">{{ t("simverse.simSpeed") }}</div>
                <input type="range" min="1" max="100" v-model="gravity" class="setting-slider" />
                <div class="setting-value">{{ gravity }}x</div>
              </div>
              <div class="setting-group">
                <div class="setting-label">{{ t("simverse.performanceTier") }}</div>
                <div class="tier-buttons">
                  <button class="tier-btn" :class="{ active: perfTier === 'low' }" @click="perfTier = 'low'">
                    {{ t("simverse.low") }}
                  </button>
                  <button class="tier-btn" :class="{ active: perfTier === 'mid' }" @click="perfTier = 'mid'">
                    {{ t("simverse.mid") }}
                  </button>
                  <button class="tier-btn" :class="{ active: perfTier === 'high' }" @click="perfTier = 'high'">
                    {{ t("simverse.high") }}
                  </button>
                </div>
              </div>
            </template>
            <template v-else>
              <div class="empty-state">{{ t("simverse.comingSoon") }}</div>
            </template>
          </div>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import type { WorldState, Chronicle, NPC } from "@encv/shared-components/types/simverse";

const { t } = useI18n();
const router = useRouter();

const canvasRef = ref<HTMLCanvasElement | null>(null);
const isPaused = ref(false);
const gravity = ref(50);
const perfTier = ref<'low' | 'mid' | 'high'>('mid');

const activePanel = ref<string | null>(null);

const worldState = ref<WorldState | null>({
  tick: 12345,
  era: "启蒙时代",
  npcCount: 8642,
  isActive: true,
});

const recentEvents = ref<Chronicle[]>([
  { id: "1", title: "村庄发现新矿脉", content: "...", tick: 12340, timestamp: Date.now() - 5000 },
  { id: "2", title: "两大家族联姻", content: "...", tick: 12338, timestamp: Date.now() - 15000 },
  { id: "3", title: "商队抵达边境", content: "...", tick: 12335, timestamp: Date.now() - 30000 },
]);

const displayNPCs = ref<NPC[]>([
  { id: "1", name: "艾尔文", status: "active" },
  { id: "2", name: "莉莉丝", status: "active" },
  { id: "3", name: "马库斯", status: "sleeping" },
  { id: "4", name: "索菲亚", status: "active" },
  { id: "5", name: "雷恩", status: "inactive" },
  { id: "6", name: "艾米莉", status: "active" },
]);

const gachaResults = ref<{ name: string; icon: string; rarity: string }[]>([]);

const eraName = computed(() => worldState.value?.era ?? `时代 ${Math.floor((worldState.value?.tick ?? 0) / 1000)}`);

const rarityColors: Record<string, string> = {
  N: "#9ca3af",
  R: "#3b82f6",
  SR: "#8b5cf6",
  SSR: "#f59e0b",
  UR: "#ef4444",
};

interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  color: string;
  mass: number;
}

let particles: Particle[] = [];
let animationId = 0;
let lastTime = 0;
let ctx: CanvasRenderingContext2D | null = null;
let canvasWidth = 0;
let canvasHeight = 0;
let dpr = 1;

const particleColors = [
  "#ff6b9d",
  "#c44dff",
  "#4d9fff",
  "#4dffb8",
  "#ffeb4d",
  "#ff884d",
  "#ff4d6d",
  "#7c4dff",
];

function initParticles(count: number) {
  particles = [];
  for (let i = 0; i < count; i++) {
    const radius = 4 + Math.random() * 12;
    particles.push({
      x: Math.random() * canvasWidth,
      y: Math.random() * canvasHeight,
      vx: (Math.random() - 0.5) * 80,
      vy: (Math.random() - 0.5) * 40,
      radius,
      color: particleColors[Math.floor(Math.random() * particleColors.length)],
      mass: radius * radius,
    });
  }
}

function resizeCanvas() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const container = canvas.parentElement;
  if (!container) return;
  dpr = window.devicePixelRatio || 1;
  canvasWidth = container.clientWidth;
  canvasHeight = container.clientHeight;
  canvas.width = canvasWidth * dpr;
  canvas.height = canvasHeight * dpr;
  canvas.style.width = `${canvasWidth}px`;
  canvas.style.height = `${canvasHeight}px`;
  if (ctx) {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
}

function drawParticle(p: Particle) {
  if (!ctx) return;
  ctx.save();
  const gradient = ctx.createRadialGradient(p.x - p.radius * 0.3, p.y - p.radius * 0.3, 0, p.x, p.y, p.radius);
  gradient.addColorStop(0, lightenColor(p.color, 60));
  gradient.addColorStop(0.5, p.color);
  gradient.addColorStop(1, darkenColor(p.color, 40));
  ctx.fillStyle = gradient;
  ctx.shadowColor = p.color;
  ctx.shadowBlur = 15;
  ctx.beginPath();
  ctx.arc(p.x, p.y, p.radius, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();
}

function lightenColor(color: string, percent: number): string {
  const num = parseInt(color.replace("#", ""), 16);
  const amt = Math.round(2.55 * percent);
  const R = Math.min(255, (num >> 16) + amt);
  const G = Math.min(255, ((num >> 8) & 0x00ff) + amt);
  const B = Math.min(255, (num & 0x0000ff) + amt);
  return `rgb(${R},${G},${B})`;
}

function darkenColor(color: string, percent: number): string {
  const num = parseInt(color.replace("#", ""), 16);
  const amt = Math.round(2.55 * percent);
  const R = Math.max(0, (num >> 16) - amt);
  const G = Math.max(0, ((num >> 8) & 0x00ff) - amt);
  const B = Math.max(0, (num & 0x0000ff) - amt);
  return `rgb(${R},${G},${B})`;
}

function updatePhysics(dt: number) {
  const g = gravity.value * 0.2;
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

        const v1 = { x: a.vx, y: a.vy };
        const v2 = { x: b.vx, y: b.vy };
        const normal = { x: Math.cos(angle), y: Math.sin(angle) };
        const tangent = { x: -normal.y, y: normal.x };

        const v1n = v1.x * normal.x + v1.y * normal.y;
        const v1t = v1.x * tangent.x + v1.y * tangent.y;
        const v2n = v2.x * normal.x + v2.y * normal.y;
        const v2t = v2.x * tangent.x + v2.y * tangent.y;

        const v1nAfter = (v1n * (a.mass - b.mass) + 2 * b.mass * v2n) / totalMass;
        const v2nAfter = (v2n * (b.mass - a.mass) + 2 * a.mass * v1n) / totalMass;

        a.vx = v1nAfter * normal.x + v1t * tangent.x;
        a.vy = v1nAfter * normal.y + v1t * tangent.y;
        b.vx = v2nAfter * normal.x + v2t * tangent.x;
        b.vy = v2nAfter * normal.y + v2t * tangent.y;
      }
    }
  }
}

function render() {
  if (!ctx) return;
  ctx.fillStyle = "rgba(15, 15, 35, 0.3)";
  ctx.fillRect(0, 0, canvasWidth, canvasHeight);

  for (const p of particles) {
    drawParticle(p);
  }
}

function gameLoop(timestamp: number) {
  if (!lastTime) lastTime = timestamp;
  const dt = Math.min(0.05, (timestamp - lastTime) / 1000);
  lastTime = timestamp;

  if (!isPaused.value) {
    updatePhysics(dt);
  }
  render();

  animationId = requestAnimationFrame(gameLoop);
}

function handlePointerDown(e: PointerEvent) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  const x = e.clientX - rect.left;
  const y = e.clientY - rect.top;
  spawnParticles(x, y, 5);
}

function spawnParticles(x: number, y: number, count: number) {
  for (let i = 0; i < count; i++) {
    if (particles.length >= 500) particles.shift();
    const radius = 4 + Math.random() * 12;
    particles.push({
      x,
      y,
      vx: (Math.random() - 0.5) * 200,
      vy: (Math.random() - 0.5) * 200 - 50,
      radius,
      color: particleColors[Math.floor(Math.random() * particleColors.length)],
      mass: radius * radius,
    });
  }
}

function togglePause() {
  isPaused.value = !isPaused.value;
}

function exitWorld() {
  router.push("/tabs/home");
}

function openPanel(name: string) {
  if (activePanel.value === name) {
    activePanel.value = null;
  } else {
    activePanel.value = name;
  }
}

function getPanelTitle(panel: string): string {
  const titles: Record<string, string> = {
    npc: t("simverse.npc"),
    org: t("simverse.org"),
    gacha: t("simverse.gacha"),
    chronicles: t("simverse.chronicles"),
    settings: t("simverse.settings"),
  };
  return titles[panel] || panel;
}

function doGacha(count: number) {
  const results: { name: string; icon: string; rarity: string }[] = [];
  const pool = [
    { name: "普通村民", icon: "👤", rarity: "N", weight: 60 },
    { name: "熟练工匠", icon: "🔨", rarity: "R", weight: 25 },
    { name: "精英战士", icon: "⚔️", rarity: "SR", weight: 10 },
    { name: "传奇英雄", icon: "👑", rarity: "SSR", weight: 4 },
    { name: "神话存在", icon: "🌠", rarity: "UR", weight: 1 },
  ];

  for (let i = 0; i < count; i++) {
    const isGuaranteed = count === 10 && i === 9;
    let rarity: string;

    if (isGuaranteed) {
      rarity = pool.filter((p) => ["SR", "SSR", "UR"].includes(p.rarity))[
        Math.floor(Math.random() * 3)
      ].rarity;
    } else {
      const total = pool.reduce((s, p) => s + p.weight, 0);
      let rand = Math.random() * total;
      rarity = pool[0].rarity;
      for (const item of pool) {
        rand -= item.weight;
        if (rand <= 0) {
          rarity = item.rarity;
          break;
        }
      }
    }

    const matched = pool.filter((p) => p.rarity === rarity)[0];
    results.push({ ...matched });
  }

  gachaResults.value = results;
  activePanel.value = "gacha";
}

watch(perfTier, (tier) => {
  if (tier === "low") {
    if (particles.length > 50) particles = particles.slice(0, 50);
  }
  if (tier === "mid") {
    if (particles.length < 80) initParticles(120);
    if (particles.length > 200) particles = particles.slice(0, 200);
  }
  if (tier === "high") {
    if (particles.length < 200) initParticles(300);
  }
});

let resizeObserver: ResizeObserver | null = null;

onMounted(async () => {
  const canvas = canvasRef.value;
  if (canvas) {
    ctx = canvas.getContext("2d");
    animationId = requestAnimationFrame(gameLoop);
    canvas.addEventListener("pointerdown", handlePointerDown);
    window.addEventListener("resize", resizeCanvas);

    const container = canvas.parentElement;
    if (container) {
      resizeObserver = new ResizeObserver(() => {
        resizeCanvas();
        if (particles.length === 0) {
          initParticles(150);
        }
      });
      resizeObserver.observe(container);
    }

    await nextTick();
    setTimeout(() => {
      resizeCanvas();
      if (particles.length === 0) {
        initParticles(150);
      }
    }, 100);
  }
});

onUnmounted(() => {
  if (animationId) cancelAnimationFrame(animationId);
  if (resizeObserver) resizeObserver.disconnect();
  const canvas = canvasRef.value;
  if (canvas) {
    canvas.removeEventListener("pointerdown", handlePointerDown);
    window.removeEventListener("resize", resizeCanvas);
  }
});
</script>

<style scoped>
.world-page {
  --background: #0f0f23;
}

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
  overflow: hidden;
}

.game-container {
  position: relative;
  width: 100%;
  height: 100vh;
  overflow: hidden;
  background: linear-gradient(180deg, #1a1a3e 0%, #0f0f23 100%);
}

.game-canvas {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  touch-action: none;
}

.top-bar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 16px 20px;
  background: linear-gradient(180deg, rgba(0, 0, 0, 0.6) 0%, transparent 100%);
  z-index: 10;
}

.resource-group {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(10px);
  padding: 6px 14px;
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.15);
}

.resource-icon {
  font-size: 16px;
}

.resource-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
}

.resource-value {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  min-width: 30px;
  text-align: right;
}

.top-actions {
  display: flex;
  gap: 8px;
}

.icon-btn {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: #fff;
  font-size: 14px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.icon-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  transform: scale(1.05);
}

.icon-btn:active {
  transform: scale(0.95);
}

.event-panel {
  position: absolute;
  top: 70px;
  right: 20px;
  width: 280px;
  max-height: 200px;
  background: rgba(20, 20, 50, 0.85);
  backdrop-filter: blur(10px);
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: 12px;
  z-index: 9;
  overflow: hidden;
}

.event-header {
  font-size: 13px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  margin-bottom: 8px;
}

.event-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.event-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
}

.event-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #4dffb8;
  flex-shrink: 0;
}

.event-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bottom-menu {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  display: flex;
  justify-content: space-around;
  align-items: flex-end;
  padding: 12px 24px 16px;
  background: linear-gradient(0deg, rgba(0, 0, 0, 0.7) 0%, transparent 100%);
  z-index: 10;
}

.menu-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  background: none;
  border: none;
  cursor: pointer;
  padding: 8px 12px;
  border-radius: 12px;
  transition: all 0.2s;
}

.menu-item:hover {
  background: rgba(255, 255, 255, 0.1);
  transform: translateY(-2px);
}

.menu-item:active {
  transform: translateY(0);
}

.menu-icon {
  font-size: 28px;
  filter: drop-shadow(0 2px 4px rgba(0, 0, 0, 0.3));
}

.menu-icon.sparkle {
  animation: sparkle 2s ease-in-out infinite;
}

@keyframes sparkle {
  0%, 100% {
    transform: scale(1);
    filter: drop-shadow(0 2px 4px rgba(255, 215, 0, 0.3));
  }
  50% {
    transform: scale(1.1);
    filter: drop-shadow(0 2px 8px rgba(255, 215, 0, 0.6));
  }
}

.menu-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.8);
  font-weight: 500;
}

.menu-item.gacha .menu-label {
  color: #ffd700;
  font-weight: 600;
}

.side-panel {
  position: absolute;
  top: 60px;
  right: 20px;
  bottom: 100px;
  width: 320px;
  background: rgba(20, 20, 50, 0.95);
  backdrop-filter: blur(20px);
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  z-index: 15;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transform: translateX(360px);
  opacity: 0;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none;
}

.side-panel.open {
  transform: translateX(0);
  opacity: 1;
  pointer-events: auto;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.panel-title {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
}

.close-btn {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  border: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.close-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.panel-content::-webkit-scrollbar {
  width: 4px;
}

.panel-content::-webkit-scrollbar-track {
  background: transparent;
}

.panel-content::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
}

.npc-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 10px;
  margin-bottom: 8px;
  transition: all 0.2s;
  cursor: pointer;
}

.npc-card:hover {
  background: rgba(255, 255, 255, 0.1);
  transform: translateX(2px);
}

.npc-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  font-size: 16px;
  flex-shrink: 0;
}

.npc-info {
  flex: 1;
  min-width: 0;
}

.npc-name {
  font-size: 14px;
  font-weight: 500;
  color: #fff;
  margin-bottom: 2px;
}

.npc-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  text-transform: capitalize;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-dot.active {
  background: #22c55e;
}

.status-dot.sleeping {
  background: #f59e0b;
}

.status-dot.inactive {
  background: #6b7280;
}

.empty-state {
  text-align: center;
  padding: 40px 20px;
  color: rgba(255, 255, 255, 0.4);
  font-size: 13px;
}

.gacha-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.gacha-banner {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.3), rgba(236, 72, 153, 0.3));
  border-radius: 12px;
  padding: 16px;
  text-align: center;
  border: 1px solid rgba(139, 92, 246, 0.3);
}

.banner-title {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 4px;
}

.banner-desc {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
}

.gacha-buttons {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.gacha-btn {
  position: relative;
  padding: 14px 16px;
  border-radius: 12px;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 12px;
  transition: all 0.2s;
}

.gacha-btn.single {
  background: linear-gradient(135deg, #3b82f6, #6366f1);
}

.gacha-btn.ten {
  background: linear-gradient(135deg, #f59e0b, #ef4444);
}

.gacha-btn:hover {
  transform: translateY(-2px);
  filter: brightness(1.1);
}

.gacha-btn:active {
  transform: translateY(0);
}

.btn-icon {
  font-size: 24px;
}

.btn-text {
  flex: 1;
  text-align: left;
  font-size: 14px;
  font-weight: 600;
  color: #fff;
}

.btn-cost {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.btn-badge {
  position: absolute;
  top: -6px;
  right: 10px;
  background: #ffd700;
  color: #1a1a1a;
  font-size: 10px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 10px;
}

.gacha-results {
  margin-top: 8px;
}

.results-title {
  font-size: 13px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
  margin-bottom: 10px;
}

.results-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
}

.result-card {
  aspect-ratio: 3 / 4;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  animation: cardReveal 0.5s ease-out;
}

@keyframes cardReveal {
  from {
    opacity: 0;
    transform: scale(0.8) rotateY(90deg);
  }
  to {
    opacity: 1;
    transform: scale(1) rotateY(0);
  }
}

.result-icon {
  font-size: 24px;
}

.result-name {
  font-size: 9px;
  color: rgba(255, 255, 255, 0.7);
  text-align: center;
}

.result-rarity {
  font-size: 9px;
  font-weight: 700;
}

.result-card.N .result-rarity { color: #9ca3af; }
.result-card.R .result-rarity { color: #3b82f6; }
.result-card.SR .result-rarity { color: #8b5cf6; }
.result-card.SSR .result-rarity { color: #f59e0b; }
.result-card.UR .result-rarity { color: #ef4444; }

.setting-group {
  margin-bottom: 20px;
}

.setting-label {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 8px;
}

.setting-slider {
  width: 100%;
  height: 4px;
  -webkit-appearance: none;
  appearance: none;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
  outline: none;
}

.setting-slider::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #8b5cf6;
  cursor: pointer;
  box-shadow: 0 2px 6px rgba(139, 92, 246, 0.4);
}

.setting-value {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  margin-top: 4px;
  text-align: right;
}

.tier-buttons {
  display: flex;
  gap: 8px;
}

.tier-btn {
  flex: 1;
  padding: 8px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.tier-btn.active {
  background: rgba(139, 92, 246, 0.3);
  border-color: rgba(139, 92, 246, 0.5);
  color: #fff;
}

.tier-btn:hover {
  background: rgba(255, 255, 255, 0.1);
}
</style>
