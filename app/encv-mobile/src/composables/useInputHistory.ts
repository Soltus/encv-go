import { computed, ref } from "vue";

const HISTORY_KEY = "encv-input-history";
const MAX_HISTORY_PER_KEY = 10;
const MAX_GLOBAL = 200;

export interface HistoryEntry {
  key: string;
  value: string;
  timestamp: number;
}

const historyMap = ref<Record<string, HistoryEntry[]>>(loadFromStorage());

function loadFromStorage(): Record<string, HistoryEntry[]> {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (raw) return JSON.parse(raw);
  } catch (e) {
    console.error("[InputHistory] Failed to load from localStorage:", e);
  }
  return {};
}

function saveToStorage() {
  try {
    const allEntries: HistoryEntry[] = [];
    for (const entries of Object.values(historyMap.value)) {
      allEntries.push(...entries);
    }
    allEntries.sort((a, b) => b.timestamp - a.timestamp);
    const trimmed = allEntries.slice(0, MAX_GLOBAL);
    const map: Record<string, HistoryEntry[]> = {};
    for (const e of trimmed) {
      if (!map[e.key]) map[e.key] = [];
      map[e.key].push(e);
    }
    localStorage.setItem(HISTORY_KEY, JSON.stringify(map));
  } catch (e) {
    console.error("[InputHistory] Failed to save to localStorage:", e);
  }
}

export function recordHistory(inputKey: string, value: string) {
  if (!inputKey || !value || !value.trim()) return;
  const trimmed = value.trim();
  const existing = historyMap.value[inputKey] || [];
  const filtered = existing.filter(e => e.value !== trimmed);
  filtered.unshift({ key: inputKey, value: trimmed, timestamp: Date.now() });
  historyMap.value[inputKey] = filtered.slice(0, MAX_HISTORY_PER_KEY);
  saveToStorage();
}

export function getHistory(inputKey: string): HistoryEntry[] {
  return historyMap.value[inputKey] || [];
}

export function clearHistory(inputKey?: string) {
  if (inputKey) {
    delete historyMap.value[inputKey];
  } else {
    historyMap.value = {};
  }
  saveToStorage();
}

export function useInputHistory(inputKey: string) {
  const entries = computed(() => getHistory(inputKey));
  return {
    entries,
    record: (value: string) => recordHistory(inputKey, value),
    clear: () => clearHistory(inputKey),
  };
}
