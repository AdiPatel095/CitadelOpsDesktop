import type { CastleStateV2, GameStateV2 } from '../../api/Contracts';
import type { AttackSetupRef } from '../../attackPresets/AppCreatedPresets';
import type { AttackPresetDocument } from '../../attackPresets/AttackPresetTypes';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import type { AutoStormClientStateV1 } from '../AutoStormClientState';
import type { CommanderEligibilityReport } from '../requirements/commanderEligibility';
import type { ObservationContext } from '../requirements/observationFreshness';
import { evaluateUnitStock } from '../requirements/unitRequirements';
import { attackSlotReadiness, dailyLimitCheck, travelBoostCheck } from './eventAttackReadiness';
import { aggregateReadiness, type ReadinessCheck, type ReadinessPlanLine, type ReadinessReport } from './Readiness';

export type StormReadinessDraft = Pick<AutoStormClientStateV1,
  'unlock' | 'decorationPresetId' | 'build' | 'harbor' | 'forts' | 'islands' | 'troopImport' | 'aquamarine' | 'dailyAttackLimit' | 'horseTravelBoostId'>;

export interface StormReadinessInput {
  draft: StormReadinessDraft;
  forts: AttackSetupRef;
  islands: AttackSetupRef;
  state: GameStateV2 | null;
  /** The Storm castle attacks launch from (kingdom 4). */
  stormCastle: CastleStateV2 | null;
  document: AttackPresetDocument;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  observation: ObservationContext;
  /** A Storm blueprint or captured target is active, so the build lane runs. */
  buildActive: boolean;
  /** Label of the selected decoration preset, when one is chosen. */
  decorationLabel?: string;
  commanders?: CommanderEligibilityReport;
}

const message = (key: MessageKey): MessageKey => key;

/**
 * Stock that supplies Storm attacks: the Storm castle, plus the donor castles'
 * troops when troop import is enabled (donors import troops only; attack tools
 * must already be in the Storm castle). Freshness follows the Storm castle.
 */
export function stormStockCastle(
  stormCastle: CastleStateV2 | null,
  donors: readonly CastleStateV2[],
  importEnabled: boolean,
  tools: Record<number, MetadataItem>,
): CastleStateV2 | null {
  if (!stormCastle) return null;
  if (!importEnabled || donors.length === 0) return stormCastle;
  const stationed: Record<string, number> = { ...(stormCastle.units?.stationed ?? {}) };
  for (const donor of donors) {
    for (const [id, amount] of Object.entries(donor.units?.stationed ?? {})) {
      if (tools[Number(id)]) continue;
      stationed[id] = (Number(stationed[id]) || 0) + Math.max(0, Number(amount) || 0);
    }
  }
  return { ...stormCastle, units: { ...stormCastle.units, stationed } };
}

/**
 * Non-action review of Auto Storm (CIT-16). Only enabled branches impose
 * prerequisites; the optional decoration preset and the build, harbor and shop
 * lanes appear as plan lines. Mirrors `Server/Automation/AutoStormPolicy.go`
 * preset resolution per branch; the runtime stays authoritative.
 */
export function evaluateStormReadiness(input: StormReadinessInput): ReadinessReport {
  const { draft, state } = input;
  const checks: ReadinessCheck[] = [];
  const plan: ReadinessPlanLine[] = [];
  const unlock = state?.kingdomTransport?.unlocks?.['4'];

  if (!state) {
    checks.push({ id: 'unlock', state: 'unavailable', messageKey: message('ui.settings.requirements.castleRequirements.castle.data.has.not.been.observed.yet.76ce81b7'), fix: 'connection' });
  } else if (unlock?.unlocked || unlock?.created || input.stormCastle) {
    checks.push(input.stormCastle
      ? { id: 'storm-castle', state: 'valid', messageKey: message('stormReadiness.stormCastle'), params: { castle: input.stormCastle.name?.trim() || `#${input.stormCastle.id}` } }
      : { id: 'storm-castle', state: 'unavailable', messageKey: message('ui.settings.readiness.stormReadiness.the.storm.castle.is.unlocked.but.has.f6946840'), fix: 'connection' });
  } else if (draft.unlock.enabled) {
    checks.push({ id: 'unlock', state: 'pending', messageKey: message('ui.settings.readiness.stormReadiness.the.storm.castle.is.not.unlocked.yet.58e0fdef') });
  } else {
    checks.push({ id: 'unlock', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.the.storm.castle.is.not.unlocked.enable.686f9a36'), fix: 'settings' });
  }

  if (!draft.forts.enabled && !draft.islands.enabled) {
    checks.push({ id: 'branches', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.enable.storm.forts.resource.islands.or.both.14f81932'), fix: 'settings' });
  }

  const donorIds = draft.troopImport.enabled ? draft.troopImport.donorCastleIds : [];
  const donors = donorIds
    .map((castleId) => state?.castles?.[String(castleId)])
    .filter((castle): castle is CastleStateV2 => castle != null && castle.kingdomId !== 4);
  const stockCastle = stormStockCastle(input.stormCastle, donors, draft.troopImport.enabled, input.tools);
  const slot = (name: 'forts' | 'islands', ref: AttackSetupRef) => attackSlotReadiness({
    slot: name,
    ref,
    castle: stockCastle,
    document: input.document,
    troops: input.troops,
    tools: input.tools,
    metadataReady: input.metadataReady,
    observation: input.observation,
  });

  if (draft.forts.enabled) {
    checks.push(draft.forts.levels.length > 0
      ? { id: 'forts-targets', slot: 'forts', state: 'valid', messageKey: message('stormReadiness.fortTargets'), params: { levels: [...draft.forts.levels].sort((a, b) => a - b).join(', ') } }
      : { id: 'forts-targets', slot: 'forts', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.choose.at.least.one.storm.fort.level.3ed33bbb'), fix: 'settings' });
    checks.push(...slot('forts', input.forts));
    if (draft.forts.minimumWins > 0) plan.push({ id: 'forts-minimum-wins', messageKey: message('stormReadiness.fortMinimumWins'), params: { wins: draft.forts.minimumWins } });
  }

  if (draft.islands.enabled) {
    checks.push(draft.islands.resources.length > 0 && draft.islands.sizes.length > 0
      ? { id: 'islands-targets', slot: 'islands', state: 'valid', messageKey: message('stormReadiness.islandTargets'), params: { resources: draft.islands.resources.length, sizes: draft.islands.sizes.length } }
      : { id: 'islands-targets', slot: 'islands', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.choose.at.least.one.island.resource.and.9239e814'), fix: 'settings' });
    checks.push(...slot('islands', input.islands));
    const units = draft.islands.defenseUnits;
    if (units.some((unit) => unit.unitId <= 0 || unit.amount <= 0)) {
      checks.push({ id: 'islands-defense-units', slot: 'islands', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.every.island.defense.unit.needs.a.troop.402e7550'), fix: 'settings' });
    } else if (units.length === 0) {
      plan.push({ id: 'islands-defense-units', messageKey: message('ui.settings.readiness.stormReadiness.no.troops.are.left.behind.to.defend.858aa63f') });
    } else {
      const stock = evaluateUnitStock({
        castle: stockCastle,
        observation: input.observation,
        requests: units.map((unit) => ({ itemId: unit.unitId, amount: unit.amount, kind: 'troop' as const })),
        troops: input.troops,
        tools: input.tools,
        metadataReady: input.metadataReady,
        id: 'islands-defense-units',
        slot: 'islands',
        decidedAtLaunch: 'quantity',
      });
      checks.push(stock.check);
    }
  }

  if (draft.troopImport.enabled) {
    const missing = donorIds.filter((castleId) => {
      const castle = state?.castles?.[String(castleId)];
      return castle == null || castle.kingdomId === 4;
    });
    if (donorIds.length === 0) {
      checks.push({ id: 'donors', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.choose.at.least.one.donor.castle.or.2dba5e0a'), fix: 'settings' });
    } else if (state && missing.length > 0) {
      checks.push({ id: 'donors', state: 'blocked', messageKey: message('stormReadiness.donorsMissing'), params: { count: missing.length }, fix: 'settings' });
    } else {
      checks.push({ id: 'donors', state: 'valid', messageKey: message('stormReadiness.donors'), params: { count: donorIds.length } });
    }
    if (draft.troopImport.minimumTroops > 0) plan.push({ id: 'donors-minimum', messageKey: message('stormReadiness.donorMinimum'), params: { troops: draft.troopImport.minimumTroops } });
  }

  const purchases = draft.aquamarine.purchases;
  if (purchases.length > 0) {
    const ids = purchases.map((purchase) => purchase.packageId);
    const valid = ids.every((id) => id > 0) && new Set(ids).size === ids.length
      && purchases.every((purchase) => purchase.unlimited || purchase.targetPurchases > 0);
    if (!valid) {
      checks.push({ id: 'shop', state: 'blocked', messageKey: message('ui.settings.readiness.stormReadiness.every.aquamarine.shop.purchase.needs.a.package.8c5f3fac'), fix: 'settings' });
    }
    plan.push({
      id: 'shop',
      messageKey: message('stormReadiness.shop'),
      params: { count: purchases.length, reserve: draft.aquamarine.reserve, unlimited: purchases.filter((purchase) => purchase.unlimited).length },
    });
  }

  if (draft.decorationPresetId && input.decorationLabel) {
    plan.push({ id: 'decoration', messageKey: message('stormReadiness.decoration'), params: { preset: input.decorationLabel } });
  }
  if (input.buildActive) {
    if (draft.build.allowPremium) plan.push({ id: 'build-premium', messageKey: message('ui.settings.readiness.stormReadiness.construction.may.pay.premium.ruby.costs.d38ea87c') });
    if (draft.build.allowDemolition) plan.push({ id: 'build-demolition', messageKey: message('ui.settings.readiness.stormReadiness.buildings.that.do.not.match.the.blueprint.1ddb1620') });
    if (draft.build.allowTimeSkips) plan.push({ id: 'build-time-skips', messageKey: message('ui.settings.readiness.stormReadiness.construction.may.use.time.skips.reserved.skips.a97fe7ac') });
    if (draft.build.allowResourceTransport) plan.push({ id: 'build-transport', messageKey: message('ui.settings.readiness.stormReadiness.resources.may.be.transported.to.the.storm.8860d6c2') });
  }
  if (draft.harbor.enabled) plan.push({ id: 'harbor', messageKey: message('stormReadiness.harbor'), params: { level: draft.harbor.targetLevel } });
  if (draft.unlock.enabled && !(unlock?.unlocked || unlock?.created)) {
    plan.push({ id: 'unlock-plan', messageKey: message('ui.settings.readiness.stormReadiness.the.unlock.plan.builds.the.chosen.storm.d2a66163') });
  }

  if (input.commanders) checks.push(input.commanders.activity, input.commanders.assignment);
  if (draft.forts.enabled || draft.islands.enabled) {
    checks.push({ id: 'tool-compatibility', state: 'pending', messageKey: message('ui.settings.readiness.eventAttackReadiness.tool.compatibility.with.each.target.is.checked.95eb4eaa') });
  }
  checks.push(dailyLimitCheck(draft.dailyAttackLimit, state));
  checks.push(travelBoostCheck(draft.horseTravelBoostId));

  return { featureId: 'autoStorm', checks, overall: aggregateReadiness(checks), plan };
}
