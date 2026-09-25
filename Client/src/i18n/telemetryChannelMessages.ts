import definitions from './telemetryChannelFallbacks.json';
import type {LocalizedMessage} from './formatMessage';
/** Explicit producer-owned channel identity mapping; never match the English label. */
export function telemetryChannelDescriptor(id:string,field:'label'|'description',legacyText?:string):LocalizedMessage|undefined {
 const channels:Readonly<Record<string,Record<'label'|'description',LocalizedMessage>>>=definitions;
 if(!Object.hasOwn(channels,id))return undefined;
 const descriptor=channels[id][field];
 return {...descriptor,...(legacyText?{fallbackText:legacyText}:{})};
}
