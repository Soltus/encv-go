<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.squad") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="clearSquad" :disabled="!squadIds.length">{{ t("simverse.squadClear") }}</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <div v-if="loading" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <template v-else>
        <!-- 编队席位 -->
        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.squadSlots") }} ({{ squadIds.length }}/6)</ion-label>
          </ion-list-header>
          <div class="slot-grid">
            <div
              v-for="i in 6"
              :key="i"
              class="slot"
              :class="squadMembers[i - 1] ? 'filled' : 'empty'"
            >
              <template v-if="squadMembers[i - 1]">
                <div class="slot-avatar">{{ initial(squadMembers[i - 1].name) }}</div>
                <div class="slot-name">{{ squadMembers[i - 1].name }}</div>
                <ion-badge :color="archColor(deriveBuildFromNPC(squadMembers[i - 1]).primary)" class="slot-badge">
                  {{ archLabel(deriveBuildFromNPC(squadMembers[i - 1]).primary) }}
                </ion-badge>
                <ion-button
                  fill="clear"
                  size="small"
                  class="slot-remove"
                  @click="removeFromSquad(squadMembers[i - 1].id)"
                >
                  {{ t("simverse.squadRemove") }}
                </ion-button>
              </template>
              <template v-else>
                <div class="slot-plus">+</div>
              </template>
            </div>
          </div>
          <p v-if="!squadIds.length" class="hint">{{ t("simverse.squadEmpty") }}</p>
        </ion-list>

        <!-- 羁绊（自走棋式 2/4/6 协同） -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.squadSynergy") }}</ion-label></ion-list-header>
          <ion-item v-for="s in synergyResult" :key="s.key">
            <ion-label>
              <ion-badge :color="archColor(s.key)">{{ archLabel(s.key) }}</ion-badge>
              <span class="syn-count"> ×{{ s.count }} · {{ t("simverse.synergyTier" + s.tier) }}</span>
            </ion-label>
            <ion-note slot="end" color="success">{{ t("simverse.synergyActive") }}</ion-note>
          </ion-item>
          <p v-if="!synergyResult.length" class="hint">{{ t("simverse.synergyHint") }}</p>
        </ion-list>

        <!-- 候选角色 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.squadPick") }}</ion-label></ion-list-header>
          <ion-searchbar v-model="filter" :placeholder="t('simverse.search')" />
          <ion-item
            v-for="npc in filteredPool"
            :key="npc.id"
            :disabled="isFull && !inSquad(npc.id)"
          >
            <ion-label>
              <h3>{{ npc.name }}</h3>
              <p>{{ npc.profession }} · Lv.{{ npc.level }}</p>
            </ion-label>
            <ion-badge slot="end" :color="archColor(deriveBuildFromNPC(npc).primary)" outline>
              {{ archLabel(deriveBuildFromNPC(npc).primary) }}
            </ion-badge>
            <ion-button
              slot="end"
              fill="clear"
              size="small"
              :disabled="inSquad(npc.id) || isFull"
              @click="addToSquad(npc.id)"
            >
              {{ inSquad(npc.id) ? "✓" : t("simverse.squadAdd") }}
            </ion-button>
          </ion-item>
          <p v-if="!filteredPool.length" class="hint">{{ t("simverse.squadFull") }}</p>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSearchbar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseNPC, useSimverse } from "@/composables/useSimverse";
import { type ArchetypeKey, deriveBuildFromNPC } from "@/game/builds";

const { t } = useI18n();
const router = useRouter();
const { loadNPCList } = useSimverse();

const SQUAD_KEY = "simverse:squad";
const MAX_SQUAD = 6;

const loading = ref(false);
const pool = ref<SimverseNPC[]>([]);
const squadIds = ref<number[]>([]);
const filter = ref("");

const isFull = computed(() => squadIds.value.length >= MAX_SQUAD);

const squadMembers = computed(() =>
  squadIds.value.map(id => pool.value.find(n => n.id === id)).filter((n): n is SimverseNPC => Boolean(n))
);

const squadBuilds = computed(() => squadMembers.value.map(m => deriveBuildFromNPC(m)));

// 自走棋式羁绊：同流派主流派数量达 2/4/6 触发（初/盛/极）
const synergyResult = computed(() => {
  const counts = {} as Record<ArchetypeKey, number>;
  (["warrior", "guardian", "scholar", "merchant", "artisan", "healer", "leader", "hermit", "rogue", "artist"] as ArchetypeKey[]).forEach(
    a => (counts[a] = 0)
  );
  squadBuilds.value.forEach(b => {
    counts[b.primary]++;
  });
  const out: { key: ArchetypeKey; count: number; tier: number }[] = [];
  (Object.keys(counts) as ArchetypeKey[]).forEach(a => {
    const c = counts[a];
    if (c >= 2) out.push({ key: a, count: c, tier: c >= 6 ? 3 : c >= 4 ? 2 : 1 });
  });
  return out.sort((x, y) => y.count - x.count);
});

const filteredPool = computed(() => {
  const q = filter.value.trim().toLowerCase();
  return pool.value.filter(n => !q || (n.name || "").toLowerCase().includes(q)).slice(0, 80);
});

function inSquad(id: number): boolean {
  return squadIds.value.includes(id);
}
function initial(name: string): string {
  return String(name || "?").charAt(0);
}
function archLabel(key: ArchetypeKey): string {
  return t(`simverse.build.${key}`);
}
const ARCH_COLOR: Record<ArchetypeKey, string> = {
  warrior: "danger",
  guardian: "warning",
  scholar: "primary",
  merchant: "success",
  artisan: "tertiary",
  healer: "success",
  leader: "secondary",
  hermit: "medium",
  rogue: "dark",
  artist: "tertiary",
};
function archColor(key: ArchetypeKey): string {
  return ARCH_COLOR[key] || "medium";
}

function persist() {
  try {
    localStorage.setItem(SQUAD_KEY, JSON.stringify(squadIds.value));
  } catch (e) {
    console.warn("[simverse] squad persist failed:", e);
  }
}
function loadSquad(): number[] {
  try {
    const v = JSON.parse(localStorage.getItem(SQUAD_KEY) || "[]");
    return Array.isArray(v) ? v.filter(x => typeof x === "number").slice(0, MAX_SQUAD) : [];
  } catch {
    return [];
  }
}

function addToSquad(id: number) {
  if (inSquad(id) || isFull.value) return;
  squadIds.value = [...squadIds.value, id];
  persist();
}
function removeFromSquad(id: number) {
  squadIds.value = squadIds.value.filter(x => x !== id);
  persist();
}
function clearSquad() {
  squadIds.value = [];
  persist();
}
function goNPC(id: number) {
  router.push(`/npc/${id}`);
}

onMounted(async () => {
  loading.value = true;
  squadIds.value = loadSquad();
  try {
    const data = await loadNPCList(1, 200);
    pool.value = data.items || [];
  } catch (e) {
    console.warn("[simverse] failed to load NPC list for squad:", e);
  } finally {
    loading.value = false;
  }
});
</script>

<style scoped>
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.slot-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  padding: 8px 4px;
}
.slot {
  border-radius: 12px;
  padding: 10px 6px;
  text-align: center;
  min-height: 96px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
}
.slot.filled {
  background: var(--ion-color-light, #f3f4f6);
  border: 1px solid var(--ion-color-step-200, #e5e7eb);
}
.slot.empty {
  border: 1px dashed var(--ion-color-medium, #9ca3af);
  color: var(--ion-color-medium);
}
.slot-avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: var(--ion-color-primary);
  color: #fff;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}
.slot-name {
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}
.slot-badge {
  font-size: 10px;
}
.slot-plus {
  font-size: 28px;
  font-weight: 300;
}
.syn-count {
  font-size: 13px;
  color: var(--ion-color-medium);
  margin-left: 6px;
}
.hint {
  font-size: 12px;
  color: var(--ion-color-medium);
  padding: 4px 12px 10px;
  margin: 0;
}
</style>
