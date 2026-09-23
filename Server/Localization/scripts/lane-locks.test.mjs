import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
const {IntlMessageFormat}=await import(pathToFileURL(path.resolve(process.argv[2])).href);
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const source=JSON.parse(fs.readFileSync(path.join(root,'en.json')));
const keys=Object.keys(source).filter(k=>k.startsWith('server.state.safety_lock.')||k.startsWith('server.buildings.ruby_')||['server.intent.safety_lock_released','server.intent.safety_lock_cleared','server.gamedata.ruby_purchase_confirmation','server.automation.ruby_upgrade_notice'].includes(k));
assert.equal(keys.length,11);
let count=0;
for(const file of fs.readdirSync(path.join(root,'locales')).filter(f=>f.endsWith('.json')&&f!=='provenance.json')) {
 const locale=file.slice(0,-5),pack=JSON.parse(fs.readFileSync(path.join(root,'locales',file)));
 const values={opcode:'EUP',code:'12345',until:'2026-09-23T12:30:00Z',cost:3100,threshold:2500,definitionID:'12345',reason:'literal reason {raw}'};
 for(const amount of [0,1,2,5,21]) { values.cost=amount;values.threshold=amount;
 for(const key of keys){
  assert.ok(pack[key]);assert.notEqual(pack[key],source[key]);
  const out=new IntlMessageFormat(pack[key],locale).format(values);
  for(const name of ['opcode','code','until','definitionID','reason'])if(source[key].includes('{'+name+'}'))assert.ok(out.includes(values[name]),`${locale}: ${name} altered`);
  if(source[key].includes('{cost, number}'))assert.ok(out.includes(new Intl.NumberFormat(locale).format(values.cost)));
  if(source[key].includes('{threshold, number}'))assert.ok(out.includes(new Intl.NumberFormat(locale).format(values.threshold)));
  count++;
 }
}
}
console.log(`Rendered ${count} lane lock templates; exact protocol IDs/deadlines and typed amounts preserved.`);
