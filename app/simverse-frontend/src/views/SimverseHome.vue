<template>
  <div>
    <div style="background: #333; color: #fff; padding: 10px; display: flex; gap: 10px;">
      <button @click="go('/')">首页</button>
      <button @click="go('/world')">世界</button>
      <button @click="go('/tabs/settings')">设置</button>
    </div>
    
    <router-view />
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router';
const router = useRouter();
function go(path) { router.push(path); }
</script>