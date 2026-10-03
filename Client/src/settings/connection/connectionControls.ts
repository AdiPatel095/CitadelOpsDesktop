import type { SessionStateV2 } from '../../api/Contracts';

/**
 * Pieces of the game-connection controls that both the Settings view and the connection repair dialog use (CIT-19), so
 * "Re-enable saved login" behaves the same in both places.
 */
export type GameConnectionMode = 'full' | 'background';

export const SAVED_LOGIN_DISABLED_MARKER = 'saved login that has been disabled';

/** Background mode, connected socket without a login, and the game says the saved login was disabled. */
export function backgroundLoginNeedsReauthorization(
  configuredMode: GameConnectionMode,
  session: Pick<SessionStateV2, 'mode' | 'loggedIn' | 'detail'> | null | undefined,
): boolean {
  const active: GameConnectionMode = session?.mode === 'background' ? 'background' : 'full';
  return configuredMode === 'background'
    && active === 'background'
    && !session?.loggedIn
    && Boolean(session?.detail?.toLowerCase().includes(SAVED_LOGIN_DISABLED_MARKER));
}

type SubmitIntent = (name: string, args?: Record<string, unknown>) => Promise<unknown>;

/** The two existing intents behind "Re-enable saved login". */
export async function reauthorizeSavedLogin(submitIntent: SubmitIntent): Promise<void> {
  await submitIntent('session.background.prepare');
  await submitIntent('session.start');
}

export interface ConnectionControlAvailability {
  /** The connection is not running or starting: "Start Bot" is offered. */
  canStart: boolean;
  /** The runtime is waiting to reconnect on its own: an early retry is offered. */
  canReconnect: boolean;
}

/** The same conditions the header uses for its Start Bot and Reconnect buttons. */
export function connectionControlAvailability(gameConnectionState: string, hasGameConnectionStatus: boolean): ConnectionControlAvailability {
  const active = hasGameConnectionStatus && ['connecting', 'authenticating', 'connected', 'cooldown', 'reconnecting', 'suspended', 'released'].includes(gameConnectionState);
  const canReconnect = hasGameConnectionStatus && ['cooldown', 'reconnecting', 'suspended', 'released'].includes(gameConnectionState);
  return { canStart: !active, canReconnect };
}
