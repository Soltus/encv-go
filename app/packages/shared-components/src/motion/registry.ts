/**
 * 全局动效注册表（可选）。
 *
 * 用于给「命名动画」做性能采样，或运行时热禁用（如低端设备一键关动效、
 * 或某个视图的入场动画被发现引起回归时临时关闭）。
 */
export interface MotionEntry {
  name: string;
  enabled: boolean;
}

const registry = new Map<string, MotionEntry>();

export function registerMotion(name: string, enabled = true): MotionEntry {
  const entry: MotionEntry = { name, enabled };
  registry.set(name, entry);
  return entry;
}

export function setMotionEnabled(name: string, enabled: boolean): void {
  const e = registry.get(name);
  if (e) e.enabled = enabled;
}

export function isMotionEnabled(name: string): boolean {
  return registry.get(name)?.enabled ?? true;
}

export function listMotion(): MotionEntry[] {
  return [...registry.values()];
}
