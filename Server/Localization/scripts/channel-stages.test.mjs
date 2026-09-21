// Authorship regression fixtures preserve dispatch/completion vocabulary.
// These checks supplement semantic review; they do not establish linguistic quality.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const source=JSON.parse(fs.readFileSync(path.join(root,'dynamic.json')));
const markers={
 de:['gestartete','abgeschlossene'],fr:['lancé','terminé'],es:['iniciad','completad'],it:['avviat','completat'],pt:['iniciad','concluíd'],nl:['gestarte','voltooid'],
 da:['igangsatte','fuldført'],no:['igangsatte','fullført'],sv:['inledda','slutför'],fi:['käynnistetyt','valmi'],pl:['rozpoczęte','zakończone'],ru:['начатые','завершён'],
 ja:['開始した','完了した'],ko:['시작된','완료된'],'zh-CN':['已发起','已完成'],'zh-TW':['已發起','已完成'],ar:['أُطلقت','المكتملة'],tr:['başlatılan','tamamlanan'],el:['ξεκίνησαν','ολοκληρωμ'],
 cs:['zahájené','dokončené'],sk:['spustené','dokončené'],ro:['lansate','finalizate'],hu:['elindított','befejezett'],bg:['започнати','завършени'],lt:['pradėtos','užbaigt']
};
const launched=['autoadvisor','autoinvasion','autonomad','autotowers','rift'];
const mixed=new Set(['autoadvisor','autonomad','rift']);
const completed=['activity','autoberiworld','autobird','autobooster','autobuyer','autoequipmentcleanup','autofoodbalance','autofortress','autohospital','autokhan','autorecruit','autosceatres','autostation','autostorm','autotci','autotool'];
const key=id=>`server.telemetry.channel.${id}.description`;
function check(locale,pack){
 const [launch,complete]=markers[locale];
 for(const id of launched){
  const text=pack[key(id)].toLocaleLowerCase(locale);
  assert.ok(text.includes(launch),`${locale}/${id}: launched stage missing`);
  if(mixed.has(id)) assert.ok(text.includes(complete),`${locale}/${id}: completed other actions missing`);
  else assert.ok(!text.includes(complete),`${locale}/${id}: dispatch mislabeled as completed`);
 }
 for(const id of completed) assert.ok(pack[key(id)].toLocaleLowerCase(locale).includes(complete),`${locale}/${id}: completed stage missing`);
}
for(const id of launched) assert.ok(source[key(id)].startsWith('Launched '),`${id}: review source stage change`);
for(const id of completed) assert.ok(source[key(id)].startsWith('Completed '),`${id}: review source stage change`);
for(const locale of Object.keys(markers)) check(locale,JSON.parse(fs.readFileSync(path.join(root,'locales',locale+'.json'))));
const bad=JSON.parse(fs.readFileSync(path.join(root,'locales/de.json')));
bad[key('autotowers')]=bad[key('autotowers')].replace('Gestartete','Abgeschlossene');
assert.throws(()=>check('de',bad),/launched stage missing/);
console.log('525 channel stage fixtures pass; dispatch-as-completion negative fixture rejected.');
