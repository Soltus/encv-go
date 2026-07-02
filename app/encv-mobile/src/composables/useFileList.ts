import { ref, computed } from 'vue'
import type { FileItem } from '@/api/encv'
import { getFileCategory } from '@/api/encv'
import {
  folder,
  videocam,
  musicalNotes,
  image,
  document as documentIcon,
  documentText,
  lockClosed,
} from 'ionicons/icons'

export const IMAGE_EXTENSIONS = new Set([
  '.jpg', '.jpeg', '.png', '.gif', '.webp',
  '.bmp', '.svg', '.heic', '.heif', '.avif',
])

export type SortBy = 'name' | 'size' | 'time' | 'relevance'

export interface SortState {
  by: SortBy
  desc: boolean
}

export const SORT_CYCLE: readonly SortState[] = [
  { by: 'name', desc: false },
  { by: 'name', desc: true },
  { by: 'size', desc: false },
  { by: 'size', desc: true },
  { by: 'time', desc: false },
  { by: 'time', desc: true },
]

const LABEL_MAP: Record<SortBy, string> = {
  name: '名字',
  size: '大小',
  time: '时间',
  relevance: '相关度',
}

export function isImageFile(file: FileItem): boolean {
  if (file.isDirectory) return false
  const ext = '.' + file.name.split('.').pop()?.toLowerCase()
  return IMAGE_EXTENSIONS.has(ext || '')
}

export function getFileIcon(file: FileItem) {
  if (file.isDirectory) return folder
  if (file.isEncrypted) return lockClosed
  const category = getFileCategory(file.name)
  switch (category) {
    case 'video': return videocam
    case 'audio': return musicalNotes
    case 'image': return image
    case 'document': return documentIcon
    default: return documentText
  }
}

export function getFileIconColor(file: FileItem): string {
  if (file.isDirectory) return 'primary'
  if (file.isEncrypted) return 'warning'
  const category = getFileCategory(file.name)
  switch (category) {
    case 'video': return 'danger'
    case 'audio': return 'tertiary'
    case 'image': return 'success'
    default: return 'medium'
  }
}

export function getSortLabel(sortBy: SortBy, desc: boolean): string {
  return `${LABEL_MAP[sortBy]}${desc ? '↓' : '↑'}`
}

export function cycleSortState(current: SortState): SortState {
  const idx = SORT_CYCLE.findIndex(s => s.by === current.by && s.desc === current.desc)
  return SORT_CYCLE[(idx + 1) % SORT_CYCLE.length]
}

export function sortFiles(files: FileItem[], sortBy: SortBy, desc: boolean): FileItem[] {
  const list = [...files]
  list.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1
    if (!a.isDirectory && b.isDirectory) return 1
    let cmp = 0
    switch (sortBy) {
      case 'name':
        cmp = a.name.localeCompare(b.name)
        break
      case 'size':
        cmp = (a.size || 0) - (b.size || 0)
        break
      case 'time':
        cmp = (Number(a.modified) || 0) - (Number(b.modified) || 0)
        break
      case 'relevance':
        // 混合相关度：score 越大越相关（降序）。无 score 的项（未走向量搜索）
        // 排到末尾，避免干扰相关度排序。
        cmp = (a.score ?? -1) - (b.score ?? -1)
        break
    }
    return desc ? -cmp : cmp
  })
  return list
}

export function useFileListSort() {
  const sortBy = ref<SortBy>('name')
  const sortDesc = ref(false)

  const sortLabel = computed(() => getSortLabel(sortBy.value, sortDesc.value))

  function cycleSort() {
    const next = cycleSortState({ by: sortBy.value, desc: sortDesc.value })
    sortBy.value = next.by
    sortDesc.value = next.desc
  }

  return { sortBy, sortDesc, sortLabel, cycleSort }
}

export const VIRTUAL_SCROLL_CONFIG = {
  THRESHOLD: 200,
  ESTIMATE_SIZE: 72,
  OVERSCAN: 5,
} as const
