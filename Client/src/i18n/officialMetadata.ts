import type { OfficialCatalog } from './formatMessage';
const provenance = Symbol('official-translation-provenance');
export type TranslationValues = Record<string,string> & {[provenance]?:OfficialCatalog};
export function translationValues(catalog: OfficialCatalog): TranslationValues { return {...catalog.values,[provenance]:catalog}; }
export function metadataName(row: Record<string,unknown>,values:Record<string,string>,fallback:string,preferredKeys:readonly string[] = []) {
  const keys = [...preferredKeys];
  for (const value of [row.type,row.name,row.Name,row.JSONKey,row.kingdomName,row.comment2]) {
    if (typeof value === 'string' && value.trim()) keys.push(`kingdomName_${value}`,`${value}_name`,value);
  }
  const catalog=(values as TranslationValues)[provenance];
  for(const key of keys) {
    if (!values[key]?.trim()) continue;
    const official=!!catalog && !(catalog.fallbackKeys ?? []).includes(key);
    return {name:values[key],localizationKey:key,nameLocale:official ? catalog.resolvedLocale : 'en',translationStatus:official ? 'official' as const : 'fallback' as const};
  }
  const name=[row._display_name,row.type,row.name,row.Name,row.kingdomName,row.comment2].find((value):value is string=>typeof value==='string'&&!!value.trim()) ?? fallback;
  return {name,nameLocale:'en',translationStatus:'fallback' as const};
}
