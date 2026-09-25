import {describeMessage} from './messages';
import type {LocalizedMessage} from './formatMessage';
export function eventDisplayMessage(id:number|undefined,name?:string):LocalizedMessage {
 const valid=Number.isSafeInteger(id)&&Number(id)>0;
 const raw=name?.trim() ? name : undefined;
 const source=valid?describeMessage('events.eventId',{id:String(id)}):describeMessage('events.event');
 return {...source,key:raw ? (valid?`game.event_title_${id}`:'events.legacyName') : source.key,...(valid?{officialKey:`event_title_${id}`} : {}),...(raw?{fallback:raw,fallbackText:raw,params:undefined}: {})};
}
