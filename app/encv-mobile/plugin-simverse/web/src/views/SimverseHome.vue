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
      <div class="hero-section text-center px-5 pt-10 pb-5">
        <div class="hero-icon text-[72px] mb-4">🌍</div>
        <h1
          class="text-2xl font-bold m-0 mb-2 bg-gradient-to-br from-primary to-secondary bg-clip-text text-transparent"
        >
          {{ t("simverse.home.title") }}
        </h1>
        <p class="text-sm text-base-content/60 m-0">
          {{ t("simverse.home.subtitle") }}
        </p>
      </div>

      <!-- Action cards -->
      <div class="grid grid-cols-2 gap-3 px-4 mb-5">
        <button
          type="button"
          class="card action-card enter-world bg-base-100 shadow-md border-2 border-primary/30 hover:-translate-y-0.5 hover:shadow-lg active:scale-[0.97] transition-transform duration-200 cursor-pointer w-full text-left"
          @click="goToWorld"
        >
          <div class="card-body items-center text-center p-4">
            <div class="text-4xl mb-2">🎮</div>
            <h2 class="card-title text-base font-semibold justify-center">
              {{ t("simverse.home.enterWorld") }}
            </h2>
            <p class="text-xs text-base-content/60">进入横屏模拟世界</p>
          </div>
        </button>

        <button
          type="button"
          class="card action-card bg-base-100 shadow-md border-2 border-base-300 hover:-translate-y-0.5 hover:shadow-lg active:scale-[0.97] transition-transform duration-200 cursor-pointer w-full text-left"
          @click="goToChronicle"
        >
          <div class="card-body items-center text-center p-4">
            <div class="text-4xl mb-2">📜</div>
            <h2 class="card-title text-base font-semibold justify-center">
              {{ t("simverse.home.chronicle") }}
            </h2>
            <p class="text-xs text-base-content/60">查看世界历史事件</p>
          </div>
        </button>
      </div>

      <!-- Stats -->
      <div class="grid grid-cols-3 gap-2.5 px-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body items-center text-center p-4">
            <div class="text-2xl font-bold text-primary mb-1">{{ npcCount }}</div>
            <div class="text-[11px] uppercase tracking-[0.5px] text-base-content/60">
              {{ t("simverse.population") }}
            </div>
          </div>
        </div>
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body items-center text-center p-4">
            <div class="text-2xl font-bold text-primary mb-1">
              {{ totalMemoryMB }}<span class="text-xs font-medium ml-0.5 text-base-content/60">MB</span>
            </div>
            <div class="text-[11px] uppercase tracking-[0.5px] text-base-content/60">
              {{ t("simverse.memory") }}
            </div>
          </div>
        </div>
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body items-center text-center p-4">
            <div class="text-2xl font-bold text-primary mb-1">{{ era }}</div>
            <div class="text-[11px] uppercase tracking-[0.5px] text-base-content/60">
              {{ t("simverse.tick") }}
            </div>
          </div>
        </div>
      </div>

      <!-- Exit -->
      <div class="px-4 pt-6 pb-10">
        <ion-button expand="block" fill="outline" color="medium" @click="exitToMainApp">
          <ion-icon slot="start" :icon="exitOutline" />
          返回主应用
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { arrowBack, exit } from "ionicons/icons";
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

// GSAP float tween for hero icon (replaces the removed CSS keyframe)
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
// Ionic content background: subtle primary-tinted gradient using daisyUI tokens.
// color-mix keeps runtime theme switching (light/dark) working without hardcoded RGB.
.home-content {
  --background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--color-primary) 8%, transparent) 0%,
    transparent 30%
  );
}
</style>
