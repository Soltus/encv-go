<template>
  <div>
    <div style="background: #333; color: #fff; padding: 10px; display: flex; gap: 10px; flex-wrap: wrap;">
      <button @click="router.push('/')">首页</button>
      <button @click="router.push('/world')">世界</button>
      <button @click="router.push('/tabs/settings')">设置</button>
      <button @click="router.push('/tabs/devlogs')">日志</button>
    </div>
    
    <div style="padding: 20px;">
      <component :is="currentComponent" />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import SimverseHome from './views/SimverseHome.vue';
import SimverseSettings from './views/SimverseSettings.vue';
import SimverseDevLogs from './views/SimverseDevLogs.vue';

const router = useRouter();
const currentComponent = computed(() => {
  const path = router.currentRoute.value.path;
  if (path === '/world') return { template: '<div><h1>🌍 世界页面</h1><button @click="router.push(\"/\")">← 返回</button></div>', methods: { router } };
  if (path === '/tabs/settings') return SimverseSettings;
  if (path === '/tabs/devlogs') return SimverseDevLogs;
  return SimverseHome;
});
</script>