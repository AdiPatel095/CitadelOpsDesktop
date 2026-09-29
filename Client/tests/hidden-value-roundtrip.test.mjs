import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

/**
 * CIT-17 hidden-value preservation: every value that lives in a collapsed
 * Advanced section survives an edit of an Essentials control, the modal's save
 * composition and a reopen (parse). Collapsed sections stay mounted and hold
 * no state of their own, so the draft the modal saves is the full draft.
 */
const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const load = (path) => vite.ssrLoadModule(`/src/settings/${path}.ts`);
const nomad = await load('AutoNomadClientState');
const invasion = await load('AutoInvasionClientState');
const khan = await load('AutoKhanClientState');
const beri = await load('AutoBeriWorldClientState');
const tower = await load('AutoTowerClientState');
const fortress = await load('AutoFortressClientState');
const storm = await load('AutoStormClientState');
const food = await load('AutoFoodBalanceClientState');
const station = await load('AutoStationClientState');
const bird = await load('AutoBirdClientState');
const recruit = await load('RecruitTroopsClientState');
const tool = await load('AutoToolClientState');
const hospital = await load('AutoHospitalClientState');
const tci = await load('AutoTCIClientState');
const tciPresets = await load('AutoTCIPresets');
const sceat = await load('AutoSceatResClientState');
const booster = await load('AutoBoosterClientState');
const buyer = await load('AutoBuyerClientState');
const advisor = await load('AutoAdvisorClientState');

after(async () => {
  await vite.close();
});

function merge(base, patch) {
  if (patch == null || typeof patch !== 'object' || Array.isArray(patch)) return patch;
  const result = { ...base };
  for (const [key, value] of Object.entries(patch)) result[key] = merge(base?.[key], value);
  return result;
}

function pick(value, patch) {
  if (patch == null || typeof patch !== 'object' || Array.isArray(patch)) return value;
  return Object.fromEntries(Object.keys(patch).map((key) => [key, pick(value?.[key], patch[key])]));
}

/**
 * @param parse     the modal's load path (section value -> draft)
 * @param defaults  feature defaults
 * @param hidden    custom values of Advanced controls (partial, nested)
 * @param edit      one Essentials edit on the loaded draft
 * @param save      the modal's save composition (draft -> section value)
 */
function assertHiddenValuesSurvive(name, { parse, defaults, hidden, edit, save = (draft) => draft }) {
  const stored = merge(defaults, hidden);
  const loaded = parse(structuredClone(stored));
  assert.deepEqual(pick(loaded, hidden), hidden, `${name}: custom Advanced values load unchanged`);
  const edited = edit(structuredClone(loaded));
  assert.notDeepEqual(edited, loaded, `${name}: the Essentials edit changes the draft`);
  const reopened = parse(structuredClone(save(edited)));
  assert.deepEqual(pick(reopened, hidden), hidden, `${name}: custom Advanced values survive save and reopen`);
  assert.deepEqual(reopened, parse(structuredClone(save(edited))), `${name}: deterministic`);
  assert.deepEqual(reopened, edited, `${name}: exactly the one Essentials change is written`);
}

test('event combat: Nomad, Invasion, Khan and Beri World', () => {
  assertHiddenValuesSurvive('autoNomad', {
    parse: nomad.parseAutoNomadClientState,
    defaults: nomad.defaultAutoNomadClientState(),
    hidden: { skipCooldowns: true, timeSkipReserve: { MS1: 4, MS7: 1 }, horseTravelBoostId: 1008, rbcTest: { enabled: true, targetX: 412, targetY: 77 } },
    edit: (draft) => ({ ...draft, scoreTarget: 250000 }),
    save: (draft) => ({ ...draft, nomadPresetId: draft.nomadPresetId, samuraiPresetId: draft.samuraiPresetId }),
  });
  assertHiddenValuesSurvive('autoInvasion', {
    parse: invasion.parseAutoInvasionClientState,
    defaults: invasion.defaultAutoInvasionClientState(),
    hidden: { horseTravelBoostId: 1007 },
    edit: (draft) => ({ ...draft, scoreTarget: 120000 }),
    save: (draft) => ({ ...draft, presetId: draft.presetId }),
  });
  assertHiddenValuesSurvive('autoKhan', {
    parse: khan.parseAutoKhanClientState,
    defaults: khan.defaultAutoKhanClientState(),
    hidden: { maxRageChain: 3, requireActiveRageBooster: true, nomadPointThreshold: 90000, horseTravelBoostId: 1009 },
    edit: (draft) => ({ ...draft, dailyAttackLimit: 400 }),
    save: (draft) => ({ ...draft, attackPresetId: draft.attackPresetId, defensePresetId: draft.defensePresetId }),
  });
  assertHiddenValuesSurvive('autoBeriWorld', {
    parse: beri.parseAutoBeriWorldSettings,
    defaults: beri.DEFAULT_AUTO_BERI_WORLD_SETTINGS,
    hidden: {
      build: { allowPremium: true, allowDemolition: true, allowTimeSkips: true, timeSkipReserve: { MS2: 5 } },
      attackCheckIntervalSec: 120,
      toolMinimums: { 614: 20 },
      useTroopTransportTimeSkips: true,
      horseTravelBoostId: 1008,
      minTroopsToTransfer: 250,
    },
    edit: (draft) => ({ ...draft, dailyAttackLimit: 75 }),
    save: (draft) => beri.parseAutoBeriWorldSettings({ ...draft }),
  });
});

test('world combat: Towers, Fortress and Storm', () => {
  const towerSave = (draft) => ({ ...tower.parseAutoTowerClientState(draft), version: 4, ...draft });
  assertHiddenValuesSurvive('autoTowers', {
    parse: tower.parseAutoTowerClientState,
    defaults: tower.defaultAutoTowerClientState(),
    hidden: {
      mapRefreshIntervalSec: 3600,
      horseTravelBoostId: 1007,
      useAdvisor: true,
      autoActivateAdvisor: true,
      maximumDailyTimeSkips: 12,
      castles: { 5: { radius: 25, maidenOnly: true } },
    },
    edit: (draft) => ({ ...draft, castles: { ...draft.castles, 5: { ...draft.castles[5], enabled: true } }, dailyAttackLimit: 30 }),
    save: towerSave,
  });
  assertHiddenValuesSurvive('autoFortress', {
    parse: fortress.parseAutoFortressClientState,
    defaults: fortress.defaultAutoFortressClientState(),
    hidden: { useTimeSkips: true, horseTravelBoostId: 1009 },
    edit: (draft) => ({ ...draft, direwolfPurchaseLimit: 500 }),
  });
  assertHiddenValuesSurvive('autoStorm', {
    parse: storm.parseAutoStormClientState,
    defaults: storm.defaultAutoStormClientState(),
    hidden: {
      checkIntervalSec: 90,
      horseTravelBoostId: 1007,
      harbor: { enabled: true, targetLevel: 2 },
      build: { allowPremium: true, allowDemolition: true, allowTimeSkips: true, allowResourceTransport: true, timeSkipReserve: { MS3: 2 } },
      troopImport: { minimumTroops: 900 },
    },
    edit: (draft) => ({ ...draft, dailyAttackLimit: 60 }),
    save: (draft) => storm.parseAutoStormClientState({ ...draft }),
  });
});

test('supply and support: Food Balance, Station and Bird', () => {
  assertHiddenValuesSurvive('autoFoodBalance', {
    parse: food.parseAutoFoodBalanceSettings,
    defaults: food.DEFAULT_AUTO_FOOD_BALANCE_SETTINGS,
    hidden: {
      checkIntervalSec: 120, minimumShipmentSize: 5000, minimumStormShipmentSize: 20000,
      useKingdomTimeSkips: true, allowedTimeSkips: ['MS1', 'MS2'], timeSkipReserve: { MS1: 3 }, horseTravelBoostId: 1007,
      safetyHours: 12, sourceSafetyHours: 36,
    },
    edit: (draft) => ({ ...draft, autoKingdomTransport: true, minimumCoinReserve: 40000 }),
    save: food.parseAutoFoodBalanceSettings,
  });
  assertHiddenValuesSurvive('autoStation', {
    parse: station.parseAutoStationClientState,
    defaults: station.DEFAULT_AUTO_STATION_STATE,
    hidden: { minRPTDays: 7, openGateFallback: true },
    edit: (draft) => ({ ...draft, leadTimeSec: 300 }),
    save: station.parseAutoStationClientState,
  });
  const birdDefaults = bird.buildAutoBirdClientState(bird.defaultAutoBirdSettings(), { version: 1, lastSelectedPresetId: null, presets: [] });
  assertHiddenValuesSurvive('autoBird', {
    parse: bird.parseAutoBirdClientState,
    defaults: birdDefaults,
    hidden: { ignoreSettings: { minDelay: 2, maxDelay: 9, minSend: 150 } },
    edit: (draft) => ({ ...draft, ignoreSettings: { ...draft.ignoreSettings, minRPTDays: 6 } }),
    save: (draft) => bird.buildAutoBirdClientState(draft.ignoreSettings, draft.presets, draft.activePresetId),
  });
});

test('production and recovery: Recruit, Tool and Hospital', () => {
  assertHiddenValuesSurvive('autoRecruit', {
    parse: recruit.normalizeRecruitTroopsSettings,
    defaults: recruit.defaultRecruitTroopsSettings(),
    hidden: { checkIntervalSec: 900 },
    edit: (draft) => ({ ...draft, mode: draft.mode === 'global' ? 'perCastle' : 'global' }),
  });
  assertHiddenValuesSurvive('autoTool', {
    parse: tool.normalizeAutoToolSettings,
    defaults: tool.defaultAutoToolSettings(),
    hidden: { checkIntervalSec: 1200 },
    edit: (draft) => ({ ...draft, mode: draft.mode === 'global' ? 'perCastle' : 'global' }),
  });
  const hospitalLoaded = hospital.normalizeAutoHospitalSettings({ version: 1, checkIntervalSec: 1800 });
  assert.equal(hospital.normalizeAutoHospitalSettings(hospitalLoaded).checkIntervalSec, 1800, 'autoHospital: the collapsed interval survives save and reopen');
});

test('upkeep and economy: TCI, Sceat, Booster, Buyer and Advisor', () => {
  const preset = { id: 'p1', name: 'Main', settings: { 1: [{ id: 10, amount: 5 }] } };
  const presets = tciPresets.emptyPresetsFile();
  assertHiddenValuesSurvive('autoTCI', {
    parse: tci.parseAutoTCIClientState,
    defaults: tci.buildAutoTCIClientState({}, presets),
    hidden: { presets: { presets: tci.parseAutoTCIClientState({ version: 1, targets: {}, presets: { version: 1, lastSelectedPresetId: 'p1', presets: [preset] } }).presets.presets, lastSelectedPresetId: 'p1' } },
    edit: (draft) => ({ ...draft, targets: { ...draft.targets, 2: [{ id: 11, amount: 7 }] } }),
    save: (draft) => tci.buildAutoTCIClientState(draft.targets, draft.presets),
  });
  assertHiddenValuesSurvive('autoSceatRes', {
    parse: sceat.normalizeAutoSceatResSettings,
    defaults: sceat.defaultAutoSceatResSettings(),
    hidden: { checkIntervalSec: 600, minimumShipmentSize: 25000, overflowThresholdPercent: 80, useKingdomTimeSkips: true, allowedTimeSkips: ['MS1'], timeSkipReserve: { MS1: 2 } },
    edit: (draft) => ({ ...draft, autoKingdomTransport: true, minimumCoinReserve: 100000 }),
    save: sceat.normalizeAutoSceatResSettings,
  });
  assertHiddenValuesSurvive('autoBooster', {
    parse: booster.parseAutoBoosterClientState,
    defaults: booster.defaultAutoBoosterClientState(),
    hidden: { checkIntervalSec: 120 },
    edit: (draft) => ({ ...draft, minimumRubyReserve: 5000 }),
    save: booster.parseAutoBoosterClientState,
  });
  assertHiddenValuesSurvive('autoBuyer', {
    parse: buyer.parseAutoBuyerClientState,
    defaults: buyer.defaultAutoBuyerClientState(),
    hidden: { checkIntervalSec: 2700 },
    edit: (draft) => ({ ...draft, minimumRubyReserve: 1234 }),
    save: buyer.parseAutoBuyerClientState,
  });
  assertHiddenValuesSurvive('autoAdvisor', {
    parse: advisor.parseAutoAdvisorClientState,
    defaults: advisor.defaultAutoAdvisorClientState(),
    hidden: { maxAttackCount: 20, minimumRemainingSec: 3600, horseTravelBoostId: 1008 },
    edit: (draft) => ({ ...draft, coinCostPerAttack: 900 }),
    save: (draft) => ({ ...draft, presetId: draft.presetId }),
  });
});
