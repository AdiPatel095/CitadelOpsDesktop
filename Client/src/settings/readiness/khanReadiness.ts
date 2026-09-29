import type { GameStateV2 } from '../../api/Contracts';
import type { AttackSetupRef } from '../../attackPresets/AppCreatedPresets';
import type { AttackPresetDocument } from '../../attackPresets/AttackPresetTypes';
import type { MetadataItem } from '../../context/MetadataContext';
import type { DefenseSetupRef } from '../../defensePresets/AppCreatedDefensePresets';
import { summarizeDefensePreset, type DefensePresetDocument, type DefensePresetDraft } from '../../defensePresets/DefensePresetTypes';
import type { MessageKey } from '../../i18n/messages';
import type { AutoKhanClientStateV1 } from '../AutoKhanClientState';
import type { CommanderEligibilityReport } from '../requirements/commanderEligibility';
import { evaluateCastleReference } from '../requirements/castleRequirements';
import type { ObservationContext } from '../requirements/observationFreshness';
import { castlesUnobserved, greatEmpireMainCastle } from '../requirements/setupReadiness';
import { attackSlotReadiness, dailyLimitCheck, travelBoostCheck } from './eventAttackReadiness';
import { aggregateReadiness, type ReadinessCheck, type ReadinessPlanLine, type ReadinessReport } from './Readiness';

/** Khan settings the review reads (saved shape). */
export type KhanReadinessDraft = Pick<AutoKhanClientStateV1,
  | 'sourceCastleId' | 'skipCooldowns' | 'timeSkipReserve' | 'openGateProtection' | 'offensiveUnitThreshold'
  | 'replenishDefenseTools' | 'maxRageChain' | 'requireActiveRageBooster' | 'triggerRage' | 'attackLaunchesEnabled'
  | 'nomadPointThreshold' | 'dailyAttackLimit' | 'horseTravelBoostId'>;

export interface KhanReadinessInput {
  draft: KhanReadinessDraft;
  attack: AttackSetupRef;
  defense: DefenseSetupRef;
  state: GameStateV2 | null;
  attackDocument: AttackPresetDocument;
  defenseDocument: DefensePresetDocument;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  observation: ObservationContext;
  commanders?: CommanderEligibilityReport;
  now?: number;
}

const RAGE_BOOSTER_ID = '27';

const message = (key: MessageKey): MessageKey => key;

/**
 * Non-action review of Auto Khan (CIT-16): the resolved multi-purpose plan
 * (defense reapplication, limits, protection, skips, purchase policy) and the
 * checks that decide whether it can run. Mirrors `Server/Automation/AutoKhanPolicy.go`
 * requirements (attack and defense presets, Great Empire main castle, skips);
 * the runtime stays authoritative and nothing here sends a game action.
 */
export function evaluateKhanReadiness(input: KhanReadinessInput): ReadinessReport {
  const { draft, state } = input;
  const checks: ReadinessCheck[] = [];
  const plan: ReadinessPlanLine[] = [];
  const mainCastle = greatEmpireMainCastle(state);
  const source = state?.castles?.[String(draft.sourceCastleId)] ?? null;
  const sourceIsMain = mainCastle != null && draft.sourceCastleId === mainCastle.id;

  checks.push(evaluateCastleReference({ castleId: draft.sourceCastleId, state, purpose: 'source-great-empire' }));
  if (castlesUnobserved(state)) {
    // No castle observed yet (first sync, offline server): waiting for data, not a configuration error.
    checks.push({ id: 'main-castle', state: 'unavailable', messageKey: message('ui.settings.requirements.castleRequirements.castle.data.has.not.been.observed.yet.76ce81b7'), fix: 'connection' });
  } else if (!mainCastle) {
    checks.push({ id: 'main-castle', state: 'blocked', messageKey: message('ui.settings.readiness.khanReadiness.the.great.empire.main.castle.is.not.94e41079') });
  } else {
    checks.push({ id: 'main-castle', state: 'valid', messageKey: message('khanReadiness.mainCastle'), params: { castle: mainCastle.name?.trim() || `#${mainCastle.id}` } });
  }

  checks.push(...attackSlotReadiness({
    slot: 'attack',
    ref: input.attack,
    castle: source,
    document: input.attackDocument,
    troops: input.troops,
    tools: input.tools,
    metadataReady: input.metadataReady,
    observation: input.observation,
  }));

  const defense = defenseComposition(input.defense, input.defenseDocument);
  if (input.defense.source === 'none') {
    checks.push({ id: 'defense-composition', slot: 'defense', state: 'blocked', messageKey: message('ui.settings.readiness.khanReadiness.no.main.castle.defense.is.chosen.pick.d5b8b9f5'), fix: 'presets' });
  } else if (!defense) {
    checks.push({ id: 'defense-composition', slot: 'defense', state: 'blocked', messageKey: message('ui.settings.readiness.khanReadiness.the.selected.defense.preset.does.not.exist.1708708f'), fix: 'presets' });
  } else {
    const summary = summarizeDefensePreset(defense);
    checks.push({ id: 'defense-composition', slot: 'defense', state: 'valid', messageKey: message('defenseSetup.summary'), params: { tools: summary.toolAmount, types: summary.toolTypes.length } });
    if (summary.toolAmount > 0) {
      checks.push(draft.replenishDefenseTools
        ? { id: 'defense-tool-stock', slot: 'defense', state: 'pending', messageKey: message('ui.settings.readiness.khanReadiness.missing.defense.tools.are.bought.when.the.bc51f5ea') }
        : { id: 'defense-tool-stock', slot: 'defense', state: 'pending', messageKey: message('ui.settings.readiness.khanReadiness.defense.tool.stock.is.checked.when.the.62123ba3') });
    }
  }

  const reserveSizes = Object.values(draft.timeSkipReserve ?? {}).filter((amount) => amount > 0).length;
  checks.push(draft.skipCooldowns
    ? { id: 'skip-cooldowns', state: 'valid', messageKey: message('khanReadiness.skipReserve'), params: { count: reserveSizes } }
    : { id: 'skip-cooldowns', state: 'blocked', messageKey: message('ui.settings.components.autoKhanSettingsModal.cooldown.skipping.is.required.before.these.chained.0dfd850e'), fix: 'settings' });

  const protection = state?.khan?.protection;
  if (protection?.active) {
    checks.push({ id: 'protection', state: 'blocked', messageKey: message('ui.settings.readiness.khanReadiness.auto.khan.is.safety.locked.until.the.e143dc70') });
  }

  if (draft.requireActiveRageBooster) {
    if (!state?.market?.boostersObservedAt) {
      checks.push({ id: 'rage-booster', state: 'pending', messageKey: message('ui.settings.readiness.khanReadiness.waiting.for.the.first.booster.snapshot.camp.e381d819') });
    } else if (rageBoosterActive(state, input.now ?? Date.now())) {
      checks.push({ id: 'rage-booster', state: 'valid', messageKey: message('ui.settings.readiness.khanReadiness.the.khan.rage.points.booster.is.active.a60a4437') });
    } else {
      checks.push({ id: 'rage-booster', state: 'blocked', messageKey: message('ui.settings.readiness.khanReadiness.no.khan.rage.points.booster.is.active.a2020dff'), fix: 'settings' });
    }
  }

  if (input.commanders) checks.push(input.commanders.activity, input.commanders.assignment);
  checks.push({ id: 'tool-compatibility', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.tool.compatibility.with.each.target.is.checked.95eb4eaa') });
  checks.push(dailyLimitCheck(draft.dailyAttackLimit, state));
  checks.push(travelBoostCheck(draft.horseTravelBoostId));

  // The resolved plan: existing advanced effects, stated before Start.
  plan.push({ id: 'defense-reapply', messageKey: message('ui.settings.components.autoKhanSettingsModal.the.selected.defense.preset.is.re.applied.e0cc99f5') });
  if (!draft.attackLaunchesEnabled) plan.push({ id: 'attacks-locked', messageKey: message('ui.settings.components.autoKhanSettingsModal.stops.only.auto.khan.s.own.attack.284cf413') });
  plan.push(draft.triggerRage
    ? { id: 'trigger-rage', messageKey: message('ui.settings.readiness.khanReadiness.at.full.rage.auto.khan.dispatches.the.eacd290a') }
    : { id: 'trigger-rage', messageKey: message('ui.settings.readiness.khanReadiness.at.full.rage.the.khan.retaliation.is.49a756dd') });
  if (draft.maxRageChain > 0) plan.push({ id: 'rage-chain-limit', messageKey: message('khanReadiness.rageChainLimit'), params: { count: draft.maxRageChain } });
  if (draft.nomadPointThreshold > 0) plan.push({ id: 'nomad-point-limit', messageKey: message('khanReadiness.nomadPointLimit'), params: { points: draft.nomadPointThreshold } });
  if (sourceIsMain && draft.openGateProtection) {
    plan.push({ id: 'open-gate', messageKey: message('khanReadiness.openGate'), params: { threshold: draft.offensiveUnitThreshold } });
  }
  plan.push(draft.replenishDefenseTools
    ? { id: 'purchase-policy', messageKey: message('ui.settings.components.autoKhanSettingsModal.ruby.priced.packages.are.rejected.auto.khan.8c84936b') }
    : { id: 'purchase-policy', messageKey: message('ui.settings.readiness.khanReadiness.defense.tools.are.never.bought.7ee2a7e0') });
  plan.push({ id: 'station-precedence', messageKey: message('ui.settings.components.autoKhanSettingsModal.auto.station.has.precedence.any.incoming.player.19ee005a') });

  return { featureId: 'autoKhan', checks, overall: aggregateReadiness(checks), plan };
}

function defenseComposition(ref: DefenseSetupRef, document: DefensePresetDocument): DefensePresetDraft | null {
  if (ref.source === 'none') return null;
  if (ref.source === 'inline') return { name: '', ...ref.setup };
  if (ref.missing) return null;
  return document.presets.find((preset) => preset.id === ref.presetId) ?? null;
}

function rageBoosterActive(state: GameStateV2, now: number): boolean {
  const booster = state.market?.boosters?.[RAGE_BOOSTER_ID];
  if (!booster) return false;
  if (booster.permanent === true) return true;
  const expiresAt = booster.expiresAt ? Date.parse(booster.expiresAt) : 0;
  return Number.isFinite(expiresAt) && expiresAt > now;
}
