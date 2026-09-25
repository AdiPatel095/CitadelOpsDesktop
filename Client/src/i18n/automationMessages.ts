import {describeMessage} from './messages';
import type {LocalizedMessage} from './formatMessage';
import {parseMessageDescriptor} from './messageDescriptor';

/** Runtime descriptors must be atomically bound to the adjacent, unchanged detail. */
export function automationDetailMessage(detail: string | undefined, value: unknown): LocalizedMessage | undefined {
  if (typeof detail !== 'string' || !detail) return undefined;
  const parsed = parseMessageDescriptor(value);
  return parsed?.fallbackText === detail ? parsed : undefined;
}
const statuses = new Set(['idle','armed','cooldown','defending','discovering','evacuating','preparing','protected','protecting','recalling','reconciling','refreshing','replenishing','resolving','taunting','threat','yielding','complete','completed','success','failed','error','blocked','gated','retrying','warning','running','enabled','scheduled','ready','waiting','disabled']);
const lanes = new Set(['overall','crafting','logistics','attacks','cooldowns','rage','defense','transfers','tools','combat','aquamarine-shop','builder','builder-missing-decorations']);
export function automationStatusMessage(status: string): LocalizedMessage | undefined {
  return statuses.has(status) || !status ? describeMessage('automation.status',{status:status || 'other'}) : undefined;
}
export function automationLaneMessage(id: string): LocalizedMessage | undefined {
  return lanes.has(id) ? describeMessage('automation.lane',{lane:id.replaceAll('-','_')}) : undefined;
}
