<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ title }}</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <div class="p-4 flex flex-col items-center justify-center min-h-[60vh] gap-4">
        <div class="text-5xl opacity-60">{{ icon }}</div>
        <h2 class="text-2xl font-bold">{{ title }}</h2>
        <p class="text-base-content/60 text-center max-w-md">
          该功能正在重构中，敬请期待。这是混合架构拆分后的详情路由占位页面，将在 P3/P4 阶段实现完整功能。
        </p>
        <button type="button" class="ui-button" @click="goBack">返回世界</button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import {
  IonPage,
  IonHeader,
  IonToolbar,
  IonTitle,
  IonContent,
  IonButtons,
  IonBackButton,
} from "@ionic/vue";

const router = useRouter();

const title = computed(() => "化身资料");
const icon = computed(() => "🧑");

function goBack() {
  router.push("/world");
}
</script>
