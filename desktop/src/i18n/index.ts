import { en as dashboardEn } from './en';
import { zh as dashboardZh } from './zh';
import { desktopCatalog } from './desktop';
import { loginMessages } from './login';
import type { Dictionary } from './types';

export type Language = 'en' | 'zh';
export type { Dictionary };

const CATALOGS: Record<Language, Dictionary> = {
  en: { ...dashboardEn, ...desktopCatalog.en, login: loginMessages.en as unknown as Dictionary },
  zh: { ...dashboardZh, ...desktopCatalog.zh, login: loginMessages.zh as unknown as Dictionary },
};

const STORAGE_KEY = 'nexus_language';
export const DEFAULT_LANGUAGE: Language = 'en';

export function normalizeLanguage(value: unknown): Language {
  if (typeof value === 'string') {
    const v = value.trim().toLowerCase();
    if (v === 'zh' || v.startsWith('zh-')) return 'zh';
    if (v === 'en' || v.startsWith('en-')) return 'en';
  }
  return DEFAULT_LANGUAGE;
}

/** Mirrors the web app: persisted choice, else browser language, else 'en'. */
export function resolveInitialLanguage(): Language {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored) return normalizeLanguage(stored);
  } catch {
    // Storage unavailable; fall through to browser detection.
  }
  const languages = typeof navigator !== 'undefined' ? navigator.languages : undefined;
  if (languages) {
    for (const candidate of languages) {
      const lower = candidate.toLowerCase();
      if (lower.startsWith('zh')) return 'zh';
      if (lower.startsWith('en')) return 'en';
    }
  }
  return DEFAULT_LANGUAGE;
}

export function persistLanguage(lang: Language): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    // Non-fatal: language resets to detection on next launch.
  }
}

export type TranslatorParams = Record<string, unknown>;

export interface Translator {
  (key: string, params?: TranslatorParams): string;
  lang: Language;
}

function lookup(dict: Dictionary, key: string): unknown {
  let current: unknown = dict;
  for (const part of key.split('.')) {
    if (current !== null && typeof current === 'object' && part in current) {
      current = (current as Dictionary)[part];
    } else {
      return undefined;
    }
  }
  return current;
}

function interpolate(template: string, params?: TranslatorParams): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_match, name: string) => {
    const value = params[name];
    return value === undefined ? '' : String(value);
  });
}

/** Dot-path lookup with en fallback, {param} interpolation and function values — same contract as the web app. */
export function createTranslator(lang: Language): Translator {
  const translator = ((key: string, params?: TranslatorParams): string => {
    const value = lookup(CATALOGS[lang], key) ?? lookup(CATALOGS.en, key);
    if (typeof value === 'function') {
      return String((value as (p: TranslatorParams) => unknown)(params ?? {}));
    }    if (typeof value === 'string') {
      return interpolate(value, params);
    }
    return key;
  }) as Translator;
  translator.lang = lang;
  return translator;
}
