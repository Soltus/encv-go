import { WebPlugin } from "@capacitor/core";

export interface GoProcessStatus {
  running: boolean;
  port: number;
  lastError?: string;
}

export interface GoProcessResult {
  success: boolean;
  port?: number;
  lastError?: string;
}

export interface PermissionResult {
  granted: boolean;
  requiresSettings?: boolean;
}

export interface PermissionCheckResult {
  notifications: boolean;
  storage: boolean;
  batteryOptimization: boolean;
}

export interface PluginFullState {
  id: string;
  status: "ready" | "not_installed" | "disabled" | "not_loaded" | "framework_not_ready" | "error" | "load_failed";
  name: string;
  version: string;
}

export interface PlayResult {
  success: boolean;
  error?: string;
  errorDetail?: string;
}

export interface GoProcessPlugin {
  restart(): Promise<GoProcessResult>;
  stop(): Promise<GoProcessResult>;
  getStatus(): Promise<GoProcessStatus>;
  requestNotificationPermission(): Promise<PermissionResult>;
  requestStoragePermission(): Promise<PermissionResult>;
  requestBatteryOptimization(): Promise<PermissionResult>;
  checkPermissions(): Promise<PermissionCheckResult>;
  isStandaloneMode(): Promise<{ standalone: boolean }>;
  getIntentFileInfo(): Promise<{ path: string; name: string; mimeType: string }>;
  openPlayer(options: { filePath: string; name: string; mimeType: string; mode?: string }): Promise<PlayResult>;
  closePlayer(): Promise<void>;
  openExternal(options: { url: string; mimeType: string }): Promise<void>;
  openInPlayer(options: { path: string; name: string; mimeType: string; mode?: string }): Promise<void>;
  openPlayerHome(): Promise<void>;
  setScreenOrientation(options: { orientation: string }): Promise<void>;
  installPlugin(options: { apkPath: string }): Promise<{ success: boolean; method?: string }>;
  pickAndInstallPlugin(): Promise<{ success: boolean; method?: string; fileName?: string }>;
  pickFolder(): Promise<{ path: string }>;
  checkInstalledPlugins(): Promise<Record<string, { installed: boolean; enabled: boolean; versionName: string }>>;
  getPluginFullState(options: { pluginId: string }): Promise<PluginFullState>;
  ensurePluginLoaded(options: { pluginId: string }): Promise<{ success: boolean }>;
  togglePluginEnabled(options: { pluginId: string; enabled: boolean }): Promise<{ success: boolean; pluginId: string; enabled: boolean }>;
  uninstallPlugin(options: { pluginId: string }): Promise<{ success: boolean; pluginId: string }>;
  debugLifecycleFlow(options?: { pluginId?: string }): Promise<Record<string, any>>;
  getLocalFilePath(options: { path: string }): Promise<{ path: string }>;
  startMpvInPlace(options: { filePath: string; name: string; mimeType?: string; containerId?: string }): Promise<PlayResult>;
  stopMpvInPlace(): Promise<{ success: boolean; embedded?: boolean }>;
  debugInstallFlow(): Promise<Record<string, any>>;
  debugKotlinReflect(): Promise<Record<string, any>>;
  debugApkValidation(): Promise<Record<string, any>>;
  debugValidationStrategy(): Promise<Record<string, any>>;
  exportLogs(): Promise<{ success: boolean; path?: string }>;
  clearLogs(): Promise<{ success: boolean }>;
  openLogViewer(): Promise<{ success: boolean }>;
  saveDevLogs(options: { logs: string }): Promise<{ success: boolean; path?: string }>;
  // 🆕 2026-06-17：读取 android-deps.json manifest (build 时由 Gradle task 生成)
  // web 端 mock 返回 null（无 Android assets）
  getAndroidDeps(): Promise<{ items: any[] } | null>;

  // 🆕 2026-07-03 spec android-workmanager-split-start-stop Phase 3.4
  // 任务取消持久化：把 cancel 意图入队 WorkManager，Go 进程重启后自动重试
  // web 端不实现，返回 success:false（浏览器没有 WorkManager）
  enqueueCancelWorker(options: { taskId: string }): Promise<{ success: boolean; workName?: string }>;

  openWorld(options: { worldId: string; worldName: string }): Promise<void>;
  startSimverseHeartbeat(): Promise<void>;
  stopSimverseHeartbeat(): Promise<void>;
  setSimverseWorldRunning(options: { running: boolean }): Promise<void>;
}

export class GoProcessWeb extends WebPlugin implements GoProcessPlugin {
  async restart(): Promise<GoProcessResult> {
    return { success: false };
  }

  async stop(): Promise<GoProcessResult> {
    return { success: false };
  }

  async getStatus(): Promise<GoProcessStatus> {
    return { running: false, port: 0 };
  }

  async requestNotificationPermission(): Promise<PermissionResult> {
    return { granted: true };
  }

  async requestStoragePermission(): Promise<PermissionResult> {
    return { granted: true };
  }

  async requestBatteryOptimization(): Promise<PermissionResult> {
    return { granted: true };
  }

  async checkPermissions(): Promise<PermissionCheckResult> {
    return { notifications: true, storage: true, batteryOptimization: true };
  }

  async isStandaloneMode(): Promise<{ standalone: boolean }> {
    return { standalone: false };
  }

  async getIntentFileInfo(): Promise<{ path: string; name: string; mimeType: string }> {
    return { path: "", name: "", mimeType: "" };
  }

  async openPlayer(_options: { filePath: string; name: string; mimeType: string; mode?: string }): Promise<PlayResult> {
    return { success: true };
  }

  async closePlayer(): Promise<void> {}

  async openExternal(_options: { url: string; mimeType: string }): Promise<void> {}

  async openInPlayer(_options: { path: string; name: string; mimeType: string; mode?: string }): Promise<void> {}

  async openPlayerHome(): Promise<void> {}

  async setScreenOrientation(_options: { orientation: string }): Promise<void> {}

  async installPlugin(_options: { apkPath: string }): Promise<{ success: boolean; method?: string }> {
    return { success: false };
  }

  async pickAndInstallPlugin(): Promise<{ success: boolean; method?: string; fileName?: string }> {
    return { success: false };
  }

  async pickFolder(): Promise<{ path: string }> {
    return { path: "" };
  }

  async checkInstalledPlugins(): Promise<Record<string, { installed: boolean; enabled: boolean; versionName: string }>> {
    return {};
  }

  async getPluginFullState(_options: { pluginId: string }): Promise<PluginFullState> {
    return { id: _options.pluginId, status: "not_installed", name: "", version: "" };
  }

  async ensurePluginLoaded(_options: { pluginId: string }): Promise<{ success: boolean }> {
    return { success: false };
  }

  async togglePluginEnabled(_options: {
    pluginId: string;
    enabled: boolean;
  }): Promise<{ success: boolean; pluginId: string; enabled: boolean }> {
    return { success: false, pluginId: "", enabled: false };
  }

  async uninstallPlugin(_options: { pluginId: string }): Promise<{ success: boolean; pluginId: string }> {
    return { success: false, pluginId: "" };
  }

  async debugLifecycleFlow(_options?: { pluginId?: string }): Promise<Record<string, any>> {
    return { debugLog: "web stub" };
  }

  async getLocalFilePath(_options: { path: string }): Promise<{ path: string }> {
    return { path: "" };
  }

  async startMpvInPlace(_options: { filePath: string; name: string; mimeType?: string; containerId?: string }): Promise<PlayResult> {
    return { success: false, error: "Native only" };
  }

  async stopMpvInPlace(): Promise<{ success: boolean; embedded?: boolean }> {
    return { success: false, embedded: false };
  }

  async debugInstallFlow(): Promise<Record<string, any>> {
    return { debugLog: "web stub" };
  }

  async debugKotlinReflect(): Promise<Record<string, any>> {
    return { debugLog: "web stub" };
  }

  async debugApkValidation(): Promise<Record<string, any>> {
    return { debugLog: "web stub" };
  }

  async debugValidationStrategy(): Promise<Record<string, any>> {
    return { debugLog: "web stub" };
  }

  async exportLogs(): Promise<{ success: boolean; path?: string }> {
    return { success: false };
  }

  async clearLogs(): Promise<{ success: boolean }> {
    return { success: false };
  }

  async openLogViewer(): Promise<{ success: boolean }> {
    return { success: false };
  }

  async saveDevLogs(_options: { logs: string }): Promise<{ success: boolean; path?: string }> {
    return { success: false };
  }

  // 🆕 2026-06-17：web 模式 mock — 没有 Android assets，返回 null
  async getAndroidDeps(): Promise<{ items: any[] } | null> {
    return null;
  }

  // 🆕 2026-07-03：web 模式 mock — 浏览器没有 WorkManager
  async enqueueCancelWorker(_options: { taskId: string }): Promise<{ success: boolean; workName?: string }> {
    return { success: false };
  }

  async openWorld(_options: { worldId: string; worldName: string }): Promise<void> {
    console.warn("[GoProcessWeb] openWorld: not available in web mode");
  }
}
