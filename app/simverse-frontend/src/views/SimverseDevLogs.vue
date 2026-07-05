<template>
  <div style="padding: 20px;">
    <h1>📋 开发日志</h1>
    <p>暂无日志</p>
    <button @click="router.push('/')">← 返回首页</button>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router';
const router = useRouter();
</script>