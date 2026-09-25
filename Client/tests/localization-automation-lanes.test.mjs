import assert from 'node:assert/strict';
import test,{after} from 'node:test';
import fs from 'node:fs';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {automationDetailMessage,automationStatusMessage,automationLaneMessage}=await vite.ssrLoadModule('/src/i18n/automationMessages.ts');
const {formatMessage}=await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const {automationDuration,nextWakeParameters,timedRemainingParameters}=await vite.ssrLoadModule('/src/i18n/automationDuration.ts');
after(()=>vite.close());
test('lane details accept only exact adjacent raw binding and preserve unknown old lockouts',()=>{
 const raw='CRA90 / EUP440 · Player <literal>{0} · 2026-09-23T00:00:00Z';
 const descriptor={key:'fixture.lock',fallback:'Lock: {raw}',params:{raw},fallbackText:raw};
 assert.deepEqual(automationDetailMessage(raw,descriptor),descriptor);
 for(const value of [undefined,{...descriptor,fallbackText:'other'}, {...descriptor,fallbackText:undefined},{...descriptor,params:{raw:Infinity}}])assert.equal(automationDetailMessage(raw,value),undefined);
 assert.equal(formatMessage(automationDetailMessage(raw,descriptor),'de',{}).text,raw);
 assert.equal(descriptor.params.raw,raw);
});
test('known status and lane selectors translate while unknown identities remain untouched',()=>{
 for(const value of ['constructor','toString','__proto__','future_status',' BLOCKED ']){
  assert.equal(automationStatusMessage(value),undefined);
  assert.equal(automationLaneMessage(value),undefined);
 }
 for(const locale of ['de','ar','ja']){
  const catalog=JSON.parse(fs.readFileSync(new URL(`../src/i18n/catalogs/${locale}.json`,import.meta.url)));
  assert.equal(formatMessage(automationStatusMessage('blocked'),locale,catalog).translated,true);
  assert.equal(formatMessage(automationLaneMessage('attacks'),locale,catalog).translated,true);
 }
});
test('countdown retains day/hour/minute granularity and original rounding boundaries',()=>{
 assert.deepEqual(nextWakeParameters(0,1000,'en'),{state:'waiting',duration:'1 min'});
 assert.equal(nextWakeParameters(1000,1000,'en').state,'due');
 const unit=(value,name,locale='en')=>new Intl.NumberFormat(locale,{style:'unit',unit:name,unitDisplay:'short'}).format(value);
 const join=(parts,locale='en')=>new Intl.ListFormat(locale,{style:'short',type:'unit'}).format(parts);
 for(const locale of ['en','de','ar']) {
  assert.equal(automationDuration(59,locale),unit(59,'minute',locale));
  assert.equal(automationDuration(60,locale),unit(1,'hour',locale));
  assert.equal(automationDuration(61,locale),join([unit(1,'hour',locale),unit(1,'minute',locale)],locale));
  assert.equal(automationDuration(1440,locale),unit(1,'day',locale));
  assert.equal(automationDuration(1501,locale),join([unit(1,'day',locale),unit(1,'hour',locale)],locale));
  assert.equal(timedRemainingParameters(1,0,locale).duration,unit(1,'minute',locale));
  assert.equal(timedRemainingParameters(-1,0,locale).duration,unit(1,'minute',locale));
  assert.equal(nextWakeParameters(60_001,0,locale).duration,unit(2,'minute',locale));
 }
});
test('source-owned automation status literals all have explicit translated branches',()=>{
 const directory=new URL('../../Server/Automation/',import.meta.url);
 const states=new Set();
 for(const file of fs.readdirSync(directory).filter(file=>file.endsWith('.go')&&!file.endsWith('_test.go'))){
  const source=fs.readFileSync(new URL(file,directory),'utf8');
  for(const match of source.matchAll(/\b[Ss]tatus\s*(?::|:=|=(?!=))\s*"([a-z_]+)"/g))states.add(match[1]);
 }
 assert.ok(states.has('idle')&&states.has('armed')&&states.has('yielding'));
 for(const state of states)assert.ok(automationStatusMessage(state),state);
});
