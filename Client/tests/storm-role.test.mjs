import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const role = await vite.ssrLoadModule(`${source}/settings/stormRole.ts`);
const setup = await vite.ssrLoadModule(`${source}/settings/requirements/setupReadiness.ts`);
const { parseAutoStationClientState, normalizeAutoStationStormSettings } = await vite.ssrLoadModule(`${source}/settings/AutoStationClientState.ts`);
const { parseAutoBirdClientState, normalizeAutoBirdStormSettings, activateAutoBirdPreset } = await vite.ssrLoadModule(`${source}/settings/AutoBirdClientState.ts`);
const { parseAutoTowerClientState } = await vite.ssrLoadModule(`${source}/settings/AutoTowerClientState.ts`);
const { castleCandidates } = await vite.ssrLoadModule(`${source}/settings/copy/candidates.ts`);
const { automationsActingOnCastle } = await vite.ssrLoadModule(`${source}/settings/castleAutomations.ts`);
const { previewCastleCopy, configuredSources } = await vite.ssrLoadModule(`${source}/settings/copy/castleCopy.ts`);
const { birdCopyDescriptor } = await vite.ssrLoadModule(`${source}/settings/copy/features/bird.ts`);
const { stationCopyDescriptor } = await vite.ssrLoadModule(`${source}/settings/copy/features/station.ts`);
const { towersCopyDescriptor } = await vite.ssrLoadModule(`${source}/settings/copy/features/towers.ts`);
after(() => vite.close());
const main = { id: 10, kingdomId: 0, name: 'Synthetic Main', units: { stationed: { 1: 100 } }, resources: {} };
const storm = { ...main, id: 20, kingdomId: 4, name: 'Synthetic Storm' };
const state = { castles: { 10: main, 20: storm } };
const reserve = [{ id: 1, amount: 37 }];
const metadata = { troops: { 1: { id: 1, name: 'Synthetic Unit' } }, tools: {}, metadataReady: true, observation: { session: { generation: 1, baselineGeneration: 1, changedAt: '2026-10-03T10:00:00Z' }, connected: true } };

test('role resolution, Save normalization and main entries preserve ownership', () => {
  const saved = { 10: reserve, 20: reserve, 99: reserve };
  const before = structuredClone(saved);
  assert.equal(role.castleSettingsKey(main), '10');
  assert.equal(role.castleSettingsKey(storm), 'storm');
  assert.equal(role.castleSettingsEntry(saved, storm), reserve);
  const normalized = role.normalizeStormKeys(saved, state);
  assert.deepEqual(normalized, { 10: reserve, 99: reserve, storm: reserve });
  assert.deepEqual(saved, before);
  assert.deepEqual(role.normalizeStormKeys({ ...saved, storm: [] }, state).storm, []);
  assert.equal(role.castleSettingsEntry({ 99: reserve }, storm), undefined);
  assert.deepEqual(role.castleSettingsEntry({ storm: [], 20: reserve }, storm), []);
  assert.equal(role.castleForSettingsKey('storm', { castles: { 10: main } }), undefined);
  assert.equal(role.stormEditorCastles([], true)[0].kingdomId, 4);
});

test('legacy repair is a draft choice, never overwrites, leaves cancel/save ownership to the editor', () => {
  const saved = { 10: reserve, 98: reserve, 99: [{ id: 1, amount: 45 }] };
  const before = structuredClone(saved);
  assert.deepEqual(role.legacyStormRepairKeys(saved, state), ['98', '99']);
  const draft = role.stormRepairDraft(saved, '98', state);
  assert.deepEqual(draft.storm, reserve);
  assert.equal(draft[98], undefined);
  assert.deepEqual(saved, before); // Cancel discards draft; no writer is involved.
  assert.deepEqual(role.legacyStormRepairKeys(draft, state), []);
  assert.deepEqual(role.stormRepairDraft(draft, '99', state), draft);
  assert.deepEqual(role.legacyStormRepairKeys({ 20: reserve, 99: reserve }, state), []);
  assert.deepEqual(role.stormRepairDraft(saved, '10', state), saved);
});

test('Storm event changes readiness and attribution without changing saved configuration', () => {
  const saved = { storm: reserve };
  for (const castle of [storm, { ...storm, id: 21 }]) {
    const live = { castles: { 10: main, [castle.id]: castle } };
    const reserveReport = setup.evaluateReserveReadiness({ ...metadata, state: live, reserves: saved, featureId: 'autoBird' });
    assert.deepEqual(reserveReport.castlesNotInWorld, []);
    assert.ok(reserveReport.stockByCastle.storm);
    const towerReport = setup.evaluateTowerReadiness({ ...metadata, state: live, castles: { storm: { enabled: true, unitId: 1 } } });
    assert.ok(towerReport.stockByCastle.storm);
    assert.ok(!towerReport.report.checks.some((check) => check.messageKey === 'setupReadiness.castlesNotInWorld'));
    const sections = { 'automation.autoBird': { ignoreSettings: { settings: saved } }, 'automation.autoStation': { settings: saved }, 'automation.autoTowers': { castles: { storm: { enabled: true } } }, 'automation.autoStorm': { unlock: { enabled: true, prebuiltCastleId: 999 } } };
    assert.deepEqual(automationsActingOnCastle(sections, castle.id, 4), ['autoTowers', 'autoStation', 'autoBird', 'autoStorm']);
    assert.deepEqual(automationsActingOnCastle(sections, 10, 0), []);
    assert.equal(castleCandidates([castle], live, { keyFor: role.castleSettingsKey })[0].key, 'storm');
  }
  assert.deepEqual(saved, { storm: reserve });
  const absent = setup.evaluateReserveReadiness({ ...metadata, state: { castles: { 10: main } }, reserves: saved, featureId: 'autoBird' });
  assert.deepEqual(absent.castlesNotInWorld, []);
  assert.equal(parseAutoStationClientState({ settings: saved }).settings.storm.length, 1);
  assert.equal(parseAutoTowerClientState({ castles: { storm: { enabled: true } } }).castles.storm.enabled, true);
});


test('Auto Bird Storm reserve check is pending, role-aware and confined to owned Storm castles', () => {
  const evaluate = (reserves, live = state, featureId = 'autoBird') => setup.evaluateReserveReadiness({ ...metadata, state: live, reserves, featureId }).report;
  const guard = (report) => report.checks.find((check) => check.id === 'storm-reserve');
  for (const reserves of [{}, { storm: [] }, { storm: [{ id: 1, amount: 0 }] }, { storm: [{ id: 0, amount: 1 }] }, { storm: [{ id: 1, amount: -1 }] }, { 20: [] }, { storm: [], 20: reserve }, { 10: [] }]) {
    const report = evaluate(reserves);
    assert.deepEqual(guard(report), { id: 'storm-reserve', state: 'pending', messageKey: 'stormRole.birdUnconfigured', params: { castle: 'Synthetic Storm' }, fix: 'settings', slot: 'storm' });
    assert.equal(report.overall, 'pending', 'the Storm guard is non-blocking');
  }
  for (const reserves of [{ storm: [{ id: 1, amount: 1 }] }, { storm: reserve }, { 20: reserve }, { 10: [], storm: reserve }]) assert.equal(guard(evaluate(reserves)), undefined);
  for (const reserves of [{}, { 10: [] }, { storm: [] }]) {
    assert.equal(guard(evaluate(reserves, { castles: { 10: main } })), undefined, 'no owned Storm castle');
    assert.equal(guard(evaluate(reserves, state, 'autoStation')), undefined, 'Station has no Bird guard');
  }
  const unnamed = { ...storm, name: '' };
  assert.equal(guard(evaluate({}, { castles: { 20: unnamed } })).params.castle, 'castle 20');
});


test('all three editor copy descriptors read the current-ID legacy Storm draft as a configured role', () => {
  const context = { ...metadata, state, candidates: castleCandidates([main, storm], state, { keyFor: role.castleSettingsKey }) };
  for (const [descriptor, record] of [[birdCopyDescriptor, reserve], [stationCopyDescriptor, reserve], [towersCopyDescriptor, { enabled: true, unitId: 1, radius: 10 }]]) {
    const saved = { 20: record };
    const draft = role.normalizeStormKeys(saved, state);
    assert.ok(configuredSources(descriptor, draft, context).some((castle) => castle.key === 'storm'), descriptor.featureId);
    assert.equal(previewCastleCopy(descriptor, draft, 'storm', ['10'], context).sourceConfigured, true, descriptor.featureId);
    assert.deepEqual(saved, { 20: record }, 'copy read does not persist or alter the legacy map');
  }
});


test('Bird and Station Save mirror every map, move only the marked mirror and clear it without a Storm castle', () => {
  const maps = { storm: reserve, 10: [], 98: [{ id: 1, amount: 8 }], 99: [{ id: 1, amount: 9 }] };
  const bird = parseAutoBirdClientState({ ignoreSettings: { settings: maps }, presets: { version: 1, presets: [
    { id: 'synthetic-one', name: 'Synthetic One', settings: { ...maps, storm: [{ id: 1, amount: 41 }] } },
    { id: 'synthetic-zero', name: 'Synthetic Zero', settings: { ...maps, storm: [{ id: 1, amount: 0 }] } },
    { id: 'synthetic-empty', name: 'Synthetic Empty', settings: {} },
  ] } });
  const station = parseAutoStationClientState({ settings: maps });
  const original = structuredClone({ bird, station });
  assert.deepEqual(normalizeAutoStationStormSettings(parseAutoStationClientState({}), { castles: { 10: main } }).settings, {});
  assert.deepEqual(normalizeAutoBirdStormSettings(parseAutoBirdClientState({}), { castles: { 10: main } }).ignoreSettings.settings, {});
  const birdMaps = (saved) => [saved.ignoreSettings.settings, ...saved.presets.presets.map((preset) => preset.settings)];
  for (const [saved, normalize, getMaps] of [[bird, normalizeAutoBirdStormSettings, birdMaps], [station, normalizeAutoStationStormSettings, (saved) => [saved.settings]]]) {
    const first = normalize(saved, state);
    assert.equal(first.stormLegacyKey, '20');
    for (const map of getMaps(first)) assert.deepEqual(map[20], map.storm);
    assert.deepEqual(normalize(first, state), first, 'Save is idempotent');
    const next = normalize(first, { castles: { 10: main, 21: { ...storm, id: 21 } } });
    assert.equal(next.stormLegacyKey, '21');
    for (const [index, map] of getMaps(next).entries()) {
      assert.equal(map[20], undefined, 'only the marked old mirror is removed');
      assert.deepEqual(map[21], map.storm);
      assert.deepEqual(map[98], getMaps(first)[index][98], 'unmarked numeric keys survive');
      assert.deepEqual(map[99], getMaps(first)[index][99]);
      assert.deepEqual(map[10], getMaps(first)[index][10], 'main castle unchanged');
    }
    const absent = normalize(next, { castles: { 10: main } });
    assert.equal(Object.hasOwn(absent, 'stormLegacyKey'), false);
    for (const [index, map] of getMaps(absent).entries()) {
      assert.equal(map[21], undefined);
      assert.deepEqual(map.storm, getMaps(first)[index].storm);
      assert.deepEqual(map[98], getMaps(first)[index][98]);
      assert.deepEqual(map[99], getMaps(first)[index][99]);
    }
  }
  assert.deepEqual({ bird, station }, original, 'Save transformations never mutate the draft');
  const legacy = normalizeAutoStationStormSettings(parseAutoStationClientState({ settings: { 20: reserve, 99: reserve } }), state);
  assert.deepEqual(legacy.settings, { 20: reserve, 99: reserve, storm: reserve });
  assert.deepEqual(normalizeAutoBirdStormSettings(bird, state).presets.presets[1].settings.storm, [{ id: 1, amount: 0 }], 'zero rows are mirrored, not filtered');
  assert.equal(activateAutoBirdPreset(normalizeAutoBirdStormSettings(bird, state), 'synthetic-one').stormLegacyKey, '20');
  for (const marker of [20, 'storm', '0', 'invalid']) {
    assert.equal(parseAutoBirdClientState({ stormLegacyKey: marker }).stormLegacyKey, undefined);
    assert.equal(parseAutoStationClientState({ stormLegacyKey: marker }).stormLegacyKey, undefined);
  }
});

test('marker mirrors are absent from readiness, attribution and repair while unmarked keys remain', () => {
  const entries = { storm: reserve, 99: reserve, 98: reserve };
  const report = setup.evaluateReserveReadiness({ ...metadata, state, reserves: entries, stormLegacyKey: '99', featureId: 'autoBird' });
  assert.deepEqual(report.castlesNotInWorld, ['98']);
  assert.equal(report.stockByCastle['99'], undefined);
  assert.deepEqual(role.legacyStormRepairKeys({ 99: reserve, 98: reserve }, state, '99'), ['98']);
  assert.deepEqual(role.legacyStormRepairKeys({ 99: reserve }, state, '99'), []);
  assert.deepEqual(role.stormRepairDraft({ 99: reserve }, '99', state, '99'), { 99: reserve });
  const sections = { 'automation.autoBird': { stormLegacyKey: '99', ignoreSettings: { settings: entries } }, 'automation.autoStation': { stormLegacyKey: '99', settings: entries } };
  assert.deepEqual(automationsActingOnCastle(sections, 99, 0), []);
  assert.deepEqual(automationsActingOnCastle(sections, 98, 0), ['autoStation', 'autoBird']);
  assert.deepEqual(automationsActingOnCastle(sections, 20, 4), ['autoStation', 'autoBird']);
  const context = { ...metadata, state, candidates: castleCandidates([main, storm], state, { keyFor: role.castleSettingsKey }) };
  assert.ok(configuredSources(birdCopyDescriptor, { storm: [{ id: 1, amount: 0 }] }, context).some((castle) => castle.key === 'storm'), 'generic copy continues to accept zero rows');
});

test('new-client Save matches the shared old-reader golden fixture', () => {
  const golden = JSON.parse(readFileSync(`${root}/tests/fixtures/storm-role-dual-write.json`, 'utf8'));
  for (const [key, parse, normalize] of [['automation.autoBird', parseAutoBirdClientState, normalizeAutoBirdStormSettings], ['automation.autoStation', parseAutoStationClientState, normalizeAutoStationStormSettings]]) {
    const source = structuredClone(golden[key]);
    delete source.stormLegacyKey;
    const maps = key === 'automation.autoBird' ? [source.ignoreSettings.settings, ...source.presets.presets.map((preset) => preset.settings)] : [source.settings];
    for (const map of maps) delete map[20];
    assert.deepEqual(normalize(parse(source), state), golden[key]);
  }
  assert.deepEqual(role.normalizeStormKeys({ 20: { enabled: true }, storm: { enabled: true } }, state), { storm: { enabled: true } }, 'Towers remains role-only');
});


for (const feature of ['autoBird', 'autoStation']) {
  test(`${feature} Save preserves explicit Storm removal, clears only marked mirrors and retains other preset rows`, () => {
    const draft = { 10: [], 20: reserve, 98: [{ id: 1, amount: 8 }], 99: [{ id: 1, amount: 9 }] };
    const parse = feature === 'autoBird' ? parseAutoBirdClientState : parseAutoStationClientState;
    const normalize = feature === 'autoBird' ? normalizeAutoBirdStormSettings : normalizeAutoStationStormSettings;
    const raw = feature === 'autoBird'
      ? { stormLegacyKey: '20', ignoreSettings: { settings: draft }, presets: { version: 1, presets: [{ id: 'synthetic-removed', name: 'Synthetic removed', settings: draft }] } }
      : { stormLegacyKey: '20', settings: draft };
    const original = structuredClone(raw);
    for (const live of [state, { castles: { 10: main } }]) {
      const saved = normalize(parse(raw), live);
      assert.equal(saved.stormLegacyKey, undefined, 'removal leaves no marker');
      const maps = feature === 'autoBird' ? [saved.ignoreSettings.settings, ...saved.presets.presets.map((preset) => preset.settings)] : [saved.settings];
      for (const map of maps) assert.deepEqual(map, { 10: [], 98: draft[98], 99: draft[99] });
      assert.deepEqual(normalize(parse(saved), live), saved, 'Save/reopen/Save does not recreate the role');
    }
    assert.deepEqual(raw, original, 'normalization never mutates the removed draft');
    if (feature === 'autoBird') {
      const mixed = parse(raw);
      mixed.presets.presets.push({ id: 'synthetic-kept', name: 'Synthetic kept', settings: { storm: [{ id: 1, amount: 0 }], 20: reserve, 99: reserve } });
      const saved = normalize(mixed, state);
      assert.equal(saved.stormLegacyKey, '20', 'retained preset keeps its compatibility mirror marker');
      assert.equal(Object.hasOwn(saved.ignoreSettings.settings, 'storm'), false);
      assert.equal(Object.hasOwn(saved.presets.presets[0].settings, 'storm'), false);
      assert.deepEqual(saved.presets.presets[1].settings, { storm: [{ id: 1, amount: 0 }], 20: [{ id: 1, amount: 0 }], 99: reserve });
    }
  });
}
