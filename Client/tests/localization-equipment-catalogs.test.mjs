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
for(const [module,expected] of [['equipment-modals',63],['activity',32]])test(`complete ${module} module in all25 catalogs retains source provenance and renders every select branch`,()=>{
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
   const contract=richMessageContract(source[key]);
   const options={};
   const visit=nodes=>{for(const node of nodes){if(node.type===TYPE.select)options[node.value]=Object.keys(node.options);if(node.type===TYPE.select||node.type===TYPE.plural)Object.values(node.options).forEach(option=>visit(option.value));if(node.type===TYPE.tag)visit(node.children);}};
   visit(parse(source[key]));
   const defaults={count:2,maximum:5,minimum:1,level:3,id:'00123',name:'Player {0} <b>literal</b>',event:'Event {0}',channel:'Channel {0}',shortcut:'Esc'};
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
