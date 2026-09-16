// useFilesHelpers.ts - Files.vue 拆分的纯函数/常量 helpers
// 拆分自 Files.vue。包含跨 composable 共用的纯函数、类型守卫、常量。
// 不持有任何 reactive state。

import type { FileItem } from "@encv/shared-components/api/encv";
import { AUDIO_DEFAULT, PLAY_MODE, type PlayMode, VIDEO_DEFAULT } from "@encv/shared-components/constants/player";

// =============================================================================
// 播放模式：合法值 + 类型守卫 + 选择器
// =============================================================================

export const ALL_VALID_MODES: PlayMode[] = [
  PLAY_MODE.ARTPLAYER,
  PLAY_MODE.MPV_PLUGIN,
  PLAY_MODE.MPV_ACTIVITY,
  PLAY_MODE.MPV_FRAGMENT,
  PLAY_MODE.MPV_COMPOSE,
  PLAY_MODE.EXTERNAL,
];

export function isValidPlayMode(value: string): value is PlayMode {
  return ALL_VALID_MODES.includes(value as PlayMode);
}

export function getPlayMode(mediaType: "video" | "audio"): PlayMode {
  return mediaType === "video" ? VIDEO_DEFAULT : AUDIO_DEFAULT;
}

// =============================================================================
// Plugin filter presets（plugin view 筛选 chip 用）
// =============================================================================

export const SIZE_PRESETS = [
  { label: "< 1MB", max: 1024 * 1024 },
  { label: "1MB - 10MB", min: 1024 * 1024, max: 10 * 1024 * 1024 },
  { label: "10MB - 100MB", min: 10 * 1024 * 1024, max: 100 * 1024 * 1024 },
  { label: "> 100MB", min: 100 * 1024 * 1024 },
] as const;

export const TIME_PRESETS = [
  { label: "今天", days: 0 },
  { label: "近 3 天", days: 3 },
  { label: "近 7 天", days: 7 },
  { label: "近 30 天", days: 30 },
] as const;

// =============================================================================
// 日期 / 工具
// =============================================================================

/** 格式化 Date → YYYY-MM-DD（input[type=date] 用） */
export function formatDateInput(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// =============================================================================
// Multi-mount 伪 item 字段访问器
// =============================================================================

/**
 * 2026-06-15 multi-mount 适配：mount 伪 item 在根目录展示 driver badge + 真实 mount_path + resolved root_path。
 * 强类型访问 (file as any) 在 template 里太丑，集中到 helper 里。
 */
export function mountDriverOf(file: FileItem): string | null {
  const f = file as FileItem & { mount_driver?: string };
  return f.mount_driver ?? null;
}
export function mountPathOf(file: FileItem): string {
  const f = file as FileItem & { mount_path?: string };
  return f.mount_path ?? "";
}
export function mountRootOf(file: FileItem): string {
  const f = file as FileItem & { mount_root?: string };
  return f.mount_root ?? "";
}
