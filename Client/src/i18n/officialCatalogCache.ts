import type { OfficialCatalog } from './formatMessage';
export type OfficialResponse = {values: Record<string,string>; locale?: {resolvedLocale:string; fallbackKeys?:string[]}};
type Request = (keys:string[],locale:string,scope:string)=>Promise<OfficialResponse>;
/** Batch concurrent consumers without mixing viewers, runtimes, or languages. */
export function createOfficialCatalogLoader(request: Request) {
  let generation = 0;
  const cache = new Map<string,Promise<{value?:string;fallback:boolean}>>();
  const batches = new Map<string,{locale:string;scope:string;items:Map<string,{resolve:(value:{value?:string;fallback:boolean})=>void;reject:(error:unknown)=>void}>}>();
  function load(keys: readonly string[],locale:string,scope:string): Promise<OfficialCatalog> {
    const revision = generation;
    const group = JSON.stringify([revision,scope,locale]);
    const unique = [...new Set(keys)].sort();
    const pending = unique.map(key=>{
      const id = JSON.stringify([revision,scope,locale,key]);
      if (!cache.has(id)) {
        const promise = new Promise<{value?:string;fallback:boolean}>((resolve,reject)=>{
          let batch = batches.get(group);
          if (!batch) {
            batch = {locale,scope,items:new Map()}; batches.set(group,batch);
            queueMicrotask(async()=>{
              const next = batches.get(group); if (!next) return;
              batches.delete(group);
              const entries = [...next.items.entries()];
              for (let index=0;index<entries.length;index+=5000) {
                const chunk = entries.slice(index,index+5000);
                try {
                  const response=await request(chunk.map(([key])=>key),locale,scope);
                  for(const [key,waiter] of chunk) {
                    const fallback = response.locale?.resolvedLocale!==locale || !!response.locale?.fallbackKeys?.includes(key);
                    if (fallback || response.values[key] === undefined) cache.delete(JSON.stringify([revision,scope,locale,key]));
                    waiter.resolve({value:response.values[key],fallback});
                  }
                } catch(error) {
                  for(const [key,waiter] of chunk) {cache.delete(JSON.stringify([revision,scope,locale,key]));waiter.reject(error);}
                }
              }
            });
          }
          batch.items.set(key,{resolve,reject});
        });
        cache.set(id,promise);
      }
      return cache.get(id)!;
    });
    return Promise.all(pending).then(values=>({values:Object.fromEntries(unique.flatMap((key,index)=>values[index].value===undefined ? [] : [[key,values[index].value!]])),resolvedLocale:locale,fallbackKeys:unique.filter((_,index)=>values[index].fallback)}));
  }
  return {load,clear:()=>{ generation += 1; cache.clear(); }};
}
