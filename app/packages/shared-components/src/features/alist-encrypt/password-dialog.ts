import { alertController } from "@ionic/vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

export async function promptPassword(fileDisplayName: string): Promise<string | null> {
  return new Promise(resolve => {
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
  });
}
