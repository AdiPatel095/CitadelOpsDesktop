import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const role = await vite.ssrLoadModule(`${source}/settings/stormRole.ts`);
const setup = await vite.ssrLoadModule(`${source}/settings/requirements/setupReadiness.ts`);
const { parseAutoStationClientState } = await vite.ssrLoadModule(`${source}/settings/AutoStationClientState.ts`);
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
  for (const reserves of [{}, { storm: [] }, { 20: [] }, { storm: [], 20: reserve }, { 10: [] }]) {
    const report = evaluate(reserves);
    assert.deepEqual(guard(report), { id: 'storm-reserve', state: 'pending', messageKey: 'stormRole.birdUnconfigured', params: { castle: 'Synthetic Storm' }, fix: 'settings', slot: 'storm' });
    assert.equal(report.overall, 'pending', 'the Storm guard is non-blocking');
  }
  for (const reserves of [{ storm: reserve }, { 20: reserve }, { 10: [], storm: reserve }]) assert.equal(guard(evaluate(reserves)), undefined);
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
