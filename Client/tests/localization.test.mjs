import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import ts from 'typescript';
import { pathToFileURL } from 'node:url';
const files = ['locales','formatMessage','gameMessage','officialKeys','sourceMessages'];
const modules = {};
for (const file of files) {
 const target = new URL(`../node_modules/.localization-${file}.mjs`,import.meta.url);
 fs.writeFileSync(target,ts.transpileModule(fs.readFileSync(new URL(`../src/i18n/${file}.ts`,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText);
 modules[file] = await import(pathToFileURL(target.pathname));
 fs.unlinkSync(target);
}
test('normalizes official and regional locales without accepting unsupported languages',()=>{
 const {normalizeLocale,locales}=modules.locales;
 assert.equal(locales.find(item=>item.code==='zh-CN').gameCode,'zh_CN'); assert.equal(locales.find(item=>item.code==='zh-TW').gameCode,'zh_TW');
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
test('authored locale subsets validate ICU and reject unknown keys; completeness belongs to the strict release gate',()=>{
 const source = fs.readFileSync(new URL('../src/i18n/messages.ts',import.meta.url),'utf8');
 const ast=ts.createSourceFile('messages.ts',source,ts.ScriptTarget.Latest,true);
 let english={...modules.sourceMessages.sourceMessages};
 function visit(node) { if(ts.isVariableDeclaration(node)&&node.name.getText(ast)==='messages') { const object=node.initializer.expression; english={...english,...Object.fromEntries(object.properties.filter(ts.isPropertyAssignment).map(property=>[property.name.text,property.initializer.text]))}; } ts.forEachChild(node,visit); }
 visit(ast);
 for (const key of Object.keys(modules.officialKeys.officialMessageKeys)) delete english[key];
 for(const locale of modules.locales.localeCodes.filter(code=>code!=='en')) {
  const catalog=JSON.parse(fs.readFileSync(new URL(`../src/i18n/catalogs/${locale}.json`,import.meta.url),'utf8'));
  assert.deepEqual(modules.formatMessage.validateAuthoredMessageSubset(english,catalog),[],locale);
 }
});
test('official game messages and nested nouns preserve exact user values and fallback coverage',()=>{
 const {formatMessage}=modules.formatMessage;
 const game={values:{castle:'Château',move:'Déplacer {0}'},resolvedLocale:'fr'};
 assert.equal(formatMessage({key:'x',fallback:'Move',officialKey:'move',officialParams:{0:'<User>{1}'}},'fr',{},game).text,'Déplacer <User>{1}');
 assert.equal(formatMessage({key:'x',fallback:'The {noun}',gameParams:{noun:{key:'castle',fallback:'Castle'}}},'fr',{x:'Le {noun}'},game).text,'Le Château');
 assert.equal(formatMessage({key:'x',fallback:'The {noun}',gameParams:{noun:{key:'missing',fallback:'Castle'}}},'fr',{x:'Le {noun}'},game).translated,false);
});

test('navigation game terms use verified semantic official keys',()=>{
 assert.deepEqual(Object.fromEntries(Object.entries(modules.officialKeys.officialMessageKeys).filter(([key])=>key.startsWith('navigation.'))),{'navigation.castle':'castle','navigation.equipment':'dialog_equipment_title','navigation.movement':'dialog_recuit_generals','navigation.rift':'event_title_133'});
});

test('structured error context preserves scalar identifiers and reports missing context translation',()=>{
 const message={key:'bad',fallback:'Invalid value',context:[{key:'section',fallback:'Section {section}',params:{section:'autoTower'}}]};
 assert.equal(modules.formatMessage.formatMessage(message,'fr',{bad:'Valeur incorrecte',section:'Section {section}'}).text,'Section autoTower: Valeur incorrecte');
 assert.equal(modules.formatMessage.formatMessage(message,'fr',{bad:'Valeur incorrecte'}).translated,false);
});

test('missing official parameters and mixed fallback never claim full translation',()=>{
 const {formatMessage}=modules.formatMessage;
 const game={values:{move:'Déplacer {0}',castle:'Château'},resolvedLocale:'fr'};
 assert.deepEqual(formatMessage({key:'move',fallback:'Could not move',officialKey:'move'},'fr',{},game),{text:'Could not move',translated:false,resolvedLocale:'en'});
 assert.equal(formatMessage({key:'x',fallback:'The {noun}',gameParams:{noun:{key:'castle',fallback:'Castle'}}},'fr',{},game).resolvedLocale,'mixed');
 assert.equal(formatMessage({key:'x',fallback:'Static source',fallbackText:'Player {x} failed'},'fr',{},game).text,'Player {x} failed');
});

test('complete legacy fallback bypasses context rather than duplicating it',()=>{
 const message={key:'done',fallback:'Completed',fallbackText:'Completed action for Player {name}',context:[{key:'action',fallback:'Action for {name}',params:{name:'Player {name}'}}]};
 assert.equal(modules.formatMessage.formatMessage(message,'en',{}).text,message.fallbackText);
 assert.equal(modules.formatMessage.formatMessage(message,'fr',{action:'Action pour {name}'}).text,message.fallbackText);
 assert.equal(modules.formatMessage.formatMessage(message,'fr',{done:'Terminé'}).translated,false);
});

test('Arabic argument isolation preserves select keys and unchanged mixed-script values',()=>{
 const params={kind:'player',name:'Player-7 {x}',x:123};
 const result=modules.formatMessage.formatMessage({key:'a',fallback:'Player {name}',params},'ar',{a:'{kind, select, player {اللاعب {name} عند {x, number}} other {آخر}}'});
 assert.equal(result.translated,true);assert.ok(result.text.includes('\u2068Player-7 {x}\u2069'));assert.ok(result.text.includes(`\u2068${new Intl.NumberFormat('ar').format(123)}\u2069`));assert.equal(params.name,'Player-7 {x}');assert.equal(params.kind,'player');
});

test('authored subset validation does not weaken strict missing-key rejection',()=>{
 assert.deepEqual(modules.formatMessage.validateAuthoredMessageSubset({a:'A',b:'B'},{a:'Un'}),[]);
 assert.deepEqual(modules.formatMessage.validateMessageCatalog({a:'A',b:'B'},{a:'Un'}),['Missing message: b']);
 assert.deepEqual(modules.formatMessage.validateAuthoredMessageSubset({a:'A'},{other:'Other'}),['Unknown message: other']);
});
