/**
 * mountPath.ts — Mount 虚拟路径的单一真相源
 *
 * 2026-06-15 重构：把 "/d/" 前缀从各处散落的字符串字面量集中到一个函数
 *
 * 铁律：
 *   - 所有前端代码写 mount 虚拟路径时**必须**用 mountPath(name)
 *   - 严禁直接写 "/d/xxx" 字符串（IDE grep 也找不到，但这就是易错点）
 *   - 改前缀（如未来变 "/m/"）只改这一个文件
 *
 * 后端对应：internal/mount/bootstrap.go 的 "/d/" + NameAutomation
 * 保持双向一致：改这里**必须**同步改后端，反之亦然
 *
 * 防回归：__tests__/mount-path.test.ts 验证所有 mount 名都能正确构造路径
 */

export const MOUNT_VIRTUAL_ROOT = '/d' as const

/**
 * 给定 mount 名字，返回完整虚拟路径
 *   mountPath('automation') → '/d/automation'
 *   mountPath('primary')    → '/d/primary'
 *   mountPath('sandbox')    → '/d/sandbox'
 */
export function mountPath(name: string): string {
  return `${MOUNT_VIRTUAL_ROOT}/${name}`
}

/**
 * 给定完整虚拟路径，提取 mount 名字
 *   unmountPath('/d/automation/foo') → 'automation'
 *   unmountPath('/d')                → ''
 */
export function unmountPath(virtualPath: string): string {
  const trimmed = virtualPath.replace(MOUNT_VIRTUAL_ROOT + '/', '')
  const slash = trimmed.indexOf('/')
  return slash === -1 ? trimmed : trimmed.slice(0, slash)
}
