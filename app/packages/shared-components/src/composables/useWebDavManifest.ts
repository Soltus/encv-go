/**
 * useWebDavManifest — 拉取并缓存 webdav manifest
 *
 * 🆕 2026-06-17：声明式重构（multi-mount-storage-refactor spec 续）
 *
 * 设计要点：
 *  - reactive state（manifest / loading / error）
 *  - 手动 refresh()
 *  - mount 选择：默认 automation（攻防测试默认 mount），可通过 setActiveMount 切换
 *
 * 与 useProxiedFetch 关系：
 *  - 此 composable 只在 Settings → DevTools → WebDavAutomationTestsDetail.vue 用
 *  - 走普通 fetch 即可（不走 SSE / proxy）
 */

import { fetchWebDavLocalInfo, fetchWebDavManifest } from "@encv/shared-components/api/encv";
import type { WebDavAuth, WebDavManifestResponse, WebDavMountManifest } from "@encv/shared-components/types/webdav-test";
import { type ComputedRef, computed, ref } from "vue";

const DEFAULT_MOUNT = "automation";

export function useWebDavManifest() {
  const manifest = ref<WebDavManifestResponse | null>(null);
  const loading = ref(false);
  const error = ref<Error | null>(null);
  const auth = ref<WebDavAuth>({});
  const webdavPath = ref("");
  const serverBaseUrl = ref("");
  const activeMountName = ref(DEFAULT_MOUNT);

  const availableMounts: ComputedRef<WebDavMountManifest[]> = computed(() => manifest.value?.mounts ?? []);

  const activeMount: ComputedRef<WebDavMountManifest | null> = computed(() => {
    const list = availableMounts.value;
    return list.find(m => m.name === activeMountName.value) ?? list.find(m => m.is_default) ?? list[0] ?? null;
  });

  const isReady: ComputedRef<boolean> = computed(() => manifest.value !== null && activeMount.value !== null);

  async function refresh() {
    if (loading.value) return;
    loading.value = true;
    error.value = null;
    try {
      const [manifestData, localInfo] = await Promise.all([
        fetchWebDavManifest(),
        // local-info 失败不阻塞（dev 环境可能没配置 webdav）
        fetchWebDavLocalInfo().catch(() => null),
      ]);
      manifest.value = manifestData;
      serverBaseUrl.value = manifestData.server_base;
      if (localInfo) {
        auth.value = { username: localInfo.username, password: localInfo.password };
        webdavPath.value = localInfo.webdavPath;
      }
    } catch (e) {
      error.value = e instanceof Error ? e : new Error(String(e));
    } finally {
      loading.value = false;
    }
  }

  function setActiveMount(mountName: string) {
    activeMountName.value = mountName;
  }

  return {
    manifest,
    loading,
    error,
    auth,
    webdavPath,
    serverBaseUrl,
    activeMount,
    activeMountName,
    availableMounts,
    isReady,
    refresh,
    setActiveMount,
  };
}
