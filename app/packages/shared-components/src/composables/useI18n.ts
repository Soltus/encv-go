import agent from "@encv/shared-components/i18n/agent";
import common from "@encv/shared-components/i18n/common";
import devlogs from "@encv/shared-components/i18n/devlogs";
import errors from "@encv/shared-components/i18n/errors";
import extensions from "@encv/shared-components/i18n/extensions";
import files from "@encv/shared-components/i18n/files";
import modals from "@encv/shared-components/i18n/modals";
import player from "@encv/shared-components/i18n/player";
import settings from "@encv/shared-components/i18n/settings";
import tasks from "@encv/shared-components/i18n/tasks";
import { ref } from "vue";

// ===== 公开类型 =====

export type Locale = "zh-CN" | "en";
export type MessageParams = Record<string, string>;
export type TFunction = (key: string, params?: MessageParams) => string;
export type TFieldFunction = (key: string) => string;
export type TSectionTitleFunction = (title: string) => string;

type MessageModule = { "zh-CN": Record<string, string>; en: Record<string, string> };

function mergeModules(modules: MessageModule[]): Record<Locale, Record<string, string>> {
  const merged: Record<Locale, Record<string, string>> = { "zh-CN": {}, en: {} };
  for (const mod of modules) {
    merged["zh-CN"] = { ...merged["zh-CN"], ...mod["zh-CN"] };
    merged.en = { ...merged.en, ...mod.en };
  }
  return merged;
}

const messages: Record<Locale, Record<string, string>> = mergeModules([
  common,
  tasks,
  files,
  player,
  settings,
  devlogs,
  extensions,
  errors,
  modals,
  agent,
]);

function getStoredLocale(): Locale {
  const stored = localStorage.getItem("encv-locale");
  if (stored === "en" || stored === "zh-CN") return stored;
  return "zh-CN";
}

const currentLocale = ref<Locale>(getStoredLocale());

const fieldKeyMap: Record<string, string> = {
  password: "settings.password",
  recover: "settings.recover",
  output_path: "settings.outputPath",
  plugin_settings: "settings.pluginSettings",
  server: "settings.httpServerSettings",
  admin: "settings.adminServerSettings",
  webdav: "settings.webdavServerSettings",
  proxy: "settings.proxyServerSettings",
  log: "settings.logSettings",
  port: "settings.port",
  dir: "settings.dir",
  username: "settings.username",
  root: "settings.root",
  level: "settings.level",
  file: "settings.file",
  host: "settings.host",
  description: "settings.description",
  sites: "settings.sites",
  disable_signature_verification: "settings.disableSignatureVerification",
  ext: "settings.ext",
  chunk_size_mb: "settings.chunkSizeMb",
  light_main_chunk_enabled: "settings.lightMainChunkEnabled",
  track_extensions: "settings.trackExtensions",
  keep_mkv_for_mkv_source: "settings.keepMkvForMkvSource",
  verify_after_pack: "settings.verifyAfterPack",
  plugin_cache_dir: "settings.pluginCacheDir",
  skip_merge_for_split_mkv: "settings.skipMergeForSplitMkv",
  video: "settings.video",
  audio: "settings.audio",
  image: "settings.image",
  wps: "settings.wps",
  pdf: "settings.pdf",
  text: "settings.text",
  custom_text_extensions: "settings.customTextExts",
  allow_no_reencode: "settings.allowNoReencode",
  default_stream_preset: "settings.defaultStreamPreset",
  suffix: "settings.alistEncryptSuffix",
  default_password: "settings.alistEncryptDefaultPassword",
  algorithm: "settings.alistEncryptAlgorithm",
  enabled: "settings.alistEncryptEnable",
  agent_settings: "settings.agentSettings",
  openai_api_key: "settings.openaiApiKey",
  openai_base_url: "settings.openaiBaseUrl",
  openai_model: "settings.openaiModel",
  openlist_base_url: "settings.openlistBaseUrl",
  openlist_token: "settings.openlistToken",
  default_container_version: "settings.defaultContainerVersion",
  enabled_tools: "settings.enabledTools",
  system_prompt: "settings.systemPrompt",
  max_tool_calls_per_turn: "settings.maxToolCallsPerTurn",
};

const sectionTitleMap: Record<string, string> = {
  全局设置: "settings.globalSettings",
  "加密/解密设置": "settings.encryptDecryptSettings",
  "内置HTTP服务器 设置": "settings.httpServerSettings",
  "管理后台服务器 设置": "settings.adminServerSettings",
  "WebDAV 服务器设置": "settings.webdavServerSettings",
  "Openlist 代理服务器设置": "settings.proxyServerSettings",
  日志设置: "settings.logSettings",
  "AI 助手设置": "settings.agentSettings",
};

/**
 * 把 `{key}` 占位符替换成 params[key]。
 * 占位符不区分大小写、支持多次出现（按 replaceAll 语义）。
 */
function injectParams(msg: string, params: MessageParams): string {
  let result = msg;
  for (const [k, v] of Object.entries(params)) {
    const re = new RegExp(`\\{${k}\\}`, "g");
    result = result.replace(re, v);
  }
  return result;
}

// DEV 下 missing key 只警告一次（避免循环 render 刷屏）
let tMissingWarned: Set<string> | null = null;

// 严格模式：测试环境缺 key 直接抛错，避免测试用 class 选择器漏测 i18n
// 用 VITE_I18N_STRICT=true 开启（Cypress 自动注入）
const isStrictMode = import.meta.env.VITE_I18N_STRICT === "true";

function t(key: string, params?: MessageParams): string {
  const lookup = messages[currentLocale.value];
  if (!lookup || !(key in lookup)) {
    // 回退链：zh-CN → en → key
    const fallback = messages.en[key];

    if (isStrictMode && !fallback) {
      const msg = `[i18n] MISSING KEY: "${key}" (locale: ${currentLocale.value}, no en fallback)`;
      // eslint-disable-next-line no-console
      console.error(msg);
      // 抛错让测试挂掉，同时返回醒目占位符让 DOM 断言也能发现
      throw new Error(msg);
    }

    const displayValue = fallback ?? `[MISSING: ${key}]`;

    if (import.meta.env.DEV && !isStrictMode) {
      const warned = (tMissingWarned ??= new Set<string>());
      if (!warned.has(key)) {
        warned.add(key);
        // eslint-disable-next-line no-console
        console.warn(`[i18n] missing key: ${key} (locale: ${currentLocale.value})`);
      }
    }

    return params ? injectParams(displayValue, params) : displayValue;
  }
  const msg = lookup[key];
  return params ? injectParams(msg, params) : msg;
}

function tField(key: string): string {
  const i18nKey = fieldKeyMap[key];
  if (i18nKey) return t(i18nKey);
  return key.replace(/_/g, " ").replace(/\b\w/g, c => c.toUpperCase());
}

function tSectionTitle(title: string): string {
  const i18nKey = sectionTitleMap[title];
  if (i18nKey) return t(i18nKey);
  return title;
}

function setLocale(locale: Locale) {
  currentLocale.value = locale;
  localStorage.setItem("encv-locale", locale);
}

function getLocale(): Locale {
  return currentLocale.value;
}

export function useI18n(): {
  t: TFunction;
  tField: TFieldFunction;
  tSectionTitle: TSectionTitleFunction;
  setLocale: (locale: Locale) => void;
  getLocale: () => Locale;
  locale: typeof currentLocale;
} {
  return {
    t,
    tField,
    tSectionTitle,
    setLocale,
    getLocale,
    locale: currentLocale,
  };
}
