import { registerI18nModules } from "../composables/useI18n";
import common from "./common";
import devlogs from "./devlogs";
import errors from "./errors";
import settings from "./settings";
import type { MessageModule } from "./types";

export const sharedI18nModules: MessageModule[] = [common, errors, settings, devlogs];

export function initSharedI18n() {
  registerI18nModules(sharedI18nModules);
}

export * from "./types";
