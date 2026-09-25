import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
const {IntlMessageFormat}=await import(pathToFileURL(path.resolve(process.argv[2])).href);
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const source=JSON.parse(fs.readFileSync(path.join(root,'en.json')));
const keys=Object.keys(source).filter(k=>k.startsWith('server.storm.')||k==='server.automation.invasion_recovery_exhausted');
assert.equal(keys.length,20);
let renders=0;
for(const file of fs.readdirSync(path.join(root,'locales')).filter(f=>f.endsWith('.json')&&f!=='provenance.json')) {
 const locale=file.slice(0,-5),pack=JSON.parse(fs.readFileSync(path.join(root,'locales',file)));
 const render=(key,values)=>new IntlMessageFormat(pack[key],locale).format(values);
 const purchases=new Intl.ListFormat(locale,{style:'long',type:'conjunction'}).format(['245','3119'].map(packageID=>render('server.storm.purchase_list_item',{packageID,amount:2})));
 const castle=render('server.storm.castle_name',{name:'Literal <Name>{1}'});
 const values={purchases,castle,cost:1234,amount:2,packageID:'12345',id:'12345',name:'Literal <Name>{1}',ordinal:2,total:3,explanation:'Literal reason {2}'};
 for(const key of keys) {
  assert.ok(pack[key],`${locale}: missing ${key}`);
  if(key!=='server.storm.castle_name')assert.notEqual(pack[key],source[key],`${locale}: copied English`);
  const output=render(key,values);assert.equal(typeof output,'string');
  if(source[key].includes('{castle}'))assert.ok(output.includes(values.name),`${locale}: literal name changed`);
  if(source[key].includes('{packageID}'))assert.ok(output.includes('12345'),`${locale}: grouped package ID`);
  if(source[key].includes('{explanation}'))assert.ok(output.includes(values.explanation),`${locale}: explanation changed`);
  renders++;
 }
 assert.notEqual(pack['server.storm.partial'],pack['server.storm.unconfirmed']);
 assert.notEqual(pack['server.storm.failed'],pack['server.storm.completed']);
}
console.log(`Rendered ${renders} Storm messages with locale lists, literal names/IDs and distinct outcomes.`);
