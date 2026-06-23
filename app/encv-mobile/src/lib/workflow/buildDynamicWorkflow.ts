/**
 * buildDynamicWorkflow 核心派生 pure 函数
 *
 * 2026-06-22 抽离：把 PluginTestsDetail.vue 内的 buildDynamicWorkflow 派生
 * 逻辑（cartesianExpand + case 生成 + wfDef 构造）抽到独立 pure 函数，
 * 让 e2e 测试能直接 import + 用真 plugin 数据派生 1000+ case。
 *
 * 抽离原因：
 * - PluginTestsDetail.vue 内部依赖 Vue ref / pinia store / API fetch，
 *   mount 起来太重（要 mock 10+ 依赖）
 * - 但 e2e 测试需要"真对齐 buildDynamicWorkflow"，不能用假数据
 * - 抽到 pure 函数：行为 100% 一致（来自同一份源码），e2e 测试能直接调
 *
 * 不破坏原 PluginTestsDetail.vue：保留它的 buildDynamicWorkflow 函数，
 * 但内部改成调 buildDynamicWorkflowPure()，0 行为变化。
 *
 * 派生量级（7 个 plugin + 真 extraFields）：
 *   video:   1 ext × 2 versions × 2 phase × 3×4×2 × 2 bool = 384
 *   audio:   1 ext × 2 versions × 2 phase × 2 × 1 = 8
 *   image:   1 ext × 2 versions × 2 phase = 4
 *   wps:     1 ext × 1 version × 2 phase = 2
 *   pdf:     1 ext × 2 versions × 2 phase = 4
 *   text:    1 ext × 2 versions × 2 phase = 4
 *   alistencrypt: 0 ext（跳过）
 *   总计 ≈ 406 step
 *   task 数 = step 数（1 step → 1 task）
 */
import { extToRelativePath } from '@/lib/mockDataGenerator'
import { formatContainerVersion } from '@/constants/containerVersion'
import type { PluginMeta } from '@/api/encv'
import type { StepDefinition, WorkflowDefinition } from '@/lib/workflow/types'

// ==================== 工具函数 ====================

/** ext → 目录分类（mp4→video / mp3→audio / png→image 等） */
function categoryForExt(ext: string): string {
  const e = ext.toLowerCase().replace(/^\./, '')
  if (['mp4', 'mkv', 'avi', 'mov', 'webm', 'flv', 'wmv'].includes(e)) return 'video'
  if (['mp3', 'flac', 'ogg', 'm4a', 'wav', 'aac', 'opus'].includes(e)) return 'audio'
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'tiff'].includes(e)) return 'image'
  if (['pdf'].includes(e)) return 'pdf'
  if (['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx'].includes(e)) return 'wps'
  if (['txt', 'md', 'rtf', 'log'].includes(e)) return 'text'
  if (['encv', 'ae'].includes(e)) return 'alist-encrypted'
  return 'misc'
}

/** 笛卡尔积展开：输入 [[1,2],[a,b,c]] → 输出 [[1,a],[1,b],[1,c],[2,a],[2,b],[2,c]] */
export function cartesianExpand(arrays: string[][]): string[][] {
  if (arrays.length === 0) return [[]]
  if (arrays.some((a) => a.length === 0)) return [[]]
  return arrays.reduce<string[][]>(
    (acc, curr) => acc.flatMap((a) => curr.map((c) => [...a, c])),
    [[]],
  )
}

/** 2^N bool 笛卡尔积（每个 bool 字段两种取值） */
export function boolCombinations(n: number): boolean[][] {
  if (n === 0) return [[]]
  const out: boolean[][] = []
  for (let mask = 0; mask < 1 << n; mask++) {
    out.push(Array.from({ length: n }, (_, i) => Boolean(mask & (1 << i))))
  }
  return out
}

// ==================== 派生接口 ====================

export interface DynamicTestCase {
  id: string
  phase: 'encrypt' | 'decrypt'
  pluginName: string
  taskType: 'encrypt' | 'decrypt'
  version: number
  sourcePath: string
  sourceExt: string
  targetPath: string
  safeId: string
  extraFields: Record<string, string>
}

export interface DynamicWorkflowResult {
  testCases: DynamicTestCase[]
  steps: StepDefinition[]
  wfDef: WorkflowDefinition
}

// ==================== 核心派生 ====================

/**
 * buildDynamicWorkflowPure — 抽离的 pure 函数版本
 *
 * 输入：plugin 列表 + mock 根路径
 * 输出：testCases + steps + wfDef
 *
 * 行为与 PluginTestsDetail.vue 的 buildDynamicWorkflow 完全一致：
 *   - 按 plugin.taskOptions.extraFields 派生 select/bool 笛卡尔积
 *   - encrypt / decrypt 各一组
 *   - encrypt-all → decrypt-all DAG（needs 依赖）
 *   - sourcePath 用 extToRelativePath → 真 mock 相对路径
 *   - safeId = plugin_version_ext_field1-val_field2-val
 *   - containerExt 从 plugin.containerExtension 拿（不硬编码）
 */
export function buildDynamicWorkflowPure(
  plugins: PluginMeta[],
  mockRoot: string,
  workflowId: string = 'dynamic-auto-test',
  workflowName: string = '自动化测试套件（动态）',
): DynamicWorkflowResult {
  const encryptSteps: StepDefinition[] = []
  const decryptSteps: StepDefinition[] = []
  const testCases: DynamicTestCase[] = []

  for (const plugin of plugins) {
    const opts = plugin.taskOptions
    if (!opts) continue

    const supportedExts = plugin.supportedExtensions ?? []
    if (supportedExts.length === 0) continue
    const sourceExt = supportedExts[0]
    const specRelPath = extToRelativePath(sourceExt)
    const sourcePath = specRelPath
      ? `${mockRoot}${specRelPath}`
      : `${mockRoot}01-plain-media/${categoryForExt(sourceExt)}/sample.${sourceExt}`

    const versions: number[] = opts.supportVersionSelect && opts.supportedVersions
      ? opts.supportedVersions
      : [opts.defaultVersion]

    // ===== 拆 select / bool field（按 condition 分 encrypt/decrypt）=====
    const allSelectFields: { field: any; values: string[] }[] = []
    const allBoolFields: { field: any }[] = []
    for (const f of opts.extraFields ?? []) {
      if (f.type === 'select' && Array.isArray(f.options) && f.options.length > 1) {
        allSelectFields.push({ field: f, values: f.options })
      } else if (f.type === 'bool') {
        allBoolFields.push({ field: f })
      }
    }

    for (const version of versions) {
      const encryptSelectFields = allSelectFields.filter(
        (sf) => !sf.field.condition || sf.field.condition === 'encrypt',
      )
      const encryptBoolFields = allBoolFields.filter(
        (bf) => !bf.field.condition || bf.field.condition === 'encrypt',
      )
      const decryptSelectFields = allSelectFields.filter(
        (sf) => !sf.field.condition || sf.field.condition === 'decrypt',
      )
      const decryptBoolFields = allBoolFields.filter(
        (bf) => !bf.field.condition || bf.field.condition === 'decrypt',
      )

      const encryptSelectCombos = cartesianExpand(encryptSelectFields.map((sf) => sf.values))
      const encryptBoolCombos = boolCombinations(encryptBoolFields.length)
      const decryptSelectCombos = cartesianExpand(decryptSelectFields.map((sf) => sf.values))
      const decryptBoolCombos = boolCombinations(decryptBoolFields.length)

      // safeId helper
      const makeSafeId = (extraFields: Record<string, string>): string => {
        const sortedKeys = Object.keys(extraFields).sort()
        const parts: string[] = [plugin.name, formatContainerVersion(version), sourceExt]
        for (const k of sortedKeys) {
          parts.push(`${k}-${extraFields[k]}`)
        }
        return parts.join('_').replace(/[^\w.-]/g, '_').replace(/_+/g, '_')
      }

      // ===== Encrypt 步骤 =====
      for (const selectCombo of encryptSelectCombos) {
        for (const boolCombo of encryptBoolCombos) {
          const extraFields: Record<string, string> = {}
          encryptSelectFields.forEach((sf, i) => {
            const val = selectCombo[i]
            if (val !== undefined) extraFields[sf.field.key] = val
          })
          encryptBoolFields.forEach((bf, i) => {
            extraFields[bf.field.key] = boolCombo[i] ? 'true' : 'false'
          })

          const safeId = makeSafeId(extraFields)
          const targetPath = `${mockRoot}02-test-output/${safeId}/`
          const stepId = `enc_${safeId}`

          const nameParts: string[] = [plugin.name, 'ENCRYPT', formatContainerVersion(version), sourceExt]
          for (const sf of encryptSelectFields) {
            const v = extraFields[sf.field.key]
            if (v) {
              const label = sf.field.optionLabels?.[v] ?? v
              nameParts.push(`${sf.field.key}=${label}`)
            }
          }
          for (const bf of encryptBoolFields) {
            const v = extraFields[bf.field.key]
            if (v) nameParts.push(`${bf.field.key}=${v}`)
          }

          encryptSteps.push({
            id: stepId,
            name: nameParts.join(' · '),
            action: {
              type: 'encv_task',
              taskType: 'encrypt',
              pluginName: plugin.name,
              params: {
                sourcePath,
                targetPath,
                password: 'automation-test-pwd',
                version,
                extraFields: Object.keys(extraFields).length > 0 ? extraFields : undefined,
              },
            },
          })

          testCases.push({
            id: stepId,
            phase: 'encrypt',
            pluginName: plugin.name,
            taskType: 'encrypt',
            version,
            sourcePath,
            sourceExt,
            targetPath,
            safeId,
            extraFields: { ...extraFields },
          })
        }
      }

      // ===== Decrypt 步骤 =====
      if (!plugin.containerExtension) {
        throw new Error(`Plugin ${plugin.name} 缺少 containerExtension（后端 plugin.GetContainerExtension() 返回空）`)
      }
      const containerExt = plugin.containerExtension
      const sourceBasename = sourcePath.split('/').pop() ?? `sample.${sourceExt}`
      const encryptedFileName = `${sourceBasename}.${containerExt}`

      for (const selectCombo of decryptSelectCombos) {
        for (const boolCombo of decryptBoolCombos) {
          const extraFields: Record<string, string> = {}
          decryptSelectFields.forEach((sf, i) => {
            const val = selectCombo[i]
            if (val !== undefined) extraFields[sf.field.key] = val
          })
          decryptBoolFields.forEach((bf, i) => {
            extraFields[bf.field.key] = boolCombo[i] ? 'true' : 'false'
          })

          const safeId = makeSafeId(extraFields)
          const sourcePathForDecrypt = `${mockRoot}02-test-output/${safeId}/${encryptedFileName}`

          const stepId = `dec_${safeId.replace(/^dec_/, '')}`
          const nameParts: string[] = [plugin.name, 'DECRYPT', formatContainerVersion(version), sourceExt]
          for (const sf of decryptSelectFields) {
            const v = extraFields[sf.field.key]
            if (v) {
              const label = sf.field.optionLabels?.[v] ?? v
              nameParts.push(`${sf.field.key}=${label}`)
            }
          }
          for (const bf of decryptBoolFields) {
            const v = extraFields[bf.field.key]
            if (v) nameParts.push(`${bf.field.key}=${v}`)
          }

          decryptSteps.push({
            id: stepId,
            name: nameParts.join(' · '),
            action: {
              type: 'encv_task',
              taskType: 'decrypt',
              pluginName: plugin.name,
              params: {
                sourcePath: sourcePathForDecrypt,
                targetPath: `${mockRoot}02-test-output/${safeId}/`,
                password: 'automation-test-pwd',
                version,
                extraFields: Object.keys(extraFields).length > 0 ? extraFields : undefined,
              },
            },
          })

          testCases.push({
            id: stepId,
            phase: 'decrypt',
            pluginName: plugin.name,
            taskType: 'decrypt',
            version,
            sourcePath: sourcePathForDecrypt,
            sourceExt,
            targetPath: `${mockRoot}02-test-output/${safeId}/`,
            safeId,
            extraFields: { ...extraFields },
          })
        }
      }
    }
  }

  // ===== 构建 WorkflowDefinition（DAG 拆 2 个 job）=====
  const wfDef: WorkflowDefinition = {
    id: workflowId,
    name: workflowName,
    description: `${plugins.length} 插件 × 源扩展名 × 版本 × 加密选项笛卡尔积
（encrypt-all 全部并行 → decrypt-all 等 encrypt 完成后并行）`,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    trigger: 'manual',
    env: { PASSWORD: 'automation-test-pwd' },
    jobs: [
      {
        id: 'encrypt-all',
        name: '🔒 Encrypt All (parallel)',
        strategy: { type: 'parallel', max: 5 },
        steps: encryptSteps,
      },
      {
        id: 'decrypt-all',
        name: '🔓 Decrypt All (parallel, after encrypt-all)',
        needs: ['encrypt-all'],
        strategy: { type: 'parallel', max: 5 },
        steps: decryptSteps,
      },
    ],
  }

  return { testCases, steps: [...encryptSteps, ...decryptSteps], wfDef }
}
