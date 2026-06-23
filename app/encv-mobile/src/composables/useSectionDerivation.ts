import { computed, type ComputedRef } from 'vue'
import type { EncvTask } from '@/api/encv'

/**
 * Section 维度枚举
 * - 'plugin': 按 task.pluginName 分桶
 * - 'type': 按 task.type 分桶
 * - 'category': 按 task.sourcePath 后缀派生 category
 * - 'none': 统一归到「全部」section（兜底，不丢失任务）
 */
export type SectionDimension = 'plugin' | 'type' | 'category' | 'none'

/**
 * Section 元数据（composable 派生结果）
 * - dimension: 维度
 * - key: 桶 key（用于 Map 索引 / sectionKeyToString）
 * - label: 显示名
 */
export interface SectionMeta {
  dimension: SectionDimension
  key: string
  label: string
}

// ext → category 映射（与 PluginTestsDetail.vue categoryForExt / automation-workflow 规则 §三 保持一致）
const EXT_TO_CATEGORY: Record<string, string> = {
  mp4: 'video', mkv: 'video', avi: 'video', mov: 'video', webm: 'video', flv: 'video', wmv: 'video',
  mp3: 'audio', flac: 'audio', ogg: 'audio', m4a: 'audio', wav: 'audio', aac: 'audio', opus: 'audio',
  png: 'image', jpg: 'image', jpeg: 'image', gif: 'image', webp: 'image', bmp: 'image', tiff: 'image',
  pdf: 'pdf',
  doc: 'wps', docx: 'wps', xls: 'wps', xlsx: 'wps', ppt: 'wps', pptx: 'wps',
  txt: 'text', md: 'text', rtf: 'text', log: 'text',
  encv: 'alist-encrypted', ae: 'alist-encrypted',
}

const CATEGORY_LABELS: Record<string, string> = {
  video: '视频',
  audio: '音频',
  image: '图片',
  pdf: 'PDF',
  wps: '文档',
  text: '文本',
  'alist-encrypted': '加密文件',
  misc: '其他',
}

/**
 * ext → category 映射（公开，方便单测 / 其他模块复用）
 */
export function categoryForExt(ext: string): string {
  const e = ext.toLowerCase().replace(/^\./, '')
  return EXT_TO_CATEGORY[e] ?? 'misc'
}

/**
 * category → 中文 label 映射（公开，方便单测 / 其他模块复用）
 */
export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category
}

/**
 * 从 task 派生 section 元数据（核心纯函数）
 *
 * 按 dimension 选择派生规则：
 *   - 'plugin': task.pluginName（缺失 fallback 'unknown' / '未知插件'）
 *   - 'type': task.type（缺失 fallback 'unknown' / '未知类型'）
 *   - 'category': task.sourcePath 后缀 → categoryForExt → categoryLabel
 *   - 'none': 统一 'all' / '全部'（兜底）
 *
 * 升级指南：未来加新维度
 *   1. SectionDimension union 加一个值
 *   2. 这里 switch 加一个 case
 *   3. 调用方按需 pick 维度
 */
export function deriveSubSection(task: EncvTask, dimension: SectionDimension): SectionMeta {
  switch (dimension) {
    case 'plugin':
      return {
        dimension,
        key: task.pluginName ?? 'unknown',
        label: task.pluginName ?? '未知插件',
      }
    case 'type':
      return {
        dimension,
        key: task.type ?? 'unknown',
        label: task.type ?? '未知类型',
      }
    case 'category': {
      const ext = (task.sourcePath ?? '').split('.').pop()?.toLowerCase() ?? ''
      const category = categoryForExt(ext)
      return {
        dimension,
        key: category,
        label: categoryLabel(category),
      }
    }
    case 'none':
    default:
      return {
        dimension: 'none',
        key: 'all',
        label: '全部',
      }
  }
}

/**
 * useSectionDerivation composable
 *
 * 用法 1（静态维度）：
 *   const { derive } = useSectionDerivation('plugin')
 *   const meta = derive(task)  // 永远按 'plugin' 维度派生
 *
 * 用法 2（响应式维度）：
 *   const dim = ref<SectionDimension>('plugin')
 *   const { derive } = useSectionDerivation(computed(() => dim.value))
 *   const meta = derive(task)  // dim 变化时 derive 自动跟随
 *
 * 返回：
 *   - derive(task): 按当前 dimension 派生 SectionMeta
 *   - deriveSubSection: 透传纯函数（方便调用方临时切换维度）
 */
export function useSectionDerivation(dimension: ComputedRef<SectionDimension> | SectionDimension) {
  const dimRef: ComputedRef<SectionDimension> = typeof dimension === 'string'
    ? computed(() => dimension)
    : dimension

  function derive(task: EncvTask): SectionMeta {
    return deriveSubSection(task, dimRef.value)
  }

  return {
    derive,
    deriveSubSection,
  }
}
