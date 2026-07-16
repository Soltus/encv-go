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
    <!-- 主题（运行时资产包：官方 == 第三方，可加载 / 卸载 / 分发） -->
    <ion-list>
      <ion-list-header>
        <ion-label>{{ t('settings.themes') }}</ion-label>
        <ion-badge slot="end" color="primary" class="scope-badge">{{ allThemes.length }}</ion-badge>
      </ion-list-header>
    </ion-list>
    <div class="theme-grid">
      <button
        v-for="theme in allThemes"
        :key="theme.id"
        type="button"
        class="theme-card"
        :class="{ 'theme-card-active': isActive(theme.id) }"
        :data-theme="theme.id"
        @click="applyTheme(theme.id)"
      >
        <div class="theme-preview">
          <!-- 真实语义组件：继承卡片 data-theme，直接呈现该主题的「形状 + 组件覆写」（霓虹发光 / 纸感衬线） -->
          <div class="ui-bubble ui-bubble--user">Aa</div>
          <div class="pv-row">
            <span class="ui-chip">Chip</span>
            <span class="ui-panel pv-panel">Panel</span>
          </div>
          <div class="pv-swatches">
            <span class="sw" :style="{ background: 'var(--color-primary)' }"></span>
            <span class="sw" :style="{ background: 'var(--color-accent)' }"></span>
            <span class="sw" :style="{ background: 'var(--color-secondary)' }"></span>
            <span class="sw" :style="{ background: 'var(--color-base-100)' }"></span>
            <span class="sw" :style="{ background: 'var(--color-base-300)' }"></span>
          </div>
        </div>
        <div class="theme-meta">
          <span class="theme-name">{{ themeLabel(theme) }}</span>
          <ion-icon v-if="isActive(theme.id)" :icon="checkmarkOutline" class="theme-check"></ion-icon>
          <button
            v-if="!isBuiltIn(theme.id)"
            type="button"
            class="theme-uninstall"
            :aria-label="t('settings.uninstall')"
            @click.stop="uninstallTheme(theme.id)"
          >
            <ion-icon :icon="trashOutline" />
          </button>
        </div>
      </button>
    </div>

    <!-- 主题集市（Bazaar）：一键安装到用户空间 -->
    <ion-list>
      <ion-list-header>
        <ion-label>{{ t('settings.bazaar') }}</ion-label>
      </ion-list-header>
      <ion-item v-for="entry in bazaar" :key="entry.id" :detail="false">
        <ion-label>
          <h3>{{ t(entry.nameKey) }}</h3>
          <p v-if="entry.descKey">{{ t(entry.descKey) }}</p>
        </ion-label>
        <ion-button
          v-if="!isInstalled(entry.id)"
          size="small"
          slot="end"
          @click="installFromBazaar(entry)"
        >
          {{ t('settings.install') }}
        </ion-button>
        <ion-badge v-else slot="end" color="success">{{ t('settings.installed') }}</ion-badge>
      </ion-item>
    </ion-list>

    <!-- 从链接安装（可分发：粘贴主题文件夹 / theme.json 地址，读取清单自动发现元信息） -->
    <ion-list>
      <ion-list-header>
        <ion-label>{{ t('settings.installFromUrl') }}</ion-label>
      </ion-list-header>
      <ion-item>
        <ion-input
          :placeholder="t('settings.themeUrlPlaceholder')"
          :value="themeUrl"
          @ionInput="themeUrl = ($event.target as HTMLInputElement).value"
        ></ion-input>
        <ion-button
          slot="end"
          size="small"
          :disabled="!themeUrlValid || themeInstalling"
          @click="installFromUrl"
        >
          {{ themeInstalling ? t('settings.installing') : t('settings.install') }}
        </ion-button>
      </ion-item>
      <ion-item v-if="themeInstallError" lines="none">
        <ion-note color="danger">{{ themeInstallError }}</ion-note>
      </ion-item>
    </ion-list>

    <!-- 主题性能指标（themeLoader 实时） -->
    <ion-list>
      <ion-list-header>
        <ion-label>{{ t('settings.themePerf') }}</ion-label>
      </ion-list-header>
      <ion-item lines="none">
        <ion-label class="theme-perf">
          <span>{{ t('settings.themePerfSwitch') }}：{{ themePerf.lastSwitchMs ?? '—' }} ms</span>
          <span>{{ t('settings.themePerfCache') }}：{{ themePerf.cacheHits }}</span>
          <span>{{ t('settings.themePerfLoaded') }}：{{ themePerf.loaded }}</span>
        </ion-label>
      </ion-item>
    </ion-list>

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

      <!-- 动效 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.motion') }}</ion-label>
        </ion-list-header>
        <ion-item lines="none">
          <ion-icon :icon="flashOffOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.reduceMotion') }}</h3>
            <p>{{ t('settings.reduceMotionHelp') }}</p>
          </ion-label>
          <ion-toggle
            slot="end"
            :checked="isForcedOff"
            @ionChange="handleMotionToggle"
          ></ion-toggle>
        </ion-item>
      </ion-list>

      <!-- Snippets（局部覆盖，热开关，Phase 5） -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.snippets') }}</ion-label>
          <ion-badge slot="end" color="medium" class="scope-badge">{{ snippets.length }}</ion-badge>
        </ion-list-header>
        <ion-item lines="none" v-for="snip in snippets" :key="snip.id">
          <ion-icon :icon="codeOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t(snip.labelKey) }}</h3>
            <p>{{ t(snip.labelKey + 'Help') }}</p>
          </ion-label>
          <ion-toggle
            slot="end"
            :checked="isEnabled(snip.id)"
            @ionChange="handleSnippetToggle(snip.id)"
          ></ion-toggle>
        </ion-item>
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
  checkmarkOutline,
  closeCircleOutline,
  codeOutline,
  colorPaletteOutline,
  eyeOutline,
  flashOffOutline,
  globeOutline,
  layersOutline,
  sparklesOutline,
  trashOutline,
  trendingUpOutline,
} from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import type { Locale } from "@encv/shared-components/composables/useI18n";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useTheme } from "@encv/shared-components/composables/useTheme";
import { useMotionPreference } from "@encv/shared-components/composables/useMotionPreference";
import { useUserThemes, type UserThemeMeta } from "@encv/shared-components/composables/useUserThemes";
import { useSnippets } from "@encv/shared-components/composables/useSnippets";

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
const { isForcedOff, setForcedOff } = useMotionPreference();
const {
  allThemes,
  applyTheme,
  isActive,
  isBuiltIn,
  isInstalled,
  themeLabel,
  installFromBazaar,
  installTheme,
  installThemeFromUrl,
  installThemeFromCssLink,
  uninstallTheme,
  ensureThemeLoaded,
  bazaar,
  themePerf,
} = useUserThemes();
const { snippets, isEnabled, toggle: toggleSnippet } = useSnippets();

// 从链接安装主题（可分发）：
//   - 主题文件夹 / theme.json 地址 → 读取清单自动发现 id/名字/CSS/JS（真正「可分发」路径）
//   - 直连 theme.css 地址 → 回退旧行为（从文件名推导 id，无清单元信息）
const themeUrl = ref("");
const themeInstalling = ref(false);
const themeInstallError = ref("");
const themeUrlValid = computed(() => /^https?:\/\/.+/i.test(themeUrl.value.trim()));
async function installFromUrl() {
  const url = themeUrl.value.trim();
  if (!themeUrlValid.value || themeInstalling.value) return;
  themeInstalling.value = true;
  themeInstallError.value = "";
  try {
    if (/\.css$/i.test(url)) {
      // 裸 .css 直链回退：同样拉取到后端本地同一目录，本地优先。
      await installThemeFromCssLink(url);
    } else {
      await installThemeFromUrl(url);
    }
    themeUrl.value = "";
  } catch (e) {
    themeInstallError.value = e instanceof Error ? e.message : String(e);
  } finally {
    themeInstalling.value = false;
  }
}

// 预热所有主题 CSS，让可视预览在渲染时即带正确样式
onMounted(() => {
  for (const theme of allThemes.value) ensureThemeLoaded(theme.id);
});

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

function handleMotionToggle(event: CustomEvent) {
  setForcedOff(event.detail.checked);
}

function handleSnippetToggle(id: string) {
  toggleSnippet(id);
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
  border: 1px solid color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
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
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
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
  border: 1px solid var(--color-base-300);
  border-radius: 14px;
  cursor: pointer;
  transition: transform 0.28s cubic-bezier(0.34, 1.56, 0.64, 1),
    box-shadow 0.25s ease, border-color 0.2s ease, background-color 0.2s ease;
  background-color: var(--color-base-200);
  min-height: 52px;
  outline: none;
  -webkit-tap-highlight-color: transparent;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04), 0 3px 10px -6px rgba(0, 0, 0, 0.1);
}
.bg-preset-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--color-primary) 40%, var(--color-base-300));
  box-shadow: 0 6px 16px -6px rgba(0, 0, 0, 0.16),
    0 2px 8px -3px color-mix(in srgb, var(--color-primary) 22%, transparent);
}
.bg-preset-active {
  border-color: var(--color-primary);
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--color-primary) 16%, transparent),
    color-mix(in srgb, var(--color-primary) 4%, transparent)
  );
  box-shadow: 0 0 0 1px var(--color-primary),
    0 8px 22px -8px color-mix(in srgb, var(--color-primary) 50%, transparent);
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
    linear-gradient(var(--color-white) 0 0) content-box,
    linear-gradient(var(--color-white) 0 0);
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
  border-top: 1px solid color-mix(in srgb, var(--color-base-200) 85%, var(--color-black));
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
  accent-color: var(--color-primary);
}
.blur-label-off, .blur-label-max {
  font-size: 11px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
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
  border: 1px solid var(--color-base-300);
  border-radius: 14px;
  cursor: pointer;
  transition: transform 0.28s cubic-bezier(0.34, 1.56, 0.64, 1),
    box-shadow 0.25s ease, border-color 0.2s ease;
  text-align: center;
  background: var(--color-base-100, transparent);
  min-height: 56px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04), 0 3px 10px -6px rgba(0, 0, 0, 0.1);
}
.p3-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--color-primary) 40%, var(--color-base-300));
  box-shadow: 0 6px 16px -6px rgba(0, 0, 0, 0.16);
}
body.dark .p3-card {
  border-color: var(--color-base-300);
}
.p3-card-active {
  border-color: var(--color-primary);
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--color-primary) 13%, transparent),
    color-mix(in srgb, var(--color-primary) 3%, transparent)
  );
  box-shadow: 0 0 0 1px var(--color-primary),
    0 8px 22px -8px color-mix(in srgb, var(--color-primary) 50%, transparent);
}
.p3-card-title {
  font-weight: 600;
  font-size: 13px;
}
.p3-card-desc {
  font-size: 10px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
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

/* ── 主题目视实时预览网格（真主题：每个卡片用该主题自身 CSS 渲染，显示设计语言）── */
.theme-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
  padding: 8px 16px 16px;
}
.theme-card {
  display: flex;
  flex-direction: column;
  padding: 0;
  border: 1px solid var(--color-base-300);
  border-radius: 16px;
  background: var(--color-base-100);
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease;
  appearance: none;
  -webkit-tap-highlight-color: transparent;
  font: inherit;
  /* 卡片文字统一用「该主题自身」的令牌，保证在暗/亮卡片上都可读（不继承文档级主题色） */
  color: var(--color-base-content);
  text-align: left;
}
.theme-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--color-primary) 45%, var(--color-base-300));
}
.theme-card-active {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 40%, transparent);
}
.theme-preview {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 96px;
}
/* 预览元素消费卡片作用域内的该主题变量 —— 形状/颜色/组件覆写差异直观可见。
   直接用真实语义组件（.ui-bubble--user / .ui-chip / .ui-panel），不另造样式：
   主题对组件的覆写（如 neon 发光描边、paper 衬线）会原样呈现。 */
.theme-preview .ui-bubble--user {
  align-self: flex-end;
  max-width: 78%;
}
.pv-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.pv-panel {
  font-size: 11px;
  padding: 2px 8px;
}
.pv-swatches {
  display: flex;
  gap: 6px;
  margin-top: auto;
}
.sw {
  width: 22px;
  height: 22px;
  border-radius: 6px;
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.08);
}
.theme-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-top: 1px solid var(--color-base-200);
}
.theme-name {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.theme-check {
  color: var(--color-primary);
  font-size: 18px;
  flex-shrink: 0;
}
.theme-uninstall {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--color-base-content);
  cursor: pointer;
  font-size: 18px;
}
.theme-uninstall:hover {
  background: color-mix(in srgb, var(--color-error) 14%, transparent);
  color: var(--color-error);
}
.theme-perf {
  display: flex !important;
  flex-wrap: wrap;
  gap: 4px 16px;
  font-size: 12px;
}
.theme-perf span {
  color: var(--ion-text-secondary);
}
@media (max-width: 599px) {
  .theme-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
  }
  .theme-preview {
    padding: 12px;
    min-height: 88px;
  }
}
</style>
