// taskServices.ts - 任务 store 的应用层依赖注入容器
//
// 任务 store（taskStore / runTasksStore）属于共享抽象层，但依赖两个应用层能力：
//   1. 后端 API：向量搜索 searchTasksVector、按 runId 分页拉取 getTasks
//   2. 本地持久化：IndexedDB（taskPersistence）
// 这些能力在 shared 内无法自行实现（依赖应用层 base URL / IndexedDB），
// 故通过"注册函数"在应用启动时注入，store 内部只消费接口，不感知实现。
//
// 应用层（encv-mobile）在 main.ts 启动期调用 setTaskServices(...) 注入真实实现；
// 未注册时返回安全空实现（不崩溃，仅行为为空），便于 shared 自身类型检查与测试。

import type { BatchTaskSpec, EncvTask, PredictPluginResponse, RunInfo, RunSummary, TaskType } from "@encv/shared-components/types/task";
import type { TriggeredBy } from "../lib/workflow/types";

/** 搜索模式（与后端 search_mode 对齐） */
export type SearchMode = "none" | "strict" | "combined" | "greedy";

/** 本地持久化接口（对应 encv-mobile 的 lib/taskPersistence） */
export interface TaskPersistence {
  loadAllTasks(): Promise<EncvTask[]>;
  ensureLRUCache(): Promise<void>;
  putTask(task: EncvTask): Promise<void>;
  bulkPutTasks(tasks: EncvTask[]): void;
  clearPutThrottle(id: string): void;
  deleteTask(id: string): Promise<void>;
}

/** 注入给共享 store 的应用层服务集合 */
export interface TaskServices {
  /** 后端向量搜索（来自应用层 api/encv_search） */
  searchTasksVector(query: string, limit?: number): Promise<{ results: EncvTask[]; search_mode: SearchMode }>;
  /** 按 runId 分页拉取 task（来自应用层 api/encv_tasks） */
  getTasks(params?: { runId?: string; offset?: number; limit?: number }): Promise<EncvTask[]>;
  /** 拉取所有 run 列表（带 summary，来自应用层 api/encv_tasks） */
  listRuns(): Promise<RunInfo[]>;
  /** 拉取单个 run 的 summary（来自应用层 api/encv_tasks） */
  getRunSummary(runId: string): Promise<RunSummary>;
  /** 批量创建 task（来自应用层 api/encv_tasks，后端生成 UUID） */
  batchCreateTasks(specs: BatchTaskSpec[], runId?: string, triggeredBy?: TriggeredBy): Promise<EncvTask[]>;
  /** 取消整个 run（来自应用层 api/encv_tasks） */
  cancelRun(runId: string): Promise<void>;
  /** 插件预测（来自应用层 api/encv_plugins，经 @/api/encv 再导出） */
  predictPlugin(sourcePath: string, type: TaskType): Promise<PredictPluginResponse>;
  /** 取消单个 task（来自应用层 api/encv_tasks） */
  cancelTask(id: string): Promise<void>;
  /** 重试单个 task（来自应用层 api/encv_tasks） */
  retryTask(id: string): Promise<void>;
  /** 删除单个 task（来自应用层 api/encv_tasks） */
  deleteTask(id: string): Promise<void>;
  /** 将 task 追加进 store（来自应用层 stores/taskStore 的 appendTask） */
  appendTask(task: EncvTask): void;
  /** IndexedDB 持久化（来自应用层 lib/taskPersistence） */
  persistence: TaskPersistence;
}

let _services: TaskServices | null = null;

/** 应用层在启动时调用，注入真实实现 */
export function setTaskServices(services: TaskServices): void {
  _services = services;
}

/** 读取已注入的服务；未注入时返回安全空实现（避免 import 期崩溃） */
export function getTaskServices(): TaskServices {
  if (!_services) {
    // eslint-disable-next-line no-console
    console.warn("[taskServices] 未注入 TaskServices，使用空实现（请在应用启动时调用 setTaskServices）");
    return NULL_TASK_SERVICES;
  }
  return _services;
}

const NULL_TASK_SERVICES: TaskServices = {
  async searchTasksVector() {
    return { results: [], search_mode: "none" };
  },
  async getTasks() {
    return [];
  },
  async listRuns() {
    return [];
  },
  async getRunSummary() {
    return { runId: "", total: 0, passed: 0, failed: 0, running: 0, pending: 0, cancelled: 0, percent: 0 };
  },
  async batchCreateTasks() {
    return [];
  },
  async cancelRun() {},
  async predictPlugin() {
    return { candidates: [], pluginName: null, taskOptions: null };
  },
  async cancelTask() {},
  async retryTask() {},
  async deleteTask() {},
  appendTask() {},
  persistence: {
    async loadAllTasks() {
      return [];
    },
    async ensureLRUCache() {},
    async putTask() {},
    bulkPutTasks() {},
    clearPutThrottle() {},
    async deleteTask() {},
  },
};
