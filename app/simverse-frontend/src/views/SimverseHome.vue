<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>SimVerse 模拟世界</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="goToSettings">
            <ion-icon :icon="settingsOutline" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 世界概览卡片 -->
      <div class="world-overview card">
        <h2>{{ eraName }}</h2>
        <p>Tick: {{ worldState?.tick || 0 }}</p>
        <p>NPC 数: {{ worldState?.npcCount || 0 }}</p>
      </div>

      <!-- 进入世界按钮 -->
      <ion-button expand="block" class="enter-world-btn" @click="enterWorld">
        <ion-icon :icon="planetOutline" slot="start" />
        进入世界
      </ion-button>

      <!-- 快速入口 -->
      <div class="quick-actions">
        <ion-button fill="outline" @click="goToChronicles">
          <ion-icon :icon="documentTextOutline" slot="start" />
          编年史
        </ion-button>
        <ion-button fill="outline" @click="goToSettings">
          <ion-icon :icon="settingsOutline" slot="start" />
          设置
        </ion-button>
        <ion-button fill="outline" @click="goToDevLogs">
          <ion-icon :icon="listOutline" slot="start" />
          日志
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useSimverseStore } from "@self/stores/simverse";
import { computed, onMounted } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();
const store = useSimverseStore();

const _worldState = computed(() => store.worldState);
const _eraName = computed(() => `时代 ${Math.floor((store.worldState?.tick || 0) / 1000)}`);

onMounted(async () => {
  await store.fetchWorldState();
});

const _enterWorld = () => {
  router.push("/world");
};

const _goToChronicles = () => {
  router.push("/chronicle");
};

const _goToSettings = () => {
  router.push("/tabs/settings");
};

const _goToDevLogs = () => {
  router.push("/tabs/devlogs");
};
</script>

<style scoped>
.world-overview {
  padding: 20px;
  margin: 20px;
  border-radius: 12px;
  background: var(--ion-background-color, #fff);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.world-overview h2 {
  margin: 0 0 12px 0;
  font-size: 24px;
}

.world-overview p {
  margin: 4px 0;
  color: var(--ion-color-medium);
}

.enter-world-btn {
  margin: 20px;
  font-size: 18px;
  font-weight: bold;
}

.quick-actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 20px;
}
</style>
