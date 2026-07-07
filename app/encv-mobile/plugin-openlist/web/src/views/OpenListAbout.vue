<template>
  <AboutPage
    app-name="OpenList"
    :app-version="`v${version}`"
    engine-name="OpenList Backend"
    license="MIT License"
    back-href="/settings"
  />
</template>

<script setup lang="ts">
import AboutPage from "@encv/shared-components/components/about/AboutPage.vue";
import { ref, onMounted } from "vue";
import { OpenListNative } from "@/plugins/openlist-native";

const version = ref("unknown");

onMounted(() => {
  version.value = OpenListNative.getVersion();
});
</script>
