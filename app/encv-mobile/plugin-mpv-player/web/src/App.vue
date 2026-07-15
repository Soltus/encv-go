<template>
  <ion-app>
    <div v-if="rootError" class="root-error-fallback">
      <div class="error-content">
        <ion-icon :icon="bugOutline" class="error-icon" color="danger" />
        <h2>组件渲染错误</h2>
        <p class="error-message">某个 plugin 视图崩溃了。请截图上报给开发者。</p>
        <code class="error-detail">{{ rootErrorMessage }}</code>
        <pre v-if="rootErrorStack" class="error-stack">{{ rootErrorStack }}</pre>
        <ion-button @click="reloadPage" class="error-reload-btn" color="primary">
          <ion-icon :icon="refreshOutline" slot="start" />
          重新加载
        </ion-button>
      </div>
    </div>
    <ion-router-outlet v-else />
  </ion-app>
</template>

<script setup lang="ts">
import { bugOutline, refreshOutline } from "ionicons/icons";
import { onErrorCaptured, ref } from "vue";

const rootError = ref(false);
const rootErrorMessage = ref("");
const rootErrorStack = ref("");

onErrorCaptured((err: any, _instance, info) => {
  if (rootError.value) return false;
  console.error("[plugin-mpv/web] Vue error captured:", err, "| info:", info);
  rootError.value = true;
  rootErrorMessage.value = err?.message || String(err) || "Unknown render error";
  rootErrorStack.value = err?.stack || "";
  return false;
});

function reloadPage() {
  if (typeof window !== "undefined") {
    window.location.reload();
  }
}
</script>

<style scoped>
ion-app {
}

.root-error-fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
  width: 100vw;
  background: var(--ion-background-color, var(--color-white));
  padding: 24px;
}

.error-content {
  text-align: center;
  max-width: 480px;
}

.error-icon {
  font-size: 64px;
  margin-bottom: 16px;
}

.error-content h2 {
  font-size: 20px;
  font-weight: 700;
  color: var(--ion-text-color, var(--color-black));
  margin: 0 0 8px;
}

.error-message {
  font-size: 14px;
  color: var(--ion-color-medium);
  margin: 0 0 16px;
  line-height: 1.5;
}

.error-detail {
  display: block;
  font-size: 12px;
  color: var(--ion-color-danger);
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-radius: 8px;
  padding: 10px 14px;
  margin: 0 0 12px;
  text-align: left;
  word-break: break-all;
  white-space: pre-wrap;
  font-family: ui-monospace, Menlo, monospace;
}

.error-stack {
  display: block;
  font-size: 10.5px;
  color: var(--ion-color-medium);
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 6px;
  padding: 8px 12px;
  margin: 0 0 20px;
  text-align: left;
  white-space: pre-wrap;
  max-height: 30vh;
  overflow-y: auto;
  font-family: ui-monospace, Menlo, monospace;
}

.error-reload-btn {
  --border-radius: 8px;
}
</style>
