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
const units = await vite.ssrLoadModule('/src/settings/requirements/unitRequirements.ts');

after(async () => {
  await vite.close();
});

const troops = { 1: { id: 1, name: 'A' }, 2: { id: 2, name: 'B' } };
const tools = { 500: { id: 500, name: 'Ladder' } };
// The dashboard projection zeroes unitsObservedAt (Go zero time); freshness comes from the session (CIT-15 D1).
const ZERO_TIME = '0001-01-01T00:00:00Z';
const castle = { id: 7, kingdomId: 0, units: { stationed: { 1: 100, 2: 5, 500: 10 } }, unitsObservedAt: ZERO_TIME };
const SESSION = { generation: 25, baselineGeneration: 25, changedAt: '2026-09-29T09:00:00Z' };
const LIVE = { session: SESSION, connected: true };

function stock(requests, extra = {}) {
  return units.evaluateUnitStock({ castle, observation: LIVE, requests, troops, tools, metadataReady: true, ...extra });
}

test('required amounts: valid, short (pending) and missing (blocked) with per-line states', () => {
  const valid = stock([{ itemId: 1, amount: 100, kind: 'troop' }, { itemId: 500, amount: 10, kind: 'tool' }]);
  assert.equal(valid.check.state, 'valid');
  assert.deepEqual(valid.lines.map((line) => line.state), ['valid', 'valid']);
  const short = stock([{ itemId: 2, amount: 6, kind: 'troop' }]);
  assert.equal(short.check.state, 'pending');
  assert.deepEqual(short.lines[0], { itemId: 2, kind: 'troop', required: 6, stationed: 5, state: 'short' });
  const missing = stock([{ itemId: 3, amount: 1, kind: 'troop' }]);
  assert.equal(missing.check.state, 'blocked');
  assert.equal(missing.lines[0].state, 'missing');
});

test('repeated requests are merged before comparing', () => {
  const merged = stock([{ itemId: 2, amount: 3, kind: 'troop' }, { itemId: 2, amount: 3, kind: 'troop' }]);
  assert.equal(merged.lines.length, 1);
  assert.equal(merged.lines[0].required, 6);
  assert.equal(merged.check.state, 'pending');
});

test('decided at launch: quantity keeps missing blocked, stock refills make it pending', () => {
  const pendingKey = 'eventAttackReadiness.inventoryShort';
  assert.equal(stock([{ itemId: 3, amount: 1, kind: 'troop' }], { decidedAtLaunch: 'quantity' }).check.state, 'blocked');
  assert.equal(stock([{ itemId: 2, amount: 9, kind: 'troop' }], { decidedAtLaunch: 'quantity', messages: { decidedAtLaunch: pendingKey } }).check.messageKey, pendingKey);
  assert.equal(stock([{ itemId: 3, amount: 1, kind: 'troop' }], { decidedAtLaunch: 'stock' }).check.state, 'pending');
});

test('no castle and loading family data are unavailable', () => {
  assert.equal(units.evaluateUnitStock({ castle: null, observation: LIVE, requests: [], troops, tools, metadataReady: true }).check.state, 'unavailable');
  assert.equal(stock([{ itemId: 1, amount: 1, kind: 'troop' }], { useTroopFamilies: true, metadataReady: false }).check.state, 'unavailable');
});

test('D1: zero-time counts are compared only while this connection is current', () => {
  const request = [{ itemId: 1, amount: 1, kind: 'troop' }];
  const live = stock(request);
  assert.equal(live.check.state, 'valid');
  assert.deepEqual(live.freshness, { state: 'observed', scope: 'session', since: SESSION.changedAt });
  const cases = [
    [{ session: SESSION, connected: false }, 'ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99'],
    [{ session: { ...SESSION, baselineGeneration: 24 }, connected: true }, 'ui.settings.requirements.observationFreshness.waiting.for.the.game.connection.to.finish.c661a838'],
    [{ ...LIVE, hostedPresence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-29T08:00:00Z' } }, 'ui.settings.requirements.observationFreshness.this.is.a.saved.checkpoint.troop.counts.4a2b7024'],
  ];
  for (const [observation, messageKey] of cases) {
    const result = stock(request, { observation });
    assert.equal(result.check.state, 'unavailable');
    assert.equal(result.check.messageKey, messageKey);
    assert.equal(result.check.fix, 'connection');
    assert.deepEqual(result.lines, [], 'no stale per-line counts are shown');
  }
  const stale = stock(request, { castle: { ...castle, unitsObservedAt: '2026-07-31T09:00:00Z' } });
  assert.equal(stale.check.state, 'unavailable');
  assert.equal(stale.freshness.reason, 'stale-before-connection');
  const fresh = stock(request, { castle: { ...castle, unitsObservedAt: '2026-09-29T10:00:00Z' } });
  assert.equal(fresh.freshness.scope, 'castle');
});

test('reserves: above stock is pending, unknown units block, covered reserves are valid', () => {
  const covered = stock([{ itemId: 1, amount: 50, kind: 'troop' }], { mode: 'reserve' });
  assert.equal(covered.check.state, 'valid');
  const above = stock([{ itemId: 2, amount: 50, kind: 'troop' }], { mode: 'reserve' });
  assert.equal(above.check.state, 'pending');
  assert.equal(above.lines[0].state, 'short');
  const unknown = stock([{ itemId: 99, amount: 1, kind: 'troop' }], { mode: 'reserve' });
  assert.equal(unknown.check.state, 'blocked');
  assert.equal(unknown.lines[0].state, 'unknown');
});

test('composition requests include waves and courtyard support', () => {
  const lane = (troops, tools = []) => ({ troops: troops.map(([itemId, quantity]) => ({ itemId, quantity })), tools: tools.map(([itemId, quantity]) => ({ itemId, quantity })) });
  const requests = units.requestsFromComposition({
    name: '',
    waves: [{ L: lane([[1, 5]], [[500, 2]]), M: lane([[1, 3]]), R: lane([]) }],
    courtyardSupport: { troops: [{ itemId: 2, quantity: 4 }], tools: [{ itemId: 500, quantity: 0 }] },
  });
  assert.deepEqual(requests, [
    { itemId: 1, amount: 5, kind: 'troop' },
    { itemId: 500, amount: 2, kind: 'tool' },
    { itemId: 1, amount: 3, kind: 'troop' },
    { itemId: 2, amount: 4, kind: 'troop' },
    { itemId: 500, amount: 1, kind: 'tool' },
  ]);
});
