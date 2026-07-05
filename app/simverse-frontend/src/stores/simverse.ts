import type { Chronicle, NPC, WorldState } from "@shared/types/simverse";
import { defineStore } from "pinia";

export const useSimverseStore = defineStore("simverse", {
  state: () => ({
    worldState: null as WorldState | null,
    npcs: [] as NPC[],
    chronicles: [] as Chronicle[],
    focusNPCs: [] as string[],
    performanceTier: "mid" as "low" | "mid" | "high",
    isConnected: false,
    lastTick: 0,
  }),

  getters: {
    activeNPCCount: state => state.npcs.filter(n => n.status === "active").length,
    eraName: state => `时代 ${Math.floor(state.worldState?.tick || 0) / 1000}`,
  },

  actions: {
    async fetchWorldState() {
      try {
        const apiBase = import.meta.env.VITE_API_BASE || "http://localhost:8080";
        const res = await fetch(`${apiBase}/api/simverse/world/state`);
        if (res.ok) {
          this.worldState = await res.json();
          this.lastTick = this.worldState?.tick || 0;
        }
      } catch (error) {
        console.error("Failed to fetch world state:", error);
      }
    },

    async setFocusNPCs(npcIds: string[]) {
      this.focusNPCs = npcIds;
      try {
        const apiBase = import.meta.env.VITE_API_BASE || "http://localhost:8080";
        await fetch(`${apiBase}/api/simverse/focus`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ npcs: npcIds.map(id => ({ id, level: "core" })) }),
        });
      } catch (error) {
        console.error("Failed to set focus NPCs:", error);
      }
    },

    async setPerformanceTier(tier: "low" | "mid" | "high") {
      this.performanceTier = tier;
      try {
        const apiBase = import.meta.env.VITE_API_BASE || "http://localhost:8080";
        await fetch(`${apiBase}/api/simverse/world/config`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ performance_tier: tier }),
        });
      } catch (error) {
        console.error("Failed to set performance tier:", error);
      }
    },
  },
});
