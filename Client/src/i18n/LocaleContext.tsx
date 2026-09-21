import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { localeStorageKey, normalizeLocale } from './locales';
import type { Locale } from './locales';
import { interpolate, messages } from './messages';
import type { MessageKey, MessageParameters } from './messages';
function initialLocale(): Locale {
  try { return normalizeLocale(localStorage.getItem(localeStorageKey)) ?? 'en'; } catch { return 'en'; }
}
function createValue(locale: Locale, setLocale: (locale: Locale) => void) {
  return {
    locale, setLocale, direction: locale === 'ar' ? 'rtl' as const : 'ltr' as const,
    // Honest provenance: custom catalogs are not yet authored for other locales.
    messageLocale: 'en' as const,
    t: (key: MessageKey, parameters?: MessageParameters) => interpolate(messages[key], parameters),
    number: (value: number, options?: Intl.NumberFormatOptions) => new Intl.NumberFormat(locale,options).format(value),
    date: (value: Date | number, options?: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(locale,options).format(value),
    plural: (value: number, options?: Intl.PluralRulesOptions) => new Intl.PluralRules(locale,options).select(value),
  };
}
const LocaleContext = createContext(createValue('en', () => {}));
export function LocaleProvider({children}: {children: React.ReactNode}) {
  const [locale,setLocale] = useState<Locale>(initialLocale);
  useEffect(() => {
    // Until custom catalogs exist, surrounding interface text remains English.
    document.documentElement.lang = 'en';
    document.documentElement.dataset.viewerLocale = locale;
    document.documentElement.dir = locale === 'ar' ? 'rtl' : 'ltr';
    try { localStorage.setItem(localeStorageKey,locale); } catch { /* Session selection still works without storage. */ }
  },[locale]);
  useEffect(() => {
    const sync = (event: StorageEvent) => { if (event.key === localeStorageKey) setLocale(normalizeLocale(event.newValue) ?? 'en'); };
    window.addEventListener('storage',sync);
    return () => window.removeEventListener('storage',sync);
  },[]);
  const value = useMemo(() => createValue(locale,setLocale),[locale]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
export function useLocale() { return useContext(LocaleContext); }
