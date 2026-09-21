import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { localeStorageKey, normalizeLocale } from './locales';
import type { Locale } from './locales';
import { messages } from './messages';
import { CitadelAPI } from '../api/CitadelClient';
import { formatMessage } from './formatMessage';
import type { Catalog, OfficialCatalog } from './formatMessage';
import { loadMessageCatalog } from './catalogs';
import type { MessageKey, MessageParameters } from './messages';
function initialLocale(): Locale {
  try { return normalizeLocale(localStorage.getItem(localeStorageKey)) ?? 'en'; } catch { return 'en'; }
}
const officialKeys: Partial<Record<MessageKey,string>> = {'navigation.castle':'castle','navigation.equipment':'dialog_equipment_title','navigation.movement':'dialog_recuit_generals'};
function createValue(locale: Locale, setLocale: (locale: Locale) => void, catalog: Catalog = {}, game?: OfficialCatalog) {
  return {
    locale, setLocale, direction: locale === 'ar' ? 'rtl' as const : 'ltr' as const,
    messageLocale: Object.keys(catalog).length ? locale : 'en',
    t: (key: MessageKey, parameters?: MessageParameters) => formatMessage({key,fallback:messages[key],params:parameters ? {...parameters} : undefined,officialKey:officialKeys[key]},locale,catalog,game).text,
    number: (value: number, options?: Intl.NumberFormatOptions) => new Intl.NumberFormat(locale,options).format(value),
    date: (value: Date | number, options?: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(locale,options).format(value),
    plural: (value: number, options?: Intl.PluralRulesOptions) => new Intl.PluralRules(locale,options).select(value),
  };
}
const LocaleContext = createContext(createValue('en', () => {}));
export function LocaleProvider({children}: {children: React.ReactNode}) {
  const [locale,setLocale] = useState<Locale>(initialLocale);
  const [game,setGame] = useState<{locale: Locale; catalog: OfficialCatalog} | null>(null);
  const [loaded,setLoaded] = useState<{locale: Locale; catalog: Catalog}>({locale:'en',catalog:{}});
  useEffect(() => {
    let active = true;
    void loadMessageCatalog(locale).then(catalog => { if (active) setLoaded({locale,catalog}); }).catch(() => { if (active) setLoaded({locale,catalog:{}}); });
    return () => { active = false; };
  },[locale]);
  useEffect(() => {
    let active = true;
    void CitadelAPI.localizeCatalog(Object.values(officialKeys),locale).then(result => {
      if (active) setGame({locale,catalog:{values:result.values,resolvedLocale:result.locale?.resolvedLocale ?? 'en',fallbackKeys:result.locale?.fallbackKeys}});
    }).catch(() => { if (active) setGame(null); });
    return () => { active = false; };
  },[locale]);
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
  const value = useMemo(() => createValue(locale,setLocale,loaded.locale === locale ? loaded.catalog : {},game?.locale === locale ? game.catalog : undefined),[locale,loaded,game]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
export function useLocale() { return useContext(LocaleContext); }
