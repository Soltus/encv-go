<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.npcDetail") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="page-root p-4 space-y-4">
      <div v-if="loading" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
      </div>

      <div v-else-if="npc" class="detail">
        <div class="hero">
          <div class="ui-bubble !w-14 !h-14 !text-3xl">
            <span>{{ avatarEmoji }}</span>
          </div>
          <div class="hero-info">
            <h2 class="text-lg font-semibold">{{ npc.name }}</h2>
            <div class="flex items-center gap-2 flex-wrap mt-1">
              <span class="ui-chip !text-xs !py-0.5" :class="professionChipClass(npc.profession)">
                {{ npc.profession }}
              </span>
              <span class="text-xs text-base-content/70 font-medium">Lv.{{ npc.level }}</span>
              <span :class="npc.is_alive ? 'text-success' : 'text-base-content/50'" class="text-xs font-medium">
                {{ npc.is_alive ? t("simverse.alive") : t("simverse.deceased") }}
              </span>
            </div>
            <p class="text-xs text-base-content/70 mt-1">
              {{ npc.age }}{{ t("simverse.yearsOld") }} · {{ npc.species }} · {{ npc.life_stage }}
              <span v-if="npc.current_behavior_cn">· {{ npc.current_behavior_cn }}</span>
            </p>
          </div>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.stats") }}</div>
            <div class="space-y-1">
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.health") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ Math.round(npc.health) }}/{{ npc.max_health }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.energy") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ Math.round(npc.energy) }}/{{ npc.max_energy }}</span>
              </div>
              <div v-if="npc.mana !== undefined" class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.detail.mana") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ Math.round(npc.mana) }}/{{ npc.max_mana }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.wealthTier") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ npc.wealth_tier }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.socialTier") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ npc.social_tier }}</span>
              </div>
            </div>
          </div>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.actions") }}</div>
            <div class="space-y-1">
              <div
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="goSub('inventory')"
              >
                <ion-icon :icon="bagOutline" class="text-primary text-xl" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ t("simverse.detail.viewInventory") }}</div>
                </div>
                <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
              </div>
              <div
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="goSub('timeline')"
              >
                <ion-icon :icon="timeOutline" class="text-tertiary text-xl" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ t("simverse.detail.viewTimeline") }}</div>
                </div>
                <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
              </div>
              <div
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="goSub('relations')"
              >
                <ion-icon :icon="gitNetworkOutline" class="text-success text-xl" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ t("simverse.detail.viewRelations") }}</div>
                </div>
                <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
              </div>
            </div>
          </div>
        </div>
      </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, bagOutline, chevronForwardOutline, gitNetworkOutline, refreshOutline, timeOutline } from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useGsap } from "@/composables/useGsap";
import { useRouteTransition } from "@/composables/useRouteTransition";
import { type SimverseNPCDetail, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadNPCDetail } = useSimverse();
const { gsap } = useGsap();
useRouteTransition();

const loading = ref(false);
const error = ref("");
const npc = ref<SimverseNPCDetail | null>(null);

const avatarEmoji = computed(() => {
  const avatars = ["🧙", "⚔️", "🛡️", "🧑‍🌾", "👨‍🍳", "🗡️", "🏹", "📜", "💎", "🪓"];
  const id = Number(route.params.id) || 0;
  return avatars[id % avatars.length];
});

function professionChipClass(profession: string): string {
  const map: Record<string, string> = {
    farmer: "!bg-success/15 !text-success !border-success/30",
    warrior: "!bg-error/15 !text-error !border-error/30",
    mage: "!bg-primary/15 !text-primary !border-primary/30",
    merchant: "!bg-warning/15 !text-warning !border-warning/30",
    priest: "!bg-tertiary/15 !text-tertiary !border-tertiary/30",
    rogue: "!bg-base-content/15 !text-base-content/70 !border-base-content/20",
  };
  return map[profession?.toLowerCase()] || "!bg-base-content/15 !text-base-content/70 !border-base-content/20";
}

function goSub(which: "inventory" | "timeline" | "relations") {
  router.push(`/npc/${route.params.id}/${which}`);
}

async function reload() {
  const id = Number(route.params.id);
  if (!id) return;
  loading.value = true;
  error.value = "";
  try {
    npc.value = await loadNPCDetail(id);
  } catch (e: any) {
    error.value = e.message || "Failed to load NPC";
  } finally {
    loading.value = false;
  }
}
onMounted(() => {
  reload();
  const el = document.querySelector(".page-root");
  if (el) {
    gsap.fromTo(el, { opacity: 0, y: 24 }, { opacity: 1, y: 0, duration: 0.35, ease: "power2.out" });
  }
});
watch(() => route.params.id, reload);
</script>

<style scoped lang="scss">
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-container p { color: var(--color-error); margin: 0; }
.hero { display: flex; align-items: center; gap: 16px; padding: 8px 4px 16px; }
.hero-info h2 { margin: 0; }
</style>
