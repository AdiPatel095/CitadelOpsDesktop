import { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { loadOfficialMessages, officialMessageKeysFor, officialCatalogGeneration, subscribeOfficialCatalog } from './officialMessages';
import { useLocale } from './LocaleContext';
import { formatMessage } from './formatMessage';
import type { LocalizedMessage, OfficialCatalog } from './formatMessage';
/** Keep the descriptor in component state so changing viewer locale rerenders existing messages. */
export function useLocalizedMessage(descriptor: LocalizedMessage | undefined, legacyText: string) {
  const {locale,catalog,runtimeStatus} = useLocale();
  const generation = useSyncExternalStore(subscribeOfficialCatalog,officialCatalogGeneration,()=>0);
  const keys = JSON.stringify(officialMessageKeysFor(descriptor));
  const [official,setOfficial] = useState<{identity:string;catalog:OfficialCatalog} | null>(null);
  const identity = `${locale}:${runtimeStatus}:${generation}:${keys}`;
  useEffect(() => {
    let active = true;
    const requested = JSON.parse(keys) as string[];
    if (!requested.length) return;
    void loadOfficialMessages(requested,locale).then(result=>{
      if (active) setOfficial({identity,catalog:result});
    }).catch(()=>{ if(active) setOfficial(null); });
    return ()=>{active=false;};
  },[locale,keys,identity]);
  return useMemo(()=>descriptor ? formatMessage(descriptor,locale,catalog,official?.identity===identity ? official.catalog : undefined) : {text:legacyText,translated:false,resolvedLocale:'en'},[descriptor,legacyText,locale,catalog,official,identity]);
}
