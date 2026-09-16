/**
 * doPredict 时序契约：
 * 1. 设置 500ms 防抖 timer（immediate=true 可跳过防抖）
 * 2. timer 触发后调用 plugin.predict() API（predictPlugin）
 * 3. resolve(true) 表示有推荐插件
 *
 * 调用方必须 await 这个 Promise，否则 syncState() 拿到空数据。
 * 总延迟 = 500ms(防抖) + API耗时(~100-300ms) = 600-800ms
 */

import { computed, nextTick, ref, watch } from "vue";
import { usePathResolver } from "@encv/shared-components/composables/usePathResolver";
import { getTaskServices } from "@encv/shared-components/stores/taskServices";
import type { PluginCandidate, TaskField, TaskOptions } from "@encv/shared-components/types/task";

const { normalize } = usePathResolver();
const { predictPlugin } = getTaskServices();

export interface QueryInitParams {
  sourcePath: string;
  taskType: "encrypt" | "decrypt";
}

export function useTaskForm() {
  const candidates = ref<PluginCandidate[]>([]);
  const selectedPluginIndex = ref(0);
  const extraValues = ref<Record<string, string>>({});
  const primaryOverride = ref("");
  const secondaryPassword = ref("");

  const predictedPlugin = computed(() => {
    if (candidates.value.length === 0) return null;
    return candidates.value[selectedPluginIndex.value]?.name ?? null;
  });

  const taskOptions = computed<TaskOptions | null>(() => {
    if (candidates.value.length === 0) return null;
    return candidates.value[selectedPluginIndex.value]?.taskOptions ?? null;
  });

  watch(selectedPluginIndex, () => {
    const opts = taskOptions.value;
    const defaults: Record<string, string> = {};
    opts?.extraFields?.forEach(f => {
      if (f.defaultValue) defaults[f.key] = f.defaultValue;
    });
    extraValues.value = defaults;
  });

  let predictTimer: ReturnType<typeof setTimeout> | null = null;

  function doPredict(sourcePath: string, taskType: "encrypt" | "decrypt", options?: { immediate?: boolean }): Promise<void> {
    return new Promise(resolve => {
      if (predictTimer) clearTimeout(predictTimer);
      const delay = options?.immediate ? 0 : 500;
      predictTimer = setTimeout(async () => {
        const normalized = normalize(sourcePath);
        if (!normalized) {
          candidates.value = [];
          resolve();
          return;
        }

        try {
          const result = await predictPlugin(normalized, taskType);
          candidates.value = result.candidates ?? [];
          selectedPluginIndex.value = 0;
          const defaults: Record<string, string> = {};
          candidates.value[0]?.taskOptions?.extraFields?.forEach(f => {
            if (f.defaultValue) defaults[f.key] = f.defaultValue;
          });
          extraValues.value = defaults;
        } catch {
          candidates.value = [];
        }
        resolve();
      }, delay);
    });
  }

  function getExtraPayload(): Record<string, string> {
    const payload: Record<string, string> = {};
    for (const [k, v] of Object.entries(extraValues.value)) {
      if (v !== undefined && v !== "") payload[k] = v;
    }
    return payload;
  }

  const visibleExtraFields = computed<TaskField[]>(() => {
    if (!taskOptions.value?.extraFields) return [];
    return taskOptions.value.extraFields;
  });

  const versionOptions = computed(() => {
    if (!taskOptions.value?.supportedVersions) return undefined;
    return taskOptions.value.supportedVersions.map(v => ({
      version: v,
      status: v === taskOptions.value!.defaultVersion ? ("recommended" as const) : v === 2 ? ("deprecated" as const) : ("stable" as const),
      label: `V${v}`,
    }));
  });

  function reset() {
    if (predictTimer) {
      clearTimeout(predictTimer);
      predictTimer = null;
    }
    candidates.value = [];
    selectedPluginIndex.value = 0;
    extraValues.value = {};
    primaryOverride.value = "";
    secondaryPassword.value = "";
  }

  async function initFromQuery(params: QueryInitParams): Promise<void> {
    reset();
    await nextTick();
    doPredict(params.sourcePath, params.taskType);
  }

  return {
    candidates,
    selectedPluginIndex,
    predictedPlugin,
    taskOptions,
    extraValues,
    primaryOverride,
    secondaryPassword,
    visibleExtraFields,
    versionOptions,
    predictPlugin: doPredict,
    getExtraPayload,
    reset,
    initFromQuery,
  };
}
