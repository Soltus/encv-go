import { ref, computed, onMounted, onUnmounted } from "vue";
import { useProxiedFetch } from "./useProxiedFetch";
import { eventBus } from "./useEventBus";

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
  const { fetch: proxiedFetch } = useProxiedFetch();
  const res = await proxiedFetch(`${getApiBase()}${path}`, {
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
        eventBus.emit(`simverse:${msg.type}`, msg.data);
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

    connectWebSocket,
    disconnectWebSocket,
    sendWS,

    init,
    cleanup,
  };
}
