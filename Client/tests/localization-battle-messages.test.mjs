import assert from 'node:assert/strict';
import test,{after} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {describeMessage}=await vite.ssrLoadModule('/src/i18n/messages.ts');
const {formatMessage}=await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const {eventDisplayMessage}=await vite.ssrLoadModule('/src/i18n/eventDisplayMessage.ts');
after(()=>vite.close());
test('battle tooltip keeps literal names and formats only finite numeric parameters',()=>{
 const params={name:'Player {0} <b>literal</b>',amount:1234,phase:'wall',changed:12,changeKind:'lost'};
 const descriptor=describeMessage('battle.itemTooltip',params);
 const source=formatMessage(descriptor,'en',{});
 assert.equal(source.text,'Player {0} <b>literal</b> · ×1,234 · Wall · lost 12');
 const catalog={'battle.itemTooltip':'{name}: {amount, number}{phase, select, wall { an der Mauer} courtyard { im Innenhof} support { Unterstützung} other {}}{changeKind, select, lost {; {changed, number} verloren} used {; {changed, number} verbraucht} other {}}'};
 const translated=formatMessage(descriptor,'de',catalog);
 assert.equal(translated.text,'Player {0} <b>literal</b>: 1.234 an der Mauer; 12 verloren');
 assert.deepEqual(params,{name:'Player {0} <b>literal</b>',amount:1234,phase:'wall',changed:12,changeKind:'lost'});
});
test('empty roster messages and identity fallbacks are whole sentences',()=>{
 for(const [key,expected] of [['battle.noParsedUnits','No parsed units.'],['battle.noParsedTools','No parsed tools.']])assert.equal(formatMessage(describeMessage(key),'en',{}).text,expected);
 assert.equal(formatMessage(describeMessage('battle.unitId',{id:'001234'}),'de',{'battle.unitId':'Einheit {id}'}).text,'Einheit 001234');
});

test('official event identity overrides legacy display names without classifying by prose',()=>{
 const message=eventDisplayMessage(71,'Foreign Lords Invasion');
 assert.equal(message.officialKey,'event_title_71');
 assert.equal(formatMessage(message,'de',{}, {values:{event_title_71:'Krieg der Imperien'},resolvedLocale:'de'}).text,'Krieg der Imperien');
 assert.equal(formatMessage(eventDisplayMessage(999,'Historic {0} <b>literal</b>'),'de',{}).text,'Historic {0} <b>literal</b>');
 assert.equal(formatMessage(eventDisplayMessage(999),'de',{'events.eventId':'Ereignis {id}'}).text,'Ereignis 999');
 assert.equal(eventDisplayMessage(-1,'Unknown').officialKey,undefined);
 assert.equal(formatMessage(eventDisplayMessage(999,'  Historic {0}  '),'de',{}).text,'  Historic {0}  ');
});
