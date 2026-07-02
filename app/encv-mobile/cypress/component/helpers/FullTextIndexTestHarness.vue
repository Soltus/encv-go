<template>
  <div data-testid="fulltext-harness">
    <ion-app>
      <ion-router-outlet>
        <full-text-index-detail />
      </ion-router-outlet>
    </ion-app>
    <div data-testid="error-log" class="error-log">
      <div v-for="(e, i) in errors" :key="i" class="error-item">
        {{ e.message }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, h } from 'vue'
import { IonApp, IonRouterOutlet } from '@ionic/vue'
import FullTextIndexDetail from '@/views/FullTextIndexDetail.vue'

const errors = ref<string[]>([])

// 捕获全局错误
function handleError(e: ErrorEvent) {
  errors.value.push(e.message)
}
function handleRejection(e: PromiseRejectionEvent) {
  errors.value.push(String(e.reason))
}

onMounted(() => {
  window.addEventListener('error', handleError)
  window.addEventListener('unhandledrejection', handleRejection)
})
onUnmounted(() => {
  window.removeEventListener('error', handleError)
  window.removeEventListener('unhandledrejection', handleRejection)
})
</script>

<style scoped>
.error-log {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  background: rgba(255, 0, 0, 0.1);
  padding: 8px;
  font-size: 12px;
  max-height: 200px;
  overflow-y: auto;
  z-index: 9999;
}
.error-item {
  color: red;
  margin: 2px 0;
}
</style>
