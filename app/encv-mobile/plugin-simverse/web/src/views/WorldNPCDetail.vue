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
      <div class="page-root">
      <div v-if="loading" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>

      <div v-else-if="npc" class="detail">
        <div class="hero">
          <div class="hero-avatar">{{ avatarEmoji }}</div>
          <div class="hero-info">
            <h2>{{ npc.name }}</h2>
            <p>
              <ion-badge :color="professionColor(npc.profession)" size="small">{{ npc.profession }}</ion-badge>
              <span class="hero-meta">Lv.{{ npc.level }}</span>
              <span :class="npc.is_alive ? 'alive' : 'dead'">
                {{ npc.is_alive ? t("simverse.alive") : t("simverse.deceased") }}
              </span>
            </p>
            <p class="hero-sub">
              {{ npc.age }}{{ t("simverse.yearsOld") }} · {{ npc.species }} · {{ npc.life_stage }}
              <span v-if="npc.current_behavior_cn">· {{ npc.current_behavior_cn }}</span>
            </p>
          </div>
        </div>

        <ion-list :inset="true">
          <ion-item>
            <ion-label>{{ t("simverse.health") }}</ion-label>
            <ion-note slot="end">{{ Math.round(npc.health) }}/{{ npc.max_health }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.energy") }}</ion-label>
            <ion-note slot="end">{{ Math.round(npc.energy) }}/{{ npc.max_energy }}</ion-note>
          </ion-item>
          <ion-item v-if="npc.mana !== undefined">
            <ion-label>{{ t("simverse.detail.mana") }}</ion-label>
            <ion-note slot="end">{{ Math.round(npc.mana) }}/{{ npc.max_mana }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.wealthTier") }}</ion-label>
            <ion-note slot="end">{{ npc.wealth_tier }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.socialTier") }}</ion-label>
            <ion-note slot="end">{{ npc.social_tier }}</ion-note>
          </ion-item>
        </ion-list>

        <ion-list :inset="true">
          <ion-item button detail @click="goSub('inventory')">
            <ion-icon :icon="bagOutline" slot="start" color="primary" />
            <ion-label>{{ t("simverse.detail.viewInventory") }}</ion-label>
          </ion-item>
          <ion-item button detail @click="goSub('timeline')">
            <ion-icon :icon="timeOutline" slot="start" color="tertiary" />
            <ion-label>{{ t("simverse.detail.viewTimeline") }}</ion-label>
          </ion-item>
          <ion-item button detail @click="goSub('relations')">
            <ion-icon :icon="gitNetworkOutline" slot="start" color="success" />
            <ion-label>{{ t("simverse.detail.viewRelations") }}</ion-label>
          </ion-item>
        </ion-list>
      </div>
      </div>
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
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonNote,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, bagOutline, gitNetworkOutline, refreshOutline, timeOutline } from "ionicons/icons";
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
// useRouteTransition exposes onEnter/onLeave hooks for a parent <Transition> wrapper.
// App.vue currently uses plain ion-router-outlet, so this view self-animates
// .page-root via gsap.fromTo as a fallback (matches useRouteTransition defaults).
useRouteTransition();

const loading = ref(false);
const error = ref("");
const npc = ref<SimverseNPCDetail | null>(null);

const avatarEmoji = computed(() => {
  const avatars = ["🧙", "⚔️", "🛡️", "🧑‍🌾", "👨‍🍳", "🗡️", "🏹", "📜", "💎", "🪓"];
  const id = Number(route.params.id) || 0;
  return avatars[id % avatars.length];
});

function professionColor(profession: string): string {
  const map: Record<string, string> = {
    farmer: "success",
    warrior: "danger",
    mage: "primary",
    merchant: "warning",
    priest: "tertiary",
    rogue: "medium",
  };
  return map[profession?.toLowerCase()] || "medium";
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
  // 详情路由入场动效（与 useRouteTransition 默认参数一致）
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
.hero { display: flex; align-items: center; gap: 16px; padding: 20px 16px 8px; }
.hero-avatar {
  width: 56px; height: 56px; border-radius: 50%;
  background: var(--color-base-200);
  display: flex; align-items: center; justify-content: center;
  font-size: 30px; flex-shrink: 0;
}
.hero-info h2 { margin: 0 0 4px; font-size: 18px; }
.hero-info p { margin: 0 0 4px; font-size: 12px; color: var(--color-base-content); opacity: 0.7; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.hero-meta { font-weight: 600; }
.alive { color: var(--color-success); }
.dead { color: var(--color-base-content); opacity: 0.6; }
</style>
