import type { MessageKey } from '../../../i18n/messages';
import type { CastleCandidate, CastleCopyContext, CastleCopyField } from '../castleCopy';
import { troopName, type TroopDemand } from './unitChecks';

const message = (key: MessageKey): MessageKey => key;

export interface ReserveEntry {
  id: number;
  amount: number;
}

function normalized(value: unknown): ReserveEntry[] {
  if (!Array.isArray(value)) return [];
  return (value as ReserveEntry[]).map((entry) => ({ id: entry.id, amount: entry.amount })).sort((left, right) => left.id - right.id);
}

/** Reserve lists are sets of `{id, amount}`: order is not a difference. */
export function reserveListsEqual(left: unknown, right: unknown): boolean {
  return JSON.stringify(normalized(left)) === JSON.stringify(normalized(right));
}

export function describeReserveList(value: unknown, context: CastleCopyContext) {
  const entries = normalized(value);
  return {
    messageKey: message('castleCopy.value.reserves'),
    params: { count: entries.length, list: entries.map((entry) => `${troopName(context, entry.id)} ×${entry.amount.toLocaleString()}`).join(', ') },
  };
}

export function reserveDemands(entries: readonly ReserveEntry[]): TroopDemand[] {
  return entries.map((entry) => ({ id: entry.id, amount: entry.amount }));
}

/** The one list field of a per-castle troop reserve map (Station reserves, Bird keep list). */
export function reserveField(
  id: string,
  labelKey: MessageKey,
  hooks: {
    readFor?: (record: ReserveEntry[], destination: CastleCandidate) => unknown;
    write: (record: ReserveEntry[], value: unknown, destination: CastleCandidate) => ReserveEntry[];
  },
): CastleCopyField<ReserveEntry[]> {
  return {
    id, labelKey,
    read: (record) => record,
    ...(hooks.readFor ? { readFor: hooks.readFor } : {}),
    write: hooks.write,
    equal: reserveListsEqual,
    describe: describeReserveList,
    entryId: (entry) => (entry as ReserveEntry).id,
  };
}
