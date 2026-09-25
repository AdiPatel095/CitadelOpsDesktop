import {parseMessageDescriptor} from './messageDescriptor';
import { CitadelAPI } from '../api/CitadelClient';
import { runtimeBasePath } from '../api/RuntimeURL';
import { createOfficialCatalogLoader } from './officialCatalogCache';
import type { LocalizedMessage } from './formatMessage';
const loader = createOfficialCatalogLoader((keys,locale,scope)=>CitadelAPI.localizeCatalog(keys,locale,scope));
export function officialMessageKeysFor(descriptor?: LocalizedMessage): string[] {
  if(descriptor?.listParams!==undefined)descriptor=parseMessageDescriptor(descriptor);
  return [...new Set([descriptor,...descriptor?.context ?? [],...Object.values(descriptor?.listParams??{}).flat()].flatMap(item=>item ? [item.officialKey,...Object.values(item.gameParams ?? {}).map(noun=>noun.key)].filter((key):key is string=>!!key) : []))].sort();
}
export function loadOfficialMessages(keys:readonly string[],locale:string) { return loader.load(keys,locale,runtimeBasePath()); }
let generation = 0;
const listeners = new Set<()=>void>();
export function officialCatalogGeneration() { return generation; }
export function subscribeOfficialCatalog(notify:()=>void) { listeners.add(notify); return ()=>{listeners.delete(notify);}; }
export function invalidateOfficialMessages() { loader.clear(); generation += 1; for(const notify of listeners) notify(); }
