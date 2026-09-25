import { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { loadOfficialMessages, officialMessageKeysFor, officialCatalogGeneration, subscribeOfficialCatalog } from './officialMessages';
import { useLocale } from './LocaleContext';
import { formatMessage } from './formatMessage';
import type { LocalizedMessage, OfficialCatalog } from './formatMessage';
export type MessageInput = { descriptor?: LocalizedMessage; legacyText: string };
/** One deduplicated official lookup for a list; stale locale results never render. */
export function useLocalizedMessages(inputs: readonly MessageInput[]) {
  const { locale, catalog, runtimeStatus } = useLocale();
  const generation = useSyncExternalStore(subscribeOfficialCatalog, officialCatalogGeneration, () => 0);
  const keys = JSON.stringify([...new Set(inputs.flatMap(input => officialMessageKeysFor(input.descriptor)))].sort());
  const identity = `${locale}:${runtimeStatus}:${generation}:${keys}`;
  const [official, setOfficial] = useState<{ identity: string; catalog: OfficialCatalog } | null>(null);
  useEffect(() => {
    let active = true;
    const requested = JSON.parse(keys) as string[];
    if (!requested.length) return;
    void loadOfficialMessages(requested, locale).then(result => {
      if (active) setOfficial({ identity, catalog: result });
    }).catch(() => { if (active) setOfficial(null); });
    return () => { active = false; };
  }, [locale, keys, identity]);
  return useMemo(() => inputs.map(input => input.descriptor
    ? formatMessage(input.descriptor, locale, catalog, official?.identity === identity ? official.catalog : undefined)
    : { text: input.legacyText, translated: false, resolvedLocale: 'en' }), [inputs, locale, catalog, official, identity]);
}
