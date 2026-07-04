import { document as documentIcon, documentText, folder, image, lockClosed, musicalNotes, videocam } from "ionicons/icons";
import { computed, ref } from "vue";
import type { FileItem } from "@/api/encv";
import { getFileCategory } from "@/api/encv";

export const IMAGE_EXTENSIONS = new Set([".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".heic", ".heif", ".avif"]);

export type SortBy = "name" | "size" | "time" | "relevance";

export interface SortState {
  by: SortBy;
  desc: boolean;
}

export const SORT_CYCLE: readonly SortState[] = [
  { by: "name", desc: false },
  { by: "name", desc: true },
  { by: "size", desc: false },
  { by: "size", desc: true },
  { by: "time", desc: false },
  { by: "time", desc: true },
];

const LABEL_MAP: Record<SortBy, string> = {
  name: "名字",
  size: "大小",
  time: "时间",
  relevance: "相关度",
};

export function isImageFile(file: FileItem): boolean {
  if (!file || file.isDirectory) return false;
  if (!file.name) return false;
  const dotIdx = file.name.lastIndexOf(".");
  if (dotIdx === -1) return false;
  const ext = "." + file.name.substring(dotIdx + 1).toLowerCase();
  return IMAGE_EXTENSIONS.has(ext);
}

export function getFileIcon(file: FileItem) {
  if (!file) return documentText;
  if (file.isDirectory) return folder;
  if (file.isEncrypted) return lockClosed;
  const category = getFileCategory(file.name || "");
  switch (category) {
    case "video":
      return videocam;
    case "audio":
      return musicalNotes;
    case "image":
      return image;
    case "document":
      return documentIcon;
    default:
      return documentText;
  }
}

export function getFileIconColor(file: FileItem): string {
  if (!file) return "medium";
  if (file.isDirectory) return "primary";
  if (file.isEncrypted) return "warning";
  const category = getFileCategory(file.name || "");
  switch (category) {
    case "video":
      return "danger";
    case "audio":
      return "tertiary";
    case "image":
      return "success";
    default:
      return "medium";
  }
}

export function getSortLabel(sortBy: SortBy, desc: boolean): string {
  return `${LABEL_MAP[sortBy]}${desc ? "↓" : "↑"}`;
}

export function cycleSortState(current: SortState): SortState {
  const idx = SORT_CYCLE.findIndex(s => s.by === current.by && s.desc === current.desc);
  return SORT_CYCLE[(idx + 1) % SORT_CYCLE.length];
}

export function sortFiles(files: FileItem[], sortBy: SortBy, desc: boolean): FileItem[] {
  const list = [...files];
  list.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1;
    if (!a.isDirectory && b.isDirectory) return 1;
    let cmp = 0;
    switch (sortBy) {
      case "name":
        cmp = a.name.localeCompare(b.name);
        break;
      case "size":
        cmp = (a.size || 0) - (b.size || 0);
        break;
      case "time":
        cmp = (Number(a.modified) || 0) - (Number(b.modified) || 0);
        break;
      case "relevance":
        // 混合相关度：score 越大越相关（降序）。无 score 的项（未走向量搜索）
        // 排到末尾，避免干扰相关度排序。
        cmp = (a.score ?? -1) - (b.score ?? -1);
        break;
    }
    return desc ? -cmp : cmp;
  });
  return list;
}

export function useFileListSort() {
  const sortBy = ref<SortBy>("name");
  const sortDesc = ref(false);

  const sortLabel = computed(() => getSortLabel(sortBy.value, sortDesc.value));

  function cycleSort() {
    const next = cycleSortState({ by: sortBy.value, desc: sortDesc.value });
    sortBy.value = next.by;
    sortDesc.value = next.desc;
  }

  return { sortBy, sortDesc, sortLabel, cycleSort };
}

export const VIRTUAL_SCROLL_CONFIG = {
  THRESHOLD: 200,
  ESTIMATE_SIZE: 72,
  OVERSCAN: 5,
} as const;

// 🆕 2026-07-02 客户端搜索过滤（解决"先搜'在线'再搜'在线 视频'重新 loading 几秒"问题）
//
// 性能目标：在 200 个结果上客户端过滤 < 1ms。
// 配合后端 LRU 缓存（30s TTL）实现综合 10x 加速（详见 internal/server/search_cache.go）。
//
// 切词规则（与后端 expandCJKQueryForSearch + matchKeyword 行为保持一致）：
//   - 含空格/制表/换行 → 按空白切分（用户在 UI 已显式分词）
//   - 纯 CJK（中日韩连续无空格）→ 拆为单字 AND（"在线视频" → ["在","线","视","频"]）
//   - 纯英文/数字 → 整体作为 token
export function clientSearchTokenize(query: string): string[] {
  if (/[\s　]/.test(query)) {
    return query
      .split(/\s+/)
      .filter(Boolean)
      .map(t => t.toLowerCase());
  }
  // 检测 CJK 字符（汉/日/韩）
  if (/[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]/.test(query)) {
    return Array.from(query).map(c => c.toLowerCase());
  }
  // 纯英文/数字
  return query.trim() ? [query.toLowerCase()] : [];
}

// clientFilter 在已有结果上做客户端过滤（无网络、无后端）。
//
// 返回按"命中 token 数"降序排序的 FileItem[]（简化版相关度，与后端 hybrid score 近似）。
// 用于 performSearch 入口：客户端过滤命中则立即更新结果（无 loading）。
//
// 空 items 返回空数组；空 query 返回 items 本身（不过滤）。
export function clientFilterFiles(items: FileItem[], query: string): FileItem[] {
  if (!items || items.length === 0) return [];
  const tokens = clientSearchTokenize(query);
  if (tokens.length === 0) return items;

  const results: Array<{ item: FileItem; score: number }> = [];
  for (const item of items) {
    const nameLower = item.name.toLowerCase();
    let hitCount = 0;
    let allHit = true;
    for (const tok of tokens) {
      if (nameLower.includes(tok)) {
        hitCount++;
      } else {
        allHit = false;
        break;
      }
    }
    if (allHit) {
      // 把原 score 也透传（如果有），便于混合排序
      results.push({ item, score: hitCount + (item.score ?? 0) * 0.001 });
    }
  }
  // 按相关度降序
  results.sort((a, b) => b.score - a.score);
  return results.map(x => x.item);
}
