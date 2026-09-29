import { useMemo, useState } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { useAuth } from '../../context/AuthContext';
import { movementViewFromState } from '../../Movement/types/MovementState';
import { accountKey, advanceAccountSession, type AccountSessionTracking } from './castleRequirements';

/**
 * Observations a settings modal needs to evaluate requirements (CIT-18).
 * The movement view is derived from state only: nothing here asks the game to
 * refresh commander movements (`useMovement().refreshMovement` would submit
 * `game.refresh_movements`, so it is deliberately not used).
 */
export function useSetupContext(section: string) {
  const { state } = useCitadelAPI();
  const { gameLoggedIn } = useAuth();
  const movement = useMemo(() => movementViewFromState(state), [state]);
  const sessionKey = useAccountSessionKey(section, accountKey(state));
  return { state, gameLoggedIn, movement, sessionKey };
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
