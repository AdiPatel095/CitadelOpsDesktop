import { castleForSettingsKey, castleSettingsEntry, normalizeStormKeys, stormReserveConfigured } from '../stormRole';
import type { CastleStateV2, GameStateV2 } from '../../api/Contracts';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import { aggregateReadiness, type CheckState, type ReadinessCheck, type ReadinessReport } from '../readiness/Readiness';
import { castleMatchesPurpose } from './castleRequirements';
import type { CommanderEligibilityReport } from './commanderEligibility';
import { observationTimestamp, observationUnavailableMessage, unitObservationFreshness, type ObservationContext } from './observationFreshness';
import { evaluateUnitStock, type UnitStockResult } from './unitRequirements';

/**
 * Readiness for the support/combat modules without attack presets (CIT-18):
 * Towers, Fortress, Station/Bird reserves and Food Balance. Pure; the runtime
 * stays authoritative. Only saved values and observed stock are compared — no
 * amounts are invented.
 */

const message = (key: MessageKey): MessageKey => key;
const severity: Record<CheckState, number> = { valid: 0, pending: 1, unavailable: 2, blocked: 3 };

function worst(checks: readonly ReadinessCheck[], fallback: ReadinessCheck): ReadinessCheck {
  return checks.reduce((current, check) => severity[check.state] > severity[current.state] ? check : current, fallback);
}

interface MetadataInput {
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  /** Stock is compared only while unit counts are current (CIT-15 D1). */
  observation: ObservationContext;
}

/** True when castle data is missing or has no castles yet: nothing can be validated against it (waiting for data). */
export function castlesUnobserved(state: GameStateV2 | null): boolean {
  return state == null || Object.keys(state.castles ?? {}).length === 0;
}

const CASTLES_NOT_OBSERVED = message('ui.settings.requirements.setupReadiness.castle.data.has.not.been.observed.yet.76ce81b7');

/**
 * Session-level freshness (checkpoint, disconnected, awaiting baseline) for lines that describe current
 * stock. `troops` keeps the troop wording; `account` is neutral for food, coins and unlocks.
 */
function sessionUnavailable(observation: ObservationContext, subject: 'troops' | 'account' = 'troops'): MessageKey | null {
  const freshness = unitObservationFreshness({ castle: null, ...observation });
  if (freshness.state !== 'unavailable') return null;
  if (subject === 'account' && freshness.reason === 'checkpoint') {
    return message('ui.settings.requirements.setupReadiness.this.is.a.saved.checkpoint.account.data.3513aa23');
  }
  if (subject === 'account' && freshness.reason === 'awaiting-baseline') {
    return message('ui.settings.requirements.setupReadiness.waiting.for.the.game.connection.to.finish.c6e90c18');
  }
  return observationUnavailableMessage(freshness.reason);
}

// ——— Auto Towers ———

export interface TowerCastleInput {
  enabled: boolean;
  unitId: number;
  maidenOnly: boolean;
}

export interface TowerReadinessInput extends MetadataInput {
  state: GameStateV2 | null;
  castles: Readonly<Record<string, TowerCastleInput>>;
  commanders?: CommanderEligibilityReport;
}

export interface TowerReadiness {
  report: ReadinessReport;
  stockByCastle: Record<string, UnitStockResult>;
}

export function evaluateTowerReadiness(input: TowerReadinessInput): TowerReadiness {
  const checks: ReadinessCheck[] = [];
  const enabled = Object.entries(normalizeStormKeys(input.castles, input.state)).filter(([, castle]) => castle.enabled);
  const stockByCastle: Record<string, UnitStockResult> = {};
  const missingCastles = enabled.filter(([castleId]) => input.state != null && castleId !== 'storm' && castleForSettingsKey(castleId, input.state) == null);
  const withoutUnit = enabled.filter(([castleId, castle]) => castle.unitId <= 0 && castleForSettingsKey(castleId, input.state) != null);
  if (enabled.length === 0) {
    checks.push({ id: 'enabled-castles', state: 'blocked', messageKey: message('ui.settings.requirements.setupReadiness.enable.at.least.one.castle.and.choose.f11e9525'), fix: 'settings' });
  } else if (castlesUnobserved(input.state)) {
    // Saved castles cannot be validated without castle data: waiting, not a configuration error.
    checks.push({ id: 'enabled-castles', state: 'unavailable', messageKey: CASTLES_NOT_OBSERVED, fix: 'connection' });
  } else if (missingCastles.length > 0) {
    checks.push({ id: 'enabled-castles', state: 'blocked', messageKey: message('setupReadiness.castlesNotInWorld'), params: { count: missingCastles.length, castle: '#' + missingCastles[0][0], others: missingCastles.length - 1 }, fix: 'settings' });
  } else if (enabled.every(([key]) => key === 'storm') && !castleForSettingsKey('storm', input.state)) {
    checks.push({ id: 'enabled-castles', state: 'pending', messageKey: message('stormRole.waiting') });
  } else if (withoutUnit.length > 0) {
    checks.push({ id: 'enabled-castles', state: 'blocked', messageKey: message('setupReadiness.castlesWithoutTroop'), params: { count: withoutUnit.length }, fix: 'settings' });
  } else {
    checks.push({ id: 'enabled-castles', state: 'valid', messageKey: message('setupReadiness.enabledCastles'), params: { count: enabled.length } });
  }
  for (const [castleId, castle] of enabled) {
    if (castleId === 'storm' && !castleForSettingsKey(castleId, input.state)) continue;
    if (castle.unitId <= 0 || castleForSettingsKey(castleId, input.state) == null) continue;
    stockByCastle[castleId] = evaluateUnitStock({
      observation: input.observation,
      castle: castleForSettingsKey(castleId, input.state) ?? null,
      requests: [{ itemId: castle.unitId, amount: 1, kind: 'troop' }],
      troops: input.troops,
      tools: input.tools,
      metadataReady: input.metadataReady,
      slot: castleId,
      decidedAtLaunch: 'quantity',
      messages: {
        valid: message('ui.settings.requirements.setupReadiness.this.troop.is.stationed.two.full.flanks.e344abc7'),
      },
    });
  }
  const stockChecks = Object.values(stockByCastle).map((result) => result.check);
  if (stockChecks.length > 0) {
    const stock = worst(stockChecks, stockChecks[0]);
    checks.push({ ...stock, id: 'inventory', slot: undefined });
  }
  if (input.commanders) {
    checks.push(input.commanders.activity, input.commanders.assignment);
  }
  if (enabled.some(([, castle]) => castle.maidenOnly)) {
    checks.push({ id: 'maiden-relic', state: 'pending', messageKey: message('ui.settings.requirements.setupReadiness.maiden.only.castles.need.a.commander.with.9c8b410b') });
  }
  return { report: { featureId: 'autoTowers', checks, overall: aggregateReadiness(checks) }, stockByCastle };
}

// ——— Auto Fortress ———

export const FORTRESS_KINGDOMS = [1, 2, 3] as const;

export interface FortressReadinessInput extends MetadataInput {
  state: GameStateV2 | null;
  kingdoms: Readonly<Record<string, { enabled: boolean }>>;
  direwolfId: number;
  direwolfPurchaseLimit: number;
  commanders?: CommanderEligibilityReport;
}

export interface FortressKingdomReadiness {
  kingdomId: number;
  castle: CastleStateV2 | null;
  castleCheck: ReadinessCheck;
  stock: UnitStockResult | null;
}

export interface FortressReadiness {
  report: ReadinessReport;
  kingdoms: FortressKingdomReadiness[];
}

/** The outer-kingdom main castle (slot 12) used by Auto Fortress in that kingdom. */
export function fortressMainCastle(state: GameStateV2 | null, kingdomId: number): CastleStateV2 | null {
  return Object.values(state?.castles ?? {}).find((castle) => castle.kingdomId === kingdomId && castleMatchesPurpose(castle, 'outer-main')) ?? null;
}

/** The Great Empire main castle (kingdom 0, slot type 1). */
export function greatEmpireMainCastle(state: GameStateV2 | null): CastleStateV2 | null {
  return Object.values(state?.castles ?? {}).find((castle) => castle.kingdomId === 0 && castle.slotType === 1) ?? null;
}

export function evaluateFortressReadiness(input: FortressReadinessInput): FortressReadiness {
  const checks: ReadinessCheck[] = [];
  // The supply lane moves owned Direwolves from the Great Empire main castle whether or not
  // purchases are allowed (AutoFortressPolicy), so only no purchases and no owned stock there block.
  const greatEmpireDirewolves = Math.max(0, Number(greatEmpireMainCastle(input.state)?.units?.stationed?.[String(input.direwolfId)]) || 0);
  const supplyAvailable = input.direwolfPurchaseLimit > 0 || greatEmpireDirewolves > 0;
  const kingdoms: FortressKingdomReadiness[] = FORTRESS_KINGDOMS.map((kingdomId) => {
    const castle = fortressMainCastle(input.state, kingdomId);
    const castleCheck: ReadinessCheck = castlesUnobserved(input.state)
      ? { id: 'kingdom-castle', slot: String(kingdomId), state: 'unavailable', messageKey: CASTLES_NOT_OBSERVED, fix: 'connection' }
      : castle
        ? { id: 'kingdom-castle', slot: String(kingdomId), state: 'valid', messageKey: message('ui.settings.requirements.setupReadiness.the.kingdom.main.castle.is.available.25e1efec') }
        : { id: 'kingdom-castle', slot: String(kingdomId), state: 'blocked', messageKey: message('ui.settings.requirements.setupReadiness.main.castle.is.unavailable.or.the.kingdom.552f7db4'), fix: 'settings' };
    const evaluated = castle ? evaluateUnitStock({
      observation: input.observation,
      castle,
      requests: [{ itemId: input.direwolfId, amount: 1, kind: 'troop' }],
      troops: input.troops,
      tools: input.tools,
      metadataReady: input.metadataReady,
      slot: String(kingdomId),
      // The Direwolf supply lane stages purchased or Great Empire Direwolves before launch.
      decidedAtLaunch: supplyAvailable ? 'stock' : 'quantity',
      messages: {
        valid: message('ui.settings.requirements.setupReadiness.direwolves.are.stationed.one.full.flank.wave.69e18c6a'),
        decidedAtLaunch: message('ui.settings.requirements.setupReadiness.direwolves.are.staged.by.the.supply.lane.448002c8'),
      },
    }) : null;
    // Maya (CIT-16): with no purchases and no Great Empire stock, say what supplies Direwolves.
    const stock = evaluated && !supplyAvailable && evaluated.check.state === 'blocked'
      ? { ...evaluated, check: { ...evaluated.check, messageKey: message('ui.settings.requirements.setupReadiness.no.direwolves.are.available.set.direwolves.per.45e831e3'), params: undefined, fix: 'settings' as const } }
      : evaluated;
    return { kingdomId, castle, castleCheck, stock };
  });
  const enabled = kingdoms.filter((kingdom) => input.kingdoms[String(kingdom.kingdomId)]?.enabled);
  const enabledAvailable = enabled.filter((kingdom) => kingdom.castle != null);
  if (castlesUnobserved(input.state)) {
    checks.push({ id: 'enabled-kingdoms', state: 'unavailable', messageKey: CASTLES_NOT_OBSERVED, fix: 'connection' });
  } else if (enabled.length === 0) {
    checks.push({ id: 'enabled-kingdoms', state: 'blocked', messageKey: message('ui.settings.requirements.setupReadiness.enable.at.least.one.available.outer.kingdom.49656995'), fix: 'settings' });
  } else if (enabledAvailable.length < enabled.length) {
    checks.push(worst(enabled.map((kingdom) => kingdom.castleCheck), enabled[0].castleCheck));
  } else {
    checks.push({ id: 'enabled-kingdoms', state: 'valid', messageKey: message('setupReadiness.enabledKingdoms'), params: { count: enabled.length } });
  }
  const stockChecks = enabledAvailable.flatMap((kingdom) => kingdom.stock ? [kingdom.stock.check] : []);
  if (stockChecks.length > 0) checks.push({ ...worst(stockChecks, stockChecks[0]), id: 'inventory', slot: undefined });
  if (input.commanders) {
    checks.push(input.commanders.activity, input.commanders.assignment);
    checks.push({ id: 'commander-speed', state: 'pending', messageKey: message('ui.settings.requirements.setupReadiness.the.fastest.eligible.commander.is.chosen.when.87205aff') });
  }
  return { report: { featureId: 'autoFortress', checks, overall: aggregateReadiness(checks) }, kingdoms };
}

// ——— Auto Station / Auto Bird reserves ———

export interface ReserveReadinessInput extends MetadataInput {
  featureId: 'autoStation' | 'autoBird';
  stormLegacyKey?: string;
  state: GameStateV2 | null;
  reserves: Readonly<Record<string, ReadonlyArray<{ id: number; amount: number }>>>;
}

export interface ReserveReadiness {
  report: ReadinessReport;
  stockByCastle: Record<string, UnitStockResult>;
  castlesNotInWorld: string[];
}

export function evaluateReserveReadiness(input: ReserveReadinessInput): ReserveReadiness {
  const checks: ReadinessCheck[] = [];
  const stockByCastle: Record<string, UnitStockResult> = {};
  const castlesNotInWorld: string[] = [];
  if (!input.state) {
    checks.push({ id: 'castles', state: 'unavailable', messageKey: message('ui.settings.requirements.setupReadiness.castle.data.has.not.been.observed.yet.76ce81b7'), fix: 'connection' });
    return { report: { featureId: input.featureId, checks, overall: aggregateReadiness(checks) }, stockByCastle, castlesNotInWorld };
  }
  if (input.featureId === 'autoBird') {
    const stormCastle = castleForSettingsKey('storm', input.state);
    if (stormCastle && !stormReserveConfigured(castleSettingsEntry(input.reserves, stormCastle))) {
      checks.push({ id: 'storm-reserve', state: 'pending', messageKey: message('stormRole.birdUnconfigured'), params: { castle: stormCastle.name || `castle ${stormCastle.id}` }, fix: 'settings', slot: 'storm' });
    }
  }
  const castleDataObserved = !castlesUnobserved(input.state);
  for (const [castleId, reserves] of Object.entries(normalizeStormKeys(input.reserves, input.state, { stormLegacyKey: input.stormLegacyKey }))) {
    if (reserves.length === 0) continue;
    const castle = castleForSettingsKey(castleId, input.state);
    if (!castle) {
      if (castleId === 'storm') continue;
      // With no castle data yet, a saved reserve cannot be judged "not in this world".
      if (!castleDataObserved) continue;
      castlesNotInWorld.push(castleId);
      continue;
    }
    stockByCastle[castleId] = evaluateUnitStock({
      observation: input.observation,
      castle,
      requests: reserves.map((reserve) => ({ itemId: reserve.id, amount: reserve.amount, kind: 'troop' as const })),
      troops: input.troops,
      tools: input.tools,
      metadataReady: input.metadataReady,
      slot: castleId,
      mode: 'reserve',
    });
  }
  const observedCastles = Object.keys(input.state.castles).length;
  const notCurrent = sessionUnavailable(input.observation);
  checks.push(observedCastles === 0
    ? { id: 'castles', state: 'unavailable', messageKey: CASTLES_NOT_OBSERVED, fix: 'connection' }
    : notCurrent
      ? { id: 'castles', state: 'unavailable', messageKey: notCurrent, fix: 'connection' }
      : { id: 'castles', state: 'valid', messageKey: message('setupReadiness.observedCastles'), params: { count: observedCastles } });
  if (castlesNotInWorld.length > 0) {
    checks.push({ id: 'saved-castles', state: 'pending', messageKey: message('setupReadiness.reservesNotInWorld'), params: { count: castlesNotInWorld.length }, fix: 'settings' });
  }
  const stockChecks = Object.values(stockByCastle).map((result) => result.check);
  if (stockChecks.length > 0) checks.push({ ...worst(stockChecks, stockChecks[0]), id: 'reserves', slot: undefined });
  return { report: { featureId: input.featureId, checks, overall: aggregateReadiness(checks) }, stockByCastle, castlesNotInWorld };
}

// ——— Auto Food Balance ———

export interface FoodBalanceReadinessInput {
  state: GameStateV2 | null;
  /** Food, coins and unlocks are described only while current on this connection (CIT-15 D1). */
  observation: ObservationContext;
  resources: Record<number, MetadataItem>;
  metadataReady: boolean;
  minimumSourceReserve: number;
  minimumCoinReserve: number;
  autoKingdomTransport: boolean;
}

export type FoodCastleRole = 'donor' | 'recipient' | 'unobserved';

export interface FoodCastleRow {
  castleId: number;
  name: string;
  kingdomId: number;
  food: number | null;
  role: FoodCastleRole;
  /** False while the connection is not current, or this castle's own food time predates it: the row shows last-known data. */
  current: boolean;
  /** The castle's real food-state observation time, when the runtime reports one (older runtimes do not). */
  observedAt?: string;
  /** Why the row is last-known: the connection is not current, or this castle's own time predates it. */
  unavailableReason?: MessageKey;
}

export interface FoodBalanceReadiness {
  report: ReadinessReport;
  rows: FoodCastleRow[];
  coins: number | null;
}

/** Official resource id for a JSON key (`F` food, `C1` coins), from the resources catalog. */
export function resourceIdForJsonKey(resources: Record<number, MetadataItem>, jsonKey: string): number | null {
  const match = Object.values(resources).find((item) => item.JSONKey === jsonKey);
  return match ? match.id : null;
}

export function evaluateFoodBalanceReadiness(input: FoodBalanceReadinessInput): FoodBalanceReadiness {
  const checks: ReadinessCheck[] = [];
  const foodId = input.metadataReady ? resourceIdForJsonKey(input.resources, 'F') : null;
  const coinId = input.metadataReady ? resourceIdForJsonKey(input.resources, 'C1') : null;
  const notCurrent = sessionUnavailable(input.observation, 'account');
  const rows: FoodCastleRow[] = Object.values(input.state?.castles ?? {})
    .map((castle) => {
      const balance = foodId != null ? castle.resources?.[String(foodId)] : undefined;
      const food = balance ? Math.max(0, Number(balance.amount) || 0) : null;
      const role: FoodCastleRole = food == null ? 'unobserved' : food > input.minimumSourceReserve ? 'donor' : 'recipient';
      const observedAt = observationTimestamp(castle.foodStateObservedAt);
      const since = observationTimestamp(input.observation.session?.changedAt);
      const beforeConnection = observedAt !== undefined && since !== undefined && Date.parse(observedAt) < Date.parse(since);
      return {
        castleId: castle.id, name: castle.name?.trim() || `#${castle.id}`, kingdomId: castle.kingdomId, food, role,
        current: notCurrent == null && !beforeConnection,
        ...(observedAt ? { observedAt } : {}),
        ...(notCurrent != null || beforeConnection ? { unavailableReason: notCurrent ?? observationUnavailableMessage('stale-before-connection') } : {}),
      };
    })
    .sort((left, right) => left.kingdomId - right.kingdomId || left.castleId - right.castleId);

  if (castlesUnobserved(input.state)) {
    checks.push({ id: 'food-observations', state: 'unavailable', messageKey: CASTLES_NOT_OBSERVED, fix: 'connection' });
    return { report: { featureId: 'autoFoodBalance', checks, overall: aggregateReadiness(checks) }, rows, coins: null };
  }
  if (notCurrent) {
    // Last-known data only: no claims about donors, coins or unlocks until the connection is current.
    checks.push({ id: 'food-observations', state: 'unavailable', messageKey: notCurrent, fix: 'connection' });
    checks.push({ id: 'donors', state: 'unavailable', messageKey: notCurrent, fix: 'connection' });
    checks.push({ id: 'coin-reserve', state: 'unavailable', messageKey: notCurrent, fix: 'connection' });
    if (input.autoKingdomTransport) checks.push({ id: 'kingdom-transport', state: 'unavailable', messageKey: notCurrent, fix: 'connection' });
    return { report: { featureId: 'autoFoodBalance', checks, overall: aggregateReadiness(checks) }, rows, coins: null };
  }
  if (foodId == null) {
    checks.push({ id: 'food-observations', state: 'unavailable', messageKey: message('ui.settings.requirements.setupReadiness.official.resource.data.is.still.loading.4657aec3') });
  } else if (rows.some((row) => row.role === 'unobserved')) {
    checks.push({ id: 'food-observations', state: 'unavailable', messageKey: message('setupReadiness.foodUnobserved'), params: { count: rows.filter((row) => row.role === 'unobserved').length } });
  } else {
    checks.push({ id: 'food-observations', state: 'valid', messageKey: message('ui.settings.requirements.setupReadiness.food.stock.is.observed.in.every.castle.22c2fc66') });
  }
  if (foodId != null && input.state) {
    checks.push(rows.some((row) => row.role === 'donor')
      ? { id: 'donors', state: 'valid', messageKey: message('setupReadiness.donors'), params: { count: rows.filter((row) => row.role === 'donor').length } }
      : { id: 'donors', state: 'pending', messageKey: message('ui.settings.requirements.setupReadiness.no.castle.holds.food.above.the.donor.145a5b24') });
  }
  const coins = coinId != null && input.state?.player?.resources ? Math.max(0, Number(input.state.player.resources[String(coinId)]) || 0) : null;
  if (coins == null) {
    checks.push({ id: 'coin-reserve', state: 'unavailable', messageKey: message('ui.settings.requirements.setupReadiness.coins.have.not.been.observed.yet.417dd506') });
  } else if (coins <= input.minimumCoinReserve) {
    checks.push({ id: 'coin-reserve', state: 'pending', messageKey: message('ui.settings.requirements.setupReadiness.coins.are.at.or.below.the.coin.8bfb9f90') });
  } else {
    checks.push({ id: 'coin-reserve', state: 'valid', messageKey: message('ui.settings.requirements.setupReadiness.coins.are.above.the.coin.reserve.e2151a63') });
  }
  if (input.autoKingdomTransport) {
    const unlocks = Object.values(input.state?.kingdomTransport?.unlocks ?? {});
    checks.push(unlocks.length === 0
      ? { id: 'kingdom-transport', state: 'pending', messageKey: message('ui.settings.requirements.setupReadiness.kingdom.transport.unlocks.have.not.been.observed.f12193f1') }
      : unlocks.some((unlock) => unlock.unlocked)
        ? { id: 'kingdom-transport', state: 'valid', messageKey: message('ui.settings.requirements.setupReadiness.kingdom.transport.is.unlocked.for.at.least.7e4668e2') }
        : { id: 'kingdom-transport', state: 'pending', messageKey: message('ui.settings.requirements.setupReadiness.no.kingdom.transport.is.unlocked.yet.kingdom.a60b0065') });
  }
  return { report: { featureId: 'autoFoodBalance', checks, overall: aggregateReadiness(checks) }, rows, coins };
}
