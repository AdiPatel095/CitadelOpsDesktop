import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
const formatterPath=process.argv[2];
if(!formatterPath) throw new Error('Pass the installed intl-messageformat/index.js path');
const {IntlMessageFormat}=await import(pathToFileURL(path.resolve(formatterPath)).href);
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const source=JSON.parse(fs.readFileSync(path.join(root,'en.json')));
const keys=Object.keys(source).filter(key=>key.startsWith('server.intent.')&&!key.startsWith('server.intent.description.'));
assert.equal(keys.length,45,'Review new intent messages and extend the complete authored module');
// Authorship review fixtures: these are deliberately local vocabulary assertions,
// not a claim that a substring check establishes full linguistic correctness.
// Columns: partial outcome, unconfirmed outcome, blocked additional purchase,
// no purchase until complete, explicit lock review, official commander, feast.
const vocabulary={
 de:['teilweise','nicht bestätigt','blockiert weiterhin','erst, wenn','Prüfe diese Ablehnung','Feldherr','Fest'],
 fr:['en partie','pas pu confirmer','continuera de bloquer','tant que la réponse sera incomplète','Examinez ce refus','commandant','fête'],
 es:['parcialmente','No pudimos confirmar','seguirá bloqueando','no comprará hasta','Revisa este rechazo','comandante','festín'],
 it:['in parte','Non è stato possibile confermare','continueranno a bloccare','non acquisteranno finché','Esamina questo rifiuto','comandante','banchetto'],
 pt:['parcialmente','Não foi possível confirmar','continuarão bloqueando','não comprarão até','Analise esta rejeição','comandante','banquete'],
 nl:['gedeeltelijk','niet bevestigen','blijft een volgende','koopt niets totdat','Beoordeel deze weigering','commandant','festival'],
 da:['delvist','ikke bekræfte','fortsat blokere','køber ikke, før','Gennemgå denne afvisning','kommandør','fest'],
 no:['delvis','ikke bekrefte','fortsatt blokkere','kjøper ikke før','Gjennomgå denne avvisningen','kommandør','festival'],
 sv:['delvis','inte bekräfta','fortsätter att blockera','köper inget förrän','Granska detta avslag','anförare','fest'],
 fi:['osittain','Emme voineet vahvistaa','estää edelleen','eikä osta mitään, ennen','Arvioi tämä hylkäys','komentaja','kestien'],
 pl:['częściowo','Nie udało się potwierdzić','będą blokować','nie dokonają zakupu, dopóki','Przeanalizuj tę odmowę','dowódc','święta'],
 ru:['частично','Не удалось подтвердить','продолжат блокировать','не будут покупать, пока','Разберите этот отказ','военачальник','пира'],
 ar:['جزئيًا','تعذّر علينا تأكيد','ستواصل','لن تشتري حتى','راجع هذا الرفض','قائد','الوليمة'],
 ja:['一部のみ','確認できません','引き続きブロック','完全になるまで購入しません','拒否内容を確認','司令官','宴会'],
 ko:['일부만','확인할 수 없었습니다','계속 차단','완전해질 때까지 구매하지','거부 내용을 검토','지휘관','잔치'],
 'zh-CN':['一部分','无法确认','继续阻止','完整之前不会购买','审查此次拒绝','指挥官','宴会'],
 'zh-TW':['一部分','無法確認','繼續阻止','完整之前不會購買','審查此次拒絕','指揮官','宴會'],
 tr:['kısmen','doğrulayamadık','engellemeye devam','satın alım yapmayacak','bu reddi inceleyin','komutan','ziyafet'],
 el:['εν μέρει','Δεν μπορέσαμε να επιβεβαιώσουμε','συνεχίσουν να εμποδίζουν','δεν θα αγοράσουν μέχρι','Εξετάστε αυτήν την απόρριψη','διοικητή','γιορτής'],
 cs:['částečně','Nepodařilo se potvrdit','dál blokovat','nenakoupí, dokud','prověřte toto odmítnutí','velitel','slavnosti'],
 sk:['čiastočne','Nepodarilo sa potvrdiť','ďalej blokovať','nenakúpia, kým','preskúmajte toto odmietnutie','veliteľ','hostiny'],
 ro:['parțial','Nu am putut confirma','continua să blocheze','nu vor cumpăra până','Examinați acest refuz','comandant','festin'],
 hu:['részben','Nem sikerült megerősíteni','továbbra is tiltja','nem vásárol, amíg','vizsgálja meg ezt az elutasítást','parancsnok','lakoma'],
 bg:['частично','Не успяхме да потвърдим','продължат да блокират','няма да купуват, докато','Прегледайте този отказ','командир','фестивал'],
 lt:['iš dalies','Nepavyko patvirtinti','toliau blokuos','nepirks, kol','peržiūrėkite šį atmetimą','vad','šventės'],
};
const targetKeys=[
 'server.intent.action_partial','server.intent.action_unconfirmed',
 'server.intent.auto_buyer_will_keep.60086d0a','server.intent.auto_buyer_will_retry.7675c4dc',
 'server.intent.safety_lock.review_recovery','server.intent.assign_commander',
 'server.intent.the_game_has_not.b47d0d28',
];
function check(pack,locale) {
 vocabulary[locale].forEach((phrase,index)=>assert.ok(pack[targetKeys[index]]?.includes(phrase),`${locale}: safety/terminology fixture ${targetKeys[index]}`));
 const timed=pack['server.intent.the_lane_automatically_becomes.544b1dee'];
 const manual=pack['server.intent.safety_lock.review_recovery'];
 assert.match(timed,/30/,`${locale}: timed eligibility duration changed`);
 assert.doesNotMatch(manual,/30/,`${locale}: manual review must not promise timed recovery`);
 assert.notEqual(pack['server.intent.action_partial'],pack['server.intent.action_unconfirmed']);
}
let rendered=0;
for(const locale of Object.keys(vocabulary)) {
 const pack=JSON.parse(fs.readFileSync(path.join(root,'locales',locale+'.json')));
 check(pack,locale);
 for(const key of keys) {
  assert.ok(pack[key],`${locale}: missing intent translation ${key}`);
  assert.notEqual(pack[key],source[key],`${locale}: English copy ${key}`);
  const output=new IntlMessageFormat(pack[key],locale).format({p0:'453 {admin}'});
  assert.equal(typeof output,'string');
  if(source[key].includes('{p0}')) assert.ok(output.includes('453 {admin}'),`${locale}: literal error value altered`);
  rendered++;
 }
}
const german=JSON.parse(fs.readFileSync(path.join(root,'locales/de.json')));
assert.throws(()=>check({...german,['server.intent.auto_buyer_will_retry.7675c4dc']:'Der automatische Einkauf kauft sofort.'},'de'),/safety\/terminology/);
assert.throws(()=>check({...german,['server.intent.safety_lock.review_recovery']:german['server.intent.safety_lock.review_recovery']+' 30'},'de'),/manual review/);
console.log(`Rendered ${rendered} intent messages; 225 safety/terminology fixtures and negative purchase/lock regressions passed.`);
