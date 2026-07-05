<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools/automation-hub"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.fsTests.title') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <p class="section-hint">{{ t('devtools.fsTests.hint') }}</p>

      <!-- 操作区 -->
      <ion-list>
        <ion-item button :disabled="isRunning" @click="handleRunAll">
          <ion-icon :icon="playCircleOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.fsTests.runAll') }}</h3>
            <p>{{ t('devtools.fsTests.runAllDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="isRunning" slot="end" name="dots"></ion-spinner>
        </ion-item>
      </ion-list>

      <!-- 汇总 -->
      <ion-list v-if="results.length > 0">
        <ion-list-header>
          <ion-label>{{ t('devtools.fsTests.results') }}</ion-label>
          <ion-badge slot="end" :color="failedCount === 0 ? 'success' : 'danger'">
            {{ t('devtools.fsTests.passed') }}: {{ passedCount }} / {{ results.length }}
          </ion-badge>
        </ion-list-header>

        <ion-item v-for="r in results" :key="r.name">
          <ion-icon
            :icon="r.passed ? checkmarkCircle : closeCircle"
            slot="start"
            :color="r.passed ? 'success' : 'danger'"
          ></ion-icon>
          <ion-label>
            <h3>{{ r.name }}</h3>
            <p v-if="!r.passed && r.error" class="error-text">{{ r.error }}</p>
            <p v-if="r.duration != null" class="duration-text">
              {{ t('devtools.fsTests.duration') }}: {{ r.duration }}ms
            </p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 空状态 -->
      <ion-list v-else>
        <ion-item lines="none">
          <ion-label class="ion-text-center">
            <p class="empty-text">{{ t('devtools.fsTests.noResults') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useFileSystemTests } from "@encv/shared-components/composables/useFileSystemTests";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { computed } from "vue";

const { t } = useI18n();
const { results, isRunning, runAllTests } = useFileSystemTests();

const _passedCount = computed(() => results.value.filter(r => r.passed).length);
const _failedCount = computed(() => results.value.filter(r => !r.passed).length);

async function _handleRunAll() {
  await runAllTests();
}
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 12px 16px 8px;
  line-height: 1.5;
}
.error-text {
  color: var(--ion-color-danger);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.duration-text {
  color: var(--encv-text-secondary, #999);
  font-size: 11px;
}
.empty-text {
  color: var(--encv-text-secondary, #999);
  font-size: 13px;
  padding: 24px 0;
}
</style>
