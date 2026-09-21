import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import {fileURLToPath} from 'node:url';
export default defineConfig({root:fileURLToPath(new URL('.',import.meta.url)),plugins:[react(),tailwindcss(),{name:'block-game-api',configureServer(server){server.middlewares.use('/api',(request,response)=>{
 response.setHeader('Content-Type','application/json');
 const descriptor={key:'equipment.notification.swapped',fallback:'Equipment loadouts swapped'};
 const line='2026-09-20 09:14:00 [info] [fixture] Equipment loadouts swapped';
 if(request.url?.startsWith('/v2/telemetry/channels')) {response.end(JSON.stringify({channels:[{id:'activity',label:'Fixture activity'}]}));return;}
 if(request.url?.startsWith('/v2/telemetry/activity')) {response.end(JSON.stringify({lines:[line,'2026-09-20 09:14:01 [info] [legacy] Player {0} original text'],entries:[{line,messageDescriptor:descriptor,translationStatus:'structured'}]}));return;}
 response.statusCode=503;response.end('{"error":"Synthetic localization fixture: no runtime"}');
 });}}],server:{host:'127.0.0.1',port:41882,strictPort:true,fs:{allow:[fileURLToPath(new URL('../..',import.meta.url))]}}});
