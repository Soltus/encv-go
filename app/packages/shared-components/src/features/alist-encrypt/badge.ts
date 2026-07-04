import type { FileBadge } from "@encv/shared-components/types/file-feature";
import { isAlistEncrypted } from "./useAlistEncrypt";

export function getAlistBadge(file: any): FileBadge | null {
  if (!isAlistEncrypted(file)) return null;
  return {
    text: "AE",
    color: "danger",
  };
}
