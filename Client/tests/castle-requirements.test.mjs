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
    1: castle(1, 0, 1, { unitsObservedAt: '2026-09-29T10:00:00Z' }),
    2: castle(2, 0, 4),
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
  assert.equal(castles.evaluateCastleReference({ castleId: 1, state: null, purpose: 'any-owned' }).state, 'unavailable');
  assert.equal(check(0, 'source-great-empire').state, 'blocked');
  assert.equal(check(0, 'source-great-empire').fix, 'settings');
  assert.equal(check(404, 'source-great-empire').state, 'blocked');
  assert.equal(check(11, 'source-great-empire').state, 'blocked');
  assert.equal(check(12, 'outer-main').state, 'blocked');
  assert.equal(check(2, 'source-great-empire', true).state, 'unavailable');
  assert.equal(check(1, 'source-great-empire', true).state, 'valid');
  assert.equal(check(31, 'berimond').state, 'valid');
  assert.equal(castles.evaluateCastleReference({ castleId: 1, state, purpose: 'any-owned', id: 'donor' }).id, 'donor');
});

test('account key and reference validity', () => {
  assert.equal(castles.accountKey(state), '5:EmpireEx_21');
  assert.equal(castles.accountKey(null), '');
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [1, 0, 404] }, state), { valid: false, missing: [404] });
  assert.deepEqual(castles.draftReferencesValid({ castleIds: [1, 11] }, state), { valid: true, missing: [] });
});
