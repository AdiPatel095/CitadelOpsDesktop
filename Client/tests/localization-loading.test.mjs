import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import ts from 'typescript';
const url=code=>'data:text/javascript;base64,'+Buffer.from(code).toString('base64');
const compile=source=>ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ES2022,jsx:ts.JsxEmit.React}}).outputText;
const strip=source=>source.replace(/^import .*?;\n/gm,'');
const deferred=()=>{let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject};};
const flush=async()=>{for(let i=0;i<8;i++)await Promise.resolve();};

test('backend loader deduplicates success and evicts transient rejection for retry',async()=>{
 const source=fs.readFileSync(new URL('../src/i18n/backendCatalog.ts',import.meta.url),'utf8');
 let calls=0;let attempt=deferred();
 globalThis.__localeLoaders={'./backend/de.json':()=>{calls++;return attempt.promise;}};
 const patched=strip(source).replace(/const loaders = .*?;\n/,'const loaders = globalThis.__localeLoaders;\n');
 const {loadBackendCatalog}=await import(url(compile("const normalizeLocale=value=>value;\n"+patched)));
 try {
  const one=loadBackendCatalog('de');const two=loadBackendCatalog('de');assert.equal(one,two);assert.equal(calls,1);
  attempt.reject(Error('offline'));await Promise.all([assert.rejects(one,/offline/),assert.rejects(two,/offline/)]);
  attempt=deferred();const retry=loadBackendCatalog('de');assert.equal(calls,2);attempt.resolve({default:{key:'Deutsch'}});
  assert.deepEqual(await retry,{key:'Deutsch'});assert.equal(loadBackendCatalog('de'),retry);assert.equal(calls,2);
 } finally {delete globalThis.__localeLoaders;}
});

test('actual provider retains healthy packs, merge precedence, and fences stale locale completions',async()=>{
 let locale='de',preference='auto',cursor=0;const states=[],effects=[],pendingEffects=[],requests=[];
 const dependenciesEqual=(left,right)=>left&&left.length===right.length&&left.every((value,index)=>value===right[index]);
 const harness={
  React:{createElement:(type,props)=>({type,props})},
  createContext:value=>({Provider:'provider',value}),useContext:context=>context.value,
  useMemo:fn=>fn(),useSyncExternalStore:(_subscribe,read)=>read(),
  useState:initial=>{const index=cursor++;if(!(index in states))states[index]=initial;return [states[index],value=>{states[index]=value;}];},
  useEffect:(effect,deps)=>{const index=cursor++;if(!dependenciesEqual(effects[index]?.deps,deps)){pendingEffects.push(()=>{effects[index]?.cleanup?.();effects[index]={deps,cleanup:effect()};});}},
  readViewerLocale:()=>locale,readViewerLocalePreference:()=>preference,setViewerLocale:()=>{},subscribeViewerLocale:()=>()=>{},
  loadBundledOfficial:()=>Promise.resolve({values:{}}),mergeOfficialCatalogs:()=>({values:{}}),invalidateOfficialMessages:()=>{},
  officialMessageKeys:{},officialMessageNouns:{},describeMessage:()=>({}),formatMessage:()=>({text:'fixture'}),
  CitadelAPI:{subscribeStatus:()=>()=>{},localizeCatalog:()=>Promise.resolve({values:{}})},
 };
 for(const name of ['loadMessageCatalog','loadBackendCatalog','loadServerCatalog'])harness[name]=language=>{const item={name,language,...deferred()};requests.push(item);return item.promise;};
 globalThis.__localeHarness=harness;const previousDocument=globalThis.document;globalThis.document={documentElement:{dataset:{}}};
 const source=strip(fs.readFileSync(new URL('../src/i18n/LocaleContext.tsx',import.meta.url),'utf8'));
 const {LocaleProvider}=await import(url(compile(`const {${Object.keys(harness).join(',')}}=globalThis.__localeHarness;\n${source}`)));
 const render=()=>{cursor=0;const result=LocaleProvider({children:null});pendingEffects.splice(0).forEach(run=>run());return result.props.value;};
 try {
  render();const german=requests.slice();
  locale='fr';preference='fr';assert.deepEqual(render().catalog,{});const french=requests.slice(3);
  french.find(item=>item.name==='loadMessageCatalog').resolve({custom:'Français',collision:'custom'});
  french.find(item=>item.name==='loadBackendCatalog').reject(Error('transient'));
  french.find(item=>item.name==='loadServerCatalog').resolve({server:'Serveur',collision:'server'});
  await flush();assert.deepEqual(render().catalog,{server:'Serveur',custom:'Français',collision:'custom'});assert.equal(render().preference,'fr');
  for(const item of german)item.resolve({stale:'Deutsch'});await flush();assert.equal(render().catalog.stale,undefined);
  locale='ar';render();const arabic=requests.slice(6);
  arabic.forEach(item=>item.reject(Error('offline')));await flush();assert.deepEqual(render().catalog,{});assert.equal(render().direction,'rtl');
 } finally {effects.forEach(item=>item?.cleanup?.());globalThis.document=previousDocument;delete globalThis.__localeHarness;}
});
