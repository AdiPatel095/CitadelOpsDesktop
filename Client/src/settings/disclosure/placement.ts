import type { MessageKey } from '../../i18n/messages';

/**
 * Essentials/Advanced placement of every automation settings section
 * (CIT-17). One table drives the modals (`SettingsSection` looks its tier up
 * here), the collapsed-setting notes beside readiness errors, and the
 * reviewed placement matrix in `Docs/Features/SettingsPlacement.md`
 * (`tests/settings-placement-matrix.test.mjs` keeps all three in sync).
 *
 * Essentials: the task, required choices and dependencies, effective scope,
 * operational limits, reserves, spending and consumable policy, Stop.
 * Advanced: timers, tooling, reserve tiers, filters and expert flags. An
 * Advanced section is collapsed by default and always shows a summary line,
 * so consequential values stay visible without expanding it.
 */
export type SettingsTier = 'essentials' | 'advanced';

export type SettingsFeatureId =
  | 'autoNomad'
  | 'autoInvasion'
  | 'autoKhan'
  | 'autoBeriWorld'
  | 'autoTowers'
  | 'autoFortress'
  | 'autoStorm'
  | 'autoFoodBalance'
  | 'autoStation'
  | 'autoBird'
  | 'autoRecruit'
  | 'autoTool'
  | 'autoHospital'
  | 'autoTCI'
  | 'autoSceatRes'
  | 'autoBooster'
  | 'autoBuyer'
  | 'autoAdvisor'
  | 'autoEquipmentCleanup';

export interface SettingsSectionPlacement {
  id: string;
  tier: SettingsTier;
  titleKey: MessageKey;
}

const message = (key: MessageKey): MessageKey => key;
const essentials = (id: string, titleKey: MessageKey): SettingsSectionPlacement => ({ id, tier: 'essentials', titleKey });
const advanced = (id: string, titleKey: MessageKey): SettingsSectionPlacement => ({ id, tier: 'advanced', titleKey });

const TRAVEL = message('ui.settings.disclosure.placement.paid.travel.1589a3d1');
const DAILY_LIMIT = message('ui.settings.disclosure.placement.daily.attack.limit.f3128ffd');
const TIMING = message('ui.settings.disclosure.placement.check.timing.a4e41aaa');

export const SETTINGS_PLACEMENT: Readonly<Record<SettingsFeatureId, readonly SettingsSectionPlacement[]>> = {
  autoNomad: [
    essentials('setup', message('ui.settings.disclosure.placement.source.castle.and.attack.setups.be31033a')),
    essentials('event', message('ui.settings.disclosure.placement.event.difficulty.and.stop.points.77021f1a')),
    essentials('limits', DAILY_LIMIT),
    advanced('cooldown-skips', message('ui.settings.disclosure.placement.cooldown.time.skips.64aca386')),
    advanced('travel', TRAVEL),
    advanced('rbc-trial', message('ui.settings.disclosure.placement.robber.baron.castle.trial.aa3e7da5')),
  ],
  autoInvasion: [
    essentials('setup', message('ui.settings.disclosure.placement.source.castle.and.attack.setup.f31eb787')),
    essentials('event', message('ui.settings.disclosure.placement.event.difficulty.and.stop.points.77021f1a')),
    essentials('fortify', message('ui.settings.disclosure.placement.fortification.spending.f59e460c')),
    essentials('limits', DAILY_LIMIT),
    advanced('travel', TRAVEL),
  ],
  autoKhan: [
    essentials('setup', message('ui.settings.disclosure.placement.castles.attack.and.main.castle.defense.4e2cbb69')),
    essentials('policy', message('ui.settings.disclosure.placement.attacks.rage.and.protection.22c935fa')),
    essentials('skips', message('ui.settings.disclosure.placement.cooldown.time.skips.64aca386')),
    essentials('limits', DAILY_LIMIT),
    advanced('stop-limits', message('ui.settings.disclosure.placement.rage.chain.booster.and.nomad.point.limits.6c72afc5')),
    advanced('travel', TRAVEL),
  ],
  autoBeriWorld: [
    essentials('attack', message('ui.settings.disclosure.placement.tower.attack.62826c7e')),
    essentials('limits', DAILY_LIMIT),
    essentials('building', message('ui.settings.disclosure.placement.camp.construction.1f65100d')),
    advanced('building-options', message('ui.settings.disclosure.placement.construction.spending.and.time.skips.b1e5acdb')),
    advanced('attack-options', message('ui.settings.disclosure.placement.attack.timing.tools.and.transfers.50d19b5e')),
    advanced('travel', TRAVEL),
  ],
  autoTowers: [
    essentials('castles', message('ui.settings.disclosure.placement.castles.and.troops.302176fb')),
    essentials('limits', DAILY_LIMIT),
    advanced('advisor', message('ui.settings.disclosure.placement.robber.baron.advisor.04006eb4')),
    advanced('scan', message('ui.settings.disclosure.placement.map.scan.and.target.filters.84f78022')),
    advanced('travel', TRAVEL),
  ],
  autoFortress: [
    essentials('kingdoms', message('ui.settings.disclosure.placement.kingdom.targets.3e092efa')),
    essentials('supply', message('ui.settings.disclosure.placement.direwolf.supply.and.reserves.c82fbfe4')),
    essentials('limits', DAILY_LIMIT),
    advanced('transfer-skips', message('ui.settings.disclosure.placement.direwolf.transfer.time.skips.eebafd6d')),
    advanced('travel', TRAVEL),
  ],
  autoStorm: [
    essentials('castle', message('ui.settings.disclosure.placement.storm.castle.2fd1da3a')),
    essentials('targets', message('ui.settings.disclosure.placement.forts.and.resource.islands.4ccb548f')),
    essentials('donors', message('ui.settings.disclosure.placement.troop.import.5f199e98')),
    essentials('shop', message('ui.settings.disclosure.placement.aquamarine.spending.9cd6c95d')),
    essentials('limits', DAILY_LIMIT),
    advanced('construction', message('ui.settings.disclosure.placement.construction.harbor.and.decoration.5f921c34')),
    advanced('priority', message('ui.settings.disclosure.placement.target.priority.64abb5b0')),
    advanced('import-tuning', message('ui.settings.disclosure.placement.troop.import.sizing.e723039f')),
    advanced('travel', TRAVEL),
    advanced('timing', TIMING),
  ],
  autoFoodBalance: [
    essentials('reserves', message('ui.settings.disclosure.placement.reserves.and.kingdom.transport.ba8b84e0')),
    advanced('timing', message('ui.settings.disclosure.placement.check.timing.and.shipment.sizes.839d9f52')),
    advanced('transport-skips', message('ui.settings.disclosure.placement.transport.time.skips.63b09664')),
    advanced('travel', TRAVEL),
  ],
  autoStation: [
    essentials('evacuation', message('ui.settings.disclosure.placement.evacuation.192f44d7')),
    essentials('reserves', message('ui.settings.disclosure.placement.troops.left.to.defend.0bf30703')),
    advanced('filters', message('ui.settings.disclosure.placement.attack.filters.and.gate.fallback.d5f7ca04')),
  ],
  autoBird: [
    essentials('targets', message('ui.settings.disclosure.placement.bird.targets.d5d6103a')),
    essentials('castles', message('ui.settings.disclosure.placement.castles.and.kept.units.8cd69143')),
    advanced('timing', message('ui.settings.disclosure.placement.send.timing.and.filters.d0613be3')),
  ],
  autoRecruit: [
    essentials('plan', message('ui.settings.disclosure.placement.recruitment.plan.4d345c07')),
    advanced('timing', TIMING),
  ],

  autoTool: [
    essentials('plan', message('ui.settings.disclosure.placement.tool.production.plan.1c38ea30')),
    advanced('timing', TIMING),
  ],

  autoHospital: [
    essentials('schedule', message('ui.settings.disclosure.placement.scan.windows.bfdbbd17')),
    advanced('timing', TIMING),
  ],
  autoTCI: [
    essentials('items', message('ui.settings.disclosure.placement.construction.items.per.castle.9f40003c')),
    advanced('presets', message('ui.settings.disclosure.placement.saved.item.presets.505b181d')),
  ],

  autoSceatRes: [
    essentials('reserves', message('ui.settings.disclosure.placement.reserves.and.spending.permissions.a41c00a3')),
    essentials('crafting', message('ui.settings.disclosure.placement.crafting.and.queue.rentals.e95625e8')),
    advanced('timing', message('ui.settings.disclosure.placement.check.timing.and.shipments.75af1bd4')),
    advanced('transport-skips', message('ui.settings.disclosure.placement.transport.time.skips.63b09664')),
    advanced('storage', message('ui.settings.disclosure.placement.additional.storage.nodes.de9521fa')),
  ],

  autoBooster: [
    essentials('purchase', message('ui.settings.disclosure.placement.daily.purchase.and.ruby.reserve.a4baec27')),
    advanced('evidence', message('ui.settings.disclosure.placement.purchase.evidence.and.safeguards.d1e3b8cb')),
  ],
  autoBuyer: [
    essentials('limits', message('ui.settings.disclosure.placement.account.wide.safety.limits.f0306444')),
    essentials('goals', message('ui.settings.disclosure.placement.shop.specialist.and.feast.goals.39924a52')),
    advanced('timing', TIMING),
  ],

  autoAdvisor: [
    essentials('access', message('ui.settings.disclosure.placement.advisor.access.and.activation.5b4dfd12')),
    essentials('setup', message('ui.settings.disclosure.placement.source.castle.attack.preset.and.difficulty.5b087d6d')),
    essentials('gates', message('ui.settings.disclosure.placement.resource.gates.and.reserves.ef34da3f')),
    advanced('run-sizing', message('ui.settings.disclosure.placement.run.sizing.878bd208')),
    advanced('travel', TRAVEL),
  ],

  autoEquipmentCleanup: [
    essentials('schedule', message('ui.settings.disclosure.placement.cleanup.schedule.b2827eb5')),
    advanced('timing', TIMING),
  ],
};

/** The `automation.enabled` key each feature's Start/Stop toggle writes. */
export const AUTOMATION_ENABLED_KEYS: Readonly<Record<SettingsFeatureId, string>> = {
  autoNomad: 'auto_nomad',
  autoInvasion: 'auto_invasion',
  autoKhan: 'auto_khan',
  autoBeriWorld: 'auto_beri_world',
  autoTowers: 'auto_towers',
  autoFortress: 'auto_fortress',
  autoStorm: 'auto_storm',
  autoFoodBalance: 'auto_food_balance',
  autoStation: 'auto_station',
  autoBird: 'auto_bird',
  autoRecruit: 'recruit_troops',
  autoTool: 'auto_tool',
  autoHospital: 'auto_hospital',
  autoTCI: 'auto_tci',
  autoSceatRes: 'auto_sceat_resources',
  autoBooster: 'auto_booster',
  autoBuyer: 'auto_buyer',
  autoAdvisor: 'auto_advisor',
  autoEquipmentCleanup: 'auto_equipment_cleanup',
};

export function sectionPlacement(featureId: SettingsFeatureId, sectionId: string): SettingsSectionPlacement {
  const placement = SETTINGS_PLACEMENT[featureId].find((section) => section.id === sectionId);
  if (!placement) throw new Error(`Unknown settings section ${featureId}/${sectionId}`);
  return placement;
}

export function advancedSectionIds(featureId: SettingsFeatureId): string[] {
  return SETTINGS_PLACEMENT[featureId].filter((section) => section.tier === 'advanced').map((section) => section.id);
}

/** DOM id of a section's wrapper; readiness fixes and "Edit" links scroll here. */
export function settingsSectionElementId(featureId: SettingsFeatureId, sectionId: string): string {
  return `settings-${featureId}-${sectionId}`;
}
