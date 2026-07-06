import type { MessageModule } from "@encv/shared-components/i18n";
import tasks from "./tasks";
import files from "./files";
import agent from "./agent";
import player from "./player";
import modals from "./modals";
import extensions from "./extensions";
import simverse from "./simverse";

export const encvI18nModules: MessageModule[] = [
  tasks,
  files,
  player,
  extensions,
  modals,
  agent,
  simverse,
];

export { initEncvI18n } from "./init";
