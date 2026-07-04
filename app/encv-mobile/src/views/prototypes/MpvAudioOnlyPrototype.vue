<template>
  <div class="audio-player-prototype">
    <div class="audio-screen">
      <div class="audio-top-bar">
        <button class="icon-btn" @click="onBack">
          <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z"/></svg>
        </button>
        <span class="audio-top-title">Now Playing</span>
        <div style="width:40px"></div>
      </div>

      <div class="album-art-area">
        <div class="album-art" :class="{ spinning: isPlaying }">
          <div class="album-disc">
            <div class="disc-groove"></div>
            <div class="disc-groove outer"></div>
            <div class="disc-center">
              <svg viewBox="0 0 24 24" width="32" height="32" fill="currentColor" style="opacity:.6"><path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z"/></svg>
            </div>
          </div>
        </div>
      </div>

      <div class="track-info">
        <h2 class="track-title">{{ fileName }}</h2>
        <p class="track-artist">Unknown Artist</p>
      </div>

      <div class="progress-section">
        <mpv-progress-bar
          :progress="progress"
          :current-position="currentPosition"
          :duration="duration"
          @seek="handleSeek"
        />
      </div>

      <div class="main-controls">
        <button class="ctrl-btn" @click="seekDelta(-10000)">
          <svg viewBox="0 0 24 24" width="28" height="28" fill="currentColor"><path d="M11 18V6l-8.5 6 8.5 6zm.5-6l8.5 6V6l-8.5 6z"/></svg>
        </button>
        <button class="play-pause-btn" @click="togglePlay">
          <svg v-if="isPlaying" viewBox="0 0 24 24" width="36" height="36" fill="currentColor"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>
          <svg v-else viewBox="0 0 24 24" width="36" height="36" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>
        </button>
        <button class="ctrl-btn" @click="seekDelta(10000)">
          <svg viewBox="0 0 24 24" width="28" height="28" fill="currentColor"><path d="M4 18l8.5-6L4 6v12zm9-12v12l8.5-6L13 6z"/></svg>
        </button>
      </div>

      <div class="secondary-controls">
        <button class="sec-btn" @click="cycleSpeed">
          <span class="speed-label">{{ playbackSpeed }}x</span>
        </button>
        <button class="sec-btn" @click="toggleMute">
          <svg v-if="volume > 0" viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z"/></svg>
          <svg v-else viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63z"/></svg>
        </button>
        <div class="volume-mini">
          <div class="volume-track" ref="volumeTrackRef" @click="onVolumeTrackClick">
            <div class="volume-fill" :style="{ width: (volume * 100) + '%' }"></div>
          </div>
        </div>
      </div>
    </div>

    <div class="prototype-toolbar">
      <label>State:</label>
      <select v-model="playerState">
        <option value="playing">Playing</option>
        <option value="paused">Paused</option>
      </select>
      <label>Track:</label>
      <input v-model="fileName" class="file-input" />
      <label>Duration:</label>
      <input v-model.number="durationInput" type="number" class="dur-input" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import MpvProgressBar from "./MpvProgressBarWeb.vue";

const playerState = ref<"playing" | "paused">("paused");
const fileName = ref("Bohemian Rhapsody.flac");
const durationInput = ref(354000);
const currentPosition = ref(86000);
const isPlaying = computed(() => playerState.value === "playing");
const duration = computed(() => durationInput.value);
const progress = computed(() => (duration.value > 0 ? currentPosition.value / duration.value : 0));
const playbackSpeed = ref(1.0);
const volume = ref(0.8);
const volumeTrackRef = ref<HTMLElement>();
const SPEED_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2];

function togglePlay() {
  playerState.value = playerState.value === "playing" ? "paused" : "playing";
}

function handleSeek(ratio: number) {
  currentPosition.value = Math.round(ratio * duration.value);
}

function seekDelta(ms: number) {
  currentPosition.value = Math.max(0, Math.min(duration.value, currentPosition.value + ms));
}

function cycleSpeed() {
  const idx = SPEED_OPTIONS.indexOf(playbackSpeed.value);
  playbackSpeed.value = SPEED_OPTIONS[(idx + 1) % SPEED_OPTIONS.length];
}

function toggleMute() {
  volume.value = volume.value > 0 ? 0 : 0.8;
}

function onVolumeTrackClick(e: MouseEvent) {
  if (!volumeTrackRef.value) return;
  const rect = volumeTrackRef.value.getBoundingClientRect();
  volume.value = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
}

function onBack() {}

watch(isPlaying, val => {
  if (val) {
    const interval = setInterval(() => {
      if (!isPlaying.value) {
        clearInterval(interval);
        return;
      }
      if (currentPosition.value < duration.value) {
        currentPosition.value += 1000;
      } else {
        playerState.value = "paused";
        clearInterval(interval);
      }
    }, 1000);
  }
});
</script>

<style scoped>
.audio-player-prototype {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.audio-screen {
  background: linear-gradient(170deg, #1a1a2e 0%, #16213e 40%, #0f3460 100%);
  padding: 0 20px 24px;
  display: flex;
  flex-direction: column;
  min-height: 520px;
}

.audio-top-bar {
  display: flex;
  align-items: center;
  padding: 12px 0;
}

.audio-top-title {
  flex: 1;
  text-align: center;
  color: rgba(255,255,255,0.6);
  font-size: 13px;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.album-art-area {
  display: flex;
  justify-content: center;
  padding: 20px 0 28px;
}

.album-art {
  width: 200px;
  height: 200px;
  border-radius: 50%;
  overflow: hidden;
  box-shadow: 0 8px 40px rgba(0,0,0,0.5), 0 0 80px rgba(187,134,252,0.15);
}

.album-art.spinning {
  animation: spin 8s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.album-disc {
  width: 100%;
  height: 100%;
  background: radial-gradient(circle at 50% 50%,
    #2a2a3e 0%, #1e1e30 20%, #2a2a3e 21%, #1e1e30 40%,
    #2a2a3e 41%, #1e1e30 60%, #2a2a3e 61%, #1e1e30 80%,
    #2a2a3e 81%, #1a1a2e 100%
  );
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
}

.disc-groove {
  position: absolute;
  border-radius: 50%;
  border: 1px solid rgba(255,255,255,0.04);
  width: 140px;
  height: 140px;
}

.disc-groove.outer {
  width: 180px;
  height: 180px;
}

.disc-center {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: linear-gradient(135deg, #BB86FC, #7C4DFF);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  z-index: 1;
}

.track-info {
  text-align: center;
  padding: 0 16px 20px;
}

.track-title {
  color: #fff;
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.track-artist {
  color: rgba(255,255,255,0.5);
  font-size: 14px;
  margin: 0;
}

.progress-section {
  padding: 0 4px 16px;
}

.main-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 36px;
  padding: 4px 0 20px;
}

.play-pause-btn {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  border: none;
  background: linear-gradient(135deg, #BB86FC, #7C4DFF);
  color: #fff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 20px rgba(187,134,252,0.4);
  transition: transform 0.15s, box-shadow 0.15s;
}

.play-pause-btn:hover {
  transform: scale(1.08);
  box-shadow: 0 6px 28px rgba(187,134,252,0.5);
}

.play-pause-btn:active {
  transform: scale(0.96);
}

.ctrl-btn {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  border: none;
  background: rgba(255,255,255,0.08);
  color: rgba(255,255,255,0.8);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s;
}

.ctrl-btn:hover {
  background: rgba(255,255,255,0.14);
}

.secondary-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 0 8px;
}

.sec-btn {
  height: 32px;
  padding: 0 12px;
  border-radius: 16px;
  border: 1px solid rgba(255,255,255,0.15);
  background: transparent;
  color: rgba(255,255,255,0.7);
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  transition: border-color 0.15s;
}

.sec-btn:hover {
  border-color: rgba(187,134,252,0.5);
}

.speed-label {
  font-weight: 600;
  color: #BB86FC;
}

.volume-mini {
  width: 100px;
}

.volume-track {
  height: 4px;
  background: rgba(255,255,255,0.15);
  border-radius: 2px;
  position: relative;
  cursor: pointer;
}

.volume-track:hover {
  height: 6px;
  margin-top: -1px;
  margin-bottom: -1px;
}

.volume-fill {
  position: absolute;
  left: 0;
  top: 0;
  height: 100%;
  background: #BB86FC;
  border-radius: 2px;
  pointer-events: none;
}

.icon-btn {
  width: 40px;
  height: 40px;
  border: none;
  background: none;
  color: #fff;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
}

.prototype-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  background: var(--ion-card-background, #1e1e1e);
  border-radius: 0 0 12px 12px;
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  flex-wrap: wrap;
}

.prototype-toolbar label {
  font-weight: 600;
  color: var(--ion-text-color, #fff);
}

.prototype-toolbar select,
.prototype-toolbar input {
  background: rgba(255,255,255,0.08);
  border: 1px solid rgba(255,255,255,0.12);
  border-radius: 6px;
  padding: 4px 8px;
  color: var(--ion-text-color, #fff);
  font-size: 12px;
}

.file-input { width: 160px; }
.dur-input { width: 80px; }
</style>
