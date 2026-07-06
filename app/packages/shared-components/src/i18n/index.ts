import type { MessageModule } from "./types";
import { registerI18nModules } from "../composables/useI18n";
import common from "./common";
import errors from "./errors";
import settings from "./settings";
import devlogs from "./devlogs";

export const sharedI18nModules: MessageModule[] = [
  common,
  errors,
  settings,
  devlogs,
];

export function initSharedI18n() {
  registerI18nModules(sharedI18nModules);
}

export * from "./types";
