import { ref, type Ref } from "vue";
import { getMotionDisabled, setMotionDisabled } from "../motion/guard";

export interface MotionPreference {
  /** 当前动效总闸：true=强制关闭，false=强制开启，null=跟随系统 reduced-motion。 */
  disabled: Ref<boolean | null>;
  /** 用户是否勾选了「强制关闭动效」（绑定 UI 开关）。 */
  isForcedOff: Ref<boolean>;
  /** 开关处理：on=强制关闭动效，off=改回跟随系统 reduced-motion。 */
  setForcedOff: (off: boolean) => void;
}

/**
 * 「减少动效」用户偏好（Appearance 设置项）。
 *
 * 底层走 motion/guard 的全局总闸 setMotionDisabled + localStorage 持久化，
 * 这里只提供响应式绑定与「开关语义」映射：
 *   - 勾选 => setMotionDisabled(true)  强制关闭所有动效；
 *   - 取消 => setMotionDisabled(null)  恢复跟随系统 prefers-reduced-motion。
 * 不直接暴露 force-on（false）以避免覆盖系统无障碍偏好。
 */
export function useMotionPreference(): MotionPreference {
  const disabled = ref<boolean | null>(getMotionDisabled());
  const isForcedOff = ref(disabled.value === true);

  function setForcedOff(off: boolean): void {
    isForcedOff.value = off;
    disabled.value = off ? true : null;
    setMotionDisabled(off ? true : null);
  }

  return { disabled, isForcedOff, setForcedOff };
}
