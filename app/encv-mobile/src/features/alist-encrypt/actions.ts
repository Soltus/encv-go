import { lockClosed, lockOpen, videocam } from "ionicons/icons";
import type { FileItem } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import { useNewTaskModal } from "@/composables/useNewTaskModal";
import router from "@/router";
import type { FileAction } from "@/types/file-feature";
import { promptPassword } from "./password-dialog";
import { getDecodedName, isAlistEncrypted, loadDecodedName, setSessionPassword } from "./useAlistEncrypt";

const { t } = useI18n();

export function getAlistActions(file: FileItem): FileAction[] {
  if (isAlistEncrypted(file)) {
    return [
      {
        id: "alist-stream-preview",
        text: () => t("alistEncrypt.streamPreview"),
        icon: videocam,
        color: "primary",
        handler: async (f: FileItem) => {
          try {
            const password = await promptPassword(f.name);
            if (password == null) return;
            setSessionPassword(f.path, password);
            await loadDecodedName(f, password);
            const decodedName = getDecodedName(f.path) || f.name;
            router.push({ path: "/player", query: { path: f.path, name: decodedName, alistPath: f.path, alistPassword: password } });
          } catch (e) {
            console.error("[alist] stream-preview error:", e);
          }
        },
      },
      {
        id: "alist-decrypt",
        text: () => t("files.decrypt"),
        icon: lockClosed,
        color: "warning",
        handler: async (f: FileItem) => {
          try {
            const { openNewTask } = useNewTaskModal();
            openNewTask(f.path, "decrypt");
          } catch (e) {
            console.error("[alist] decrypt error:", e);
          }
        },
      },
    ];
  }

  if (file.isEncrypted === true) {
    return [
      {
        id: "alist-decrypt-container",
        text: () => t("files.decrypt"),
        icon: lockOpen,
        color: "primary",
        handler: async (f: FileItem) => {
          const { openNewTask } = useNewTaskModal();
          openNewTask(f.path, "decrypt");
        },
      },
    ];
  }

  return [
    {
      id: "alist-encrypt",
      text: () => t("alistEncrypt.encrypt"),
      icon: lockClosed,
      color: "warning",
      handler: async (f: FileItem) => {
        const { openNewTask } = useNewTaskModal();
        openNewTask(f.path, "encrypt");
      },
    },
  ];
}
