#!/bin/bash
set -e
APP_ROOT="/tmp/workspace/87faec08-954f-4023-84e4-26de28f1e94e/app"
cd "$APP_ROOT"

echo "=== Step 1: Copy encv-mobile/src to shared-components ==="
mkdir -p packages/shared-components/src
cp -r encv-mobile/src/* packages/shared-components/src/ 2>/dev/null || true

echo "=== Step 2: Remove Simverse-specific and app-specific files from shared-components ==="
rm -f packages/shared-components/src/views/SimverseWorld.vue
rm -f packages/shared-components/src/composables/useSimverse.ts
rm -f packages/shared-components/src/composables/useWorldRenderer.ts
rm -f packages/shared-components/src/simverse-world-main.ts
rm -f packages/shared-components/src/main.ts
rm -f packages/shared-components/src/App.vue
rm -f packages/shared-components/src/router/index.ts

# Clean empty dirs that may have been left
find packages/shared-components/src -type d -empty -delete 2>/dev/null || true

echo "=== Step 3: Replace @/ with relative self-ref in shared-components ==="
# In shared-components, all files are under src/
# @/composables/x -> ../composables/x or ./composables/x depending on file depth
# For simplicity, we use @encv/shared-components/ for all shared internal imports.
# Self-referential imports in a workspace package resolve correctly via node_modules symlink.
find packages/shared-components/src -type f \( -name "*.ts" -o -name "*.vue" -o -name "*.js" \) -exec sed -i 's~from "@/~from "@encv/shared-components/~g' {} +
find packages/shared-components/src -type f \( -name "*.ts" -o -name "*.vue" -o -name "*.js" \) -exec sed -i "s~from '@/~from '@encv/shared-components/~g" {} +

# Also handle import("@/...)  dynamic imports
find packages/shared-components/src -type f \( -name "*.ts" -o -name "*.vue" -o -name "*.js" \) -exec sed -i 's~import("@/~import("@encv/shared-components/~g' {} +

echo "=== Step 4: Create re-export stubs in encv-mobile for singletons ==="
# These ensure encv-mobile's own local files still get the shared singleton instances
# when using the old @/ path.

# List of modules moved to shared-components that are stateful or heavily shared
for f in \
  composables/useI18n.ts \
  composables/useEventBus.ts \
  composables/useTheme.ts \
  composables/useClipboard.ts \
  composables/useToast.ts \
  composables/useConfig.ts \
  composables/useServerStatus.ts \
  composables/useFileFeatures.ts \
  composables/useFrontendLogs.ts \
  composables/useRealtimeTransport.ts \
  composables/useProxiedFetch.ts \
  composables/useErrorCapture.ts \
  composables/useDevTools.ts \
  composables/useHighRefreshRate.ts \
  composables/useApiBaseProbe.ts \
  api/encv.ts \
  config/schemaParser.ts \
  constants/player.ts \
  features/alist-encrypt.ts \
  features/config-schema.ts \
  utils/IncrementalFilter.ts \
  plugins/SimVerse.ts \
  plugins/GoProcess.ts \
  i18n/agent.ts \
  i18n/common.ts \
  i18n/devlogs.ts \
  i18n/errors.ts \
  i18n/extensions.ts \
  i18n/files.ts \
  i18n/modals.ts \
  i18n/player.ts \
  i18n/settings.ts \
  i18n/tasks.ts \
  components/ConfigFieldItem.vue \
  components/FilePickerModal.vue \
  components/VirtualLogList.vue \
  components/shared/FilterDropdown.vue \
; do
  if [ -f "packages/shared-components/src/$f" ]; then
    dir=$(dirname "encv-mobile/src/$f")
    mkdir -p "$dir"
    base=$(basename "$f")
    ext="${base##*.}"
    name="${base%.*}"
    shared_path="@encv/shared-components/${f%.*}"  # remove extension for vue import
    if [ "$ext" = "vue" ]; then
      cat > "encv-mobile/src/$f" <<EOF
<script setup lang="ts">
export { default } from '${shared_path}.vue'
</script>
EOF
    else
      cat > "encv-mobile/src/$f" <<EOF
export * from '${shared_path}.${ext}'
EOF
    fi
  fi
done

# CSS stubs
for f in theme/variables.css styles/timeline-tokens.css styles/timeline-utilities.css; do
  if [ -f "packages/shared-components/src/$f" ]; then
    dir=$(dirname "encv-mobile/src/$f")
    mkdir -p "$dir"
    cat > "encv-mobile/src/$f" <<EOF
@import "@encv/shared-components/${f%.css}";
EOF
  fi
done

# Special: AgentEntry.vue composable (used by HomePage)
if [ -f "packages/shared-components/src/components/agent/AgentEntry.vue" ]; then
  mkdir -p encv-mobile/src/components/agent
  cat > encv-mobile/src/components/agent/AgentEntry.vue <<'EOF'
<script setup lang="ts">
export { default } from '@encv/shared-components/components/agent/AgentEntry.vue'
</script>
EOF
fi

echo "=== Step 5: Create shared-components barrel export ==="
cat > packages/shared-components/src/index.ts <<'EOF'
// Barrel exports for commonly used modules
export { useI18n } from './composables/useI18n'
export { eventBus } from './composables/useEventBus'
export { useTheme } from './composables/useTheme'
export { useClipboard } from './composables/useClipboard'
export { useToast } from './composables/useToast'
export { useConfig } from './composables/useConfig'
export { useServerStatus } from './composables/useServerStatus'
export { useFrontendLogs } from './composables/useFrontendLogs'
export { useRealtimeTransport } from './composables/useRealtimeTransport'
export { useApiBaseProbe } from './composables/useApiBaseProbe'

export * from './api/encv'
export { isNative } from './plugins/GoProcess'
export { openWorld } from './plugins/SimVerse'
EOF

echo "=== Step 6: Update encv-mobile package.json ==="
node -e "
const fs = require('fs');
const pkg = JSON.parse(fs.readFileSync('encv-mobile/package.json', 'utf8'));
pkg.dependencies['@encv/shared-components'] = 'workspace:*';
fs.writeFileSync('encv-mobile/package.json', JSON.stringify(pkg, null, 2) + '\n');
"

echo "=== Step 7: Create simverse-frontend router and pages ==="
mkdir -p simverse-frontend/src/views

cat > simverse-frontend/src/router/index.ts <<'EOF'
import { createRouter, createWebHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";
import SimverseTabs from "@self/views/SimverseTabs.vue";

const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/tabs/home" },
  {
    path: "/tabs/",
    component: SimverseTabs,
    children: [
      { path: "", redirect: "/tabs/home" },
      {
        path: "home",
        component: () => import("@self/views/SimverseHome.vue"),
      },
      {
        path: "settings",
        component: () => import("@self/views/SimverseSettings.vue"),
      },
      {
        path: "devlogs",
        component: () => import("@self/views/SimverseDevLogs.vue"),
      },
    ],
  },
  {
    path: "/world",
    component: () => import("@self/views/SimverseWorld.vue"),
  },
];

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});
EOF

cat > simverse-frontend/src/views/SimverseTabs.vue <<'EOF'
<template>
  <ion-page>
    <ion-tabs>
      <ion-router-outlet></ion-router-outlet>
      <ion-tab-bar slot="bottom">
        <ion-tab-button tab="home" href="/tabs/home">
          <ion-icon :icon="homeIcon"></ion-icon>
          <ion-label>Home</ion-label>
        </ion-tab-button>
        <ion-tab-button tab="settings" href="/tabs/settings">
          <ion-icon :icon="settingsIcon"></ion-icon>
          <ion-label>Settings</ion-label>
        </ion-tab-button>
        <ion-tab-button tab="devlogs" href="/tabs/devlogs">
          <ion-icon :icon="bugIcon"></ion-icon>
          <ion-label>DevLogs</ion-label>
        </ion-tab-button>
      </ion-tab-bar>
    </ion-tabs>
  </ion-page>
</template>

<script setup lang="ts">
import { IonPage, IonTabs, IonRouterOutlet, IonTabBar, IonTabButton, IonIcon, IonLabel } from "@ionic/vue";
import { home as homeIcon, settings as settingsIcon, bug as bugIcon } from "ionicons/icons";
</script>
EOF

cat > simverse-frontend/src/views/SimverseHome.vue <<'EOF'
<template>
  <HomePage simverse-action="enter-world" />
</template>

<script setup lang="ts">
import HomePage from "@encv/shared-components/views/HomePage.vue";
</script>
EOF

cat > simverse-frontend/src/views/SimverseSettings.vue <<'EOF'
<template>
  <Settings />
</template>

<script setup lang="ts">
import Settings from "@encv/shared-components/views/Settings.vue";
</script>
EOF

cat > simverse-frontend/src/views/SimverseDevLogs.vue <<'EOF'
<template>
  <DevLogs />
</template>

<script setup lang="ts">
import DevLogs from "@encv/shared-components/views/DevLogs.vue";
</script>
EOF

echo "=== Step 8: Create simverse-frontend hooks for enhanced HomePage behavior ==="
mkdir -p simverse-frontend/src/composables
cat > simverse-frontend/src/composables/useSimverse.ts <<'EOF'
import { ref, computed } from "vue";
import { eventBus } from "@encv/shared-components/composables/useEventBus";
import type { EncvEvents } from "@encv/shared-components/composables/useEventBus";

export interface SimverseWorldState {
  tick: number;
  tier: string;
  running: boolean;
  npc_count: number;
  npc_mb: number;
  brain_count: number;
  brain_mb: number;
  cell_count: number;
  cell_mb: number;
  focus_count: number;
  total_mb: number;
}

export interface SimverseWorldConfig {
  tier: number;
  tier_name: string;
  event_rate_mul: number;
  cache_size: number;
  sub_sim_active: boolean;
  sub_sim_depth: number;
}

export interface SimverseNPC {
  id: number;
  name: string;
  species: string;
  gender: string;
  age: number;
  life_stage: string;
  profession: string;
  level: number;
  health: number;
  max_health: number;
  energy: number;
  max_energy: number;
  is_alive: boolean;
  wealth_tier: number;
  social_tier: number;
}

export interface SimverseNPCDetail extends SimverseNPC {
  gender_identity: string;
  sexual_orient: string;
  career_stage: number;
  mana: number;
  max_mana: number;
  mood: number;
  satisfaction: number;
  experience: number;
  num_children: number;
  num_marriages: number;
  org_id: number;
  region_id: number;
  home_region_id: number;
  skills: Record<string, number>;
  inventory: Record<string, number>;
  bank: Record<string, number>;
  big_five: Record<string, number>;
  values: Record<string, number>;
  interests: Record<string, number>;
  top_values: string[];
  top_interests: string[];
  short_term_mem: SimverseMemory[];
  life_events: number;
  born_at: number;
  died_at: number;
  last_update: number;
}

export interface SimverseMemory {
  id: number;
  type: string;
  importance: number;
  target_id: number;
  content_tag: number;
  emotion_tag: number;
  created_at: number;
  strength: number;
}

export interface SimverseFocusNPC {
  id: number;
  name: string;
  level: string;
  life_stage: string;
}

export interface SimversePerfMetrics {
  avg_tick_ns: number;
  min_tick_ns: number;
  max_tick_ns: number;
  ticks_per_sec: number;
  samples: number;
  npc_count: number;
  npc_mb: number;
  total_mb: number;
  running: boolean;
  tier: string;
}

export interface SimverseChronicleEvent {
  id: number;
  tick: number;
  level: string;
  level_cn: string;
  type: string;
  type_cn: string;
  importance: number;
  imp_name: string;
  imp_cn: string;
  entity_id: number;
  target_id: number;
  data_tag: number;
  cause1_id: number;
  cause2_id: number;
  cause3_id: number;
  causes?: SimverseChronicleEvent[];
  effects?: SimverseChronicleEvent[];
}

export interface SimverseChronicleWorldResponse {
  count: number;
  era: number;
  total_events: number;
  items: SimverseChronicleEvent[];
}

export interface SimverseChronicleNPCResponse {
  npc_id: number;
  count: number;
  items: SimverseChronicleEvent[];
}

export type PerfTier = "background" | "foreground" | "fg_idle";
export type FocusLevel = "none" | "distant" | "near" | "core" | "player";
export type WorldAction = "start" | "stop" | "pause" | "resume" | "step";

const worldState = ref<SimverseWorldState | null>(null);
const worldConfig = ref<SimverseWorldConfig | null>(null);
const perfMetrics = ref<SimversePerfMetrics | null>(null);
const focusNPCs = ref<SimverseNPC[]>([]);
const isConnected = ref(false);
const isLoading = ref(false);
const error = ref("");

let ws: WebSocket | null = null;
let wsReconnectTimer: number | null = null;
let pollTimer: number | null = null;
let initialized = false;

function getApiBase(): string {
  if (typeof window !== "undefined") {
    return window.location.origin;
  }
  return "http://localhost:8780";
}

async function fetchJSON(path: string, options: RequestInit = {}) {
  const res = await fetch(`${getApiBase()}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
  });
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${res.statusText}`);
  }
  return res.json();
}

export function useSimverse() {
  async function loadWorldState() {
    try {
      isLoading.value = true;
      error.value = "";
      const data = await fetchJSON("/api/simverse/world/state");
      worldState.value = data;
      return data;
    } catch (e: any) {
      error.value = e.message || "Failed to load world state";
      throw e;
    } finally {
      isLoading.value = false;
    }
  }

  async function loadWorldConfig() {
    try {
      const data = await fetchJSON("/api/simverse/world/config");
      worldConfig.value = data;
      return data;
    } catch (e: any) {
      console.warn("Failed to load world config:", e);
      throw e;
    }
  }

  async function setPerformanceTier(tier: PerfTier) {
    const data = await fetchJSON("/api/simverse/world/config", {
      method: "POST",
      body: JSON.stringify({ tier }),
    });
    worldConfig.value = data;
    if (worldState.value) {
      worldState.value.tier = data.tier_name;
    }
    return data;
  }

  async function controlWorld(action: WorldAction) {
    const data = await fetchJSON("/api/simverse/world/control", {
      method: "POST",
      body: JSON.stringify({ action }),
    });
    if (worldState.value) {
      if (action === "start" || action === "resume") {
        worldState.value.running = true;
      } else if (action === "stop" || action === "pause") {
        worldState.value.running = false;
      }
      if (data.tick !== undefined) {
        worldState.value.tick = data.tick;
      }
    }
    return data;
  }

  async function loadNPCList(page = 1, pageSize = 50) {
    const data = await fetchJSON(
      `/api/simverse/npc/list?page=${page}&page_size=${pageSize}`,
    );
    return data as {
      page: number;
      page_size: number;
      total: number;
      items: SimverseNPC[];
    };
  }

  async function loadNPCDetail(id: number) {
    const data = await fetchJSON(`/api/simverse/npc/${id}`);
    return data as SimverseNPCDetail;
  }

  async function loadFocusNPCs() {
    try {
      const data = await fetchJSON("/api/simverse/focus");
      focusNPCs.value = data.items || [];
      return data.items || [];
    } catch (e) {
      console.warn("Failed to load focus NPCs:", e);
      return [];
    }
  }

  async function setFocusNPCs(npcs: { id: number; level: FocusLevel }[]) {
    const data = await fetchJSON("/api/simverse/focus", {
      method: "POST",
      body: JSON.stringify({ npcs }),
    });
    await loadFocusNPCs();
    return data;
  }

  async function loadPerfMetrics() {
    try {
      const data = await fetchJSON("/api/simverse/perf/metrics");
      perfMetrics.value = data;
      return data;
    } catch (e) {
      console.warn("Failed to load perf metrics:", e);
      return null;
    }
  }

  async function loadChronicleWorld(minImportance = 2, limit = 50) {
    try {
      const data = await fetchJSON(
        `/api/simverse/chronicle/world?min_importance=${minImportance}&limit=${limit}`,
      );
      return data as SimverseChronicleWorldResponse;
    } catch (e) {
      console.warn("Failed to load world chronicle:", e);
      return null;
    }
  }

  async function loadChronicleNPC(npcID: number, limit = 50) {
    try {
      const data = await fetchJSON(
        `/api/simverse/chronicle/npc/${npcID}?limit=${limit}`,
      );
      return data as SimverseChronicleNPCResponse;
    } catch (e) {
      console.warn(`Failed to load NPC ${npcID} chronicle:`, e);
      return null;
    }
  }

  async function loadChronicleEvent(eventID: number) {
    try {
      const data = await fetchJSON(`/api/simverse/chronicle/event/${eventID}`);
      return data as SimverseChronicleEvent;
    } catch (e) {
      console.warn(`Failed to load chronicle event ${eventID}:`, e);
      return null;
    }
  }

  function connectWebSocket() {
    if (ws) return;

    try {
      const wsUrl = `${getApiBase().replace("http", "ws")}/api/simverse/ws`;
      ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        isConnected.value = true;
        error.value = "";
        console.log("[simverse] WS connected");
        eventBus.emit("simverse:ws:connected", {});
      };

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          handleWSMessage(msg);
        } catch (e) {
          console.warn("[simverse] Failed to parse WS message:", e);
        }
      };

      ws.onclose = () => {
        isConnected.value = false;
        console.log("[simverse] WS disconnected");
        eventBus.emit("simverse:ws:disconnected", {});
        scheduleReconnect();
      };

      ws.onerror = (e) => {
        console.error("[simverse] WS error:", e);
      };
    } catch (e) {
      console.error("[simverse] Failed to create WS:", e);
      scheduleReconnect();
    }
  }

  function handleWSMessage(msg: { type: string; data: any }) {
    switch (msg.type) {
      case "world:tick":
        if (worldState.value) {
          worldState.value.tick = msg.data.tick;
          worldState.value.tier = msg.data.tier;
        }
        eventBus.emit("simverse:tick", msg.data);
        break;
      case "world:stats":
        if (worldState.value) {
          worldState.value.npc_count = msg.data.npc_count;
          worldState.value.npc_mb = msg.data.npc_mb;
          worldState.value.brain_count = msg.data.brain_count;
          worldState.value.cell_count = msg.data.cell_count;
          worldState.value.total_mb = msg.data.total_mb;
          worldState.value.focus_count = msg.data.focus_count;
        }
        eventBus.emit("simverse:stats", msg.data);
        break;
      case "pong":
        eventBus.emit("simverse:pong", msg.data);
        break;
      default:
        eventBus.emit(`simverse:${msg.type}` as keyof EncvEvents, msg.data);
    }
  }

  function scheduleReconnect() {
    if (wsReconnectTimer) return;
    wsReconnectTimer = window.setTimeout(() => {
      wsReconnectTimer = null;
      ws = null;
      connectWebSocket();
    }, 3000);
  }

  function disconnectWebSocket() {
    if (wsReconnectTimer) {
      clearTimeout(wsReconnectTimer);
      wsReconnectTimer = null;
    }
    if (ws) {
      ws.close();
      ws = null;
    }
    isConnected.value = false;
  }

  function sendWS(type: string, data: any = {}) {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type, ...data }));
    }
  }

  function startPolling(intervalMs = 2000) {
    if (pollTimer) return;
    pollTimer = window.setInterval(() => {
      if (!isConnected.value) {
        loadWorldState().catch(() => {});
      }
    }, intervalMs);
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  async function init() {
    if (initialized) return;
    initialized = true;

    try {
      await loadWorldState();
      await loadWorldConfig();
      await loadFocusNPCs();
      await loadPerfMetrics();
      connectWebSocket();
      startPolling();
    } catch (e: any) {
      error.value = e.message || "Init failed";
      startPolling();
    }
  }

  function cleanup() {
    disconnectWebSocket();
    stopPolling();
  }

  const isRunning = computed(() => worldState.value?.running ?? false);
  const currentTick = computed(() => worldState.value?.tick ?? 0);
  const currentTier = computed(() => worldConfig.value?.tier_name ?? "background");
  const totalMemoryMB = computed(() => worldState.value?.total_mb ?? 0);
  const npcCount = computed(() => worldState.value?.npc_count ?? 0);

  return {
    worldState,
    worldConfig,
    perfMetrics,
    focusNPCs,
    isConnected,
    isLoading,
    error,

    isRunning,
    currentTick,
    currentTier,
    totalMemoryMB,
    npcCount,

    loadWorldState,
    loadWorldConfig,
    setPerformanceTier,
    controlWorld,
    loadNPCList,
    loadNPCDetail,
    loadFocusNPCs,
    setFocusNPCs,
    loadPerfMetrics,
    loadChronicleWorld,
    loadChronicleNPC,
    loadChronicleEvent,

    connectWebSocket,
    disconnectWebSocket,
    sendWS,

    init,
    cleanup,
  };
}
EOF

cat > simverse-frontend/src/composables/useWorldRenderer.ts <<'EOF'
import { ref, onUnmounted } from 'vue';
import { Leafer, Rect, Ellipse, Text, PointerEvent } from 'leafer-ui';
import Matter from 'matter-js';

export interface WorldEntity {
  id: string;
  type: 'npc' | 'building' | 'tree' | 'rock' | 'ground' | 'water';
  x: number;
  y: number;
  width: number;
  height: number;
  color: string;
  label?: string;
  static?: boolean;
  onClick?: (entity: WorldEntity) => void;
}

export interface WorldRendererOptions {
  container: HTMLElement;
  worldWidth: number;
  worldHeight: number;
  gravity?: number;
}

export function useWorldRenderer(options: WorldRendererOptions) {
  const { container, worldWidth, worldHeight, gravity = 0 } = options;

  const isReady = ref(false);

  let leafer: Leafer | null = null;
  let engine: Matter.Engine | null = null;
  let runner: Matter.Runner | null = null;

  const bodyToEntity = new Map<number, WorldEntity>();
  const entityToLeafer = new Map<string, any>();
  const entityToBody = new Map<string, Matter.Body>();

  function init() {
    leafer = new Leafer({
      view: container,
      width: container.clientWidth,
      height: container.clientHeight,
    });

    engine = Matter.Engine.create();
    engine.gravity.y = gravity;

    const worldSize = { width: worldWidth, height: worldHeight };
    Matter.Composite.add(engine.world, [
      Matter.Bodies.rectangle(worldSize.width / 2, -50, worldSize.width, 100, { isStatic: true, label: 'wall-top' }),
      Matter.Bodies.rectangle(worldSize.width / 2, worldSize.height + 50, worldSize.width, 100, { isStatic: true, label: 'wall-bottom' }),
      Matter.Bodies.rectangle(-50, worldSize.height / 2, 100, worldSize.height, { isStatic: true, label: 'wall-left' }),
      Matter.Bodies.rectangle(worldSize.width + 50, worldSize.height / 2, 100, worldSize.height, { isStatic: true, label: 'wall-right' }),
    ]);

    runner = Matter.Runner.create();
    Matter.Runner.run(runner, engine);

    requestAnimationFrame(gameLoop);

    isReady.value = true;
  }

  let lastTime = 0;
  function gameLoop(timestamp: number) {
    if (!engine || !leafer) return;

    const delta = timestamp - lastTime;
    lastTime = timestamp;

    Matter.Engine.update(engine, delta);

    for (const [entityId, body] of entityToBody.entries()) {
      const leaf = entityToLeafer.get(entityId);
      if (leaf && !body.isStatic) {
        leaf.x = body.position.x - body.bounds.min.x;
        leaf.y = body.position.y - body.bounds.min.y;
        leaf.rotation = body.angle * 57.2958;
      }
    }

    requestAnimationFrame(gameLoop);
  }

  function addEntity(entity: WorldEntity) {
    if (!leafer || !engine) return;

    const isStatic = entity.static ?? (entity.type === 'building' || entity.type === 'tree' || entity.type === 'ground' || entity.type === 'rock' || entity.type === 'water');

    let leaf: any;
    if (entity.type === 'npc') {
      leaf = new Ellipse({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
        stroke: '#ffffff',
        strokeWidth: 2,
        cursor: 'pointer',
      });
    } else if (entity.type === 'tree' || entity.type === 'rock' || entity.type === 'building') {
      leaf = new Rect({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
        stroke: '#333',
        strokeWidth: 1,
        cornerRadius: 4,
      });
    } else if (entity.type === 'water' || entity.type === 'ground') {
      leaf = new Rect({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
      });
    } else {
      leaf = new Rect({
        x: entity.x,
        y: entity.y,
        width: entity.width,
        height: entity.height,
        fill: entity.color,
      });
    }

    if (entity.label) {
      const labelText = new Text({
        x: entity.x + entity.width / 2,
        y: entity.y - 18,
        text: entity.label,
        fontSize: 12,
        fill: '#fff',
        textAlign: 'center',
        verticalAlign: 'middle',
        textWrap: false,
      });
      leafer.add(labelText);
    }

    leafer.add(leaf);
    entityToLeafer.set(entity.id, leaf);

    const body = Matter.Bodies.rectangle(
      entity.x + entity.width / 2,
      entity.y + entity.height / 2,
      entity.width,
      entity.height,
      { isStatic, label: entity.id, friction: 0.1, restitution: 0.3 }
    );
    Matter.Composite.add(engine.world, body);
    bodyToEntity.set(body.id, entity);
    entityToBody.set(entity.id, body);

    if (entity.onClick && entity.type === 'npc') {
      leaf.on(PointerEvent.DOWN, () => {
        entity.onClick?.(entity);
      });
    }
  }

  function removeEntity(id: string) {
    const leaf = entityToLeafer.get(id);
    if (leaf) {
      leafer?.remove(leaf);
      entityToLeafer.delete(id);
    }
    const body = entityToBody.get(id);
    if (body && engine) {
      Matter.Composite.remove(engine.world, body);
      bodyToEntity.delete(body.id);
      entityToBody.delete(id);
    }
  }

  function updateEntityPosition(id: string, x: number, y: number) {
    const body = entityToBody.get(id);
    if (body) {
      Matter.Body.setPosition(body, { x, y });
    }
  }

  function applyForce(id: string, x: number, y: number) {
    const body = entityToBody.get(id);
    if (body) {
      Matter.Body.applyForce(body, body.position, { x, y });
    }
  }

  function setVelocity(id: string, x: number, y: number) {
    const body = entityToBody.get(id);
    if (body) {
      Matter.Body.setVelocity(body, { x, y });
    }
  }

  function resize() {
    if (leafer) {
      leafer.resize({ width: container.clientWidth, height: container.clientHeight });
    }
  }

  function clearAll() {
    for (const id of Array.from(entityToBody.keys())) {
      removeEntity(id);
    }
  }

  function destroy() {
    clearAll();
    if (runner) {
      Matter.Runner.stop(runner);
      runner = null;
    }
    if (engine) {
      Matter.Engine.clear(engine);
      engine = null;
    }
    if (leafer) {
      leafer.destroy();
      leafer = null;
    }
    bodyToEntity.clear();
    entityToLeafer.clear();
    entityToBody.clear();
    isReady.value = false;
  }

  onUnmounted(() => {
    destroy();
  });

  return {
    isReady,
    init,
    destroy,
    resize,
    addEntity,
    removeEntity,
    updateEntityPosition,
    applyForce,
    setVelocity,
    clearAll,
  };
}
EOF

echo "=== Step 9: Update HomePage.vue to accept simverse action prop ==="
# We need to modify the shared HomePage so it can behave differently
# in encv-mobile vs simverse-frontend.
# Approach: add a handleOpenSimverse prop that the parent can customize.
# For now, the simverse-frontend SimverseHome wrapper will handle the click event
# to route to /world, and encv-mobile's click will open the Simverse app.

# Modify the shared HomePage to emit event instead of direct routing
# But that's a big change. For now, we'll keep two copies:
# Shared HomePage stays as-is (uses router.push).
# In simverse-frontend, SimverseHome.vue wraps it differently.

echo "=== Step 10: Cleanup encv-mobile-simverse files ==="
rm -f encv-mobile/src/views/SimverseWorld.vue
rm -f encv-mobile/src/composables/useSimverse.ts
rm -f encv-mobile/src/composables/useWorldRenderer.ts
rm -f encv-mobile/src/simverse-world-main.ts
rm -f encv-mobile/simverse-world.html

echo "=== Step 11: Update encv-mobile vite config ==="
# Remove simverse-world entry from rollupOptions
sed -i "s~'simverse-world': path.resolve(__dirname, 'simverse-world.html'),~~g" encv-mobile/vite.config.ts

echo "=== Step 12: Update encv-mobile router ==="
# Remove /simverse/world route
node -e "
const fs = require('fs');
let content = fs.readFileSync('encv-mobile/src/router/index.ts', 'utf8');
content = content.replace(/  {\s*path: '\/simverse\/world',\s*component: \(\) => import\('@\/views\/SimverseWorld.vue'\),\s*},\n/g, '');
fs.writeFileSync('encv-mobile/src/router/index.ts', content);
"

echo "=== Step 13: Update encv-mobile HomePage to import from shared ==="
# HomePage in encv-mobile should now import from shared-components
# But we created a stub re-export at encv-mobile/src/views/HomePage.vue
# Let me make sure the stub exports the shared HomePage
mkdir -p encv-mobile/src/views
cat > encv-mobile/src/views/HomePage.vue <<'EOF'
<script setup lang="ts">
export { default } from '@encv/shared-components/views/HomePage.vue'
</script>
EOF

echo "=== Done ==="
