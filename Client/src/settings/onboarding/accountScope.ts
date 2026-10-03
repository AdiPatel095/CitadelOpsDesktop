import type { GameStateV2 } from '../../api/Contracts';
import { accountKey } from '../requirements/castleRequirements';

/**
 * The account and world the game is currently logged in to, as the key goals and recovered drafts are stored under.
 * Empty until the game has reported both, so nothing is ever attributed to an account that is not known yet.
 */
export function scopeKey(state: GameStateV2 | null | undefined): string {
  const key = accountKey(state ?? null);
  return key === '' || key === ':' || key.startsWith(':') || key.endsWith(':') ? '' : key;
}
