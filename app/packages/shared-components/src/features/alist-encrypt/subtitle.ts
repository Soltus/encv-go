import { useI18n } from "@encv/shared-components/composables/useI18n";
import type { FileSubtitle } from "@encv/shared-components/types/file-feature";
import { getDecodedName, isAlistEncrypted, loadDecodedName } from "@encv/shared-components/composables/useAlistEncrypt";

const { t } = useI18n();

const DEBOUNCE_MS = 300;
const pendingRequests = new Map<string, Promise<FileSubtitle | null>>();

export async function getAlistSubtitle(file: any): Promise<FileSubtitle | null> {
  if (!isAlistEncrypted(file)) return null;

  const cached = getDecodedName(file.path);
  if (cached) {
    return {
      text: `${t("alistEncrypt.realFilename")}: ${cached}`,
      color: "var(--ion-color-danger)",
    };
  }

  const existing = pendingRequests.get(file.path);
  if (existing) return existing;

  const promise = new Promise<FileSubtitle | null>(resolve => {
    setTimeout(async () => {
      pendingRequests.delete(file.path);
      const plainName = await loadDecodedName(file);
      if (plainName) {
        resolve({
          text: `${t("alistEncrypt.realFilename")}: ${plainName}`,
          color: "var(--ion-color-danger)",
        });
      } else {
        resolve(null);
      }
    }, DEBOUNCE_MS);
  });

  pendingRequests.set(file.path, promise);
  return promise;
}

export function preloadSubtitles(files: any[]): void {
  for (const file of files) {
    if (!isAlistEncrypted(file)) continue;
    if (getDecodedName(file.path)) continue;
    const existing = pendingRequests.get(file.path);
    if (existing) continue;

    const promise = new Promise<FileSubtitle | null>(resolve => {
      setTimeout(async () => {
        pendingRequests.delete(file.path);
        await loadDecodedName(file);
        resolve(null);
      }, DEBOUNCE_MS);
    });

    pendingRequests.set(file.path, promise);
  }
}
