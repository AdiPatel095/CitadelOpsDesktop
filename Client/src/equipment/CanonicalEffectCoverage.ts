import manifest from './canonicalEffectCoverage.v4357.json';
const prefixes=['relicequip_effect_description_','equip_effect_description_','ci_effect_','effect_name_'];
const retired=new Set(manifest.entries.filter(entry=>entry.absence==='retired-template').map(entry=>entry.name));
/** Versions document the audit; exact identity permits unrelated catalog version changes. */
export function canonicalEffectCoverage(rows:Array<Record<string,unknown>>, translations:Record<string,string>, _itemVersion:string, languageVersion:string) {
  const known=new Map(manifest.entries.map(entry=>[entry.id,entry]));
  const missing:number[]=[];
  const intentionalAbsences:number[]=[];
  let expectedTemplates=0;
  for(const row of rows) {
    const id=Number(row.effectID), name=String(row.name ?? '');
    const entry=known.get(id);
    const documented=entry?.name===name;
    const hasTemplate=prefixes.some(prefix=>!(languageVersion==='4357' && retired.has(name) && prefix==='equip_effect_description_') && translations[prefix+name]?.trim());
    if(hasTemplate) {expectedTemplates++;continue;}
    if(documented && !entry.key)intentionalAbsences.push(id);
    else missing.push(id);
  }
  const compatible=rows.length>0 && new Set(rows.map(row=>Number(row.effectID))).size===rows.length;
  return {ready:compatible && expectedTemplates>0 && missing.length===0,compatible,missing,expectedTemplates,intentionalAbsences};
}
