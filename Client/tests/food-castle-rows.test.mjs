import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const setup = await vite.ssrLoadModule('/src/settings/requirements/setupReadiness.ts');
const { FoodCastleTableRow } = await vite.ssrLoadModule('/src/settings/components/FoodCastleTableRow.tsx');

after(async () => {
  await vite.close();
});

const resources = { 1: { id: 1, name: 'currency1', JSONKey: 'C1' }, 5: { id: 5, name: 'food', JSONKey: 'F' } };
const SESSION = { generation: 25, baselineGeneration: 25, changedAt: '2026-09-29T09:00:00Z' };
const castle = (id, kingdomId, food, extra = {}) => ({
  id, kingdomId, slotType: 1, name: `Castle ${id}`, units: { stationed: {} }, unitsObservedAt: '0001-01-01T00:00:00Z',
  resources: food == null ? {} : { 5: { amount: food } }, ...extra,
});
const evaluate = (observation, castles) => setup.evaluateFoodBalanceReadiness({
  state: { player: { resources: { 1: 500 } }, kingdomTransport: { unlocks: {} }, castles }, resources, metadataReady: true,
  minimumSourceReserve: 1000, minimumCoinReserve: 100, autoKingdomTransport: false, observation,
});
const cells = (row) => renderToStaticMarkup(createElement('table', null, createElement('tbody', null, createElement(FoodCastleTableRow, { row }))));
const cellTexts = (html) => [...html.matchAll(/<td[^>]*>(.*?)<\/td>/g)].map((match) => match[1].replace(/<[^>]+>/g, '').trim());
const LIVE = { session: SESSION, connected: true };

const world = {
  1: castle(1, 0, 5000),
  2: castle(2, 0, 200, { foodStateObservedAt: '2026-09-29T11:00:00Z' }),
  3: castle(3, 1, null),
};

test('every row has a role and a non-empty role cell, in every connection state', () => {
  const states = [
    LIVE,
    { session: SESSION, connected: false },
    { session: SESSION, connected: true, hostedPresence: { mode: 'checkpoint', checkpointObservedAt: '2026-09-29T08:00:00Z' } },
    { session: { ...SESSION, baselineGeneration: 24 }, connected: true },
  ];
  for (const observation of states) {
    const { rows } = evaluate(observation, world);
    assert.equal(rows.length, 3);
    for (const row of rows) {
      assert.ok(['donor', 'recipient', 'unobserved'].includes(row.role), `role ${row.role}`);
      const texts = cellTexts(cells(row));
      assert.equal(texts.length, 4, 'name, food, role, last checked');
      assert.ok(texts.every((text) => text.length > 0), `no blank cell: ${JSON.stringify(texts)}`);
      assert.match(texts[2], /^(Donor|Below reserve|Not observed)$/);
    }
  }
});

test('a disconnected fixture shows every row as last known with the reason, never as checked now', () => {
  const { rows } = evaluate({ session: SESSION, connected: false }, world);
  assert.ok(rows.every((row) => !row.current && row.unavailableReason), 'all rows last-known with a reason');
  for (const row of rows) {
    const html = cells(row);
    assert.match(html, /data-food-checked="last-known"/);
    assert.match(cellTexts(html)[3], /^Last known/);
    assert.doesNotMatch(cellTexts(html)[3], /This connection/);
  }
  const withTime = cellTexts(cells(rows.find((row) => row.castleId === 2)))[3];
  assert.match(withTime, /^Last known · /, 'the castle\'s own time follows when the game reported one');
});

test('a current row shows the game time when reported and "This connection" otherwise', () => {
  const { rows } = evaluate(LIVE, world);
  assert.ok(rows.every((row) => row.current));
  const timed = cellTexts(cells(rows.find((row) => row.castleId === 2)))[3];
  assert.doesNotMatch(timed, /Last known|This connection/);
  assert.match(timed, /\d/);
  assert.equal(cellTexts(cells(rows.find((row) => row.castleId === 1)))[3], 'This connection');
  assert.match(cells(rows[0]), /data-food-checked="current"/);
});

test('a castle time from before this connection is last known even while connected', () => {
  const { rows } = evaluate(LIVE, { 1: castle(1, 0, 5000, { foodStateObservedAt: '2026-09-29T08:00:00Z' }) });
  assert.equal(rows[0].current, false);
  assert.match(cells(rows[0]), /data-food-checked="last-known"/);
  assert.match(cellTexts(cells(rows[0]))[3], /^Last known · .+/);
});
