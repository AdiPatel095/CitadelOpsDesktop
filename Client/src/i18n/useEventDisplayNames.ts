import {useEffect,useMemo,useState,useSyncExternalStore} from 'react';
import {useLocale} from './LocaleContext';
import {loadOfficialMessages,officialCatalogGeneration,subscribeOfficialCatalog} from './officialMessages';
import {formatMessage,type OfficialCatalog} from './formatMessage';
import {eventDisplayMessage} from './eventDisplayMessage';
/** Event IDs are official catalog identities. Display names never feed event classifiers. */
export function useEventDisplayNames(eventIds:readonly (number|undefined)[]){
 const {locale,catalog,runtimeStatus}=useLocale();
 const generation=useSyncExternalStore(subscribeOfficialCatalog,officialCatalogGeneration,()=>0);
 const keys=JSON.stringify([...new Set(eventIds.filter((id):id is number=>Number.isSafeInteger(id)&&Number(id)>0).map(id=>`event_title_${id}`))].sort());
 const identity=`${locale}:${runtimeStatus}:${generation}:${keys}`;
 const [loaded,setLoaded]=useState<{identity:string;catalog:OfficialCatalog}|null>(null);
 useEffect(()=>{let active=true;const requested=JSON.parse(keys) as string[];if(!requested.length)return;
  void loadOfficialMessages(requested,locale).then(result=>{if(active)setLoaded({identity,catalog:result});}).catch(()=>{if(active)setLoaded(null);});
  return()=>{active=false;};
 },[identity,keys,locale]);
 return useMemo(()=>(id:number|undefined,name?:string)=>{
  const descriptor=eventDisplayMessage(id,name);
  return formatMessage(descriptor,locale,catalog,loaded?.identity===identity?loaded.catalog:undefined);
 },[locale,catalog,loaded,identity]);
}
