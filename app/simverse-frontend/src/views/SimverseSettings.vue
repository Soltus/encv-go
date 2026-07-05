<template>
  <div style="padding: 20px;">
    <h1>🧩 SimVerse 设置</h1>
    <p>这是设置页面</p>
    <button @click="router.push('/')">← 返回首页</button>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router';
const router = useRouter();
</script>