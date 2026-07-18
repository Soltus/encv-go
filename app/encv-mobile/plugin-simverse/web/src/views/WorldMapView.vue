<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/home" />
        </ion-buttons>
        <ion-title>{{ t("simverse.tabs.world") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-4">
        <div v-if="error" class="state-container">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
        </div>

        <div class="world-hero">
          <div ref="worldIconRef" class="world-icon">🌍</div>
          <h2>{{ t("simverse.home.title") }}</h2>
          <p class="world-sub">{{ t("simverse.home.subtitle") }}</p>
          <span class="ui-chip" :class="isRunning ? '!bg-success/15 !text-success !border-success/30' : '!bg-base-content/15 !text-base-content/70 !border-base-content/20'">
            {{ isRunning ? t("simverse.intervention.running") : t("simverse.intervention.stopped") }}
          </span>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.configDetails") }}</div>
            <div class="space-y-1">
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.tick") }}</span>
                <span class="text-sm text-base-content/70 font-mono">{{ currentTick }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.population") }}</span>
                <span class="text-sm text-base-content/70 font-mono">{{ npcCount }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.memory") }}</span>
                <span class="text-sm text-base-content/70 font-mono">{{ totalMemoryMB.toFixed(1) }} MB</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.perfTier") }}</span>
                <span class="text-sm text-base-content/70 font-mono">{{ currentTier }}</span>
              </div>
            </div>
          </div>
        </div>

        <div class="entry-grid">
          <button type="button" class="ui-button w-full" @click="enterWorld">
            <ion-icon :icon="gameControllerOutline" class="mr-2" />
            {{ t("simverse.home.enterWorld") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goBehavior">
            <ion-icon :icon="sparklesOutline" class="mr-2" />
            {{ t("simverse.behavior") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goChronicles">
            <ion-icon :icon="documentTextOutline" class="mr-2" />
            {{ t("simverse.chronicles") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goIntervention">
            <ion-icon :icon="constructOutline" class="mr-2" />
            {{ t("simverse.intervention") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goSettings">
            <ion-icon :icon="settingsOutline" class="mr-2" />
            {{ t("simverse.settings") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goQuests">
            <ion-icon :icon="ribbonOutline" class="mr-2" />
            {{ t("simverse.quests") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goSocial">
            <ion-icon :icon="heartOutline" class="mr-2" />
            {{ t("simverse.social") }}
          </button>
          <button type="button" class="ui-button w-full !bg-base-200 !text-base-content hover:!bg-base-300" @click="goSquad">
            <ion-icon :icon="peopleOutline" class="mr-2" />
            {{ t("simverse.squad") }}
          </button>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import {
  alertCircleOutline,
  constructOutline,
  documentTextOutline,
  gameControllerOutline,
  heartOutline,
  peopleOutline,
  refreshOutline,
  ribbonOutline,
  settingsOutline,
  sparklesOutline,
} from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useGsap } from "@/composables/useGsap";
import { useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { gsap } = useGsap();
const { isRunning, currentTick, npcCount, totalMemoryMB, currentTier, loadWorldState } = useSimverse();

const error = ref("");
const worldIconRef = ref<HTMLElement | null>(null);

function enterWorld() {
  router.push("/world");
}
function goBehavior() {
  router.push("/world/behavior");
}
function goChronicles() {
  router.push("/world/chronicles");
}
function goIntervention() {
  router.push("/world/intervention");
}
function goSettings() {
  router.push("/world/settings");
}
function goQuests() {
  router.push("/world/quests");
}
function goSocial() {
  router.push("/world/social");
}
function goSquad() {
  router.push("/world/squad");
}

async function reload() {
  try {
    await loadWorldState();
  } catch (e: any) {
    error.value = e.message || "Failed to load world state";
  }
}

onMounted(() => {
  reload();
  if (worldIconRef.value) {
    gsap.to(worldIconRef.value, { y: -8, duration: 1.5, ease: "sine.inOut", repeat: -1, yoyo: true });
  }
});
</script>

<style scoped lang="scss">
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;

  p {
    color: var(--color-error);
    margin: 0;
  }
}

.world-hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 24px 16px 16px;

  h2 {
    margin: 0 0 4px;
    font-size: 20px;
  }
}

.world-icon {
  font-size: 56px;
  margin-bottom: 8px;
}

.world-sub {
  color: var(--color-base-content);
  opacity: 0.7;
  font-size: 13px;
  margin: 0 0 12px;
}

.entry-grid {
  display: grid;
  gap: 10px;
  padding: 8px 0 24px;
}
</style>
