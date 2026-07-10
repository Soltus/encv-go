// registerSharedTaskServices.ts - 在应用启动时把任务 store 所需的应用层能力注入共享抽象层
//
// 共享 store（@encv/shared-components/stores/*）不感知应用层实现，所有应用层依赖
// （向量搜索 searchTasksVector / 分页拉取 getTasks / IndexedDB 持久化）通过
// setTaskServices 注入。必须早于任何 useTaskStore() / useRunTasksStore() 调用。

import { setTaskServices } from "@encv/shared-components/stores/taskServices";
import { getTasks, searchTasksVector } from "@/api/encv";
import { bulkPutTasks, clearPutThrottle, deleteTask, ensureLRUCache, loadAllTasks, putTask } from "@/lib/taskPersistence";

let registered = false;

/** 应用启动时调用一次，注入任务 store 的应用层服务实现 */
export function registerSharedTaskServices(): void {
  if (registered) return;
  registered = true;
  setTaskServices({
    searchTasksVector,
    getTasks,
    persistence: {
      loadAllTasks,
      ensureLRUCache,
      putTask,
      bulkPutTasks,
      clearPutThrottle,
      deleteTask,
    },
  });
}
