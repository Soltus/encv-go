<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.automationTests') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <p class="section-hint">{{ t('devtools.automationTestsHint') }}</p>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.automationHub.testEntries') }}</ion-label>
        </ion-list-header>

        <ion-item button detail @click="goPluginTests">
          <ion-icon :icon="flaskOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.automationHub.pluginTests') }}</h3>
            <p>{{ t('devtools.automationHub.pluginTestsDesc') }}</p>
          </ion-label>
        </ion-item>

        <ion-item button detail @click="goWebDavTests">
          <ion-icon :icon="cloudUploadOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.webdavTests') }}</h3>
            <p>{{ t('devtools.webdavTestsHint') }}</p>
          </ion-label>
        </ion-item>

        <ion-item button detail @click="goSparseContainerTest">
          <ion-icon :icon="serverOutline" slot="start" color="warning"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.sparseContainer.title') }}</h3>
            <p>{{ t('devtools.sparseContainer.entryHint') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonListHeader, IonItem, IonIcon, IonLabel,
} from '@ionic/vue'
import { flaskOutline, cloudUploadOutline, serverOutline } from 'ionicons/icons'
import { useRouter } from 'vue-router'
import { useI18n } from '@/composables/useI18n'

const { t } = useI18n()
const router = useRouter()

// 2026-06-17：插件加密/解密自动化测试（重命名自 AutomationTestsDetail）
function goPluginTests() {
  router.push('/tabs/settings/devtools/plugin-tests')
}

function goWebDavTests() {
  router.push('/tabs/settings/devtools/webdav-tests')
}

function goSparseContainerTest() {
  router.push('/tabs/settings/devtools/sparse-container-test')
}
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 12px 16px 8px;
  line-height: 1.5;
}
</style>
