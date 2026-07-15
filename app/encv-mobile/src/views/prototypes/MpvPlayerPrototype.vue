<template>
  <div class="mpv-prototype-root">
    <div class="player-screen" @click="handleTap">
    <div class="video-area">
      <div class="video-placeholder">
        <svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="currentColor" stroke-width="1.5">
          <rect x="2" y="4" width="20" height="16" rx="2"/>
          <polygon points="10,8 16,12 10,16"/>
        </svg>
        <span class="placeholder-text">Video Surface</span>
      </div>
    </div>

    <transition name="fade">
      <div v-if="showControls && !isLocked" class="controls-overlay">
        <div class="top-bar">
          <button class="icon-btn" @click.stop="onBack">
            <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z"/></svg>
          </button>
          <span class="top-title">{{ fileName }}</span>
          <button class="icon-btn" @click.stop="togglePanel('settings')">
            <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor"><path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58a.49.49 0 00.12-.61l-1.92-3.32a.49.49 0 00-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54a.484.484 0 00-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.49.49 0 00-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.07.62-.07.94s.02.64.07.94l-2.03 1.58a.49.49 0 00-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/></svg>
          </button>
        </div>

        <div class="center-controls">
          <button class="seek-delta-btn" @click.stop="seekDelta(-10000)">-10s</button>
          <button class="play-btn" @click.stop="togglePlay">
            <svg v-if="isPlaying" viewBox="0 0 24 24" width="40" height="40" fill="currentColor"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>
            <svg v-else viewBox="0 0 24 24" width="40" height="40" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>
          </button>
          <button class="seek-delta-btn" @click.stop="seekDelta(10000)">+10s</button>
        </div>

        <div class="side-lock" @click.stop>
          <button class="lock-btn" @click.stop="isLocked = true">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M18 8h-1V6c0-2.76-2.24-5-5-5S7 3.24 7 6v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zM12 17c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2zM15.1 8H8.9V6c0-1.71 1.39-3.1 3.1-3.1s3.1 1.39 3.1 3.1v2z"/></svg>
          </button>
        </div>

        <div class="bottom-bar">
          <mpv-progress-bar
            :progress="progress"
            :current-position="currentPosition"
            :duration="duration"
            @seek="handleSeek"
          />
          <div class="bottom-actions">
            <button class="speed-chip" @click.stop="cycleSpeed">{{ playbackSpeed.toFixed(playbackSpeed === Math.round(playbackSpeed) ? 1 : 2) }}x</button>
            <div class="spacer"></div>
            <div class="popover-wrap">
              <transition name="pop">
                <div v-if="activePanel === 'subtitles'" class="popup-panel" @click.stop>
                  <div class="popup-title">Subtitles</div>
                  <button v-for="s in subtitleTracks" :key="s.id" class="popup-item" :class="{ active: selectedSubtitle === s.id }" @click="selectedSubtitle = s.id; activePanel = ''">
                    <span>{{ s.label }}</span>
                    <svg v-if="selectedSubtitle === s.id" viewBox="0 0 24 24" width="16" height="16" fill="#BB86FC"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
                  </button>
                  <div class="popup-divider"></div>
                  <button class="popup-item file-pick-item" @click="pickSubtitleFile">
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" style="margin-right:8px;flex-shrink:0"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" y1="18" x2="12" y2="12"/><line x1="9" y1="15" x2="15" y2="15"/></svg>
                    <span>{{ externalSubtitleName || 'Choose subtitle file…' }}</span>
                  </button>
                </div>
              </transition>
              <button class="icon-btn sm" :class="{ active: activePanel === 'subtitles' || selectedSubtitle !== 'none' }" @click.stop="togglePanel('subtitles')" title="Subtitles">
                <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="4" width="20" height="16" rx="2"/><path d="M7 12h2m4 0h4M7 16h10"/></svg>
              </button>
            </div>
            <div class="popover-wrap">
              <transition name="pop">
                <div v-if="activePanel === 'audio'" class="popup-panel" @click.stop>
                  <div class="popup-title">Audio Track</div>
                  <button v-for="a in audioTracks" :key="a.id" class="popup-item" :class="{ active: selectedAudio === a.id }" @click="selectedAudio = a.id; activePanel = ''">
                    <span>{{ a.label }}</span>
                    <svg v-if="selectedAudio === a.id" viewBox="0 0 24 24" width="16" height="16" fill="#BB86FC"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
                  </button>
                </div>
              </transition>
              <button class="icon-btn sm" :class="{ active: activePanel === 'audio' }" @click.stop="togglePanel('audio')" title="Audio track">
                <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M8 12h8"/></svg>
              </button>
            </div>
            <div class="volume-popover-wrap">
              <transition name="vol-pop">
                <div v-if="showVolumeSlider" class="volume-popover" @click.stop>
                  <div class="vol-vertical-track" ref="volumeTrackRef" @click="onVolumeTrackClick" @mousemove="onVolumeDrag" @mousedown="startDrag" @mouseup="stopDrag">
                    <div class="vol-vertical-fill" :style="{ height: (volume * 100) + '%' }"></div>
                    <div class="vol-vertical-thumb" :style="{ bottom: (volume * 100) + '%' }"></div>
                  </div>
                  <span class="vol-pct">{{ Math.round(volume * 100) }}</span>
                </div>
              </transition>
              <button class="icon-btn sm" @click.stop="toggleVolumeSlider" title="Volume">
                <svg v-if="volume > 0" viewBox="0 0 24 24" width="22" height="22" fill="currentColor"><path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z"/></svg>
                <svg v-else viewBox="0 0 24 24" width="22" height="22" fill="currentColor"><path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z"/></svg>
              </button>
            </div>
            <button class="icon-btn sm" @click.stop="sandbox.toggleLandscape()" :title="sandbox.isLandscape.value ? 'Portrait' : 'Landscape'">
              <svg v-if="!sandbox.isLandscape.value" viewBox="0 0 24 24" width="22" height="22" fill="currentColor"><path d="M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z"/></svg>
              <svg v-else viewBox="0 0 24 24" width="22" height="22" fill="currentColor"><path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z"/></svg>
            </button>
          </div>
        </div>
      </div>
    </transition>

    <transition name="fade">
      <div v-if="isLocked && showControls" class="locked-overlay">
        <div class="side-lock locked" @click.stop>
          <button class="lock-btn locked" @click.stop="isLocked = false">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M18 8h-1V6c0-2.76-2.24-5-5-5S7 3.24 7 6h1.9c0-1.71 1.39-3.1 3.1-3.1s3.1 1.39 3.1 3.1v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zm-6 9c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z"/></svg>
          </button>
        </div>
        <div class="locked-bottom">
          <mpv-progress-bar
            :progress="progress"
            :current-position="currentPosition"
            :duration="duration"
            @seek="handleSeek"
          />
        </div>
      </div>
    </transition>

    <transition name="fade">
      <div v-if="activePanel === 'settings' && showControls" class="settings-overlay" @click.stop="activePanel = ''">
        <div class="settings-panel" @click.stop>
          <div class="popup-title">Settings</div>
          <div class="settings-group">
            <div class="speed-slider-header">
              <span class="settings-label">Playback Speed</span>
              <span class="speed-value">{{ playbackSpeed.toFixed(playbackSpeed === Math.round(playbackSpeed) ? 1 : 2) }}x</span>
            </div>
            <div class="speed-slider-wrap" ref="speedSliderRef" @mousedown="onSpeedSliderDown" @click="onSpeedSliderClick">
              <div class="speed-track">
                <div class="speed-track-fill" :style="{ width: speedSliderPercent + '%' }"></div>
                <div
                  v-for="pt in SPEED_ANCHORS"
                  :key="pt"
                  class="speed-anchor"
                  :class="{ active: Math.abs(playbackSpeed - pt) < 0.05 }"
                  :style="{ left: ((pt - SPEED_MIN) / (SPEED_MAX - SPEED_MIN) * 100) + '%' }"
                ></div>
                <div class="speed-thumb" :style="{ left: speedSliderPercent + '%' }"></div>
              </div>
              <div class="speed-labels">
                <span v-for="pt in SPEED_ANCHORS" :key="pt" class="speed-label" :class="{ active: Math.abs(playbackSpeed - pt) < 0.05 }" :style="{ left: ((pt - SPEED_MIN) / (SPEED_MAX - SPEED_MIN) * 100) + '%' }">
                  {{ pt }}x
                </span>
              </div>
            </div>
          </div>
          <div class="settings-group">
            <span class="settings-label">Subtitle Delay</span>
            <div class="delay-row">
              <button class="delay-btn" @click="subtitleDelay -= 0.5">-0.5s</button>
              <span class="delay-val">{{ subtitleDelay >= 0 ? '+' : '' }}{{ subtitleDelay.toFixed(1) }}s</span>
              <button class="delay-btn" @click="subtitleDelay += 0.5">+0.5s</button>
            </div>
          </div>
          <div class="settings-group">
            <span class="settings-label">Audio Delay</span>
            <div class="delay-row">
              <button class="delay-btn" @click="audioDelay -= 0.5">-0.5s</button>
              <span class="delay-val">{{ audioDelay >= 0 ? '+' : '' }}{{ audioDelay.toFixed(1) }}s</span>
              <button class="delay-btn" @click="audioDelay += 0.5">+0.5s</button>
            </div>
          </div>
          <div class="settings-group">
            <div class="toggle-row">
              <div class="toggle-info">
                <span class="toggle-label">Background Playback</span>
                <span class="toggle-desc">Audio only in background, resume position on return</span>
              </div>
              <button class="toggle-switch" :class="{ on: bgPlayback }" @click="bgPlayback = !bgPlayback">
                <span class="toggle-knob"></span>
              </button>
            </div>
          </div>
          <div class="settings-group">
            <button class="pip-btn" @click="enterPip">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" style="margin-right:8px;flex-shrink:0"><rect x="2" y="3" width="20" height="14" rx="2"/><rect x="11" y="9" width="10" height="7" rx="1.5" fill="rgba(187,134,252,0.2)" stroke="currentColor"/></svg>
              <span>Picture-in-Picture</span>
            </button>
          </div>
        </div>
      </div>
    </transition>

    <transition name="pip-fade">
      <div v-if="isPipMode" class="pip-window" @click.stop="exitPip">
        <div class="pip-video">
          <div class="video-placeholder pip-placeholder">
            <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="4" width="20" height="16" rx="2"/>
              <polygon points="10,8 16,12 10,16"/>
            </svg>
          </div>
          <div class="pip-controls">
            <button class="pip-ctrl-btn" @click.stop="togglePlay">
              <svg v-if="isPlaying" viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>
              <svg v-else viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>
            </button>
            <button class="pip-ctrl-btn pip-close-ctrl" @click.stop="exitPip">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
            </button>
          </div>
        </div>
      </div>
    </transition>
    </div>
    <div class="prototype-toolbar">
      <label>State:</label>
      <select v-model="playerState">
        <option value="idle">Idle</option>
        <option value="loading">Loading</option>
        <option value="playing">Playing</option>
        <option value="paused">Paused</option>
        <option value="audioOnly">Audio Only</option>
        <option value="error">Error</option>
      </select>
      <label>File:</label>
      <input v-model="fileName" class="file-input" />
      <label>Duration:</label>
      <input v-model.number="durationInput" type="number" class="dur-input" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, type Ref, ref, watch } from "vue";

const sandbox = inject<{ isLandscape: Ref<boolean>; toggleLandscape: () => void }>("sandboxLandscape", {
  isLandscape: ref(false),
  toggleLandscape: () => {},
});

const SPEED_MIN = 0.5;
const SPEED_MAX = 3.0;
const SPEED_ANCHORS = [0.5, 0.75, 1.0, 1.25, 1.5, 2.0, 3.0];
const SNAP_THRESHOLD = 0.08;

const playerState = ref<"idle" | "loading" | "playing" | "paused" | "audioOnly" | "error">("paused");
const fileName = ref("Big.Buck.Bunny.1080p.mkv");
const durationInput = ref(596000);
const currentPosition = ref(142000);
const isPlaying = computed(() => playerState.value === "playing" || playerState.value === "audioOnly");
const duration = computed(() => durationInput.value);
const progress = computed(() => (duration.value > 0 ? currentPosition.value / duration.value : 0));
const showControls = ref(true);
const isLocked = ref(false);
const playbackSpeed = ref(1.0);
const volume = ref(0.8);
const showVolumeSlider = ref(false);
const isDragging = ref(false);
const volumeTrackRef = ref<HTMLElement>();
const isSpeedDragging = ref(false);
const speedSliderRef = ref<HTMLElement>();
const activePanel = ref<"" | "settings" | "subtitles" | "audio">("");
const selectedSubtitle = ref("none");
const selectedAudio = ref("1");
const subtitleDelay = ref(0);
const audioDelay = ref(0);
const bgPlayback = ref(false);
const isPipMode = ref(false);
const externalSubtitleName = ref("");

const speedSliderPercent = computed(() => {
  return ((playbackSpeed.value - SPEED_MIN) / (SPEED_MAX - SPEED_MIN)) * 100;
});

const subtitleTracks = [
  { id: "none", label: "None" },
  { id: "1", label: "English" },
  { id: "2", label: "Chinese (Simplified)" },
  { id: "3", label: "Chinese (Traditional)" },
  { id: "4", label: "Japanese" },
];

const audioTracks = [
  { id: "1", label: "Japanese (5.1)" },
  { id: "2", label: "English (Stereo)" },
  { id: "3", label: "Chinese (Stereo)" },
];

function snapSpeed(val: number): number {
  for (const anchor of SPEED_ANCHORS) {
    if (Math.abs(val - anchor) < SNAP_THRESHOLD) return anchor;
  }
  return val;
}

function speedFromX(clientX: number) {
  if (!speedSliderRef.value) return;
  const rect = speedSliderRef.value.getBoundingClientRect();
  const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
  let raw = SPEED_MIN + ratio * (SPEED_MAX - SPEED_MIN);
  raw = Math.round(raw * 10) / 10;
  playbackSpeed.value = snapSpeed(raw);
}

function onSpeedSliderClick(e: MouseEvent) {
  if (isSpeedDragging.value) return;
  speedFromX(e.clientX);
}

function onSpeedSliderDown(e: MouseEvent) {
  isSpeedDragging.value = true;
  speedFromX(e.clientX);
  const onMove = (ev: MouseEvent) => speedFromX(ev.clientX);
  const onUp = () => {
    isSpeedDragging.value = false;
    window.removeEventListener("mousemove", onMove);
    window.removeEventListener("mouseup", onUp);
  };
  window.addEventListener("mousemove", onMove);
  window.addEventListener("mouseup", onUp);
}

function togglePanel(panel: typeof activePanel.value) {
  if (activePanel.value === panel) {
    activePanel.value = "";
  } else {
    activePanel.value = panel;
    showVolumeSlider.value = false;
  }
}

function handleTap() {
  if (isLocked.value) {
    isLocked.value = false;
  } else {
    showControls.value = !showControls.value;
    activePanel.value = "";
    showVolumeSlider.value = false;
  }
}

function togglePlay() {
  if (playerState.value === "playing") playerState.value = "paused";
  else if (playerState.value === "paused") playerState.value = "playing";
  else playerState.value = "playing";
  showControls.value = true;
}

function handleSeek(ratio: number) {
  currentPosition.value = Math.round(ratio * duration.value);
}

function seekDelta(ms: number) {
  currentPosition.value = Math.max(0, Math.min(duration.value, currentPosition.value + ms));
  showControls.value = true;
}

function cycleSpeed() {
  const current = playbackSpeed.value;
  let best = SPEED_ANCHORS[0];
  for (const a of SPEED_ANCHORS) {
    if (a > current + 0.01) {
      best = a;
      break;
    }
    best = a;
  }
  playbackSpeed.value = best;
}

function toggleVolumeSlider() {
  showVolumeSlider.value = !showVolumeSlider.value;
  if (showVolumeSlider.value) activePanel.value = "";
}

function onVolumeTrackClick(e: MouseEvent) {
  if (!volumeTrackRef.value) return;
  const rect = volumeTrackRef.value.getBoundingClientRect();
  const ratio = 1 - Math.max(0, Math.min(1, (e.clientY - rect.top) / rect.height));
  volume.value = Math.round(ratio * 100) / 100;
}

function onVolumeDrag(e: MouseEvent) {
  if (!isDragging.value) return;
  onVolumeTrackClick(e);
}

function startDrag(e: MouseEvent) {
  isDragging.value = true;
  onVolumeTrackClick(e);
}

function stopDrag() {
  isDragging.value = false;
}

function pickSubtitleFile() {
  const input = document.createElement("input");
  input.type = "file";
  input.accept = ".srt,.ass,.ssa,.vtt,.lrc,.sub";
  input.onchange = () => {
    const file = input.files?.[0];
    if (file) {
      externalSubtitleName.value = file.name;
      selectedSubtitle.value = "external";
      activePanel.value = "";
    }
  };
  input.click();
}

function enterPip() {
  isPipMode.value = true;
  activePanel.value = "";
  showControls.value = false;
}

function exitPip() {
  isPipMode.value = false;
  showControls.value = true;
}

function onBack() {
  showControls.value = true;
}

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
.mpv-prototype-root {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
}

.player-screen {
  position: relative;
  flex: 1;
  min-height: 0;
  background: #121212;
  overflow: hidden;
  user-select: none;
  cursor: pointer;
}

.video-area {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.video-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: rgba(255,255,255,0.2);
}

.placeholder-text {
  font-size: 12px;
  letter-spacing: 1px;
  text-transform: uppercase;
}

.controls-overlay,
.locked-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  pointer-events: none;
}

.controls-overlay > *,
.locked-overlay > * {
  pointer-events: auto;
}

.top-bar {
  display: flex;
  align-items: center;
  padding: 8px 8px 16px;
  background: linear-gradient(to bottom, rgba(0,0,0,0.6), transparent);
}

.top-title {
  flex: 1;
  color: var(--color-white);
  font-size: 14px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 0 4px;
}

.center-controls {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 32px;
}

.side-lock {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  z-index: 5;
}

.lock-btn {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: 1px solid rgba(255,255,255,0.15);
  background: rgba(255,255,255,0.08);
  color: rgba(255,255,255,0.6);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.lock-btn.locked {
  border-color: rgba(187,134,252,0.5);
  background: rgba(187,134,252,0.15);
  color: #BB86FC;
}

.lock-btn:hover {
  background: rgba(255,255,255,0.14);
}

.play-btn {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  border: none;
  background: rgba(255,255,255,0.15);
  color: rgba(255,255,255,0.9);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s;
}

.play-btn:hover {
  background: rgba(255,255,255,0.25);
}

.seek-delta-btn {
  background: none;
  border: none;
  color: rgba(255,255,255,0.7);
  font-size: 14px;
  font-weight: 700;
  cursor: pointer;
  padding: 8px;
}

.bottom-bar {
  display: flex;
  flex-direction: column;
  background: linear-gradient(to top, rgba(0,0,0,0.6), transparent);
  padding-top: 16px;
}

.bottom-actions {
  display: flex;
  align-items: center;
  padding: 6px 12px;
  gap: 4px;
}

.spacer { flex: 1; }

.speed-chip {
  height: 28px;
  padding: 0 10px;
  border-radius: 14px;
  border: 1px solid rgba(187,134,252,0.5);
  background: transparent;
  color: #BB86FC;
  font-size: 12px;
  cursor: pointer;
}

.popover-wrap {
  position: relative;
}

.popup-panel {
  position: absolute;
  bottom: 44px;
  right: 0;
  min-width: 200px;
  background: rgba(24, 24, 30, 0.96);
  border-radius: 12px;
  padding: 6px 0;
  box-shadow: 0 4px 20px rgba(0,0,0,0.5);
  backdrop-filter: blur(12px);
  z-index: 20;
}

.popup-title {
  padding: 8px 14px 6px;
  font-size: 12px;
  font-weight: 700;
  color: rgba(255,255,255,0.5);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.popup-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 14px;
  border: none;
  background: none;
  color: rgba(255,255,255,0.85);
  font-size: 13px;
  cursor: pointer;
  text-align: left;
  transition: background 0.12s;
}

.popup-item:hover {
  background: rgba(255,255,255,0.08);
}

.popup-item.active {
  color: #BB86FC;
}

.popup-divider {
  height: 1px;
  background: rgba(255,255,255,0.08);
  margin: 4px 0;
}

.file-pick-item {
  color: rgba(187,134,252,0.9);
}

.file-pick-item:hover {
  background: rgba(187,134,252,0.08);
}

.settings-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0,0,0,0.4);
  z-index: 30;
}

.settings-panel {
  width: 300px;
  max-width: 90%;
  max-height: 90%;
  overflow-y: auto;
  background: rgba(24, 24, 30, 0.97);
  border-radius: 16px;
  padding: 16px;
  box-shadow: 0 8px 40px rgba(0,0,0,0.5);
  backdrop-filter: blur(16px);
}

.settings-group {
  padding: 10px 0;
  border-bottom: 1px solid rgba(255,255,255,0.06);
}

.settings-group:last-child {
  border-bottom: none;
}

.settings-label {
  display: block;
  font-size: 12px;
  color: rgba(255,255,255,0.5);
  margin-bottom: 8px;
}

.speed-slider-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 8px;
}

.speed-slider-header .settings-label {
  margin-bottom: 0;
}

.speed-value {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 14px;
  font-weight: 600;
  color: #BB86FC;
}

.speed-slider-wrap {
  position: relative;
  padding: 8px 0 24px;
  cursor: pointer;
  touch-action: none;
}

.speed-track {
  position: relative;
  height: 4px;
  background: rgba(255,255,255,0.12);
  border-radius: 2px;
}

.speed-track-fill {
  position: absolute;
  top: 0;
  left: 0;
  height: 100%;
  background: #BB86FC;
  border-radius: 2px;
  pointer-events: none;
}

.speed-anchor {
  position: absolute;
  top: 50%;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: rgba(255,255,255,0.2);
  transform: translate(-50%, -50%);
  transition: all 0.15s;
  pointer-events: none;
}

.speed-anchor.active {
  background: #BB86FC;
  width: 10px;
  height: 10px;
  box-shadow: 0 0 6px rgba(187,134,252,0.5);
}

.speed-thumb {
  position: absolute;
  top: 50%;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #BB86FC;
  transform: translate(-50%, -50%);
  box-shadow: 0 1px 4px rgba(0,0,0,0.3);
  pointer-events: none;
  transition: box-shadow 0.15s;
}

.speed-slider-wrap:hover .speed-thumb,
.speed-slider-wrap:active .speed-thumb {
  box-shadow: 0 0 0 6px rgba(187,134,252,0.15), 0 1px 4px rgba(0,0,0,0.3);
}

.speed-labels {
  position: relative;
  height: 16px;
  margin-top: 6px;
}

.speed-label {
  position: absolute;
  transform: translateX(-50%);
  font-size: 10px;
  color: rgba(255,255,255,0.3);
  transition: color 0.15s;
  white-space: nowrap;
}

.speed-label.active {
  color: #BB86FC;
  font-weight: 600;
}

.delay-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.delay-btn {
  padding: 4px 10px;
  border-radius: 8px;
  border: 1px solid rgba(255,255,255,0.12);
  background: transparent;
  color: rgba(255,255,255,0.7);
  font-size: 12px;
  cursor: pointer;
}

.delay-val {
  flex: 1;
  text-align: center;
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 13px;
  color: #BB86FC;
}

.toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.toggle-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
}

.toggle-label {
  font-size: 14px;
  color: rgba(255,255,255,0.9);
}

.toggle-desc {
  font-size: 11px;
  color: rgba(255,255,255,0.4);
  line-height: 1.3;
}

.toggle-switch {
  width: 44px;
  height: 24px;
  border-radius: 12px;
  border: none;
  background: rgba(255,255,255,0.15);
  cursor: pointer;
  position: relative;
  transition: background 0.2s;
  flex-shrink: 0;
}

.toggle-switch.on {
  background: #BB86FC;
}

.toggle-knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--color-white);
  transition: transform 0.2s;
}

.toggle-switch.on .toggle-knob {
  transform: translateX(20px);
}

.pip-btn {
  display: flex;
  align-items: center;
  width: 100%;
  padding: 8px 0;
  border: none;
  background: none;
  color: rgba(255,255,255,0.85);
  font-size: 14px;
  cursor: pointer;
  text-align: left;
  transition: color 0.15s;
}

.pip-btn:hover {
  color: #BB86FC;
}

.pip-window {
  position: absolute;
  bottom: 12px;
  right: 12px;
  width: 180px;
  border-radius: 10px;
  overflow: hidden;
  box-shadow: 0 4px 24px rgba(0,0,0,0.6);
  z-index: 40;
  cursor: pointer;
  background: var(--color-black);
}

.pip-video {
  position: relative;
  width: 100%;
  aspect-ratio: 16/9;
  background: #121212;
}

.pip-placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255,255,255,0.2);
}

.pip-controls {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: rgba(0,0,0,0.3);
  opacity: 0;
  transition: opacity 0.15s;
}

.pip-window:hover .pip-controls {
  opacity: 1;
}

.pip-ctrl-btn {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: none;
  background: rgba(255,255,255,0.2);
  color: var(--color-white);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(4px);
  transition: background 0.15s;
}

.pip-ctrl-btn:hover {
  background: rgba(255,255,255,0.35);
}

.pip-close-ctrl {
  width: 28px;
  height: 28px;
  position: absolute;
  top: 6px;
  right: 6px;
  background: rgba(0,0,0,0.5);
}

.pip-close-ctrl:hover {
  background: rgba(255,82,82,0.6);
}

.volume-popover-wrap {
  position: relative;
}

.volume-popover {
  position: absolute;
  bottom: 44px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  background: rgba(30, 30, 30, 0.92);
  border-radius: 20px;
  padding: 12px 8px;
  box-shadow: 0 4px 16px rgba(0,0,0,0.4);
  backdrop-filter: blur(8px);
  z-index: 10;
}

.vol-vertical-track {
  width: 4px;
  height: 100px;
  background: rgba(255,255,255,0.2);
  border-radius: 2px;
  position: relative;
  cursor: pointer;
}

.vol-vertical-track:hover {
  width: 6px;
}

.vol-vertical-fill {
  position: absolute;
  bottom: 0;
  left: 0;
  width: 100%;
  background: #BB86FC;
  border-radius: 2px;
  pointer-events: none;
}

.vol-vertical-thumb {
  position: absolute;
  left: 50%;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #BB86FC;
  transform: translate(-50%, 50%);
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.15s;
}

.vol-vertical-track:hover .vol-vertical-thumb {
  opacity: 1;
}

.vol-pct {
  font-size: 10px;
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: rgba(255,255,255,0.7);
}

.vol-pop-enter-active,
.vol-pop-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.vol-pop-enter-from,
.vol-pop-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(4px);
}

.pop-enter-active,
.pop-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.pop-enter-from,
.pop-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

.icon-btn {
  width: 40px;
  height: 40px;
  border: none;
  background: none;
  color: var(--color-white);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  transition: background 0.15s;
}

.icon-btn:hover {
  background: rgba(255,255,255,0.1);
}

.icon-btn.sm {
  width: 36px;
  height: 36px;
  color: rgba(191,191,191,0.9);
}

.icon-btn.sm.active {
  color: #BB86FC;
}

.locked-bottom {
  margin-top: auto;
  background: linear-gradient(to top, rgba(0,0,0,0.5), transparent);
  padding-top: 16px;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.25s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.pip-fade-enter-active,
.pip-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.pip-fade-enter-from,
.pip-fade-leave-to {
  opacity: 0;
  transform: scale(0.85);
}

.prototype-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: rgba(255,255,255,0.04);
  border-top: 1px solid rgba(255,255,255,0.08);
  font-size: 11px;
  color: rgba(255,255,255,0.5);
  flex-wrap: wrap;
  flex-shrink: 0;
}

.prototype-toolbar label {
  font-weight: 600;
  color: rgba(255,255,255,0.7);
}

.prototype-toolbar select,
.prototype-toolbar input {
  background: rgba(255,255,255,0.06);
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 4px;
  padding: 2px 6px;
  color: rgba(255,255,255,0.8);
  font-size: 11px;
}

.file-input { width: 140px; }
.dur-input { width: 64px; }
</style>
