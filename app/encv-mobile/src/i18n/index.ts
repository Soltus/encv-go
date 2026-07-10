import type { MessageModule } from "@encv/shared-components/i18n";
import agent from "./agent";
import extensions from "./extensions";
import files from "./files";
import modals from "./modals";
import player from "./player";
import simverse from "./simverse";
import tasks from "./tasks";

export const encvI18nModules: MessageModule[] = [tasks, files, player, extensions, modals, agent, simverse];

export { initEncvI18n } from "./init";
