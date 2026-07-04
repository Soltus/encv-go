<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.appearance') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 背景色（驱动暗黑/亮色模式） -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.bgColor') }}</ion-label>
          <ion-badge slot="end" :color="isDark ? 'dark' : 'light'" class="scope-badge">
            {{ isDark ? 'Dark' : 'Light' }}
          </ion-badge>
        </ion-list-header>
        <ion-item lines="full">
          <ion-icon :icon="layersOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.bgColor') }}</h3>
            <p>{{ t('settings.bgColorHelp') }}</p>
          </ion-label>
        </ion-item>

        <div class="bg-section">
          <div class="bg-category" v-for="cat in bgCategories" :key="cat.key">
            <div class="bg-category-title">{{ t(cat.label) }}</div>
            <div class="bg-preset-grid">
              <button
                v-for="(preset, idx) in cat.presets"
                :key="`${cat.key}-${idx}`"
                class="bg-preset-card"
                :class="{
                  'bg-preset-active': cat.key === 'gradient' ? currentGradient === preset.name : currentBgColor === preset.value,
                  'bg-gradient-card': preset.category === 'gradient',
                }"
                :style="getPresetStyle(preset)"
                :title="t(preset.name)"
                @click="cat.key === 'gradient' ? handleGradientSelect(preset) : handleBgColorChange(preset.value!)"
              >
                <span class="bg-preset-name">{{ t(preset.name) }}</span>
              </button>
            </div>
          </div>

          <div class="custom-bg-row">
            <label class="custom-color-label">{{ t('settings.customColor') }}</label>
            <input
              type="color"
              class="color-input"
              :value="currentBgColor || '#ffffff'"
              @input="setBgColor(($event.target as HTMLInputElement).value)"
            />
            <span class="color-hex">{{ (currentBgColor || '--').toUpperCase() }}</span>
            <ion-button v-if="currentBgColor" fill="clear" size="small" @click="setBgColor(null)">
              <ion-icon :icon="closeCircleOutline" slot="icon-only"></ion-icon>
            </ion-button>
          </div>
        </div>
      </ion-list>

      <!-- 主题色 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.themeColor') }}</ion-label>
        </ion-list-header>
        <ion-item lines="full">
          <ion-icon :icon="colorPaletteOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.themeColor') }}</h3>
            <p>{{ t('settings.themeColorHelp') }}</p>
          </ion-label>
        </ion-item>
        <div class="theme-color-picker">
          <div class="preset-colors">
            <button
              v-for="preset in THEME_PRESETS"
              :key="preset.value"
              class="color-dot"
              :class="{ active: currentColor === preset.value }"
              :style="{ backgroundColor: preset.value }"
              :title="preset.name"
              @click="setThemeColor(preset.value)"
            ></button>
          </div>
          <div class="custom-color-row">
            <label class="custom-color-label">{{ t('settings.customColor') }}</label>
            <input
              type="color"
              class="color-input"
              :value="currentColor"
              @input="setThemeColor(($event.target as HTMLInputElement).value)"
            />
            <span class="color-hex">{{ currentColor.toUpperCase() }}</span>
          </div>
        </div>
      </ion-list>

      <!-- 背景模糊 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.bgBlur') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="eyeOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.bgBlur') }}</h3>
            <p>{{ t('settings.bgBlurHelp') }}</p>
          </ion-label>
          <ion-badge slot="end" color="primary" class="blur-value-badge">{{ bgBlur }}px</ion-badge>
        </ion-item>
        <div class="blur-slider-row">
          <span class="blur-label-off">0</span>
          <input
            type="range"
            class="blur-slider"
            min="0"
            max="40"
            step="1"
            :value="bgBlur"
            @input="handleBgBlurChange($event)"
          />
          <span class="blur-label-max">40</span>
        </div>
      </ion-list>

      <!-- 瑰彩显示（CSS 滤镜 + P3 色域） -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.p3Mode') }}</ion-label>
          <ion-badge v-if="isP3Supported" slot="end" color="success" class="scope-badge">
            {{ t('settings.p3Supported') }}
          </ion-badge>
          <ion-badge v-else slot="end" color="medium" class="scope-badge">
            {{ t('settings.p3Unsupported') }}
          </ion-badge>
        </ion-list-header>

        <!-- 瑰彩开关 -->
        <ion-item lines="none">
          <ion-icon :icon="sparklesOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.vividMode') }}</h3>
            <p>{{ t('settings.vividModeHelp') }}</p>
          </ion-label>
          <ion-toggle slot="end" :checked="vividMode === 'on'" @ionChange="handleVividToggle"></ion-toggle>
        </ion-item>

        <!-- 滤镜强度 -->
        <div v-if="vividMode === 'on'" class="vivid-controls">
          <ion-item>
            <ion-icon :icon="trendingUpOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('settings.vividIntensity') }}</h3>
              <p>{{ t('settings.vividIntensityHelp') }}</p>
            </ion-label>
            <ion-badge slot="end" color="primary" class="blur-value-badge">{{ vividIntensity }}%</ion-badge>
          </ion-item>
          <div class="blur-slider-row">
            <span class="blur-label-off">50</span>
            <input
              type="range"
              class="blur-slider"
              min="50"
              max="200"
              step="5"
              :value="vividIntensity"
              @input="handleVividIntensityChange($event)"
            />
            <span class="blur-label-max">200</span>
          </div>
        </div>

        <!-- P3 色域 -->
        <div class="p3-cards">
          <div
            v-for="mode in p3Modes"
            :key="mode.value"
            class="p3-card"
            :class="{ 'p3-card-active': p3Mode === mode.value }"
            @click="handleP3ModeChange(mode.value)"
          >
            <div class="p3-card-title">{{ t(mode.label) }}</div>
            <div v-if="mode.description" class="p3-card-desc">{{ t(mode.description) }}</div>
          </div>
        </div>
      </ion-list>

      <!-- 语言 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.language') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="globeOutline" slot="start"></ion-icon>
          <ion-select :value="locale" @ionChange="handleLocaleChange" interface="action-sheet" mode="ios">
            <ion-select-option value="zh-CN">简体中文</ion-select-option>
            <ion-select-option value="en">English</ion-select-option>
          </ion-select>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
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
  IonPage,
  IonSelect,
  IonSelectOption,
  IonTitle,
  IonToggle,
  IonToolbar,
} from "@ionic/vue";
import {
  closeCircleOutline,
  colorPaletteOutline,
  eyeOutline,
  globeOutline,
  layersOutline,
  sparklesOutline,
  trendingUpOutline,
} from "ionicons/icons";
import { computed, ref } from "vue";
import type { Locale } from "@encv/shared-components/composables/useI18n";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useTheme } from "@encv/shared-components/composables/useTheme";

const {
  isDark,
  currentColor,
  currentBgColor,
  bgBlur,
  p3Mode,
  vividMode,
  vividIntensity,
  isP3Supported,
  THEME_PRESETS,
  BG_PRESETS,
  setThemeColor,
  setBgColor,
  setBgBlur,
  setP3Mode,
  setVividMode,
  setVividIntensity,
  setBgGradient,
} = useTheme();
const { t, locale, setLocale } = useI18n();

const currentGradient = ref<string | null>(null);

const bgCategories = computed(() => [
  {
    key: "light",
    label: "settings.bgLight",
    presets: BG_PRESETS.filter(p => p.category === "light"),
  },
  {
    key: "eye",
    label: "settings.bgEyeCare",
    presets: BG_PRESETS.filter(p => p.category === "eye"),
  },
  {
    key: "dark",
    label: "settings.bgDark",
    presets: BG_PRESETS.filter(p => p.category === "dark"),
  },
  {
    key: "gradient",
    label: "settings.bgGradient",
    presets: BG_PRESETS.filter(p => p.category === "gradient"),
  },
]);

const p3Modes = [
  { value: "auto", label: "settings.p3Auto", description: "" },
  { value: "on", label: "settings.p3On", description: "" },
  { value: "off", label: "settings.p3Off", description: "" },
];

function handleLocaleChange(event: CustomEvent) {
  setLocale(event.detail.value as Locale);
}

function handleBgColorChange(value: string) {
  setBgColor(value);
  currentGradient.value = null;
}

function handleGradientSelect(preset: (typeof BG_PRESETS)[number]) {
  if (preset.gradientColors) {
    setBgGradient(preset.gradientColors);
    currentGradient.value = preset.name;
  }
}

function getPresetStyle(preset: (typeof BG_PRESETS)[number]) {
  if (preset.gradientColors) {
    return {
      background: `linear-gradient(135deg, ${preset.gradientColors.join(", ")})`,
      color: preset.textColor,
      "--gradient-colors": preset.gradientColors.join(", "),
    } as Record<string, string>;
  }
  return {
    backgroundColor: preset.value ?? "#ffffff",
    color: preset.textColor,
  };
}

function handleBgBlurChange(event: Event) {
  const target = event.target as HTMLInputElement;
  setBgBlur(parseInt(target.value, 10));
}

function handleP3ModeChange(value: string) {
  setP3Mode(value as "off" | "on" | "auto");
}

function handleVividToggle(event: CustomEvent) {
  setVividMode(event.detail.checked ? "on" : "off");
}

function handleVividIntensityChange(event: Event) {
  const target = event.target as HTMLInputElement;
  setVividIntensity(parseInt(target.value, 10));
}
</script>

<style scoped>
.theme-color-picker {
  padding: 8px 16px 16px;
}
.preset-colors {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.color-dot {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 2.5px solid transparent;
  cursor: pointer;
  transition: transform 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease;
  outline: none;
  -webkit-tap-highlight-color: transparent;
}
.color-dot.active {
  border-color: var(--ion-text-color, #333);
  transform: scale(1.15);
  box-shadow: 0 0 0 3px rgba(0, 0, 0, 0.08);
}
.custom-color-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.custom-color-label {
  font-size: 13px;
  color: var(--ion-text-secondary);
  white-space: nowrap;
}
.color-input {
  width: 40px;
  height: 32px;
  padding: 0;
  border: 1px solid var(--ion-color-medium, #ccc);
  border-radius: 6px;
  cursor: pointer;
  background: none;
  outline: none;
  appearance: none;
  -webkit-appearance: none;
}
.color-input::-webkit-color-swatch-wrapper {
  padding: 2px;
}
.color-input::-webkit-color-swatch {
  border: none;
  border-radius: 4px;
}
.color-hex {
  font-size: 13px;
  font-family: monospace;
  color: var(--ion-text-secondary);
}

.bg-section {
  padding: 8px 16px 16px;
}
.bg-category {
  margin-bottom: 12px;
}
.bg-category:last-of-type {
  margin-bottom: 0;
}
.bg-category-title {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--ion-color-medium);
  margin-bottom: 8px;
}
.bg-preset-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
.bg-preset-card {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 14px 8px;
  border: 2px solid transparent;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  background-color: #f0f2f5;
  min-height: 52px;
  outline: none;
  -webkit-tap-highlight-color: transparent;
}
.bg-preset-active {
  border-color: var(--ion-color-primary);
  box-shadow: 0 0 0 2px rgba(var(--ion-color-primary-rgb), 0.2);
}
.bg-preset-name {
  font-size: 11px;
  font-weight: 600;
  text-align: center;
}

.bg-gradient-card {
  position: relative;
  overflow: hidden;
  background-size: 200% 200% !important;
  animation: bgGradientShift 6s ease infinite;
}
.bg-gradient-card::before {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: inherit;
  background: linear-gradient(135deg, var(--gradient-colors));
  filter: blur(6px);
  opacity: 0.35;
  z-index: 0;
}
.bg-gradient-card .bg-preset-name {
  position: relative;
  z-index: 1;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
}
.bg-gradient-card.bg-preset-active::after {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: 14px;
  padding: 2px;
  background: linear-gradient(135deg, var(--gradient-colors));
  -webkit-mask:
    linear-gradient(#fff 0 0) content-box,
    linear-gradient(#fff 0 0);
  mask-composite: exclude;
  pointer-events: none;
}

@keyframes bgGradientShift {
  0%, 100% { background-position: 0% 50%; }
  25% { background-position: 100% 50%; }
  50% { background-position: 100% 100%; }
  75% { background-position: 0% 100%; }
}
.custom-bg-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--ion-color-light-shade, #e0e0e0);
}
body.dark .custom-bg-row {
  border-top-color: #2a2a2c;
}

.blur-slider-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 16px 16px;
}
.blur-slider {
  flex: 1;
  --thumb-size: 20px;
  accent-color: var(--ion-color-primary);
}
.blur-label-off, .blur-label-max {
  font-size: 11px;
  color: var(--ion-color-medium);
  width: 20px;
  text-align: center;
  flex-shrink: 0;
}
.blur-value-badge {
  font-size: 11px;
  --padding-start: 6px;
  --padding-end: 6px;
}

.vivid-controls {
  margin-top: 4px;
}

.p3-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  padding: 8px 16px 16px;
}
.p3-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 14px 8px;
  border: 2px solid var(--ion-color-light-shade, #e0e0e0);
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
  background: var(--ion-background-color, transparent);
  min-height: 56px;
}
body.dark .p3-card {
  border-color: #3a3a3c;
}
.p3-card-active {
  border-color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}
.p3-card-title {
  font-weight: 600;
  font-size: 13px;
}
.p3-card-desc {
  font-size: 10px;
  color: var(--ion-color-medium);
  margin-top: 3px;
  line-height: 1.2;
}

.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}
.scope-badge-icon {
  font-size: 12px;
}
@media (max-width: 599px) {
  .scope-badge {
    --padding-start: 5px;
    --padding-end: 5px;
    --padding-top: 2px;
    --padding-bottom: 2px;
  }
  .scope-badge .scope-text {
    display: none;
  }
  .bg-preset-grid {
    grid-template-columns: repeat(3, 1fr);
    gap: 6px;
  }
  .bg-preset-card {
    padding: 10px 4px;
    min-height: 44px;
  }
  .bg-preset-name {
    font-size: 10px;
  }
  .p3-cards {
    grid-template-columns: repeat(3, 1fr);
    gap: 6px;
  }
  .p3-card {
    padding: 10px 4px;
    min-height: 48px;
  }
  .p3-card-title {
    font-size: 12px;
  }
}
</style>
