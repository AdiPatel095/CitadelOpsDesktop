import type { GameStateV2 } from '../../api/Contracts';
import type { CastleCandidate } from './castleCopy';

/** The castles an editor lists, as copy candidates. `keyFor` supplies the configuration key when it is not the castle id. */
type CandidateSource = { id: number; name: string; kingdomId: number };

export function castleCandidates<C extends CandidateSource>(
  castles: readonly C[],
  state: GameStateV2 | null,
  options: {
    keyFor?: (castle: C) => string;
    flagsFor?: (castle: { id: number; kingdomId: number; slotType?: number }) => Record<string, boolean>;
  } = {},
): CastleCandidate[] {
  return castles.map((castle) => {
    const live = state?.castles?.[String(castle.id)] ?? null;
    const slotType = live?.slotType;
    return {
      key: options.keyFor ? options.keyFor(castle) : String(castle.id),
      liveId: castle.id,
      name: castle.name,
      kingdomId: castle.kingdomId,
      ...(slotType !== undefined ? { slotType } : {}),
      castle: live,
      ...(options.flagsFor ? { flags: options.flagsFor({ id: castle.id, kingdomId: castle.kingdomId, slotType }) } : {}),
    };
  });
}
