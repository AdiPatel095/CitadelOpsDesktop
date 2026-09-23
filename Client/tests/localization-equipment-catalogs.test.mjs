import test,{after} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
import {parse,TYPE} from '@formatjs/icu-messageformat-parser';
import {createElement} from 'react';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {formatMessage}=await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const {renderRichMessage,richMessageContract}=await vite.ssrLoadModule('/src/i18n/RichMessage.ts');
const {localeCodes}=await vite.ssrLoadModule('/src/i18n/locales.ts');
after(()=>vite.close());
const read=path=>JSON.parse(fs.readFileSync(new URL(path,import.meta.url)));
const source=read('../localization/ui.en.json');
const provenance=read('../localization/module-authorship.json');
const hash=value=>createHash('sha256').update(value).digest('hex');
for(const [module,expected] of [['equipment-modals',63],['activity',32],['battle',66],['events',41],['automation-lanes',11]])test(`complete authored ${module} group in all25 catalogs retains source provenance and renders every select branch`,()=>{
 let rendered=0;
 for(const locale of localeCodes.filter(code=>code!=='en')) {
  const filename=`${module}.${locale}.json`;
  const authored=read(`../localization/authored/${filename}`);
  const catalog=read(`../src/i18n/catalogs/${locale}.json`);
  assert.equal(Object.keys(authored).length,expected);
  for(const [short,value] of Object.entries(authored)) {
   const key=Object.hasOwn(source,short)?short:`ui.equipment.components.equipmentModals.${short}`;
   assert.equal(catalog[key],value);
   assert.equal(provenance[filename].sourceHashes[key],hash(source[key]));
   if(['events','automation-lanes'].includes(module))assert.equal(provenance[filename].translationHashes[key],hash(value));
   const contract=richMessageContract(source[key]);
   const options={};
   const visit=nodes=>{for(const node of nodes){if(node.type===TYPE.select)options[node.value]=Object.keys(node.options);if(node.type===TYPE.select||node.type===TYPE.plural)Object.values(node.options).forEach(option=>visit(option.value));if(node.type===TYPE.tag)visit(node.children);}};
   visit(parse(source[key]));
   const defaults={duration:'1 day 2 hours',feature:'Auto Khan <literal>',minutes:1234,query:'Player {0}',board:'001',month:'September 2026',range:'10–20',date:'2026-09-01',page:2,pages:3,first:1,last:10,total:23,count:2,maximum:5,minimum:1,level:3,id:'00123',name:'Player {0} <b>literal</b>',event:'Event {0}',channel:'Channel {0}',shortcut:'Esc',amount:1234,changed:12,number:3,visible:2,filtered:4,parsed:5,attacker:'Player {0}',defender:'Other <b>literal</b>'};
   let variants=[{}];
   for(const [argument,values] of Object.entries(options))variants=variants.flatMap(previous=>values.map(value=>({...previous,[argument]:value})));
   for(const variant of variants)for(const count of [0,1,2,5,21,1.5]) {
    const params=Object.fromEntries(contract.arguments.map(arg=>[arg,variant[arg]??(arg==='count'?count:defaults[arg])]));
    assert.ok(Object.values(params).every(value=>value!==undefined),key);
    const descriptor={key,fallback:source[key],params};
    const result=contract.tags.length?renderRichMessage(descriptor,locale,catalog,Object.fromEntries(contract.tags.map(tag=>[tag,children=>createElement('bdi',null,...children)]))):formatMessage(descriptor,locale,catalog);
    assert.equal(result.translated,true,`${locale}:${key}:${JSON.stringify(params)}`);
    rendered++;
   }
  }
 }
 assert.ok(rendered>=expected*25*6);
});
