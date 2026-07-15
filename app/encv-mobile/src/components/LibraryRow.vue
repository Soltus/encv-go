<template>
  <div class="lib-row">
    <div class="lib-title-row">
      <div class="lib-name-version">
        <ion-icon
          v-if="iconName"
          :icon="resolvedIcon"
          class="lib-icon"
          :title="t('about.libsIcon') + ': ' + iconName"
        ></ion-icon>
        <span class="lib-name">{{ item.name }}</span>
        <span class="lib-version">{{ item.version }}</span>
      </div>
      <div class="lib-badges">
        <span class="lib-source-badge" :title="t('about.libsSource') + ': ' + sourceLabel">
          {{ sourceLabel }}
        </span>
        <span
          class="lib-status-badge"
          :class="['lib-status-' + item.status]"
          :title="statusLabel"
        >
          {{ statusLabel }}
        </span>
        <span
          class="lib-importance-badge"
          :class="['lib-importance-' + item.importance]"
          :title="importanceLabel"
        >
          {{ importanceLabel }}
        </span>
        <span
          v-if="item.license && item.license !== 'unknown'"
          class="lib-license-badge"
          :title="t('about.libsLicense') + ': ' + item.license"
        >
          {{ item.license }}
        </span>
        <span
          v-else
          class="lib-license-badge lib-license-unknown"
          :title="t('about.libsLicenseUnknown')"
        >
          {{ t('about.libsLicenseUnknown') }}
        </span>
      </div>
    </div>
    <div class="lib-description-row">
      <span v-if="item.description" class="lib-description lib-description-explicit">
        {{ item.description }}
      </span>
      <span v-else-if="item.descriptionStatus === 'fetching'" class="lib-description lib-description-fetching">
        <span class="lib-description-spinner"></span>
        {{ t('about.libFetchingDescription') }}
      </span>
      <span v-else-if="resolvedDescription" class="lib-description lib-description-fetched">
        {{ resolvedDescription }}
      </span>
      <span v-else class="lib-description lib-description-placeholder">
        {{ t('about.libDescriptionPlaceholder') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { addIcons } from "ionicons";
import {
  analytics,
  apps,
  archive,
  arrowForward,
  bug,
  cafe,
  chatbubbles,
  checkmarkCircle,
  code,
  codeSlash,
  cog,
  colorPalette,
  construct,
  cube,
  documentText,
  eye,
  film,
  flash,
  flask,
  gitNetwork,
  globe,
  grid,
  helpCircle,
  image,
  key,
  layers,
  list,
  lockClosed,
  logoAndroid,
  logoGoogle,
  logoIonic,
  logoVue,
  phonePortrait,
  phonePortraitOutline,
  playCircle,
  server,
  shareSocial,
  speedometer,
  sync,
  terminal,
  text,
} from "ionicons/icons";
import { computed, ref, watch } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import type { LibraryItem } from "@encv/shared-components/composables/useLibraries";

const props = defineProps<{ item: LibraryItem }>();
const { t } = useI18n();

// 注册所有可能用到的 ionicon（避免按需引用导致 svg 缺失）
addIcons({
  "help-circle": helpCircle,
  "logo-android": logoAndroid,
  "logo-google": logoGoogle,
  "logo-ionic": logoIonic,
  "logo-vue": logoVue,
  cube: cube,
  apps: apps,
  grid: grid,
  image: image,
  construct: construct,
  eye: eye,
  layers: layers,
  "color-palette": colorPalette,
  sync: sync,
  "git-network": gitNetwork,
  bug: bug,
  flask: flask,
  cafe: cafe,
  speedometer: speedometer,
  "phone-portrait": phonePortrait,
  "phone-portrait-outline": phonePortraitOutline,
  "share-social": shareSocial,
  chatbubbles: chatbubbles,
  "play-circle": playCircle,
  "document-text": documentText,
  terminal: terminal,
  list: list,
  analytics: analytics,
  globe: globe,
  server: server,
  "code-slash": codeSlash,
  "checkmark-circle": checkmarkCircle,
  flash: flash,
  film: film,
  text: text,
  code: code,
  key: key,
  archive: archive,
  "lock-closed": lockClosed,
  cog: cog,
  "arrow-forward": arrowForward,
});

const resolvedDescription = ref<string>(props.item.descriptionFallback || "");

const iconName = computed<string>(() => props.item.icon || "cube");

// ion-icon 通过 :icon prop 接受 string（自动查 addIcons 注册的 svg 路径）
const resolvedIcon = computed<string>(() => iconName.value);

const sourceLabel = computed(() => {
  const s = props.item.source;
  if (s === "package.json") return t("about.libSource.packageJson");
  if (s === "libs.versions.toml") return t("about.libSource.libsVersionsToml");
  if (s === "build.gradle.kts") return t("about.libSource.buildGradleKts");
  if (s === "go.mod") return t("about.libSource.goMod");
  if (s === "runtime.Version()") return t("about.libSource.runtimeVersion");
  return t("about.libSource.unknown");
});

const statusLabel = computed(() => {
  const s = props.item.status;
  if (s === "active") return t("about.libStatus.active");
  if (s === "broken") return t("about.libStatus.broken");
  return t("about.libStatus.historical");
});

const importanceLabel = computed(() => {
  const i = props.item.importance;
  if (i === "core") return t("about.libImportance.core");
  if (i === "light") return t("about.libImportance.light");
  return t("about.libImportance.transitive");
});

watch(
  () => props.item.descriptionFallback,
  v => {
    if (v) resolvedDescription.value = v;
  }
);
</script>

<style scoped>
.lib-row {
  padding: 10px 16px;
  border-bottom: 1px solid color-mix(in srgb, var(--color-base-200) 85%, var(--color-black));
  display: flex;
  flex-direction: column;
  gap: 4px;
}

body.dark .lib-row {
  border-bottom-color: #2a2a2c;
}

.lib-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 6px;
}

.lib-name-version {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 auto;
  min-width: 0;
}

.lib-icon {
  font-size: 18px;
  color: var(--color-primary);
  flex-shrink: 0;
}

body.dark .lib-icon {
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-white));
}

.lib-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--ion-text-color);
  word-break: break-all;
}

.lib-version {
  font-size: 12px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  font-family: monospace;
  flex-shrink: 0;
}

.lib-badges {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  flex-shrink: 0;
}

.lib-source-badge,
.lib-status-badge,
.lib-importance-badge,
.lib-license-badge {
  font-size: 10px;
  padding: 2px 8px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  white-space: nowrap;
  line-height: 1.4;
}

.lib-source-badge {
  background: color-mix(in srgb, var(--color-base-200) 85%, var(--color-black));
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-black));
}

body.dark .lib-source-badge {
  background: #3a3a3c;
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-white));
}

.lib-status-active {
  background: color-mix(in srgb, var(--color-success) 16%, transparent);
  color: color-mix(in srgb, var(--color-success) 85%, var(--color-black));
}

.lib-status-broken {
  background: color-mix(in srgb, var(--color-error) 16%, transparent);
  color: color-mix(in srgb, var(--color-error) 85%, var(--color-black));
}

.lib-status-historical {
  background: color-mix(in srgb, var(--color-base-content) 16%, var(--color-base-100));
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-black));
}

.lib-importance-core {
  background: color-mix(in srgb, var(--color-primary) 16%, transparent);
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-black));
}

.lib-importance-light {
  background: color-mix(in srgb, var(--color-accent, var(--color-secondary)) 16%, transparent);
  color: color-mix(in srgb, var(--color-accent) 85%, var(--color-black));
}

.lib-importance-transitive {
  background: color-mix(in srgb, var(--color-base-content) 12%, var(--color-base-100));
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-black));
}

.lib-license-badge {
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-black));
  font-family: monospace;
}

body.dark .lib-license-badge {
  background: color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.lib-license-unknown {
  background: color-mix(in srgb, var(--color-base-content) 10%, var(--color-base-100));
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-black));
  font-style: italic;
}

.lib-description-row {
  font-size: 12px;
  line-height: 1.4;
}

.lib-description {
  display: inline;
  word-break: break-word;
}

.lib-description-explicit {
  color: var(--ion-text-color);
}

.lib-description-fetched {
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-black));
  font-style: italic;
}

.lib-description-placeholder {
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  font-style: italic;
  opacity: 0.6;
}

.lib-description-fetching {
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
}

.lib-description-spinner {
  display: inline-block;
  width: 10px;
  height: 10px;
  border: 2px solid color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  border-top-color: transparent;
  border-radius: 50%;
  animation: lib-spin 0.8s linear infinite;
  margin-right: 4px;
  vertical-align: middle;
}

@keyframes lib-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
