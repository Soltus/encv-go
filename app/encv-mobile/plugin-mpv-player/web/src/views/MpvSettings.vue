<template>
  <SettingsPage :title="t('mpv.settings.title')" :show-back-button="true" @save="onSave" @reset="onReset">
    <SettingsGroup :title="t('mpv.settings.player')">
      <SettingsItem :icon="playCircleOutline" :title="t('mpv.settings.autoPlay')" :description="autoPlay ? '开启' : '关闭'">
        <template #default>
          <ion-toggle v-model="autoPlay" @ionChange="onAutoPlayChange" />
        </template>
      </SettingsItem>
      <SettingsItem :icon="hardwareChipOutline" :title="t('mpv.settings.hardwareDecoding')" :description="hardwareDecoding ? '开启' : '关闭'">
        <template #default>
          <ion-toggle v-model="hardwareDecoding" @ionChange="onHardwareDecodingChange" />
        </template>
      </SettingsItem>
      <SettingsItem :icon="volumeHighOutline" :title="t('mpv.settings.defaultVolume')">
        <template #default>
          <div class="volume-control">
            <span class="volume-value">{{ defaultVolume }}%</span>
            <ion-range
              :value="defaultVolume"
              min="0"
              max="100"
              step="1"
              @ionChange="onVolumeChange"
              class="volume-slider"
            />
          </div>
        </template>
      </SettingsItem>
    </SettingsGroup>

    <SettingsGroup :title="t('mpv.settings.video')">
      <SettingsItem :icon="videocamOutline" :title="t('mpv.settings.video')" button @click="goToVideoSettings">
        <template #default>
          <ion-icon :icon="chevronForwardOutline" />
        </template>
      </SettingsItem>
    </SettingsGroup>

    <SettingsGroup :title="t('mpv.settings.audio')">
      <SettingsItem :icon="musicalNotesOutline" :title="t('mpv.settings.audio')" button @click="goToAudioSettings">
        <template #default>
          <ion-icon :icon="chevronForwardOutline" />
        </template>
      </SettingsItem>
    </SettingsGroup>

    <SettingsGroup :title="t('mpv.settings.subtitle')">
      <SettingsItem :icon="documentTextOutline" :title="t('mpv.settings.subtitle')" button @click="goToSubtitleSettings">
        <template #default>
          <ion-icon :icon="chevronForwardOutline" />
        </template>
      </SettingsItem>
    </SettingsGroup>

    <SettingsGroup :title="t('mpv.settings.about')">
      <SettingsItem :icon="informationCircleOutline" :title="t('mpv.about.title')" button @click="goToAbout">
        <template #default>
          <span class="version-text">v{{ pluginVersion }}</span>
          <ion-icon :icon="chevronForwardOutline" />
        </template>
      </SettingsItem>
    </SettingsGroup>
  </SettingsPage>
</template>

<script setup lang="ts">
import SettingsGroup from "@encv/shared-components/components/settings/SettingsGroup.vue";
import SettingsItem from "@encv/shared-components/components/settings/SettingsItem.vue";
import SettingsPage from "@encv/shared-components/components/settings/SettingsPage.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  chevronForwardOutline,
  documentTextOutline,
  hardwareChipOutline,
  informationCircleOutline,
  musicalNotesOutline,
  playCircleOutline,
  videocamOutline,
  volumeHighOutline,
} from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { logBuffer, MpvNative } from "@/plugins/mpv-native";

const { t } = useI18n();
const router = useRouter();

const autoPlay = ref(false);
const hardwareDecoding = ref(true);
const defaultVolume = ref(100);
const pluginVersion = ref("0.1.0");

onMounted(() => {
  pluginVersion.value = MpvNative.getVersion();
  defaultVolume.value = MpvNative.getVolume();

  logBuffer.info("MPV 设置页已加载");
});

function onSave() {
  logBuffer.info("保存 MPV 设置");
}

function onReset() {
  logBuffer.info("重置 MPV 设置");
  autoPlay.value = false;
  hardwareDecoding.value = true;
  defaultVolume.value = 100;
}

function onAutoPlayChange() {
  logBuffer.info(`自动播放: ${autoPlay.value ? "开启" : "关闭"}`);
}

function onHardwareDecodingChange() {
  logBuffer.info(`硬件解码: ${hardwareDecoding.value ? "开启" : "关闭"}`);
}

function onVolumeChange(e: any) {
  const value = e.detail.value;
  defaultVolume.value = value;
  MpvNative.setVolume(value);
}

function goToVideoSettings() {
  logBuffer.info("打开视频设置");
}

function goToAudioSettings() {
  logBuffer.info("打开音频设置");
}

function goToSubtitleSettings() {
  logBuffer.info("打开字幕设置");
}

function goToAbout() {
  router.push("/settings/about");
}
</script>

<style scoped>
.volume-control {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.volume-value {
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-color-primary);
  min-width: 48px;
  text-align: right;
}

.volume-slider {
  flex: 1;
}

.version-text {
  color: var(--ion-color-medium);
  font-size: 14px;
  margin-right: 8px;
}
</style>
