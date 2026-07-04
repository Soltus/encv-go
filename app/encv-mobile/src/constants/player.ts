export const PLAY_MODE = {
  ARTPLAYER: "artplayer",
  MPV_PLUGIN: "mpv-plugin",
  MPV_ACTIVITY: "mpv-activity",
  MPV_FRAGMENT: "mpv-fragment",
  MPV_COMPOSE: "mpv-compose",
  EXTERNAL: "external",
} as const;

export type PlayMode = (typeof PLAY_MODE)[keyof typeof PLAY_MODE];

export const VIDEO_DEFAULT: PlayMode = PLAY_MODE.ARTPLAYER;
export const AUDIO_DEFAULT: PlayMode = PLAY_MODE.MPV_PLUGIN;

export const MPV_SUB_MODES = [PLAY_MODE.MPV_ACTIVITY, PLAY_MODE.MPV_FRAGMENT, PLAY_MODE.MPV_COMPOSE] as const;

export function isMpvSubMode(mode: string): boolean {
  return mode.startsWith("mpv-");
}

export function resolveMpvMode(mode: string): string {
  if (mode === "mpv" || mode === "mpv-plugin") return PLAY_MODE.MPV_ACTIVITY;
  if (MPV_SUB_MODES.includes(mode as any)) return mode;
  return PLAY_MODE.MPV_ACTIVITY;
}
