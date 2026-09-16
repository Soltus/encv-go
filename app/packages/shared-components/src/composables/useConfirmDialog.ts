import { alertController } from "@ionic/vue";
import { useI18n } from "./useI18n";

/**
 * 确认弹窗统一封装（K8）。
 *
 * 收敛自散落各处的 `alertController.create({ buttons:[cancel, destructive] })` 确认模板
 * （useFilesView / useTasksView / GroupDetail 等 10+ 处）。返回 `Promise<boolean>`，
 * 调用方按布尔结果决定是否执行副作用，消除重复的「创建 alert + present + 两按钮 handler」样板。
 *
 * 另提供 `showAlert`（单 OK 信息/错误弹窗），收敛「`buttons:[t("common.ok")]` 的
 * 错误/成功提示」模板（B 类），与 `confirm`（A 类二选一决策）互补，避免把 32 处
 * alertController.create 一股脑塞进 confirm 而改变行为（错误抽象）。
 */
export interface ConfirmOptions {
  header?: string;
  subHeader?: string;
  /** 确认正文。部分场景（仅 header 的警告型确认）可省略，故可选。 */
  message?: string;
  confirmText?: string;
  cancelText?: string;
  /** 确认按钮用 destructive 角色（红色危险样式） */
  danger?: boolean;
}

export interface AlertOptions {
  header?: string;
  subHeader?: string;
  message?: string;
  /** 单 OK 按钮文案，默认 t("common.ok") */
  okText?: string;
}

export function useConfirmDialog() {
  const { t } = useI18n();

  function confirm(opts: ConfirmOptions): Promise<boolean> {
    return new Promise(resolve => {
      void alertController
        .create({
          header: opts.header,
          subHeader: opts.subHeader,
          message: opts.message,
          buttons: [
            {
              text: opts.cancelText ?? t("common.cancel"),
              role: "cancel",
              handler: () => resolve(false),
            },
            {
              text: opts.confirmText ?? t("common.confirm"),
              role: opts.danger ? "destructive" : "confirm",
              handler: () => resolve(true),
            },
          ],
        })
        .then(alert => alert.present());
    });
  }

  /** 单 OK 信息/错误弹窗（B 类）。返回 present 的 Promise，无决策语义。 */
  function showAlert(opts: AlertOptions): Promise<void> {
    return alertController
      .create({
        header: opts.header,
        subHeader: opts.subHeader,
        message: opts.message,
        buttons: [opts.okText ?? t("common.ok")],
      })
      .then(alert => alert.present());
  }

  return { confirm, showAlert };
}
