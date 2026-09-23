import assert from 'node:assert/strict';
import test,{after} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const {formatMessage}=await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const {parseMessageDescriptor}=await vite.ssrLoadModule('/src/i18n/messageDescriptor.ts');
after(()=>vite.close());
const leaf={key:'item',fallback:'{name}',params:{name:'Player {0} <b>literal</b>'}};
const descriptor={key:'purchase',fallback:'Bought {items}',fallbackText:'Original complete purchase {0}',listParams:{items:[leaf,{key:'unit',fallback:'Troops'}]},context:[{key:'prefix',fallback:'Action'}]};
test('lists are translated ordered leaves and never reparsed as templates',()=>{
 const result=formatMessage(descriptor,'de',{purchase:'Gekauft: {items}',item:'{name}',unit:'Truppen',prefix:'Aktion'});
 assert.equal(result.text,'Aktion: Gekauft: Player {0} <b>literal</b> und Truppen');
 assert.equal(result.resolvedLocale,'de');
 assert.deepEqual(descriptor.listParams.items[0].params,{name:'Player {0} <b>literal</b>'});
});
test('one missing or invalid leaf restores complete root text before context',()=>{
 for(const catalog of [{purchase:'Gekauft: {items}',item:'{name}'},{purchase:'Gekauft: {items}',item:'{name}',unit:'{missing}'}]){
  assert.deepEqual(formatMessage(descriptor,'de',catalog),{text:descriptor.fallbackText,translated:false,resolvedLocale:'en'});
 }
 const invalid={...descriptor,listParams:{items:[{key:'bad',fallback:'{missing}'}]}};
 assert.equal(formatMessage(invalid,'en',{}).text,descriptor.fallbackText);
});
test('list schema rejects nesting, collisions, missing binding and bounds without truncation',()=>{
 assert.ok(parseMessageDescriptor(descriptor));
 const invalid=[{...descriptor,fallbackText:undefined},{...descriptor,params:{items:'collision'}},{...descriptor,listParams:{items:[]}},{...descriptor,listParams:{items:Array(33).fill(leaf)}},{...descriptor,listParams:{items:[{...leaf,context:[]}]}},{...descriptor,listParams:{items:[{...leaf,listParams:{}}]}},{...descriptor,listParams:{items:[{...leaf,params:{name:'文'.repeat(23_000)}}]}}];
 for(const value of invalid){assert.equal(parseMessageDescriptor(value),undefined);assert.equal(formatMessage(value,'de',{}).text,value.fallbackText??'');}
});
test('English leaves and official nouns resolve without foreign translation credit',()=>{
 const source={...descriptor,context:undefined};
 assert.equal(formatMessage(source,'en',{}).text,source.fallbackText);
 const official={...source,listParams:{items:[{key:'game.wood',fallback:'Wood',officialKey:'wood'}]}};
 assert.equal(formatMessage(official,'de',{purchase:'Gekauft: {items}'},{values:{wood:'Holz'},resolvedLocale:'de'}).text,'Gekauft: Holz');
 assert.equal(formatMessage(official,'de',{purchase:'Gekauft: {items}'},{values:{wood:'Wood'},resolvedLocale:'en'}).text,source.fallbackText);
});

test('Go JSON escaped characters count toward the shared byte budget',()=>{
 const input={...descriptor,listParams:{items:[{...leaf,params:{name:'<'.repeat(11_000)}}]}};
 assert.equal(parseMessageDescriptor(input),undefined);
 assert.equal(formatMessage(input,'de',{}).text,descriptor.fallbackText);
});

test('lists cannot be silently omitted or interpreted as ICU typed arguments',()=>{
 for(const template of ['Done',"Bought '{items}'",'Bought {items, number}','{items, select, other {Done}}','{mode, select, yes {{items}} other {Done}}']){
  const input={...descriptor,params:{mode:'yes'}};
  assert.equal(formatMessage(input,'de',{purchase:template,item:'{name}',unit:'Truppen'}).text,descriptor.fallbackText,template);
  assert.equal(formatMessage({...input,fallback:template},'de',{purchase:'{items}',item:'{name}',unit:'Truppen'}).text,descriptor.fallbackText,template);
 }
 assert.equal(parseMessageDescriptor({...descriptor,officialKey:'wood'}),undefined);
});
test('aggregate limits accept exact boundaries and reject overflow and collisions',()=>{
 const bounded={...descriptor,fallback:'{a}{b}{c}{d}',listParams:Object.fromEntries(['a','b','c','d'].map(name=>[name,Array(16).fill(leaf)]))};
 assert.ok(parseMessageDescriptor(bounded));
 assert.equal(parseMessageDescriptor({...bounded,listParams:{a:Array(32).fill(leaf),b:Array(32).fill(leaf),c:[leaf]}}),undefined);
 assert.equal(parseMessageDescriptor({...bounded,listParams:{a:[leaf],b:[leaf],c:[leaf],d:[leaf],e:[leaf]}}),undefined);
 assert.equal(parseMessageDescriptor({...descriptor,gameParams:{items:{key:'wood',fallback:'Wood'}}}),undefined);
 assert.equal(parseMessageDescriptor({...descriptor,listParams:JSON.parse('{"__proto__":[{"key":"x","fallback":"x"}]}')}),undefined);
});
test('Arabic lists isolate literal Latin names and keep list conjunction locale-aware',()=>{
 const result=formatMessage({...descriptor,context:undefined},'ar',{purchase:'اشتريت {items}',item:'{name}',unit:'جنود'});
 assert.equal(result.resolvedLocale,'ar');
 assert.ok(result.text.includes('Player {0} <b>literal</b>'));
 assert.ok(result.text.includes('جنود'));
 assert.ok(result.text.includes('\u2068'));
});
