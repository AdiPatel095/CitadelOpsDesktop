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
const castle = { id: 7, kingdomId: 0, units: { stationed: { 1: 100, 2: 5, 500: 10 } }, unitsObservedAt: '2026-09-29T10:00:00Z' };

function stock(requests, extra = {}) {
  return units.evaluateUnitStock({ castle, requests, troops, tools, metadataReady: true, ...extra });
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

test('unobserved castle and loading family data are unavailable', () => {
  assert.equal(units.evaluateUnitStock({ castle: { ...castle, unitsObservedAt: undefined }, requests: [], troops, tools, metadataReady: true }).check.state, 'unavailable');
  assert.equal(units.evaluateUnitStock({ castle: null, requests: [], troops, tools, metadataReady: true }).check.state, 'unavailable');
  assert.equal(stock([{ itemId: 1, amount: 1, kind: 'troop' }], { useTroopFamilies: true, metadataReady: false }).check.state, 'unavailable');
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
