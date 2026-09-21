import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import ts from 'typescript';
import { pathToFileURL } from 'node:url';
const files = ['locales','formatMessage','gameMessage'];
const modules = {};
for (const file of files) {
 const target = new URL(`../node_modules/.localization-${file}.mjs`,import.meta.url);
 fs.writeFileSync(target,ts.transpileModule(fs.readFileSync(new URL(`../src/i18n/${file}.ts`,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText);
 modules[file] = await import(pathToFileURL(target.pathname));
 fs.unlinkSync(target);
}
test('normalizes official and regional locales without accepting unsupported languages',()=>{
 const {normalizeLocale,locales}=modules.locales;
 assert.equal(locales.length,26); assert.equal(normalizeLocale('zh_tw'),'zh-TW'); assert.equal(normalizeLocale('zh-Hant-HK'),'zh-TW'); assert.equal(normalizeLocale('ar-SA'),'ar'); assert.equal(normalizeLocale('nb-NO'),'no'); assert.equal(normalizeLocale('xx'),undefined);
});
test('ICU plurals, literal HTML, placeholders and untranslated provenance remain distinct',()=>{
 const {formatMessage,validateMessageCatalog}=modules.formatMessage;
 const fallback='{count, plural, one {# item} other {# items}}';
 assert.deepEqual(formatMessage({key:'items',fallback,params:{count:2}},'ar',{}),{text:'2 items',translated:false,resolvedLocale:'en'});
 assert.equal(formatMessage({key:'x',fallback:'',params:{name:'<b>{count}</b>'}},'fr',{x:'Bonjour {name}'}).text,'Bonjour <b>{count}</b>');
 assert.deepEqual(validateMessageCatalog({x:'{n, plural, one {# item} other {# items}}'},{x:'{n, plural, one {# objet} other {# objets}}'}),[]);
 assert.equal(validateMessageCatalog({x:'{n}'},{x:'{m}'}).length,1);
 assert.equal(validateMessageCatalog({x:'Text'},{y:'Text'}).length,2);
 assert.equal(formatMessage({key:'x',fallback:'Safe fallback'},'fr',{x:'{broken'}).text,'Safe fallback');
 assert.equal(formatMessage({key:'x',fallback:'Missing {name}'},'xx',{}).translated,false);
});
test('official positional placeholders never recursively process inserted values or HTML',()=>{
 assert.equal(modules.gameMessage.formatGameMessage('+{0}% for {1} fields',['{1}',2]),'+{1}% for 2 fields');
 assert.equal(modules.gameMessage.formatGameMessage('<b>{0}</b> {2}', ['Name']),'<b>Name</b> {2}');
});
