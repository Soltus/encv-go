import { computed, ref } from "vue";

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
  tier: string;
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

export interface SimverseSaveInfo {
  has_save: boolean;
  saved_at?: string;
  tick?: number;
  npc_count?: number;
  size_bytes?: number;
}

export interface SimverseStorageStatus {
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
}

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
    if ((window as any).__ENCV_API_BASE__) {
      return (window as any).__ENCV_API_BASE__;
    }
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
    const data = await fetchJSON(`/api/simverse/npc/list?page=${page}&page_size=${pageSize}`);
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
      const data = await fetchJSON(`/api/simverse/chronicle/world?min_importance=${minImportance}&limit=${limit}`);
      return data as SimverseChronicleWorldResponse;
    } catch (e) {
      console.warn("Failed to load world chronicle:", e);
      return null;
    }
  }

  async function loadChronicleNPC(npcID: number, limit = 50) {
    try {
      const data = await fetchJSON(`/api/simverse/chronicle/npc/${npcID}?limit=${limit}`);
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

  async function loadSaveInfo() {
    try {
      const data = await fetchJSON("/api/simverse/world/save");
      return data as SimverseSaveInfo;
    } catch (e) {
      console.warn("Failed to load save info:", e);
      return null;
    }
  }

  async function saveWorld() {
    const data = await fetchJSON("/api/simverse/world/save", {
      method: "POST",
    });
    return data as SimverseSaveInfo;
  }

  async function loadWorld() {
    const data = await fetchJSON("/api/simverse/world/load", {
      method: "POST",
    });
    if (worldState.value && data.tick !== undefined) {
      worldState.value.tick = data.tick;
    }
    return data as SimverseSaveInfo;
  }

  async function loadStorageStatus() {
    try {
      const data = await fetchJSON("/api/simverse/world/storage");
      return data as SimverseStorageStatus;
    } catch (e) {
      console.warn("Failed to load storage status:", e);
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
      };

      ws.onmessage = event => {
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
        scheduleReconnect();
      };

      ws.onerror = e => {
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
        break;
      case "pong":
        break;
      default:
        console.log("[simverse] WS event:", msg.type, msg.data);
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
    loadSaveInfo,
    saveWorld,
    loadWorld,
    loadStorageStatus,

    connectWebSocket,
    disconnectWebSocket,
    sendWS,

    init,
    cleanup,
  };
}
