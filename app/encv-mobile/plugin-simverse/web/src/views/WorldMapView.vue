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

    <ion-content class="ion-padding">
      <div v-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
      </div>

      <div class="world-hero">
        <div ref="worldIconRef" class="world-icon">🌍</div>
        <h2>{{ t("simverse.home.title") }}</h2>
        <p class="world-sub">{{ t("simverse.home.subtitle") }}</p>
        <ion-badge :color="isRunning ? 'success' : 'medium'" size="large">
          {{ isRunning ? t("simverse.intervention.running") : t("simverse.intervention.stopped") }}
        </ion-badge>
      </div>

      <ion-list :inset="true">
        <ion-list-header><ion-label>{{ t("simverse.configDetails") }}</ion-label></ion-list-header>
        <ion-item>
          <ion-label>{{ t("simverse.tick") }}</ion-label>
          <ion-note slot="end">{{ currentTick }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.population") }}</ion-label>
          <ion-note slot="end">{{ npcCount }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.memory") }}</ion-label>
          <ion-note slot="end">{{ totalMemoryMB.toFixed(1) }} MB</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.perfTier") }}</ion-label>
          <ion-note slot="end">{{ currentTier }}</ion-note>
        </ion-item>
      </ion-list>

      <div class="entry-grid">
        <ion-button expand="block" color="primary" @click="enterWorld">
          🎮 {{ t("simverse.home.enterWorld") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goBehavior">
          🧠 {{ t("simverse.behavior") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goChronicles">
          📜 {{ t("simverse.chronicles") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goIntervention">
          🛠️ {{ t("simverse.intervention") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goSettings">
          ⚙️ {{ t("simverse.settings") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goQuests">
          📜 {{ t("simverse.quests") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goSocial">
          💞 {{ t("simverse.social") }}
        </ion-button>
        <ion-button expand="block" fill="outline" @click="goSquad">
          🃏 {{ t("simverse.squad") }}
        </ion-button>
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
  IonListHeader,
  IonNote,
  IonPage,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
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
