<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button @click="exitToMainApp">
            <ion-icon slot="icon-only" :icon="arrowBackOutline" />
          </ion-button>
        </ion-buttons>
        <ion-title>{{ t("simverse.home.title") }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content :fullscreen="true" class="home-content">
      <!-- Hero -->
      <div class="px-4 pt-8 pb-5">
        <div class="ui-card">
          <div class="p-6 text-center">
            <div class="ui-bubble hero-icon w-20 h-20 mx-auto mb-4 flex items-center justify-center text-4xl">
              🌍
            </div>
            <h1
              class="text-2xl font-bold m-0 mb-2 bg-gradient-to-br from-primary to-secondary bg-clip-text text-transparent"
            >
              {{ t("simverse.home.title") }}
            </h1>
            <p class="text-sm text-base-content/60 m-0">
              {{ t("simverse.home.subtitle") }}
            </p>
            <div class="flex gap-3 justify-center mt-5">
              <button type="button" class="ui-button" @click="goToWorld">
                <ion-icon :icon="gameControllerOutline" class="mr-2" />
                {{ t("simverse.home.enterWorld") }}
              </button>
              <button type="button" class="ui-button ui-button--ghost" @click="goToChronicle">
                <ion-icon :icon="documentTextOutline" class="mr-2" />
                {{ t("simverse.home.chronicle") }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Action cards -->
      <div class="grid grid-cols-2 gap-3 px-4 mb-5">
        <button
          type="button"
          class="ui-card hover:-translate-y-0.5 hover:shadow-lg active:scale-[0.97] transition-transform duration-200 cursor-pointer w-full text-left"
          @click="goToWorld"
        >
          <div class="p-4 flex flex-col items-center text-center">
            <div class="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center mb-3">
              <ion-icon :icon="gameControllerOutline" class="text-primary text-2xl" />
            </div>
            <h2 class="text-base font-semibold m-0 mb-1">
              {{ t("simverse.home.enterWorld") }}
            </h2>
            <p class="text-xs text-base-content/60 m-0">进入横屏模拟世界</p>
          </div>
        </button>

        <button
          type="button"
          class="ui-card hover:-translate-y-0.5 hover:shadow-lg active:scale-[0.97] transition-transform duration-200 cursor-pointer w-full text-left"
          @click="goToChronicle"
        >
          <div class="p-4 flex flex-col items-center text-center">
            <div class="w-12 h-12 rounded-full bg-secondary/10 flex items-center justify-center mb-3">
              <ion-icon :icon="documentTextOutline" class="text-secondary text-2xl" />
            </div>
            <h2 class="text-base font-semibold m-0 mb-1">
              {{ t("simverse.home.chronicle") }}
            </h2>
            <p class="text-xs text-base-content/60 m-0">查看世界历史事件</p>
          </div>
        </button>
      </div>

      <!-- Stats -->
      <div class="grid grid-cols-3 gap-2.5 px-4">
        <div class="ui-card">
          <div class="p-4 flex flex-col items-center text-center">
            <div class="text-2xl font-bold text-primary mb-1">{{ npcCount }}</div>
            <div class="text-[11px] uppercase tracking-[0.5px] text-base-content/60">
              {{ t("simverse.population") }}
            </div>
          </div>
        </div>
        <div class="ui-card">
          <div class="p-4 flex flex-col items-center text-center">
            <div class="text-2xl font-bold text-primary mb-1">
              {{ totalMemoryMB }}<span class="text-xs font-medium ml-0.5 text-base-content/60">MB</span>
            </div>
            <div class="text-[11px] uppercase tracking-[0.5px] text-base-content/60">
              {{ t("simverse.memory") }}
            </div>
          </div>
        </div>
        <div class="ui-card">
          <div class="p-4 flex flex-col items-center text-center">
            <div class="text-2xl font-bold text-primary mb-1">{{ era }}</div>
            <div class="text-[11px] uppercase tracking-[0.5px] text-base-content/60">
              {{ t("simverse.tick") }}
            </div>
          </div>
        </div>
      </div>

      <!-- Exit -->
      <div class="px-4 pt-6 pb-10">
        <button type="button" class="ui-button ui-button--ghost w-full" @click="exitToMainApp">
          <ion-icon :icon="exitOutline" class="mr-2" />
          返回主应用
        </button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { arrowBack, exit, gameController, documentText } from "ionicons/icons";
import { computed, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import { gsap } from "@/composables/useGsap";
import { useSimverse } from "@/composables/useSimverse";
import { closeWorld, isNativePluginMode, unlockScreenOrientation } from "@/plugins/SimVerse";

const { t } = useI18n();
const router = useRouter();
const { npcCount, totalMemoryMB, currentTick, loadWorldState } = useSimverse();

const era = computed(() => Math.floor(currentTick.value / 1000));

const arrowBackOutline = arrowBack;
const exitOutline = exit;
const gameControllerOutline = gameController;
const documentTextOutline = documentText;

let floatTween: gsap.core.Tween | null = null;

function goToWorld() {
  router.push("/world");
}

function goToChronicle() {
  router.push("/tabs/chronicles");
}

onMounted(() => {
  loadWorldState().catch(() => {});
  floatTween = gsap.to(".hero-icon", {
    y: -10,
    duration: 2,
    ease: "sine.inOut",
    yoyo: true,
    repeat: -1,
  });
});

onUnmounted(() => {
  if (floatTween) {
    floatTween.kill();
    floatTween = null;
  }
});

async function exitToMainApp() {
  try {
    if (isNativePluginMode()) {
      await unlockScreenOrientation();
      await closeWorld();
    } else {
      window.close();
    }
  } catch (e) {
    console.warn("[SimverseHome] Exit failed:", e);
  }
}
</script>

<style scoped lang="scss">
.home-content {
  --background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--color-primary) 8%, transparent) 0%,
    transparent 30%
  );
}
</style>
