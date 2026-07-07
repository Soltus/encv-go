/**
 * MPV Native 桥接（plugin-mpv-player web 端）
 *
 * 调 window.MpvNative（由 plugin-mpv-player/MpvPluginJSInterface 注册），
 * 与 MPV 播放器通信。
 *
 * 开发模式（无 WebView）：window.MpvNative 不存在 → 返回安全默认值
 * 生产模式（嵌入 WebView）：window.MpvNative 真实存在
 */

declare global {
  interface Window {
    MpvNative?: {
      getVersion(): string;
      getStatus(): string;
      play(url: string): boolean;
      pause(): boolean;
      stop(): boolean;
      seek(seconds: number): boolean;
      openPlayer(url: string): boolean;
      getVolume(): number;
      setVolume(volume: number): boolean;
      getMute(): boolean;
      setMute(mute: boolean): boolean;
      getDuration(): number;
      getPosition(): number;
      setPosition(position: number): boolean;
      getPlaylist(): string;
      addToPlaylist(url: string): boolean;
      removeFromPlaylist(index: number): boolean;
      clearPlaylist(): boolean;
      playNext(): boolean;
      playPrev(): boolean;
      getProperty(name: string): string;
      setProperty(name: string, value: string): boolean;
    };
  }
}

function safe<T>(fallback: T, fn: () => T): T {
  try {
    return fn();
  } catch {
    return fallback;
  }
}

export interface MpvStatus {
  playing: boolean;
  paused: boolean;
  volume: number;
  mute: boolean;
  duration: number;
  position: number;
  title: string;
  artist: string;
  videoCodec: string;
  audioCodec: string;
  width: number;
  height: number;
  fps: number;
  lastError: string;
  lastUpdateTs: number;
}

export interface MpvPlaylistItem {
  index: number;
  title: string;
  url: string;
  current: boolean;
  playing: boolean;
}

export const MpvNative = {
  /** 获取 MPV 版本 */
  getVersion(): string {
    return safe("unknown", () => window.MpvNative?.getVersion() ?? "unknown");
  },

  /** 获取播放器状态 */
  getStatus(): MpvStatus {
    const empty: MpvStatus = {
      playing: false,
      paused: false,
      volume: 100,
      mute: false,
      duration: 0,
      position: 0,
      title: "",
      artist: "",
      videoCodec: "",
      audioCodec: "",
      width: 0,
      height: 0,
      fps: 0,
      lastError: "",
      lastUpdateTs: 0,
    };
    return safe(empty, () => {
      const json = window.MpvNative?.getStatus() ?? "{}";
      return JSON.parse(json) as MpvStatus;
    });
  },

  /** 播放指定 URL */
  play(url: string): boolean {
    return safe(false, () => window.MpvNative?.play(url) ?? false);
  },

  /** 暂停播放 */
  pause(): boolean {
    return safe(false, () => window.MpvNative?.pause() ?? false);
  },

  /** 停止播放 */
  stop(): boolean {
    return safe(false, () => window.MpvNative?.stop() ?? false);
  },

  /** 跳转到指定位置（秒） */
  seek(seconds: number): boolean {
    return safe(false, () => window.MpvNative?.seek(seconds) ?? false);
  },

  /** 打开播放器 */
  openPlayer(url: string): boolean {
    return safe(false, () => window.MpvNative?.openPlayer(url) ?? false);
  },

  /** 获取音量 */
  getVolume(): number {
    return safe(100, () => window.MpvNative?.getVolume() ?? 100);
  },

  /** 设置音量 (0-100) */
  setVolume(volume: number): boolean {
    return safe(false, () => window.MpvNative?.setVolume(volume) ?? false);
  },

  /** 获取静音状态 */
  getMute(): boolean {
    return safe(false, () => window.MpvNative?.getMute() ?? false);
  },

  /** 设置静音 */
  setMute(mute: boolean): boolean {
    return safe(false, () => window.MpvNative?.setMute(mute) ?? false);
  },

  /** 获取总时长（秒） */
  getDuration(): number {
    return safe(0, () => window.MpvNative?.getDuration() ?? 0);
  },

  /** 获取当前播放位置（秒） */
  getPosition(): number {
    return safe(0, () => window.MpvNative?.getPosition() ?? 0);
  },

  /** 设置播放位置（秒） */
  setPosition(position: number): boolean {
    return safe(false, () => window.MpvNative?.setPosition(position) ?? false);
  },

  /** 获取播放列表 */
  getPlaylist(): MpvPlaylistItem[] {
    return safe([], () => {
      const json = window.MpvNative?.getPlaylist() ?? "[]";
      return JSON.parse(json) as MpvPlaylistItem[];
    });
  },

  /** 添加到播放列表 */
  addToPlaylist(url: string): boolean {
    return safe(false, () => window.MpvNative?.addToPlaylist(url) ?? false);
  },

  /** 从播放列表移除 */
  removeFromPlaylist(index: number): boolean {
    return safe(false, () => window.MpvNative?.removeFromPlaylist(index) ?? false);
  },

  /** 清空播放列表 */
  clearPlaylist(): boolean {
    return safe(false, () => window.MpvNative?.clearPlaylist() ?? false);
  },

  /** 下一首 */
  playNext(): boolean {
    return safe(false, () => window.MpvNative?.playNext() ?? false);
  },

  /** 上一首 */
  playPrev(): boolean {
    return safe(false, () => window.MpvNative?.playPrev() ?? false);
  },

  /** 获取 MPV 属性 */
  getProperty(name: string): string {
    return safe("", () => window.MpvNative?.getProperty(name) ?? "");
  },

  /** 设置 MPV 属性 */
  setProperty(name: string, value: string): boolean {
    return safe(false, () => window.MpvNative?.setProperty(name, value) ?? false);
  },
};

/**
 * 简单日志缓冲器（web 端独立维护）
 */
export class LogBuffer {
  private logs: Array<{ level: string; message: string; timestamp: number }> = [];
  private listeners: Array<(logs: any[]) => void> = [];
  private maxLength = 500;

  add(level: string, message: string) {
    this.logs.push({ level, message, timestamp: Date.now() });
    if (this.logs.length > this.maxLength) {
      this.logs = this.logs.slice(-this.maxLength);
    }
    this.notify();
  }

  info(message: string) {
    this.add("info", message);
  }
  warn(message: string) {
    this.add("warn", message);
  }
  error(message: string) {
    this.add("error", message);
  }
  debug(message: string) {
    this.add("debug", message);
  }

  getAll(): any[] {
    return this.logs;
  }

  clear() {
    this.logs = [];
    this.notify();
  }

  subscribe(listener: (logs: any[]) => void): () => void {
    this.listeners.push(listener);
    listener(this.logs);
    return () => {
      this.listeners = this.listeners.filter(l => l !== listener);
    };
  }

  private notify() {
    for (const l of this.listeners) l(this.logs);
  }
}

export const logBuffer = new LogBuffer();
