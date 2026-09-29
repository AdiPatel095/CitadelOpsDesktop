import type { MessageKey } from '../../i18n/messages';
import type { ReadinessCheck } from '../readiness/Readiness';
import type { SettingsFeatureId } from './placement';

/**
 * Where each readiness check's in-context fix lives (CIT-17): the settings
 * section (from `placement.ts`) and the control's DOM id. A Fix on a check
 * whose control sits in a collapsed Advanced section expands it first, and the
 * collapsed control's current value is shown beside the error.
 *
 * Keys are check ids, or `id:slot` for slot-specific checks; `null` marks a
 * check with no settings control (connection, runtime-decided or commander
 * checks, which the commander panel repairs). `tests/fix-targets.test.mjs`
 * requires an entry for every check id each feature's evaluator can emit.
 */
export interface SettingsFixTarget {
  section: string;
  control: string;
  /** Label of the control, shown with its current value while its section is collapsed. */
  labelKey?: MessageKey;
}

type FixTable = Readonly<Record<string, SettingsFixTarget | null>>;

const target = (section: string, control: string, labelKey?: MessageKey): SettingsFixTarget => (
  labelKey ? { section, control, labelKey } : { section, control }
);

const COMMANDERS = { commanders: null, 'commander-assignment': null } as const;

/** Checks shared by the Nomad, Invasion and Beri World event-attack evaluator. */
function eventAttackTable(prefix: string, slots: readonly string[], extra: FixTable = {}): FixTable {
  const perSlot: Record<string, SettingsFixTarget> = {};
  for (const slot of slots) {
    perSlot[`composition:${slot}`] = target('setup', `${prefix}-${slot}`);
    perSlot[`inventory:${slot}`] = target('setup', `${prefix}-${slot}`);
    perSlot[`difficulty:${slot}`] = target('event', `${prefix}-difficulty`);
  }
  return {
    'source-castle': target('setup', `${prefix}-source`),
    composition: target('setup', `${prefix}-${slots[0]}`),
    inventory: target('setup', `${prefix}-${slots[0]}`),
    difficulty: target('event', `${prefix}-difficulty`),
    'score-target': target('event', `${prefix}-score`),
    'daily-limit': target('limits', `${prefix}-daily-limit`),
    'horse-travel-boost': null,
    'tool-compatibility': null,
    ...COMMANDERS,
    ...perSlot,
    ...extra,
  };
}

export const SETTINGS_FIX_TARGETS: Readonly<Record<SettingsFeatureId, FixTable>> = {
  autoNomad: eventAttackTable('auto-nomad', ['nomad', 'samurai'], {
    'fortify-currency': null,
    'gallantry-booster': null,
  }),
  autoInvasion: eventAttackTable('auto-invasion', ['attack'], {
    'fortify-currency': target('fortify', 'auto-invasion-fortify'),
    'gallantry-booster': null,
  }),
  autoBeriWorld: {
    ...eventAttackTable('auto-beri', ['attack'], {
      'fortify-currency': null,
      'gallantry-booster': target('attack', 'auto-beri-gallantry'),
    }),
    'source-castle': target('attack', 'auto-beri-source'),
    composition: target('attack', 'auto-beri-attack'),
    inventory: target('attack', 'auto-beri-attack'),
    'composition:attack': target('attack', 'auto-beri-attack'),
    'inventory:attack': target('attack', 'auto-beri-attack'),
    difficulty: null,
    'difficulty:attack': null,
    'score-target': null,
  },
  autoKhan: {
    'source-castle': target('setup', 'auto-khan-source'),
    'main-castle': null,
    'composition:attack': target('setup', 'auto-khan-attack'),
    'inventory:attack': target('setup', 'auto-khan-attack'),
    composition: target('setup', 'auto-khan-attack'),
    inventory: target('setup', 'auto-khan-attack'),
    'defense-composition': target('setup', 'auto-khan-defense'),
    'defense-composition:defense': target('setup', 'auto-khan-defense'),
    'defense-tool-stock': null,
    'defense-tool-stock:defense': null,
    'skip-cooldowns': target('skips', 'auto-khan-skips'),
    protection: null,
    'rage-booster': target('stop-limits', 'auto-khan-rage', 'ui.settings.components.autoKhanSettingsModal.require.rage.points.booster.ad17ec97'),
    'tool-compatibility': null,
    'daily-limit': target('limits', 'auto-khan-daily-limit'),
    'horse-travel-boost': null,
    ...COMMANDERS,
  },
  autoTowers: {
    'enabled-castles': target('castles', 'auto-towers-castles'),
    inventory: target('castles', 'auto-towers-castles'),
    'maiden-relic': null,
    ...COMMANDERS,
  },
  autoFortress: {
    'kingdom-castle': target('kingdoms', 'auto-fortress-kingdoms'),
    'enabled-kingdoms': target('kingdoms', 'auto-fortress-kingdoms'),
    inventory: target('supply', 'auto-fortress-supply'),
    'commander-speed': null,
    ...COMMANDERS,
  },
  autoStorm: {
    unlock: target('castle', 'auto-storm-access'),
    'storm-castle': null,
    'forts-targets': target('targets', 'auto-storm-branches'),
    'forts-targets:forts': target('targets', 'auto-storm-branches'),
    'composition:forts': target('targets', 'auto-storm-forts'),
    'inventory:forts': target('targets', 'auto-storm-forts'),
    'islands-targets': target('targets', 'auto-storm-branches'),
    'islands-targets:islands': target('targets', 'auto-storm-branches'),
    'composition:islands': target('targets', 'auto-storm-islands'),
    'inventory:islands': target('targets', 'auto-storm-islands'),
    'islands-defense-units': target('targets', 'auto-storm-islands'),
    'islands-defense-units:islands': target('targets', 'auto-storm-islands'),
    composition: target('targets', 'auto-storm-branches'),
    inventory: target('targets', 'auto-storm-branches'),
    donors: target('donors', 'auto-storm-donors'),
    shop: target('shop', 'auto-storm-shop'),
    'tool-compatibility': null,
    'daily-limit': target('limits', 'auto-storm-daily-limit'),
    'horse-travel-boost': null,
    ...COMMANDERS,
  },
  autoFoodBalance: {
    'food-observations': null,
    donors: target('reserves', 'auto-food-donor-reserve'),
    'coin-reserve': target('reserves', 'auto-food-coin-reserve'),
    'kingdom-transport': target('reserves', 'auto-food-kingdom-transport'),
  },
  autoStation: {
    castles: target('reserves', 'auto-station-castles'),
    'saved-castles': target('reserves', 'auto-station-castles'),
    reserves: target('reserves', 'auto-station-castles'),
  },
  autoBird: {
    castles: target('castles', 'auto-bird-castles'),
    'saved-castles': target('castles', 'auto-bird-castles'),
    reserves: target('castles', 'auto-bird-castles'),
  },
  autoRecruit: {},
  autoTool: {},
  autoHospital: {},
  autoTCI: {},
  autoSceatRes: {},
  autoBooster: {},
  autoBuyer: {},
  autoAdvisor: {},
  autoEquipmentCleanup: {},
};

export function fixTargetFor(featureId: SettingsFeatureId, check: Pick<ReadinessCheck, 'id' | 'slot'>): SettingsFixTarget | null {
  const table = SETTINGS_FIX_TARGETS[featureId];
  if (check.slot) {
    const slotted = table[`${check.id}:${check.slot}`];
    if (slotted !== undefined) return slotted;
  }
  return table[check.id] ?? null;
}
