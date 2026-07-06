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

const VAR_REGEX = /\{([^}:]+)(?::([^}]+))?\}/g;

let messageMaps: Record<Locale, Map<string, string>> = {
  "zh-CN": new Map(),
  en: new Map(),
};

function mergeModulesToMap(modules: MessageModule[]): Record<Locale, Map<string, string>> {
  const merged: Record<Locale, Map<string, string>> = {
    "zh-CN": new Map(),
    en: new Map(),
  };
  for (const mod of modules) {
    const zhMap = mod["zh-CN"];
    const enMap = mod.en;
    const zhTarget = merged["zh-CN"];
    const enTarget = merged.en;
    for (const key in zhMap) {
      zhTarget.set(key, zhMap[key]);
    }
    for (const key in enMap) {
      enTarget.set(key, enMap[key]);
    }
  }
  return merged;
}

const registeredModules: MessageModule[] = [common, errors];
messageMaps = mergeModulesToMap(registeredModules);

export function registerI18nModule(mod: MessageModule) {
  registeredModules.push(mod);
  const zhSrc = mod["zh-CN"];
  const enSrc = mod.en;
  const zhTarget = messageMaps["zh-CN"];
  const enTarget = messageMaps.en;
  for (const key in zhSrc) {
    zhTarget.set(key, zhSrc[key]);
  }
  for (const key in enSrc) {
    enTarget.set(key, enSrc[key]);
  }
}

export function registerI18nModules(mods: MessageModule[]) {
  for (const mod of mods) {
    registerI18nModule(mod);
  }
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
  let current: any = obj;
  const parts = path.split(".");
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (current == null || typeof current !== "object") {
      return "";
    }
    current = current[part];
  }
  return String(current ?? "");
}

const numberFormatCache = new Map<string, Intl.NumberFormat>();
const dateTimeFormatCache = new Map<string, Intl.DateTimeFormat>();

function getNumberFormat(locale: string): Intl.NumberFormat {
  let fmt = numberFormatCache.get(locale);
  if (!fmt) {
    fmt = new Intl.NumberFormat(locale);
    numberFormatCache.set(locale, fmt);
  }
  return fmt;
}

function getDateTimeFormat(locale: string, opts?: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = opts ? locale + JSON.stringify(opts) : locale;
  let fmt = dateTimeFormatCache.get(key);
  if (!fmt) {
    fmt = new Intl.DateTimeFormat(locale, opts);
    dateTimeFormatCache.set(key, fmt);
  }
  return fmt;
}

function injectParams(msg: string, params: MessageParams, locale: Locale): string {
  if (!params || Object.keys(params).length === 0) {
    return msg;
  }

  return msg.replace(VAR_REGEX, (match, key, format) => {
    const trimmedKey = key.trim();

    if (trimmedKey.includes("|")) {
      const sepIdx = trimmedKey.indexOf("|");
      const pluralKey = trimmedKey.slice(0, sepIdx).trim();
      const countStr = trimmedKey.slice(sepIdx + 1).trim();
      const count = Number(params[countStr] ?? 0);
      const category = getPluralCategory(locale, count);
      const baseKey = pluralKey.replace(/\.(zero|one|other)$/, "");
      const pluralLookupKey = `${baseKey}.${category}`;
      const lookup = messageMaps[locale];
      const val = lookup.get(pluralLookupKey);
      if (val !== undefined) {
        return injectParams(val, params, locale);
      }
      const otherKey = `${baseKey}.other`;
      const otherVal = lookup.get(otherKey);
      if (otherVal !== undefined) {
        return injectParams(otherVal, params, locale);
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
          strValue = getNumberFormat(locale).format(num);
        }
      } else if (format === "date") {
        const ts = Number(value);
        if (!Number.isNaN(ts) && ts > 0) {
          strValue = getDateTimeFormat(locale).format(new Date(ts));
        }
      } else if (format === "datetime") {
        const ts = Number(value);
        if (!Number.isNaN(ts) && ts > 0) {
          strValue = getDateTimeFormat(locale, {
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
}

let tMissingWarned: Set<string> | null = null;

const isStrictMode = import.meta.env.VITE_I18N_STRICT === "true";

function t(key: string, params?: MessageParams): string {
  const locale = currentLocale.value;
  const lookup = messageMaps[locale];
  const msg = lookup.get(key);

  if (msg !== undefined) {
    return params ? injectParams(msg, params, locale) : msg;
  }

  const fallback = messageMaps.en.get(key);

  if (isStrictMode && fallback === undefined) {
    const msg = `[i18n] MISSING KEY: "${key}" (locale: ${locale}, no en fallback)`;
    console.error(msg);
    throw new Error(msg);
  }

  const displayValue = fallback ?? `[MISSING: ${key}]`;

  if (import.meta.env.DEV && !isStrictMode) {
    const warned = (tMissingWarned ??= new Set<string>());
    if (!warned.has(key)) {
      warned.add(key);
      console.warn(`[i18n] missing key: ${key} (locale: ${locale})`);
    }
  }

  return params ? injectParams(displayValue, params, locale) : displayValue;
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

if (import.meta.hot) {
  import.meta.hot.on("i18n-update", (data: { locale: Locale; changes: Record<string, string> }) => {
    const targetMap = messageMaps[data.locale];
    for (const key in data.changes) {
      targetMap.set(key, data.changes[key]);
    }
    currentLocale.value = currentLocale.value;
    console.debug(`[i18n-hmr] Updated ${Object.keys(data.changes).length} keys for ${data.locale}`);
  });
}
