import { useMemo, useState } from 'react';
import { useCitadelAPI } from '../../api/useCitadelAPI';
import { useAuth } from '../../context/useAuth';
import { movementViewFromState } from '../../Movement/types/MovementState';
import { accountKey, advanceAccountSession, type AccountSessionTracking } from './castleRequirements';
import type { ObservationContext, ObservationPresence } from './observationFreshness';

/**
 * Observations a settings modal needs to evaluate requirements (CIT-18).
 * The movement view is derived from state only: nothing here asks the game to
 * refresh commander movements (`useMovement().refreshMovement` would submit
 * `game.refresh_movements`, so it is deliberately not used).
 */
export function useSetupContext(section: string, hostedPresence?: ObservationPresence) {
  const { state } = useCitadelAPI();
  const { gameLoggedIn } = useAuth();
  const movement = useMemo(() => movementViewFromState(state), [state]);
  const sessionKey = useAccountSessionKey(section, accountKey(state));
  const session = state?.session ?? null;
  const presenceMode = hostedPresence?.mode;
  const checkpointObservedAt = hostedPresence?.checkpointObservedAt;
  // Unit counts are used only while current on this connection (CIT-15 D1).
  const observation = useMemo<ObservationContext>(() => ({
    session,
    connected: gameLoggedIn,
    hostedPresence: presenceMode ? { mode: presenceMode, checkpointObservedAt } : undefined,
  }), [checkpointObservedAt, gameLoggedIn, presenceMode, session]);
  return { state, gameLoggedIn, movement, sessionKey, observation };
}

/**
 * Draft-session key that changes only when the selected account/world really
 * changes. The first observation after open is not a switch, so an edit made
 * while state loads is never discarded.
 */
export function useAccountSessionKey(section: string, key: string): string {
  const [tracked, setTracked] = useState<AccountSessionTracking>({ key: '', generation: 0 });
  const next = advanceAccountSession(tracked, key);
  // Derived-state update during render (React's documented pattern for tracking a changed prop).
  if (next !== tracked) setTracked(next);
  return `${section}:${next.generation}`;
}
