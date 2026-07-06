import common from "@/i18n/common";
import errors from "@/i18n/errors";
import { ref } from "vue";

export type Locale = "zh-CN" | "en";
export type MessageParams = Record<string, string>;
export type TFunction = (key: string, params?: MessageParams) => string;
export type TFieldFunction = (key: string) => string;
export type TSectionTitleFunction = (title: string) => string;

export type MessageModule = { "zh-CN": Record<string, string>; en: Record<string, string> };

function mergeModules(modules: MessageModule[]): Record<Locale, Record<string, string>> {
  const merged: Record<Locale, Record<string, string>> = { "zh-CN": {}, en: {} };
  for (const mod of modules) {
    merged["zh-CN"] = { ...merged["zh-CN"], ...mod["zh-CN"] };
    merged.en = { ...merged.en, ...mod.en };
  }
  return merged;
}

const registeredModules: MessageModule[] = [common, errors];
let messages: Record<Locale, Record<string, string>> = mergeModules(registeredModules);

export function registerI18nModule(mod: MessageModule) {
  registeredModules.push(mod);
  messages = mergeModules(registeredModules);
}

export function registerI18nModules(mods: MessageModule[]) {
  registeredModules.push(...mods);
  messages = mergeModules(registeredModules);
}

function getStoredLocale(): Locale {
  const stored = localStorage.getItem("encv-locale");
  if (stored === "en" || stored === "zh-CN") return stored;
  return "zh-CN";
}

const currentLocale = ref<Locale>(getStoredLocale());

const fieldKeyMap: Record<string, string> = {};
const sectionTitleMap: Record<string, string> = {};

export function registerFieldKeyMap(map: Record<string, string>) {
  Object.assign(fieldKeyMap, map);
}

export function registerSectionTitleMap(map: Record<string, string>) {
  Object.assign(sectionTitleMap, map);
}

function injectParams(msg: string, params: MessageParams): string {
  let result = msg;
  for (const [k, v] of Object.entries(params)) {
    const re = new RegExp(`\\{${k}\\}`, "g");
    result = result.replace(re, v);
  }
  return result;
}

let tMissingWarned: Set<string> | null = null;

const isStrictMode = import.meta.env.VITE_I18N_STRICT === "true";

function t(key: string, params?: MessageParams): string {
  const lookup = messages[currentLocale.value];
  if (!lookup || !(key in lookup)) {
    const fallback = messages.en[key];

    if (isStrictMode && !fallback) {
      const msg = `[i18n] MISSING KEY: "${key}" (locale: ${currentLocale.value}, no en fallback)`;
      // eslint-disable-next-line no-console
      console.error(msg);
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
