import type {Locale} from './locales';
import type {Catalog, OfficialCatalog} from './formatMessage';
const loaders = import.meta.glob<{default:Catalog}>('./officialBundled/*.json');
/** Small, provenance-checked official v4357 subset for disconnected application controls. */
export async function loadBundledOfficial(locale:Locale):Promise<OfficialCatalog> {
  const loader=loaders[`./officialBundled/${locale}.json`];
  return {values:loader?(await loader()).default:{},resolvedLocale:locale};
}
export function mergeOfficialCatalogs(locale:Locale,bundled?:OfficialCatalog,live?:OfficialCatalog):OfficialCatalog|undefined {
  if(!bundled)return live;
  const values={...bundled.values};
  const fallbackKeys:string[]=[];
  for(const [key,value] of Object.entries(live?.values??{})) {
    const translated=live?.resolvedLocale===locale && !live.fallbackKeys?.includes(key);
    if(translated || !Object.hasOwn(values,key)) {
      values[key]=value;
      if(!translated)fallbackKeys.push(key);
    }
  }
  return {values,resolvedLocale:locale,fallbackKeys};
}
