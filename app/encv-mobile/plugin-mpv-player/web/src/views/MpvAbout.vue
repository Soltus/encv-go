<template>
  <AboutPage
    app-name="MPV Player"
    :app-version="`v${pluginVersion}`"
    engine-name="MPV Media Player"
    license="GNU General Public License v2.0"
    :license-url="licenseUrl"
    back-href="/settings"
  >
    <template #extra>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('mpv.settings.version') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="extensionPuzzleOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('mpv.about.mpvVersion') }}</h3>
            <p>{{ mpvVersion }}</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </template>
  </AboutPage>
</template>

<script setup lang="ts">
import AboutPage from "@encv/shared-components/components/about/AboutPage.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { extensionPuzzleOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { logBuffer, MpvNative } from "@/plugins/mpv-native";

const { t } = useI18n();

const mpvVersion = ref("unknown");
const pluginVersion = ref("0.1.0");
const licenseUrl = "https://www.gnu.org/licenses/old-licenses/gpl-2.0.html";

onMounted(() => {
  mpvVersion.value = MpvNative.getVersion();
  pluginVersion.value = "0.1.0";

  logBuffer.info("MPV 关于页已加载");
});
</script>

<style scoped>
</style>
