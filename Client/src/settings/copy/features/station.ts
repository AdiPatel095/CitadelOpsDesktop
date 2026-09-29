import type { MessageKey } from '../../../i18n/messages';
import type { AutoStationTroopReserve } from '../../AutoStationClientState';
import type { CastleCopyDescriptor } from '../castleCopy';
import { reserveDemands, reserveField, type ReserveEntry } from './reserveList';
import { troopEntryReasons } from './unitChecks';

const message = (key: MessageKey): MessageKey => key;

export type StationCopyDraft = Record<string, AutoStationTroopReserve[]>;

/**
 * Auto Station per-castle reserves (`automation.autoStation` settings[id]): the troops left to defend. Lead time,
 * recall, the protection filter and the open-gate fallback are account-wide and never copied.
 */
export const stationCopyDescriptor: CastleCopyDescriptor<StationCopyDraft, ReserveEntry[]> = {
  featureId: 'autoStation',
  sectionId: 'reserves',
  primaryFieldId: 'reserves',
  fields: [reserveField('reserves', message('castleCopy.field.reserves'), {
    write: (_record, value) => (Array.isArray(value) ? (value as ReserveEntry[]).map((entry) => ({ id: entry.id, amount: entry.amount })) : []),
  })],
  notCopiedKey: message('castleCopy.notCopied.station'),
  defaultRecord: () => [],
  recordFor: (draft, key) => draft[key],
  isConfigured: (record) => record.length > 0,
  validate: (source, destination, context) => troopEntryReasons('reserves', reserveDemands(source), destination, context),
  apply: (draft, key, next) => ({ ...draft, [key]: next }),
};
