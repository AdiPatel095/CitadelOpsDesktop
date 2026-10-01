import { PLAYER_STATUS_ROLE, connectionPlayerStatus } from '../settings/readiness/playerStatus';
import type { PlayerStatusDescription, ConnectionPlayerStatusInput } from '../settings/readiness/playerStatus';
const ROLE_PRIORITY = { danger: 5, warning: 4, info: 3, success: 2, neutral: 1 };
/** Shared display policy: the spec's most severe semantic role, with stable ties. */
export function mostSeverePlayerStatus(values: readonly PlayerStatusDescription[], fallback: PlayerStatusDescription): PlayerStatusDescription {
  return values.reduce((best, value) => ROLE_PRIORITY[PLAYER_STATUS_ROLE[value.status]] > ROLE_PRIORITY[PLAYER_STATUS_ROLE[best.status]] ? value : best, values[0] ?? fallback);
}
/** Both forked headers pass their observed session and surface/account context here. */
export function headerPlayerStatus(input: ConnectionPlayerStatusInput): PlayerStatusDescription {
  return connectionPlayerStatus(input);
}
