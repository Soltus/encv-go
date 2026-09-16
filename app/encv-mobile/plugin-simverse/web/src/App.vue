<template>
  <ion-app>
    <ion-router-outlet />
  </ion-app>
</template>

<script setup lang="ts">
import { alertCircleOutline, bugOutline, codeSlashOutline, copyOutline, refreshOutline } from "ionicons/icons";
import { onErrorCaptured, ref } from "vue";

const rootError = ref(false);
const rootErrorSummary = ref("");
const rootErrorStack = ref("");
const rootErrorInfo = ref("");

onErrorCaptured((err, instance, info) => {
  rootError.value = true;
  rootErrorSummary.value = err?.message || String(err);
  rootErrorStack.value = (err as Error)?.stack || "";
  rootErrorInfo.value = info;
  console.error("[Simverse] Vue error captured:", err, info);
  return false;
});
</script>

<style>
ion-app {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Oxygen, Ubuntu, sans-serif;
}
</style>
