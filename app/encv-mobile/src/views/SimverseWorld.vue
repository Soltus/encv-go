<template>
  <div class="simverse-world">
    <header class="top-bar">
      <div class="left">
        <button class="icon-btn" @click="toggleMenu" :title="menuOpen ? 'Close Menu' : 'Open Menu'">
          <span class="menu-icon">☰</span>
        </button>
        <div class="world-name">
          <span class="name">{{ worldName }}</span>
          <span class="era" v-if="worldState">Era {{ worldState.tier }}</span>
        </div>
      </div>
      <div class="center">
        <div class="tick-display" v-if="worldState">
          <span class="tick-label">TICK</span>
          <span class="tick-value">{{ worldState.tick.toLocaleString() }}</span>
        </div>
      </div>
      <div class="right">
        <div class="status-indicator" :class="{ running: worldState?.running, paused: !worldState?.running }">
          <span class="dot"></span>
          <span class="label">{{ worldState?.running ? 'Running' : 'Paused' }}</span>
        </div>
        <button class="icon-btn" @click="exitWorld" title="Exit">
          <span>✕</span>
        </button>
      </div>
    </header>

    <div class="main-area">
      <aside class="left-panel">
        <div class="panel-section">
          <h3>World Stats</h3>
          <div class="stat-row" v-if="worldState">
            <span class="stat-label">NPCs</span>
            <span class="stat-value">{{ worldState.npc_count }}</span>
          </div>
          <div class="stat-row" v-if="worldState">
            <span class="stat-label">Focus</span>
            <span class="stat-value">{{ worldState.focus_count }}</span>
          </div>
          <div class="stat-row" v-if="worldState">
            <span class="stat-label">Cells</span>
            <span class="stat-value">{{ worldState.cell_count }}</span>
          </div>
          <div class="stat-row" v-if="worldState">
            <span class="stat-label">Memory</span>
            <span class="stat-value">{{ worldState.total_mb.toFixed(1) }} MB</span>
          </div>
        </div>

        <div class="panel-section">
          <h3>Storage</h3>
          <div class="stat-row" v-if="storageStatus">
            <span class="stat-label">Level</span>
            <span class="stat-value storage-level" :class="storageStatus.level">{{ storageStatus.level }}</span>
          </div>
          <div class="stat-row" v-if="storageStatus">
            <span class="stat-label">Used</span>
            <span class="stat-value">{{ formatBytes(storageStatus.used_bytes) }}</span>
          </div>
          <div class="stat-row" v-if="storageStatus">
            <span class="stat-label">Free</span>
            <span class="stat-value">{{ formatBytes(storageStatus.available_bytes) }}</span>
          </div>
        </div>

        <div class="panel-section" v-if="shortcutSupported">
          <h3>Shortcut</h3>
          <button class="shortcut-btn" @click="handleAddShortcut">
            <span>＋</span>
            <span>添加桌面快捷方式</span>
          </button>
        </div>
      </aside>

      <main class="center-panel">
        <div class="tab-bar">
          <button class="tab-btn" :class="{ active: activeTab === 'world' }" @click="activeTab = 'world'">World</button>
          <button class="tab-btn" :class="{ active: activeTab === 'npcs' }" @click="activeTab = 'npcs'">NPCs</button>
          <button class="tab-btn" :class="{ active: activeTab === 'chronicle' }" @click="activeTab = 'chronicle'">Chronicle</button>
        </div>

        <div class="tab-content">
          <div v-if="activeTab === 'world'" class="world-tab">
            <div ref="worldCanvasRef" class="world-canvas"></div>
            <div class="world-hint">
              <span>🖱 点击 NPC 查看详情</span>
              <span>⚡ Matter.js 物理模拟中</span>
            </div>
          </div>

          <div v-if="activeTab === 'npcs'" class="npcs-tab">
            <div class="npc-grid">
              <div
                v-for="npc in npcs.slice(0, 16)"
                :key="npc.id"
                class="npc-card"
                @click="selectNPC(npc)"
              >
                <div class="npc-avatar" :class="npc.gender">
                  {{ npc.name.charAt(0) }}
                </div>
                <div class="npc-info">
                  <div class="npc-name">{{ npc.name }}</div>
                  <div class="npc-role">{{ npc.profession }} · Lv.{{ npc.level }}</div>
                </div>
                <div class="npc-hp-bar">
                  <div class="hp-fill" :style="{ width: (npc.health / npc.max_health * 100) + '%' }"></div>
                </div>
              </div>
            </div>
            <div v-if="npcs.length === 0" class="empty-state">No NPCs loaded</div>
          </div>

          <div v-if="activeTab === 'chronicle'" class="chronicle-tab">
            <div class="chronicle-list">
              <div
                v-for="ev in chronicleEvents.slice(0, 30)"
                :key="ev.id"
                class="chronicle-item"
              >
                <div class="chronicle-tick">T{{ ev.tick }}</div>
                <div class="chronicle-category" v-if="ev.category">{{ ev.category }}</div>
                <div class="chronicle-text">{{ ev.event_text || ev.description || '' }}</div>
              </div>
            </div>
          </div>
        </div>
      </main>

      <aside class="right-panel" v-if="selectedNPC">
        <div class="panel-section">
          <div class="npc-detail-header">
            <div class="npc-avatar large" :class="selectedNPC.gender">
              {{ selectedNPC.name.charAt(0) }}
            </div>
            <div>
              <h3>{{ selectedNPC.name }}</h3>
              <p class="npc-subtitle">{{ selectedNPC.species }} · {{ selectedNPC.gender }} · {{ selectedNPC.age }}y</p>
            </div>
          </div>
        </div>
        <div class="panel-section">
          <h4>Stats</h4>
          <div class="stat-row">
            <span class="stat-label">Level</span>
            <span class="stat-value">{{ selectedNPC.level }}</span>
          </div>
          <div class="stat-row">
            <span class="stat-label">Health</span>
            <span class="stat-value">{{ selectedNPC.health }} / {{ selectedNPC.max_health }}</span>
          </div>
          <div class="stat-row">
            <span class="stat-label">Energy</span>
            <span class="stat-value">{{ selectedNPC.energy }} / {{ selectedNPC.max_energy }}</span>
          </div>
          <div class="stat-row">
            <span class="stat-label">Profession</span>
            <span class="stat-value">{{ selectedNPC.profession }}</span>
          </div>
          <div class="stat-row">
            <span class="stat-label">Life Stage</span>
            <span class="stat-value">{{ selectedNPC.life_stage }}</span>
          </div>
          <div class="stat-row" v-if="selectedNPC.wealth_tier !== undefined">
            <span class="stat-label">Wealth Tier</span>
            <span class="stat-value">{{ selectedNPC.wealth_tier }}</span>
          </div>
        </div>
        <button class="close-detail" @click="selectedNPC = null">Close</button>
      </aside>
    </div>

    <div class="bottom-bar">
      <div class="controls">
        <button class="control-btn primary" @click="togglePause">
          {{ worldState?.running ? '⏸ Pause' : '▶ Play' }}
        </button>
        <button class="control-btn" @click="stepTick" :disabled="worldState?.running">
          ⏭ Step
        </button>
        <button class="control-btn" @click="saveCheckpoint">
          💾 Save
        </button>
        <button class="control-btn" @click="loadCheckpoint">
          📂 Load
        </button>
      </div>
      <div class="perf-info" v-if="worldState">
        <span>Speed: {{ worldState.tier_name }}</span>
        <span>Brain: {{ worldState.brain_count }}</span>
      </div>
    </div>

    <div class="side-menu" :class="{ open: menuOpen }" @click.self="toggleMenu">
      <div class="menu-panel">
        <h2>Menu</h2>
        <ul class="menu-list">
          <li @click="menuOpen = false; activeTab = 'world'">
            <span class="menu-icon">🗺</span>
            <span>World Map</span>
          </li>
          <li @click="menuOpen = false; activeTab = 'npcs'">
            <span class="menu-icon">👥</span>
            <span>NPCs</span>
          </li>
          <li @click="menuOpen = false; activeTab = 'chronicle'">
            <span class="menu-icon">📜</span>
            <span>Chronicle</span>
          </li>
          <li class="divider"></li>
          <li @click="openSettings">
            <span class="menu-icon">⚙</span>
            <span>Settings</span>
          </li>
          <li @click="openDevLogs">
            <span class="menu-icon">📋</span>
            <span>Dev Logs</span>
          </li>
          <li @click="toggleFullscreen">
            <span class="menu-icon">⛶</span>
            <span>Fullscreen</span>
          </li>
          <li class="divider"></li>
          <li class="danger" @click="exitWorld">
            <span class="menu-icon">🚪</span>
            <span>Exit World</span>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { closeWorld, addWorldShortcut, isWorldShortcutSupported } from '@/plugins/SimVerse'
import { useWorldRenderer, type WorldEntity } from '@/composables/useWorldRenderer'

const worldName = ref('SimVerse')
const worldState = ref<any>(null)
const storageStatus = ref<any>(null)
const npcs = ref<any[]>([])
const chronicleEvents = ref<any[]>([])
const recentEvents = ref<any[]>([])
const selectedNPC = ref<any>(null)
const activeTab = ref('world')
const menuOpen = ref(false)
const shortcutSupported = ref(false)
const worldCanvasRef = ref<HTMLElement | null>(null)

let renderer: ReturnType<typeof useWorldRenderer> | null = null
let initialized = false

const WORLD_WIDTH = 2000
const WORLD_HEIGHT = 2000
const NPC_COLORS = ['#4f8cff', '#8b5cf6', '#22c55e', '#f97316', '#ef4444', '#ec4899', '#14b8a6']

function npcToEntity(npc: any): WorldEntity {
  const color = NPC_COLORS[Math.abs(hashCode(npc.id)) % NPC_COLORS.length]
  const x = 100 + Math.abs(hashCode(npc.id + 'x')) % (WORLD_WIDTH - 200)
  const y = 100 + Math.abs(hashCode(npc.id + 'y')) % (WORLD_HEIGHT - 200)
  return {
    id: npc.id,
    type: 'npc',
    x,
    y,
    width: 24,
    height: 24,
    color,
    label: npc.name,
    static: false,
    onClick: () => {
      selectedNPC.value = npc
    },
  }
}

function hashCode(str: string): number {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i)
    hash = ((hash << 5) - hash) + char
    hash = hash & hash
  }
  return hash
}

function generateStaticEntities(): WorldEntity[] {
  const entities: WorldEntity[] = []
  for (let i = 0; i < 15; i++) {
    entities.push({
      id: `tree-${i}`,
      type: 'tree',
      x: 200 + (i * 137) % (WORLD_WIDTH - 400),
      y: 200 + (i * 211) % (WORLD_HEIGHT - 400),
      width: 32,
      height: 48,
      color: '#166534',
      static: true,
    })
  }
  for (let i = 0; i < 8; i++) {
    entities.push({
      id: `rock-${i}`,
      type: 'rock',
      x: 300 + (i * 193) % (WORLD_WIDTH - 600),
      y: 300 + (i * 277) % (WORLD_HEIGHT - 600),
      width: 40,
      height: 30,
      color: '#6b7280',
      static: true,
    })
  }
  for (let i = 0; i < 5; i++) {
    entities.push({
      id: `building-${i}`,
      type: 'building',
      x: 500 + (i * 300) % (WORLD_WIDTH - 700),
      y: 500 + (i * 400) % (WORLD_HEIGHT - 700),
      width: 80,
      height: 60,
      color: '#7c3aed',
      static: true,
    })
  }
  entities.push({
    id: 'ground',
    type: 'ground',
    x: 0,
    y: WORLD_HEIGHT - 40,
    width: WORLD_WIDTH,
    height: 40,
    color: '#1e3a5f',
    static: true,
  })
  entities.push({
    id: 'water',
    type: 'water',
    x: WORLD_WIDTH / 2 - 150,
    y: WORLD_HEIGHT / 2 - 80,
    width: 300,
    height: 160,
    color: '#0369a1',
    static: true,
  })
  return entities
}

async function initRenderer() {
  if (initialized || !worldCanvasRef.value) return
  initialized = true

  renderer = useWorldRenderer({
    container: worldCanvasRef.value,
    worldWidth: WORLD_WIDTH,
    worldHeight: WORLD_HEIGHT,
    gravity: 0,
  })

  renderer.init()

  for (const ent of generateStaticEntities()) {
    renderer.addEntity(ent)
  }

  for (const npc of npcs.value) {
    renderer.addEntity(npcToEntity(npc))
    if (Math.random() > 0.3) {
      setTimeout(() => {
        renderer?.setVelocity(npc.id, (Math.random() - 0.5) * 2, (Math.random() - 0.5) * 2)
      }, Math.random() * 2000)
    }
  }

  setInterval(() => {
    if (!renderer) return
    for (const npc of npcs.value) {
      if (Math.random() > 0.92) {
        renderer.setVelocity(npc.id, (Math.random() - 0.5) * 3, (Math.random() - 0.5) * 3)
      }
    }
  }, 3000)
}

watch(npcs, (newNpcs, oldNpcs) => {
  if (!renderer) return
  const oldIds = new Set(oldNpcs.map(n => n.id))
  for (const npc of newNpcs) {
    if (!oldIds.has(npc.id)) {
      renderer.addEntity(npcToEntity(npc))
    }
  }
  const newIds = new Set(newNpcs.map(n => n.id))
  for (const npc of oldNpcs) {
    if (!newIds.has(npc.id)) {
      renderer.removeEntity(npc.id)
    }
  }
}, { deep: true })

watch(activeTab, async (tab) => {
  if (tab === 'world') {
    await nextTick()
    initRenderer()
  }
})

let pollTimer: number | null = null

async function fetchWorldState() {
  try {
    const r = await fetch('/api/simverse/world/state')
    if (r.ok) {
      worldState.value = await r.json()
    }
  } catch (e) {
    console.warn('fetchWorldState failed', e)
  }
}

async function fetchStorageStatus() {
  try {
    const r = await fetch('/api/simverse/world/storage')
    if (r.ok) {
      storageStatus.value = await r.json()
    }
  } catch (e) {
    console.warn('fetchStorageStatus failed', e)
  }
}

async function fetchNPCs() {
  try {
    const r = await fetch('/api/simverse/npcs?limit=20')
    if (r.ok) {
      const data = await r.json()
      npcs.value = data.npcs || data.items || []
    }
  } catch (e) {
    console.warn('fetchNPCs failed', e)
  }
}

async function fetchChronicle() {
  try {
    const r = await fetch('/api/simverse/chronicle/world?limit=30')
    if (r.ok) {
      const data = await r.json()
      chronicleEvents.value = data.events || data.items || []
      recentEvents.value = data.events?.slice(0, 8) || []
    }
  } catch (e) {
    console.warn('fetchChronicle failed', e)
  }
}

async function togglePause() {
  try {
    const action = worldState.value?.running ? 'pause' : 'resume'
    await fetch(`/api/simverse/world/${action}`, { method: 'POST' })
    setTimeout(fetchWorldState, 200)
  } catch (e) {
    console.warn('togglePause failed', e)
  }
}

async function stepTick() {
  try {
    await fetch('/api/simverse/world/step', { method: 'POST' })
    setTimeout(fetchWorldState, 200)
  } catch (e) {
    console.warn('stepTick failed', e)
  }
}

async function saveCheckpoint() {
  try {
    await fetch('/api/simverse/world/save', { method: 'POST' })
  } catch (e) {
    console.warn('saveCheckpoint failed', e)
  }
}

async function loadCheckpoint() {
  try {
    await fetch('/api/simverse/world/load', { method: 'POST' })
    setTimeout(() => {
      fetchWorldState()
      fetchNPCs()
      fetchChronicle()
    }, 200)
  } catch (e) {
    console.warn('loadCheckpoint failed', e)
  }
}

function selectNPC(npc: any) {
  selectedNPC.value = npc
}

function toggleMenu() {
  menuOpen.value = !menuOpen.value
}

function exitWorld() {
  closeWorld()
  if (window.history.length > 1) {
    window.history.back()
  }
}

function openSettings() {
  menuOpen.value = false
}

function openDevLogs() {
  menuOpen.value = false
}

function toggleFullscreen() {
  menuOpen.value = false
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen()
  } else {
    document.exitFullscreen()
  }
}

async function handleAddShortcut() {
  await addWorldShortcut()
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function startPolling() {
  pollTimer = window.setInterval(() => {
    fetchWorldState()
    if (activeTab.value === 'npcs') fetchNPCs()
    if (activeTab.value === 'chronicle') fetchChronicle()
    if (Math.random() > 0.8) fetchStorageStatus()
  }, 2000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

let resizeObserver: ResizeObserver | null = null

onMounted(async () => {
  await fetchWorldState()
  await fetchStorageStatus()
  await fetchNPCs()
  await fetchChronicle()
  startPolling()
  shortcutSupported.value = await isWorldShortcutSupported()

  await nextTick()
  initRenderer()

  resizeObserver = new ResizeObserver(() => {
    renderer?.resize()
  })
  if (worldCanvasRef.value) {
    resizeObserver.observe(worldCanvasRef.value)
  }
})

onUnmounted(() => {
  stopPolling()
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (renderer) {
    renderer.destroy()
    renderer = null
    initialized = false
  }
})
</script>

<style scoped>
.simverse-world {
  width: 100vw;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: linear-gradient(135deg, #0a0a1a 0%, #1a1a3a 50%, #0f1629 100%);
  color: #e0e0ff;
  font-family: system-ui, -apple-system, sans-serif;
  overflow: hidden;
  user-select: none;
}

.top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  background: rgba(15, 20, 40, 0.9);
  border-bottom: 1px solid rgba(100, 120, 200, 0.2);
  backdrop-filter: blur(10px);
  flex-shrink: 0;
}

.left, .right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.center {
  flex: 1;
  display: flex;
  justify-content: center;
}

.icon-btn {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  border: 1px solid rgba(100, 120, 200, 0.3);
  background: rgba(30, 40, 70, 0.6);
  color: #e0e0ff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  transition: all 0.2s;
}
.icon-btn:hover {
  background: rgba(60, 80, 140, 0.6);
  border-color: rgba(120, 150, 220, 0.5);
}

.menu-icon {
  font-size: 18px;
}

.world-name {
  display: flex;
  flex-direction: column;
}
.world-name .name {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
}
.world-name .era {
  font-size: 11px;
  color: #8b9dc3;
}

.tick-display {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 4px 16px;
  background: rgba(0, 0, 0, 0.3);
  border-radius: 20px;
  border: 1px solid rgba(100, 120, 200, 0.2);
}
.tick-label {
  font-size: 10px;
  color: #6b7db3;
  letter-spacing: 2px;
  font-weight: 600;
}
.tick-value {
  font-size: 18px;
  font-weight: 700;
  font-family: 'SF Mono', Monaco, monospace;
  color: #8b9dc3;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}
.status-indicator.running {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}
.status-indicator.paused {
  background: rgba(249, 115, 22, 0.15);
  color: #f97316;
}
.status-indicator .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
  animation: pulse 2s infinite;
}
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.main-area {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.left-panel, .right-panel {
  width: 220px;
  background: rgba(15, 20, 40, 0.7);
  border: 1px solid rgba(100, 120, 200, 0.15);
  margin: 8px;
  border-radius: 12px;
  padding: 12px;
  overflow-y: auto;
  flex-shrink: 0;
}

.right-panel {
  width: 260px;
}

.panel-section {
  margin-bottom: 16px;
}
.panel-section h3, .panel-section h4 {
  font-size: 12px;
  font-weight: 600;
  color: #8b9dc3;
  text-transform: uppercase;
  letter-spacing: 1px;
  margin: 0 0 10px 0;
}

.stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 0;
  font-size: 13px;
  border-bottom: 1px solid rgba(100, 120, 200, 0.1);
}
.stat-row:last-child {
  border-bottom: none;
}
.stat-label {
  color: #6b7db3;
}
.stat-value {
  color: #e0e0ff;
  font-weight: 500;
}
.storage-level.normal { color: #22c55e; }
.storage-level.low { color: #f97316; }
.storage-level.critical { color: #ef4444; }

.shortcut-btn {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  background: linear-gradient(135deg, #4f8cff, #8b5cf6);
  border: none;
  border-radius: 8px;
  color: white;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: opacity 0.2s;
}
.shortcut-btn:hover {
  opacity: 0.9;
}

.center-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  margin: 8px 0;
  overflow: hidden;
}

.tab-bar {
  display: flex;
  gap: 4px;
  padding: 0 8px;
  margin-bottom: 8px;
}
.tab-btn {
  flex: 1;
  max-width: 140px;
  padding: 8px 16px;
  background: rgba(30, 40, 70, 0.5);
  border: 1px solid rgba(100, 120, 200, 0.2);
  border-bottom: none;
  color: #8b9dc3;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border-radius: 8px 8px 0 0;
  transition: all 0.2s;
}
.tab-btn.active {
  background: rgba(50, 70, 130, 0.6);
  color: #fff;
  border-color: rgba(120, 150, 220, 0.4);
}

.tab-content {
  flex: 1;
  background: rgba(15, 20, 40, 0.7);
  border: 1px solid rgba(100, 120, 200, 0.2);
  border-radius: 0 12px 12px 12px;
  margin: 0 8px 0 8px;
  overflow: hidden;
}

.world-tab {
  position: relative;
  height: 100%;
  width: 100%;
  overflow: hidden;
}
.world-canvas {
  width: 100%;
  height: 100%;
  cursor: grab;
}
.world-canvas:active {
  cursor: grabbing;
}
.world-hint {
  position: absolute;
  bottom: 12px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 16px;
  padding: 6px 14px;
  background: rgba(0, 0, 0, 0.5);
  border-radius: 12px;
  font-size: 11px;
  color: #8b9dc3;
  backdrop-filter: blur(4px);
  pointer-events: none;
}

.npcs-tab {
  height: 100%;
  padding: 12px;
  overflow-y: auto;
}
.npc-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}
.npc-card {
  background: rgba(30, 40, 70, 0.5);
  border: 1px solid rgba(100, 120, 200, 0.2);
  border-radius: 10px;
  padding: 10px;
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.npc-card:hover {
  background: rgba(50, 70, 130, 0.6);
  border-color: rgba(120, 150, 220, 0.4);
  transform: translateY(-1px);
}
.npc-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, #4f8cff, #8b5cf6);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-weight: 700;
  font-size: 16px;
}
.npc-avatar.large {
  width: 56px;
  height: 56px;
  font-size: 22px;
}
.npc-info {
  flex: 1;
}
.npc-name {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
}
.npc-role {
  font-size: 11px;
  color: #8b9dc3;
}
.npc-hp-bar {
  height: 4px;
  background: rgba(0, 0, 0, 0.3);
  border-radius: 2px;
  overflow: hidden;
}
.hp-fill {
  height: 100%;
  background: linear-gradient(90deg, #22c55e, #4ade80);
  border-radius: 2px;
  transition: width 0.3s;
}

.npc-detail-header {
  display: flex;
  gap: 12px;
  align-items: center;
}
.npc-detail-header h3 {
  margin: 0;
  font-size: 16px;
  color: #fff;
}
.npc-subtitle {
  margin: 2px 0 0 0;
  font-size: 11px;
  color: #6b7db3;
}

.close-detail {
  width: 100%;
  padding: 8px;
  background: rgba(30, 40, 70, 0.6);
  border: 1px solid rgba(100, 120, 200, 0.3);
  color: #8b9dc3;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.chronicle-tab {
  height: 100%;
  overflow-y: auto;
  padding: 12px;
}
.chronicle-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.chronicle-item {
  background: rgba(30, 40, 70, 0.4);
  border-left: 3px solid #4f8cff;
  padding: 8px 12px;
  border-radius: 0 6px 6px 0;
}
.chronicle-tick {
  font-size: 11px;
  color: #6b7db3;
  font-family: monospace;
  margin-bottom: 4px;
}
.chronicle-category {
  font-size: 10px;
  color: #8b5cf6;
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-bottom: 4px;
}
.chronicle-text {
  font-size: 13px;
  color: #c0c8e0;
  line-height: 1.5;
}

.bottom-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  background: rgba(15, 20, 40, 0.9);
  border-top: 1px solid rgba(100, 120, 200, 0.2);
  backdrop-filter: blur(10px);
  flex-shrink: 0;
}

.controls {
  display: flex;
  gap: 8px;
}
.control-btn {
  padding: 8px 16px;
  background: rgba(30, 40, 70, 0.6);
  border: 1px solid rgba(100, 120, 200, 0.3);
  color: #e0e0ff;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  transition: all 0.2s;
}
.control-btn:hover {
  background: rgba(60, 80, 140, 0.6);
}
.control-btn.primary {
  background: linear-gradient(135deg, #4f8cff, #8b5cf6);
  border: none;
  color: white;
}
.control-btn.primary:hover {
  opacity: 0.9;
}
.control-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.perf-info {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: #6b7db3;
}

.side-menu {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.6);
  z-index: 1000;
  opacity: 0;
  visibility: hidden;
  transition: all 0.3s;
}
.side-menu.open {
  opacity: 1;
  visibility: visible;
}
.menu-panel {
  position: absolute;
  top: 0;
  left: 0;
  width: 280px;
  height: 100%;
  background: rgba(15, 20, 40, 0.95);
  border-right: 1px solid rgba(100, 120, 200, 0.3);
  padding: 20px;
  transform: translateX(-100%);
  transition: transform 0.3s ease-out;
  backdrop-filter: blur(20px);
  overflow-y: auto;
}
.side-menu.open .menu-panel {
  transform: translateX(0);
}
.menu-panel h2 {
  font-size: 18px;
  color: #fff;
  margin: 0 0 20px 0;
}
.menu-list {
  list-style: none;
  padding: 0;
  margin: 0;
}
.menu-list li {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 8px;
  cursor: pointer;
  color: #c0c8e0;
  font-size: 14px;
  transition: all 0.2s;
}
.menu-list li:hover {
  background: rgba(60, 80, 140, 0.4);
  color: #fff;
}
.menu-list li.divider {
  height: 1px;
  padding: 0;
  margin: 8px 0;
  background: rgba(100, 120, 200, 0.2);
  cursor: default;
}
.menu-list li.divider:hover {
  background: rgba(100, 120, 200, 0.2);
}
.menu-list li.danger {
  color: #ef4444;
}
.menu-list li.danger:hover {
  background: rgba(239, 68, 68, 0.15);
}
.menu-icon {
  width: 20px;
  text-align: center;
}

.empty-state {
  text-align: center;
  color: #6b7db3;
  padding: 40px;
  font-size: 14px;
}

@media (max-width: 900px) {
  .left-panel, .right-panel {
    display: none;
  }
}
</style>
