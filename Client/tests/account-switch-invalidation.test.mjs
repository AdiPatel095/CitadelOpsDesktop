import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const castles = await vite.ssrLoadModule('/src/settings/requirements/castleRequirements.ts');
const setup = await vite.ssrLoadModule('/src/settings/requirements/setupReadiness.ts');

after(async () => {
  await vite.close();
});

const worldA = { account: { uid: 1, worldId: 'A' }, castles: { 7: { id: 7, kingdomId: 0, units: { stationed: { 1: 10 } }, unitsObservedAt: 'x' } } };
const worldB = { account: { uid: 1, worldId: 'B' }, castles: { 9: { id: 9, kingdomId: 0, units: { stationed: {} }, unitsObservedAt: 'x' } } };
const metadata = { troops: { 1: { id: 1, name: 'A' } }, tools: {}, metadataReady: true };

test('the draft session key changes only on a real account/world switch', () => {
  let tracked = { key: '', generation: 0 };
  tracked = castles.advanceAccountSession(tracked, '');
  assert.deepEqual(tracked, { key: '', generation: 0 });
  tracked = castles.advanceAccountSession(tracked, castles.accountKey(worldA));
  assert.deepEqual(tracked, { key: '1:A', generation: 0 }, 'the first observation after open is not a switch');
  const same = castles.advanceAccountSession(tracked, '1:A');
  assert.equal(same, tracked);
  tracked = castles.advanceAccountSession(tracked, castles.accountKey(worldB));
  assert.deepEqual(tracked, { key: '1:B', generation: 1 });
  assert.equal(castles.advanceAccountSession(tracked, ''), tracked, 'a disconnect is not a switch');
});

test('saved castle references from another world are reported for reselection, never cleared', () => {
  const check = castles.evaluateCastleReference({ castleId: 7, state: worldB, purpose: 'source-great-empire' });
  assert.equal(check.state, 'blocked');
  assert.equal(check.fix, 'settings');
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [7] }, worldB), { valid: false, missing: [7] });
  const towers = setup.evaluateTowerReadiness({ state: worldB, castles: { 7: { enabled: true, unitId: 1, maidenOnly: false } }, ...metadata });
  assert.equal(towers.report.checks.find((entry) => entry.id === 'enabled-castles').state, 'blocked');
  assert.equal(towers.report.overall, 'blocked');
  const reserves = setup.evaluateReserveReadiness({ featureId: 'autoStation', state: worldB, reserves: { 7: [{ id: 1, amount: 5 }] }, ...metadata });
  assert.deepEqual(reserves.castlesNotInWorld, ['7']);
  assert.equal(reserves.report.checks.find((entry) => entry.id === 'saved-castles').state, 'pending');
});

test('modals key their draft session on the account and never auto-clear castle references', async () => {
  const files = ['AutoNomadSettingsModal', 'AutoInvasionSettingsModal', 'AutoBeriWorldSettingsModal', 'AutoTowerSettingsModal', 'AutoFortressSettingsModal', 'AutoStationSettingsModal', 'AutoBirdSettingsModal', 'AutoFoodBalanceSettingsModal'];
  for (const file of files) {
    const source = await readFile(new URL(`../src/settings/components/${file}.tsx`, import.meta.url), 'utf8');
    assert.match(source, /useSetupContext\(/, file);
    assert.match(source, /sessionKey: setup\.sessionKey/, file);
  }
  const field = await readFile(new URL('../src/settings/components/CastleRequirementField.tsx', import.meta.url), 'utf8');
  assert.doesNotMatch(field, /onChange\(0\)/, 'the field never clears the saved castle by itself');
});
