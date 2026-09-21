import assert from 'node:assert/strict';
import {after,test} from 'node:test';
import fs from 'node:fs';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {loadBundledOfficial,mergeOfficialCatalogs}=await vite.ssrLoadModule('/src/i18n/bundledOfficial.ts');
const {formatMessage}=await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const {localeCodes}=await vite.ssrLoadModule('/src/i18n/locales.ts');
const {officialMessageKeys}=await vite.ssrLoadModule('/src/i18n/officialKeys.ts');
after(()=>vite.close());
test('all viewer locales have exact provenance-backed offline official controls',async()=>{
 const provenance=JSON.parse(fs.readFileSync(new URL('../localization/bundled-official-provenance.json',import.meta.url)));
 for(const locale of localeCodes) {
  const bytes=fs.readFileSync(new URL(`../src/i18n/officialBundled/${locale}.json`,import.meta.url));
  assert.equal(createHash('sha256').update(bytes).digest('hex'),provenance.locales[locale].subsetSha256);
  const catalog=await loadBundledOfficial(locale);
  for(const key of Object.values(officialMessageKeys))assert.ok(catalog.values[key],`${locale}:${key}`);
  const result=formatMessage({key:'game.cancel',officialKey:'cancel',fallback:'Cancel'},locale,{},catalog);
  assert.equal(result.translated,true);assert.equal(result.resolvedLocale,locale);
 }
});
test('live English fallback cannot mask offline requested-language values',async()=>{
 const offline=await loadBundledOfficial('de');
 const fallback=mergeOfficialCatalogs('de',offline,{resolvedLocale:'en',values:{cancel:'Cancel',unknown:'Unknown'}});
 assert.equal(fallback.values.cancel,offline.values.cancel);assert.deepEqual(fallback.fallbackKeys,['unknown']);
 const updated=mergeOfficialCatalogs('de',offline,{resolvedLocale:'de',values:{cancel:'New official value',unknown:'Unknown'},fallbackKeys:['unknown']});
 assert.equal(updated.values.cancel,'New official value');assert.deepEqual(updated.fallbackKeys,['unknown']);
});
