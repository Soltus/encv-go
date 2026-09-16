/**
 * useModal — 命令式 modal 的最小封装。
 *
 * 收敛「modalController.create + present + onDidDismiss」样板（全仓 14+ 处重复）。
 * 与 useDisclosure（声明式 isOpen 范式）一起，把 Overlay 的两种意图统一。
 *
 * 行为契约（与裸 modalController 等价）：
 *   - openModal(options) = create({...options}) → present() → await onDidDismiss()
 *     返回完整 `{ data, role }`，调用方自行按 data/role 处理（两种既有变体都兼容）。
 *   - dismiss(data?, role?) 透传关闭当前最上层 modal。
 *
 * ⚠️ componentProps 快照坑（参考 useNewTaskModal）：
 *   modalController.create 对 componentProps 做浅快照。若子组件需在打开后读取
 *   最新值，务必传入 reactive 对象（如 reactive(state)），而非普通字面量。
 *   本封装不修改 componentProps 透传，故该约定由调用方遵守。
 */
import { modalController } from "@ionic/vue";

// 直接取 modalController.create 入参的精确类型，避免与 vue 的 Component/ComponentRef 命名冲突
type IonicModalOptions = Parameters<typeof modalController.create>[0];

export interface ModalDismissResult<T = unknown> {
  data: T | null;
  role?: string;
}

export interface OpenModalOptions extends Omit<IonicModalOptions, "componentProps"> {
  component: IonicModalOptions["component"];
  componentProps?: Record<string, unknown>;
}

export function useModal() {
  async function openModal<T = unknown>(options: OpenModalOptions): Promise<ModalDismissResult<T>> {
    const { component, componentProps, ...rest } = options;
    const modal = await modalController.create({
      component,
      componentProps,
      ...rest,
    });
    await modal.present();
    const result = await modal.onDidDismiss<T>();
    return result as ModalDismissResult<T>;
  }

  function dismiss(data?: unknown, role?: string): Promise<boolean> {
    return modalController.dismiss(data, role);
  }

  return { openModal, dismiss };
}
