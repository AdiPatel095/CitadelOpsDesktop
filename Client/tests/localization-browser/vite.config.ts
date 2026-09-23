import laneOfficial from './lane-official-fixtures.json';
import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import {fileURLToPath} from 'node:url';
let operationalRequests = 0;
export default defineConfig({root:fileURLToPath(new URL('.',import.meta.url)),plugins:[react(),tailwindcss(),{name:'block-game-api',configureServer(server){server.middlewares.use('/__fixture-requests',(_request,response)=>{response.setHeader('Content-Type','application/json');response.end(JSON.stringify({operationalRequests}));});server.middlewares.use('/api',(request,response)=>{
 if(request.method !== 'GET' && !request.url?.startsWith('/v2/game-data/localize')) operationalRequests++;
 response.setHeader('Content-Type','application/json');
 if(request.url?.startsWith('/v2/game-data/localize')) {
  const locale=new URL(request.url,'http://fixture').searchParams.get('locale') ?? 'en';
  const value=locale==='de'||locale==='ar' ? laneOfficial[locale].errorCode_90 : 'You need to wait at least 4 seconds between attacks.';
  response.end(JSON.stringify({values:{errorCode_90:value},locale:{requestedLocale:locale,resolvedLocale:locale,fallback:false}}));return;
 }
 const descriptor={key:'equipment.notification.swapped',fallback:'Equipment loadouts swapped'};
 const line='2026-09-20 09:14:00 [info] [EQUIPMENT] Equipment loadouts swapped';
 if(request.url?.startsWith('/v2/telemetry/channels')) {response.end(JSON.stringify({channels:[{id:'activity',label:'Fixture activity'}]}));return;}
 if(request.url?.startsWith('/v2/telemetry/activity')) {response.end(JSON.stringify({lines:[line,'2026-09-20 09:14:01 [info] [legacy] Player {0} original text'],entries:[{line,messageDescriptor:descriptor,translationStatus:'structured'}]}));return;}
 response.statusCode=503;response.end('{"error":"Synthetic localization fixture: no runtime"}');
 });}}],server:{host:'127.0.0.1',port:41882,strictPort:true,fs:{allow:[fileURLToPath(new URL('../..',import.meta.url))]}}});
