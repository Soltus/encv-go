<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.settings") }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="settings-layout">
        <!-- 左侧分类导航（手游左右布局） -->
        <div class="settings-nav">
          <button
            v-for="cat in categories"
            :key="cat.key"
            class="nav-item"
            :class="{ active: activeCategory === cat.key }"
            @click="activeCategory = cat.key"
          >
            <ion-icon :icon="cat.icon" />
            <span>{{ cat.label }}</span>
          </button>
        </div>

        <!-- 右侧选项面板 -->
        <div class="settings-panel">
          <!-- 画面设置 -->
          <section v-if="activeCategory === 'graphics'" class="panel-section">
            <h3 class="section-title">{{ t("simverse.graphics") }}</h3>

            <div class="option-block">
              <div class="option-label">{{ t("simverse.frameRate") }}</div>
              <ion-segment :value="String(fps)" @ionChange="onFpsChange" scrollable>
                <ion-segment-button v-for="opt in FPS_OPTIONS" :key="opt" :value="String(opt)">
                  <ion-label>{{ opt }}</ion-label>
                </ion-segment-button>
              </ion-segment>
            </div>

            <div class="option-block">
              <div class="option-label">{{ t("simverse.renderQuality") }}</div>
              <ion-segment :value="quality" @ionChange="onQualityChange">
                <ion-segment-button v-for="opt in QUALITY_OPTIONS" :key="opt.value" :value="opt.value">
                  <ion-label>{{ opt.label }}</ion-label>
                </ion-segment-button>
              </ion-segment>
              <p class="option-hint">
                {{ QUALITY_RESOLUTION[quality].width }} × {{ QUALITY_RESOLUTION[quality].height }}
              </p>
            </div>
          </section>

          <!-- 性能档位 -->
          <section v-else-if="activeCategory === 'performance'" class="panel-section">
            <h3 class="section-title">{{ t("simverse.perfTier") }}</h3>
            <div class="option-block">
              <ion-segment :value="currentTier" @ionChange="onTierChange">
                <ion-segment-button value="background">
                  <ion-label>{{ t("simverse.tierBackground") }}</ion-label>
                </ion-segment-button>
                <ion-segment-button value="foreground">
                  <ion-label>{{ t("simverse.tierForeground") }}</ion-label>
                </ion-segment-button>
                <ion-segment-button value="fg_idle">
                  <ion-label>{{ t("simverse.tierIdle") }}</ion-label>
                </ion-segment-button>
              </ion-segment>
            </div>
          </section>

          <!-- 关于 -->
          <section v-else class="panel-section">
            <h3 class="section-title">{{ t("simverse.about.title") }}</h3>
            <p class="about-desc">{{ t("simverse.about.desc") }}</p>
            <ion-list :inset="true">
              <ion-item>
                <ion-label>{{ t("simverse.about.version") }}</ion-label>
                <ion-note slot="end">v0.1.0</ion-note>
              </ion-item>
              <ion-item>
                <ion-label>{{ t("simverse.frameRate") }}</ion-label>
                <ion-note slot="end">{{ fps }} FPS</ion-note>
              </ion-item>
              <ion-item>
                <ion-label>{{ t("simverse.renderQuality") }}</ion-label>
                <ion-note slot="end">{{ quality.toUpperCase() }}</ion-note>
              </ion-item>
            </ion-list>
          </section>
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
  IonItem,
  IonLabel,
  IonList,
  IonNote,
  IonPage,
  IonSegment,
  IonSegmentButton,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { brushOutline, informationCircleOutline, speedometerOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type PerfTier, useSimverse } from "@/composables/useSimverse";
import { type RenderFps, type RenderQuality, useWorldRenderSettings } from "@/composables/useWorldRenderSettings";

const { t } = useI18n();
const { fps, quality, FPS_OPTIONS, QUALITY_OPTIONS, QUALITY_RESOLUTION } = useWorldRenderSettings();
const { currentTier, setPerformanceTier, loadWorldConfig } = useSimverse();

const categories = [
  { key: "graphics", label: t("simverse.graphics"), icon: brushOutline },
  { key: "performance", label: t("simverse.perfTier"), icon: speedometerOutline },
  { key: "about", label: t("simverse.about"), icon: informationCircleOutline },
];
const activeCategory = ref("graphics");

function onFpsChange(ev: any) {
  fps.value = Number(ev.detail.value) as RenderFps;
}
function onQualityChange(ev: any) {
  quality.value = ev.detail.value as RenderQuality;
}
async function onTierChange(ev: any) {
  try {
    await setPerformanceTier(ev.detail.value as PerfTier);
  } catch (e) {
    console.warn("[WorldSettings] set tier failed:", e);
  }
}

onMounted(() => {
  loadWorldConfig().catch(() => {});
});
</script>

<style scoped lang="scss">
.settings-layout {
  display: flex;
  height: 100%;
  min-height: 100%;
}

.settings-nav {
  width: 132px;
  flex-shrink: 0;
  border-right: 1px solid var(--color-base-300);
  padding: 12px 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--color-base-200);
}

.nav-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 12px 4px;
  border: none;
  border-radius: 12px;
  background: transparent;
  color: var(--color-base-content);
  opacity: 0.7;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;

  ion-icon {
    font-size: 22px;
  }

  &.active {
    background: color-mix(in srgb, var(--color-primary) 18%, transparent);
    color: var(--color-primary);
    opacity: 1;
    font-weight: 600;
  }
}

.settings-panel {
  flex: 1;
  padding: 16px;
  overflow-y: auto;
}

.section-title {
  font-size: 16px;
  font-weight: 700;
  margin: 0 0 16px;
}

.option-block {
  margin-bottom: 24px;
}

.option-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-base-content);
  margin-bottom: 8px;
}

.option-hint {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
  margin: 8px 0 0;
  font-family: monospace;
}

.about-desc {
  color: var(--color-base-content);
  opacity: 0.7;
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 12px;
}
</style>
