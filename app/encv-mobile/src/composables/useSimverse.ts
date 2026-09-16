import { ref } from "vue";

export interface SimverseChronicleEvent {
  id: number;
  tick: number;
  type_cn: string;
  level: string;
  level_cn: string;
  importance: number;
  imp_cn: string;
  entity_id?: number;
  causes?: Array<{ id: number; type_cn: string; tick: number; level_cn: string }>;
  effects?: Array<{ id: number; type_cn: string; tick: number; level_cn: string }>;
}

export interface SimverseChronicleWorldResponse {
  era: number;
  total_events: number;
  count: number;
  items: SimverseChronicleEvent[];
}

const currentTick = ref(0);

export function useSimverse() {
  async function loadChronicleWorld(_minImportance: number, _limit: number): Promise<SimverseChronicleWorldResponse> {
    return {
      era: 0,
      total_events: 0,
      count: 0,
      items: [],
    };
  }

  async function loadChronicleEvent(_id: number): Promise<SimverseChronicleEvent | null> {
    return null;
  }

  return {
    currentTick,
    loadChronicleWorld,
    loadChronicleEvent,
  };
}
