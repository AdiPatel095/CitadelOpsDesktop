import type { MetadataItem } from '../context/MetadataContext';
export type CanonicalEffectState = {scope:string; status:'loading'|'ready'|'unavailable'; values:Record<number,MetadataItem>};
export type CanonicalEffectAction = {type:'scope';scope:string} | {type:'resolved';scope:string;values:Record<number,MetadataItem>} | {type:'failed';scope:string};
/** An HTTP-successful empty/partial dictionary is not usable calculation metadata. */
export function usableCanonicalEffects(values:Record<number,MetadataItem>):boolean {
  const definitions=Object.values(values);
  return definitions.length>0 && definitions.every(value=>value.canonicalTemplateAbsent===true || (typeof value.semanticTemplate==='string' && value.semanticTemplate.trim().length>0));
}
/** Viewer language never changes the scope. Only canonical runtime/version changes reset calculations. */
export function canonicalEffectReducer(state:CanonicalEffectState,action:CanonicalEffectAction):CanonicalEffectState {
  if(action.type==='scope')return action.scope===state.scope ? state : {scope:action.scope,status:'loading',values:{}};
  if(action.scope!==state.scope)return state;
  if(action.type==='failed')return {...state,status:'unavailable'};
  if(!usableCanonicalEffects(action.values))return {...state,status:'unavailable'};
  return {...state,status:'ready',values:action.values};
}
