/**
 * 测试用例生成 composable
 *
 * 从 useAutomationTests 抽取的纯逻辑模块，负责按 plugin 元数据动态派生测试用例。
 *
 * 核心设计（对齐 automation-workflow 规则 §三）：
 * - 零硬编码 cipherMode / compressionMode —— 从 plugin.taskOptions.extraFields 派生笛卡尔积
 * - 按 plugin.supportedExtensions[0] 选源（避免笛卡尔积爆炸）
 * - ext → category → sample 文件名 三级映射
 *
 * 与 useAutomationTests.generateTestCases 的差异：
 * - 旧实现硬编码 v4 encrypt 的 cipher=[0,1] × compression=['none','zstd']
 * - 新实现遍历 extraFields，凡是 type='select' 且 options.length>1 的字段都参与笛卡尔积
 * - bool 字段不参与（避免 2^N 爆炸，由调用方按需展开）
 * - taskType / versions 由调用方传入，composable 不再自行从 taskOptions 派生
 */

import type { Ref } from "vue";
import type { PluginMeta, TaskType } from "@encv/shared-components/api/encv";
import { normalizeExt } from "@encv/shared-components/lib/string";

/** 生成的测试用例 */
export interface GeneratedTestCase {
  id: string;
  taskType: TaskType;
  pluginName: string;
  sourcePath: string;
  version: number;
  /** 笛卡尔积展开后的 extraFields 键值对（仅 select 字段） */
  extraFields: Record<string, string>;
  expectedBehavior: "success" | "might-fail";
}

export interface UseTestCaseGenerationOptions {
  /** mock 根目录（带尾斜杠），例如 '/d/automation/' */
  mockRoot: Ref<string>;
  /** 已加载的 plugin 列表 */
  plugins: Ref<PluginMeta[]>;
}

/**
 * 测试用例生成 composable
 *
 * 用法：
 * ```ts
 * const { generateCases, selectSourcePath } = useTestCaseGeneration({
 *   mockRoot: ref('/d/automation/'),
 *   plugins: pluginsRef,
 * })
 * const cases = generateCases('encrypt', [2, 3, 4])
 * ```
 */
export function useTestCaseGeneration(options: UseTestCaseGenerationOptions) {
  const { mockRoot, plugins } = options;

  // ext -> category 映射（对齐 automation-workflow 规则 §三）
  const extToCategory: Record<string, string> = {
    mp4: "video",
    mkv: "video",
    avi: "video",
    mov: "video",
    webm: "video",
    flv: "video",
    wmv: "video",
    mp3: "audio",
    flac: "audio",
    ogg: "audio",
    m4a: "audio",
    wav: "audio",
    aac: "audio",
    opus: "audio",
    png: "image",
    jpg: "image",
    jpeg: "image",
    gif: "image",
    webp: "image",
    bmp: "image",
    tiff: "image",
    pdf: "pdf",
    doc: "wps",
    docx: "wps",
    xls: "wps",
    xlsx: "wps",
    ppt: "wps",
    pptx: "wps",
    txt: "text",
    md: "text",
    rtf: "text",
    log: "text",
    encv: "alist-encrypted",
    ae: "alist-encrypted",
  };

  /** ext → category（容错：支持带点前缀如 '.mp4'，未知 ext 返回 'misc'） */
  function categoryForExt(ext: string): string {
    const e = normalizeExt(ext);
    return extToCategory[e] ?? "misc";
  }

  /** category → sample 文件名（每个 category 固定一个 sample，避免笛卡尔积爆炸） */
  function sampleFileForCategory(category: string): string {
    switch (category) {
      case "video":
        return "sample.mp4";
      case "audio":
        return "sample.mp3";
      case "image":
        return "sample.png";
      case "pdf":
        return "sample.pdf";
      case "wps":
        return "sample.docx";
      case "text":
        return "sample.txt";
      case "alist-encrypted":
        return "sample.encv";
      default:
        return "sample.bin";
    }
  }

  /**
   * 按 plugin.supportedExtensions[0] 选源（避免笛卡尔积爆炸）
   *
   * 路径模式：${mockRoot}01-plain-media/${category}/${sample}
   * 例如：/d/automation/01-plain-media/video/sample.mp4
   */
  function selectSourcePath(plugin: PluginMeta): string {
    const ext = plugin.supportedExtensions?.[0] ?? "bin";
    const category = categoryForExt(ext);
    const sample = sampleFileForCategory(category);
    return `${mockRoot.value}01-plain-media/${category}/${sample}`;
  }

  /**
   * 按 plugin.taskOptions.extraFields 派生笛卡尔积（消除硬编码 cipherMode/compressionMode）
   *
   * 规则：
   * - 仅 type='select' 且 options.length>1 的字段参与笛卡尔积
   * - bool / string / password 字段被忽略（bool 由调用方按需 2^N 展开）
   * - 无 extraFields 或无可展开字段时返回 [{}]（即一个空组合）
   *
   * 注意：不处理 condition 过滤（encrypt/decrypt 专属字段）—— 调用方如需按 taskType
   * 过滤，应在调用前预处理 plugin.taskOptions.extraFields。
   */
  function deriveExtraFieldCombinations(plugin: PluginMeta): Record<string, string>[] {
    const opts = plugin.taskOptions;
    if (!opts?.extraFields || !Array.isArray(opts.extraFields)) return [{}];

    const selectFields: { fieldName: string; values: string[] }[] = [];
    for (const f of opts.extraFields) {
      if (f.type === "select" && Array.isArray(f.options) && f.options.length > 1) {
        // TaskField.key 是字段标识符（如 'cipherMode' / 'compressionMode'）
        selectFields.push({ fieldName: f.key, values: f.options });
      }
    }

    if (selectFields.length === 0) return [{}];

    // 笛卡尔积展开：[{}] × [v1,v2] × [v1,v2] → [{f1:v1,f2:v1}, {f1:v1,f2:v2}, ...]
    const results: Record<string, string>[] = [{}];
    for (const { fieldName, values } of selectFields) {
      const newResults: Record<string, string>[] = [];
      for (const r of results) {
        for (const v of values) {
          newResults.push({ ...r, [fieldName]: v });
        }
      }
      results.length = 0;
      results.push(...newResults);
    }
    return results;
  }

  /**
   * 生成测试用例
   *
   * @param taskType 'encrypt' | 'decrypt'
   * @param versions 要测试的版本列表（如 [2, 3, 4]），由调用方决定
   * @returns GeneratedTestCase[]
   */
  function generateCases(taskType: TaskType, versions: number[]): GeneratedTestCase[] {
    const cases: GeneratedTestCase[] = [];
    for (const plugin of plugins.value) {
      const sourcePath = selectSourcePath(plugin);
      const combinations = deriveExtraFieldCombinations(plugin);
      for (const version of versions) {
        for (const combo of combinations) {
          const comboPart = Object.entries(combo)
            .map(([k, v]) => `${k}=${v}`)
            .join("-");
          cases.push({
            id: `${plugin.name}-${taskType}-v${version}-${comboPart || "default"}`,
            taskType,
            pluginName: plugin.name,
            sourcePath,
            version,
            extraFields: combo,
            expectedBehavior: "might-fail",
          });
        }
      }
    }
    return cases;
  }

  return {
    generateCases,
    selectSourcePath,
    deriveExtraFieldCombinations,
    categoryForExt,
    sampleFileForCategory,
  };
}
