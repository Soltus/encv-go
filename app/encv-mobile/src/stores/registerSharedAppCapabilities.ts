// registerSharedAppCapabilities.ts - 在应用启动时把 shared 通用模块所需的应用层能力
// 注入共享抽象层（@encv/shared-components/runtime/appCapabilities）。
// 必须早于任何使用这些能力的运行时调用（agent base URL / alist-encrypt 特征）。
import { alertController } from "@ionic/vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useNewTaskModal } from "@encv/shared-components/composables/useNewTaskModal";
import { getLocalFilePath, isNative, openExternal, openPlayer, requestStoragePermission } from "@/plugins/GoProcess";
import { setAppCapabilities } from "@encv/shared-components/runtime/appCapabilities";

export function registerSharedAppCapabilities(): void {
  const { t } = useI18n();

  setAppCapabilities({
    isNative,
    openPlayer,
    openExternal,
    getLocalFilePath,
    requestStoragePermission,
    openNewTask: (initialSourcePath, initialTaskType) => {
      const { openNewTask } = useNewTaskModal();
      openNewTask(initialSourcePath, initialTaskType);
    },
    alertPassword: (fileDisplayName: string) =>
      new Promise<string | null>(resolve => {
        alertController
          .create({
            header: t("alistEncrypt.encryptedFile"),
            message: `${fileDisplayName}`,
            inputs: [
              {
                type: "password",
                name: "password",
                placeholder: "",
              },
            ],
            buttons: [
              {
                text: t("common.cancel"),
                role: "cancel",
                handler: () => resolve(null),
              },
              {
                text: t("common.confirm"),
                handler: (data: any) => {
                  const pwd = data?.password || "";
                  resolve(pwd || null);
                  return true;
                },
              },
            ],
          })
          .then(alert => alert.present());
      }),
    registerTestBackdoor: ctx => {
      import("@/composables/useTestBackdoor").then(({ useTestBackdoor }) => {
        import("@encv/shared-components/composables/useNewTaskModal").then(({ useNewTaskModal }) => {
          const { openNewTask } = useNewTaskModal();
          useTestBackdoor(ctx.files, {
            onLongPress: ctx.onLongPress,
            onClick: ctx.onClick,
            navigateTo: ctx.navigateTo,
            openNewTask: (sourcePath?: string, taskType?: "encrypt" | "decrypt") => openNewTask(sourcePath, taskType),
            __debugOnFileChange: ctx.__debugOnFileChange,
            __debugGetPendingChanges: ctx.__debugGetPendingChanges,
            __debugIsStreamLoading: ctx.__debugIsStreamLoading,
          });
        });
      });
    },
  });
}
