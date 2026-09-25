import assert from 'node:assert/strict';
import test from 'node:test';
import {createOfficialCatalogLoader} from '../src/i18n/officialCatalogCache.ts';
test('official requests batch and deduplicate keys without truncating beyond 5000',async()=>{
 const calls=[];
 const loader=createOfficialCatalogLoader(async(keys,locale,scope)=>{calls.push({keys,locale,scope});return {values:Object.fromEntries(keys.map(key=>[key,`fr:${key}`])),locale:{resolvedLocale:locale}};});
 const keys=Array.from({length:5001},(_,i)=>`key${i}`);
 const [first,second]=await Promise.all([loader.load(keys,'fr','/one'),loader.load(['key1'],'fr','/one')]);
 assert.deepEqual(calls.map(call=>call.keys.length),[5000,1]);assert.equal(Object.keys(first.values).length,5001);assert.equal(second.values.key1,'fr:key1');
 await loader.load(['key1'],'fr','/two');assert.equal(calls.length,3);assert.equal(calls[2].scope,'/two');
});
test('English fallback and absent values are retried instead of permanently cached',async()=>{
 let calls=0;
 const loader=createOfficialCatalogLoader(async()=>{calls++;return calls===1 ? {values:{x:'English'},locale:{resolvedLocale:'en'}} : {values:{x:'Français'},locale:{resolvedLocale:'fr'}};});
 assert.deepEqual((await loader.load(['x'],'fr','/')).fallbackKeys,['x']);
 assert.equal((await loader.load(['x'],'fr','/')).values.x,'Français');assert.equal(calls,2);
});
test('invalidating while requests are pending never strands or merges waiters',async()=>{
 const pending=[];
 const loader=createOfficialCatalogLoader((keys,locale)=>new Promise(resolve=>pending.push(()=>resolve({values:{[keys[0]]:`${locale}:${pending.length}`},locale:{resolvedLocale:locale}}))));
 const old=loader.load(['x'],'fr','/');loader.clear();const fresh=loader.load(['x'],'fr','/');
 await Promise.resolve();assert.equal(pending.length,2);pending.forEach(resolve=>resolve());
 assert.equal((await old).resolvedLocale,'fr');assert.equal((await fresh).resolvedLocale,'fr');
});
