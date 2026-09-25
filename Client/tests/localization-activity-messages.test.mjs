import assert from 'node:assert/strict';
import fs from 'node:fs';
import test,{after} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {assertTelemetryResponse,activityError}=await vite.ssrLoadModule('/src/i18n/activityMessages.ts');
const {formatMessage}=await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
after(()=>vite.close());
test('activity HTTP errors retain validated descriptors and literal legacy text',async()=>{
 const descriptor={key:'server.test',fallback:'Channel {channel}',params:{channel:'Player {0}'},fallbackText:'Channel Player {0}'};
 await assert.rejects(assertTelemetryResponse(new Response(JSON.stringify({error:{message:'Channel Player {0}',messageDescriptor:descriptor}}),{status:503})),error=>{
  const failure=activityError(error,'activity.loadChannelsError');
  assert.deepEqual(failure.messageDescriptor,descriptor);assert.equal(failure.message,'Channel Player {0}');
  assert.equal(formatMessage(failure.messageDescriptor,'de',{'server.test':'Kanal {channel}'}).text,'Kanal Player {0}');return true;
 });
});
test('unstructured transport and clipboard errors preserve source beneath a keyed summary',async()=>{
 const failure=activityError(new Error('User {0} <b>literal</b>'),'activity.copyError');
 assert.equal(failure.messageDescriptor.key,'activity.copyError');assert.equal(failure.detail,'User {0} <b>literal</b>');
 await assert.rejects(assertTelemetryResponse(new Response('Unavailable',{status:503})),error=>{
  const failure=activityError(error,'activity.loadLogError');assert.equal(failure.detail,'HTTP 503');assert.equal(failure.messageDescriptor.key,'activity.loadLogError');return true;
 });
});
test('legacy known channels use explicit producer keys; unknown names remain original',async()=>{
 const {telemetryChannelDescriptor}=await vite.ssrLoadModule('/src/i18n/telemetryChannelMessages.ts');
 assert.equal(telemetryChannelDescriptor('custom-player-name','label','Player {0}'),undefined);
 const descriptor=telemetryChannelDescriptor('autobird','label','Auto Bird');
 assert.equal(descriptor.key,'server.telemetry.channel.autobird.label');
 assert.equal(formatMessage(descriptor,'de',{}).text,'Auto Bird');
 assert.equal(formatMessage(descriptor,'de',{[descriptor.key]:'Automatischer Truppenschutz'}).text,'Automatischer Truppenschutz');
});

test('bundled channel fallbacks match synchronized producer source exactly',()=>{
 const channels=JSON.parse(fs.readFileSync(new URL('../src/i18n/telemetryChannelFallbacks.json',import.meta.url)));
 const source=JSON.parse(fs.readFileSync(new URL('../src/i18n/server/en.json',import.meta.url)));
 assert.equal(Object.keys(channels).length,23);
 for(const fields of Object.values(channels))for(const descriptor of Object.values(fields))assert.equal(descriptor.fallback,source[descriptor.key]);
});
test('finite application event labels translate while unknown game opcodes remain identities',async()=>{
 const {activityEventMessageKey}=await vite.ssrLoadModule('/src/i18n/activityEventMessages.ts');
 const {describeMessage}=await vite.ssrLoadModule('/src/i18n/messages.ts');
 assert.equal(activityEventMessageKey('gbd'),undefined);
 assert.equal(activityEventMessageKey('UNKNOWN {0}'),undefined);
 const key=activityEventMessageKey('PURCHASE');assert.equal(key,'activity.rowPurchase');
 assert.equal(formatMessage(describeMessage(key),'de',{[key]:'Kauf'}).text,'Kauf');
 for(const event of ['ACTION','ALLIANCE HELP','ATTACK','BUILDING','CONSTRUCTION','CRAFTING','DEFENSE','EQUIPMENT','ESPIONAGE','EVENT','HOSPITAL','PURCHASE','QUEUE','TIME SKIP','TRANSPORT'])assert.ok(activityEventMessageKey(event),event);
});
