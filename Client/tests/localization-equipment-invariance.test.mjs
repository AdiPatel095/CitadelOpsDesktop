import assert from 'node:assert/strict';
import {after,test} from 'node:test';
import fs from 'node:fs';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
const vite=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
const effects=await vite.ssrLoadModule('/src/equipment/components/EquipmentEffects.ts');
const priorities=await vite.ssrLoadModule('/src/equipment/components/EquipmentOptimizerState.ts');
const {equipmentEffectTemplates}=await vite.ssrLoadModule('/src/equipment/EquipmentEffectLocalization.ts');
const fixture=JSON.parse(fs.readFileSync(new URL('./fixtures/official-effect-signs-v4357.json',import.meta.url),'utf8'));
after(()=>vite.close());
const rows=profile=>profile.sections.flatMap(section=>section.groups.flatMap(group=>group.rows));
function definition(id,template,canonical,translatedLabel) {
 return {id,name:translatedLabel,internalName:'Meadreduction',effectTypeId:id,effectTypeName:'Meadreduction',sortCategory:7,sortGroup:id,categoryName:translatedLabel,effectGroupPassive:translatedLabel,semanticName:'Mead consumption',semanticEffectGroupPassive:'Mead consumption',semanticTemplate:canonical,effectTemplate:template,capId:1,maxTotalBonus:20};
}
test('English Arabic German and Japanese displays preserve signed values caps scope and identities',()=>{
 const key='ci_effect_Meadreduction';
 const templates={en:fixture.values.en[key],ar:fixture.values.ar[key],de:'Metverbrauch pro Stunde um {0}% reduziert',ja:fixture.values.ja[key]};
 const projections=[];
 for(const [locale,template] of Object.entries(templates)) {
  const metadata={101:definition(101,template,fixture.values.en[key],`label-${locale}`)};
  const profile=effects.buildEquipmentEffectProfile([{label:'Head',effects:[{definitionId:101,values:[25]}]}],metadata,{},1);
  const details=rows(profile);assert.equal(details.length,1);assert.equal(details[0].value,-20);assert.equal(details[0].rawValue,-25);assert.equal(details[0].unit,'percent');
  projections.push(details.map(({key,definitionId,value,rawValue,cap,capped,scope,argumentId})=>({key,definitionId,value,rawValue,cap,capped,scope,argumentId})));
  assert.match(effects.formatEquipmentEffectValue(details[0],details[0].value,locale),/-/);
 }
 for(const projection of projections)assert.deepEqual(projection,projections[0]);
});
test('same displayed noun cannot merge distinct authoritative effect arguments; user braces survive',()=>{
 const canonical='+{0}% for {1}, unknown {2}';
 for(const label of ['Unit','وحدة','Einheit']) {
  const metadata={201:definition(201,canonical,canonical,'strength')};
  const profile=effects.buildEquipmentEffectProfile([{label:'Head',effects:[{definitionId:201,values:[7,10,8,10]}]}],metadata,{7:{id:7,name:`${label} {0}`},8:{id:8,name:`${label} {0}`}},1);
  const details=rows(profile);assert.equal(details.length,2);assert.deepEqual(details.map(row=>row.argumentId).sort(),[7,8]);
  const text=effects.formatEquipmentEffectText(details[0],false,'de');assert.match(text,/\{0\}/);assert.match(text,/\{2\}/);
 }
});
test('official Japanese sign corrections are narrow versioned display adjustments without catalog mutation',()=>{
 for(const key of ['equip_effect_description_wallReductionCharge','equip_effect_description_gateReductionCharge']) {
  const selected={...fixture.values.ja};
  const corrected=equipmentEffectTemplates([key],selected,fixture.values.en,'ja','4357');
  assert.ok(corrected.effectTemplate.includes('-{0}%'));assert.ok(corrected.displayCorrection);assert.ok(selected[key].includes('+{0}%'));
  assert.ok(equipmentEffectTemplates([key],selected,fixture.values.en,'ja','4358').effectTemplate.includes('+{0}%'));
  const profile=effects.buildEquipmentEffectProfile([{label:'Weapon',effects:[{definitionId:101,values:[12]}]}],{101:{...definition(101,corrected.effectTemplate,corrected.semanticTemplate,'Wall protection'),...corrected}}, {},1);
  assert.match(effects.formatEquipmentEffectText(rows(profile)[0],false,'ja'),/-12%/);
 }
});
test('default optimizer priorities use canonical semantic labels rather than selected language',()=>{
 const results=[];
 for(const locale of ['en','ar','de']) {
  const metadata=Object.fromEntries([1,2,3].map(id=>[id,{id,name:`${locale}-${id}`,effectTypeId:id,effectTypeName:`strength${id}`,sortCategory:1,sortGroup:id,effectGroupPassive:`${locale}-${id}`,semanticTemplate:'{0}%',effectTemplate:'{0}%',semanticName:id===1?'Melee combat strength':id===2?'Ranged combat strength':'Other effect',semanticEffectGroupPassive:id===1?'Melee combat strength':id===2?'Ranged combat strength':'Other effect'}]));
  const groups=priorities.groupEquipmentPriorityEffects([1,2,3],metadata);
  results.push(priorities.inferredEquipmentPriorityProfile(groups,'commander'));
 }
 assert.deepEqual(results[0],{tier1:[],tier2:['official-group-1-1','official-group-1-2']});
 assert.deepEqual(results[1],results[0]);assert.deepEqual(results[2],results[0]);
});
test('canonical metadata loading/failure retains stable calculations across viewer locale changes',async()=>{
 const {canonicalEffectReducer}=await vite.ssrLoadModule('/src/equipment/CanonicalEffectState.ts');
 const values={101:definition(101,'-{0}% mead','-{0}% mead','Mead')};
 let state={scope:'runtime-a:4357',status:'ready',values};
 // Locale is deliberately absent from canonical scope.
 assert.equal(canonicalEffectReducer(state,{type:'scope',scope:'runtime-a:4357'}),state);
 state=canonicalEffectReducer(state,{type:'failed',scope:'runtime-a:4357'});
 assert.equal(state.status,'unavailable');assert.equal(state.values,values);
 const afterFailure=effects.buildEquipmentEffectProfile([{label:'Head',effects:[{definitionId:101,values:[25]}]}],state.values,{},1);
 assert.equal(rows(afterFailure)[0].value,-20);
 state=canonicalEffectReducer(state,{type:'scope',scope:'runtime-b:4358'});
 assert.equal(state.status,'loading');assert.deepEqual(state.values,{});
 assert.equal(canonicalEffectReducer(state,{type:'resolved',scope:'runtime-a:4357',values}),state);
});
test('missing official effect arguments stay visible rather than silently disappearing',()=>{
 const template='-{0}% for {1}, threshold {2}';
 const detail=rows(effects.buildEquipmentEffectProfile([{label:'Head',effects:[{definitionId:101,values:[12]}]}],{101:definition(101,template,template,'Effect')},{},1))[0];
 const text=effects.formatEquipmentEffectText(detail,false,'en');
 assert.match(text,/\{1\}/);assert.match(text,/\{2\}/);
});

test('HTTP-successful empty and partial canonical results cannot replace stable calculation state',async()=>{
 const {canonicalEffectReducer,usableCanonicalEffects}=await vite.ssrLoadModule('/src/equipment/CanonicalEffectState.ts');
 const values={101:definition(101,'selected','-{0}%','Mead')};
 const state={scope:'v4357',status:'ready',values};
 for(const incomplete of [{},{101:{...values[101],semanticTemplate:''}},{...values,102:{id:102,semanticTemplate:''}}]) {
  assert.equal(usableCanonicalEffects(incomplete),false);
  const result=canonicalEffectReducer(state,{type:'resolved',scope:state.scope,values:incomplete});
  assert.equal(result.status,'unavailable');assert.equal(result.values,values);
 }
 assert.equal(usableCanonicalEffects(values),true);
});

test('canonical coverage accepts audited absences but rejects successful-empty or partial known templates',async()=>{
 const {canonicalEffectCoverage}=await vite.ssrLoadModule('/src/equipment/CanonicalEffectCoverage.ts');
 const manifest=JSON.parse(fs.readFileSync(new URL('../src/equipment/canonicalEffectCoverage.v4357.json',import.meta.url),'utf8'));
 const records=manifest.entries.map(entry=>({effectID:entry.id,name:entry.name}));
 const canonical=Object.fromEntries(manifest.entries.filter(entry=>entry.key).map(entry=>[entry.key,'canonical template']));
 const healthy=canonicalEffectCoverage(records,canonical,'786.03','4357');
 assert.equal(healthy.ready,true);assert.equal(healthy.expectedTemplates,671);assert.equal(healthy.intentionalAbsences.length,159);
 assert.equal(canonicalEffectCoverage(records,{},'786.03','4357').ready,false);
 const partial={...canonical};const missingName=manifest.entries.find(entry=>entry.key).name;
 for(const prefix of ['relicequip_effect_description_','equip_effect_description_','ci_effect_','effect_name_'])delete partial[prefix+missingName];
 assert.equal(canonicalEffectCoverage(records,partial,'786.03','4357').ready,false);
 assert.equal(canonicalEffectCoverage(records,canonical,'786.04','4357').ready,true);
 const absent=manifest.entries.find(entry=>entry.absence==='no-official-template');
 const changed=records.map(row=>row.effectID===absent.id?{...row,name:'newUnclassifiedIdentity'}:row);
 assert.equal(canonicalEffectCoverage(changed,canonical,'786.04','4357').ready,false);
 const newlyResolved={...canonical,[`equip_effect_description_${absent.name}`]:'+{0}% newly published'};
 assert.equal(canonicalEffectCoverage(records,newlyResolved,'786.04','4357').intentionalAbsences.includes(absent.id),false);
});
test('audited skin and enableUnits absences retain canonical raw-value baseline across locales',()=>{
 for(const name of ['skin','enableUnits']) {
  const projections=[];
  for(const locale of ['en','ar','de']) {
   const metadata={101:{...definition(101,`label-${locale}`,'',name),internalName:name,effectTypeName:name,canonicalTemplateAbsent:true,maxTotalBonus:0}};
   const profile=effects.buildEquipmentEffectProfile([{label:'slot',effects:[{definitionId:101,values:[7]}]}],metadata,{},1);
   projections.push(rows(profile).map(({key,value,rawValue,unit,definitionId})=>({key,value,rawValue,unit,definitionId})));
  }
  assert.deepEqual(projections[1],projections[0]);assert.deepEqual(projections[2],projections[0]);
 }
});
