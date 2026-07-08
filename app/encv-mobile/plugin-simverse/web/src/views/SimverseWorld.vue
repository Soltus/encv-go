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
            <div class="resource-item" v-tooltip="t('simverse.tick')">
              <span class="resource-icon">⏱️</span>
              <span class="resource-value">{{ worldState?.tick ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.population')">
              <span class="resource-icon">👥</span>
              <span class="resource-value">{{ worldState?.npc_count ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.brains')">
              <span class="resource-icon">🧠</span>
              <span class="resource-value">{{ worldState?.brain_count ?? 0 }}</span>
            </div>
            <div class="resource-item" v-tooltip="t('simverse.memory')">
              <span class="resource-icon">💾</span>
              <span class="resource-value">{{ (worldState?.total_mb ?? 0).toFixed(1) }}M</span>
            </div>
          </div>
          <div class="top-actions">
            <button class="game-btn play-btn" :class="{ running: worldState?.running }" @click="toggleRunning">
              <span class="btn-icon">{{ worldState?.running ? "⏸" : "▶" }}</span>
            </button>
            <button class="game-btn" @click="stepOnce">
              <span class="btn-icon">⏭</span>
            </button>
            <button class="game-btn" @click="refreshState">
              <span class="btn-icon">↻</span>
            </button>
          </div>
        </div>

        <div class="left-menu">
          <button class="menu-btn" :class="{ active: activePanel === 'npc' }" @click="openPanel('npc')">
            <span class="menu-icon">👤</span>
            <span class="menu-label">{{ t("simverse.npc") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'org' }" @click="openPanel('org')">
            <span class="menu-icon">🏰</span>
            <span class="menu-label">{{ t("simverse.org") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'chronicles' }" @click="openPanel('chronicles')">
            <span class="menu-icon">📖</span>
            <span class="menu-label">{{ t("simverse.chronicles") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'economy' }" @click="openPanel('economy')">
            <span class="menu-icon">💰</span>
            <span class="menu-label">{{ t("simverse.economy") }}</span>
          </button>
          <div class="menu-divider"></div>
          <button class="menu-btn gacha-btn" @click="openPanel('gacha')">
            <span class="menu-icon sparkle">✨</span>
            <span class="menu-label">{{ t("simverse.gacha") }}</span>
          </button>
        </div>

        <div class="right-menu">
          <button class="menu-btn" :class="{ active: activePanel === 'settings' }" @click="openPanel('settings')">
            <span class="menu-icon">⚙️</span>
            <span class="menu-label">{{ t("simverse.settings") }}</span>
          </button>
          <button class="menu-btn" :class="{ active: activePanel === 'logs' }" @click="openPanel('logs')">
            <span class="menu-icon">📜</span>
            <span class="menu-label">日志</span>
          </button>
          <button class="menu-btn" @click="openPanel('intervention')">
            <span class="menu-icon">⚡</span>
            <span class="menu-label">{{ t("simverse.intervention") }}</span>
          </button>
          <button class="menu-btn" @click="openPanel('debug')">
            <span class="menu-icon">🔧</span>
            <span class="menu-label">{{ t("simverse.debug") }}</span>
          </button>
          <div class="menu-divider"></div>
          <button class="menu-btn exit-btn" @click="handleExitWorld">
            <span class="menu-icon">🚪</span>
            <span class="menu-label">{{ t("simverse.exitWorld") }}</span>
          </button>
        </div>

        <div class="stats-bar">
          <div class="stat-pill">
            <span class="stat-label">{{ t("simverse.perfTier") }}</span>
            <span class="stat-value">{{ worldState?.tier || "-" }}</span>
          </div>
          <div class="stat-pill">
            <span class="stat-label">{{ t("simverse.focusNPCs") }}</span>
            <span class="stat-value">{{ worldState?.focus_count ?? 0 }}</span>
          </div>
          <div class="stat-pill">
            <span class="stat-label">{{ t("simverse.cellCount") }}</span>
            <span class="stat-value">{{ worldState?.cell_count ?? 0 }}</span>
          </div>
        </div>

        <div class="event-ticker" v-if="recentEvents.length > 0">
          <div class="ticker-icon">📜</div>
          <div class="ticker-content">
            <transition-group name="ticker" tag="div" class="ticker-list">
              <div v-for="ev in recentEvents.slice(0, 3)" :key="ev.id" class="ticker-item">
                <span class="event-dot" :class="'imp-' + ev.importance"></span>
                <span class="event-text">{{ ev.type_cn }}</span>
              </div>
            </transition-group>
          </div>
        </div>

        <div v-if="activePanel" class="side-panel" :class="{ open: !!activePanel, 'panel-left': panelOnLeft }">
          <div class="panel-header">
            <span class="panel-title">{{ getPanelTitle(activePanel) }}</span>
            <button class="panel-close-btn" @click="activePanel = null">✕</button>
          </div>
          <div class="panel-content">
            <template v-if="activePanel === 'npc'">
              <div v-for="npc in npcList" :key="npc.id" class="list-card" @click="selectNPC(npc)">
                <div class="card-avatar">{{ npc.name?.[0] || '?' }}</div>
                <div class="card-info">
                  <div class="card-title">{{ npc.name }}</div>
                  <div class="card-subtitle">
                    <span class="prof-tag">{{ npc.profession }}</span>
                    <span class="level-tag">Lv.{{ npc.level }}</span>
                  </div>
                </div>
                <div class="card-action">
                  <div class="mini-hp-bar">
                    <div class="mini-hp-fill" :style="{ width: (npc.health / npc.max_health * 100) + '%' }"></div>
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
              <div v-for="ev in recentEvents" :key="ev.id" class="chronicle-card">
                <div class="chronicle-tick">Tick {{ ev.tick }}</div>
                <div class="chronicle-title">{{ ev.type_cn }}</div>
                <div class="chronicle-desc">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
              </div>
              <div v-if="recentEvents.length === 0" class="empty-state">
                {{ t("simverse.noData") }}
              </div>
            </template>

            <template v-else-if="activePanel === 'settings'">
              <div class="setting-section">
                <div class="section-title">{{ t("simverse.performanceTier") }}</div>
                <div class="tier-selector">
                  <button class="tier-option" :class="{ active: worldConfig?.tier === 'background' }"
                          @click="changeTier('background')">
                    <span class="tier-name">{{ t("simverse.tierBackground") }}</span>
                    <span class="tier-desc">低功耗</span>
                  </button>
                  <button class="tier-option" :class="{ active: worldConfig?.tier === 'foreground' }"
                          @click="changeTier('foreground')">
                    <span class="tier-name">{{ t("simverse.tierForeground") }}</span>
                    <span class="tier-desc">标准</span>
                  </button>
                  <button class="tier-option" :class="{ active: worldConfig?.tier === 'fg_idle' }"
                          @click="changeTier('fg_idle')">
                    <span class="tier-name">{{ t("simverse.tierIdle") }}</span>
                    <span class="tier-desc">高性能</span>
                  </button>
                </div>
              </div>
              <div v-if="worldConfig" class="setting-section">
                <div class="section-title">{{ t("simverse.configDetails") }}</div>
                <div class="config-list">
                  <div class="config-row">
                    <span>{{ t("simverse.eventRate") }}</span>
                    <span class="config-value">{{ worldConfig.event_rate_mul }}x</span>
                  </div>
                  <div class="config-row">
                    <span>{{ t("simverse.cacheSize") }}</span>
                    <span class="config-value">{{ worldConfig.cache_size }}</span>
                  </div>
                  <div class="config-row">
                    <span>{{ t("simverse.subSim") }}</span>
                    <span class="config-value">{{ worldConfig.sub_sim_active ? t("common.on") : t("common.off") }}</span>
                  </div>
                  <div class="config-row">
                    <span>{{ t("simverse.subSimDepth") }}</span>
                    <span class="config-value">{{ worldConfig.sub_sim_depth }}</span>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'gacha'">
              <div class="gacha-banner">
                <div class="banner-icon">🎲</div>
                <div class="banner-text">
                  <div class="banner-title">{{ t("simverse.gachaTitle") }}</div>
                  <div class="banner-desc">{{ t("simverse.gachaDesc") }}</div>
                </div>
              </div>
              <div class="gacha-actions">
                <button class="gacha-action-btn single" @click="doGacha(1)">
                  <span class="action-icon">🎴</span>
                  <span class="action-name">{{ t("simverse.singlePull") }}</span>
                  <span class="action-cost">100 💎</span>
                </button>
                <button class="gacha-action-btn ten" @click="doGacha(10)">
                  <span class="action-icon">🎴×10</span>
                  <span class="action-name">{{ t("simverse.tenPull") }}</span>
                  <span class="action-cost">900 💎</span>
                  <span class="action-badge">{{ t("simverse.guaranteedRare") }}</span>
                </button>
              </div>
              <div v-if="gachaResults.length > 0" class="gacha-results">
                <div class="results-header">{{ t("simverse.results") }}</div>
                <div class="results-grid">
                  <div v-for="(item, i) in gachaResults" :key="i" class="result-item" :class="item.rarity">
                    <div class="result-icon">{{ item.icon }}</div>
                    <div class="result-name">{{ item.name }}</div>
                    <div class="result-rarity">{{ item.rarity }}</div>
                  </div>
                </div>
              </div>
            </template>

            <template v-else-if="activePanel === 'logs'">
              <div class="logs-panel">
                <div class="logs-tabs">
                  <button class="log-tab-btn" :class="{ active: logTab === 'frontend' }" @click="logTab = 'frontend'">
                    前端
                  </button>
                  <button class="log-tab-btn" :class="{ active: logTab === 'backend' }" @click="logTab = 'backend'">
                    后端
                  </button>
                </div>
                <div class="log-list-container">
                  <div v-if="logTab === 'frontend' && frontendLogs.length === 0" class="empty-state">
                    暂无前端日志
                  </div>
                  <div v-else-if="logTab === 'backend' && backendWorldLogs.length === 0" class="empty-state">
                    暂无后端日志
                  </div>
                  <div v-else class="log-list">
                    <div v-for="log in logTab === 'frontend' ? frontendLogs : backendWorldLogs" :key="log.id" class="log-item" :class="'log-' + log.level">
                      <span class="log-time">[{{ log.timestamp }}]</span>
                      <span class="log-level">{{ log.level.toUpperCase() }}</span>
                      <span class="log-msg">{{ log.message }}</span>
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

        <div v-if="selectedNPC" class="detail-modal" @click.self="selectedNPC = null">
          <div class="detail-card">
            <div class="detail-header">
              <div class="detail-avatar">{{ selectedNPC.name?.[0] }}</div>
              <div class="detail-info">
                <div class="detail-name">{{ selectedNPC.name }}</div>
                <div class="detail-meta">
                  {{ selectedNPC.species }} · {{ selectedNPC.gender }} · {{ selectedNPC.age }}岁
                </div>
              </div>
              <button class="detail-close" @click="selectedNPC = null">✕</button>
            </div>
            <div class="detail-body">
              <div class="detail-grid">
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.profession") }}</span>
                  <span class="item-value">{{ selectedNPC.profession }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.level") }}</span>
                  <span class="item-value highlight">Lv.{{ selectedNPC.level }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.health") }}</span>
                  <span class="item-value success">{{ selectedNPC.health }} / {{ selectedNPC.max_health }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.energy") }}</span>
                  <span class="item-value warning">{{ selectedNPC.energy }} / {{ selectedNPC.max_energy }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.wealthTier") }}</span>
                  <span class="item-value">{{ selectedNPC.wealth_tier }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.socialTier") }}</span>
                  <span class="item-value">{{ selectedNPC.social_tier }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.lifeStage") }}</span>
                  <span class="item-value">{{ selectedNPC.life_stage }}</span>
                </div>
                <div class="detail-item">
                  <span class="item-label">{{ t("simverse.alive") }}</span>
                  <span class="item-value" :class="{ alive: selectedNPC.is_alive, dead: !selectedNPC.is_alive }">
                    {{ selectedNPC.is_alive ? t("common.on") : t("common.off") }}
                  </span>
                </div>
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
import { useFrontendLogs, type LogEntry } from "@encv/shared-components/composables/useFrontendLogs";
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
const logTab = ref<"frontend" | "backend">("frontend");
const { logs: frontendLogs } = useFrontendLogs();
const backendWorldLogs = ref<LogEntry[]>([]);
let backendLogPollInterval: number | null = null;
let lastWorldLogId = "";

let pollInterval: number | null = null;

const visibleNPCs = computed(() => npcList.value.slice(0, 12));

const panelOnLeft = computed(() => {
  const leftPanels = ['npc', 'org', 'economy', 'chronicles'];
  return leftPanels.includes(activePanel.value || '');
});

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
    logs: "日志",
    economy: t("simverse.economy"),
    intervention: t("simverse.intervention"),
    debug: t("simverse.debug"),
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
  backendLogPollInterval = window.setInterval(() => {
    if (activePanel.value === "logs" && logTab.value === "backend") {
      loadBackendWorldLogs();
    }
  }, 5000);
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval);
    pollInterval = null;
  }
  if (backendLogPollInterval) {
    clearInterval(backendLogPollInterval);
    backendLogPollInterval = null;
  }
}

function chronicleLevelToLogLevel(level: string): string {
  if (level === "critical" || level === "catastrophe") return "error";
  if (level === "major" || level === "minor") return "warn";
  if (level === "trivial") return "debug";
  return "info";
}

async function loadBackendWorldLogs() {
  try {
    const data = await loadChronicleWorld(0, 50);
    const events = data?.items || [];
    const newLogs: LogEntry[] = [];
    let logId = backendWorldLogs.value.length;

    for (const evt of events) {
      const evtId = String(evt.id || "");
      if (evtId && evtId === lastWorldLogId) break;

      const level = chronicleLevelToLogLevel(evt.level || evt.imp_name || "info");
      const tags = ["simverse", "chronicle", evt.type || "event"];

      newLogs.push({
        id: ++logId,
        timestamp: new Date().toLocaleTimeString(),
        level,
        message: `[Tick ${evt.tick}] ${evt.type_cn || evt.type}: ${(evt as any).data_tag || "(world event)"}`,
        source: "simverse.chronicle",
        tags,
      });
    }

    if (newLogs.length > 0) {
      backendWorldLogs.value = [...newLogs.reverse(), ...backendWorldLogs.value].slice(-500);
      if (events.length > 0) {
        lastWorldLogId = String(events[0].id || "");
      }
    }
  } catch (e) {
    // 静默失败
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
  await loadBackendWorldLogs();
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
  --background: #0a0a1a;
}

.world-content {
  --background: #0a0a1a;
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
  background: 
    radial-gradient(ellipse at 30% 20%, rgba(124, 58, 237, 0.15) 0%, transparent 50%),
    radial-gradient(ellipse at 70% 80%, rgba(236, 72, 153, 0.1) 0%, transparent 50%),
    linear-gradient(180deg, #12122a 0%, #0a0a1a 100%);
}

.world-map {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  padding: 60px 100px 70px;
}

.map-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  grid-template-rows: repeat(6, 1fr);
  gap: 4px;
  height: 100%;
  opacity: 0.5;
}

.map-cell {
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  transition: all 0.3s;
}

.map-cell.forest { background: rgba(34, 197, 94, 0.12); }
.map-cell.mountain { background: rgba(107, 114, 128, 0.18); }
.map-cell.water { background: rgba(59, 130, 246, 0.12); }
.map-cell.plain { background: rgba(234, 179, 8, 0.08); }
.map-cell.village { background: rgba(139, 92, 246, 0.12); }
.map-cell.city { background: rgba(236, 72, 153, 0.12); }
.map-cell.desert { background: rgba(249, 115, 22, 0.08); }

.map-overlay {
  position: absolute;
  top: 60px;
  left: 100px;
  right: 100px;
  bottom: 70px;
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
  align-items: center;
  padding: 10px 16px;
  background: linear-gradient(180deg, rgba(0, 0, 0, 0.7) 0%, transparent 100%);
  z-index: 10;
}

.resource-group {
  display: flex;
  gap: 8px;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  padding: 6px 12px;
  border-radius: 20px;
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-bottom: 3px solid rgba(139, 92, 246, 0.3);
}

.resource-icon {
  font-size: 14px;
}

.resource-value {
  font-size: 13px;
  font-weight: 700;
  color: #fff;
  font-variant-numeric: tabular-nums;
}

.top-actions {
  display: flex;
  gap: 6px;
}

.game-btn {
  width: 38px;
  height: 38px;
  border-radius: 50%;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.25);
  border-bottom: 3px solid rgba(139, 92, 246, 0.35);
  color: #fff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1);
  padding: 0;
}

.game-btn:hover {
  background: rgba(139, 92, 246, 0.2);
  transform: translateY(-1px);
}

.game-btn:active {
  transform: translateY(2px);
  border-bottom-width: 1px;
}

.game-btn .btn-icon {
  font-size: 14px;
  line-height: 1;
}

.game-btn.play-btn.running {
  background: rgba(34, 197, 94, 0.2);
  border-color: rgba(34, 197, 94, 0.4);
  border-bottom-color: rgba(34, 197, 94, 0.5);
}

.left-menu,
.right-menu {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 10;
  padding: 10px 8px;
}

.left-menu {
  left: 10px;
}

.right-menu {
  right: 10px;
}

.menu-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-bottom: 3px solid rgba(139, 92, 246, 0.3);
  border-radius: 12px;
  cursor: pointer;
  padding: 10px 8px;
  min-width: 56px;
  transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1);
}

.menu-btn:hover {
  background: rgba(139, 92, 246, 0.2);
  transform: translateY(-1px);
}

.menu-btn:active {
  transform: translateY(2px);
  border-bottom-width: 1px;
}

.menu-btn.active {
  background: rgba(139, 92, 246, 0.3);
  border-color: rgba(139, 92, 246, 0.6);
  border-bottom-color: rgba(139, 92, 246, 0.7);
}

.menu-icon {
  font-size: 22px;
  filter: drop-shadow(0 2px 4px rgba(0, 0, 0, 0.3));
  line-height: 1;
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

.menu-btn.gacha-btn .menu-label {
  color: #ffd700;
  font-weight: 600;
}

.menu-btn.exit-btn .menu-label {
  color: #ef4444;
}

.menu-divider {
  width: 40px;
  height: 1px;
  background: rgba(139, 92, 246, 0.2);
  margin: 4px auto;
}

.stats-bar {
  position: absolute;
  bottom: 12px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 8px;
  z-index: 9;
}

.stat-pill {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(20, 20, 40, 0.7);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.15);
  border-radius: 16px;
  padding: 5px 12px;
}

.stat-label {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
}

.stat-value {
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  font-variant-numeric: tabular-nums;
}

.event-ticker {
  position: absolute;
  top: 58px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 10px;
  background: rgba(20, 20, 40, 0.8);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(139, 92, 246, 0.2);
  border-radius: 20px;
  padding: 6px 14px;
  z-index: 9;
  max-width: 400px;
}

.ticker-icon {
  font-size: 14px;
  flex-shrink: 0;
}

.ticker-content {
  overflow: hidden;
  flex: 1;
  min-width: 0;
}

.ticker-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ticker-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.7);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ticker-enter-active,
.ticker-leave-active {
  transition: all 0.3s ease;
}

.ticker-enter-from {
  opacity: 0;
  transform: translateY(-10px);
}

.ticker-leave-to {
  opacity: 0;
  transform: translateY(10px);
}

.event-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #4ade80;
  flex-shrink: 0;
}

.event-dot.warning { background: #f59e0b; }
.event-dot.danger { background: #ef4444; }

.event-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.side-panel {
  position: absolute;
  top: 50px;
  bottom: 60px;
  width: 300px;
  background: rgba(15, 15, 35, 0.95);
  backdrop-filter: blur(20px);
  border-radius: 16px;
  border: 1px solid rgba(139, 92, 246, 0.2);
  z-index: 15;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  right: 80px;
  transform: translateX(20px);
  opacity: 0;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  pointer-events: none;
}

.side-panel.panel-left {
  left: 80px;
  right: auto;
  transform: translateX(-20px);
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
  border-bottom: 1px solid rgba(139, 92, 246, 0.1);
  flex-shrink: 0;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.1) 0%, rgba(236, 72, 153, 0.05) 100%);
}

.panel-title {
  font-size: 15px;
  font-weight: 700;
  color: #fff;
}

.panel-close-btn {
  width: 28px;
  height: 28px;
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

.panel-close-btn:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.panel-content::-webkit-scrollbar {
  width: 5px;
}

.panel-content::-webkit-scrollbar-track {
  background: transparent;
}

.panel-content::-webkit-scrollbar-thumb {
  background: rgba(139, 92, 246, 0.25);
  border-radius: 3px;
}

.list-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 12px;
  margin-bottom: 8px;
  border: 1px solid rgba(139, 92, 246, 0.1);
  transition: all 0.2s;
  cursor: pointer;
}

.list-card:hover {
  background: rgba(139, 92, 246, 0.1);
  border-color: rgba(139, 92, 246, 0.25);
  transform: translateX(2px);
}

.card-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 16px;
  flex-shrink: 0;
  border: 2px solid rgba(255, 255, 255, 0.1);
}

.card-info {
  flex: 1;
  min-width: 0;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  margin-bottom: 4px;
}

.card-subtitle {
  display: flex;
  gap: 6px;
  align-items: center;
}

.prof-tag,
.level-tag {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 6px;
  font-weight: 500;
}

.prof-tag {
  background: rgba(139, 92, 246, 0.2);
  color: #c4b5fd;
  text-transform: capitalize;
}

.level-tag {
  background: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
}

.card-action {
  width: 50px;
  flex-shrink: 0;
}

.mini-hp-bar {
  width: 100%;
  height: 5px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 3px;
  overflow: hidden;
}

.mini-hp-fill {
  height: 100%;
  background: linear-gradient(90deg, #22c55e, #4ade80);
  border-radius: 3px;
  transition: width 0.3s;
}

.empty-state {
  text-align: center;
  padding: 30px 16px;
  color: rgba(255, 255, 255, 0.3);
  font-size: 12px;
}

.chronicle-card {
  padding: 12px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 12px;
  margin-bottom: 8px;
  border-left: 3px solid #8b5cf6;
  border: 1px solid rgba(139, 92, 246, 0.1);
  border-left: 3px solid #8b5cf6;
}

.chronicle-tick {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
  margin-bottom: 4px;
  font-variant-numeric: tabular-nums;
}

.chronicle-title {
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  margin-bottom: 4px;
}

.chronicle-desc {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  line-height: 1.4;
}

.setting-section {
  margin-bottom: 20px;
}

.section-title {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.6);
  margin-bottom: 10px;
  padding-left: 2px;
  letter-spacing: 0.3px;
}

.tier-selector {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tier-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid rgba(139, 92, 246, 0.2);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
}

.tier-option:hover {
  background: rgba(139, 92, 246, 0.1);
  border-color: rgba(139, 92, 246, 0.35);
}

.tier-option.active {
  background: rgba(139, 92, 246, 0.2);
  border-color: rgba(139, 92, 246, 0.6);
  color: #fff;
}

.tier-name {
  font-weight: 600;
}

.tier-desc {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.4);
}

.tier-option.active .tier-desc {
  color: rgba(255, 255, 255, 0.6);
}

.config-list {
  background: rgba(255, 255, 255, 0.03);
  border-radius: 10px;
  border: 1px solid rgba(139, 92, 246, 0.1);
  overflow: hidden;
}

.config-row {
  display: flex;
  justify-content: space-between;
  padding: 10px 14px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(139, 92, 246, 0.05);
}

.config-row:last-child {
  border-bottom: none;
}

.config-value {
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.gacha-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.25), rgba(236, 72, 153, 0.2));
  border-radius: 14px;
  padding: 16px;
  border: 1px solid rgba(139, 92, 246, 0.35);
  margin-bottom: 14px;
}

.banner-icon {
  font-size: 36px;
  flex-shrink: 0;
}

.banner-text {
  flex: 1;
  min-width: 0;
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
  line-height: 1.4;
}

.gacha-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 14px;
}

.gacha-action-btn {
  position: relative;
  padding: 14px 16px;
  border-radius: 12px;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 12px;
  transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1);
  border-bottom: 4px solid rgba(0, 0, 0, 0.2);
}

.gacha-action-btn.single {
  background: linear-gradient(135deg, #3b82f6, #6366f1);
}

.gacha-action-btn.ten {
  background: linear-gradient(135deg, #f59e0b, #ef4444);
}

.gacha-action-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.1);
}

.gacha-action-btn:active {
  transform: translateY(3px);
  border-bottom-width: 1px;
}

.action-icon {
  font-size: 24px;
  flex-shrink: 0;
}

.action-name {
  flex: 1;
  text-align: left;
  font-size: 14px;
  font-weight: 700;
  color: #fff;
}

.action-cost {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.95);
  font-weight: 600;
}

.action-badge {
  position: absolute;
  top: -6px;
  right: 12px;
  background: #ffd700;
  color: #1a1a1a;
  font-size: 9px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 8px;
}

.gacha-results {
  margin-top: 4px;
}

.results-header {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 10px;
}

.results-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
}

.result-item {
  aspect-ratio: 3 / 4;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(139, 92, 246, 0.15);
  animation: cardReveal 0.4s ease-out;
  padding: 6px 4px;
}

@keyframes cardReveal {
  from { opacity: 0; transform: scale(0.8) rotateY(90deg); }
  to { opacity: 1; transform: scale(1) rotateY(0); }
}

.result-item .result-icon { font-size: 22px; }

.result-item .result-name {
  font-size: 8px;
  color: rgba(255, 255, 255, 0.6);
  text-align: center;
  line-height: 1.2;
}

.result-item .result-rarity {
  font-size: 9px;
  font-weight: 700;
}

.result-item.N .result-rarity { color: #9ca3af; }
.result-item.R .result-rarity { color: #3b82f6; }
.result-item.SR .result-rarity { color: #a78bfa; }
.result-item.SSR .result-rarity { color: #fbbf24; }
.result-item.UR .result-rarity { color: #f87171; }

.detail-modal {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 20;
  backdrop-filter: blur(4px);
}

.detail-card {
  width: 85%;
  max-width: 360px;
  background: rgba(15, 15, 35, 0.98);
  border-radius: 18px;
  border: 1px solid rgba(139, 92, 246, 0.25);
  overflow: hidden;
  animation: modalIn 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes modalIn {
  from {
    opacity: 0;
    transform: scale(0.9) translateY(20px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.detail-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px;
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.2), rgba(236, 72, 153, 0.15));
  border-bottom: 1px solid rgba(139, 92, 246, 0.1);
  position: relative;
}

.detail-avatar {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #ec4899);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 24px;
  flex-shrink: 0;
  border: 3px solid rgba(255, 255, 255, 0.15);
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.detail-name {
  font-size: 20px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 4px;
}

.detail-meta {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.detail-close {
  position: absolute;
  top: 14px;
  right: 14px;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  border: none;
  color: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.detail-close:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.detail-body {
  padding: 16px 18px 18px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px 16px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
}

.item-value {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.9);
  font-weight: 500;
}

.item-value.highlight {
  color: #fbbf24;
  font-weight: 600;
}

.item-value.success {
  color: #4ade80;
}

.item-value.warning {
  color: #fbbf24;
}

.item-value.alive { color: #4ade80; }
.item-value.dead { color: #f87171; }

.logs-panel {
  display: flex;
  flex-direction: column;
  height: 420px;
  margin: -12px;
}

.logs-tabs {
  display: flex;
  gap: 4px;
  padding: 8px;
  border-bottom: 1px solid rgba(139, 92, 246, 0.1);
}

.log-tab-btn {
  flex: 1;
  padding: 8px 12px;
  border: none;
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.5);
  border-radius: 8px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.log-tab-btn:hover {
  background: rgba(139, 92, 246, 0.1);
  color: rgba(255, 255, 255, 0.7);
}

.log-tab-btn.active {
  background: rgba(139, 92, 246, 0.2);
  color: #fff;
  border: 1px solid rgba(139, 92, 246, 0.3);
}

.log-list-container {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.log-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.log-item {
  display: flex;
  gap: 6px;
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  line-height: 1.4;
}

.log-item:hover {
  background: rgba(255, 255, 255, 0.04);
}

.log-time {
  color: rgba(255, 255, 255, 0.3);
  flex-shrink: 0;
}

.log-level {
  font-weight: 700;
  font-size: 9px;
  text-transform: uppercase;
  flex-shrink: 0;
  min-width: 36px;
}

.log-info .log-level { color: #60a5fa; }
.log-warn .log-level { color: #fbbf24; }
.log-error .log-level { color: #f87171; }
.log-debug .log-level { color: #9ca3af; }

.log-msg {
  color: rgba(255, 255, 255, 0.7);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-list-container::-webkit-scrollbar {
  width: 4px;
}

.log-list-container::-webkit-scrollbar-thumb {
  background: rgba(139, 92, 246, 0.2);
  border-radius: 2px;
}
</style>
