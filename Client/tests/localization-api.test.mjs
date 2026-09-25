import assert from 'node:assert/strict';
import {after,test} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite = await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {CitadelAPI,APIError} = await vite.ssrLoadModule('/src/api/CitadelClient.ts');
const {parseMessageDescriptor} = await vite.ssrLoadModule('/src/i18n/messageDescriptor.ts');
after(()=>vite.close());
test('HTTP API error retains legacy text and validated message descriptor',async()=>{
 const descriptor={key:'backend.test',fallback:'Section {section}',params:{section:'autoTower'},fallbackText:'Section autoTower'};
 await assert.rejects(CitadelAPI.decodeResponse(new Response(JSON.stringify({error:{code:'invalid',message:'Section autoTower',messageDescriptor:descriptor}}),{status:422})),error=>{
  assert.ok(error instanceof APIError);assert.equal(error.message,'Section autoTower');assert.deepEqual(error.messageDescriptor,descriptor);return true;
 });
});
test('malformed descriptor preserves exact legacy API text',async()=>{
 await assert.rejects(CitadelAPI.decodeResponse(new Response(JSON.stringify({error:{message:'Player {name}',messageDescriptor:{key:'x',fallback:'x',params:{unsafe:{value:'x'}}}}}),{status:400})),error=>{assert.equal(error.message,'Player {name}');assert.equal(error.messageDescriptor,undefined);return true;});
});
test('descriptor boundary accepts one-level official nouns and rejects recursive context',()=>{
 const descriptor={key:'x',fallback:'{castle}',gameParams:{castle:{key:'castle',fallback:'Castle'}},context:[{key:'section',fallback:'Section {id}',params:{id:'unchanged'}}]};
 assert.deepEqual(parseMessageDescriptor(descriptor),descriptor);
 assert.equal(parseMessageDescriptor({...descriptor,context:[{key:'a',fallback:'a',context:[]}]}),undefined);
 assert.equal(parseMessageDescriptor({...descriptor,params:{n:Infinity}}),undefined);
});
