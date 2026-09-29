import type { GameStateV2 } from '../../api/Contracts';
import type { AttackSetupRef } from '../../attackPresets/AppCreatedPresets';
import {
  summarizeAttackPreset,
  type AttackPresetDocument,
} from '../../attackPresets/AttackPresetTypes';
import type { AttackSetupDraft } from '../../components/AttackSetupModal';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import type { CommanderFeatureConfigurationV2 } from '../../Movement/types/CommanderFeatureAssignments';
import type { MovementViewModel } from '../../Movement/types/MovementState';
import { evaluateCommanderEligibility } from '../requirements/commanderEligibility';
import { evaluateUnitStock, requestsFromComposition } from '../requirements/unitRequirements';
import { aggregateReadiness, type ReadinessCheck, type ReadinessReport } from './Readiness';
import type { ObservationContext } from '../requirements/observationFreshness';

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
  /** Session, connection and hosted presence used to decide whether unit counts are current (D1). */
  observation: ObservationContext;
  /** Saved commander assignments and movement; when present, commander checks follow CIT-18 eligibility. */
  commanders?: {
    assignments: CommanderFeatureConfigurationV2;
    movement: MovementViewModel | null;
    gameLoggedIn: boolean;
  };
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
    checks.push(...attackSlotReadiness({
      slot,
      ref,
      castle,
      document: input.document,
      troops: input.troops,
      tools: input.tools,
      metadataReady: input.metadataReady,
      observation: input.observation,
      // Berimond attacks launch from the camp: transfers move troops there and the armorer lane buys
      // coin tools there, so source-castle stock alone cannot decide this before launch.
      ...(input.featureId === 'autoBeriWorld'
        ? { decidedAtLaunch: 'stock' as const, decidedAtLaunchMessage: message('ui.settings.readiness.eventAttackReadiness.berimond.camp.stock.is.checked.at.launch.e26aa185') }
        : {}),
    }));
  }

  if (input.difficulties) {
    const { selections, achievementsObserved, loading } = input.difficulties;
    if (loading && selections.every((selection) => !selection.available)) {
      checks.push({ id: 'difficulty', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.official.event.difficulties.are.still.loading.c5abdc91') });
    } else if (selections.some((selection) => !selection.available)) {
      checks.push({ id: 'difficulty', state: 'blocked', messageKey: message('ui.settings.readiness.eventAttackReadiness.choose.an.unlocked.difficulty.for.every.event.005d1686'), fix: 'settings' });
    } else if (!achievementsObserved) {
      checks.push({ id: 'difficulty', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.achievements.are.still.syncing.unlocked.difficulties.are.183b4a1f') });
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
  if (input.commanders) {
    // Assignment-aware preview (CIT-18): which assigned, qualified commanders exist and are free.
    const report = evaluateCommanderEligibility({
      featureId: input.featureId,
      state,
      assignments: input.commanders.assignments,
      movement: input.commanders.movement,
      gameLoggedIn: input.commanders.gameLoggedIn,
      now: input.now ?? Date.now(),
    });
    checks.push(report.activity, report.assignment);
  } else if (!state || commanders.length === 0) {
    checks.push({ id: 'commanders', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.commanders.have.not.been.observed.yet.44c4e9ab'), fix: 'connection' });
  } else if (commanders.some((commander) => commander.available)) {
    checks.push({ id: 'commanders', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.at.least.one.commander.is.available.now.229f8af6') });
  } else {
    checks.push({ id: 'commanders', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.no.commander.is.available.right.now.the.8b8f881e') });
  }
  if (!input.commanders) {
    checks.push({ id: 'commander-assignment', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.commanders.assigned.to.this.automation.under.commanders.462970de'), fix: 'assignment' });
  }
  checks.push({ id: 'tool-compatibility', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.tool.compatibility.with.each.target.is.checked.95eb4eaa') });

  if (draft.dailyAttackLimit !== undefined) checks.push(dailyLimitCheck(draft.dailyAttackLimit, state));

  if (draft.horseTravelBoostId !== undefined) checks.push(travelBoostCheck(draft.horseTravelBoostId));

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

export interface AttackSlotReadinessInput {
  slot: string;
  ref: AttackSetupRef;
  /** Castle whose stationed stock supplies the attack (Storm: Storm castle plus enabled donors). */
  castle: GameStateV2['castles'][string] | null;
  document: AttackPresetDocument;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  observation: ObservationContext;
  /** `stock`: the module refills stock before launch, so a shortage is decided at launch. */
  decidedAtLaunch?: 'stock';
  decidedAtLaunchMessage?: MessageKey;
}

/**
 * Composition and stationed-stock checks for one attack slot (CIT-15 events,
 * reused by Khan and Storm in CIT-16). Every module policy limits lanes to
 * capacity before its inventory check, so a raw quantity above stock is pending.
 */
export function attackSlotReadiness(input: AttackSlotReadinessInput): ReadinessCheck[] {
  const { slot, ref } = input;
  const checks: ReadinessCheck[] = [];
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
  if (composition) checks.push(inventoryCheck(slot, composition, input));
  return checks;
}

export function dailyLimitCheck(dailyAttackLimit: number, state: GameStateV2 | null): ReadinessCheck {
  const daily = state?.dailyAttacks;
  const observed = Boolean(daily?.observedAt && !daily.observedAt.startsWith('0001-01-01'));
  if (dailyAttackLimit <= 0) {
    return { id: 'daily-limit', state: 'valid', messageKey: message('ui.settings.readiness.eventAttackReadiness.no.daily.attack.limit.is.set.0863264a') };
  }
  if (!observed || !daily) {
    return { id: 'daily-limit', state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.the.server.daily.attack.count.has.not.e5f46250') };
  }
  if (daily.count >= dailyAttackLimit) {
    return { id: 'daily-limit', state: 'blocked', messageKey: message('eventAttackReadiness.dailyLimitReached'), params: { count: daily.count, limit: dailyAttackLimit }, fix: 'settings' };
  }
  return { id: 'daily-limit', state: 'valid', messageKey: message('eventAttackReadiness.dailyLimit'), params: { count: daily.count, limit: dailyAttackLimit } };
}

export function travelBoostCheck(horseTravelBoostId: number): ReadinessCheck {
  const boost = horseTravelBoostId === 1007 ? 'coins' : horseTravelBoostId === 1008 || horseTravelBoostId === 1009 ? 'rubies' : 'feather';
  return { id: 'horse-travel-boost', state: 'valid', messageKey: message('eventAttackReadiness.travelBoost'), params: { boost } };
}

function inventoryCheck(
  slot: string,
  composition: AttackSetupDraft,
  input: AttackSlotReadinessInput,
): ReadinessCheck {
  const castle = input.castle;
  if (!castle) {
    return { id: 'inventory', slot, state: 'unavailable', messageKey: message('ui.settings.readiness.eventAttackReadiness.stationed.troops.are.unknown.until.the.source.335de03d') };
  }
  const { check, freshness } = evaluateUnitStock({
    castle,
    observation: input.observation,
    requests: requestsFromComposition(composition),
    troops: input.troops,
    tools: input.tools,
    metadataReady: input.metadataReady,
    useTroopFamilies: composition.useTroopFamilies,
    slot,
    decidedAtLaunch: input.decidedAtLaunch,
    messages: {
      unobserved: message('ui.settings.readiness.eventAttackReadiness.stationed.troops.are.unknown.until.the.source.335de03d'),
      familiesLoading: message('ui.settings.readiness.eventAttackReadiness.troop.family.data.is.still.loading.74980d2f'),
      ...(input.decidedAtLaunchMessage ? { decidedAtLaunch: input.decidedAtLaunchMessage } : {}),
    },
  });
  if (check.state !== 'valid' || freshness?.state !== 'observed') return check;
  return freshness.scope === 'castle' && freshness.observedAt
    ? { ...check, messageKey: message('eventAttackReadiness.inventoryObservedAt'), params: { observedAt: Date.parse(freshness.observedAt) } }
    // No per-castle time reaches the client today: counts are from this connection's baseline.
    : { ...check, messageKey: message('ui.settings.readiness.eventAttackReadiness.the.source.castle.has.the.troops.and.57e05dcd') };
}

function gallantryBoosterActive(state: GameStateV2, now: number): boolean {
  const booster = state.market?.boosters?.[GALLANTRY_BOOSTER_ID];
  if (!booster) return false;
  if (booster.permanent === true) return true;
  const expiresAt = booster.expiresAt ? Date.parse(booster.expiresAt) : 0;
  return Number.isFinite(expiresAt) && expiresAt > now;
}
