import type { GameStateV2, MovementStateV2 } from '../../api/Contracts';

export type CommanderActivity = 'syncing' | 'unknown' | 'free' | 'outbound' | 'busy' | 'posted' | 'returning';

export interface CommanderStatusRow {
  commanderId: number;
  name: string;
  visiblePosition: number;
  status: CommanderActivity;
  busy: boolean;
  movement: MovementStateV2 | null;
}

export interface MovementViewModel {
  activeMovements: MovementStateV2[];
  commanderStatuses: CommanderStatusRow[];
  snapshotReady: boolean;
  lastSnapshotUnix: number;
  freshnessWindowSec: number;
}

export function movementViewFromState(state: GameStateV2 | null): MovementViewModel | null {
  if (!state) return null;
  const movements = Object.values(state.movements).filter((movement) => isCurrentPlayerMovement(state, movement));
  const byCommander = new Map<number, MovementStateV2>();
  for (const movement of movements) {
    if (movement.commanderId != null && movement.commanderId >= 0) byCommander.set(movement.commanderId, movement);
  }
  const commanderStatuses = Object.values(state.commanders).map((commander): CommanderStatusRow => {
    const movement = byCommander.get(commander.id) ?? null;
    const status: CommanderActivity = movement
      ? movement.direction === 1 ? 'returning' : 'outbound'
      : commander.available ? 'free' : 'busy';
    return {
      commanderId: commander.id,
      name: commander.name ?? '',
      visiblePosition: commander.visiblePosition ?? commander.id,
      status,
      busy: movement != null || !commander.available,
      movement,
    };
  });
  for (const [commanderId, movement] of byCommander) {
    if (commanderStatuses.some((row) => row.commanderId === commanderId)) continue;
    commanderStatuses.push({
      commanderId,
      name: '',
      visiblePosition: commanderId,
      status: movement.direction === 1 ? 'returning' : 'outbound',
      busy: true,
      movement,
    });
  }
  const observation = state.observations.gam;
  return {
    activeMovements: movements,
    commanderStatuses,
    snapshotReady: (observation?.inboundCount ?? 0) > 0,
    lastSnapshotUnix: observation ? Math.floor(Date.parse(observation.lastSeenAt) / 1000) : 0,
    freshnessWindowSec: 45,
  };
}

function isCurrentPlayerMovement(state: GameStateV2, movement: MovementStateV2): boolean {
  if ((movement.ownerPlayerId ?? 0) > 0) {
    return state.player.id > 0 && movement.ownerPlayerId === state.player.id;
  }
  if ((movement.sourceCastleId ?? 0) <= 0) return false;
  return Object.prototype.hasOwnProperty.call(state.castles, movement.sourceCastleId);
}

export function labelTargetType(typeId: number | undefined): string {
  return typeId && typeId > 0 ? `Map type ${typeId}` : '—';
}

export function labelKingdom(kingdomId: number): string {
  return `Kingdom ${kingdomId}`;
}

export function formatTroopSummary(units: Record<string, number> | undefined): string {
  if (!units) return '—';
  const stacks = Object.values(units).filter((amount) => amount > 0);
  if (stacks.length === 0) return '—';
  const total = stacks.reduce((sum, amount) => sum + amount, 0);
  return `${stacks.length} stack${stacks.length === 1 ? '' : 's'} · ${total.toLocaleString()} troops`;
}
