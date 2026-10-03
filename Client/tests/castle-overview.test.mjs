import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const { NEAR_CAP_RATIO, resourceRows, troopTotals, castleDataAge } = await vite.ssrLoadModule(`${source}/dashboard/castleOverview.ts`);
after(() => vite.close());

test('near-cap threshold is inclusive at 90%, and missing or zero capacity is never near cap', () => {
  assert.equal(NEAR_CAP_RATIO, 0.9);
  for (const [amount, nearCap] of [[89, false], [90, true], [100, true], [120, true]]) {
    const [row] = resourceRows({ resources: { 1: { amount, capacity: 100 } } }, {});
    assert.equal(row.ratio, amount / 100);
    assert.equal(row.nearCap, nearCap);
  }
  for (const capacity of [undefined, 0]) {
    const [row] = resourceRows({ resources: { 1: { amount: 100, capacity } } }, {});
    assert.equal(row.capacity, capacity);
    assert.equal(row.ratio, 0);
    assert.equal(row.nearCap, false);
  }
});

test('resources preserve descending id order, metadata fallbacks and signed hourly production', () => {
  const castle = { resources: {
    1: { amount: 15, capacity: 100, productionPerHour: 12.5 },
    2: { amount: 20, productionPerHour: -7.5 },
    3: { amount: 0, capacity: 100, productionPerHour: 0 },
    4: { amount: 1 },
    0: { amount: 99 }, invalid: { amount: 99 },
  } };
  const before = structuredClone(castle);
  const rows = resourceRows(castle, { 1: { id: 1, name: 'Wood', image: '/wood.png' }, 2: { id: 2, name: '', internalName: 'Food' } });
  assert.deepEqual(rows.map((row) => row.id), [4, 3, 2, 1]);
  assert.deepEqual(rows.map((row) => row.perHour), [0, 0, -7.5, 12.5]);
  assert.equal(rows[2].name, 'Food', 'food remains present without capacity');
  assert.equal(rows[0].name, 'Resource 4');
  assert.equal(rows[3].icon, '/wood.png');
  assert.equal(rows[3].amount, 15);
  assert.deepEqual(castle, before);
});

test('troop totals sum every unit id and both hospitals without using aggregate total', () => {
  assert.deepEqual(troopTotals({ stationed: { 1: 10, 2: 20 }, traveling: { 1: 4, 3: 8 }, hospital: { 1: 3 }, specialHospital: { 1: 7, 2: 2 }, total: { 1: 9999 } }),
    { stationed: 30, traveling: 12, hospital: 12 });
  assert.deepEqual(troopTotals({ stationed: {}, traveling: {}, hospital: {}, specialHospital: {}, total: {} }),
    { stationed: 0, traveling: 0, hospital: 0 });
});

const at = '2026-09-30T12:00:00Z';
const zero = '0001-01-01T00:00:00Z';
test('live data age requires connection and no checkpoint', () => {
  assert.deepEqual(castleDataAge({ connected: true, castle: null }), { kind: 'live' });
  assert.deepEqual(castleDataAge({ presence: { mode: 'live' }, connected: true, castle: { unitsObservedAt: zero } }), { kind: 'live' });
});

test('checkpoint time wins over connection and castle observations, and invalid checkpoint is unknown', () => {
  const castle = { unitsObservedAt: '2026-09-30T15:00:00Z' };
  assert.deepEqual(castleDataAge({ presence: { mode: 'checkpoint', checkpointObservedAt: at }, connected: true, castle }), { kind: 'saved', at });
  for (const checkpointObservedAt of [undefined, zero, 'invalid']) {
    assert.deepEqual(castleDataAge({ presence: { mode: 'checkpoint', checkpointObservedAt }, connected: false, castle }), { kind: 'saved' });
  }
});

test('disconnected data age chooses latest real observation by instant, ignoring zero and invalid times', () => {
  assert.deepEqual(castleDataAge({ connected: false, castle: { unitsObservedAt: zero, foodStateObservedAt: at, contextSnapshotObservedAt: 'invalid' } }), { kind: 'saved', at });
  const latest = '2026-09-30T09:00:00-04:00';
  assert.deepEqual(castleDataAge({ connected: false, castle: { unitsObservedAt: at, foodStateObservedAt: latest, contextSnapshotObservedAt: '2026-09-30T12:30:00Z' } }), { kind: 'saved', at: latest });
  assert.deepEqual(castleDataAge({ connected: false, castle: null }), { kind: 'saved' });
  assert.deepEqual(castleDataAge({ connected: false, castle: { unitsObservedAt: zero, foodStateObservedAt: 'invalid' } }), { kind: 'saved' });
});
