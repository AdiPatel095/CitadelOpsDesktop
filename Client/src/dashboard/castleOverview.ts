import type { CastleStateV2, CastleUnitsV2 } from '../api/Contracts';
import type { MetadataItem } from '../context/MetadataContext';
import { observationTimestamp, type ObservationPresence } from '../settings/requirements/observationFreshness';

export const NEAR_CAP_RATIO = 0.9;

export interface CastleResourceRow {
  id: number;
  name: string;
  icon?: string;
  amount: number;
  capacity?: number;
  ratio: number;
  nearCap: boolean;
  perHour: number;
}

/** Preserve the resource card's descending id order and metadata fallbacks. */
export function resourceRows(
  castle: Pick<CastleStateV2, 'resources'>,
  definitions: Readonly<Record<number, MetadataItem>>,
): CastleResourceRow[] {
  return Object.entries(castle.resources)
    .filter(([id]) => Number.isFinite(Number(id)) && Number(id) > 0)
    .map(([rawId, balance]) => {
      const id = Number(rawId);
      const definition = definitions[id];
      const internalName = typeof definition?.internalName === 'string' ? definition.internalName : '';
      const amount = balance.amount ?? 0;
      const capacity = balance.capacity;
      const ratio = capacity != null && capacity > 0 ? amount / capacity : 0;
      return {
        id,
        name: definition?.name || internalName || `Resource ${id}`,
        icon: definition?.image,
        amount,
        capacity,
        ratio,
        nearCap: ratio >= NEAR_CAP_RATIO,
        perHour: balance.productionPerHour ?? 0,
      };
    })
    .sort((left, right) => right.id - left.id);
}

export function troopTotals(units: CastleUnitsV2): { stationed: number; traveling: number; hospital: number } {
  const sum = (counts: Record<string, number>) => Object.values(counts).reduce((total, count) => total + count, 0);
  return {
    stationed: sum(units.stationed),
    traveling: sum(units.traveling),
    hospital: sum(units.hospital) + sum(units.specialHospital),
  };
}

export type CastleDataAge = { kind: 'live' } | { kind: 'saved'; at?: string };

export function castleDataAge(input: {
  presence?: ObservationPresence;
  connected: boolean;
  castle: Pick<CastleStateV2, 'unitsObservedAt' | 'foodStateObservedAt' | 'contextSnapshotObservedAt'> | null;
}): CastleDataAge {
  if (input.presence?.mode === 'checkpoint') {
    const at = observationTimestamp(input.presence.checkpointObservedAt);
    return at ? { kind: 'saved', at } : { kind: 'saved' };
  }
  if (input.connected) return { kind: 'live' };
  const times = [input.castle?.unitsObservedAt, input.castle?.foodStateObservedAt, input.castle?.contextSnapshotObservedAt]
    .map(observationTimestamp)
    .filter((time): time is string => time !== undefined)
    .sort((left, right) => Date.parse(right) - Date.parse(left));
  return times[0] ? { kind: 'saved', at: times[0] } : { kind: 'saved' };
}
