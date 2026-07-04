import { ref } from "vue";
import { type ContainerExtensionsResponse, fetchContainerExtensions } from "@encv/shared-components/api/encv";

const data = ref<ContainerExtensionsResponse | null>(null);
const loading = ref(false);
const error = ref<string | null>(null);

let fetchPromise: Promise<ContainerExtensionsResponse> | null = null;

const UNAVAILABLE = "__unavailable__";

async function load(): Promise<ContainerExtensionsResponse> {
  if (data.value && !error.value) return data.value;
  if (fetchPromise) return fetchPromise;

  loading.value = true;
  error.value = null;
  fetchPromise = fetchContainerExtensions()
    .then(res => {
      data.value = res;
      return res;
    })
    .catch(e => {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    })
    .finally(() => {
      loading.value = false;
      fetchPromise = null;
    });

  return fetchPromise;
}

function invalidate() {
  data.value = null;
  error.value = null;
  fetchPromise = null;
}

function getConflictingPlugins(suffix: string): string[] {
  const normalized = suffix.startsWith(".") ? suffix : "." + suffix.toLowerCase();
  if (!normalized || normalized === ".") return [];
  if (!data.value) return [UNAVAILABLE];

  const conflict = data.value.conflicts.find(c => c.extension === normalized);
  if (conflict) return conflict.pluginNames;

  for (const [ext, plugin] of Object.entries(data.value.extensions)) {
    if (ext.toLowerCase() === normalized) {
      return [plugin];
    }
  }
  return [];
}

function isExtensionCheckAvailable(): boolean {
  return data.value !== null;
}

function getExtensions(): ContainerExtensionsResponse["extensions"] | null {
  return data.value?.extensions ?? null;
}

export function usePluginExtensions() {
  return {
    data,
    loading,
    error,
    load,
    invalidate,
    getConflictingPlugins,
    isExtensionCheckAvailable,
    getExtensions,
    UNAVAILABLE,
  };
}
