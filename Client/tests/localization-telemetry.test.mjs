import assert from 'node:assert/strict';
import {after,test} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite = await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {telemetryEntries} = await vite.ssrLoadModule('/src/i18n/telemetryMessages.ts');
after(()=>vite.close());
test('telemetry descriptors require exact aligned line; historic text is preserved verbatim',()=>{
 const raw='2026-09-20 09:42:16 [INFO] [attack] Player {name}';
 const messageDescriptor={key:'server.attack',fallback:'Player {name}',params:{name:'User {x}'}};
 assert.deepEqual(telemetryEntries({lines:[raw,'historic'],entries:[{line:raw,messageDescriptor}]}),[{raw,messageDescriptor},{raw:'historic',messageDescriptor:undefined}]);
 assert.equal(telemetryEntries({lines:[raw],entries:[{line:'other',messageDescriptor} ]})[0].messageDescriptor,undefined);
});
test('malformed structured entries never replace legacy text or admit nested values',()=>{
 const result=telemetryEntries({lines:['exact {raw}',42],entries:[{line:'exact {raw}',messageDescriptor:{key:'x',fallback:'x',params:{x:{unsafe:true}}}}]});
 assert.deepEqual(result,[{raw:'exact {raw}',messageDescriptor:undefined}]);
 assert.deepEqual(telemetryEntries({entries:[]}),[]);
});
