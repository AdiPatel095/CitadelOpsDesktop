import type { CastleStateV2 } from '../../api/Contracts';
import type { AttackSetupDraft } from '../../components/AttackSetupModal';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import type { ReadinessCheck } from '../readiness/Readiness';
import { unitUpgradeFamily } from '../UnitUpgradeFamily';
import {
  observationUnavailableMessage,
  unitObservationFreshness,
  type ObservationContext,
  type ObservationFreshness,
} from './observationFreshness';

/**
 * Stationed stock versus what a setup names (CIT-18). Pure; reads observed
 * castle units only. The runtime stays authoritative for capacity and launch.
 */

export type UnitStockKind = 'troop' | 'tool';
export type UnitStockLineState = 'valid' | 'short' | 'missing' | 'unknown';

export interface UnitStockRequest {
  itemId: number;
  amount: number;
  kind: UnitStockKind;
}

export interface UnitStockLine {
  itemId: number;
  kind: UnitStockKind;
  required: number;
  stationed: number;
  state: UnitStockLineState;
}

export interface UnitStockMessages {
  valid?: MessageKey;
  unobserved?: MessageKey;
  familiesLoading?: MessageKey;
  /** Used when a shortage is decided at launch (see `decidedAtLaunch`). */
  decidedAtLaunch?: MessageKey;
}

export interface UnitStockInput {
  castle: CastleStateV2 | null;
  /** Session, connection and hosted presence; stock counts are used only while current (CIT-15 D1). */
  observation: ObservationContext;
  requests: readonly UnitStockRequest[];
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  useTroopFamilies?: boolean;
  id?: string;
  slot?: string;
  /**
   * `required`: the setup sends these amounts. `reserve`: these amounts stay in
   * the castle, so a reserve above stock only means nothing is sent yet.
   */
  mode?: 'required' | 'reserve';
  /**
   * `quantity`: amounts are sized at launch (lane capacity), so a shortage is pending.
   * `stock`: stock is refilled before launch (transfers, purchases), so missing stock is pending too.
   */
  decidedAtLaunch?: 'quantity' | 'stock';
  messages?: UnitStockMessages;
}

export interface UnitStockResult {
  check: ReadinessCheck;
  /** Empty unless the counts are current. */
  lines: UnitStockLine[];
  /** Null when there is no castle. */
  freshness: ObservationFreshness | null;
}

const message = (key: MessageKey): MessageKey => key;

export function evaluateUnitStock(input: UnitStockInput): UnitStockResult {
  const id = input.id ?? 'inventory';
  const base = input.slot ? { id, slot: input.slot } : { id };
  const mode = input.mode ?? 'required';
  const castle = input.castle;
  if (!castle) {
    return {
      check: { ...base, state: 'unavailable', messageKey: input.messages?.unobserved ?? message('ui.settings.requirements.unitRequirements.stationed.troops.are.unknown.until.the.castle.1e3d382e') },
      lines: [],
      freshness: null,
    };
  }
  // The projection zeroes unitsObservedAt, so its presence proves nothing (CIT-15 D1).
  const freshness = unitObservationFreshness({ castle, ...input.observation });
  if (freshness.state === 'unavailable') {
    return {
      check: { ...base, state: 'unavailable', messageKey: observationUnavailableMessage(freshness.reason), fix: 'connection' },
      lines: [],
      freshness,
    };
  }
  if (input.useTroopFamilies && !input.metadataReady) {
    return {
      check: { ...base, state: 'unavailable', messageKey: input.messages?.familiesLoading ?? message('ui.settings.requirements.unitRequirements.troop.family.data.is.still.loading.74980d2f') },
      lines: [],
      freshness,
    };
  }
  const stationed = castle.units?.stationed ?? {};
  const lines: UnitStockLine[] = mergeRequests(input.requests).map((request) => {
    const ids = request.kind === 'troop' && input.useTroopFamilies
      ? unitUpgradeFamily(request.itemId, input.troops)?.ids ?? [request.itemId]
      : [request.itemId];
    const stock = ids.reduce((total, itemId) => total + Math.max(0, Number(stationed[String(itemId)]) || 0), 0);
    const known = request.kind === 'troop' ? input.troops[request.itemId] != null : input.tools[request.itemId] != null;
    const state: UnitStockLineState = mode === 'reserve' && input.metadataReady && !known
      ? 'unknown'
      : stock >= request.amount ? 'valid' : stock <= 0 && mode === 'required' ? 'missing' : 'short';
    return { itemId: request.itemId, kind: request.kind, required: request.amount, stationed: stock, state };
  });
  const count = (state: UnitStockLineState) => lines.filter((line) => line.state === state).length;
  const unknown = count('unknown');
  const missing = count('missing');
  const short = count('short');

  if (mode === 'reserve') {
    if (unknown > 0) {
      return { lines, freshness, check: { ...base, state: 'blocked', messageKey: message('unitStock.unknownUnits'), params: { count: unknown }, fix: 'settings' } };
    }
    if (short > 0) {
      return { lines, freshness, check: { ...base, state: 'pending', messageKey: message('unitStock.reserveAboveStock'), params: { count: short } } };
    }
    return { lines, freshness, check: { ...base, state: 'valid', messageKey: input.messages?.valid ?? message('ui.settings.requirements.unitRequirements.stationed.stock.covers.every.reserve.c4f36080') } };
  }

  if ((missing > 0 || short > 0) && input.decidedAtLaunch === 'stock') {
    return { lines, freshness, check: { ...base, state: 'pending', messageKey: input.messages?.decidedAtLaunch ?? message('ui.settings.requirements.unitRequirements.stock.is.refilled.and.checked.at.launch.34262f8a') } };
  }
  if (missing > 0) {
    return { lines, freshness, check: { ...base, state: 'blocked', messageKey: message('eventAttackReadiness.inventoryMissing'), params: { count: missing }, fix: 'settings' } };
  }
  if (short > 0) {
    return input.decidedAtLaunch === 'quantity' && input.messages?.decidedAtLaunch
      ? { lines, freshness, check: { ...base, state: 'pending', messageKey: input.messages.decidedAtLaunch } }
      // The runtime limits each lane to its capacity before checking stock, so a raw
      // quantity above stock is decided at launch rather than here.
      : { lines, freshness, check: { ...base, state: 'pending', messageKey: message('eventAttackReadiness.inventoryShort'), params: { count: short } } };
  }
  return { lines, freshness, check: { ...base, state: 'valid', messageKey: input.messages?.valid ?? message('ui.settings.requirements.unitRequirements.the.castle.has.the.troops.and.tools.fe3e4d59') } };
}

/** Troop and tool amounts named by an attack composition (waves plus courtyard support). */
export function requestsFromComposition(composition: AttackSetupDraft): UnitStockRequest[] {
  const requests: UnitStockRequest[] = [];
  const add = (kind: UnitStockKind, itemId: number | null, quantity: number) => {
    if (itemId == null || quantity <= 0) return;
    requests.push({ itemId, amount: quantity, kind });
  };
  for (const wave of composition.waves) {
    for (const lane of [wave.L, wave.M, wave.R]) {
      for (const slot of lane.troops) add('troop', slot.itemId, slot.quantity);
      for (const slot of lane.tools) add('tool', slot.itemId, slot.quantity);
    }
  }
  for (const slot of composition.courtyardSupport?.troops ?? []) add('troop', slot.itemId, slot.quantity);
  for (const slot of composition.courtyardSupport?.tools ?? []) add('tool', slot.itemId, slot.itemId == null ? 0 : 1);
  return requests;
}

function mergeRequests(requests: readonly UnitStockRequest[]): UnitStockRequest[] {
  const merged = new Map<string, UnitStockRequest>();
  for (const request of requests) {
    if (!Number.isInteger(request.itemId) || request.itemId <= 0) continue;
    const key = `${request.kind}:${request.itemId}`;
    const current = merged.get(key);
    if (current) current.amount += Math.max(0, request.amount);
    else merged.set(key, { ...request, amount: Math.max(0, request.amount) });
  }
  return [...merged.values()];
}
