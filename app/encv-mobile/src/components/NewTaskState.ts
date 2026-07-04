import type { ContainerVersionInfo, PluginCandidate, TaskField, TaskOptions } from "@/api/encv";

export interface NewTaskState {
  taskType: string;
  sourcePath: string;
  targetPath: string;
  candidates: PluginCandidate[];
  predictedPlugin: string | null;
  taskOptions: TaskOptions | null;
  primaryOverride: string;
  secondaryPassword: string;
  version: number;
  versionOptions: ContainerVersionInfo[];
  extraValues: Record<string, string>;
  filteredExtraFields: TaskField[];
  selectedPluginIndex: number;
  /** ECv4 CipherMode: 0 = AES-128-CTR（默认）, 1 = AES-256-CTR */
  cipherMode: number;
  /** ECv4 CompressionMode: 'none'（默认）| 'zstd' */
  compressionMode: "none" | "zstd";
}
