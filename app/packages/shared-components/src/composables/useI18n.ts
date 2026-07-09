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
const HAS_VAR_REGEX = /\{/;

type CompiledTemplate = (params: MessageParams, locale: Locale) => string;

let messageMaps: Record<Locale, Map<string, string>> = {
  "zh-CN": new Map(),
  en: new Map(),
};

const compiledTemplates = new Map<string, CompiledTemplate>();
const simpleResultCache: Record<Locale, Map<string, string>> = {
  "zh-CN": new Map(),
  en: new Map(),
};
const paramResultCache: Record<Locale, Map<string, Map<string, string>>> = {
  "zh-CN": new Map(),
  en: new Map(),
};
const PARAM_CACHE_MAX = 500;

const numberFormatCache = new Map<string, Intl.NumberFormat>();
const dateTimeFormatCache = new Map<string, Intl.DateTimeFormat>();

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

function clearCaches() {
  compiledTemplates.clear();
  simpleResultCache["zh-CN"].clear();
  simpleResultCache.en.clear();
  paramResultCache["zh-CN"].clear();
  paramResultCache.en.clear();
}

function hashParams(params: MessageParams): string {
  const keys = Object.keys(params);
  if (keys.length === 0) return "";
  if (keys.length === 1) {
    const k = keys[0];
    return k + "=" + params[k];
  }
  keys.sort();
  let result = "";
  for (let i = 0; i < keys.length; i++) {
    if (i > 0) result += "&";
    result += keys[i] + "=" + params[keys[i]];
  }
  return result;
}

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
  clearCaches();
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

function compileTemplate(template: string): CompiledTemplate {
  const cached = compiledTemplates.get(template);
  if (cached) return cached;

  if (!HAS_VAR_REGEX.test(template)) {
    const fn: CompiledTemplate = () => template;
    compiledTemplates.set(template, fn);
    return fn;
  }

  const parts: Array<
    | { type: "literal"; value: string }
    | { type: "var"; key: string; format?: string }
  > = [];

  let lastIndex = 0;
  let match: RegExpExecArray | null;
  VAR_REGEX.lastIndex = 0;

  while ((match = VAR_REGEX.exec(template)) !== null) {
    if (match.index > lastIndex) {
      parts.push({ type: "literal", value: template.slice(lastIndex, match.index) });
    }
    const key = match[1].trim();
    const format = match[2];
    parts.push({ type: "var", key, format });
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < template.length) {
    parts.push({ type: "literal", value: template.slice(lastIndex) });
  }

  const fn: CompiledTemplate = (params: MessageParams, locale: Locale) => {
    let result = "";
    for (const part of parts) {
      if (part.type === "literal") {
        result += part.value;
      } else {
        const { key, format } = part;
        let value: MessageParamValue | undefined;

        if (key.includes("|")) {
          const sepIdx = key.indexOf("|");
          const pluralKey = key.slice(0, sepIdx).trim();
          const countStr = key.slice(sepIdx + 1).trim();
          const count = Number(params[countStr] ?? 0);
          const category = getPluralCategory(locale, count);
          const baseKey = pluralKey.replace(/\.(zero|one|other)$/, "");
          const pluralLookupKey = `${baseKey}.${category}`;
          const lookup = messageMaps[locale];
          const val = lookup.get(pluralLookupKey);
          if (val !== undefined) {
            result += compileTemplate(val)(params, locale);
            continue;
          }
          const otherKey = `${baseKey}.other`;
          const otherVal = lookup.get(otherKey);
          if (otherVal !== undefined) {
            result += compileTemplate(otherVal)(params, locale);
            continue;
          }
          result += `{${key}}`;
          continue;
        }

        if (key.includes(".")) {
          value = resolveNestedValue(params as Record<string, any>, key);
        } else {
          value = params[key];
        }

        if (value === undefined || value === null) {
          result += format ?? `{${key}}`;
          continue;
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

        result += strValue;
      }
    }
    return result;
  };

  compiledTemplates.set(template, fn);
  return fn;
}

const isStrictMode = import.meta.env.VITE_I18N_STRICT === "true";
const isDev = import.meta.env.DEV;

let missingKeysBatch: Set<string> | null = null;
let missingKeysTimer: number | null = null;

function flushMissingKeys() {
  if (missingKeysBatch && missingKeysBatch.size > 0) {
    const keys = Array.from(missingKeysBatch);
    console.warn(
      `[i18n] ${keys.length} missing keys:\n  ${keys.slice(0, 20).join("\n  ")}` +
        (keys.length > 20 ? `\n  ... and ${keys.length - 20} more` : ""),
    );
    missingKeysBatch.clear();
  }
  missingKeysTimer = null;
}

function reportMissingKey(key: string, locale: Locale) {
  if (!isDev || isStrictMode) return;

  if (!missingKeysBatch) {
    missingKeysBatch = new Set<string>();
  }
  if (missingKeysBatch.has(key)) return;
  missingKeysBatch.add(key);

  if (missingKeysTimer === null) {
    missingKeysTimer = (self.setTimeout ?? setTimeout)(flushMissingKeys, 1000) as unknown as number;
  }
}

function t(key: string, params?: MessageParams): string {
  const locale = currentLocale.value;
  const lookup = messageMaps[locale];
  const msg = lookup.get(key);

  if (msg !== undefined) {
    if (!params) {
      const cache = simpleResultCache[locale];
      const cached = cache.get(key);
      if (cached !== undefined) return cached;
      cache.set(key, msg);
      return msg;
    }
    const paramCache = paramResultCache[locale];
    let keyCache = paramCache.get(key);
    const paramHash = hashParams(params);
    if (keyCache) {
      const cached = keyCache.get(paramHash);
      if (cached !== undefined) return cached;
    } else {
      keyCache = new Map();
      paramCache.set(key, keyCache);
      if (paramCache.size > PARAM_CACHE_MAX) {
        const firstKey = paramCache.keys().next().value;
        if (firstKey) paramCache.delete(firstKey);
      }
    }
    const result = compileTemplate(msg)(params, locale);
    keyCache.set(paramHash, result);
    return result;
  }

  const fallback = messageMaps.en.get(key);

  if (isStrictMode && fallback === undefined) {
    const errMsg = `[i18n] MISSING KEY: "${key}" (locale: ${locale}, no en fallback)`;
    console.error(errMsg);
    throw new Error(errMsg);
  }

  const displayValue = fallback ?? `[MISSING: ${key}]`;

  if (!isStrictMode) {
    reportMissingKey(key, locale);
  }

  if (!params) {
    return displayValue;
  }

  return compileTemplate(displayValue)(params, locale);
}

function tField(key: string): string {
  const i18nKey = fieldKeyMap[key];
  if (i18nKey) return t(i18nKey);
  return key.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function tSectionTitle(title: string): string {
  const i18nKey = sectionTitleMap[title];
  if (i18nKey) return t(i18nKey);
  return title;
}

function setLocale(locale: Locale) {
  if (currentLocale.value === locale) return;
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
  import.meta.hot.on(
    "i18n-update",
    (data: { locale: Locale; changes: Record<string, string> }) => {
      const targetMap = messageMaps[data.locale];
      const simpleCache = simpleResultCache[data.locale];
      const paramCache = paramResultCache[data.locale];
      for (const key in data.changes) {
        targetMap.set(key, data.changes[key]);
        simpleCache.delete(key);
        paramCache.delete(key);
        compiledTemplates.delete(data.changes[key]);
      }
      currentLocale.value = currentLocale.value;
      console.debug(
        `[i18n-hmr] Updated ${Object.keys(data.changes).length} keys for ${data.locale}`,
      );
    },
  );
}
