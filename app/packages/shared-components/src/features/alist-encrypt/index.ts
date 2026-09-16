import { lockClosed } from "ionicons/icons";
import type { FileFeature } from "@encv/shared-components/types/file-feature";
import { getAlistActions } from "./actions";
import { getAlistBadge } from "./badge";
import { getAlistSubtitle, preloadSubtitles } from "./subtitle";
import {
  clearDecodeCache,
  clearPasswordCache,
  getDecodedName,
  getSessionPassword,
  getStreamUrl,
  isAlistEncrypted,
  loadDecodedName,
  setSessionPassword,
} from "@encv/shared-components/composables/useAlistEncrypt";

export function createAlistEncryptFeature(): FileFeature {
  return {
    id: "alist-encrypt",
    isActive: file => !file.isDirectory,
    getBadge: file => getAlistBadge(file),
    getSubtitle: file => getAlistSubtitle(file),
    getFileActions: file => getAlistActions(file),
    isContainerFile: file => isAlistEncrypted(file),
    handleClick: (file): { handled: true } | null => {
      if (!isAlistEncrypted(file)) return null;
      return { handled: true };
    },
    icon: lockClosed,
    onActivate() {
      console.info("[alist-encrypt] Feature activated");
    },
    onDeactivate() {
      clearPasswordCache();
      clearDecodeCache();
    },
  };
}

export { promptPassword } from "./password-dialog";
export { getDecodedName, getSessionPassword, getStreamUrl, isAlistEncrypted, loadDecodedName, preloadSubtitles, setSessionPassword };
