import { ref, watch } from "vue";

export type RenderFps = 30 | 45 | 60 | 90 | 120;
export type RenderQuality = "720p" | "1080p" | "2k";

export const FPS_OPTIONS: RenderFps[] = [30, 45, 60, 90, 120];
export const QUALITY_OPTIONS: { value: RenderQuality; label: string }[] = [
  { value: "720p", label: "720P" },
  { value: "1080p", label: "1080P" },
  { value: "2k", label: "2K" },
];

// 等效渲染分辨率（长边像素），供 Phaser 画布按档位缩放
export const QUALITY_RESOLUTION: Record<RenderQuality, { width: number; height: number }> = {
  "720p": { width: 1280, height: 720 },
  "1080p": { width: 1920, height: 1080 },
  "2k": { width: 2560, height: 1440 },
};

const STORAGE_FPS = "simverse.render.fps";
const STORAGE_QUALITY = "simverse.render.quality";

function loadFps(): RenderFps {
  try {
    const v = Number(localStorage.getItem(STORAGE_FPS));
    return (FPS_OPTIONS as number[]).includes(v) ? (v as RenderFps) : 60;
  } catch {
    return 60;
  }
}

function loadQuality(): RenderQuality {
  try {
    const v = localStorage.getItem(STORAGE_QUALITY) as RenderQuality | null;
    return v === "720p" || v === "1080p" || v === "2k" ? v : "1080p";
  } catch {
    return "1080p";
  }
}

const fps = ref<RenderFps>(loadFps());
const quality = ref<RenderQuality>(loadQuality());

function notify() {
  if (typeof window !== "undefined") {
    window.dispatchEvent(
      new CustomEvent("simverse:render-settings", {
        detail: { fps: fps.value, quality: quality.value },
      }),
    );
  }
}

watch(fps, (v) => {
  try {
    localStorage.setItem(STORAGE_FPS, String(v));
  } catch {
    /* ignore */
  }
  notify();
});

watch(quality, (v) => {
  try {
    localStorage.setItem(STORAGE_QUALITY, v);
  } catch {
    /* ignore */
  }
  notify();
});

export function useWorldRenderSettings() {
  return { fps, quality, notify, FPS_OPTIONS, QUALITY_OPTIONS, QUALITY_RESOLUTION };
}
