import assert from 'node:assert/strict';
import { after, test } from 'node:test';
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

after(async () => {
  await vite.close();
});

function castle(id, kingdomId, slotType, extra = {}) {
  return { id, kingdomId, slotType, name: `C${id}`, x: 1, y: 2, units: { stationed: {} }, ...extra };
}

const state = {
  account: { uid: 5, worldId: 'EmpireEx_21' },
  castles: {
    1: castle(1, 0, 1, { unitsObservedAt: '0001-01-01T00:00:00Z' }),
    2: castle(2, 0, 4, { unitsObservedAt: '2026-07-31T09:00:00Z' }),
    11: castle(11, 1, 12),
    12: castle(12, 1, 3),
    31: castle(31, 10, 17),
  },
};

test('purposes match the castles each module offers today', () => {
  assert.deepEqual(castles.castleOptionsFor(state, 'source-great-empire').map((option) => option.id), [1, 2]);
  assert.deepEqual(castles.castleOptionsFor(state, 'outer-main').map((option) => option.id), [11]);
  assert.deepEqual(castles.castleOptionsFor(state, 'berimond').map((option) => option.id), [31]);
  assert.equal(castles.castleOptionsFor(state, 'any-owned').length, 5);
  assert.deepEqual(castles.castleOptionsFor(null, 'any-owned'), []);
});

test('castle references report missing, wrong kind, unobserved and valid', () => {
  const check = (castleId, purpose, requireObservedUnits) => castles.evaluateCastleReference({ castleId, state, purpose, requireObservedUnits });
  const session = { generation: 25, baselineGeneration: 25, changedAt: '2026-09-29T09:00:00Z' };
  const live = { session, connected: true };
  assert.equal(castles.evaluateCastleReference({ castleId: 1, state: null, purpose: 'any-owned' }).state, 'unavailable');
  assert.equal(check(0, 'source-great-empire').state, 'blocked');
  assert.equal(check(0, 'source-great-empire').fix, 'settings');
  assert.equal(check(404, 'source-great-empire').state, 'blocked');
  assert.equal(check(11, 'source-great-empire').state, 'blocked');
  assert.equal(check(12, 'outer-main').state, 'blocked');
  // D1: zero-time counts follow the session; a real time before this connection is stale.
  assert.equal(check(1, 'source-great-empire', live).state, 'valid');
  assert.equal(check(2, 'source-great-empire', live).state, 'unavailable');
  assert.equal(check(1, 'source-great-empire', { session, connected: false }).state, 'unavailable');
  assert.equal(check(1, 'source-great-empire', { session, connected: false }).fix, 'connection');
  assert.equal(check(1, 'source-great-empire').state, 'valid');
  assert.equal(check(31, 'berimond').state, 'valid');
  assert.equal(castles.evaluateCastleReference({ castleId: 1, state, purpose: 'any-owned', id: 'donor' }).id, 'donor');
});

test('account key and reference validity', () => {
  assert.equal(castles.accountKey(state), '5:EmpireEx_21');
  assert.equal(castles.accountKey(null), '');
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [1, 0, 404] }, state), { valid: false, missing: [404], unobserved: false });
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [1, 11] }, state), { valid: true, missing: [], unobserved: false });
  // Zero observed castles is waiting for data, never "reselect" (CIT-18 QA).
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [404] }, { castles: {} }), { valid: false, missing: [], unobserved: true });
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [404] }, null), { valid: false, missing: [], unobserved: true });
  const waiting = castles.evaluateCastleReference({ castleId: 404, state: { castles: {} }, purpose: 'source-great-empire' });
  assert.equal(waiting.state, 'unavailable');
  assert.equal(waiting.fix, 'connection');
  assert.equal(castles.evaluateCastleReference({ castleId: 0, state: { castles: {} }, purpose: 'source-great-empire' }).state, 'blocked', 'no castle chosen is still a setting');
});
