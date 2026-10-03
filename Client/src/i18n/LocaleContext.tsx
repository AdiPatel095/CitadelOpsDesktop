import { createValue, LocaleContext } from './useLocale';
import {loadBundledOfficial,mergeOfficialCatalogs} from './bundledOfficial';
import { loadServerCatalog } from './serverCatalog';
import { invalidateOfficialMessages } from './officialMessages';
import { loadBackendCatalog } from './backendCatalog';
import { officialMessageKeys, officialMessageNouns } from './officialKeys';
import { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { readViewerLocale, readViewerLocalePreference, setViewerLocale, subscribeViewerLocale } from './viewerLocaleStore';
import type { Locale } from './locales';
import { CitadelAPI } from '../api/CitadelClient';
import type { Catalog, OfficialCatalog } from './formatMessage';
import { loadMessageCatalog } from './catalogs';
import type { MessageKey } from './messages';
const officialKeys: Partial<Record<MessageKey,string>> = officialMessageKeys;
export function LocaleProvider({children}: {children: React.ReactNode}) {
  const locale = useSyncExternalStore(subscribeViewerLocale,readViewerLocale,()=> 'en' as Locale);
  const preference = useSyncExternalStore(subscribeViewerLocale,readViewerLocalePreference,()=> 'auto' as const);
  const setLocale = setViewerLocale;
  const [connection,setConnection] = useState('Disconnected');
  useEffect(() => CitadelAPI.subscribeStatus(status => { invalidateOfficialMessages(); setConnection(status); }),[]);
  const [game,setGame] = useState<{locale: Locale; catalog: OfficialCatalog} | null>(null);
  const [bundled,setBundled] = useState<{locale:Locale;catalog:OfficialCatalog}|null>(null);
  useEffect(()=>{let active=true;void loadBundledOfficial(locale).then(catalog=>{if(active)setBundled({locale,catalog});}).catch(()=>{if(active)setBundled(null);});return ()=>{active=false;};},[locale]);
  const [loaded,setLoaded] = useState<{locale: Locale; catalog: Catalog}>({locale:'en',catalog:{}});
  useEffect(() => {
    let active = true;
    void Promise.allSettled([loadMessageCatalog(locale),loadBackendCatalog(locale),loadServerCatalog(locale)]).then(([custom,backend,server]) => {
      const catalog = (result: PromiseSettledResult<Catalog>) => result.status === 'fulfilled' ? result.value : {};
      if (active) setLoaded({locale,catalog:{...catalog(server),...catalog(backend),...catalog(custom)}});
    });
    return () => { active = false; };
  },[locale]);
  useEffect(() => {
    let active = true;
    void CitadelAPI.localizeCatalog([...Object.values(officialKeys),...Object.values(officialMessageNouns).flatMap(nouns=>Object.values(nouns).map(noun=>noun.key))],locale).then(result => {
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
  const value = useMemo(() => createValue(locale,setLocale,loaded.locale === locale ? loaded.catalog : {},mergeOfficialCatalogs(locale,bundled?.locale===locale?bundled.catalog:undefined,game?.locale === locale ? game.catalog : undefined),connection,preference),[locale,preference,loaded,game,bundled,connection]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
