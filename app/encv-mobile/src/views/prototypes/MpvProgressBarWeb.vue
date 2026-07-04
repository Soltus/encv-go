<template>
  <div class="mpv-progress-bar">
    <span class="time-label">{{ formatTime(currentPosition) }}</span>
    <div class="slider-track" ref="trackRef" @click="handleTrackClick">
      <div class="slider-fill" :style="{ width: (progress * 100) + '%' }"></div>
      <div class="slider-thumb" :style="{ left: (progress * 100) + '%' }"></div>
    </div>
    <span class="time-label">{{ formatTime(duration) }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";

defineProps<{
  progress: number;
  currentPosition: number;
  duration: number;
}>();

const emit = defineEmits<{
  seek: [ratio: number];
}>();

const trackRef = ref<HTMLElement>();

function formatTime(ms: number): string {
  if (ms < 0) return "0:00";
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function handleTrackClick(e: MouseEvent) {
  if (!trackRef.value) return;
  const rect = trackRef.value.getBoundingClientRect();
  const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
  emit("seek", ratio);
}
</script>

<style scoped>
.mpv-progress-bar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px 4px;
}

.time-label {
  width: 44px;
  font-size: 11px;
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: rgba(191, 191, 191, 0.8);
  text-align: center;
  flex-shrink: 0;
}

.slider-track {
  flex: 1;
  height: 4px;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
  position: relative;
  cursor: pointer;
}

.slider-track:hover {
  height: 6px;
  margin-top: -1px;
  margin-bottom: -1px;
}

.slider-fill {
  position: absolute;
  left: 0;
  top: 0;
  height: 100%;
  background: #BB86FC;
  border-radius: 2px;
  pointer-events: none;
}

.slider-thumb {
  position: absolute;
  top: 50%;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #BB86FC;
  transform: translate(-50%, -50%);
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.15s;
}

.slider-track:hover .slider-thumb {
  opacity: 1;
}
</style>
