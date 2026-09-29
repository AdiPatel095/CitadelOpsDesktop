import type { MessageKey } from '../../../i18n/messages';
import { AUTO_FORTRESS_DIREWOLF_ID } from '../../AutoFortressClientState';
import { autoFortressReservesDirewolves } from '../../AutoBirdFortressReserve';
import type { CastleCandidate, CastleCopyContext, CastleCopyDescriptor } from '../castleCopy';
import { reserveDemands, reserveField, type ReserveEntry } from './reserveList';
import { troopEntryReasons } from './unitChecks';

const message = (key: MessageKey): MessageKey => key;

/** The map the editor is changing: the active preset's map or `ignoreSettings.settings`. Other presets are never touched. */
export type BirdCopyDraft = Record<string, ReserveEntry[]>;

/** Flag set on a candidate when Auto Fortress reserves its Direwolves (outer main castle of an enabled kingdom). */
export const DIREWOLVES_RESERVED = 'direwolvesReserved';

export function birdCandidateFlags(castle: { kingdomId: number; slotType?: number }, context: Pick<CastleCopyContext, 'fortress'>): Record<string, boolean> {
  return {
    [DIREWOLVES_RESERVED]: autoFortressReservesDirewolves(context.fortress?.enabled === true, castle, context.fortress?.section),
  };
}

const reserved = (destination: CastleCandidate) => destination.flags?.[DIREWOLVES_RESERVED] === true;
const withoutDirewolves = (entries: ReserveEntry[]) => entries.filter((entry) => entry.id !== AUTO_FORTRESS_DIREWOLF_ID);

/**
 * Auto Bird keep lists. On an outer main castle whose Direwolves Auto Fortress reserves, Direwolves are dropped from
 * the copy (named in the preview) and the destination's own Direwolf row is kept as it is. Send delay, minimum group
 * size, the protection filter and preset management are account-wide and never copied.
 */
export const birdCopyDescriptor: CastleCopyDescriptor<BirdCopyDraft, ReserveEntry[]> = {
  featureId: 'autoBird',
  sectionId: 'castles',
  primaryFieldId: 'keep',
  fields: [reserveField('keep', message('castleCopy.field.keep'), {
    readFor: (record, destination) => (reserved(destination) ? withoutDirewolves(record) : record),
    write: (record, value, destination) => {
      const incoming = Array.isArray(value) ? (value as ReserveEntry[]).map((entry) => ({ id: entry.id, amount: entry.amount })) : [];
      if (!reserved(destination)) return incoming;
      const manual = record.find((entry) => entry.id === AUTO_FORTRESS_DIREWOLF_ID);
      return manual ? [...withoutDirewolves(incoming), manual] : withoutDirewolves(incoming);
    },
  })],
  notCopiedKey: message('castleCopy.notCopied.bird'),
  defaultRecord: () => [],
  recordFor: (draft, key) => draft[key],
  isConfigured: (record) => record.length > 0,
  validate: (source, destination, context) => {
    const reasons = troopEntryReasons('keep', reserveDemands(source), destination, context);
    if (reserved(destination) && source.some((entry) => entry.id === AUTO_FORTRESS_DIREWOLF_ID)) {
      reasons.push({
        id: 'direwolves-reserved', state: 'incompatible', messageKey: message('castleCopy.reason.direwolves'),
        params: { castle: destination.name }, fieldId: 'keep', dropEntryIds: [AUTO_FORTRESS_DIREWOLF_ID],
      });
    }
    return reasons;
  },
  apply: (draft, key, next) => ({ ...draft, [key]: next }),
};
