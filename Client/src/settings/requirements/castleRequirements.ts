import type { CastleStateV2, GameStateV2 } from '../../api/Contracts';
import { castleOptionsFromState, type CastleOptionV2 } from '../../api/Selectors';
import type { MessageKey } from '../../i18n/messages';
import type { ReadinessCheck } from '../readiness/Readiness';
import { observationUnavailableMessage, unitObservationFreshness, type ObservationContext } from './observationFreshness';

/**
 * Castle references in automation settings (CIT-18). A saved castle id is never
 * cleared automatically: when it is not in the selected account/world the check
 * is blocked with "reselect" and the user decides.
 */

export type CastlePurpose = 'source-great-empire' | 'outer-main' | 'berimond' | 'any-owned';

const message = (key: MessageKey): MessageKey => key;

/** Great Empire castles are kingdom 0; outer-kingdom main castles are slot 12 in kingdoms 1–3; Berimond is kingdom 10. */
export function castleMatchesPurpose(castle: Pick<CastleStateV2, 'kingdomId' | 'slotType'>, purpose: CastlePurpose): boolean {
  if (purpose === 'source-great-empire') return castle.kingdomId === 0;
  if (purpose === 'outer-main') return castle.kingdomId >= 1 && castle.kingdomId <= 3 && castle.slotType === 12;
  if (purpose === 'berimond') return castle.kingdomId === 10;
  return true;
}

/** The castles a field offers today for this purpose (same ordering as castleOptionsFromState). */
export function castleOptionsFor(state: GameStateV2 | null, purpose: CastlePurpose): CastleOptionV2[] {
  return castleOptionsFromState(state).filter((option) => {
    const castle = state?.castles[String(option.id)];
    return castle != null && castleMatchesPurpose(castle, purpose);
  });
}

export interface CastleReferenceInput {
  castleId: number;
  state: GameStateV2 | null;
  purpose: CastlePurpose;
  /** When set, the castle's unit counts must be current on this connection (CIT-15 D1). */
  requireObservedUnits?: ObservationContext;
  /** Readiness check id; defaults to `source-castle`. */
  id?: string;
}

export function evaluateCastleReference(input: CastleReferenceInput): ReadinessCheck {
  const id = input.id ?? 'source-castle';
  const { state, castleId, purpose } = input;
  if (!state) {
    return { id, state: 'unavailable', messageKey: message('ui.settings.requirements.castleRequirements.castle.data.has.not.been.observed.yet.76ce81b7'), fix: 'connection' };
  }
  if (castleId <= 0) {
    return { id, state: 'blocked', messageKey: message('ui.settings.requirements.castleRequirements.choose.a.castle.ddcc3a1f'), fix: 'settings' };
  }
  const castle = state.castles[String(castleId)];
  if (!castle) {
    return { id, state: 'blocked', messageKey: message('ui.settings.requirements.castleRequirements.the.saved.castle.is.not.in.this.e718c127'), fix: 'settings' };
  }
  if (!castleMatchesPurpose(castle, purpose)) {
    const key = purpose === 'source-great-empire'
      ? message('ui.settings.requirements.castleRequirements.the.source.castle.must.be.in.the.949334c8')
      : purpose === 'outer-main'
        ? message('ui.settings.requirements.castleRequirements.main.castle.is.unavailable.or.the.kingdom.552f7db4')
        : message('ui.settings.requirements.castleRequirements.this.castle.is.not.in.berimond.ba05be9b');
    return { id, state: 'blocked', messageKey: key, fix: 'settings' };
  }
  if (input.requireObservedUnits) {
    const freshness = unitObservationFreshness({ castle, ...input.requireObservedUnits });
    if (freshness.state === 'unavailable') {
      return { id, state: 'unavailable', messageKey: observationUnavailableMessage(freshness.reason), fix: 'connection' };
    }
  }
  return { id, state: 'valid', messageKey: message('ui.settings.requirements.castleRequirements.the.castle.is.available.in.this.account.41064dc3') };
}

/** Identifies the selected account and world; a change means saved castle references may not apply. */
export function accountKey(state: GameStateV2 | null): string {
  if (!state) return '';
  return `${state.account?.uid ?? ''}:${state.account?.worldId ?? ''}`;
}

export function draftReferencesValid(
  references: { castleIds: readonly number[] },
  state: GameStateV2 | null,
): { valid: boolean; missing: number[] } {
  const missing = references.castleIds.filter((castleId) => castleId > 0 && state?.castles[String(castleId)] == null);
  return { valid: state != null && missing.length === 0, missing };
}

export interface AccountSessionTracking {
  key: string;
  generation: number;
}

/**
 * Advances the draft-session generation only on a real account/world switch:
 * the first non-empty key after open is not a switch.
 */
export function advanceAccountSession(tracked: AccountSessionTracking, key: string): AccountSessionTracking {
  if (!key || key === tracked.key) return tracked;
  return { key, generation: tracked.key ? tracked.generation + 1 : tracked.generation };
}
