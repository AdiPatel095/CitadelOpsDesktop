import type { CastleStateV2, GameStateV2 } from '../../api/Contracts';
import { attackSetupRef } from '../../attackPresets/AppCreatedPresets';
import { ATTACK_PRESETS_SECTION, parseAttackPresetDocument } from '../../attackPresets/AttackPresetTypes';
import type { MetadataItem } from '../../context/MetadataContext';
import { defenseSetupRef } from '../../defensePresets/AppCreatedDefensePresets';
import { DEFENSE_PRESETS_SECTION, parseDefensePresetDocument } from '../../defensePresets/DefensePresetTypes';
import type { MessageKey, MessageParameters } from '../../i18n/messages';
import type { MovementViewModel } from '../../Movement/types/MovementState';
import { AUTO_ADVISOR_SECTION, parseAutoAdvisorClientState } from '../AutoAdvisorClientState';
import { AUTO_BOOSTER_SECTION, parseAutoBoosterClientState } from '../AutoBoosterClientState';
import { parseAutoBeriWorldSettings } from '../AutoBeriWorldClientState';
import { AUTO_BUYER_SECTION, parseAutoBuyerClientState } from '../AutoBuyerClientState';
import { parseAutoBirdClientState } from '../AutoBirdClientState';
import { AUTO_FORTRESS_DIREWOLF_ID, AUTO_FORTRESS_SECTION, parseAutoFortressClientState } from '../AutoFortressClientState';
import { parseAutoFoodBalanceSettings } from '../AutoFoodBalanceClientState';
import { AUTO_INVASION_SECTION, parseAutoInvasionClientState } from '../AutoInvasionClientState';
import { AUTO_KHAN_SECTION, parseAutoKhanClientState } from '../AutoKhanClientState';
import { AUTO_NOMAD_SECTION, parseAutoNomadClientState } from '../AutoNomadClientState';
import { AUTO_STORM_BLUEPRINTS_SECTION, AUTO_STORM_SECTION, parseAutoStormBlueprintDocument, parseAutoStormClientState } from '../AutoStormClientState';
import { parseAutoStationClientState } from '../AutoStationClientState';
import { parseAutoTowerClientState } from '../AutoTowerClientState';
import { normalizeAutoSceatResSettings } from '../AutoSceatResClientState';
import { normalizeRecruitTroopsSettings } from '../RecruitTroopsClientState';
import { normalizeAutoToolSettings } from '../AutoToolClientState';
import { savedCommanderAssignments } from '../requirements/commanderAssignmentDraft';
import { evaluateCastleReference } from '../requirements/castleRequirements';
import { evaluateCommanderEligibility } from '../requirements/commanderEligibility';
import type { ObservationContext } from '../requirements/observationFreshness';
import {
  castlesUnobserved,
  evaluateFoodBalanceReadiness,
  evaluateFortressReadiness,
  evaluateReserveReadiness,
  evaluateTowerReadiness,
} from '../requirements/setupReadiness';
import { scheduleAllowsAt, type WeeklySchedule } from '../SchedulerTypes';
import { evaluateEventAttackReadiness, attackSlotReadiness, type EventAttackDifficultyInput } from './eventAttackReadiness';
import { evaluateKhanReadiness } from './khanReadiness';
import { aggregateReadiness, type ReadinessCheck, type ReadinessPlanLine, type ReadinessReport } from './Readiness';
import { evaluateStormReadiness } from './stormReadiness';

/**
 * Readiness before Start for every automation on the Automation page (CIT-20).
 * One dispatcher over the existing pure evaluators, fed from the SAVED
 * configuration and the current observations (never a modal draft), so the
 * Automation page and the Start check say what the settings editors say.
 * Nothing here sends a game action, writes configuration or submits an
 * intent, and the runtime stays authoritative: `pending` means the game
 * decides at Start or while running, `unavailable` means an observation has
 * not arrived. Modules without an evaluator get `configurationReadiness`,
 * which reports only what the saved section proves.
 */

export interface DifficultyCatalogInput {
  optionsByEvent: Readonly<Record<string, ReadonlyArray<{ value: string }>>>;
  achievementsObserved: boolean;
  loading: boolean;
}

export interface FeatureReadinessInputs {
  /** The saved configuration (`ConfigurationSnapshot.sections`). */
  sections: Record<string, unknown> | undefined;
  state: GameStateV2 | null;
  observation: ObservationContext;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  /** Food Balance reads the resource catalog. */
  resources?: Record<number, MetadataItem>;
  movement: MovementViewModel | null;
  gameLoggedIn: boolean;
  now?: number;
  /** Loaded official event difficulties; without them the difficulty check is decided at launch. */
  difficulties?: DifficultyCatalogInput;
  /** Official Storm starter castles currently on offer (`prebuiltcastles`); undefined while not loaded. */
  stormUnlockOffer?: { loaded: boolean; offeredIds: readonly number[] };
  /** The feature's weekly schedule, when it has one. */
  schedule?: WeeklySchedule;
}

const message = (key: MessageKey): MessageKey => key;

function section(inputs: FeatureReadinessInputs, name: string): unknown {
  return inputs.sections?.[name];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function finish(featureId: string, checks: ReadinessCheck[], plan?: ReadinessPlanLine[]): ReadinessReport {
  return { featureId, checks, overall: aggregateReadiness(checks), ...(plan && plan.length > 0 ? { plan } : {}) };
}

function scheduleCheck(inputs: FeatureReadinessInputs): ReadinessCheck[] {
  const schedule = inputs.schedule;
  if (!schedule?.enabled) return [];
  const open = scheduleAllowsAt(schedule, new Date(inputs.now ?? Date.now()));
  return [open
    ? { id: 'schedule', state: 'valid', messageKey: message('featureReadiness.scheduleOpen') }
    : { id: 'schedule', state: 'pending', messageKey: message('featureReadiness.scheduleClosed'), fix: 'settings' }];
}

/** Difficulty availability for Nomad and Invasion; decided at launch when the catalogs were not loaded. */
function difficultyInput(inputs: FeatureReadinessInputs, selections: ReadonlyArray<{ eventId: number; difficultyId: number }>): EventAttackDifficultyInput | undefined {
  const catalog = inputs.difficulties;
  if (!catalog) return undefined;
  return {
    selections: selections.map(({ eventId, difficultyId }) => ({
      eventId,
      available: (catalog.optionsByEvent[String(eventId)] ?? []).some((option) => option.value === String(difficultyId)),
    })),
    achievementsObserved: catalog.achievementsObserved,
    loading: catalog.loading,
  };
}

function withDifficultyAtLaunch(report: ReadinessReport, inputs: FeatureReadinessInputs): ReadinessReport {
  if (inputs.difficulties || report.checks.some((check) => check.id === 'difficulty')) return report;
  const checks = [...report.checks, {
    id: 'difficulty', state: 'pending' as const, messageKey: message('featureReadiness.difficultyAtLaunch'),
  }];
  return { ...report, checks, overall: aggregateReadiness(checks) };
}

function commanders(inputs: FeatureReadinessInputs) {
  return { assignments: savedCommanderAssignments(inputs.sections), movement: inputs.movement, gameLoggedIn: inputs.gameLoggedIn };
}

function eligibility(inputs: FeatureReadinessInputs, featureId: Parameters<typeof evaluateCommanderEligibility>[0]['featureId']) {
  return evaluateCommanderEligibility({
    featureId, state: inputs.state, assignments: savedCommanderAssignments(inputs.sections),
    movement: inputs.movement, gameLoggedIn: inputs.gameLoggedIn, now: inputs.now ?? Date.now(),
  });
}

function greatEmpireCastles(state: GameStateV2 | null): CastleStateV2[] {
  return Object.values(state?.castles ?? {}).filter((castle) => castle.kingdomId === 0).sort((left, right) => left.id - right.id);
}

// ——— Modules with an evaluator ———

function nomadReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoNomadClientState(section(inputs, AUTO_NOMAD_SECTION));
  const document = parseAttackPresetDocument(section(inputs, ATTACK_PRESETS_SECTION));
  const trial = saved.rbcTest.enabled;
  const nomad = attackSetupRef(saved.nomadPresetId, document, AUTO_NOMAD_SECTION, 'nomad');
  const samurai = attackSetupRef(saved.samuraiPresetId, document, AUTO_NOMAD_SECTION, 'samurai');
  const report = evaluateEventAttackReadiness({
    featureId: 'autoNomad',
    draft: {
      sourceCastleId: saved.sourceCastleId,
      slots: trial ? [{ slot: 'nomad', ref: nomad }] : [{ slot: 'nomad', ref: nomad }, { slot: 'samurai', ref: samurai }],
      scoreTarget: trial ? undefined : saved.scoreTarget,
      dailyAttackLimit: saved.dailyAttackLimit,
      horseTravelBoostId: saved.horseTravelBoostId,
    },
    state: inputs.state, document, troops: inputs.troops, tools: inputs.tools, metadataReady: inputs.metadataReady,
    observation: inputs.observation, commanders: commanders(inputs), now: inputs.now,
    difficulties: trial ? undefined : difficultyInput(inputs, [
      { eventId: 72, difficultyId: saved.nomadDifficultyId },
      { eventId: 80, difficultyId: saved.samuraiDifficultyId },
    ]),
  });
  return trial ? report : withDifficultyAtLaunch(report, inputs);
}

function invasionReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoInvasionClientState(section(inputs, AUTO_INVASION_SECTION));
  const document = parseAttackPresetDocument(section(inputs, ATTACK_PRESETS_SECTION));
  const report = evaluateEventAttackReadiness({
    featureId: 'autoInvasion',
    draft: {
      sourceCastleId: saved.sourceCastleId,
      slots: [{ slot: 'attack', ref: attackSetupRef(saved.presetId, document, AUTO_INVASION_SECTION, 'attack') }],
      scoreTarget: saved.scoreTarget,
      dailyAttackLimit: saved.dailyAttackLimit,
      horseTravelBoostId: saved.horseTravelBoostId,
      fortifyCurrency: saved.fortifyCurrency,
    },
    state: inputs.state, document, troops: inputs.troops, tools: inputs.tools, metadataReady: inputs.metadataReady,
    observation: inputs.observation, commanders: commanders(inputs), now: inputs.now,
    difficulties: difficultyInput(inputs, [
      { eventId: 71, difficultyId: saved.foreignLordsDifficultyId },
      { eventId: 103, difficultyId: saved.bloodcrowDifficultyId },
    ]),
  });
  return withDifficultyAtLaunch(report, inputs);
}

function beriReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const settings = parseAutoBeriWorldSettings(section(inputs, 'automation.autoBeriWorld'));
  const document = parseAttackPresetDocument(section(inputs, ATTACK_PRESETS_SECTION));
  const castles = greatEmpireCastles(inputs.state);
  const effectiveSourceId = settings.sourceCastleId || castles.find((castle) => castle.slotType === 1)?.id || 0;
  return evaluateEventAttackReadiness({
    featureId: 'autoBeriWorld',
    draft: {
      sourceCastleId: effectiveSourceId,
      slots: [{ slot: 'attack', ref: attackSetupRef(settings.presetId, document, 'automation.autoBeriWorld', 'attack') }],
      dailyAttackLimit: settings.dailyAttackLimit,
      horseTravelBoostId: settings.horseTravelBoostId,
      requireActiveGallantryBooster: settings.requireActiveGallantryBooster,
    },
    state: inputs.state, document, troops: inputs.troops, tools: inputs.tools, metadataReady: inputs.metadataReady,
    observation: inputs.observation, commanders: commanders(inputs), now: inputs.now,
  });
}

function khanReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoKhanClientState(section(inputs, AUTO_KHAN_SECTION));
  const attackDocument = parseAttackPresetDocument(section(inputs, ATTACK_PRESETS_SECTION));
  const defenseDocument = parseDefensePresetDocument(section(inputs, DEFENSE_PRESETS_SECTION));
  return evaluateKhanReadiness({
    draft: saved,
    attack: attackSetupRef(saved.attackPresetId, attackDocument, AUTO_KHAN_SECTION, 'attack'),
    defense: defenseSetupRef(saved.defensePresetId, defenseDocument, AUTO_KHAN_SECTION, 'defense'),
    state: inputs.state, attackDocument, defenseDocument, troops: inputs.troops, tools: inputs.tools,
    metadataReady: inputs.metadataReady, observation: inputs.observation,
    commanders: eligibility(inputs, 'autoKhan'), now: inputs.now,
  });
}

function stormReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoStormClientState(section(inputs, AUTO_STORM_SECTION));
  const document = parseAttackPresetDocument(section(inputs, ATTACK_PRESETS_SECTION));
  const blueprints = parseAutoStormBlueprintDocument(section(inputs, AUTO_STORM_BLUEPRINTS_SECTION));
  const stormCastle = Object.values(inputs.state?.castles ?? {}).filter((castle) => castle.kingdomId === 4)
    .sort((left, right) => left.id - right.id)[0] ?? null;
  return evaluateStormReadiness({
    draft: saved,
    forts: attackSetupRef(saved.forts.presetId, document, AUTO_STORM_SECTION, 'forts'),
    islands: attackSetupRef(saved.islands.presetId, document, AUTO_STORM_SECTION, 'islands'),
    state: inputs.state, stormCastle, document, troops: inputs.troops, tools: inputs.tools,
    metadataReady: inputs.metadataReady, observation: inputs.observation,
    buildActive: saved.target != null || Boolean(blueprints.activeId),
    commanders: eligibility(inputs, 'autoStorm'),
    unlockOffer: inputs.stormUnlockOffer,
  });
}

function towersReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoTowerClientState(section(inputs, 'automation.autoTowers'));
  return evaluateTowerReadiness({
    state: inputs.state, castles: saved.castles, troops: inputs.troops, tools: inputs.tools,
    metadataReady: inputs.metadataReady, observation: inputs.observation,
    commanders: eligibility(inputs, 'autoTowers'),
  }).report;
}

function fortressReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoFortressClientState(section(inputs, AUTO_FORTRESS_SECTION));
  return evaluateFortressReadiness({
    state: inputs.state, kingdoms: saved.kingdoms, direwolfId: AUTO_FORTRESS_DIREWOLF_ID,
    direwolfPurchaseLimit: saved.direwolfPurchaseLimit, troops: inputs.troops, tools: inputs.tools,
    metadataReady: inputs.metadataReady, observation: inputs.observation,
    commanders: eligibility(inputs, 'autoFortress'),
  }).report;
}

function reserveReadiness(featureId: 'autoStation' | 'autoBird', inputs: FeatureReadinessInputs): ReadinessReport {
  const reserves = featureId === 'autoStation'
    ? parseAutoStationClientState(section(inputs, 'automation.autoStation')).settings
    : parseAutoBirdClientState(section(inputs, 'automation.autoBird')).ignoreSettings.settings;
  return evaluateReserveReadiness({
    featureId, state: inputs.state, reserves, troops: inputs.troops, tools: inputs.tools,
    metadataReady: inputs.metadataReady, observation: inputs.observation,
  }).report;
}

function foodReadiness(inputs: FeatureReadinessInputs): ReadinessReport {
  const saved = parseAutoFoodBalanceSettings(section(inputs, 'automation.autoFoodBalance'));
  return evaluateFoodBalanceReadiness({
    state: inputs.state, resources: inputs.resources ?? {}, metadataReady: inputs.metadataReady,
    minimumSourceReserve: saved.minimumSourceReserve, minimumCoinReserve: saved.minimumCoinReserve,
    autoKingdomTransport: saved.autoKingdomTransport, observation: inputs.observation,
  }).report;
}

// ——— Modules without an evaluator: only what the saved section proves ———

const CONFIGURATION_SECTIONS: Readonly<Record<string, string>> = {
  autoRecruit: 'automation.recruitTroops',
  autoTool: 'automation.autoTool',
  autoHospital: 'automation.autoHospital',
  autoTCI: 'automation.constructionItems',
  autoSceatRes: 'automation.autoSceatResources',
  autoBooster: AUTO_BOOSTER_SECTION,
  autoBuyer: AUTO_BUYER_SECTION,
  autoAdvisor: AUTO_ADVISOR_SECTION,
  autoEquipmentCleanup: 'automation.autoEquipmentCleanup',
};

const line = (id: string, messageKey: MessageKey, params?: MessageParameters): ReadinessPlanLine => (
  params ? { id, messageKey, params } : { id, messageKey }
);

/** Spending the saved settings permit (what the section proves; the game still checks price and reserve at each purchase). */
function spendingPlan(featureId: string, saved: unknown): ReadinessPlanLine[] {
  if (featureId === 'autoBooster') {
    const settings = parseAutoBoosterClientState(saved);
    return [line('spending', message('featureReadiness.spendingBooster'), { reserve: settings.minimumRubyReserve })];
  }
  if (featureId === 'autoBuyer') {
    const settings = parseAutoBuyerClientState(saved);
    return [line('spending', message('featureReadiness.spendingBuyer'), { rubies: settings.allowRubyPackages ? 'allowed' : 'never', reserve: settings.minimumRubyReserve })];
  }
  if (featureId === 'autoSceatRes') {
    const settings = normalizeAutoSceatResSettings(saved);
    return [line('spending', message('featureReadiness.spendingSceat'), { rubies: settings.allowRubyRecipes ? 'allowed' : 'never', reserve: settings.minimumRubyReserve })];
  }
  return [];
}

/** Recruit and Tool: a plan exists when the shared list or an enabled castle plan has items. */
function productionPlanCheck(featureId: 'autoRecruit' | 'autoTool', saved: unknown): ReadinessCheck {
  const settings = featureId === 'autoRecruit' ? normalizeRecruitTroopsSettings(saved) : normalizeAutoToolSettings(saved);
  const castlePlans = Object.values(settings.castles).filter((castle) => castle.enabled && castle.items.length > 0).length;
  const planned = settings.mode === 'global' ? settings.globalItems.length > 0 : castlePlans > 0;
  return planned
    ? { id: 'plan', state: 'valid', messageKey: message('featureReadiness.planSaved'), params: { mode: settings.mode, count: settings.mode === 'global' ? settings.globalItems.length : castlePlans } }
    : { id: 'plan', state: 'blocked', messageKey: message('featureReadiness.planMissing'), fix: 'settings' };
}

export function configurationReadiness(featureId: string, inputs: FeatureReadinessInputs): ReadinessReport {
  const name = CONFIGURATION_SECTIONS[featureId];
  const saved = name ? section(inputs, name) : undefined;
  const checks: ReadinessCheck[] = [];
  const plan: ReadinessPlanLine[] = [];
  if (!name) {
    checks.push({ id: 'saved-settings', state: 'pending', messageKey: message('featureReadiness.noSavedSettings'), fix: 'settings' });
    return finish(featureId, checks);
  }
  checks.push(isRecord(saved) && Object.keys(saved).length > 0
    ? { id: 'saved-settings', state: 'valid', messageKey: message('featureReadiness.savedSettings') }
    : { id: 'saved-settings', state: 'pending', messageKey: message('featureReadiness.noSavedSettings'), fix: 'settings' });
  if (featureId === 'autoRecruit' || featureId === 'autoTool') checks.push(productionPlanCheck(featureId, saved));
  if (featureId === 'autoAdvisor') {
    // Advisor chains start from a Great Empire castle and an attack preset.
    const advisor = parseAutoAdvisorClientState(saved);
    const document = parseAttackPresetDocument(section(inputs, ATTACK_PRESETS_SECTION));
    const castle = evaluateCastleReference({ castleId: advisor.sourceCastleId, state: inputs.state, purpose: 'source-great-empire' });
    checks.push(castle);
    checks.push(...attackSlotReadiness({
      slot: 'attack',
      ref: attackSetupRef(advisor.presetId, document, AUTO_ADVISOR_SECTION, 'attack'),
      castle: inputs.state?.castles?.[String(advisor.sourceCastleId)] ?? null,
      document, troops: inputs.troops, tools: inputs.tools, metadataReady: inputs.metadataReady, observation: inputs.observation,
    }));
    checks.push({ id: 'commander-assignment', state: 'pending', messageKey: message('featureReadiness.commandersAtLaunch') });
  }
  checks.push(...scheduleCheck(inputs));
  plan.push(...spendingPlan(featureId, saved));
  if (castlesUnobserved(inputs.state) && featureId !== 'autoEquipmentCleanup') {
    checks.push({ id: 'castles', state: 'unavailable', messageKey: message('ui.settings.requirements.setupReadiness.castle.data.has.not.been.observed.yet.76ce81b7'), fix: 'connection' });
  }
  return finish(featureId, checks, plan);
}

export function evaluateFeatureReadiness(featureId: string, inputs: FeatureReadinessInputs): ReadinessReport {
  switch (featureId) {
    case 'autoNomad': return nomadReadiness(inputs);
    case 'autoInvasion': return invasionReadiness(inputs);
    case 'autoBeriWorld': return beriReadiness(inputs);
    case 'autoKhan': return khanReadiness(inputs);
    case 'autoStorm': return stormReadiness(inputs);
    case 'autoTowers': return towersReadiness(inputs);
    case 'autoFortress': return fortressReadiness(inputs);
    case 'autoStation':
    case 'autoBird': return reserveReadiness(featureId, inputs);
    case 'autoFoodBalance': return foodReadiness(inputs);
    default: return configurationReadiness(featureId, inputs);
  }
}

/**
 * Start-time rule (CIT-20): only blocked checks in the saved settings ask for confirmation. Pending and
 * unavailable checks are decided by the game or wait for data, so they never block a Start; and even a
 * confirmed Start only writes the switch, because the game's own guards stay authoritative.
 */
export function startRequiresConfirmation(report: ReadinessReport): boolean {
  return report.overall === 'blocked';
}

/** The blocked checks of a report: what the Start confirmation lists. */
export function blockedChecks(report: ReadinessReport): ReadinessCheck[] {
  return report.checks.filter((check) => check.state === 'blocked');
}

/** Checks the game decides or that wait for data; listed at Start but never block it. */
export function undecidedChecks(report: ReadinessReport): ReadinessCheck[] {
  return report.checks.filter((check) => check.state === 'pending' || check.state === 'unavailable');
}
