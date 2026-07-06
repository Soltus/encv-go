<!-- DebugConsole.vue - 控制台输出调试面板 -->

<template>
  <div class="debug-console" @click="expanded = !expanded">
    <div class="debug-console-header">
      <span>Console</span>
      <span class="debug-console-count" v-if="logs.length">{{ logs.length }}</span>
      <button type="button" class="debug-console-clear" @click.stop="clear">×</button>
    </div>
    <div class="debug-console-body" v-if="expanded || !logs.length">
      <div
        v-for="(log, i) in logs"
        :key="i"
        class="debug-console-line"
        :class="log.type"
      >{{ log.text }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";

const expanded = ref(true);
const logs = ref<{ type: string; text: string }[]>([]);

function clear() {
  logs.value = [];
}

onMounted(() => {
  // 劫持 console 方法
  const origLog = console.log;
  const origError = console.error;
  const origWarn = console.warn;

  function addLog(type: string, args: unknown[]) {
    const text = args
      .map(a => {
        if (typeof a === "object") {
          try {
            return JSON.stringify(a);
          } catch {
            return String(a);
          }
        }
        return String(a);
      })
      .join(" ");
    logs.value.push({ type, text });
    // 只保留最近 50 条
    if (logs.value.length > 50) {
      logs.value = logs.value.slice(-50);
    }
  }

  console.log = (...args: unknown[]) => {
    addLog("log", args);
    origLog.apply(console, args);
  };
  console.error = (...args: unknown[]) => {
    addLog("error", args);
    origError.apply(console, args);
  };
  console.warn = (...args: unknown[]) => {
    addLog("warn", args);
    origWarn.apply(console, args);
  };
});
</script>

<style scoped>
.debug-console {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 120px;
  background: #222;
  color: #0f0;
  font: 10px monospace;
  z-index: 99999;
  overflow: hidden;
  border-bottom: 1px solid #444;
}

.debug-console-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  background: #111;
  cursor: pointer;
}

.debug-console-count {
  font-size: 9px;
  color: #888;
}

.debug-console-clear {
  margin-left: auto;
  background: none;
  border: none;
  color: #f00;
  font-size: 14px;
  cursor: pointer;
}

.debug-console-body {
  height: calc(100% - 24px);
  overflow-y: auto;
  padding: 4px 8px;
  white-space: pre-wrap;
  word-break: break-all;
}

.debug-console-line {
  line-height: 1.3;
}

.debug-console-line.error {
  color: #f55;
}

.debug-console-line.warn {
  color: #ff5;
}
</style>