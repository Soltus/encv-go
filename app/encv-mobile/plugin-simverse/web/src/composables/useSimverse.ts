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
  region_id?: number;
  org_id?: number;
  current_behavior?: string;
  current_behavior_cn?: string;
  mood?: number;
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

export interface SimverseBehaviorNeeds {
  hunger: number;
  energy: number;
  social: number;
  achievement: number;
}

export interface SimverseBehaviorState {
  npc_id: number;
  npc_name: string;
  profession: string;
  level: number;
  current_behavior: string;
  current_behavior_cn: string;
  behavior_start_tick: number;
  behavior_duration: number;
  mood: number;
  energy: number;
  needs: SimverseBehaviorNeeds;
}

export interface SimverseBehaviorStats {
  total_npcs: number;
  alive_npcs: number;
  behavior_dist: Record<string, number>;
}

export interface SimverseBehaviorListResponse {
  page: number;
  page_size: number;
  total: number;
  items: SimverseBehaviorState[];
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

export interface SimverseEraEvent {
  id: number;
  tick: number;
  type: string;
  importance: number;
  data_tag: number;
}

export interface SimverseEra {
  era: number;
  world_tick: number;
  event_count: number;
  events: SimverseEraEvent[];
}

export interface SimverseRegion {
  region_id: number;
  npc_count: number;
  alive_count: number;
  population: number;
  avg_level: number;
  avg_wealth_tier: number;
  economy?: Record<string, any>;
}

export interface SimverseRegionListResponse {
  count: number;
  items: SimverseRegion[];
}

export interface SimverseOrg {
  org_id: number;
  name: string;
  org_type: string;
  member_count: number;
  alive_count: number;
  region_distribution: Record<string, number>;
  avg_level: number;
  avg_wealth_tier: number;
  avg_career_stage: number;
}

export interface SimverseOrgListResponse {
  count: number;
  items: SimverseOrg[];
}

export interface SimverseOrgMember {
  id: number;
  name: string;
  species: string;
  gender: string;
  age: number;
  profession: string;
  level: number;
  org_id: number;
  region_id: number;
  wealth_tier: number;
  career_stage: number;
  is_alive: boolean;
}

export interface SimverseOrgMembersResponse {
  org_id: number;
  page: number;
  page_size: number;
  total: number;
  count: number;
  items: SimverseOrgMember[];
}

export interface SimverseOrgTerritory {
  region_id: number;
  members: number;
}

export interface SimverseOrgTerritoryResponse {
  org_id: number;
  name: string;
  territory: SimverseOrgTerritory[];
}

export interface SimverseEconomyPrices {
  region_id: number;
  prices: Record<string, number>;
  supply: Record<string, number>;
  demand: Record<string, number>;
  trade_volume: number;
}

export interface SimverseEconomyShock {
  type: string;
  region_id: number;
  resource: string;
  price: number;
  change: number;
  message: string;
}

export interface SimverseEconomyShocksResponse {
  count: number;
  items: SimverseEconomyShock[];
}

export type SimverseQuestType = "daily" | "achieve" | "story" | "economy";
export type SimverseQuestStatus = "locked" | "active" | "claimed" | "expired";

export interface SimverseQuestReward {
  diamond: number;
  gold: number;
  exp: number;
  icon: string;
}
export interface SimverseQuest {
  id: string;
  type: SimverseQuestType;
  title: string;
  desc: string;
  icon: string;
  goal: number;
  progress: number;
  reward: SimverseQuestReward;
  status: SimverseQuestStatus;
  sort_order: number;
}
export interface SimversePlayerStats {
  total_ticks_observed: number;
  total_npcs_checked: number;
  total_economy_checks: number;
  total_gacha_pulls: number;
  total_battles: number;
  world_ticks_seen: number;
}
export interface SimverseQuestSummary {
  quests: SimverseQuest[];
  active_count: number;
  claimed_count: number;
  completable: number;
  player_stats: SimversePlayerStats;
}

// 社交关系系统
export type SimverseRelationType =
  | "stranger"
  | "acquaintance"
  | "friend"
  | "lover"
  | "spouse"
  | "parent"
  | "child"
  | "sibling"
  | "master"
  | "apprentice"
  | "enemy"
  | "rival";

export interface SimverseRelation {
  target_id: number;
  rel_type: SimverseRelationType;
  rel_type_id: number;
  affinity: number;
  last_meet: number;
  target: SimverseNPC;
}

export interface SimverseRelationListResponse {
  npc_id: number;
  name: string;
  count: number;
  counts: Record<string, number>;
  relations: SimverseRelation[];
}

export interface SimverseSocialStats {
  sampled_npcs: number;
  total_relations: number;
  by_type: Record<string, number>;
  by_region: Record<string, number>;
  by_org: Record<string, number>;
}

// 战斗系统
export interface SimverseBattle {
  id: number;
  tick: number;
  attacker_id: number;
  attacker_name: string;
  defender_id: number;
  defender_name: string;
  winner_id: number;
  loser_id: number;
  outcome: "attacker" | "defender" | "draw";
  damage: number;
  attacker_hp: number;
  defender_hp: number;
  loot_gold: number;
  log: string[];
}

export interface SimverseBattleListResponse {
  total: number;
  count: number;
  battles: SimverseBattle[];
}

export interface SimverseBattleRankEntry {
  npc_id: number;
  name: string;
  wins: number;
}

export interface SimverseBattleRankResponse {
  count: number;
  rank: SimverseBattleRankEntry[];
}

export interface SimverseBattleSimulateResponse extends SimverseBattle {}

const worldState = ref<SimverseWorldState | null>(null);
const worldConfig = ref<SimverseWorldConfig | null>(null);
const perfMetrics = ref<SimversePerfMetrics | null>(null);
const focusNPCs = ref<SimverseNPC[]>([]);
const isConnected = ref(false);
const isLoading = ref(false);
const error = ref("");

// P7 持续演化：WebSocket 实时推送信号（递增计数，供视图 watch 触发刷新，替代轮询）
const economySignal = ref(0);
const chronicleSignal = ref(0);

let ws: WebSocket | null = null;
let wsReconnectTimer: number | null = null;
let pollTimer: number | null = null;
let initialized = false;

function getApiBase(): string {
  if (typeof window !== "undefined") {
    if ((window as any).__ENCV_API_BASE__) {
      return (window as any).__ENCV_API_BASE__;
    }
    if ((window as any).SimVerseNative && typeof (window as any).SimVerseNative.getApiBaseUrl === "function") {
      try {
        return (window as any).SimVerseNative.getApiBaseUrl();
      } catch (e) {
        console.warn("[simverse] Failed to get api base from SimVerseNative:", e);
      }
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

  async function loadBehaviorStats() {
    try {
      const data = await fetchJSON("/api/simverse/npc/behavior/stats");
      return data as SimverseBehaviorStats;
    } catch (e) {
      console.warn("Failed to load behavior stats:", e);
      return null;
    }
  }

  async function loadBehaviorList(page = 1, pageSize = 50, behavior = "") {
    try {
      const params = new URLSearchParams({
        page: String(page),
        page_size: String(pageSize),
      });
      if (behavior) {
        params.append("behavior", behavior);
      }
      const data = await fetchJSON(`/api/simverse/npc/behavior/list?${params.toString()}`);
      return data as SimverseBehaviorListResponse;
    } catch (e) {
      console.warn("Failed to load behavior list:", e);
      return null;
    }
  }

  async function loadEra(limit = 50) {
    try {
      const data = await fetchJSON(`/api/simverse/era/current?limit=${limit}`);
      return data as SimverseEra;
    } catch (e) {
      console.warn("Failed to load era:", e);
      return null;
    }
  }

  async function loadRegionList() {
    try {
      const data = await fetchJSON("/api/simverse/region/list");
      return data as SimverseRegionListResponse;
    } catch (e) {
      console.warn("Failed to load region list:", e);
      return null;
    }
  }

  async function loadRegionDetail(id: number) {
    try {
      const data = await fetchJSON(`/api/simverse/region/${id}`);
      return data as { region: SimverseRegion; events: SimverseEraEvent[] };
    } catch (e) {
      console.warn(`Failed to load region ${id}:`, e);
      return null;
    }
  }

  async function loadOrgList() {
    try {
      const data = await fetchJSON("/api/simverse/org/list");
      return data as SimverseOrgListResponse;
    } catch (e) {
      console.warn("Failed to load org list:", e);
      return null;
    }
  }

  async function loadOrgDetail(id: number) {
    try {
      const data = await fetchJSON(`/api/simverse/org/${id}`);
      return data as SimverseOrg;
    } catch (e) {
      console.warn(`Failed to load org ${id}:`, e);
      return null;
    }
  }

  async function loadOrgMembers(id: number, page = 1, pageSize = 50) {
    try {
      const data = await fetchJSON(`/api/simverse/org/${id}/members?page=${page}&page_size=${pageSize}`);
      return data as SimverseOrgMembersResponse;
    } catch (e) {
      console.warn(`Failed to load org ${id} members:`, e);
      return null;
    }
  }

  async function loadOrgTerritory(id: number) {
    try {
      const data = await fetchJSON(`/api/simverse/org/${id}/territory`);
      return data as SimverseOrgTerritoryResponse;
    } catch (e) {
      console.warn(`Failed to load org ${id} territory:`, e);
      return null;
    }
  }

  async function loadEconomyPrices(region = 1) {
    try {
      const data = await fetchJSON(`/api/simverse/economy/prices?region=${region}`);
      return data as SimverseEconomyPrices;
    } catch (e) {
      console.warn("Failed to load economy prices:", e);
      return null;
    }
  }

  async function loadEconomyShocks() {
    try {
      const data = await fetchJSON("/api/simverse/economy/shocks");
      return data as SimverseEconomyShocksResponse;
    } catch (e) {
      console.warn("Failed to load economy shocks:", e);
      return null;
    }
  }

  async function loadQuestSummary() {
    try {
      const data = await fetchJSON("/api/simverse/quest/list");
      return data as SimverseQuestSummary;
    } catch (e) {
      console.warn("Failed to load quest summary:", e);
      return null;
    }
  }

  async function claimQuest(questId: string) {
    try {
      const data = await fetchJSON("/api/simverse/quest/claim", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ quest_id: questId }),
      });
      return data as { success: boolean; reward: SimverseQuestReward };
    } catch (e) {
      console.warn(`Failed to claim quest ${questId}:`, e);
      return null;
    }
  }

  async function recordQuestAction(action: "view_npc" | "view_economy" | "gacha") {
    try {
      await fetchJSON("/api/simverse/quest/action", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action }),
      });
    } catch (e) {
      console.warn(`Failed to record quest action ${action}:`, e);
    }
  }

  async function loadSocialStats(region?: number, org?: number) {
    try {
      const qs: string[] = [];
      if (region) qs.push(`region=${region}`);
      if (org) qs.push(`org=${org}`);
      const q = qs.length ? `?${qs.join("&")}` : "";
      const data = await fetchJSON(`/api/simverse/social/stats${q}`);
      return data as SimverseSocialStats;
    } catch (e) {
      console.warn("Failed to load social stats:", e);
      return null;
    }
  }

  async function loadNPCRelations(id: number) {
    try {
      const data = await fetchJSON(`/api/simverse/npc/${id}/relations`);
      return data as SimverseRelationListResponse;
    } catch (e) {
      console.warn(`Failed to load relations for npc ${id}:`, e);
      return null;
    }
  }

  async function loadBattleRecent(limit = 20) {
    try {
      const data = await fetchJSON(`/api/simverse/battle/recent?limit=${limit}`);
      return data as SimverseBattleListResponse;
    } catch (e) {
      console.warn("Failed to load battle recent:", e);
      return null;
    }
  }

  async function loadBattleRank(limit = 20) {
    try {
      const data = await fetchJSON(`/api/simverse/battle/rank?limit=${limit}`);
      return data as SimverseBattleRankResponse;
    } catch (e) {
      console.warn("Failed to load battle rank:", e);
      return null;
    }
  }

  async function simulateBattle(attackerId: number, defenderId: number) {
    try {
      const data = await fetchJSON("/api/simverse/battle/simulate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ attacker_id: attackerId, defender_id: defenderId }),
      });
      return data as SimverseBattleSimulateResponse;
    } catch (e) {
      console.warn("Failed to simulate battle:", e);
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
      case "economy:update":
        economySignal.value++;
        break;
      case "chronicle:event":
        chronicleSignal.value++;
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

    economySignal,
    chronicleSignal,

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
    loadBehaviorStats,
    loadBehaviorList,

    loadEra,
    loadRegionList,
    loadRegionDetail,
    loadOrgList,
    loadOrgDetail,
    loadOrgMembers,
    loadOrgTerritory,
    loadEconomyPrices,
    loadEconomyShocks,
    loadQuestSummary,
    claimQuest,
    recordQuestAction,

    loadSocialStats,
    loadNPCRelations,

    loadBattleRecent,
    loadBattleRank,
    simulateBattle,

    connectWebSocket,
    disconnectWebSocket,
    sendWS,

    init,
    cleanup,
  };
}
