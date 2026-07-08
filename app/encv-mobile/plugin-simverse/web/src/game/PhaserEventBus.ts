import type { SimverseNPC } from "@/composables/useSimverse";

type EventCallback = (...args: any[]) => void;

class PhaserEventBus {
  private events: Map<string, Set<EventCallback>> = new Map();

  on(event: string, callback: EventCallback): void {
    if (!this.events.has(event)) {
      this.events.set(event, new Set());
    }
    this.events.get(event)!.add(callback);
  }

  off(event: string, callback: EventCallback): void {
    this.events.get(event)?.delete(callback);
  }

  emit(event: string, ...args: any[]): void {
    this.events.get(event)?.forEach((cb) => cb(...args));
  }

  clear(): void {
    this.events.clear();
  }
}

export const phaserEventBus = new PhaserEventBus();

export const PHASER_EVENTS = {
  NPC_CLICK: "npc:click",
  NPC_HOVER: "npc:hover",
  WORLD_READY: "world:ready",
  CAMERA_MOVE: "camera:move",
  CAMERA_ZOOM: "camera:zoom",
} as const;

export interface NPCSpriteData {
  npc: SimverseNPC;
  x: number;
  y: number;
}
