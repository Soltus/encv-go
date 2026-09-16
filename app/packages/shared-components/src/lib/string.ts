/**
 * 字符串小工具单一真源（K22）。
 *
 * 收敛散落各处的字符串规整样板，避免同一意图在多模块各写一份。
 */

/**
 * 规整文件扩展名：转小写并去掉前导点。
 *
 * 收敛自三处逐字节相同的 `ext.toLowerCase().replace(/^\./, "")`：
 *   - `lib/mockDataGenerator.ts` 的 `extToRelativePath`
 *   - `composables/useTestCaseGeneration.ts` 的 `categoryForExt`
 *   - `composables/useSectionDerivation.ts` 的 `categoryForExt`
 *
 * @example normalizeExt(".MP4") // "mp4"
 * @example normalizeExt("PNG")  // "png"
 */
export function normalizeExt(ext: string): string {
  return ext.toLowerCase().replace(/^\./, "");
}
