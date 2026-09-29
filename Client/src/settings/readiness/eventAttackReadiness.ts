import type { GameStateV2 } from '../../api/Contracts';
import type { AttackSetupRef } from '../../attackPresets/AppCreatedPresets';
import {
  summarizeAttackPreset,
  type AttackPresetDocument,
} from '../../attackPresets/AttackPresetTypes';
import type { AttackSetupDraft } from '../../components/AttackSetupModal';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import { unitUpgradeFamily } from '../UnitUpgradeFamily';
import { aggregateReadiness, type ReadinessCheck, type ReadinessReport } from './Readiness';

export type EventAttackFeatureId = 'autoNomad' | 'autoInvasion' | 'autoBeriWorld';

export interface EventAttackReadinessSlot {
  slot: string;
  ref: AttackSetupRef;
}

/** Saved-shape values the checks read. Omitted optional fields skip their checks. */
export interface EventAttackReadinessDraft {
  sourceCastleId: number;
  slots: readonly EventAttackReadinessSlot[];
  scoreTarget?: number;
  dailyAttackLimit?: number;
  horseTravelBoostId?: number;
  requireActiveGallantryBooster?: boolean;
  fortifyCurrency?: string;
}

export interface EventAttackDifficultyInput {
  selections: ReadonlyArray<{ eventId: number; available: boolean }>;
  achievementsObserved: boolean;
  loading: boolean;
}

export interface EventAttackReadinessInput {
  featureId: EventAttackFeatureId;
  draft: EventAttackReadinessDraft;
  state: GameStateV2 | null;
  document: AttackPresetDocument;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  difficulties?: EventAttackDifficultyInput;
  now?: number;
}

const GALLANTRY_BOOSTER_ID = '24';

const message = (key: MessageKey): MessageKey => key;

/** Non-action readiness for event attack modules. Reads saved-shape values and observations only. */
export function evaluateEventAttackReadiness(input: EventAttackReadinessInput): ReadinessReport {
  const { draft, state } = input;
  const checks: ReadinessCheck[] = [];
  const castle = state?.castles?.[String(draft.sourceCastleId)] ?? null;

  if (!state) {
    checks.push({ id: 'source-castle', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.castle.data.has.not.been.observed.yet.76ce81b7'), fix: 'connection' });
  } else if (draft.sourceCastleId <= 0) {
    checks.push({ id: 'source-castle', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.choose.a.source.castle.e8d29521'), fix: 'settings' });
  } else if (!castle) {
    checks.push({ id: 'source-castle', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.the.chosen.source.castle.is.not.in.9591dcc4'), fix: 'settings' });
  } else if (castle.kingdomId !== 0) {
    checks.push({ id: 'source-castle', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.the.source.castle.must.be.in.the.949334c8'), fix: 'settings' });
  } else {
    checks.push({ id: 'source-castle', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.source.castle.is.in.the.great.empire.e32f7618') });
  }

  for (const { slot, ref } of draft.slots) {
    const composition = compositionOf(ref, input.document);
    if (ref.source === 'none') {
      checks.push({ id: 'composition', slot, state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.no.attack.setup.is.chosen.pick.a.571c87b6'), fix: 'presets' });
    } else if (!composition) {
      checks.push({ id: 'composition', slot, state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.the.selected.attack.preset.does.not.exist.dec0caad'), fix: 'presets' });
    } else {
      const summary = summarizeAttackPreset(composition);
      if (summary.troops <= 0) {
        checks.push({ id: 'composition', slot, state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.this.attack.setup.has.no.troops.46b41b58'), fix: 'settings' });
      } else {
        checks.push({
          id: 'composition', slot, state: 'valid', messageKey: message('eventAttackReadiness.composition'),
          params: { waves: summary.waves, troops: summary.troops, tools: summary.tools },
        });
      }
    }
    if (composition) checks.push(inventoryCheck(slot, composition, castle, input));
  }

  if (input.difficulties) {
    const { selections, achievementsObserved, loading } = input.difficulties;
    if (loading && selections.every((selection) => !selection.available)) {
      checks.push({ id: 'difficulty', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.official.event.difficulties.are.still.loading.c5abdc91') });
    } else if (selections.some((selection) => !selection.available)) {
      checks.push({ id: 'difficulty', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.choose.an.unlocked.difficulty.for.every.event.005d1686'), fix: 'settings' });
    } else if (!achievementsObserved) {
      checks.push({ id: 'difficulty', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.achievements.are.still.syncing.the.runtime.confirms.0954fe53') });
    } else {
      checks.push({ id: 'difficulty', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.every.event.has.an.unlocked.difficulty.5d1cac20') });
    }
  }

  if (draft.scoreTarget !== undefined) {
    checks.push(draft.scoreTarget > 0
      ? { id: 'score-target', state: 'valid', messageKey: message('eventAttackReadiness.scoreTarget'), params: { score: draft.scoreTarget } }
      : { id: 'score-target', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.set.the.event.score.at.which.to.d0584bb8'), fix: 'settings' });
  }

  const commanders = Object.values(state?.commanders ?? {});
  if (!state || commanders.length === 0) {
    checks.push({ id: 'commanders', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.commanders.have.not.been.observed.yet.44c4e9ab'), fix: 'connection' });
  } else if (commanders.some((commander) => commander.available)) {
    checks.push({ id: 'commanders', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.at.least.one.commander.is.available.now.229f8af6') });
  } else {
    checks.push({ id: 'commanders', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.no.commander.is.available.right.now.the.8b8f881e') });
  }
  checks.push({ id: 'commander-assignment', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.commanders.assigned.to.this.automation.under.commanders.462970de'), fix: 'assignment' });
  checks.push({ id: 'tool-compatibility', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.tool.compatibility.with.each.target.is.verified.197b0a1d') });

  if (draft.dailyAttackLimit !== undefined) {
    const daily = state?.dailyAttacks;
    const observed = Boolean(daily?.observedAt && !daily.observedAt.startsWith('0001-01-01'));
    if (draft.dailyAttackLimit <= 0) {
      checks.push({ id: 'daily-limit', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.no.daily.attack.limit.is.set.0863264a') });
    } else if (!observed || !daily) {
      checks.push({ id: 'daily-limit', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.the.server.daily.attack.count.has.not.e5f46250') });
    } else if (daily.count >= draft.dailyAttackLimit) {
      checks.push({ id: 'daily-limit', state: 'blocked', messageKey: message('eventAttackReadiness.dailyLimitReached'), params: { count: daily.count, limit: draft.dailyAttackLimit }, fix: 'settings' });
    } else {
      checks.push({ id: 'daily-limit', state: 'valid', messageKey: message('eventAttackReadiness.dailyLimit'), params: { count: daily.count, limit: draft.dailyAttackLimit } });
    }
  }

  if (draft.horseTravelBoostId !== undefined) {
    const boost = draft.horseTravelBoostId === 1007 ? 'coins' : draft.horseTravelBoostId === 1008 || draft.horseTravelBoostId === 1009 ? 'rubies' : 'feather';
    checks.push({ id: 'horse-travel-boost', state: 'valid', messageKey: message('eventAttackReadiness.travelBoost'), params: { boost } });
  }

  if (draft.requireActiveGallantryBooster !== undefined) {
    if (!draft.requireActiveGallantryBooster) {
      checks.push({ id: 'gallantry-booster', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.a.gallantry.booster.is.not.required.da539628') });
    } else if (!state?.market?.boostersObservedAt) {
      checks.push({ id: 'gallantry-booster', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.waiting.for.the.first.booster.snapshot.auto.b7e4f17a') });
    } else if (gallantryBoosterActive(state, input.now ?? Date.now())) {
      checks.push({ id: 'gallantry-booster', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.a.gallantry.booster.is.active.fe6d1905') });
    } else {
      checks.push({ id: 'gallantry-booster', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.no.gallantry.booster.is.active.auto.beri.c76e9a6d'), fix: 'settings' });
    }
  }

  if (draft.fortifyCurrency) {
    // Explicit user choice only. Rubies (C2) are never defaulted here or anywhere else.
    checks.push({ id: 'fortify-currency', state: 'pending', messageKey: message('eventAttackReadiness.fortifyCurrency'), params: { currency: draft.fortifyCurrency } });
  }

  return { featureId: input.featureId, checks, overall: aggregateReadiness(checks) };
}

function compositionOf(ref: AttackSetupRef, document: AttackPresetDocument): AttackSetupDraft | null {
  if (ref.source === 'none') return null;
  if (ref.source === 'inline') return { name: '', ...ref.setup };
  if (ref.missing) return null;
  return document.presets.find((preset) => preset.id === ref.presetId) ?? null;
}

function inventoryCheck(
  slot: string,
  composition: AttackSetupDraft,
  castle: GameStateV2['castles'][string] | null,
  input: EventAttackReadinessInput,
): ReadinessCheck {
  if (!castle?.unitsObservedAt) {
    return { id: 'inventory', slot, state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.stationed.troops.are.unknown.until.the.source.335de03d') };
  }
  if (composition.useTroopFamilies && !input.metadataReady) {
    return { id: 'inventory', slot, state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.troop.family.data.is.still.loading.74980d2f') };
  }
  const requested = requestedItems(composition);
  const stationed = castle.units?.stationed ?? {};
  let missing = 0;
  let short = 0;
  for (const [itemId, amount] of requested.troops) {
    const ids = composition.useTroopFamilies ? unitUpgradeFamily(itemId, input.troops)?.ids ?? [itemId] : [itemId];
    const stock = ids.reduce((total, id) => total + Math.max(0, Number(stationed[String(id)]) || 0), 0);
    if (stock <= 0) missing += 1;
    else if (stock < amount) short += 1;
  }
  for (const [itemId, amount] of requested.tools) {
    const stock = Math.max(0, Number(stationed[String(itemId)]) || 0);
    if (stock <= 0) missing += 1;
    else if (stock < amount) short += 1;
  }
  if (missing > 0) {
    return { id: 'inventory', slot, state: 'blocked', messageKey: message('eventAttackReadiness.inventoryMissing'), params: { count: missing }, fix: 'settings' };
  }
  if (short > 0) {
    // The runtime limits each lane to its capacity before checking stock, so a raw
    // quantity above stock is decided at launch rather than here.
    return { id: 'inventory', slot, state: 'pending', messageKey: message('eventAttackReadiness.inventoryShort'), params: { count: short } };
  }
  return { id: 'inventory', slot, state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.the.source.castle.has.the.troops.and.a73c676a') };
}

function requestedItems(composition: AttackSetupDraft): { troops: Map<number, number>; tools: Map<number, number> } {
  const troops = new Map<number, number>();
  const tools = new Map<number, number>();
  const add = (target: Map<number, number>, itemId: number | null, quantity: number) => {
    if (itemId == null || quantity <= 0) return;
    target.set(itemId, (target.get(itemId) ?? 0) + quantity);
  };
  for (const wave of composition.waves) {
    for (const lane of [wave.L, wave.M, wave.R]) {
      for (const slot of lane.troops) add(troops, slot.itemId, slot.quantity);
      for (const slot of lane.tools) add(tools, slot.itemId, slot.quantity);
    }
  }
  for (const slot of composition.courtyardSupport?.troops ?? []) add(troops, slot.itemId, slot.quantity);
  for (const slot of composition.courtyardSupport?.tools ?? []) add(tools, slot.itemId, slot.itemId == null ? 0 : 1);
  return { troops, tools };
}

function gallantryBoosterActive(state: GameStateV2, now: number): boolean {
  const booster = state.market?.boosters?.[GALLANTRY_BOOSTER_ID];
  if (!booster) return false;
  if (booster.permanent === true) return true;
  const expiresAt = booster.expiresAt ? Date.parse(booster.expiresAt) : 0;
  return Number.isFinite(expiresAt) && expiresAt > now;
}
