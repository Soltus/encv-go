<template>
  <ion-page class="world-page">
    <ion-content :fullscreen="true" class="world-content">
      <div class="game-container">
        <div class="world-map">
          <div class="map-grid">
            <div v-for="i in 48" :key="i" class="map-cell" :class="getCellClass(i)">
              <div class="cell-icon">{{ getCellIcon(i) }}</div>
            </div>
          </div>
          <div class="map-overlay">
            <div v-for="(npc, idx) in visibleNPCs" :key="npc.id"
                 class="npc-marker"
                 :style="getNPCPosition(idx)"
                 @click="selectNPC(npc)">
              <div class="npc-dot" :class="{ alive: npc.is_alive }"></div>
              <div class="npc-name">{{ npc.name }}</div>
            </div>
          </div>
        </div>

        <div class="top-bar">
          <div class="resource-group">
            <div class="resource-item">
              <span class="resource-icon">⏱️</span>
              <div class="resource-text">
                <span class="resource-label">{{ t("simverse.tick") }}</span>
                <span class="resource-value">{{ worldState?.tick ?? 0 }}</span>
              </div>
            </div>
            <div class="resource-item">
              <span class="resource-icon">👥</span>
              <div class="resource-text">
                <span class="resource-label">{{ t("simverse.population") }}</span>
                <span class="resource-value">{{ worldState?.npc_count ?? 0 }}</span>
              </div>
            </div>
            <div class="resource-item">
              <span class="resource-icon">🧠</span>
              <div class="resource-text">
                <span class="resource-label">{{ t("simverse.brains") }}</span>
                <span class="resource-value">{{ worldState?.brain_count ?? 0 }}</span>
              </div>
            </div>
            <div class="resource-item">
              <span class="resource-icon">💾</span>
              <div class="resource-text">
                <span class="resource-label">{{ t("simverse.memory") }}</span>
                <span class="resource-value">{{ (worldState?.total_mb ?? 0).toFixed(1) }} MB</span>
              </div>
            </div>
          </div>
          <div class="top-actions">
            <button class="icon-btn play-btn" :class="{ running: worldState?.running }" @click="toggleRunning">
              {{ worldState?.running ? "⏸" : "▶" }}
            </button>
            <button class="icon-btn" @click="stepOnce">
              ⏭
            </button>
            <button class="icon-btn" @click="refreshState">
              ↻
            </button>
          </div>
        </div>

        <div class="event-panel" v-if="recentEvents.length > 0">
          <div class="event-header">📜 {{ t("simverse.recentEvents") }}</div>
          <div class="event-list">
            <div v-for="ev in recentEvents.slice(0, 5)" :key="ev.id" class="event-item">
              <span class="event-dot" :class="'imp-' + ev.importance"></span>
              <span class="event-text">{{ ev.type_cn }}</span>
            </div>
          </div>
        </div>

        <div class="stats-panel">
          <div class="stat-card">
            <div class="stat-label">{{ t("simverse.perfTier") }}</div>
            <div class="stat-value">{{ worldState?.tier || "-" }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">{{ t("simverse.focusNPCs") }}</div>
            <div class="stat-value">{{ worldState?.focus_count ?? 0 }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">{{ t("simverse.cellCount") }}</div>
            <div class="stat-value">{{ worldState?.cell_count ?? 0 }}</div>
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
          <button class="menu-item exit-btn" @click="handleExitWorld">
            <div class="menu-icon">🚪</div>
            <div class="menu-label">{{ t("simverse.exitWorld") }}</div>
          </button>
        </div>

        <div v-if="activePanel" class="side-panel" :class="{ open: !!activePanel }">
          <div class="panel-header">
            <span class="panel-title">{{ getPanelTitle(activePanel) }}</span>
            <button class="close-btn" @click="activePanel = null">✕</button>
          </div>
          <div class="panel-content">
            <template v-if="activePanel === 'npc'">
              <div v-for="npc in npcList" :key="npc.id" class="npc-card" @click="selectNPC(npc)">
                <div class="npc-avatar">{{ npc.name?.[0] || '?' }}</div>
                <div class="npc-info">
                  <div class="npc-name">{{ npc.name }}</div>
                  <div class="npc-meta">
                    <span class="npc-prof">{{ npc.profession }}</span>
                    <span class="npc-level">Lv.{{ npc.level }}</span>
                  </div>
                </div>
                <div class="npc-status-col">
                  <div class="hp-bar">
                    <div class="hp-fill" :style="{ width: (npc.health / npc.max_health * 100) + '%' }"></div>
                  </div>
                </div>
              </div>
              <div v-if="npcList.length === 0" class="empty-state">
                {{ t("simverse.noData") }}
              </div>
              <ion-infinite-scroll v-if="hasMoreNPCs" @ionInfinite="loadMoreNPCs" threshold="100px">
                <ion-infinite-scroll-content></ion-infinite-scroll-content>
              </ion-infinite-scroll>
            </template>

            <template v-else-if="activePanel === 'chronicles'">
              <div v-for="ev in recentEvents" :key="ev.id" class="chronicle-item">
                <div class="chronicle-tick">Tick {{ ev.tick }}</div>
                <div class="chronicle-title">{{ ev.type_cn }}</div>
                <div class="chronicle-content">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
              </div>
              <div v-if="recentEvents.length === 0" class="empty-state">
                {{ t("simverse.noData") }}
              </div>
            </template>

            <template v-else-if="activePanel === 'settings'">
              <div class="setting-group">
                <div class="setting-label">{{ t("simverse.performanceTier") }}</div>
                <div class="tier-buttons">
                  <button class="tier-btn" :class="{ active: worldConfig?.tier === 'background' }"
                          @click="changeTier('background')">
                    {{ t("simverse.tierBackground") }}
                  </button>
                  <button class="tier-btn" :class="{ active: worldConfig?.tier === 'foreground' }"
                          @click="changeTier('foreground')">
                    {{ t("simverse.tierForeground") }}
                  </button>
                  <button class="tier-btn" :class="{ active: worldConfig?.tier === 'fg_idle' }"
                          @click="changeTier('fg_idle')">
                    {{ t("simverse.tierIdle") }}
                  </button>
                </div>
              </div>
              <div v-if="worldConfig" class="setting-group">
                <div class="setting-label">{{ t("simverse.configDetails") }}</div>
                <div class="config-detail">
                  <span>{{ t("simverse.eventRate") }}</span>
                  <span>{{ worldConfig.event_rate_mul }}x</span>
                </div>
                <div class="config-detail">
                  <span>{{ t("simverse.cacheSize") }}</span>
                  <span>{{ worldConfig.cache_size }}</span>
                </div>
                <div class="config-detail">
                  <span>{{ t("simverse.subSim") }}</span>
                  <span>{{ worldConfig.sub_sim_active ? t("common.on") : t("common.off") }}</span>
                </div>
                <div class="config-detail">
                  <span>{{ t("simverse.subSimDepth") }}</span>
                  <span>{{ worldConfig.sub_sim_depth }}</span>
                </div>
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

            <template v-else>
              <div class="empty-state">{{ t("simverse.comingSoon") }}</div>
            </template>
          </div>
        </div>

        <div v-if="selectedNPC" class="npc-detail-modal" @click.self="selectedNPC = null">
          <div class="npc-detail-card">
            <div class="detail-header">
              <div class="detail-avatar">{{ selectedNPC.name?.[0] }}</div>
              <div class="detail-info">
                <div class="detail-name">{{ selectedNPC.name }}</div>
                <div class="detail-subtitle">
                  {{ selectedNPC.species }} · {{ selectedNPC.gender }} · {{ selectedNPC.age }}岁
                </div>
              </div>
              <button class="close-btn" @click="selectedNPC = null">✕</button>
            </div>
            <div class="detail-body">
              <div class="detail-row">
                <span>{{ t("simverse.profession") }}</span>
                <span>{{ selectedNPC.profession }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.level") }}</span>
                <span>Lv.{{ selectedNPC.level }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.health") }}</span>
                <span>{{ selectedNPC.health }} / {{ selectedNPC.max_health }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.energy") }}</span>
                <span>{{ selectedNPC.energy }} / {{ selectedNPC.max_energy }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.wealthTier") }}</span>
                <span>{{ selectedNPC.wealth_tier }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.socialTier") }}</span>
                <span>{{ selectedNPC.social_tier }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.lifeStage") }}</span>
                <span>{{ selectedNPC.life_stage }}</span>
              </div>
              <div class="detail-row">
                <span>{{ t("simverse.alive") }}</span>
                <span :class="{ alive: selectedNPC.is_alive, dead: !selectedNPC.is_alive }">
                  {{ selectedNPC.is_alive ? t("common.on") : t("common.off") }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseNPC, type SimverseChronicleEvent } from "@/composables/useSimverse";
import { IonInfiniteScroll, IonInfiniteScrollContent } from "@ionic/vue";
import { lockScreenOrientation, unlockScreenOrientation, closeWorld, isNativePluginMode } from "@/plugins/SimVerse";

const { t } = useI18n();
const {
  worldState,
  worldConfig,
  isRunning,
  currentTick,
  loadWorldState,
  loadWorldConfig,
  setPerformanceTier,
  controlWorld,
  loadNPCList,
  loadChronicleWorld,
  init,
  cleanup,
} = useSimverse();

const npcList = ref<SimverseNPC[]>([]);
const npcPage = ref(1);
const hasMoreNPCs = ref(true);
const recentEvents = ref<SimverseChronicleEvent[]>([]);
const activePanel = ref<string | null>(null);
const selectedNPC = ref<SimverseNPC | null>(null);
const gachaResults = ref<{ name: string; icon: string; rarity: string }[]>([]);

let pollInterval: number | null = null;

const visibleNPCs = computed(() => npcList.value.slice(0, 12));

const cellTypes = ["forest", "mountain", "water", "plain", "village", "city", "desert"];
const cellIcons: Record<string, string> = {
  forest: "🌲",
  mountain: "⛰️",
  water: "🌊",
  plain: "🌾",
  village: "🏘️",
  city: "🏙️",
  desert: "🏜️",
};

function getCellClass(i: number): string {
  const seed = (i * 7 + 13) % cellTypes.length;
  return cellTypes[seed];
}

function getCellIcon(i: number): string {
  const seed = (i * 7 + 13) % cellTypes.length;
  return cellIcons[cellTypes[seed]];
}

function getNPCPosition(idx: number): Record<string, string> {
  const cols = 8;
  const col = idx % cols;
  const row = Math.floor(idx / cols);
  return {
    left: `${12.5 + col * 12 + (idx % 3) * 3}%`,
    top: `${15 + row * 18 + (idx % 2) * 5}%`,
  };
}

async function refreshState() {
  await Promise.all([
    loadWorldState(),
    loadWorldConfig(),
  ]);
}

async function toggleRunning() {
  if (!worldState.value) return;
  const action = worldState.value.running ? "pause" : "resume";
  await controlWorld(action);
  await refreshState();
}

async function stepOnce() {
  await controlWorld("step");
  await refreshState();
}

async function changeTier(tier: string) {
  const result = await setPerformanceTier(tier as any);
  if (result) {
    await refreshState();
  }
}

async function loadNPCs() {
  const result = await loadNPCList(npcPage.value, 20);
  if (result) {
    if (npcPage.value === 1) {
      npcList.value = result.items;
    } else {
      npcList.value = [...npcList.value, ...result.items];
    }
    hasMoreNPCs.value = npcList.value.length < result.total;
  }
}

async function loadMoreNPCs(ev: any) {
  npcPage.value++;
  await loadNPCs();
  ev.target.complete();
}

async function loadEvents() {
  const data = await loadChronicleWorld(0, 20);
  recentEvents.value = data?.items || [];
}

function selectNPC(npc: SimverseNPC) {
  selectedNPC.value = npc;
}

function openPanel(name: string) {
  if (activePanel.value === name) {
    activePanel.value = null;
  } else {
    activePanel.value = name;
    if (name === "npc" && npcList.value.length === 0) {
      loadNPCs();
    }
    if (name === "chronicles" && recentEvents.value.length === 0) {
      loadEvents();
    }
    if (name === "settings" && !worldConfig.value) {
      refreshState();
    }
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

async function handleExitWorld() {
  try {
    if (isNativePluginMode()) {
      await unlockScreenOrientation();
      await closeWorld();
    } else {
      window.history.back();
    }
  } catch (e) {
    console.warn("[SimverseWorld] Exit world failed:", e);
    if (!isNativePluginMode()) {
      window.history.back();
    }
  }
}

function startPolling() {
  pollInterval = window.setInterval(() => {
    refreshState();
  }, 3000);
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval);
    pollInterval = null;
  }
}

onMounted(async () => {
  if (isNativePluginMode()) {
    lockScreenOrientation("landscape-primary").catch((e) => {
      console.warn("[SimverseWorld] Lock orientation failed:", e);
    });
  }
  await init();
  await refreshState();
  await loadNPCs();
  startPolling();
});

onUnmounted(() => {
  stopPolling();
  cleanup();
  if (isNativePluginMode()) {
    unlockScreenOrientation().catch(() => {});
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

.world-map {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  padding: 80px 20px 120px;
}

.map-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  grid-template-rows: repeat(6, 1fr);
  gap: 4px;
  height: 100%;
  opacity: 0.6;
}

.map-cell {
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  transition: all 0.3s;
}

.map-cell.forest { background: rgba(34, 197, 94, 0.15); }
.map-cell.mountain { background: rgba(107, 114, 128, 0.2); }
.map-cell.water { background: rgba(59, 130, 246, 0.15); }
.map-cell.plain { background: rgba(234, 179, 8, 0.1); }
.map-cell.village { background: rgba(139, 92, 246, 0.15); }
.map-cell.city { background: rgba(236, 72, 153, 0.15); }
.map-cell.desert { background: rgba(249, 115, 22, 0.1); }

.map-overlay {
  position: absolute;
  top: 80px;
  left: 20px;
  right: 20px;
  bottom: 120px;
  pointer-events: none;
}

.npc-marker {
  position: absolute;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  pointer-events: auto;
  cursor: pointer;
  transition: transform 0.2s;
}

.npc-marker:hover {
  transform: translate(-50%, -50%) scale(1.2);
  z-index: 10;
}

.npc-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #6b7280;
  border: 2px solid rgba(255, 255, 255, 0.5);
  box-shadow: 0 0 8px rgba(0, 0, 0, 0.5);
}

.npc-dot.alive {
  background: #22c55e;
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { box-shadow: 0 0 4px rgba(34, 197, 94, 0.5); }
  50% { box-shadow: 0 0 12px rgba(34, 197, 94, 0.8); }
}

.npc-name {
  font-size: 9px;
  color: rgba(255, 255, 255, 0.7);
  white-space: nowrap;
  margin-top: 2px;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.8);
}

.top-bar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 12px 16px;
  background: linear-gradient(180deg, rgba(0, 0, 0, 0.6) 0%, transparent 100%);
  z-index: 10;
}

.resource-group {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(10px);
  padding: 6px 12px;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.resource-icon {
  font-size: 14px;
}

.resource-text {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}

.resource-label {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
}

.resource-value {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
}

.top-actions {
  display: flex;
  gap: 6px;
}

.icon-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: #fff;
  font-size: 12px;
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

.icon-btn.play-btn.running {
  background: rgba(34, 197, 94, 0.3);
  border-color: rgba(34, 197, 94, 0.5);
}

.event-panel {
  position: absolute;
  top: 70px;
  right: 16px;
  width: 240px;
  max-height: 160px;
  background: rgba(20, 20, 50, 0.85);
  backdrop-filter: blur(10px);
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  padding: 10px;
  z-index: 9;
  overflow: hidden;
}

.event-header {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  margin-bottom: 6px;
}

.event-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.event-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.6);
}

.event-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #4dffb8;
  flex-shrink: 0;
}

.event-dot.warning { background: #f59e0b; }
.event-dot.danger { background: #ef4444; }

.event-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stats-panel {
  position: absolute;
  bottom: 100px;
  left: 16px;
  display: flex;
  gap: 8px;
  z-index: 9;
}

.stat-card {
  background: rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 8px 12px;
  min-width: 70px;
}

.stat-label {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: 2px;
}

.stat-value {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
}

.bottom-menu {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  display: flex;
  justify-content: space-around;
  align-items: flex-end;
  padding: 10px 20px 14px;
  background: linear-gradient(0deg, rgba(0, 0, 0, 0.7) 0%, transparent 100%);
  z-index: 10;
}

.menu-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  background: none;
  border: none;
  cursor: pointer;
  padding: 6px 10px;
  border-radius: 10px;
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
  font-size: 24px;
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
  font-size: 10px;
  color: rgba(255, 255, 255, 0.8);
  font-weight: 500;
}

.menu-item.gacha .menu-label {
  color: #ffd700;
  font-weight: 600;
}

.menu-item.exit-btn .menu-label {
  color: #ef4444;
  font-weight: 500;
}

.side-panel {
  position: absolute;
  top: 56px;
  right: 16px;
  bottom: 90px;
  width: 280px;
  background: rgba(20, 20, 50, 0.95);
  backdrop-filter: blur(20px);
  border-radius: 14px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  z-index: 15;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transform: translateX(320px);
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
  padding: 12px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
}

.close-btn {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.08);
  border: none;
  color: rgba(255, 255, 255, 0.6);
  font-size: 11px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.close-btn:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
}

.panel-content::-webkit-scrollbar {
  width: 4px;
}

.panel-content::-webkit-scrollbar-track {
  background: transparent;
}

.panel-content::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.15);
  border-radius: 2px;
}

.npc-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 8px;
  margin-bottom: 6px;
  transition: all 0.2s;
  cursor: pointer;
}

.npc-card:hover {
  background: rgba(255, 255, 255, 0.08);
  transform: translateX(2px);
}

.npc-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  font-size: 14px;
  flex-shrink: 0;
}

.npc-info {
  flex: 1;
  min-width: 0;
}

.npc-name {
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  margin-bottom: 2px;
}

.npc-meta {
  display: flex;
  gap: 8px;
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
}

.npc-prof { text-transform: capitalize; }
.npc-level { color: #f59e0b; }

.npc-status-col {
  width: 50px;
  flex-shrink: 0;
}

.hp-bar {
  width: 100%;
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
  overflow: hidden;
}

.hp-fill {
  height: 100%;
  background: linear-gradient(90deg, #22c55e, #4ade80);
  border-radius: 2px;
  transition: width 0.3s;
}

.empty-state {
  text-align: center;
  padding: 30px 16px;
  color: rgba(255, 255, 255, 0.3);
  font-size: 12px;
}

.chronicle-item {
  padding: 10px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 8px;
  margin-bottom: 6px;
  border-left: 3px solid #8b5cf6;
}

.chronicle-tick {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
  margin-bottom: 2px;
}

.chronicle-title {
  font-size: 12px;
  font-weight: 500;
  color: #fff;
  margin-bottom: 2px;
}

.chronicle-content {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  line-height: 1.4;
}

.setting-group {
  margin-bottom: 16px;
}

.setting-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 8px;
}

.tier-buttons {
  display: flex;
  gap: 6px;
}

.tier-btn {
  flex: 1;
  padding: 8px 4px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.5);
  font-size: 10px;
  cursor: pointer;
  transition: all 0.2s;
}

.tier-btn.active {
  background: rgba(139, 92, 246, 0.25);
  border-color: rgba(139, 92, 246, 0.5);
  color: #fff;
}

.tier-btn:hover {
  background: rgba(255, 255, 255, 0.08);
}

.config-detail {
  display: flex;
  justify-content: space-between;
  padding: 6px 0;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}

.config-detail span:last-child {
  color: rgba(255, 255, 255, 0.8);
  font-weight: 500;
}

.gacha-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.gacha-banner {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.25), rgba(236, 72, 153, 0.25));
  border-radius: 10px;
  padding: 12px;
  text-align: center;
  border: 1px solid rgba(139, 92, 246, 0.3);
}

.banner-title {
  font-size: 16px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 2px;
}

.banner-desc {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
}

.gacha-buttons {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.gacha-btn {
  position: relative;
  padding: 12px;
  border-radius: 10px;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 10px;
  transition: all 0.2s;
}

.gacha-btn.single {
  background: linear-gradient(135deg, #3b82f6, #6366f1);
}

.gacha-btn.ten {
  background: linear-gradient(135deg, #f59e0b, #ef4444);
}

.gacha-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.1);
}

.gacha-btn:active {
  transform: translateY(0);
}

.btn-icon { font-size: 20px; }

.btn-text {
  flex: 1;
  text-align: left;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
}

.btn-cost {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.btn-badge {
  position: absolute;
  top: -5px;
  right: 8px;
  background: #ffd700;
  color: #1a1a1a;
  font-size: 9px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 8px;
}

.gacha-results { margin-top: 4px; }

.results-title {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 8px;
}

.results-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
}

.result-card {
  aspect-ratio: 3 / 4;
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  animation: cardReveal 0.4s ease-out;
}

@keyframes cardReveal {
  from { opacity: 0; transform: scale(0.8) rotateY(90deg); }
  to { opacity: 1; transform: scale(1) rotateY(0); }
}

.result-icon { font-size: 20px; }

.result-name {
  font-size: 8px;
  color: rgba(255, 255, 255, 0.6);
  text-align: center;
}

.result-rarity {
  font-size: 8px;
  font-weight: 700;
}

.result-card.N .result-rarity { color: #9ca3af; }
.result-card.R .result-rarity { color: #3b82f6; }
.result-card.SR .result-rarity { color: #8b5cf6; }
.result-card.SSR .result-rarity { color: #f59e0b; }
.result-card.UR .result-rarity { color: #ef4444; }

.npc-detail-modal {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 20;
  backdrop-filter: blur(4px);
}

.npc-detail-card {
  width: 85%;
  max-width: 320px;
  background: rgba(20, 20, 50, 0.98);
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  overflow: hidden;
}

.detail-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.2), rgba(236, 72, 153, 0.2));
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  position: relative;
}

.detail-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 20px;
  flex-shrink: 0;
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.detail-name {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 2px;
}

.detail-subtitle {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.detail-header .close-btn {
  position: absolute;
  top: 12px;
  right: 12px;
}

.detail-body {
  padding: 12px 16px;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
  font-size: 13px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}

.detail-row span:first-child {
  color: rgba(255, 255, 255, 0.5);
}

.detail-row span:last-child {
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.detail-row .alive { color: #22c55e; }
.detail-row .dead { color: #ef4444; }
</style>
