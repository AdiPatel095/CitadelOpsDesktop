import {APIError} from '../api/CitadelClient';
import {responseMessageDescriptor} from './messageDescriptor';
import {describeMessage} from './messages';
import type {MessageKey} from './messages';
export async function assertTelemetryResponse(response:Response) {
  if(response.ok)return;
  const body:unknown=await response.clone().json().catch(()=>null);
  const descriptor=responseMessageDescriptor(body);
  const record=body && typeof body==='object'?body as Record<string,unknown>:{};
  const nested=record.error && typeof record.error==='object'?record.error as Record<string,unknown>:{};
  const text=typeof nested.message==='string'?nested.message:typeof record.message==='string'?record.message:typeof record.error==='string'?record.error:`HTTP ${response.status}`;
  throw new APIError(text,response.status,undefined,descriptor);
}
export class ActivityPresentationError extends Error {
 readonly messageDescriptor;
 constructor(key:MessageKey){const descriptor=describeMessage(key);super(descriptor.fallback);this.messageDescriptor=descriptor;}
}
export function activityError(error:unknown,key:MessageKey) {
  if((error instanceof APIError || error instanceof ActivityPresentationError) && error.messageDescriptor)return {message:error.message,messageDescriptor:error.messageDescriptor};
  const descriptor=describeMessage(key);
  return {message:descriptor.fallback,messageDescriptor:descriptor,detail:error instanceof Error?error.message:undefined};
}

