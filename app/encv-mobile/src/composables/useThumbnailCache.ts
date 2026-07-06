import { getExternalStreamUrl } from "@/api/encv";
import { nextTick, onUnmounted, ref } from "vue";

export const THUMB_CACHE_MAX = 500;

const thumbCache = new Map<string, string>();
let observer: IntersectionObserver | null = null;
const pendingQueue: string[] = [];
let queueTimer: ReturnType<typeof setTimeout> | null = null;
const MAX_CONCURRENT = 3;

type CacheUpdateListener = (paths: string[]) => void;
const cacheListeners: CacheUpdateListener[] = [];

function notifyCacheUpdate(paths: string[]) {
  if (paths.length === 0) return;
  const listeners = [...cacheListeners];
  listeners.forEach(cb => cb(paths));
}

export function getThumbCacheSize(): number {
  return thumbCache.size;
}

export function clearThumbCache() {
  thumbCache.clear();
}

function getCachedUrl(path: string): string | undefined {
  return thumbCache.get(path);
}

function processQueue() {
  if (queueTimer) return;
  queueTimer = setTimeout(() => {
    const batch = pendingQueue.splice(0, MAX_CONCURRENT);
    const newlyCached: string[] = [];
    for (const path of batch) {
      if (!thumbCache.has(path)) {
        const url = getExternalStreamUrl(path);
        if (thumbCache.size >= THUMB_CACHE_MAX) {
          const firstKey = thumbCache.keys().next().value;
          if (firstKey !== undefined) thumbCache.delete(firstKey);
        }
        thumbCache.set(path, url);
        newlyCached.push(path);
      }
    }
    queueTimer = null;
    notifyCacheUpdate(newlyCached);
    if (pendingQueue.length > 0) processQueue();
  }, 50);
}

function scheduleLoad(path: string) {
  if (thumbCache.has(path)) return;
  if (!pendingQueue.includes(path)) {
    pendingQueue.push(path);
    processQueue();
  }
}

function createObserver() {
  if (observer) return;
  observer = new IntersectionObserver(
    entries => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          const path = entry.target.getAttribute("data-file-path");
          if (path) scheduleLoad(path);
          observer!.unobserve(entry.target);
        }
      });
    },
    { rootMargin: "150px" }
  );
}

export function useThumbnailCache() {
  const thumbnailUrls = ref<Record<string, string>>({});

  function syncFromCache(paths: string[]) {
    const updated: Record<string, string> = {};
    for (const p of paths) {
      const cached = getCachedUrl(p);
      if (cached) updated[p] = cached;
    }
    if (Object.keys(updated).length > 0) {
      thumbnailUrls.value = { ...thumbnailUrls.value, ...updated };
    }
  }

  function handleCacheUpdate(paths: string[]) {
    if (paths.length === 0) return;
    syncFromCache(paths);
  }

  function setupLazyThumbnails() {
    nextTick(() => {
      createObserver();
      const targets = document.querySelectorAll(".lazy-thumb-target") as NodeListOf<Element>;
      if (!targets.length) return;
      const pathsToSync: string[] = [];
      targets.forEach((el: Element) => {
        const path = el.getAttribute("data-file-path");
        if (path) {
          pathsToSync.push(path);
          observer!.observe(el);
        }
      });
      syncFromCache(pathsToSync);
      cacheListeners.push(handleCacheUpdate);
    });
  }

  function onThumbError(path: string) {
    delete thumbnailUrls.value[path];
    thumbCache.delete(path);
  }

  onUnmounted(() => {
    const idx = cacheListeners.indexOf(handleCacheUpdate);
    if (idx !== -1) cacheListeners.splice(idx, 1);
    if (observer) {
      observer.disconnect();
      observer = null;
    }
    if (queueTimer) {
      clearTimeout(queueTimer);
      queueTimer = null;
    }
    pendingQueue.length = 0;
  });

  return { thumbnailUrls, setupLazyThumbnails, onThumbError };
}
