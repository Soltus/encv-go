import { useI18n } from "@encv/shared-components/composables/useI18n";
import { getAppCapabilities } from "@encv/shared-components/runtime/appCapabilities";

const { t } = useI18n();

/**
 * 弹出密码输入对话框。
 * 实际 UI（@ionic/vue 的 alertController + i18n 文案）由 app 经
 * setAppCapabilities({ alertPassword }) 注入，shared 不反向依赖应用层。
 */
export async function promptPassword(fileDisplayName: string): Promise<string | null> {
  return getAppCapabilities().alertPassword(fileDisplayName);
}
