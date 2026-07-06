import common from "../i18n/common";
import errors from "../i18n/errors";
import { ref } from "vue";

export type Locale = "zh-CN" | "en";
export type MessageParamValue = string | number | boolean;
export type MessageParams = Record<string, MessageParamValue>;
export type TFunction = (key: string, params?: MessageParams) => string;
export type TFieldFunction = (key: string) => string;
export type TSectionTitleFunction = (title: string) => string;

export type MessageModule = { "zh-CN": Record<string, string>; en: Record<string, string> };

const PLURAL_ZERO = "zero";
const PLURAL_ONE = "one";
const PLURAL_OTHER = "other";

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

function getPluralCategory(locale: Locale, count: number): string {
  if (locale === "zh-CN") {
    return PLURAL_OTHER;
  }
  if (count === 0) return PLURAL_ZERO;
  if (count === 1) return PLURAL_ONE;
  return PLURAL_OTHER;
}

function resolveNestedValue(obj: Record<string, any>, path: string): string {
  const parts = path.split(".");
  let current: any = obj;
  for (const part of parts) {
    if (current == null || typeof current !== "object") {
      return "";
    }
    current = current[part];
  }
  return String(current ?? "");
}

function injectParams(msg: string, params: MessageParams, locale: Locale): string {
  let result = msg;

  result = result.replace(/\{([^}:]+)(?::([^}]+))?\}/g, (match, key, format) => {
    const trimmedKey = key.trim();

    if (trimmedKey.includes("|")) {
      const [pluralKey, countStr] = trimmedKey.split("|").map(s => s.trim());
      const count = Number(params[countStr] ?? 0);
      const category = getPluralCategory(locale, count);
      const baseKey = pluralKey.replace(/\.(zero|one|other)$/, "");
      const pluralLookupKey = `${baseKey}.${category}`;
      const lookup = messages[locale];
      if (pluralLookupKey in lookup) {
        return injectParams(lookup[pluralLookupKey], params, locale);
      }
      const otherKey = `${baseKey}.other`;
      if (otherKey in lookup) {
        return injectParams(lookup[otherKey], params, locale);
      }
      return match;
    }

    let value: MessageParamValue | undefined;
    if (trimmedKey.includes(".")) {
      value = resolveNestedValue(params as Record<string, any>, trimmedKey);
    } else {
      value = params[trimmedKey];
    }

    if (value === undefined || value === null) {
      if (format) {
        return format;
      }
      return match;
    }

    let strValue = String(value);

    if (format) {
      if (format === "number") {
        const num = Number(value);
        if (!Number.isNaN(num)) {
          strValue = new Intl.NumberFormat(locale).format(num);
        }
      } else if (format === "date") {
        const ts = Number(value);
        if (!Number.isNaN(ts) && ts > 0) {
          strValue = new Intl.DateTimeFormat(locale).format(new Date(ts));
        }
      } else if (format === "datetime") {
        const ts = Number(value);
        if (!Number.isNaN(ts) && ts > 0) {
          strValue = new Intl.DateTimeFormat(locale, {
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
          }).format(new Date(ts));
        }
      }
    }

    return strValue;
  });

  return result;
}

let tMissingWarned: Set<string> | null = null;

const isStrictMode = import.meta.env.VITE_I18N_STRICT === "true";

function t(key: string, params?: MessageParams): string {
  const locale = currentLocale.value;
  const lookup = messages[locale];
  if (!lookup || !(key in lookup)) {
    const fallback = messages.en[key];

    if (isStrictMode && !fallback) {
      const msg = `[i18n] MISSING KEY: "${key}" (locale: ${locale}, no en fallback)`;
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
        console.warn(`[i18n] missing key: ${key} (locale: ${locale})`);
      }
    }

    return params ? injectParams(displayValue, params, locale) : displayValue;
  }
  const msg = lookup[key];
  return params ? injectParams(msg, params, locale) : msg;
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
