/**
 * useFilePickerBrowse — 文件/目录浏览弹窗的共享封装
 *
 * EncryptBody / DecryptBody 原先各自内联一份 `handleBrowseSource` /
 * `handleBrowseTarget`（modalController.create(FilePickerModal) + onDidDismiss
 * 取 path）。两份逐字重复，抽到此处作为单一真源。
 *
 * 返回的 Promise resolve 为选中路径，未选中（取消/关闭）则 resolve null。
 */
import { modalController } from "@ionic/vue";
import FilePickerModal from "@encv/shared-components/components/FilePickerModal.vue";

export function useFilePickerBrowse() {
  async function browseFile(): Promise<string | null> {
    const modal = await modalController.create({
      component: FilePickerModal,
      componentProps: { mode: "file" as const },
    });
    await modal.present();
    const { data, role } = await modal.onDidDismiss();
    if (role === "select" && data) return data.path;
    return null;
  }

  async function browseFolder(): Promise<string | null> {
    const modal = await modalController.create({
      component: FilePickerModal,
      componentProps: { mode: "folder" as const },
    });
    await modal.present();
    const { data, role } = await modal.onDidDismiss();
    if (role === "select" && data) return data.path;
    return null;
  }

  return { browseFile, browseFolder };
}
