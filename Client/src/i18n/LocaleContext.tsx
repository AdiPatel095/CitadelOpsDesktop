import { invalidateOfficialMessages } from './officialMessages';
import { loadBackendCatalog } from './backendCatalog';
import { officialMessageKeys } from './officialKeys';
import { createContext, useContext, useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { readViewerLocale, setViewerLocale, subscribeViewerLocale } from './viewerLocaleStore';
import type { Locale } from './locales';
import { messages } from './messages';
import { CitadelAPI } from '../api/CitadelClient';
import { formatMessage } from './formatMessage';
import type { Catalog, OfficialCatalog } from './formatMessage';
import { loadMessageCatalog } from './catalogs';
import type { MessageKey, MessageParameters } from './messages';
const officialKeys: Partial<Record<MessageKey,string>> = officialMessageKeys;
function createValue(locale: Locale, setLocale: (locale: Locale) => void, catalog: Catalog = {}, game?: OfficialCatalog, runtimeStatus = 'Disconnected') {
  const message = (key: MessageKey, parameters?: MessageParameters) => formatMessage({key,fallback:messages[key],params:parameters ? {...parameters} : undefined,officialKey:officialKeys[key]},locale,catalog,game);
  return {
    message, catalog, runtimeStatus,
    locale, setLocale, direction: locale === 'ar' ? 'rtl' as const : 'ltr' as const,
    messageLocale: Object.keys(catalog).length ? locale : 'en',
    t: (key: MessageKey, parameters?: MessageParameters) => message(key,parameters).text,
    number: (value: number, options?: Intl.NumberFormatOptions) => new Intl.NumberFormat(locale,options).format(value),
    date: (value: Date | number, options?: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(locale,options).format(value),
    plural: (value: number, options?: Intl.PluralRulesOptions) => new Intl.PluralRules(locale,options).select(value),
  };
}
const LocaleContext = createContext(createValue('en', () => {}));
export function LocaleProvider({children}: {children: React.ReactNode}) {
  const locale = useSyncExternalStore(subscribeViewerLocale,readViewerLocale,()=> 'en' as Locale);
  const setLocale = setViewerLocale;
  const [connection,setConnection] = useState('Disconnected');
  useEffect(() => CitadelAPI.subscribeStatus(status => { invalidateOfficialMessages(); setConnection(status); }),[]);
  const [game,setGame] = useState<{locale: Locale; catalog: OfficialCatalog} | null>(null);
  const [loaded,setLoaded] = useState<{locale: Locale; catalog: Catalog}>({locale:'en',catalog:{}});
  useEffect(() => {
    let active = true;
    void Promise.all([loadMessageCatalog(locale),loadBackendCatalog(locale)]).then(([custom,backend]) => { if (active) setLoaded({locale,catalog:{...backend,...custom}}); }).catch(() => { if (active) setLoaded({locale,catalog:{}}); });
    return () => { active = false; };
  },[locale]);
  useEffect(() => {
    let active = true;
    void CitadelAPI.localizeCatalog(Object.values(officialKeys),locale).then(result => {
      if (active) setGame({locale,catalog:{values:result.values,resolvedLocale:result.locale?.resolvedLocale ?? 'en',fallbackKeys:result.locale?.fallbackKeys}});
    }).catch(() => { if (active) setGame(null); });
    return () => { active = false; };
  },[locale,connection]);
  useEffect(() => {
    // Unconverted page content remains English; converted components mark their own language.
    document.documentElement.lang = 'en';
    document.documentElement.dataset.viewerLocale = locale;
    document.documentElement.dir = locale === 'ar' ? 'rtl' : 'ltr';
  },[locale]);
  const value = useMemo(() => createValue(locale,setLocale,loaded.locale === locale ? loaded.catalog : {},game?.locale === locale ? game.catalog : undefined,connection),[locale,loaded,game,connection]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
export function useLocale() { return useContext(LocaleContext); }
