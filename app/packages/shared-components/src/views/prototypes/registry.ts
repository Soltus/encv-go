import type { Component } from "vue";

export interface PrototypeDefinition {
  id: string;
  name: string;
  route: string;
  composePath: string;
  description: string;
  icon: string;
  accentColor: string;
  component: () => Promise<{ default: Component }>;
  composeSource: () => Promise<string>;
  webSource: () => Promise<string>;
}

const prototypes: PrototypeDefinition[] = [
  {
    id: "mpv-video-player",
    name: "MPV Video Player",
    route: "com.encvgo.plugin.mpv.MpvPlayerActivity",
    composePath: "com.encvgo.plugin.mpv.MpvPlayerScreen",
    description: "视频播放器：控制叠加层、进度条、字幕/音轨选择、画中画",
    icon: "play-circle",
    accentColor: "rgba(139, 92, 246, 0.15)",
    component: () => import("./MpvPlayerPrototype.vue"),
    composeSource: () => import("./sources/mpv-video-player-compose.txt?raw").then(m => m.default),
    webSource: () => import("./MpvPlayerPrototype.vue?raw").then(m => m.default),
  },
  {
    id: "mpv-audio-player",
    name: "MPV Audio Player",
    route: "com.encvgo.plugin.mpv.MpvPlayerActivity",
    composePath: "com.encvgo.plugin.mpv.MpvAudioPlayerScreen",
    description: "音乐播放器：唱片旋转动画、曲目信息、播放控制",
    icon: "musical-notes",
    accentColor: "rgba(187, 134, 252, 0.15)",
    component: () => import("./MpvAudioOnlyPrototype.vue"),
    composeSource: () => import("./sources/mpv-audio-player-compose.txt?raw").then(m => m.default),
    webSource: () => import("./MpvAudioOnlyPrototype.vue?raw").then(m => m.default),
  },
];

export function getPrototype(id: string): PrototypeDefinition | undefined {
  return prototypes.find(p => p.id === id);
}

export function getAllPrototypes(): PrototypeDefinition[] {
  return prototypes;
}
